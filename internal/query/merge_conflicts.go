// Package query provides CQRS query services for the genealogy application.
package query

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// MergeConflictKind names the three conflict classes ADR-005 §Conflict
// definition requires merge review to detect. There is no fourth kind:
// structural relationship divergence (link vs. unlink) is an edit-vs-edit
// conflict keyed on the relationship rather than on a field name.
type MergeConflictKind string

const (
	// ConflictEditEdit — both sides changed the same field (or the same
	// relationship) to different final values.
	ConflictEditEdit MergeConflictKind = "edit_edit"

	// ConflictDeleteEdit — exactly one side deleted the aggregate while the
	// other changed it. A merge must never silently resurrect a main-deleted
	// entity nor silently discard a main edit to a branch-deleted one.
	ConflictDeleteEdit MergeConflictKind = "delete_edit"

	// ConflictCreateCreate — both sides independently created an aggregate that
	// resolves to the same identity (the same GEDCOM xref).
	ConflictCreateCreate MergeConflictKind = "create_create"
)

// The resolution values a conflict can accept. They are plain strings rather
// than the command layer's MergeResolution type because internal/command
// imports internal/query, not the other way round; the command converts.
const (
	resolveBranchValue = "branch"
	resolveMainValue   = "main"
)

// MergeConflict is one aggregate on which the branch and main made incompatible
// changes after the branch's base position.
type MergeConflict struct {
	StreamID   uuid.UUID         `json:"stream_id"`
	EntityType string            `json:"entity_type"`
	EntityName string            `json:"entity_name"`
	Kind       MergeConflictKind `json:"kind"`
	Fields     []string          `json:"fields,omitempty"` // edit_edit only
	Detail     string            `json:"detail"`

	// SupportedResolutions lists the resolutions that would actually produce
	// the outcome they name, sorted. Most conflicts accept both sides, but two
	// shapes do not, and offering a resolution that silently does nothing is
	// worse than refusing it:
	//
	//   - main deleted the entity while the branch edited it. Replaying the
	//     branch's edits cannot resurrect it — the *Updated projections skip an
	//     absent row (internal/repository/projection.go) — so the merge would
	//     report success while main stayed deleted. Only "main" is offered
	//     until an undelete exists.
	//   - create_create. The two sides are different streams by construction,
	//     so "branch" promotes the branch's entity and leaves main's beside it,
	//     producing the duplicate the conflict class exists to prevent. Only
	//     "main" is offered.
	//
	// The command rejects a resolution outside this list, and a review UI can
	// use it to offer only the choices that mean something.
	SupportedResolutions []string `json:"supported_resolutions"`
}

// MergePlan is everything the merge command needs to decide and then execute:
// what would be replayed onto main, and what stands in the way.
//
// It is a plan, not a verdict — PlanMerge reports conflicts without judging
// them. Whether a non-empty Conflicts blocks the merge is the command layer's
// policy, not the query layer's.
type MergePlan struct {
	Branch *domain.Branch

	// ReplayEvents are the branch's own mutation events in ascending position
	// order, with the branch-lifecycle events already stripped — exactly the set
	// ADR-005 §Merge says a merge re-appends onto main.
	ReplayEvents []repository.StoredEvent

	Conflicts []MergeConflict

	// BranchTruncated reports that the BRANCH's own scan hit EventCap, so its
	// replay set is incomplete. This is a property of the branch itself and
	// does not improve on its own.
	BranchTruncated bool

	// MainTruncated reports that a scan of MAIN hit EventCap, so the conflict
	// list is not known to be complete even though the branch may be tiny. It
	// grows with mainline activity since the fork, not with branch size, and is
	// therefore a different problem with a different remedy.
	MainTruncated bool

	// EventCap is the per-scan event cap Truncated was measured against. It is
	// reported so a refusal can tell the caller the actual limit instead of
	// leaving them to guess at "too large".
	EventCap int
}

// PlanMerge builds the merge plan for a branch: its replayable events and the
// conflicts that a merge of them would run into.
//
// It shares loadBranchDiff with CompareBranch, so the review UI and the merge
// command see the same events and reach the same conflict verdict.
func (s *BranchService) PlanMerge(ctx context.Context, branchID uuid.UUID) (*MergePlan, error) {
	diff, err := s.loadBranchDiff(ctx, branchID)
	if err != nil {
		return nil, err
	}

	conflicts, tailTruncated, err := s.detectConflicts(ctx, diff)
	if err != nil {
		return nil, err
	}

	return &MergePlan{
		Branch:          diff.branch,
		ReplayEvents:    diff.branchEvents,
		Conflicts:       conflicts,
		BranchTruncated: diff.branchTruncated,
		MainTruncated:   diff.mainTruncated || tailTruncated,
		EventCap:        maxComparisonEvents,
	}, nil
}

