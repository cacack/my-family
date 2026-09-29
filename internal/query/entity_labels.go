package query

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

// EntityRef names one entity whose display name a caller needs: its
// ChangeEntry entity type (EntityTypeOfStream maps a stream type onto it) and
// its id.
type EntityRef struct {
	EntityType string
	ID         uuid.UUID
}

// EntityTypeOfStream maps an event-store stream type ("Person", "person",
// "EvidenceAnalysis", ...) onto the ChangeEntry entity-type vocabulary
// ("person", "evidence_analysis"), which is also what the review UI labels
// entities by. Stream types are written both CamelCase (commands) and lower
// case (GEDCOM import), so the mapping is purely lexical.
func EntityTypeOfStream(streamType string) string {
	var b strings.Builder
	for i, r := range streamType {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

// NameEntities returns the display name of every ref as branchID sees it (an
// entity the branch deleted keeps the name main knows it by, and one deleted
// everywhere is named from its folded stream), using the same naming as change
// entries and merge conflicts. An entity nothing names maps to "" rather than
// its id: the caller already carries the id.
//
// The read model is asked first, and only the entities it cannot name are
// folded from their streams, as describeConflictValues does. Media items are
// named from their read-model title or file name (GetMedia never reads the
// bytes), so an item's history — whose MediaCreated carries the file and
// thumbnail bytes — is read only for an item gone from the read model on both
// the branch and main.
//
// The cost is one batched read-model lookup per named type (twice: once for
// the refs, once for the people and sources a folded entity's name refers
// to), one metadata read per media ref, one set-based stream read per side for
// the entities still unnamed, and one further read for related people only the
// log still names (#697).
func (s *BranchService) NameEntities(ctx context.Context, branchID domain.BranchID, refs []EntityRef) (map[EntityRef]string, error) {
	names := make(map[EntityRef]string, len(refs))
	if len(refs) == 0 {
		return names, nil
	}
	hs := s.historyService

	lookup := newEntityRefs()
	for _, ref := range refs {
		if ref.ID != uuid.Nil {
			lookup.add(ref.EntityType, ref.ID)
		}
	}
	desc := &historyDescription{states: make(map[uuid.UUID]*streamState)}
	var err error
	if desc.names, err = hs.resolveEntityNamesOn(ctx, branchID, lookup); err != nil {
		return nil, err
	}
	mediaNames, err := hs.mediaNames(ctx, branchID, refs)
	if err != nil {
		return nil, err
	}

	if err := hs.foldStreams(ctx, branchID, desc, unnamedRefIDs(refs, desc.names, mediaNames)); err != nil {
		return nil, err
	}
	if err := hs.nameRelated(ctx, branchID, desc, refs, lookup); err != nil {
		return nil, err
	}

	for _, ref := range refs {
		if name, ok := mediaNames[ref.ID]; ok && ref.EntityType == entityTypeMedia {
			names[ref] = name
			continue
		}
		name := desc.name(ref.EntityType, ref.ID, nil)
		if name == ref.ID.String() {
			name = ""
		}
		names[ref] = name
	}
	return names, nil
}

// unnamedRefIDs returns, sorted and once each, the ids of the refs neither
// the read model nor mediaNames named: the ones to fold from their streams.
func unnamedRefIDs(refs []EntityRef, names *entityNames, mediaNames map[uuid.UUID]string) []uuid.UUID {
	seen := make(map[uuid.UUID]bool, len(refs))
	var unnamed []uuid.UUID
	for _, ref := range refs {
		if ref.ID == uuid.Nil || seen[ref.ID] || names.has(ref.EntityType, ref.ID) {
			continue
		}
		if _, ok := mediaNames[ref.ID]; ok && ref.EntityType == entityTypeMedia {
			continue
		}
		seen[ref.ID] = true
		unnamed = append(unnamed, ref.ID)
	}
	sort.Slice(unnamed, func(i, j int) bool { return unnamed[i].String() < unnamed[j].String() })
	return unnamed
}

// nameRelated names the entities a folded ref's name refers to (a family's
// partners, a citation's source, an association's people): one more batched
// read-model lookup, then a fold of the people only the log still names.
func (s *HistoryService) nameRelated(ctx context.Context, branchID domain.BranchID, desc *historyDescription, refs []EntityRef, lookup entityRefs) error {
	related := newEntityRefs()
	for _, ref := range refs {
		desc.registerRelated(related, ref.EntityType, ref.ID)
	}
	if len(related) > 0 {
		more, err := s.resolveEntityNamesOn(ctx, branchID, related)
		if err != nil {
			return err
		}
		desc.names.absorb(more)
	}
	for entityType, ids := range related {
		for id := range ids {
			lookup.add(entityType, id)
		}
	}
	return s.foldRelatedPeople(ctx, branchID, desc, lookup)
}

// mediaNames names each media ref from the read model, as branchID sees it
// and else as main does (an item the branch deleted keeps main's name). An
// item neither holds, or one with neither a title nor a file name, is left
// out, for the caller to fold.
func (s *HistoryService) mediaNames(ctx context.Context, branchID domain.BranchID, refs []EntityRef) (map[uuid.UUID]string, error) {
	names := make(map[uuid.UUID]string)
	tried := make(map[uuid.UUID]bool)
	for _, ref := range refs {
		if ref.EntityType != entityTypeMedia || ref.ID == uuid.Nil || tried[ref.ID] {
			continue
		}
		tried[ref.ID] = true
		scopes := []domain.BranchID{branchID}
		if !branchID.IsMain() {
			scopes = append(scopes, domain.MainBranchID)
		}
		for _, scope := range scopes {
			media, err := s.readStore.GetMedia(ctx, scope, ref.ID)
			if err != nil {
				return nil, fmt.Errorf("resolve media name: %w", err)
			}
			if media == nil {
				continue
			}
			label := strings.TrimSpace(media.Title)
			if label == "" {
				label = strings.TrimSpace(media.Filename)
			}
			if label != "" {
				names[ref.ID] = label
			}
			break
		}
	}
	return names, nil
}

// has reports whether the read model named the entity.
func (n *entityNames) has(entityType string, id uuid.UUID) bool {
	switch entityType {
	case entityTypePerson:
		return n.persons[id] != nil
	case entityTypeFamily:
		return n.families[id] != nil
	case entityTypeSource:
		return n.sources[id] != nil
	case entityTypeCitation:
		return n.citations[id] != nil
	}
	return false
}

// absorb adds other's rows to n.
func (n *entityNames) absorb(other *entityNames) {
	n.persons = absorbRows(n.persons, other.persons)
	n.families = absorbRows(n.families, other.families)
	n.sources = absorbRows(n.sources, other.sources)
	n.citations = absorbRows(n.citations, other.citations)
}

func absorbRows[T any](into, from map[uuid.UUID]*T) map[uuid.UUID]*T {
	if len(from) == 0 {
		return into
	}
	if into == nil {
		into = make(map[uuid.UUID]*T, len(from))
	}
	for id, row := range from {
		if into[id] == nil {
			into[id] = row
		}
	}
	return into
}
