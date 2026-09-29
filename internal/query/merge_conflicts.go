// Package query provides CQRS query services for the genealogy application.
package query

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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

	// DeletedBy names the side that deleted the entity ("branch" or "main").
	// Set for delete_edit only.
	DeletedBy string `json:"deleted_by,omitempty"`

	// FieldValues says, per contested field, what the entity held at the fork
	// and what each side now asserts, in words (#828). For edit_edit it covers
	// Fields; for delete_edit it covers the fields the editing side changed,
	// with the deleting side's value absent. Empty for create_create.
	FieldValues []MergeConflictField `json:"field_values,omitempty"`

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

	// MainStreamVersions is main's version for every stream in ReplayEvents, as
	// observed while this plan was being built, and for every person a
	// replayed PersonMerged merges away (#834) — those streams are not
	// replayed, but the verdict covers main's changes to them. Streams main has
	// never seen are present with 0.
	//
	// It belongs on the plan rather than being re-read at replay time because it
	// is part of what the conflict verdict was computed against: Conflicts says
	// "given main at THESE versions, here is the disagreement". The two travel
	// together so the command can assert, before it writes anything, that main
	// still sits where the verdict assumed — a mainline write landing after this
	// snapshot was never compared against the branch's events, so the verdict no
	// longer describes reality (#698).
	//
	// EVERY stream in ReplayEvents must be present. A missing entry is NOT
	// fail-safe and must not be treated as one: a stream main has never seen is
	// pinned at 0, so an absent entry reads as 0 too and the command's
	// `current == planned` comparison passes silently. The command therefore
	// requires the key rather than defaulting it (see validatePlanNotStale).
	// PlanMerge always populates it from the same event slice ReplayEvents comes
	// from; the requirement is for any future constructor. (#685's resume does
	// not build a MergePlan: it replays from the plan recorded on the claim.)
	//
	// DELIBERATELY NOT PINNED: the create-vs-create class. A colliding create on
	// main lives on a DIFFERENT stream by definition (see readMainCreateTail), so
	// it is not in ReplayEvents and has no version here to pin. That class is
	// inert in v0.12 — its gate never opens without branch-scoped GEDCOM import,
	// an explicit non-goal of epic #54 — so the gap is recorded, not built for.
	MainStreamVersions map[uuid.UUID]int64

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

	// diff is the loaded diff the verdict was computed from, kept so
	// DescribeConflictValues can value the conflicts on demand (#828) without
	// re-reading either side. The merge itself never needs the values, so
	// PlanMerge does not compute them.
	diff *branchDiffSources
}

// DescribeConflictValues fills FieldValues on the plan's conflicts: what each
// side says, per contested field, in words (#828). The merge command calls it
// only when it refuses over undecided conflicts and hands them back to the
// reviewer; a merge that goes ahead never pays for the display-only reads. A
// plan not built by PlanMerge (or without conflicts) is left as it is.
func (s *BranchService) DescribeConflictValues(ctx context.Context, plan *MergePlan) error {
	if plan == nil || plan.diff == nil || len(plan.Conflicts) == 0 {
		return nil
	}
	if err := s.describeConflictValues(ctx, plan.diff, plan.Conflicts); err != nil {
		return fmt.Errorf("describe conflicting values: %w", err)
	}
	return nil
}