// detectConflicts runs the conflict scan over an already-loaded diff and
// enriches each conflict with its entity's display name. The second return
// value reports whether the gated create-vs-create tail read hit the cap.
func (s *BranchService) detectConflicts(ctx context.Context, diff *branchDiffSources) ([]MergeConflict, bool, error) {
	mainTail, tailTruncated, err := s.readMainCreateTail(ctx, diff)
	if err != nil {
		return nil, false, err
	}

	conflicts := classifyConflicts(diff.branchEvents, diff.mainEvents, mainTail)
	s.enrichConflictEntities(ctx, diff.branchEvents, conflicts)

	return conflicts, tailTruncated, nil
}

// readMainCreateTail reads main's OWN events after the base position, for the
// create-vs-create class only.
//
// GATED DELIBERATELY. Unlike the other two classes, create-vs-create cannot be
// scoped to the branch's streams — a colliding create on main lives on a
// different stream by definition, so the only way to find it is to look at
// main's tail. ADR-005's Implementation Notes call a full-tail scan out as the
// anti-pattern to avoid (it grows with all main activity and is re-paid on
// every compare/merge), so the read is issued only when it could possibly
// match: when the branch created at least one entity carrying a GEDCOM xref.
//
// In v0.12 that gate never opens. Branch-scoped GEDCOM import is a stated
// non-goal of epic #54, and an xref is only ever assigned by import, so branch
// creates carry no xref and this read is not issued today. The code exists so
// that the day branch-scoped import lands, the class is already detected.
func (s *BranchService) readMainCreateTail(ctx context.Context, diff *branchDiffSources) ([]repository.StoredEvent, bool, error) {
	if len(createdXrefs(diff.branchEvents)) == 0 {
		return nil, false, nil
	}

	events, err := s.eventStore.ReadBranch(ctx, domain.MainBranchID, diff.branch.BasePosition, maxComparisonEvents)
	if err != nil {
		return nil, false, fmt.Errorf("read main tail for create collisions: %w", err)
	}

	// Same conservative partial-read rule the two diff sides use.
	return events, len(events) >= maxComparisonEvents, nil
}

// classifyConflicts is the entire conflict rule, expressed as a pure function
// over event slices so it can be tested without a store.
//
// branchEvents and mainEvents are the two sides of the diff — the branch's own
// mutation events, and main's events on the streams the branch touched, both
// after the base position. mainTail is main's own events after the base
// position and is nil unless the create-vs-create gate opened
// (see readMainCreateTail); nil simply means that class finds nothing.
//
// Conflicts come back in the branch's first-touch stream order, at most one per
// stream, so the response and the tests are stable.
func classifyConflicts(branchEvents, mainEvents, mainTail []repository.StoredEvent) []MergeConflict {
	branchSides := summarizeStreams(branchEvents)
	mainSides := summarizeStreams(mainEvents)
	branchXrefs := createdXrefs(branchEvents)
	mainXrefOwners := xrefOwners(mainTail)

	var conflicts []MergeConflict
	for _, streamID := range branchStreamIDs(branchEvents) {
		if conflict, ok := classifyStream(streamID, branchSides[streamID], mainSides[streamID]); ok {
			conflicts = append(conflicts, conflict)
			continue
		}
		if conflict, ok := classifyCreateCollision(streamID, branchXrefs[streamID], mainXrefOwners); ok {
			conflicts = append(conflicts, conflict)
		}
	}
	return conflicts
}

