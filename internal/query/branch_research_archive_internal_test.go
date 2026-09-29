package query

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// archiveEventStore interposes on the two reads the archive makes.
type archiveEventStore struct {
	repository.EventStore
	branchErr, mainErr error
	branchEvents       []repository.StoredEvent
}

func (s *archiveEventStore) ReadBranch(ctx context.Context, branchID domain.BranchID, from int64, limit int) ([]repository.StoredEvent, error) {
	if s.branchErr != nil {
		return nil, s.branchErr
	}
	if s.branchEvents != nil {
		return s.branchEvents, nil
	}
	return s.EventStore.ReadBranch(ctx, branchID, from, limit)
}

func (s *archiveEventStore) ReadStreamsForBranch(ctx context.Context, ids []uuid.UUID, branchID domain.BranchID, from int64, limit int) ([]repository.StoredEvent, error) {
	if s.mainErr != nil {
		return nil, s.mainErr
	}
	return s.EventStore.ReadStreamsForBranch(ctx, ids, branchID, from, limit)
}

func newResearchLog(subject uuid.UUID) *domain.ResearchLog {
	return domain.NewResearchLog(subject, "person", "Archive", "Search", domain.ResearchOutcomeNotFound, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
}

func TestBranchResearchArchive_TruncatesAtCap(t *testing.T) {
	f := newBranchTestFixture(t)
	subject := uuid.New()

	// Main: one log edited past the cap before the branch touches it.
	mainLog := newResearchLog(subject)
	mainEvents := []domain.Event{domain.NewResearchLogCreated(mainLog)}
	for i := 0; i < maxComparisonEvents; i++ {
		mainEvents = append(mainEvents, domain.NewResearchLogUpdated(mainLog.ID, map[string]any{"notes": "edit"}))
	}
	require.NoError(t, f.eventStore.Append(f.ctx, mainLog.ID, "ResearchLog", mainEvents, anyVersion, repository.MainScope))
	branch := f.forkBranch(t, "big")
	f.appendBranch(t, branch, mainLog.ID, "ResearchLog", domain.NewResearchLogUpdated(mainLog.ID, map[string]any{"notes": "branch"}))

	archive, err := f.service.BranchResearchArchive(f.ctx, branch.ID)
	require.NoError(t, err)
	assert.True(t, archive.Truncated, "a base read that fills the cap is reported as truncated")

	// Branch: more GPS events than the cap.
	other := f.forkBranch(t, "busy")
	var branchEvents []domain.Event
	for i := 0; i <= maxComparisonEvents; i++ {
		branchEvents = append(branchEvents, domain.NewResearchLogCreated(newResearchLog(subject)))
	}
	f.appendBranch(t, other, uuid.New(), "ResearchLog", branchEvents...)
	archive, err = f.service.BranchResearchArchive(f.ctx, other.ID)
	require.NoError(t, err)
	assert.True(t, archive.Truncated)
}

func TestBranchResearchArchive_ReadErrors(t *testing.T) {
	f := newBranchTestFixture(t)
	subject := uuid.New()
	mainLog := newResearchLog(subject)
	f.appendMain(t, mainLog.ID, domain.NewResearchLogCreated(mainLog))
	branch := f.forkBranch(t, "edits")
	f.appendBranch(t, branch, mainLog.ID, "ResearchLog", domain.NewResearchLogUpdated(mainLog.ID, map[string]any{"notes": "x"}))

	boom := errors.New("boom")
	for _, tc := range []struct {
		name  string
		store *archiveEventStore
	}{
		{"branch read", &archiveEventStore{EventStore: f.eventStore, branchErr: boom}},
		{"main read", &archiveEventStore{EventStore: f.eventStore, mainErr: boom}},
		{"corrupt GPS event", &archiveEventStore{EventStore: f.eventStore, branchEvents: []repository.StoredEvent{{
			ID: uuid.New(), StreamID: uuid.New(), StreamType: "ResearchLog", EventType: "ResearchLogCreated",
			Data: json.RawMessage(`{not json`), Version: 1, Position: branch.BasePosition + 1,
		}}}},
		{"corrupt person event", &archiveEventStore{EventStore: f.eventStore, branchEvents: []repository.StoredEvent{{
			ID: uuid.New(), StreamID: uuid.New(), StreamType: "person", EventType: "PersonCreated",
			Data: json.RawMessage(`{not json`), Version: 1, Position: branch.BasePosition + 1,
		}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := NewBranchService(f.branchStore, tc.store, NewHistoryService(tc.store, f.readStore))
			_, err := service.BranchResearchArchive(f.ctx, branch.ID)
			assert.Error(t, err)
		})
	}
}