// PlanMerge builds the merge plan for a branch: its replayable events and the
// conflicts that a merge of them would run into.
//
// It reads the same two sides as CompareBranch, through the same helpers, so the
// review UI and the merge command see the same events and reach the same
// conflict verdict. It does NOT call loadBranchDiff, because the ORDER of the
// three reads below is the whole guarantee of the #698 pin:
//
//  1. the branch side, which is what tells us WHICH streams to pin;
//  2. the pin itself;
//  3. main's side — the only main state classifyConflicts ever compares against.
//
// The pin must sit between 1 and 3, not after 3. Capturing it after the main
// read would make the pinned versions strictly NEWER than the verdict: a write
// landing in that window would be baked into the pin while never appearing in
// the compared tail, so the merge command's `current == planned` guard would
// pass and the branch would replay over an event nothing ever compared it
// against — the exact silent override #698 is about, in the one direction the
// guard cannot see. Pinning first makes that same write show up as
// `current > planned`, which is a refusal, and leaves the compared main tail a
// superset of the pinned state.
//
// detectConflicts is deliberately last and reads nothing new for the edit and
// delete classes — it is a pure function over the diff — so its position carries
// no ordering requirement of its own. Its one read (readMainCreateTail, for the
// create-vs-create class) is unpinned; see MainStreamVersions.
func (s *BranchService) PlanMerge(ctx context.Context, branchID uuid.UUID) (*MergePlan, error) {
	diff, err := s.loadBranchSide(ctx, branchID)
	if err != nil {
		return nil, err
	}

	mainVersions, err := s.captureMainStreamVersions(ctx, diff.branchEvents)
	if err != nil {
		return nil, err
	}

	if err := s.loadMainSide(ctx, diff); err != nil {
		return nil, err
	}

	conflicts, tailTruncated, err := s.detectConflicts(ctx, diff)
	if err != nil {
		return nil, err
	}

	return &MergePlan{
		Branch:             diff.branch,
		ReplayEvents:       diff.branchEvents,
		Conflicts:          conflicts,
		MainStreamVersions: mainVersions,
		BranchTruncated:    diff.branchTruncated,
		MainTruncated:      diff.mainTruncated || tailTruncated,
		EventCap:           maxComparisonEvents,
		diff:               diff,
	}, nil
}

// MergeReplaySet is a branch's replay set on its own, without a conflict
// verdict: what a merge of the branch re-appends onto main.
type MergeReplaySet struct {
	Branch *domain.Branch

	// ReplayEvents are the branch's own mutation events in ascending position
	// order, lifecycle events stripped — the same slice MergePlan.ReplayEvents
	// carries, read through the same helper.
	ReplayEvents []repository.StoredEvent

	// Truncated reports that the branch's scan hit EventCap, so ReplayEvents is
	// incomplete.
	Truncated bool

	// EventCap is the scan cap Truncated was measured against.
	EventCap int
}

// LoadMergeReplaySet reads a branch's replay set and nothing else. It is what a
// resumed merge (#685) needs: the conflict verdict and the staleness pins were
// fixed when the merge was claimed and are recorded on its BranchMerged event,
// so re-running PlanMerge would only recompute a verdict against a main the
// replay has already partly written to — a verdict that no longer means
// anything. A claimed branch accepts no further writes, so this set is the same
// one the original merge replayed from.
func (s *BranchService) LoadMergeReplaySet(ctx context.Context, branchID uuid.UUID) (*MergeReplaySet, error) {
	diff, err := s.loadBranchSide(ctx, branchID)
	if err != nil {
		return nil, err
	}
	return &MergeReplaySet{
		Branch:       diff.branch,
		ReplayEvents: diff.branchEvents,
		Truncated:    diff.branchTruncated,
		EventCap:     maxComparisonEvents,
	}, nil
}

