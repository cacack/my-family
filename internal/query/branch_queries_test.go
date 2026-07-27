package query

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// branchTestFixture wires the in-memory backends the branch query tests share.
type branchTestFixture struct {
	ctx         context.Context
	eventStore  *memory.EventStore
	branchStore *memory.BranchStore
	service     *BranchService
}

func newBranchTestFixture(t *testing.T) *branchTestFixture {
	t.Helper()

	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	branchStore := memory.NewBranchStore()

	return &branchTestFixture{
		ctx:         context.Background(),
		eventStore:  eventStore,
		branchStore: branchStore,
		service:     NewBranchService(branchStore, eventStore, NewHistoryService(eventStore, readStore)),
	}
}

// anyVersion skips the optimistic-concurrency check on append. These tests
// exercise the comparison, not versioning, which internal/repository covers.
const anyVersion = int64(-1)

// appendMain appends events to a stream on the mainline.
func (f *branchTestFixture) appendMain(t *testing.T, streamID uuid.UUID, events ...domain.Event) {
	t.Helper()
	require.NoError(t, f.eventStore.Append(f.ctx, streamID, "person", events, anyVersion, repository.MainScope))
}

// appendBranch appends events to a stream on a branch.
func (f *branchTestFixture) appendBranch(t *testing.T, branch *domain.Branch, streamID uuid.UUID, streamType string, events ...domain.Event) {
	t.Helper()
	scope := repository.AppendScope{BranchID: domain.BranchID(branch.ID), BasePosition: branch.BasePosition}
	require.NoError(t, f.eventStore.Append(f.ctx, streamID, streamType, events, anyVersion, scope))
}

// maxPosition returns the current tip of the log — the position a branch forked
// here would record as its base.
func (f *branchTestFixture) maxPosition(t *testing.T) int64 {
	t.Helper()
	events, err := f.eventStore.ReadAll(f.ctx, 0, maxComparisonEvents)
	require.NoError(t, err)
	if len(events) == 0 {
		return 0
	}
	return events[len(events)-1].Position
}

// forkBranch registers an active branch at the current tip of the log.
func (f *branchTestFixture) forkBranch(t *testing.T, name string) *domain.Branch {
	t.Helper()
	branch, err := domain.NewBranch(name, "", f.maxPosition(t))
	require.NoError(t, err)
	require.NoError(t, f.branchStore.Create(f.ctx, branch))
	return branch
}

func entityIDs(entries []ChangeEntry) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.EntityID)
	}
	return ids
}

func TestNewBranchService(t *testing.T) {
	branchStore := memory.NewBranchStore()
	eventStore := memory.NewEventStore()
	historyService := &HistoryService{}

	service := NewBranchService(branchStore, eventStore, historyService)

	assert.NotNil(t, service)
	assert.Equal(t, branchStore, service.branchStore)
	assert.Equal(t, eventStore, service.eventStore)
	assert.Equal(t, historyService, service.historyService)
}

func TestBranchService_ListBranches(t *testing.T) {
	f := newBranchTestFixture(t)

	t.Run("empty", func(t *testing.T) {
		branches, err := f.service.ListBranches(f.ctx)
		require.NoError(t, err)
		assert.Empty(t, branches)
	})

	t.Run("returns registered branches", func(t *testing.T) {
		first := f.forkBranch(t, "Maternal line")
		second := f.forkBranch(t, "Paternal line")

		branches, err := f.service.ListBranches(f.ctx)
		require.NoError(t, err)
		require.Len(t, branches, 2)

		names := []string{branches[0].Name, branches[1].Name}
		assert.ElementsMatch(t, []string{first.Name, second.Name}, names)
	})
}

func TestBranchService_GetBranch(t *testing.T) {
	f := newBranchTestFixture(t)
	branch := f.forkBranch(t, "Smith hypothesis")

	t.Run("found", func(t *testing.T) {
		got, err := f.service.GetBranch(f.ctx, branch.ID)
		require.NoError(t, err)
		assert.Equal(t, branch.ID, got.ID)
		assert.Equal(t, "Smith hypothesis", got.Name)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := f.service.GetBranch(f.ctx, uuid.New())
		assert.ErrorIs(t, err, repository.ErrBranchNotFound)
	})
}

func TestBranchService_CompareBranch_NotFound(t *testing.T) {
	f := newBranchTestFixture(t)

	_, err := f.service.CompareBranch(f.ctx, uuid.New())
	assert.ErrorIs(t, err, repository.ErrBranchNotFound)
}