// classifyStream applies the delete-vs-edit and edit-vs-edit rules to one
// aggregate. branchSide is never nil (the stream came from the branch's own
// events); mainSide is nil when main never touched the stream.
func classifyStream(streamID uuid.UUID, branchSide, mainSide *streamSide) (MergeConflict, bool) {
	// Only one side moved: nothing to disagree with, so it merges cleanly.
	if mainSide == nil {
		return MergeConflict{}, false
	}

	if branchSide.deleted != mainSide.deleted {
		deleter, editor := "The branch", "main"
		// When the BRANCH is the deleter, replaying its delete onto main works
		// normally, so both sides remain choosable. When MAIN is the deleter,
		// replaying the branch's edits onto a row that no longer exists is a
		// no-op — see SupportedResolutions.
		supported := []string{resolveBranchValue, resolveMainValue}
		detail := "%s deleted this entity while %s changed it"
		if mainSide.deleted {
			deleter, editor = "Main", "the branch"
			supported = []string{resolveMainValue}
			detail += "; the branch's changes cannot be replayed onto a deleted entity, so only \"main\" is available"
		}
		return MergeConflict{
			StreamID:             streamID,
			Kind:                 ConflictDeleteEdit,
			Detail:               fmt.Sprintf(detail, deleter, editor),
			SupportedResolutions: supported,
		}, true
	}

	// Both sides deleted it. They agree the entity is gone, so any field-level
	// divergence before the delete is moot.
	if branchSide.deleted {
		return MergeConflict{}, false
	}

	fields := divergentFields(branchSide.fields, mainSide.fields)
	if len(fields) == 0 {
		return MergeConflict{}, false
	}
	return MergeConflict{
		StreamID:             streamID,
		Kind:                 ConflictEditEdit,
		Fields:               fields,
		Detail:               fmt.Sprintf("The branch and main set %s to different values", strings.Join(fields, ", ")),
		SupportedResolutions: []string{resolveBranchValue, resolveMainValue},
	}, true
}

// classifyCreateCollision reports a branch-created aggregate whose GEDCOM xref
// is already claimed by an aggregate main created after the base position.
func classifyCreateCollision(streamID uuid.UUID, branchXref string, mainXrefOwners map[string]uuid.UUID) (MergeConflict, bool) {
	if branchXref == "" {
		return MergeConflict{}, false
	}
	mainStreamID, claimed := mainXrefOwners[branchXref]
	if !claimed || mainStreamID == streamID {
		return MergeConflict{}, false
	}
	return MergeConflict{
		StreamID: streamID,
		Kind:     ConflictCreateCreate,
		Detail: fmt.Sprintf(
			"The branch and main each created an entity with GEDCOM xref %s (main created %s); promoting the branch's would leave two entities sharing one xref, so only \"main\" is available",
			branchXref, mainStreamID),
		// Deliberately main-only: the two sides are different streams, so
		// "branch" would add a duplicate rather than resolve anything. See
		// SupportedResolutions.
		SupportedResolutions: []string{resolveMainValue},
	}, true
}

// streamSide is one side's net effect on a single aggregate after the base
// position: whether it deleted the aggregate, and the FINAL value it asserted
// for each field it touched.
//
// Final value, not every value: ADR-005 asks whether the two sides ended up
// incompatible, so a field the branch set to "Lovelace" and then back to
// "Byron" agrees with a main that never moved off "Byron".
type streamSide struct {
	deleted bool
	fields  map[string]any
}

// summarizeStreams folds one side's events, which must be in ascending position
// order, into a per-aggregate net effect.
func summarizeStreams(events []repository.StoredEvent) map[uuid.UUID]*streamSide {
	sides := make(map[uuid.UUID]*streamSide)
	for _, evt := range events {
		side, ok := sides[evt.StreamID]
		if !ok {
			side = &streamSide{fields: make(map[string]any)}
			sides[evt.StreamID] = side
		}

		switch conflictFoldFor(evt.EventType) {
		case foldAggregateDelete:
			side.deleted = true
		case foldChildLinked:
			applyChildRelation(side, evt, relationLinked)
		case foldChildUnlinked:
			applyChildRelation(side, evt, relationUnlinked)
		case foldPersonName:
			applyNameEvent(side, evt)
		case foldChangesMap:
			for field, value := range updatedChanges(evt) {
				side.fields[field] = value
			}
		case foldIgnored:
			// Nothing to compare — see conflictFoldFor.
		}
	}
	return sides
}

// conflictFold names how summarizeStreams folds one event into a side's net
// effect. It exists so the mapping lives in exactly one place: ConflictFold
// exposes it to the drift test that checks every branch-writable event type is
// consciously classified, and a switch on this type makes a newly added fold
// a compile-time obligation rather than a silently-missing case.
type conflictFold int

const (
	// foldIgnored asserts nothing comparable about an aggregate's state.
	foldIgnored conflictFold = iota
	foldAggregateDelete
	foldChildLinked
	foldChildUnlinked
	foldPersonName
	foldChangesMap
)

// conflictFoldFor classifies an event type for conflict comparison.
//
// Order matters: the person-name events are checked BEFORE the "*Updated"
// suffix rule, because NameUpdated matches that suffix but carries no Changes
// map (see applyNameEvent) — folding it as foldChangesMap would read nothing
// and make two divergent renames look like agreement.
func conflictFoldFor(eventType string) conflictFold {
	switch {
	case isAggregateDelete(eventType):
		return foldAggregateDelete
	case eventType == "ChildLinkedToFamily":
		return foldChildLinked
	case eventType == "ChildUnlinkedFromFamily":
		return foldChildUnlinked
	case isPersonNameEvent(eventType):
		return foldPersonName
	case strings.HasSuffix(eventType, "Updated"):
		return foldChangesMap
	default:
		return foldIgnored
	}
}

