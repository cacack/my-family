// Package query provides CQRS query services for the genealogy application.
package query

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// researchMetadataEventTypes are the events that describe a research artifact —
// a branch, a snapshot — rather than the genealogy data it points at. ADR-005
// §Merge excludes them from replay, and a diff excludes them for the same
// reason: "this branch was created" is not a change to anyone's family tree.
//
// Membership matters beyond replay: two classifiers in merge_conflicts.go key
// off the "Created"/"Deleted" suffix, so an omission here would see
// SnapshotDeleted as a genealogy delete. Snapshot events do not reach those
// classifiers today: branch-scoped snapshots (#839) carry their branch in the
// payload and append on the mainline envelope, so no snapshot event lands on a
// branch's own event set. They stay listed as a guard should that ever change.
var researchMetadataEventTypes = map[string]bool{
	"BranchCreated":   true,
	"BranchUpdated":   true,
	"BranchDeleted":   true,
	"BranchMerged":    true,
	"SnapshotCreated": true,
	"SnapshotDeleted": true,
	// A resumed merge's decision record (#685), on the branch's own stream.
	"BranchMergeResumed": true,
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

// BranchLinks is the display data for a branch's research record (#835): the
// names of its subjects and the proof summaries it links. Entries that no
// longer resolve are simply absent.
type BranchLinks struct {
	// SubjectNames maps a subject's id to its display name.
	SubjectNames map[uuid.UUID]string
	// ProofSummaries are the linked proof summaries still visible, in the
	// branch's proof_summary_ids order.
	ProofSummaries []repository.ProofSummaryReadModel
}

// ResolveBranchLinks resolves the branch's subjects and proof summaries for
// display. An active branch resolves through its own overlay; a merged or
// archived one has no overlay left (its rows were purged), so it resolves
// through main — where a merged branch's records now live.
//
// Subject names cost one batched lookup per subject type (two more on a
// branch, for ids the overlay does not resolve; see resolveEntityNamesOn);
// proof summaries, capped at domain.MaxBranchProofSummaries, are read one at a
// time.
func (s *BranchService) ResolveBranchLinks(ctx context.Context, branch *domain.Branch) (*BranchLinks, error) {
	links := &BranchLinks{SubjectNames: map[uuid.UUID]string{}}
	if s.historyService == nil || s.historyService.readStore == nil {
		return links, nil
	}

	scope := domain.MainBranchID
	if branch.Status == domain.BranchStatusActive {
		scope = domain.BranchID(branch.ID)
	}

	refs := newEntityRefs()
	for _, subject := range branch.Subjects {
		refs.add(string(subject.Type), subject.ID)
	}
	names, err := s.historyService.resolveEntityNamesOn(ctx, scope, refs)
	if err != nil {
		return nil, err
	}
	for _, subject := range branch.Subjects {
		switch subject.Type {
		case domain.BranchSubjectPerson:
			if names.persons[subject.ID] != nil {
				links.SubjectNames[subject.ID] = names.personName(subject.ID, nil)
			}
		case domain.BranchSubjectFamily:
			if names.families[subject.ID] != nil {
				links.SubjectNames[subject.ID] = names.familyName(subject.ID, nil)
			}
		}
	}

	for _, id := range branch.ProofSummaryIDs {
		summary, err := s.historyService.readStore.GetProofSummary(ctx, scope, id)
		if err != nil {
			return nil, fmt.Errorf("resolve proof summary %s: %w", id, err)
		}
		if summary != nil {
			links.ProofSummaries = append(links.ProofSummaries, *summary)
		}
	}
	return links, nil
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

	// ReplayedChangeCount is, for a merged branch, how many of main's events
	// on the branch's entities are the merge's replay of the branch's own
	// changes (#832). They are left out of MainChanges, OverlappingStreamIDs
	// and Conflicts, which therefore describe the mainline's independent
	// changes only. Zero for any other branch.
	ReplayedChangeCount int `json:"replayed_change_count"`

	// MergeRecord is the merged branch's record of its merge (#832): nil for
	// a branch that was never merged.
	MergeRecord *MergeRecord `json:"merge_record,omitempty"`
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
// would include main's replayed copies of the branch's own changes; they are
// left out and counted instead (ReplayedChangeCount), and the merge's own
// record is attached (MergeRecord), so the page shows what was decided rather
// than a verdict recomputed against a main the merge itself changed (#832).
func (s *BranchService) CompareBranch(ctx context.Context, branchID uuid.UUID) (*BranchComparisonResult, error) {
	diff, err := s.loadBranchDiff(ctx, branchID)
	if err != nil {
		return nil, err
	}

	var (
		record   *MergeRecord
		replayed int
	)
	if diff.branch.Status == domain.BranchStatusMerged {
		markers, claimed, err := s.readMergeMarkers(ctx, diff.branch.ID)
		if err != nil {
			return nil, err
		}
		if claimed {
			if record, err = s.buildMergeRecord(ctx, diff.branch, markers, diff.branchEvents); err != nil {
				return nil, err
			}
			// The replayed copies are main's events on the branch's streams
			// after its fork, which is what mainEvents holds, up to its cap.
			if last, ok := replayedThroughPosition(&markers, diff); ok {
				record.ReplayedThroughPosition = &last
			}
		}
		diff.mainEvents, replayed = withoutReplayedCopies(diff.branch.ID, diff.branchEvents, diff.mainEvents)
	}

	// Each side is named as it sees itself: the branch's changes through the
	// branch overlay, main's on main.
	branchChanges, err := s.historyService.transformStoredEventsOn(ctx, domain.BranchID(diff.branch.ID), diff.branchEvents)
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
	// The review shows what each side says per contested field (#828). Only
	// the review needs it, so it is not part of detectConflicts, which the
	// merge plan shares.
	if err := s.describeConflictValues(ctx, diff, conflicts); err != nil {
		return nil, fmt.Errorf("describe conflicting values: %w", err)
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
		ReplayedChangeCount:  replayed,
		MergeRecord:          record,
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
//
// The two sides are loaded by separate calls rather than inline here because
// PlanMerge has to pin main's stream versions BETWEEN them — see PlanMerge.
// CompareBranch, which needs no pin, composes them through this function.
func (s *BranchService) loadBranchDiff(ctx context.Context, branchID uuid.UUID) (*branchDiffSources, error) {
	diff, err := s.loadBranchSide(ctx, branchID)
	if err != nil {
		return nil, err
	}
	if err := s.loadMainSide(ctx, diff); err != nil {
		return nil, err
	}
	return diff, nil
}

// loadBranchSide loads the branch and its own events — the half of the diff that
// is read from the branch. The returned sources have no main side yet;
// loadMainSide fills it in.
func (s *BranchService) loadBranchSide(ctx context.Context, branchID uuid.UUID) (*branchDiffSources, error) {
	branch, err := s.branchStore.Get(ctx, branchID)
	if err != nil {
		return nil, fmt.Errorf("get branch: %w", err)
	}

	branchEvents, truncated, err := s.readBranchMutations(ctx, branch)
	if err != nil {
		return nil, err
	}

	return &branchDiffSources{
		branch:          branch,
		branchEvents:    branchEvents,
		branchTruncated: truncated,
	}, nil
}

// readBranchMutations reads the branch's own events with the research-metadata
// events (researchMetadataEventTypes) stripped, and reports whether the scan
// hit maxComparisonEvents.
//
// The cap is measured against the events KEPT, not the raw rows read: a
// branch's own stream also carries its BranchUpdated research edits (#835),
// one per saved hypothesis, subject or outcome change, and none of them is
// shown in a comparison or replayed by a merge. Counting them would let a
// long-running line of research edge a branch towards a partial comparison and
// ErrBranchTooLargeToMerge without a single extra genealogy change. So the
// scan pages on until it has maxComparisonEvents kept events or reaches the
// end of the branch. Every extra page is bounded by the number of research
// edits, which are appended one at a time by a person.
//
// Truncation keeps the conservative rule used elsewhere: a scan that stops
// with the cap reached reports truncation even if nothing followed.
func (s *BranchService) readBranchMutations(ctx context.Context, branch *domain.Branch) ([]repository.StoredEvent, bool, error) {
	// fromPosition is exclusive, and every branch event is appended after the
	// fork, so the base position is a no-op filter on the first page — it is
	// passed for symmetry with the main side.
	from := branch.BasePosition
	var kept []repository.StoredEvent
	for {
		page, err := s.eventStore.ReadBranch(ctx, domain.BranchID(branch.ID), from, maxComparisonEvents)
		if err != nil {
			return nil, false, fmt.Errorf("read branch events: %w", err)
		}
		kept = append(kept, withoutBranchLifecycleEvents(page)...)
		if len(kept) >= maxComparisonEvents {
			return kept[:maxComparisonEvents], true, nil
		}
		if len(page) < maxComparisonEvents {
			return kept, false, nil
		}
		from = page[len(page)-1].Position
	}
}

// loadMainSide fills in the main half of the diff: main's events after the base
// position, restricted to the streams the branch actually touched — plus the
// streams of the persons the branch merged away (#834), whose PersonMerged ends
// them without writing to their stream (see comparedStreamIDs).
func (s *BranchService) loadMainSide(ctx context.Context, diff *branchDiffSources) error {
	rawMainEvents, mainHasMore, err := s.readMainTail(ctx, comparedStreamIDs(diff.branchEvents), diff.branch.BasePosition)
	if err != nil {
		return err
	}

	diff.mainEvents = withoutBranchLifecycleEvents(rawMainEvents)
	diff.mainTruncated = mainHasMore
	return nil
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
		if researchMetadataEventTypes[evt.EventType] {
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

// comparedStreamIDs returns the streams whose main events a branch is compared
// against: every stream the branch wrote, in first-touch order, then every
// person the branch merged away (PersonMerged.MergedID) that it did not also
// write. A person merge lands on the survivor's stream but deletes the merged
// person too, so main changing the merged person after the fork is a
// disagreement the classifier must see (mergedPersonsMainChanged), and main's
// version of that stream is pinned with the rest (#698).
func comparedStreamIDs(events []repository.StoredEvent) []uuid.UUID {
	ids := branchStreamIDs(events)
	seen := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		seen[id] = true
	}
	for _, id := range mergedPersonIDs(events) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
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

// maxDriftCount caps each "main moved" count. The counts are a cheap
// at-a-glance indicator, not a diff, so past this the UI says "N+" rather than
// scanning main's whole tail.
const maxDriftCount = 10000

// BranchDrift reports how far main has moved underneath a branch since it
// forked. Branches are live overlays (ADR-005 §The model): main's later edits
// show through for every entity the branch has not touched, so this is the
// cheap indicator the branch UI shows without running a full compare.
type BranchDrift struct {
	BranchID     uuid.UUID `json:"branch_id"`
	BasePosition int64     `json:"base_position"`

	// MainChangeCount is main's genealogy changes since the fork, on any entity.
	MainChangeCount int `json:"main_change_count"`

	// MainChangeCountOnBranchEntities is the subset of MainChangeCount on
	// entities the branch itself changed: the main events CompareBranch draws
	// its MainChanges from, counted rather than loaded and transformed.
	MainChangeCountOnBranchEntities int `json:"main_change_count_on_branch_entities"`

	// HasMore reports that a count hit maxDriftCount and is a lower bound.
	HasMore bool `json:"has_more"`
}

// GetBranchDrift returns the "main moved" counts for one branch, returning
// repository.ErrBranchNotFound when it does not exist.
func (s *BranchService) GetBranchDrift(ctx context.Context, id uuid.UUID) (*BranchDrift, error) {
	branch, err := s.branchStore.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get branch: %w", err)
	}

	drifts, err := s.BranchDrifts(ctx, []*domain.Branch{branch})
	if err != nil {
		return nil, err
	}
	drift := drifts[branch.ID]
	return &drift, nil
}

// BranchDrifts returns the "main moved" counts for every given branch with a
// single event-store query, however many branches there are, keyed by branch
// ID. Every given branch has an entry.
func (s *BranchService) BranchDrifts(ctx context.Context, branches []*domain.Branch) (map[uuid.UUID]BranchDrift, error) {
	result := make(map[uuid.UUID]BranchDrift, len(branches))
	if len(branches) == 0 {
		return result, nil
	}

	scopes := make([]repository.DriftScope, len(branches))
	for i, b := range branches {
		scopes[i] = repository.DriftScope{BranchID: domain.BranchID(b.ID), BasePosition: b.BasePosition}
	}

	counts, err := s.eventStore.CountMainDrift(ctx, scopes, driftExcludedEventTypeList(), maxDriftCount)
	if err != nil {
		return nil, fmt.Errorf("count main drift: %w", err)
	}

	for _, b := range branches {
		count := counts[domain.BranchID(b.ID)]
		result[b.ID] = BranchDrift{
			BranchID:                        b.ID,
			BasePosition:                    b.BasePosition,
			MainChangeCount:                 count.MainChanges,
			MainChangeCountOnBranchEntities: count.MainChangesOnBranchStreams,
			HasMore:                         count.MainChanges >= maxDriftCount || count.MainChangesOnBranchStreams >= maxDriftCount,
		}
	}
	return result, nil
}

// driftExcludedEventTypes are the main events the drift counts skip:
// research metadata, plus GedcomImported — the import's own summary record, on
// its own stream, alongside the per-entity events that carry the actual changes.
// History (and so compare) drops it as a non-change, so counting it would make
// a 500-person import on main read as 501 changes.
var driftExcludedEventTypes = func() map[string]bool {
	excluded := map[string]bool{"GedcomImported": true}
	for t := range researchMetadataEventTypes {
		excluded[t] = true
	}
	return excluded
}()

// driftExcludedEventTypeList is driftExcludedEventTypes as a sorted slice, for
// stores that filter on it in the query itself.
func driftExcludedEventTypeList() []string {
	types := make([]string, 0, len(driftExcludedEventTypes))
	for t := range driftExcludedEventTypes {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}
