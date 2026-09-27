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
// read-model queries is therefore bounded by the number of entity types (four;
// twice that for a branch scope, see resolveEntityNamesOn), not by the number
// of entries.

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

// resolveEntityNames resolves names against main — the scope of the history
// endpoints and snapshot compare, which take no ?branch= parameter, so an
// entry is labelled with the mainline's current name (ADR-005). See
// resolveEntityNamesOn.
func (s *HistoryService) resolveEntityNames(ctx context.Context, refs entityRefs) (*entityNames, error) {
	return s.resolveEntityNamesOn(ctx, domain.MainBranchID, refs)
}

// resolveEntityNamesOn reads every registered entity with one batched lookup
// per entity type that has any ids, through branchID's overlay. A lookup
// failure is returned, not papered over: an entity that merely no longer exists
// is absent from the batch and still gets its fallback name, but a store that
// cannot answer is an error the caller must see.
//
// On main that is at most four queries, whatever the number of refs. On a
// branch, the overlay already serves main's row for every entity the branch
// has not touched, so an id it does not resolve is one the branch deleted (a
// tombstone) or one that exists nowhere. Those — and only those — are looked
// up once more on main, in one further batched lookup per type, so an entity
// the branch deletes is still labelled with the name main knows it by. A branch
// scope is therefore at most eight queries, still independent of the number of
// refs.
func (s *HistoryService) resolveEntityNamesOn(ctx context.Context, branchID domain.BranchID, refs entityRefs) (*entityNames, error) {
	names := &entityNames{}
	var err error

	if names.persons, err = lookupByIDs(ctx, branchID, refs.ids(entityTypePerson), s.readStore.GetPersonsByIDs,
		func(p *repository.PersonReadModel) uuid.UUID { return p.ID }); err != nil {
		return nil, fmt.Errorf("resolve person names: %w", err)
	}
	if names.families, err = lookupByIDs(ctx, branchID, refs.ids(entityTypeFamily), s.readStore.GetFamiliesByIDs,
		func(f *repository.FamilyReadModel) uuid.UUID { return f.ID }); err != nil {
		return nil, fmt.Errorf("resolve family names: %w", err)
	}
	if names.sources, err = lookupByIDs(ctx, branchID, refs.ids(entityTypeSource), s.readStore.GetSourcesByIDs,
		func(src *repository.SourceReadModel) uuid.UUID { return src.ID }); err != nil {
		return nil, fmt.Errorf("resolve source names: %w", err)
	}
	if names.citations, err = lookupByIDs(ctx, branchID, refs.ids(entityTypeCitation), s.readStore.GetCitationsByIDs,
		func(c *repository.CitationReadModel) uuid.UUID { return c.ID }); err != nil {
		return nil, fmt.Errorf("resolve citation names: %w", err)
	}
	return names, nil
}

// lookupByIDs reads ids with one batched call on branchID and, on a branch,
// one more on main for the ids the branch did not resolve (see
// resolveEntityNamesOn). No ids, no call.
func lookupByIDs[T any](
	ctx context.Context,
	branchID domain.BranchID,
	ids []uuid.UUID,
	get func(context.Context, domain.BranchID, []uuid.UUID) ([]T, error),
	id func(*T) uuid.UUID,
) (map[uuid.UUID]*T, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := get(ctx, branchID, ids)
	if err != nil {
		return nil, err
	}
	index := indexByID(rows, id)
	if branchID == domain.MainBranchID {
		return index, nil
	}
	var unresolved []uuid.UUID
	for _, want := range ids {
		if index[want] == nil {
			unresolved = append(unresolved, want)
		}
	}
	if len(unresolved) == 0 {
		return index, nil
	}
	mainRows, err := get(ctx, domain.MainBranchID, unresolved)
	if err != nil {
		return nil, err
	}
	for i := range mainRows {
		index[id(&mainRows[i])] = &mainRows[i]
	}
	return index, nil
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