// ConflictComparable reports whether summarizeStreams extracts anything
// comparable from an event type — i.e. whether divergence expressed through
// this event can surface as a conflict.
//
// Exported for the drift test in internal/command, which asserts that every
// event type a branch is allowed to write (command.BranchAwareEventTypes) is
// either comparable here or is on that test's explicit
// ignored-with-a-reason list. internal/query cannot import internal/command
// (the dependency runs the other way), so the check lives there and the
// predicate lives here, next to the switch it describes.
func ConflictComparable(eventType string) bool {
	return conflictFoldFor(eventType) != foldIgnored
}

// isPersonNameEvent reports whether an event mutates one of a person's names.
func isPersonNameEvent(eventType string) bool {
	switch eventType {
	case "NameAdded", "NameUpdated", "NameRemoved":
		return true
	default:
		return false
	}
}

// isAggregateDelete reports whether an event removes the aggregate itself.
//
// Matched by suffix rather than by an enumerated list on purpose: every entity
// type's delete is named "<Entity>Deleted", and a list would silently stop
// detecting delete-vs-edit for each new entity type someone forgot to add —
// the same failure mode ES-007 and PR-004 guard against elsewhere. The branch
// lifecycle events are excluded because "this branch was discarded" is not a
// delete of any genealogy aggregate; callers strip them first, and the check
// here keeps the rule true of the function on its own.
func isAggregateDelete(eventType string) bool {
	return strings.HasSuffix(eventType, "Deleted") && !branchLifecycleEventTypes[eventType]
}

// The two directions a child-of-family relationship can be asserted in. They
// are the compared VALUES; childFieldKey names the field they are asserted on.
const (
	relationLinked   = "linked"
	relationUnlinked = "unlinked"
)

// childFieldKey names the child-of-family relationship as a field, e.g.
// "children[3f2a…]", so link-vs-unlink reports through MergeConflict.Fields
// exactly as a diverging *Updated field does.
func childFieldKey(personID uuid.UUID) string {
	return "children[" + personID.String() + "]"
}

// applyChildRelation records a link/unlink as a field keyed on the child, so a
// structural change compares under the same rule as an *Updated field.
func applyChildRelation(side *streamSide, evt repository.StoredEvent, direction string) {
	var payload struct {
		PersonID uuid.UUID `json:"person_id"`
	}
	if err := json.Unmarshal(evt.Data, &payload); err != nil {
		return
	}
	side.fields[childFieldKey(payload.PersonID)] = direction
}

// nameFieldKey names one of a person's names as a field, e.g. "names[3f2a…]",
// so add/update/remove of that name reports through MergeConflict.Fields
// exactly as a diverging *Updated field does.
func nameFieldKey(nameID uuid.UUID) string {
	return "names[" + nameID.String() + "]"
}

// nameRemovedValue is the value a side asserts for a name it removed, so a
// removal on one side and an edit on the other compare as different values.
// It is a string where a surviving name is a map, so the two can never be
// DeepEqual by accident.
const nameRemovedValue = "removed"

