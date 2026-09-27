package query

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Display-name resolution for change entries and merge conflicts (#697).
//
// Naming an entry used to cost one single-row read-model Get per entry, so one
// history/compare response could become hundreds of round trips. Resolution is
// now two-phase: a caller first registers every entity it will need a name for
// (entityRefs), then resolveEntityNames reads each entity type with ONE batched
// ReadModelStore lookup, and the names are served from memory. The number of
// read-model queries is therefore bounded by the number of entity types (four),
// not by the number of entries.

// Entity-type vocabulary shared with ChangeEntry.EntityType.
const (
	entityTypePerson   = "person"
	entityTypeFamily   = "family"
	entityTypeSource   = "source"
	entityTypeCitation = "citation"
)

// entityRefs collects, per entity type, the ids whose display names a batch of
// entries needs.
type entityRefs map[string]map[uuid.UUID]struct{}

func newEntityRefs() entityRefs {
	return entityRefs{}
}

// add registers one entity. Types without a read-model name are ignored: their
// name is always the id fallback, so there is nothing to fetch.
func (r entityRefs) add(entityType string, id uuid.UUID) {
	switch entityType {
	case entityTypePerson, entityTypeFamily, entityTypeSource, entityTypeCitation:
	default:
		return
	}
	ids, ok := r[entityType]
	if !ok {
		ids = make(map[uuid.UUID]struct{})
		r[entityType] = ids
	}
	ids[id] = struct{}{}
}

// addEvent registers the entity an event is about plus every other entity its
// name or change summary may need: the child of a link/unlink (named in the
// change summary) and the partners of a FamilyCreated (the family's name when
// the family itself is gone from the read model). Registering the partners up
// front even when the family will resolve keeps resolution to a single round
// per type instead of a second, dependent one.
func (r entityRefs) addEvent(entityType string, entityID uuid.UUID, evt *repository.StoredEvent) {
	r.add(entityType, entityID)
	if evt == nil {
		return
	}
	switch evt.EventType {
	case "ChildLinkedToFamily", "ChildUnlinkedFromFamily":
		var link struct {
			PersonID uuid.UUID `json:"person_id"`
		}
		if err := json.Unmarshal(evt.Data, &link); err == nil && link.PersonID != uuid.Nil {
			r.add(entityTypePerson, link.PersonID)
		}
	case "FamilyCreated":
		var created domain.FamilyCreated
		if err := json.Unmarshal(evt.Data, &created); err == nil {
			if created.Partner1ID != nil {
				r.add(entityTypePerson, *created.Partner1ID)
			}
			if created.Partner2ID != nil {
				r.add(entityTypePerson, *created.Partner2ID)
			}
		}
	}
}

// ids returns the registered ids of one type.
func (r entityRefs) ids(entityType string) []uuid.UUID {
	set := r[entityType]
	if len(set) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	return ids
}

// entityNames holds the read-model rows resolved for a set of entityRefs and
// derives display names from them, falling back to event data and finally to
// the id exactly as the per-entry lookups it replaces did.
type entityNames struct {
	persons   map[uuid.UUID]*repository.PersonReadModel
	families  map[uuid.UUID]*repository.FamilyReadModel
	sources   map[uuid.UUID]*repository.SourceReadModel
	citations map[uuid.UUID]*repository.CitationReadModel
}

// resolveEntityNames reads every registered entity with one batched lookup per
// entity type that has any ids (so at most four queries, whatever the number of
// refs). A lookup failure is returned, not papered over: an entity that merely
// no longer exists is absent from the batch and still gets its fallback name,
// but a store that cannot answer is an error the caller must see.
//
// Names resolve against main: the history and compare endpoints take no
// ?branch= parameter, so, as before, an entry is labelled with the mainline's
// current name (ADR-005). The batch methods honour branchID exactly like the
// single-row getters, so scoping this to a branch is a one-argument change.
func (s *HistoryService) resolveEntityNames(ctx context.Context, refs entityRefs) (*entityNames, error) {
	branchID := domain.MainBranchID
	names := &entityNames{}

	if ids := refs.ids(entityTypePerson); len(ids) > 0 {
		rows, err := s.readStore.GetPersonsByIDs(ctx, branchID, ids)
		if err != nil {
			return nil, fmt.Errorf("resolve person names: %w", err)
		}
		names.persons = indexByID(rows, func(p *repository.PersonReadModel) uuid.UUID { return p.ID })
	}
	if ids := refs.ids(entityTypeFamily); len(ids) > 0 {
		rows, err := s.readStore.GetFamiliesByIDs(ctx, branchID, ids)
		if err != nil {
			return nil, fmt.Errorf("resolve family names: %w", err)
		}
		names.families = indexByID(rows, func(f *repository.FamilyReadModel) uuid.UUID { return f.ID })
	}
	if ids := refs.ids(entityTypeSource); len(ids) > 0 {
		rows, err := s.readStore.GetSourcesByIDs(ctx, branchID, ids)
		if err != nil {
			return nil, fmt.Errorf("resolve source names: %w", err)
		}
		names.sources = indexByID(rows, func(src *repository.SourceReadModel) uuid.UUID { return src.ID })
	}
	if ids := refs.ids(entityTypeCitation); len(ids) > 0 {
		rows, err := s.readStore.GetCitationsByIDs(ctx, branchID, ids)
		if err != nil {
			return nil, fmt.Errorf("resolve citation names: %w", err)
		}
		names.citations = indexByID(rows, func(c *repository.CitationReadModel) uuid.UUID { return c.ID })
	}
	return names, nil
}

// indexByID maps each row to its id.
func indexByID[T any](rows []T, id func(*T) uuid.UUID) map[uuid.UUID]*T {
	index := make(map[uuid.UUID]*T, len(rows))
	for i := range rows {
		index[id(&rows[i])] = &rows[i]
	}
	return index
}

// name returns the display name of an entity. evt, when non-nil, is an event of
// the entity whose payload is the fallback when the read model has no row.
func (n *entityNames) name(entityType string, entityID uuid.UUID, evt *repository.StoredEvent) string {
	switch entityType {
	case entityTypePerson:
		return n.personName(entityID, evt)
	case entityTypeFamily:
		return n.familyName(entityID, evt)
	case entityTypeSource:
		return n.sourceName(entityID, evt)
	case entityTypeCitation:
		return n.citationName(entityID)
	default:
		return entityID.String()
	}
}

// personName returns a person's name from the read model, else from its
// creation event, else its id.
func (n *entityNames) personName(personID uuid.UUID, evt *repository.StoredEvent) string {
	if person := n.persons[personID]; person != nil {
		return person.FullName
	}

	if evt != nil && evt.EventType == "PersonCreated" {
		var created domain.PersonCreated
		if err := json.Unmarshal(evt.Data, &created); err == nil {
			if created.GivenName != "" || created.Surname != "" {
				return fmt.Sprintf("%s %s", created.GivenName, created.Surname)
			}
		}
	}

	return personID.String()
}

// familyName returns a family's partner names from the read model, else the
// partners named by its creation event, else its id.
func (n *entityNames) familyName(familyID uuid.UUID, evt *repository.StoredEvent) string {
	if family := n.families[familyID]; family != nil {
		p1Name := fullName(family.Partner1GivenName, family.Partner1Surname)
		p2Name := fullName(family.Partner2GivenName, family.Partner2Surname)
		if p1Name != "" && p2Name != "" {
			return fmt.Sprintf("%s & %s", p1Name, p2Name)
		}
		if p1Name != "" {
			return p1Name
		}
		if p2Name != "" {
			return p2Name
		}
	}

	if evt != nil && evt.EventType == "FamilyCreated" {
		var created domain.FamilyCreated
		if err := json.Unmarshal(evt.Data, &created); err == nil {
			names := make([]string, 0, 2)
			if created.Partner1ID != nil {
				names = append(names, n.personName(*created.Partner1ID, nil))
			}
			if created.Partner2ID != nil {
				names = append(names, n.personName(*created.Partner2ID, nil))
			}
			if len(names) == 2 {
				return fmt.Sprintf("%s & %s", names[0], names[1])
			}
			if len(names) == 1 {
				return names[0]
			}
		}
	}

	return familyID.String()
}

// sourceName returns a source's title from the read model, else from its
// creation event, else its id.
func (n *entityNames) sourceName(sourceID uuid.UUID, evt *repository.StoredEvent) string {
	if source := n.sources[sourceID]; source != nil {
		return source.Title
	}

	if evt != nil && evt.EventType == "SourceCreated" {
		var created domain.SourceCreated
		if err := json.Unmarshal(evt.Data, &created); err == nil && created.Title != "" {
			return created.Title
		}
	}

	return sourceID.String()
}

// citationName returns "<source title> (<fact type>)" from the read model, else
// the citation's id.
func (n *entityNames) citationName(citationID uuid.UUID) string {
	if citation := n.citations[citationID]; citation != nil {
		return fmt.Sprintf("%s (%s)", citation.SourceTitle, citation.FactType)
	}
	return citationID.String()
}