// captureMainStreamVersions records main's current version for each stream the
// branch touched — the versions MergePlan.MainStreamVersions carries.
//
// This is one single-row read per stream, for every stream the branch touched.
// A stream that is then REPLAYED onto main costs two more — the command's
// pre-claim check, and the one replayStream already made to compute its
// expected version — so it is three reads, not one. A stream resolved to "main"
// costs only this capture: both the pre-claim check and replayStream skip it,
// because a stream whose branch events are never replayed cannot be overridden
// by a mainline write. The whole set is bounded by the streams the BRANCH
// touched rather than by main's size.
//
// The passes that do happen are inherent: the pre-claim check exists precisely
// to observe a version FRESHER than this one, so it cannot reuse this read.
// Collapsing each pass into one set-based read needs a batched stream-version
// read on the EventStore. #697 batched only the read-model name lookups
// (enrichConflictEntities, transformStoredEvents), so that remains separate work.
//
// Only PlanMerge calls this. CompareBranch shares the two diff reads but not
// this capture — the versions are merge-plan internals with no meaning in a
// read-only diff, and ADR-005 keeps them off the public comparison response, so
// compare must not pay N extra reads for them.
func (s *BranchService) captureMainStreamVersions(ctx context.Context, branchEvents []repository.StoredEvent) (map[uuid.UUID]int64, error) {
	// comparedStreamIDs, not branchStreamIDs: the persons the branch merged
	// away are pinned too. Their streams are not replayed, but main writing to
	// one after this verdict was computed is exactly the unreviewed change the
	// pin exists to catch (see the command's validatePlanNotStale).
	streamIDs := comparedStreamIDs(branchEvents)
	versions := make(map[uuid.UUID]int64, len(streamIDs))
	for _, streamID := range streamIDs {
		version, err := s.eventStore.GetStreamVersion(ctx, streamID, domain.MainBranchID)
		if err != nil {
			return nil, fmt.Errorf("read main version for stream %s: %w", streamID, err)
		}
		// 0 for a stream main has never seen is kept as 0, not translated to
		// the -1 "new stream" sentinel. That translation is an Append concern
		// and stays in replayStream, where the append happens; here 0 is simply
		// the true version and compares as one.
		versions[streamID] = version
	}
	return versions, nil
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
	if err := s.enrichConflictEntities(ctx, domain.BranchID(diff.branch.ID), diff.branchEvents, conflicts); err != nil {
		return nil, false, fmt.Errorf("name conflicting entities: %w", err)
	}
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
		mergedEdited := mergedPersonsMainChanged(branchSides[streamID], mainSides)
		if conflict, ok := classifyStream(streamID, branchSides[streamID], mainSides[streamID]); ok {
			conflicts = append(conflicts, withMergedPersonEdits(conflict, mergedEdited))
			continue
		}
		if conflict, ok := classifyCreateCollision(streamID, branchXrefs[streamID], mainXrefOwners); ok {
			conflicts = append(conflicts, withMergedPersonEdits(conflict, mergedEdited))
			continue
		}
		if len(mergedEdited) > 0 {
			conflicts = append(conflicts, mergedPersonConflict(streamID, mergedEdited))
		}
	}
	return conflicts
}

// mergedPersonsMainChanged returns the persons a branch stream merged away
// (PersonMerged.MergedID, #834) that main changed after the fork, in the order
// the branch merged them.
//
// A PersonMerged lands on the SURVIVOR's stream, but it also ends the merged
// person: the projection re-links their data to the survivor and deletes
// them, with no event on their own stream. Per-stream comparison therefore
// cannot see main editing the merged person — main's events are on a stream
// the branch never wrote. PlanMerge reads main's events on the merged
// persons' streams too (mergedPersonIDs), so they are in mainSides here. A
// merged person main deleted is not a disagreement: both sides removed them.
func mergedPersonsMainChanged(branchSide *streamSide, mainSides map[uuid.UUID]*streamSide) []uuid.UUID {
	if branchSide == nil {
		return nil
	}
	var changed []uuid.UUID
	for _, mergedID := range branchSide.merged {
		if mainSide := mainSides[mergedID]; mainSide != nil && !mainSide.deleted {
			changed = append(changed, mergedID)
		}
	}
	return changed
}

// mergedPersonConflict reports that the branch merged persons into this one
// whom main changed after the fork. It is a delete_edit with the branch as
// the deleter, keyed on the SURVIVOR's stream, because that is the stream
// whose resolution decides the outcome: "branch" replays the merge (main's
// changes to the merged person's names, facts and evidence move to the
// survivor with the rest; its changed fields go with the deleted record), and
// "main" skips the survivor's stream, so main keeps both persons as it has
// them.
func mergedPersonConflict(streamID uuid.UUID, mergedEdited []uuid.UUID) MergeConflict {
	return MergeConflict{
		StreamID: streamID,
		Kind:     ConflictDeleteEdit,
		Detail: fmt.Sprintf(
			"The branch merged %s into this person while main changed %s; "+
				"\"branch\" completes the merge over main's changes, \"main\" keeps both persons as main has them",
			joinIDs(mergedEdited), pluralPerson(len(mergedEdited))),
		SupportedResolutions: []string{resolveBranchValue, resolveMainValue},
	}
}

// withMergedPersonEdits adds the merged-person disagreement to a conflict
// already reported on the survivor's stream, so the one resolution the caller
// makes for that stream is made knowing both.
func withMergedPersonEdits(conflict MergeConflict, mergedEdited []uuid.UUID) MergeConflict {
	if len(mergedEdited) == 0 {
		return conflict
	}
	conflict.Detail += fmt.Sprintf(". The branch also merged %s into this person, which main changed after the fork",
		joinIDs(mergedEdited))
	return conflict
}

