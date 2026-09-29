package command

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

// MergeBlockerKind names which cross-entity rule a merge blocker breaks (#831).
type MergeBlockerKind string

const (
	// BlockerMissingPerson: a family partner or child link, or an
	// association, names a person main will not have.
	BlockerMissingPerson MergeBlockerKind = "missing_person"
	// BlockerMissingSource: a citation ends up citing a source main will not
	// have.
	BlockerMissingSource MergeBlockerKind = "missing_source"
	// BlockerSourceDeleteOrphansCitation: a source delete would cascade onto a
	// citation main still has.
	BlockerSourceDeleteOrphansCitation MergeBlockerKind = "source_delete_orphans_citation"
	// BlockerMissingMediaOwner: a media upload lands on an owner main will not
	// have.
	BlockerMissingMediaOwner MergeBlockerKind = "missing_media_owner"
	// BlockerOwnerDeleteOrphansMedia: an owner delete would cascade onto a
	// media item main wrote to after the branch's delete.
	BlockerOwnerDeleteOrphansMedia MergeBlockerKind = "owner_delete_orphans_media"
	// BlockerMissingGPSArtifact: the branch edits a GPS artifact main no
	// longer has.
	BlockerMissingGPSArtifact MergeBlockerKind = "missing_gps_artifact"
	// BlockerMissingGPSSubject: a GPS artifact is about a person or family main
	// will not have.
	BlockerMissingGPSSubject MergeBlockerKind = "missing_gps_subject"
	// BlockerSubjectDeleteOrphansGPS: a person or family delete would cascade
	// onto GPS research main added or changed after the fork.
	BlockerSubjectDeleteOrphansGPS MergeBlockerKind = "subject_delete_orphans_gps"
)

// MergeBlockerFix is the one-step change to the resolutions that clears a
// blocker (#831). It is a suggestion: leaving a stream out can surface another
// blocker (leaving out a person can strand a family that links them), which is
// why the review re-validates after every change. Including a stream is only
// suggested when it raises nothing new (see suggestMergeFixes), so following
// the suggestions never cycles.
type MergeBlockerFix string

const (
	// FixLeaveOut resolves the blocker's own stream (StreamID) to main.
	FixLeaveOut MergeBlockerFix = "leave_out"
	// FixIncludeReferenced resolves the referenced entity's stream
	// (ReferencedID) to branch, so the replay brings it along.
	FixIncludeReferenced MergeBlockerFix = "include_referenced"
)

// MergeBlocker is one cross-entity reference a merge (or resume) would break:
// the stream whose replay breaks it, and the entity it references (#831).
type MergeBlocker struct {
	// StreamID is the replayed stream at fault; EntityType and EntityName say
	// what it is (EntityType in the ChangeEntry vocabulary).
	StreamID   uuid.UUID
	EntityType string
	EntityName string

	// ReferencedID is the entity the stream references (or would cascade onto).
	ReferencedID   uuid.UUID
	ReferencedType string
	ReferencedName string

	Kind                MergeBlockerKind
	SuggestedResolution MergeBlockerFix

	// Message is the refusal in words, as the single-blocker error said it.
	Message string
}

// MergeBlockedError is the refusal a merge or resume returns when it has
// blockers. It wraps ErrMergeDanglingReference, and its message is the first
// blocker's, so a caller that only reads the error sees what it always saw.
type MergeBlockedError struct {
	Blockers []MergeBlocker
}

func (e *MergeBlockedError) Error() string {
	if len(e.Blockers) == 0 {
		return ErrMergeDanglingReference.Error()
	}
	msg := fmt.Sprintf("%s: %s", ErrMergeDanglingReference, e.Blockers[0].Message)
	if more := len(e.Blockers) - 1; more > 0 {
		msg += fmt.Sprintf(" (and %d more blocker(s))", more)
	}
	return msg
}

// Unwrap makes errors.Is(err, ErrMergeDanglingReference) hold.
func (e *MergeBlockedError) Unwrap() error { return ErrMergeDanglingReference }

// blockerList collects blockers, dropping exact repeats.
type blockerList struct {
	items []MergeBlocker
}

func (l *blockerList) add(b MergeBlocker) {
	for i := range l.items {
		have := l.items[i]
		if have.StreamID == b.StreamID && have.ReferencedID == b.ReferencedID && have.Kind == b.Kind {
			return
		}
	}
	if b.SuggestedResolution == "" {
		b.SuggestedResolution = FixLeaveOut
	}
	l.items = append(l.items, b)
}

func (l *blockerList) len() int { return len(l.items) }

// entityTypeOfStream maps a stream type onto the entity-type vocabulary.
func entityTypeOfStream(streamType string) string {
	return query.EntityTypeOfStream(streamType)
}

// streamBlocker starts a blocker about a replayed stream.
func streamBlocker(group streamGroup, kind MergeBlockerKind, referencedID uuid.UUID, referencedType string, format string, args ...any) MergeBlocker {
	return MergeBlocker{
		StreamID:       group.streamID,
		EntityType:     entityTypeOfStream(group.streamType),
		ReferencedID:   referencedID,
		ReferencedType: referencedType,
		Kind:           kind,
		Message:        fmt.Sprintf(format, args...),
	}
}