// TestBranchService_CompareBranch covers the full comparison shape in one
// scenario: three persons exist on main before the fork; the branch edits two of
// them; main then edits one of those two (contested) plus one the branch never
// touched (churn that must stay out of the diff).
func TestBranchService_CompareBranch(t *testing.T) {
	f := newBranchTestFixture(t)

	contested := uuid.New()
	branchOnly := uuid.New()
	untouched := uuid.New()

	f.appendMain(t, contested, domain.NewPersonCreated(&domain.Person{ID: contested, GivenName: "Ada", Surname: "Byron"}))
	f.appendMain(t, branchOnly, domain.NewPersonCreated(&domain.Person{ID: branchOnly, GivenName: "Grace", Surname: "Hopper"}))
	f.appendMain(t, untouched, domain.NewPersonCreated(&domain.Person{ID: untouched, GivenName: "Alan", Surname: "Turing"}))

	branch := f.forkBranch(t, "Byron parentage")

	// Branch-side work, bracketed by the lifecycle events that must never show up
	// in a diff.
	f.appendBranch(t, branch, branch.ID, "branch", domain.NewBranchCreated(branch))
	f.appendBranch(t, branch, contested, "person", domain.NewPersonUpdated(contested, map[string]any{"surname": "Lovelace"}))
	f.appendBranch(t, branch, branchOnly, "person", domain.NewPersonUpdated(branchOnly, map[string]any{"surname": "Murray"}))
	f.appendBranch(t, branch, branch.ID, "branch", domain.NewBranchDeleted(branch.ID))

	// Main moved on underneath the branch.
	f.appendMain(t, contested, domain.NewPersonUpdated(contested, map[string]any{"given_name": "Augusta Ada"}))
	f.appendMain(t, untouched, domain.NewPersonUpdated(untouched, map[string]any{"given_name": "Alan Mathison"}))

	result, err := f.service.CompareBranch(f.ctx, branch.ID)
	require.NoError(t, err)

	assert.Equal(t, branch.ID, result.Branch.ID)
	assert.Equal(t, branch.BasePosition, result.BasePosition)
	assert.False(t, result.HasMore)

	// Branch side: the branch's two edits, lifecycle events excluded.
	assert.Equal(t, []uuid.UUID{contested, branchOnly}, entityIDs(result.BranchChanges))
	assert.Equal(t, 2, result.BranchChangeCount)

	// Main side: the contested edit only — main's churn on a stream the branch
	// never touched proves the scan is scoped, not global.
	assert.Equal(t, []uuid.UUID{contested}, entityIDs(result.MainChanges))
	assert.Equal(t, 1, result.MainChangeCount)
	assert.NotContains(t, entityIDs(result.MainChanges), untouched)

	// Main's pre-fork PersonCreated events are below base_position and excluded.
	for _, entry := range result.MainChanges {
		assert.Equal(t, "updated", entry.Action)
	}

	// The both-sides hint names exactly the contested stream.
	assert.Equal(t, []uuid.UUID{contested}, result.OverlappingStreamIDs)

	// Neither side carries a branch-lifecycle change.
	for _, entry := range append(append([]ChangeEntry{}, result.BranchChanges...), result.MainChanges...) {
		assert.NotEqual(t, branch.ID, entry.EntityID, "branch lifecycle event leaked into the diff")
	}
}

func TestBranchService_CompareBranch_NoOverlap(t *testing.T) {
	f := newBranchTestFixture(t)

	personID := uuid.New()
	f.appendMain(t, personID, domain.NewPersonCreated(&domain.Person{ID: personID, GivenName: "Ida", Surname: "Wells"}))

	branch := f.forkBranch(t, "Wells research")
	f.appendBranch(t, branch, personID, "person", domain.NewPersonUpdated(personID, map[string]any{"surname": "Wells-Barnett"}))

	result, err := f.service.CompareBranch(f.ctx, branch.ID)
	require.NoError(t, err)

	assert.Len(t, result.BranchChanges, 1)
	assert.Empty(t, result.MainChanges)
	assert.Empty(t, result.OverlappingStreamIDs)
	assert.False(t, result.HasMore)
}