// joinIDs renders ids for a conflict detail.
func joinIDs(ids []uuid.UUID) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = "person " + id.String()
	}
	return strings.Join(parts, ", ")
}

// pluralPerson names one or several merged persons in a conflict detail.
func pluralPerson(n int) string {
	if n == 1 {
		return "that person"
	}
	return "those persons"
}

// mergedPersonIDs returns the persons the events merge away
// (PersonMerged.MergedID), in the order they were merged, without
// duplicates. PlanMerge and CompareBranch read main's events on these streams
// alongside the streams the branch wrote, since a branch merge ends each of
// them without writing to their stream (see mergedPersonsMainChanged).
func mergedPersonIDs(events []repository.StoredEvent) []uuid.UUID {
	seen := make(map[uuid.UUID]bool)
	var ids []uuid.UUID
	for _, evt := range events {
		if evt.EventType != personMergedEventType {
			continue
		}
		mergedID, ok := mergedPersonOf(evt)
		if !ok || seen[mergedID] {
			continue
		}
		seen[mergedID] = true
		ids = append(ids, mergedID)
	}
	return ids
}

// personMergedEventType is the event a person merge writes on the survivor's
// stream.
const personMergedEventType = "PersonMerged"

// mergedPersonOf decodes the person a PersonMerged merges away.
func mergedPersonOf(evt repository.StoredEvent) (uuid.UUID, bool) {
	var payload struct {
		MergedID uuid.UUID `json:"merged_id"`
	}
	if err := json.Unmarshal(evt.Data, &payload); err != nil || payload.MergedID == uuid.Nil {
		warnMalformedPersonMerge(evt, err)
		return uuid.Nil, false
	}
	return payload.MergedID, true
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
		deletedBy := resolveBranchValue
		// When the BRANCH is the deleter, replaying its delete onto main works
		// normally, so both sides remain choosable. When MAIN is the deleter,
		// replaying the branch's edits onto a row that no longer exists is a
		// no-op — see SupportedResolutions.
		supported := []string{resolveBranchValue, resolveMainValue}
		detail := "%s deleted this entity while %s changed it"
		if mainSide.deleted {
			deleter, editor = "Main", "the branch"
			deletedBy = resolveMainValue
			supported = []string{resolveMainValue}
			detail += "; the branch's changes cannot be replayed onto a deleted entity, so only \"main\" is available"
		}
		return MergeConflict{
			StreamID:             streamID,
			Kind:                 ConflictDeleteEdit,
			Detail:               fmt.Sprintf(detail, deleter, editor),
			DeletedBy:            deletedBy,
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

	// merged lists the persons this side merged into the stream's person
	// (PersonMerged.MergedID), in order. Only the branch side's is read: see
	// mergedPersonsMainChanged.
	merged []uuid.UUID
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
		case foldEvidenceConflictResolved:
			applyEvidenceConflictResolved(side, evt)
		case foldPersonMerged:
			applyPersonMerged(side, evt)
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
	foldEvidenceConflictResolved
	foldPersonMerged
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
	case eventType == "EvidenceConflictResolved":
		return foldEvidenceConflictResolved
	case eventType == personMergedEventType:
		return foldPersonMerged
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
	return strings.HasSuffix(eventType, "Deleted") && !researchMetadataEventTypes[eventType]
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

// applyEvidenceConflictResolved records an evidence-conflict resolution (#760) as
// the two fields it asserts. The event carries them flat, with no Changes map,
// so the *Updated fold would read nothing from it: a branch and main resolving
// the same evidence conflict differently would then merge with no review. (The
// evidence conflict is a genealogical finding on its own stream; this only
// compares its events like any other aggregate's.)
func applyEvidenceConflictResolved(side *streamSide, evt repository.StoredEvent) {
	var payload struct {
		Resolution string `json:"resolution"`
		Status     string `json:"status"`
	}
	if err := json.Unmarshal(evt.Data, &payload); err != nil {
		return
	}
	side.fields["resolution"] = payload.Resolution
	side.fields["status"] = payload.Status
}

// applyPersonMerged records a person merge (#834) on the survivor's side: the
// fields it resolved onto the survivor, compared like an *Updated event's
// Changes (the keys are the same person field names), and the person it
// merged away. The event carries its fields as ResolvedFields, not Changes, so
// the *Updated fold would read nothing from it — and main merging someone into
// the same survivor with a different surname would merge with no review.
func applyPersonMerged(side *streamSide, evt repository.StoredEvent) {
	var payload struct {
		MergedID       uuid.UUID      `json:"merged_id"`
		ResolvedFields map[string]any `json:"resolved_fields"`
	}
	if err := json.Unmarshal(evt.Data, &payload); err != nil {
		warnMalformedPersonMerge(evt, err)
		return
	}
	for field, value := range payload.ResolvedFields {
		side.fields[field] = value
	}
	if payload.MergedID != uuid.Nil {
		side.merged = append(side.merged, payload.MergedID)
	}
}

// warnMalformedPersonMerge logs a PersonMerged the conflict fold cannot read.
// The fold stays a pure function that drops what it cannot decode, like every
// other fold here; the merge and resume commands decode the same payload
// strictly (personMergesIn) and refuse to replay it, so a corrupt merge is
// reported there rather than merged past. The warning makes the compare-side
// drop visible too.
func warnMalformedPersonMerge(evt repository.StoredEvent, err error) {
	slog.Warn("merge conflicts: cannot decode person merge; its merged person is left out of the comparison",
		"stream_id", evt.StreamID, "position", evt.Position, "error", err)
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
	if !strings.HasSuffix(evt.EventType, "Created") || researchMetadataEventTypes[evt.EventType] {
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

// conflictEntityType is the ChangeEntry entity type of the entity evt belongs
// to — the one vocabulary a compare response speaks (historyEventCatalog). An
// event type the catalog does not map (unreachable for any decodable type)
// falls back to its lower-cased stream type, which is at least the entity's
// real name.
func conflictEntityType(evt *repository.StoredEvent) string {
	if class, ok := classifyHistoryEvent(evt.EventType); ok && !class.Excluded() {
		return class.EntityType
	}
	return strings.ToLower(evt.StreamType)
}

// enrichConflictEntities labels each conflict with the type and display name of
// the entity it is about, so a reviewer sees "Ada Lovelace" and not a UUID.
//
// Names resolve in one batched read-model lookup per entity type, however many
// conflicts there are (#697), through the branch's overlay (branchID): a
// conflict is about the branch's change, so an entity is named as the branch
// sees it, and one the branch deletes by the name main still has for it
// (resolveEntityNamesOn). An entity with no resolvable name degrades to an
// empty string rather than an error, the same posture transformStoredEvents
// takes: a missing name makes a conflict less readable, never wrong, and the
// caller still has StreamID. A read-model failure, by contrast, is returned.
func (s *BranchService) enrichConflictEntities(ctx context.Context, branchID domain.BranchID, branchEvents []repository.StoredEvent, conflicts []MergeConflict) error {
	if len(conflicts) == 0 {
		return nil
	}

	firstTouch := make(map[uuid.UUID]*repository.StoredEvent, len(conflicts))
	for i := range branchEvents {
		if _, seen := firstTouch[branchEvents[i].StreamID]; !seen {
			firstTouch[branchEvents[i].StreamID] = &branchEvents[i]
		}
	}

	// Pass 1: type every conflict from the event that first touched it.
	touches := make([]repository.StoredEvent, 0, len(conflicts))
	for i := range conflicts {
		evt := firstTouch[conflicts[i].StreamID]
		if evt == nil {
			continue
		}
		conflicts[i].EntityType = conflictEntityType(evt)
		touches = append(touches, *evt)
	}

	// One batched description for every conflicted entity: the same naming
	// change entries use, so a conflict and the entries it contests read alike.
	desc, err := s.historyService.describeEvents(ctx, branchID, touches)
	if err != nil {
		return err
	}

	// Pass 2: name every typed conflict from the batch.
	for i := range conflicts {
		evt := firstTouch[conflicts[i].StreamID]
		if evt == nil {
			continue
		}
		name := desc.name(conflicts[i].EntityType, conflicts[i].StreamID, evt)
		if name == conflicts[i].StreamID.String() {
			// name falls back to the id when nothing resolves. The conflict
			// already carries the id in StreamID, so report the absence as
			// absence.
			name = ""
		}
		conflicts[i].EntityName = name
	}
	return nil
}