// refusal names the blockers and turns them into the refusal, or returns nil
// when there are none.
func (h *Handler) refusal(ctx context.Context, branchID uuid.UUID, list *blockerList) error {
	if list.len() == 0 {
		return nil
	}
	if err := h.nameBlockers(ctx, branchID, list.items); err != nil {
		return err
	}
	return &MergeBlockedError{Blockers: list.items}
}

// nameBlockers fills EntityName and ReferencedName on every blocker with one
// batched naming pass, as the branch sees the entities.
func (h *Handler) nameBlockers(ctx context.Context, branchID uuid.UUID, blockers []MergeBlocker) error {
	refs := make([]query.EntityRef, 0, 2*len(blockers))
	for i := range blockers {
		refs = append(refs,
			query.EntityRef{EntityType: blockers[i].EntityType, ID: blockers[i].StreamID},
			query.EntityRef{EntityType: blockers[i].ReferencedType, ID: blockers[i].ReferencedID})
	}
	names, err := h.branchService.NameEntities(ctx, domain.BranchID(branchID), refs)
	if err != nil {
		return fmt.Errorf("naming merge blockers: %w", err)
	}
	for i := range blockers {
		blockers[i].EntityName = names[query.EntityRef{EntityType: blockers[i].EntityType, ID: blockers[i].StreamID}]
		blockers[i].ReferencedName = names[query.EntityRef{EntityType: blockers[i].ReferencedType, ID: blockers[i].ReferencedID}]
	}
	return nil
}

// referenceKinds are the blocker kinds a replay of the referenced entity's
// stream can clear: the reference is dangling because the entity will not
// exist, so bringing it along fixes it.
var referenceKinds = map[MergeBlockerKind]bool{
	BlockerMissingPerson:     true,
	BlockerMissingSource:     true,
	BlockerMissingMediaOwner: true,
	BlockerMissingGPSSubject: true,
}

// suggestMergeFixes picks each merge blocker's one-step fix. Including the
// referenced entity is suggested when that is what excluded it and including
// it clears the blocker for good:
//
//   - its stream is in the replay set, resolved to main, would leave the
//     entity in existence if replayed, and accepts "branch" (a conflict whose
//     only supported resolution is main cannot be included); and
//   - re-running the checks with it included raises no blocker the current
//     resolutions do not already have, and clears this one.
//
// The second condition is what keeps the one-click fixes from cycling: a
// branch-created family left out because its partner is gone strands a
// research log about it, and "include the family" would bring the partner
// blocker straight back, so the log's fix is to leave it out too. Anything
// else is fixed by leaving the blocker's own stream out, which "main" always
// accepts. Each distinct referenced stream is re-checked once.
func (h *Handler) suggestMergeFixes(ctx context.Context, plan *query.MergePlan, groups []streamGroup, resolutions map[uuid.UUID]MergeResolution, blockers []MergeBlocker) error {
	candidates := includeCandidates(blockers, groups, resolutions, plan.Conflicts)
	current := make(map[blockerKey]bool, len(blockers))
	for i := range blockers {
		current[keyOf(&blockers[i])] = true
	}
	// after caches, per referenced stream, the blockers left once it is
	// included — or nil when including it raises a new one.
	after := make(map[uuid.UUID]map[blockerKey]bool)
	for i := range blockers {
		b := &blockers[i]
		b.SuggestedResolution = FixLeaveOut
		if !candidates[i] {
			continue
		}
		left, checked := after[b.ReferencedID]
		if !checked {
			var err error
			if left, err = h.blockersIncluding(ctx, plan, groups, resolutions, b.ReferencedID, current); err != nil {
				return err
			}
			after[b.ReferencedID] = left
		}
		if left != nil && !left[keyOf(b)] {
			b.SuggestedResolution = FixIncludeReferenced
		}
	}
	return nil
}

// includeCandidates reports, per blocker, whether its referenced entity was
// excluded by a "main" resolution that "branch" could undo: the blocker is a
// dangling reference, the referenced stream is in the replay set resolved to
// main, replaying it leaves the entity in existence, and its conflict (if
// any) accepts "branch".
func includeCandidates(blockers []MergeBlocker, groups []streamGroup, resolutions map[uuid.UUID]MergeResolution, conflicts []query.MergeConflict) []bool {
	byID := make(map[uuid.UUID]streamGroup, len(groups))
	for _, group := range groups {
		byID[group.streamID] = group
	}
	branchRefused := make(map[uuid.UUID]bool)
	for _, conflict := range conflicts {
		if !slices.Contains(conflict.SupportedResolutions, string(ResolveBranch)) {
			branchRefused[conflict.StreamID] = true
		}
	}
	out := make([]bool, len(blockers))
	for i := range blockers {
		b := &blockers[i]
		if !referenceKinds[b.Kind] || resolutions[b.ReferencedID] != ResolveMain || branchRefused[b.ReferencedID] {
			continue
		}
		group, ok := byID[b.ReferencedID]
		out[i] = ok && leavesEntityInExistence(group)
	}
	return out
}