// A branch with no work of its own compares cleanly rather than scanning main.
func TestBranchService_CompareBranch_EmptyBranch(t *testing.T) {
	f := newBranchTestFixture(t)

	personID := uuid.New()
	f.appendMain(t, personID, domain.NewPersonCreated(&domain.Person{ID: personID, GivenName: "Bess", Surname: "Coleman"}))

	branch := f.forkBranch(t, "Untouched")

	// Main churn after the fork: with no branch streams there is nothing to
	// compare against, so none of it appears.
	f.appendMain(t, personID, domain.NewPersonUpdated(personID, map[string]any{"surname": "Colman"}))

	result, err := f.service.CompareBranch(f.ctx, branch.ID)
	require.NoError(t, err)

	assert.Empty(t, result.BranchChanges)
	assert.Empty(t, result.MainChanges)
	assert.Empty(t, result.OverlappingStreamIDs)
	assert.Equal(t, 0, result.BranchChangeCount)
	assert.Equal(t, 0, result.MainChangeCount)
}

// Terminal branches keep their history in the append-only log (ES-002), so a
// merged or archived branch still compares instead of erroring or going blank.
func TestBranchService_CompareBranch_TerminalBranch(t *testing.T) {
	for _, status := range []domain.BranchStatus{domain.BranchStatusMerged, domain.BranchStatusArchived} {
		t.Run(string(status), func(t *testing.T) {
			f := newBranchTestFixture(t)

			personID := uuid.New()
			f.appendMain(t, personID, domain.NewPersonCreated(&domain.Person{ID: personID, GivenName: "Mary", Surname: "Seacole"}))

			branch := f.forkBranch(t, "Seacole line")
			f.appendBranch(t, branch, personID, "person", domain.NewPersonUpdated(personID, map[string]any{"surname": "Grant"}))
			require.NoError(t, f.branchStore.UpdateStatus(f.ctx, branch.ID, status))

			result, err := f.service.CompareBranch(f.ctx, branch.ID)
			require.NoError(t, err)

			assert.Equal(t, status, result.Branch.Status)
			assert.Len(t, result.BranchChanges, 1)
			assert.Equal(t, personID, result.BranchChanges[0].EntityID)
		})
	}
}

func TestBranchService_CompareBranch_HasMore(t *testing.T) {
	t.Run("branch side hits the cap", func(t *testing.T) {
		f := newBranchTestFixture(t)

		personID := uuid.New()
		f.appendMain(t, personID, domain.NewPersonCreated(&domain.Person{ID: personID, GivenName: "Nikola", Surname: "Tesla"}))

		branch := f.forkBranch(t, "Prolific")

		edits := make([]domain.Event, maxComparisonEvents)
		for i := range edits {
			edits[i] = domain.NewPersonUpdated(personID, map[string]any{"note": i})
		}
		f.appendBranch(t, branch, personID, "person", edits...)

		result, err := f.service.CompareBranch(f.ctx, branch.ID)
		require.NoError(t, err)

		assert.True(t, result.HasMore)
		assert.Equal(t, maxComparisonEvents, result.BranchChangeCount)
	})

	t.Run("main side hits the cap", func(t *testing.T) {
		f := newBranchTestFixture(t)

		personID := uuid.New()
		f.appendMain(t, personID, domain.NewPersonCreated(&domain.Person{ID: personID, GivenName: "Nikola", Surname: "Tesla"}))

		branch := f.forkBranch(t, "Quiet")
		f.appendBranch(t, branch, personID, "person", domain.NewPersonUpdated(personID, map[string]any{"surname": "Teslic"}))

		edits := make([]domain.Event, maxComparisonEvents+1)
		for i := range edits {
			edits[i] = domain.NewPersonUpdated(personID, map[string]any{"note": i})
		}
		f.appendMain(t, personID, edits...)

		result, err := f.service.CompareBranch(f.ctx, branch.ID)
		require.NoError(t, err)

		assert.True(t, result.HasMore)
		assert.Equal(t, maxComparisonEvents, result.MainChangeCount)
		assert.Equal(t, []uuid.UUID{personID}, result.OverlappingStreamIDs)
	})
}

// failingEventStore fails the one read it is configured for. The embedded
// interface is nil — only the overridden methods may be called.
type failingEventStore struct {
	repository.EventStore
	readBranchErr error
	readStreamErr error
}

func (s *failingEventStore) ReadBranch(_ context.Context, _ domain.BranchID, _ int64, _ int) ([]repository.StoredEvent, error) {
	if s.readBranchErr != nil {
		return nil, s.readBranchErr
	}
	return []repository.StoredEvent{{StreamID: uuid.New(), EventType: "PersonUpdated"}}, nil
}

