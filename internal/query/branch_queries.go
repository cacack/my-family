// Package query provides CQRS query services for the genealogy application.
package query

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// branchLifecycleEventTypes are the events that describe a branch itself rather
// than the genealogy data on it. ADR-005 §Merge excludes them from replay, and a
// diff excludes them for the same reason: "this branch was created" is not a
// change to anyone's family tree.
var branchLifecycleEventTypes = map[string]bool{
	"BranchCreated": true,
	"BranchDeleted": true,
	"BranchMerged":  true,
}

// BranchService provides query operations for research branches.
type BranchService struct {
	branchStore    repository.BranchStore
	eventStore     repository.EventStore
	historyService *HistoryService
}

// NewBranchService creates a new branch query service.
func NewBranchService(branchStore repository.BranchStore, eventStore repository.EventStore, historyService *HistoryService) *BranchService {
	return &BranchService{
		branchStore:    branchStore,
		eventStore:     eventStore,
		historyService: historyService,
	}
}

// ListBranches returns all branches ordered by created_at DESC.
func (s *BranchService) ListBranches(ctx context.Context) ([]*domain.Branch, error) {
	return s.branchStore.List(ctx)
}

// GetBranch retrieves a single branch by ID, returning
// repository.ErrBranchNotFound when it does not exist.
func (s *BranchService) GetBranch(ctx context.Context, id uuid.UUID) (*domain.Branch, error) {
	return s.branchStore.Get(ctx, id)
}

// BranchComparisonResult is a structured diff of a branch against main: what the
// branch changed, and what main changed underneath it since the branch forked.
type BranchComparisonResult struct {
	Branch       *domain.Branch `json:"branch"`
	BasePosition int64          `json:"base_position"`

	// BranchChanges are the branch's own changes since BasePosition, with the
	// branch-lifecycle events excluded.
	BranchChanges []ChangeEntry `json:"branch_changes"`

	// MainChanges are main's changes after BasePosition, restricted to the
	// streams the branch itself touched. Main activity on any other stream is
	// deliberately not reported — it cannot diverge from this branch.
	MainChanges []ChangeEntry `json:"main_changes"`

	BranchChangeCount int `json:"branch_change_count"`
	MainChangeCount   int `json:"main_change_count"`

	// HasMore reports that at least one side hit the read cap and the comparison
	// is therefore partial.
	HasMore bool `json:"has_more"`

	// OverlappingStreamIDs are the streams that both the branch and main changed.
	// This is a HINT — entities worth a human's attention because both lines of
	// research moved them — and explicitly NOT a conflict verdict: two sides can
	// move the same entity and still agree. Conflicts is the verdict; a stream
	// can appear here and be perfectly mergeable.
	OverlappingStreamIDs []uuid.UUID `json:"overlapping_stream_ids"`

	// Conflicts are the aggregates on which the two sides made incompatible
	// changes, classified per ADR-005 §Conflict definition. Empty means the
	// branch merges cleanly.
	Conflicts []MergeConflict `json:"conflicts"`
}

// CompareBranch returns a structured diff of a branch against main.
//
// The branch side is the branch's own events after its base position; the main
// side is main's events after that same position, restricted to the streams the
// branch touched (ADR-005 Implementation Notes: scoping the scan to the branch's
// own aggregates keeps compare independent of unrelated main activity).
//
// Merged and archived branches are still comparable. The event log is
// append-only (ES-002), so a terminal branch retains everything it changed and
// this call reports it as a historical diff. For a merged branch the main side
// will normally include main's replayed copies of the branch's own changes,
// which is exactly what the merge did.
func (s *BranchService) CompareBranch(ctx context.Context, branchID uuid.UUID) (*BranchComparisonResult, error) {
	diff, err := s.loadBranchDiff(ctx, branchID)
	if err != nil {
		return nil, err
	}

	branchChanges, err := s.historyService.transformStoredEvents(ctx, diff.branchEvents)
	if err != nil {
		return nil, fmt.Errorf("transform branch events: %w", err)
	}

	mainChanges, err := s.historyService.transformStoredEvents(ctx, diff.mainEvents)
	if err != nil {
		return nil, fmt.Errorf("transform main events: %w", err)
	}

	conflicts, tailTruncated, err := s.detectConflicts(ctx, diff)
	if err != nil {
		return nil, err
	}

	return &BranchComparisonResult{
		Branch:               diff.branch,
		BasePosition:         diff.branch.BasePosition,
		BranchChanges:        branchChanges,
		MainChanges:          mainChanges,
		BranchChangeCount:    len(branchChanges),
		MainChangeCount:      len(mainChanges),
		HasMore:              diff.branchTruncated || diff.mainTruncated || tailTruncated,
		OverlappingStreamIDs: overlappingStreamIDs(diff.branchEvents, diff.mainEvents),
		Conflicts:            conflicts,
	}, nil
}