// blockerKey identifies a blocker across two runs of the checks.
type blockerKey struct {
	stream, referenced uuid.UUID
	kind               MergeBlockerKind
}

func keyOf(b *MergeBlocker) blockerKey {
	return blockerKey{stream: b.StreamID, referenced: b.ReferencedID, kind: b.Kind}
}

// blockersIncluding re-runs the checks with streamID resolved to branch and
// returns the blockers that leaves, or nil when it raises one current does not
// have (including the stream would trade one blocker for another).
func (h *Handler) blockersIncluding(ctx context.Context, plan *query.MergePlan, groups []streamGroup, resolutions map[uuid.UUID]MergeResolution, streamID uuid.UUID, current map[blockerKey]bool) (map[blockerKey]bool, error) {
	trial := make(map[uuid.UUID]MergeResolution, len(resolutions))
	for id, r := range resolutions {
		trial[id] = r
	}
	trial[streamID] = ResolveBranch
	found, err := h.findMergeBlockers(ctx, plan, groups, trial)
	if err != nil {
		return nil, err
	}
	left := make(map[blockerKey]bool, len(found))
	for i := range found {
		key := keyOf(&found[i])
		if !current[key] {
			return nil, nil
		}
		left[key] = true
	}
	return left, nil
}

// leavesEntityInExistence reports whether replaying a stream creates its
// entity and leaves it standing.
func leavesEntityInExistence(group streamGroup) bool {
	if len(group.events) == 0 || endsInDelete(group.events) {
		return false
	}
	for i := range group.events {
		t := group.events[i].EventType
		if strings.HasSuffix(t, "Created") || t == "EvidenceConflictDetected" {
			return true
		}
	}
	return false
}

// firstMainWritesAfter returns, for each of streamIDs main wrote to after
// position, main's first such event — one per offending stream, found with a
// limit-one read per offender so the check never materializes a stream's
// history (a media item's carries its file bytes). Naming the blockers found
// reads media metadata from the read model, not the history (NameEntities).
func (h *Handler) firstMainWritesAfter(ctx context.Context, streamIDs []uuid.UUID, after int64) ([]repository.StoredEvent, error) {
	remaining := slices.Clone(streamIDs)
	var found []repository.StoredEvent
	for len(remaining) > 0 {
		later, err := h.eventStore.ReadStreamsForBranch(ctx, remaining, domain.MainBranchID, after, 1)
		if err != nil {
			return nil, err
		}
		if len(later) == 0 {
			break
		}
		found = append(found, later[0])
		before := len(remaining)
		remaining = slices.DeleteFunc(remaining, func(id uuid.UUID) bool { return id == later[0].StreamID })
		if len(remaining) == before {
			return nil, fmt.Errorf("reading main writes after position %d: got stream %s, which was not asked for", after, later[0].StreamID)
		}
	}
	return found, nil
}

// PrecheckMergeInput is a proposed merge to check for blockers: the
// resolutions (and so the exclusions) the review currently holds.
type PrecheckMergeInput struct {
	BranchID    uuid.UUID
	Resolutions map[uuid.UUID]MergeResolution
}

// PrecheckMerge reports the blockers MergeBranch would refuse the proposed
// resolutions with, without writing anything (#831). It plans the merge the
// same way, refuses what MergeBranch refuses before its reference check (an
// inactive, truncated or empty branch; an unknown or unsupported resolution),
// and runs the very same collectMergeBlockers, so a precheck with the same
// resolutions against the same main returns what the merge would.
//
// Undecided conflicts are not blockers: an undecided stream is checked as if
// replayed, exactly as MergeBranch checks it, and the conflict itself is the
// review's to decide.
func (h *Handler) PrecheckMerge(ctx context.Context, input PrecheckMergeInput) ([]MergeBlocker, error) {
	if h.branchStore == nil {
		return nil, ErrBranchStoreRequired
	}
	branch, err := h.branchStore.Get(ctx, input.BranchID)
	if err != nil {
		return nil, err // includes repository.ErrBranchNotFound
	}
	if branch.Status != domain.BranchStatusActive {
		return nil, fmt.Errorf("%w: %s", ErrBranchNotActive, branch.Status)
	}
	plan, err := h.branchService.PlanMerge(ctx, branch.ID)
	if err != nil {
		return nil, fmt.Errorf("planning merge: %w", err)
	}
	if err := refuseUnmergeablePlan(branch, plan); err != nil {
		return nil, err
	}
	groups, err := orderEvidenceForReplay(groupEventsByStream(plan.ReplayEvents))
	if err != nil {
		return nil, err
	}
	if err := validateResolutions(input.Resolutions, groups); err != nil {
		return nil, err
	}
	if err := validateConflictResolutions(plan.Conflicts, input.Resolutions); err != nil {
		return nil, err
	}
	return h.collectMergeBlockers(ctx, plan, groups, input.Resolutions)
}
