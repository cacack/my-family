package query

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Evidence coverage of a research branch (#838).
//
// "Proven findings merged back with review" (ETHOS): before a merge, the
// review points out every genealogical claim the branch changed that the
// branch has not documented. It is a soft, non-blocking warning — proof
// summaries are a later roadmap phase — so nothing here gates a merge.
//
// A CHANGED FACT is read from the branch's own events, classified by the
// event-type table (history_catalog.go):
//
//   - a person's key fields: name (given name, surname and the other name
//     parts, and the name variants), gender, birth and death;
//   - a life event or attribute: its fact type, on the person or family it
//     belongs to;
//   - a family's marriage date or place (family_marriage);
//   - a RELATIONSHIP: a family's partners or partnership type, or a child
//     linked to or unlinked from it;
//   - a DELETION of a person or family the mainline has: the branch concludes
//     the person never existed, or the partnership never did, and that claim
//     needs documenting as much as any other.
//
// Notes, research status and other bookkeeping fields are not claims and are
// not counted. A fact the branch created and deleted again, or a person or
// family the branch both created and deleted, is not counted either: there is
// nothing left to document. The individual fact changes on a mainline person
// or family the branch then deleted fold into that one deletion.
//
// DOCUMENTED means an evidence analysis or a proof summary written or revised
// ON THIS BRANCH — created or updated by its own events, and not deleted by
// them — for the same fact type and subject. One inherited from the mainline
// documents the conclusion the branch is changing, not the branch's, so it
// does not count. A relationship has no fact type of its own, so any analysis
// or proof summary the branch wrote about the family documents it; likewise a
// deletion, by any the branch wrote about the deleted person or family.

// Changed-fact kinds.
const (
	ChangedFactKindFact         = "fact"
	ChangedFactKindRelationship = "relationship"
	ChangedFactKindDeletion     = "deletion"
)

// ChangedFact is one fact or relationship the branch changed.
type ChangedFact struct {
	// Kind is ChangedFactKindFact, ChangedFactKindRelationship or
	// ChangedFactKindDeletion.
	Kind string `json:"kind"`
	// FactType is the fact changed; empty for a relationship or a deletion.
	FactType domain.FactType `json:"fact_type,omitempty"`
	// SubjectType ("person" or "family") and SubjectID name the entity the
	// fact is about: the subject an analysis of it would have.
	SubjectType string    `json:"subject_type"`
	SubjectID   uuid.UUID `json:"subject_id"`
	SubjectName string    `json:"subject_name"`
	// ChangeCount is how many of the branch's changes touched the fact.
	ChangeCount int `json:"change_count"`
}

// BranchEvidenceCoverage is the evidence-coverage warning of a branch.
type BranchEvidenceCoverage struct {
	// ChangedFactCount is how many facts and relationships the branch changed.
	ChangedFactCount int `json:"changed_fact_count"`
	// Uncovered are the changed facts the branch has no evidence analysis or
	// proof summary for, ordered by subject name, then fact.
	Uncovered []ChangedFact `json:"uncovered"`
	// HasMore reports that the branch's own events hit the read cap, so the
	// lists may be incomplete.
	HasMore bool `json:"has_more"`
}

// personFieldFacts maps a person's key fields (PersonCreated payload and
// PersonUpdated change keys) onto the fact they state.
var personFieldFacts = map[string]domain.FactType{
	"given_name":     domain.FactPersonName,
	"surname":        domain.FactPersonName,
	"name_prefix":    domain.FactPersonName,
	"name_suffix":    domain.FactPersonName,
	"surname_prefix": domain.FactPersonName,
	"nickname":       domain.FactPersonName,
	"gender":         domain.FactPersonGender,
	"birth_date":     domain.FactPersonBirth,
	"birth_place":    domain.FactPersonBirth,
	"death_date":     domain.FactPersonDeath,
	"death_place":    domain.FactPersonDeath,
}

// familyMarriageFields state the family_marriage fact; familyRelationshipFields
// state who the partnership is between.
var (
	familyMarriageFields     = map[string]bool{"marriage_date": true, "marriage_place": true}
	familyRelationshipFields = map[string]bool{"partner1_id": true, "partner2_id": true, "relationship_type": true}
)

// changedFactKey identifies a changed fact: a relationship and a deletion
// have no fact type, so the kind tells them apart.
type changedFactKey struct {
	kind     string
	factType domain.FactType
	subject  uuid.UUID
}

// coverageScan accumulates a branch's changed and documented facts.
type coverageScan struct {
	states map[uuid.UUID]*streamState
	// createdOnBranch and deletedOnBranch say, per stream, whether the
	// branch's first event on it created it and its last one deleted it.
	createdOnBranch map[uuid.UUID]bool
	deletedOnBranch map[uuid.UUID]bool

	facts []*ChangedFact
	index map[changedFactKey]*ChangedFact
	// inEvent holds the facts the event being classified has touched, so an
	// event that sets several fields of one fact counts once.
	inEvent map[changedFactKey]bool

	documented         map[changedFactKey]bool
	documentedSubjects map[uuid.UUID]bool
}

// EvidenceCoverage reports which facts and relationships the branch changed
// without an evidence analysis or proof summary on the branch. See the
// comment at the top of this file for what counts. Returns
// repository.ErrBranchNotFound (wrapped) for an unknown branch.
//
// Cost: one read of the branch's own events, one set-based read per side of
// the streams they touch, and the batched naming of the subjects — never a
// query per change.
func (s *BranchService) EvidenceCoverage(ctx context.Context, branchID uuid.UUID) (*BranchEvidenceCoverage, error) {
	diff, err := s.loadBranchSide(ctx, branchID)
	if err != nil {
		return nil, err
	}
	scope := domain.BranchID(diff.branch.ID)

	scan := &coverageScan{
		createdOnBranch:    make(map[uuid.UUID]bool),
		deletedOnBranch:    make(map[uuid.UUID]bool),
		index:              make(map[changedFactKey]*ChangedFact),
		documented:         make(map[changedFactKey]bool),
		documentedSubjects: make(map[uuid.UUID]bool),
	}
	streamIDs := scan.markLifecycle(diff.branchEvents)

	// The folded state of every touched stream, as the branch sees it, gives
	// the fact type and owner of a life event or attribute (their update and
	// delete events carry neither) and the subject of an analysis.
	desc := &historyDescription{states: make(map[uuid.UUID]*streamState)}
	if err := s.historyService.foldStreams(ctx, scope, desc, streamIDs); err != nil {
		return nil, fmt.Errorf("fold changed streams: %w", err)
	}
	scan.states = desc.states

	for i := range diff.branchEvents {
		scan.add(&diff.branchEvents[i])
	}

	result := &BranchEvidenceCoverage{Uncovered: []ChangedFact{}, HasMore: diff.branchTruncated}
	var uncovered []*ChangedFact
	for _, fact := range scan.facts {
		if scan.deletedOnBranch[fact.SubjectID] && fact.Kind != ChangedFactKindDeletion {
			// Gone on the branch: nothing left to document if the branch
			// created it too, and otherwise its deletion stands for it.
			continue
		}
		result.ChangedFactCount++
		if !scan.isDocumented(fact) {
			uncovered = append(uncovered, fact)
		}
	}
	if err := s.nameSubjects(ctx, scope, uncovered); err != nil {
		return nil, err
	}
	sort.SliceStable(uncovered, func(i, j int) bool {
		a, b := uncovered[i], uncovered[j]
		if a.SubjectName != b.SubjectName {
			return a.SubjectName < b.SubjectName
		}
		if a.SubjectID != b.SubjectID {
			return a.SubjectID.String() < b.SubjectID.String()
		}
		return a.FactType < b.FactType
	})
	for _, fact := range uncovered {
		result.Uncovered = append(result.Uncovered, *fact)
	}
	return result, nil
}

// nameSubjects names each fact's subject as the branch sees it, and a deleted
// subject as the mainline still has it, with one batched lookup per side.
func (s *BranchService) nameSubjects(ctx context.Context, scope domain.BranchID, facts []*ChangedFact) error {
	var onBranch, deleted []*ChangedFact
	for _, fact := range facts {
		if fact.Kind == ChangedFactKindDeletion {
			deleted = append(deleted, fact)
		} else {
			onBranch = append(onBranch, fact)
		}
	}
	for _, side := range []struct {
		scope domain.BranchID
		facts []*ChangedFact
	}{{scope, onBranch}, {domain.MainBranchID, deleted}} {
		if len(side.facts) == 0 {
			continue
		}
		refs := make([]EntityRef, 0, len(side.facts))
		for _, fact := range side.facts {
			refs = append(refs, EntityRef{EntityType: fact.SubjectType, ID: fact.SubjectID})
		}
		names, err := s.NameEntities(ctx, side.scope, refs)
		if err != nil {
			return fmt.Errorf("name changed-fact subjects: %w", err)
		}
		for _, fact := range side.facts {
			fact.SubjectName = names[EntityRef{EntityType: fact.SubjectType, ID: fact.SubjectID}]
		}
	}
	return nil
}

// markLifecycle records, per stream, whether the branch created it and
// whether it deleted it last, and returns the streams in first-touch order.
func (c *coverageScan) markLifecycle(events []repository.StoredEvent) []uuid.UUID {
	var streamIDs []uuid.UUID
	seen := make(map[uuid.UUID]bool)
	for i := range events {
		evt := &events[i]
		class, ok := classifyHistoryEvent(evt.EventType)
		if !seen[evt.StreamID] {
			seen[evt.StreamID] = true
			streamIDs = append(streamIDs, evt.StreamID)
			c.createdOnBranch[evt.StreamID] = ok && class.Action == actionCreated
		}
		c.deletedOnBranch[evt.StreamID] = ok && class.Action == actionDeleted
	}
	return streamIDs
}

// add classifies one of the branch's events: a changed fact, a documenting
// analysis or proof summary, or neither.
func (c *coverageScan) add(evt *repository.StoredEvent) {
	class, ok := classifyHistoryEvent(evt.EventType)
	if !ok || class.Excluded() {
		return
	}
	c.inEvent = make(map[changedFactKey]bool)
	switch class.EntityType {
	case entityTypePerson:
		c.addPersonEvent(evt)
	case entityTypeFamily:
		c.addFamilyEvent(evt)
	case entityTypeLifeEvent, entityTypeAttribute:
		c.addFactRecord(evt, class.EntityType)
	case entityTypeEvidenceAnalysis, entityTypeProofSummary:
		c.addDocumentation(evt.StreamID)
	}
}

func (c *coverageScan) addPersonEvent(evt *repository.StoredEvent) {
	switch evt.EventType {
	case "PersonCreated":
		for field, value := range eventFields(evt) {
			if factType, ok := personFieldFacts[field]; ok && statesClaim(field, value) {
				c.touch(ChangedFactKindFact, factType, entityTypePerson, evt.StreamID)
			}
		}
	case "PersonUpdated":
		for field := range updateChanges(evt) {
			if factType, ok := personFieldFacts[field]; ok {
				c.touch(ChangedFactKindFact, factType, entityTypePerson, evt.StreamID)
			}
		}
	case "NameAdded", "NameUpdated", "NameRemoved":
		c.touch(ChangedFactKindFact, domain.FactPersonName, entityTypePerson, evt.StreamID)
	case "PersonDeleted":
		c.touchDeletion(entityTypePerson, evt.StreamID)
	}
}

func (c *coverageScan) addFamilyEvent(evt *repository.StoredEvent) {
	var fields map[string]any
	switch evt.EventType {
	case "FamilyCreated":
		fields = eventFields(evt)
		for field, value := range fields {
			if !statesClaim(field, value) {
				delete(fields, field)
			}
		}
	case "FamilyUpdated":
		fields = updateChanges(evt)
	case "ChildLinkedToFamily", "ChildUnlinkedFromFamily":
		c.touch(ChangedFactKindRelationship, "", entityTypeFamily, evt.StreamID)
		return
	case "FamilyDeleted":
		c.touchDeletion(entityTypeFamily, evt.StreamID)
		return
	default:
		return
	}
	for field := range fields {
		switch {
		case familyMarriageFields[field]:
			c.touch(ChangedFactKindFact, domain.FactFamilyMarriage, entityTypeFamily, evt.StreamID)
		case field == "relationship_type" && evt.EventType == "FamilyCreated":
			// A new family's partnership type alone names no one; its
			// partners, when it has any, are the relationship.
		case familyRelationshipFields[field]:
			c.touch(ChangedFactKindRelationship, "", entityTypeFamily, evt.StreamID)
		}
	}
}

// addFactRecord counts a life event's or attribute's change against its fact
// type and owner, unless the branch both created and deleted it.
func (c *coverageScan) addFactRecord(evt *repository.StoredEvent, entityType string) {
	if c.createdOnBranch[evt.StreamID] && c.deletedOnBranch[evt.StreamID] {
		return
	}
	state := c.states[evt.StreamID]
	if evt.EventType == "LifeEventCreated" || evt.EventType == "AttributeCreated" {
		// The creation carries both; the fold may have been capped.
		state = &streamState{fields: eventFields(evt)}
	}
	factType := domain.FactType(state.str("fact_type"))
	if factType == "" {
		return
	}
	if id, ok := state.id("person_id"); ok {
		c.touch(ChangedFactKindFact, factType, entityTypePerson, id)
		return
	}
	if id, ok := state.id("family_id"); ok && entityType == entityTypeLifeEvent {
		c.touch(ChangedFactKindFact, factType, entityTypeFamily, id)
	}
}

// addDocumentation records an analysis or proof summary the branch wrote and
// did not delete, as the branch sees it now.
func (c *coverageScan) addDocumentation(streamID uuid.UUID) {
	if c.deletedOnBranch[streamID] {
		return
	}
	state := c.states[streamID]
	subject, ok := state.id("subject_id")
	if !ok {
		return
	}
	c.documentedSubjects[subject] = true
	if factType := state.str("fact_type"); factType != "" {
		c.documented[changedFactKey{kind: ChangedFactKindFact, factType: domain.FactType(factType), subject: subject}] = true
	}
}

// touchDeletion counts the deletion of a person or family the mainline has
// and the branch leaves deleted; one the branch created is not a claim about
// the mainline's tree, and one it restored again is not deleted.
func (c *coverageScan) touchDeletion(subjectType string, subject uuid.UUID) {
	if c.createdOnBranch[subject] || !c.deletedOnBranch[subject] {
		return
	}
	c.touch(ChangedFactKindDeletion, "", subjectType, subject)
}

// touch counts one change against a fact, registering it on first sight.
func (c *coverageScan) touch(kind string, factType domain.FactType, subjectType string, subject uuid.UUID) {
	key := changedFactKey{kind: kind, factType: factType, subject: subject}
	if c.inEvent[key] {
		return
	}
	c.inEvent[key] = true
	fact := c.index[key]
	if fact == nil {
		fact = &ChangedFact{Kind: kind, FactType: factType, SubjectType: subjectType, SubjectID: subject}
		c.index[key] = fact
		c.facts = append(c.facts, fact)
	}
	fact.ChangeCount++
}

func (c *coverageScan) isDocumented(fact *ChangedFact) bool {
	if fact.Kind == ChangedFactKindRelationship || fact.Kind == ChangedFactKindDeletion {
		return c.documentedSubjects[fact.SubjectID]
	}
	return c.documented[changedFactKey{kind: fact.Kind, factType: fact.FactType, subject: fact.SubjectID}]
}

// statesClaim reports whether a creation event's field value states anything:
// an empty value, or an unknown gender, claims nothing.
func statesClaim(field string, value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case string:
		if field == "gender" && v == string(domain.GenderUnknown) {
			return false
		}
		return v != ""
	default:
		return true
	}
}

// updateChanges returns an Updated event's change map (empty when it has
// none).
func updateChanges(evt *repository.StoredEvent) map[string]any {
	if changes, ok := eventFields(evt)["changes"].(map[string]any); ok {
		return changes
	}
	return map[string]any{}
}