// branchDiffSources is the raw material of both branch-vs-main answers: the
// review diff (CompareBranch) and the merge plan (PlanMerge). Both need the
// same two event sets read the same way, so they read them once, here, rather
// than each issuing its own pair of store calls that could drift apart.
type branchDiffSources struct {
	branch *domain.Branch

	// branchEvents are the branch's own mutation events after its base
	// position, in ascending position order, with the branch-lifecycle events
	// stripped. This is also exactly the merge's replay set (ADR-005 §Merge).
	branchEvents []repository.StoredEvent

	// mainEvents are main's events after that same position, restricted to the
	// streams branchEvents touched.
	mainEvents []repository.StoredEvent

	// branchTruncated reports that the BRANCH's own scan hit
	// maxComparisonEvents. mainTruncated reports the same for main's tail.
	// They are kept apart because they mean different things to a caller: a
	// branch too big to scan is a permanent property of that branch, while a
	// main tail too long to scan grows with unrelated mainline activity and
	// says nothing about the branch's size.
	branchTruncated bool
	mainTruncated   bool
}

// loadBranchDiff loads a branch and both sides of its divergence from main.
func (s *BranchService) loadBranchDiff(ctx context.Context, branchID uuid.UUID) (*branchDiffSources, error) {
	branch, err := s.branchStore.Get(ctx, branchID)
	if err != nil {
		return nil, fmt.Errorf("get branch: %w", err)
	}

	// Branch side: the branch's own events. fromPosition is exclusive, and every
	// branch event is appended after the fork, so the base position is a no-op
	// filter here — it is passed for symmetry with the main side.
	rawBranchEvents, err := s.eventStore.ReadBranch(ctx, domain.BranchID(branch.ID), branch.BasePosition, maxComparisonEvents)
	if err != nil {
		return nil, fmt.Errorf("read branch events: %w", err)
	}
	branchHasMore := len(rawBranchEvents) >= maxComparisonEvents
	branchEvents := withoutBranchLifecycleEvents(rawBranchEvents)

	// Main side: only the streams the branch actually touched.
	rawMainEvents, mainHasMore, err := s.readMainTail(ctx, branchStreamIDs(branchEvents), branch.BasePosition)
	if err != nil {
		return nil, err
	}

	return &branchDiffSources{
		branch:          branch,
		branchEvents:    branchEvents,
		mainEvents:      withoutBranchLifecycleEvents(rawMainEvents),
		branchTruncated: branchHasMore,
		mainTruncated:   mainHasMore,
	}, nil
}

// readMainTail reads main's events after basePosition for the given streams only.
// It reports whether the read hit the cap, in which case the comparison is
// partial.
//
// One set-based query, not one query per stream: the store filters to main,
// applies position > basePosition, orders by position, and enforces the cap,
// so the work is bounded by the cap rather than by the branch's stream count
// (ADR-005 Implementation Notes).
func (s *BranchService) readMainTail(ctx context.Context, streamIDs []uuid.UUID, basePosition int64) ([]repository.StoredEvent, bool, error) {
	events, err := s.eventStore.ReadStreamsForBranch(ctx, streamIDs, domain.MainBranchID, basePosition, maxComparisonEvents)
	if err != nil {
		return nil, false, fmt.Errorf("read main events for branch streams: %w", err)
	}

	// A full page may or may not be the last one; the branch side and
	// CompareSnapshots report it as partial on the same conservative rule.
	return events, len(events) >= maxComparisonEvents, nil
}

// withoutBranchLifecycleEvents drops the events that describe a branch rather
// than the genealogy data on it.
func withoutBranchLifecycleEvents(events []repository.StoredEvent) []repository.StoredEvent {
	filtered := make([]repository.StoredEvent, 0, len(events))
	for _, evt := range events {
		if branchLifecycleEventTypes[evt.EventType] {
			continue
		}
		filtered = append(filtered, evt)
	}
	return filtered
}

// branchStreamIDs returns the distinct streams the events touched, in the order
// they were first touched.
func branchStreamIDs(events []repository.StoredEvent) []uuid.UUID {
	seen := make(map[uuid.UUID]bool, len(events))
	ids := make([]uuid.UUID, 0, len(events))
	for _, evt := range events {
		if seen[evt.StreamID] {
			continue
		}
		seen[evt.StreamID] = true
		ids = append(ids, evt.StreamID)
	}
	return ids
}

// overlappingStreamIDs returns the streams that appear on both sides of the
// comparison, in the order the branch first touched them. See the field comment
// on BranchComparisonResult.OverlappingStreamIDs: a hint, not a conflict verdict.
func overlappingStreamIDs(branchEvents, mainEvents []repository.StoredEvent) []uuid.UUID {
	mainStreams := make(map[uuid.UUID]bool, len(mainEvents))
	for _, evt := range mainEvents {
		mainStreams[evt.StreamID] = true
	}

	var overlapping []uuid.UUID
	for _, streamID := range branchStreamIDs(branchEvents) {
		if mainStreams[streamID] {
			overlapping = append(overlapping, streamID)
		}
	}
	return overlapping
}