// applyNameEvent records a person-name mutation as a field keyed on the name.
//
// Name events need their own fold for two reasons. They are NOT *Updated-shaped
// — NameAdded/NameUpdated carry the name's fields flat (internal/domain/events.go)
// with no Changes map, so updatedChanges would read nothing and two divergent
// renames would look like agreement. And they live on the PERSON stream, so a
// rename is a change to the person aggregate that has to be compared per-name
// rather than per-person, or two edits to different names of the same person
// would collide.
//
// The compared value is the event's whole payload minus the envelope, so a new
// name field added to the domain is compared without touching this function.
func applyNameEvent(side *streamSide, evt repository.StoredEvent) {
	var envelope struct {
		NameID uuid.UUID `json:"name_id"`
	}
	if err := json.Unmarshal(evt.Data, &envelope); err != nil {
		return
	}

	if evt.EventType == "NameRemoved" {
		side.fields[nameFieldKey(envelope.NameID)] = nameRemovedValue
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(evt.Data, &payload); err != nil {
		return
	}
	// Drop the envelope: the event id and timestamp differ on every event even
	// when the asserted name is identical, and the person/name ids are the key
	// rather than part of the value.
	for _, key := range []string{"id", "timestamp", "person_id", "name_id"} {
		delete(payload, key)
	}
	side.fields[nameFieldKey(envelope.NameID)] = payload
}

// updatedChanges pulls the Changes map out of a *Updated event that carries one.
//
// Decoded as the one field that matters rather than through DecodeEvent's type
// switch, so it stays complete as new entity types are added where a type
// switch would quietly stop detecting conflicts on the ones it had not been
// taught. Note the limit this trades for: an *Updated event that does NOT
// carry a Changes map reads as "changed nothing". NameUpdated is exactly that
// shape, which is why conflictFoldFor routes it to applyNameEvent first; any
// future flat *Updated event needs the same treatment, and the drift test in
// internal/command is what catches it.
func updatedChanges(evt repository.StoredEvent) map[string]any {
	var payload struct {
		Changes map[string]any `json:"changes"`
	}
	if err := json.Unmarshal(evt.Data, &payload); err != nil {
		return nil
	}
	return payload.Changes
}

// divergentFields returns the fields both sides touched and left at different
// values, sorted for a stable response. A field only one side touched, or one
// both sides happened to land on the same value, is not a conflict.
func divergentFields(branchFields, mainFields map[string]any) []string {
	var fields []string
	for field, branchValue := range branchFields {
		mainValue, both := mainFields[field]
		if !both || reflect.DeepEqual(branchValue, mainValue) {
			continue
		}
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

// createdXrefs maps each stream a *Created event opened to the non-empty GEDCOM
// xref it carries. Streams created without an xref are absent — an xref is the
// only identity two independent creates can be said to share.
func createdXrefs(events []repository.StoredEvent) map[uuid.UUID]string {
	xrefs := make(map[uuid.UUID]string)
	for _, evt := range events {
		xref := createdGedcomXref(evt)
		if xref == "" {
			continue
		}
		if _, seen := xrefs[evt.StreamID]; !seen {
			xrefs[evt.StreamID] = xref
		}
	}
	return xrefs
}

// xrefOwners is createdXrefs inverted: which stream first claimed each xref.
func xrefOwners(events []repository.StoredEvent) map[string]uuid.UUID {
	owners := make(map[string]uuid.UUID)
	for _, evt := range events {
		xref := createdGedcomXref(evt)
		if xref == "" {
			continue
		}
		if _, claimed := owners[xref]; !claimed {
			owners[xref] = evt.StreamID
		}
	}
	return owners
}

// createdGedcomXref returns the GEDCOM xref a *Created event carries, or "".
func createdGedcomXref(evt repository.StoredEvent) string {
	if !strings.HasSuffix(evt.EventType, "Created") || branchLifecycleEventTypes[evt.EventType] {
		return ""
	}
	var payload struct {
		GedcomXref string `json:"gedcom_xref"`
	}
	if err := json.Unmarshal(evt.Data, &payload); err != nil {
		return ""
	}
	return payload.GedcomXref
}

// enrichConflictEntities labels each conflict with the type and display name of
// the entity it is about, so a reviewer sees "Ada Lovelace" and not a UUID.
//
// Name resolution degrades to an empty string rather than an error, the same
// posture transformStoredEvents takes: a missing name makes a conflict less
// readable, never wrong, and the caller still has StreamID.
func (s *BranchService) enrichConflictEntities(ctx context.Context, branchEvents []repository.StoredEvent, conflicts []MergeConflict) {
	if len(conflicts) == 0 {
		return
	}

	firstTouch := make(map[uuid.UUID]*repository.StoredEvent, len(conflicts))
	for i := range branchEvents {
		if _, seen := firstTouch[branchEvents[i].StreamID]; !seen {
			firstTouch[branchEvents[i].StreamID] = &branchEvents[i]
		}
	}

	for i := range conflicts {
		evt := firstTouch[conflicts[i].StreamID]
		if evt == nil {
			continue
		}
		// Lower-cased, because that is the vocabulary the rest of the API speaks:
		// ChangeEntry.EntityType is "person"/"family", and getEntityName switches
		// on those same lower-case names — handed "Person" it silently falls
		// through to its default and every conflict comes back unnamed. Derived
		// from StreamType rather than mapEventTypeToEntityAndAction because that
		// mapper answers "unknown" for every entity outside the four it knows,
		// whereas a stream type is always the entity's real name.
		entityType := strings.ToLower(evt.StreamType)
		conflicts[i].EntityType = entityType
		name := s.historyService.getEntityName(ctx, entityType, conflicts[i].StreamID, evt)
		if name == conflicts[i].StreamID.String() {
			// getEntityName falls back to the id when nothing resolves. The
			// conflict already carries the id in StreamID, so report the
			// absence as absence.
			name = ""
		}
		conflicts[i].EntityName = name
	}
}