func (s *failingEventStore) ReadStreamsForBranch(_ context.Context, _ []uuid.UUID, _ domain.BranchID, _ int64, _ int) ([]repository.StoredEvent, error) {
	return nil, s.readStreamErr
}

func TestBranchService_CompareBranch_ReadErrors(t *testing.T) {
	readErr := errors.New("boom")

	tests := []struct {
		name  string
		store *failingEventStore
	}{
		{"branch side", &failingEventStore{readBranchErr: readErr}},
		{"main side", &failingEventStore{readStreamErr: readErr}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			branchStore := memory.NewBranchStore()
			branch, err := domain.NewBranch("Broken", "", 0)
			require.NoError(t, err)
			require.NoError(t, branchStore.Create(context.Background(), branch))

			service := NewBranchService(branchStore, tt.store, NewHistoryService(tt.store, memory.NewReadModelStore()))

			_, err = service.CompareBranch(context.Background(), branch.ID)
			assert.ErrorIs(t, err, readErr)
		})
	}
}

// The main side is read per stream and must still be presented chronologically.
func TestBranchService_CompareBranch_MainSideOrderedByPosition(t *testing.T) {
	f := newBranchTestFixture(t)

	first := uuid.New()
	second := uuid.New()
	f.appendMain(t, first, domain.NewPersonCreated(&domain.Person{ID: first, GivenName: "Rosalind", Surname: "Franklin"}))
	f.appendMain(t, second, domain.NewPersonCreated(&domain.Person{ID: second, GivenName: "Barbara", Surname: "McClintock"}))

	branch := f.forkBranch(t, "Two streams")
	f.appendBranch(t, branch, first, "person", domain.NewPersonUpdated(first, map[string]any{"note": "branch"}))
	f.appendBranch(t, branch, second, "person", domain.NewPersonUpdated(second, map[string]any{"note": "branch"}))

	// Interleave main's edits so stream order and position order disagree.
	f.appendMain(t, second, domain.NewPersonUpdated(second, map[string]any{"note": "main-1"}))
	f.appendMain(t, first, domain.NewPersonUpdated(first, map[string]any{"note": "main-2"}))

	result, err := f.service.CompareBranch(f.ctx, branch.ID)
	require.NoError(t, err)

	assert.Equal(t, []uuid.UUID{second, first}, entityIDs(result.MainChanges))
	assert.Equal(t, []uuid.UUID{first, second}, result.OverlappingStreamIDs)
}

// countingEventStore records how many reads the main side of a comparison makes.
type countingEventStore struct {
	repository.EventStore
	setReads int
	lastCap  int
}

func (s *countingEventStore) ReadStreamsForBranch(ctx context.Context, streamIDs []uuid.UUID, branchID domain.BranchID, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
	s.setReads++
	s.lastCap = limit
	return s.EventStore.ReadStreamsForBranch(ctx, streamIDs, branchID, fromPosition, limit)
}

// The main side of a comparison must be ONE set-based read, not one read per
// aggregate the branch touched (ADR-005 Implementation Notes). The cap has to
// reach the store too, or it bounds the response instead of the work.
func TestBranchService_CompareBranch_MainSideIsOneSetRead(t *testing.T) {
	f := newBranchTestFixture(t)

	streamIDs := make([]uuid.UUID, 25)
	for i := range streamIDs {
		streamIDs[i] = uuid.New()
		f.appendMain(t, streamIDs[i], domain.NewPersonCreated(&domain.Person{ID: streamIDs[i], GivenName: "Person", Surname: "Placeholder"}))
	}

	branch := f.forkBranch(t, "Wide")
	for _, streamID := range streamIDs {
		f.appendBranch(t, branch, streamID, "person", domain.NewPersonUpdated(streamID, map[string]any{"note": "branch"}))
	}
	for _, streamID := range streamIDs {
		f.appendMain(t, streamID, domain.NewPersonUpdated(streamID, map[string]any{"note": "main"}))
	}

	counting := &countingEventStore{EventStore: f.eventStore}
	service := NewBranchService(f.branchStore, counting, NewHistoryService(counting, memory.NewReadModelStore()))

	result, err := service.CompareBranch(f.ctx, branch.ID)
	require.NoError(t, err)

	assert.Equal(t, 1, counting.setReads, "main side must be a single set-based read")
	assert.Equal(t, maxComparisonEvents, counting.lastCap, "the cap must be pushed into the store")
	assert.Len(t, result.MainChanges, len(streamIDs))
	assert.Len(t, result.OverlappingStreamIDs, len(streamIDs))
}
