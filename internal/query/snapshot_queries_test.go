package query

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// mockSnapshotStore implements repository.SnapshotStore for testing.
type mockSnapshotStore struct {
	createFunc         func(ctx context.Context, snapshot *domain.Snapshot) error
	upsertFunc         func(ctx context.Context, snapshot *domain.Snapshot) error
	getFunc            func(ctx context.Context, id uuid.UUID) (*domain.Snapshot, error)
	listFunc           func(ctx context.Context) ([]*domain.Snapshot, error)
	deleteFunc         func(ctx context.Context, id uuid.UUID) error
	getMaxPositionFunc func(ctx context.Context) (int64, error)
}

func (m *mockSnapshotStore) Create(ctx context.Context, snapshot *domain.Snapshot) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, snapshot)
	}
	return nil
}

func (m *mockSnapshotStore) Upsert(ctx context.Context, snapshot *domain.Snapshot) error {
	if m.upsertFunc != nil {
		return m.upsertFunc(ctx, snapshot)
	}
	return nil
}

func (m *mockSnapshotStore) Get(ctx context.Context, id uuid.UUID) (*domain.Snapshot, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, id)
	}
	return nil, repository.ErrSnapshotNotFound
}

func (m *mockSnapshotStore) List(ctx context.Context) ([]*domain.Snapshot, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx)
	}
	return []*domain.Snapshot{}, nil
}

func (m *mockSnapshotStore) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return nil
}

func (m *mockSnapshotStore) GetMaxPosition(ctx context.Context) (int64, error) {
	if m.getMaxPositionFunc != nil {
		return m.getMaxPositionFunc(ctx)
	}
	return 0, nil
}

func TestNewSnapshotService(t *testing.T) {
	snapshotStore := &mockSnapshotStore{}
	eventStore := &mockEventStore{}
	historyService := &HistoryService{}

	service := NewSnapshotService(snapshotStore, eventStore, historyService)

	assert.NotNil(t, service)
	assert.Equal(t, snapshotStore, service.snapshotStore)
	assert.Equal(t, eventStore, service.eventStore)
	assert.Equal(t, historyService, service.historyService)
}

func TestSnapshotService_ListSnapshots(t *testing.T) {
	now := time.Now().UTC()
	snapshots := []*domain.Snapshot{
		{
			ID:        uuid.New(),
			Name:      "Third",
			Position:  30,
			CreatedAt: now,
		},
		{
			ID:        uuid.New(),
			Name:      "Second",
			Position:  20,
			CreatedAt: now.Add(-1 * time.Hour),
		},
		{
			ID:        uuid.New(),
			Name:      "First",
			Position:  10,
			CreatedAt: now.Add(-2 * time.Hour),
		},
	}

	snapshotStore := &mockSnapshotStore{
		listFunc: func(ctx context.Context) ([]*domain.Snapshot, error) {
			return snapshots, nil
		},
	}

	service := NewSnapshotService(snapshotStore, &mockEventStore{}, &HistoryService{})
	result, err := service.ListSnapshots(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 3, len(result))
	assert.Equal(t, "Third", result[0].Name)
	assert.Equal(t, "Second", result[1].Name)
	assert.Equal(t, "First", result[2].Name)
}

func TestSnapshotService_GetSnapshot(t *testing.T) {
	snapshotID := uuid.New()
	now := time.Now().UTC()

	snapshot := &domain.Snapshot{
		ID:          snapshotID,
		Name:        "Test Snapshot",
		Description: "Test description",
		Position:    42,
		CreatedAt:   now,
	}

	snapshotStore := &mockSnapshotStore{
		getFunc: func(ctx context.Context, id uuid.UUID) (*domain.Snapshot, error) {
			if id == snapshotID {
				return snapshot, nil
			}
			return nil, repository.ErrSnapshotNotFound
		},
	}

	service := NewSnapshotService(snapshotStore, &mockEventStore{}, &HistoryService{})

	t.Run("found", func(t *testing.T) {
		result, err := service.GetSnapshot(context.Background(), snapshotID)
		require.NoError(t, err)
		assert.Equal(t, snapshot, result)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := service.GetSnapshot(context.Background(), uuid.New())
		assert.ErrorIs(t, err, repository.ErrSnapshotNotFound)
	})
}

// mockEventStoreExt extends mockEventStore with ReadAll support for snapshot comparison tests.
type mockEventStoreExt struct {
	readByStreamFunc     func(ctx context.Context, streamID uuid.UUID, branchID domain.BranchID, limit, offset int) (*repository.HistoryPage, error)
	readGlobalByTimeFunc func(ctx context.Context, fromTime, toTime time.Time, eventTypes []string, limit, offset int) (*repository.HistoryPage, error)
	readAllFunc          func(ctx context.Context, fromPosition int64, limit int) ([]repository.StoredEvent, error)
	readBranchFunc       func(ctx context.Context, branchID domain.BranchID, fromPosition int64, limit int) ([]repository.StoredEvent, error)
}

func (m *mockEventStoreExt) Append(ctx context.Context, streamID uuid.UUID, streamType string, events []domain.Event, expectedVersion int64, scope repository.AppendScope) error {
	return nil
}

func (m *mockEventStoreExt) ReadStream(ctx context.Context, streamID uuid.UUID) ([]repository.StoredEvent, error) {
	return nil, nil
}

func (m *mockEventStoreExt) ReadAll(ctx context.Context, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
	if m.readAllFunc != nil {
		return m.readAllFunc(ctx, fromPosition, limit)
	}
	return nil, nil
}

func (m *mockEventStoreExt) ReadBranch(ctx context.Context, branchID domain.BranchID, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
	if m.readBranchFunc != nil {
		return m.readBranchFunc(ctx, branchID, fromPosition, limit)
	}
	return nil, nil
}

// branchReader mimics EventStore.ReadBranch over a fixed log: the events tagged
// with branchID, after fromPosition, in position order, capped at limit.
func branchReader(log []repository.StoredEvent) func(ctx context.Context, branchID domain.BranchID, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
	return func(_ context.Context, branchID domain.BranchID, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
		var result []repository.StoredEvent
		for _, e := range log {
			if e.BranchID == branchID && e.Position > fromPosition {
				result = append(result, e)
				if len(result) >= limit {
					break
				}
			}
		}
		return result, nil
	}
}

func (m *mockEventStoreExt) GetStreamVersion(ctx context.Context, streamID uuid.UUID, branchID domain.BranchID) (int64, error) {
	return 0, nil
}

func (m *mockEventStoreExt) ReadStreamsForBranch(ctx context.Context, streamIDs []uuid.UUID, branchID domain.BranchID, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
	return nil, nil
}

func (m *mockEventStoreExt) ReadByStream(ctx context.Context, streamID uuid.UUID, branchID domain.BranchID, limit, offset int) (*repository.HistoryPage, error) {
	if m.readByStreamFunc != nil {
		return m.readByStreamFunc(ctx, streamID, branchID, limit, offset)
	}
	return &repository.HistoryPage{}, nil
}

func (m *mockEventStoreExt) readGlobalByTime(ctx context.Context, fromTime, toTime time.Time, eventTypes []string, limit, offset int) (*repository.HistoryPage, error) {
	if m.readGlobalByTimeFunc != nil {
		return m.readGlobalByTimeFunc(ctx, fromTime, toTime, eventTypes, limit, offset)
	}
	return &repository.HistoryPage{}, nil
}

func TestSnapshotService_CompareSnapshots(t *testing.T) {
	snapshot1ID := uuid.New()
	snapshot2ID := uuid.New()
	now := time.Now().UTC()

	snapshot1 := &domain.Snapshot{
		ID:        snapshot1ID,
		Name:      "First",
		Position:  10,
		CreatedAt: now.Add(-1 * time.Hour),
	}

	snapshot2 := &domain.Snapshot{
		ID:        snapshot2ID,
		Name:      "Second",
		Position:  20,
		CreatedAt: now,
	}

	personID := uuid.New()
	personCreatedEvent := domain.NewPersonCreated(&domain.Person{
		ID:        personID,
		GivenName: "John",
		Surname:   "Smith",
	})
	createdData := mustMarshal(personCreatedEvent)

	events := []repository.StoredEvent{
		{
			ID:         uuid.New(),
			StreamID:   personID,
			StreamType: "person",
			EventType:  "PersonCreated",
			Data:       createdData,
			Version:    1,
			Position:   15,
			Timestamp:  now.Add(-30 * time.Minute),
		},
	}

	snapshotStore := &mockSnapshotStore{
		getFunc: func(ctx context.Context, id uuid.UUID) (*domain.Snapshot, error) {
			if id == snapshot1ID {
				return snapshot1, nil
			}
			if id == snapshot2ID {
				return snapshot2, nil
			}
			return nil, repository.ErrSnapshotNotFound
		},
	}

	// A research branch's delta between the two positions, and a mainline
	// event after the newer one: neither belongs in the comparison.
	branchPersonID := uuid.New()
	events = append(events,
		repository.StoredEvent{
			ID:         uuid.New(),
			StreamID:   branchPersonID,
			StreamType: "person",
			BranchID:   domain.BranchID(uuid.New()),
			EventType:  "PersonCreated",
			Data:       mustMarshal(domain.NewPersonCreated(&domain.Person{ID: branchPersonID, GivenName: "Branch", Surname: "Only"})),
			Version:    1,
			Position:   16,
			Timestamp:  now.Add(-20 * time.Minute),
		},
		repository.StoredEvent{
			ID:         uuid.New(),
			StreamID:   branchPersonID,
			StreamType: "person",
			EventType:  "PersonCreated",
			Data:       mustMarshal(domain.NewPersonCreated(&domain.Person{ID: branchPersonID, GivenName: "Later", Surname: "Main"})),
			Version:    1,
			Position:   25,
			Timestamp:  now.Add(10 * time.Minute),
		},
	)

	eventStore := &mockEventStoreExt{readBranchFunc: branchReader(events)}

	readStore := &mockReadModelStore{
		getPersonFunc: func(ctx context.Context, id uuid.UUID) (*repository.PersonReadModel, error) {
			if id == personID {
				return &repository.PersonReadModel{
					ID:       personID,
					FullName: "John Smith",
				}, nil
			}
			return nil, repository.ErrStreamNotFound
		},
	}

	historyService := NewHistoryService(eventStore, readStore)
	service := NewSnapshotService(snapshotStore, eventStore, historyService)

	t.Run("compare two snapshots", func(t *testing.T) {
		result, err := service.CompareSnapshots(context.Background(), snapshot1ID, snapshot2ID)

		require.NoError(t, err)
		assert.Equal(t, snapshot1, result.Snapshot1)
		assert.Equal(t, snapshot2, result.Snapshot2)
		assert.True(t, result.OlderFirst)
		assert.Equal(t, 1, result.TotalCount)
		assert.Equal(t, 1, len(result.Changes))
		assert.Equal(t, "person", result.Changes[0].EntityType)
		assert.Equal(t, "created", result.Changes[0].Action)
	})

	t.Run("compare in reverse order", func(t *testing.T) {
		result, err := service.CompareSnapshots(context.Background(), snapshot2ID, snapshot1ID)

		require.NoError(t, err)
		assert.Equal(t, snapshot2, result.Snapshot1)
		assert.Equal(t, snapshot1, result.Snapshot2)
		assert.False(t, result.OlderFirst) // snapshot1 is actually the older one
	})

	t.Run("excludes research branch events and events past the newer snapshot", func(t *testing.T) {
		result, err := service.CompareSnapshots(context.Background(), snapshot1ID, snapshot2ID)

		require.NoError(t, err)
		require.Len(t, result.Changes, 1)
		assert.Equal(t, personID, result.Changes[0].EntityID)
		assert.False(t, result.HasMore)
	})

	t.Run("read error is returned", func(t *testing.T) {
		failing := &mockEventStoreExt{
			readBranchFunc: func(context.Context, domain.BranchID, int64, int) ([]repository.StoredEvent, error) {
				return nil, assert.AnError
			},
		}
		svc := NewSnapshotService(snapshotStore, failing, historyService)
		_, err := svc.CompareSnapshots(context.Background(), snapshot1ID, snapshot2ID)
		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("snapshot not found", func(t *testing.T) {
		snapshotStoreNotFound := &mockSnapshotStore{
			getFunc: func(ctx context.Context, id uuid.UUID) (*domain.Snapshot, error) {
				return nil, repository.ErrSnapshotNotFound
			},
		}
		svc := NewSnapshotService(snapshotStoreNotFound, eventStore, historyService)
		_, err := svc.CompareSnapshots(context.Background(), uuid.New(), snapshot2ID)
		assert.Error(t, err)
	})
}

func TestSnapshotService_CompareSnapshots_HasMore(t *testing.T) {
	older := &domain.Snapshot{ID: uuid.New(), Name: "Older", Position: 0}
	newer := &domain.Snapshot{ID: uuid.New(), Name: "Newer", Position: int64(maxComparisonEvents) + 10}

	snapshotStore := &mockSnapshotStore{
		getFunc: func(_ context.Context, id uuid.UUID) (*domain.Snapshot, error) {
			switch id {
			case older.ID:
				return older, nil
			case newer.ID:
				return newer, nil
			}
			return nil, repository.ErrSnapshotNotFound
		},
	}

	// A mainline log of `n` events at positions 1..n. The event type is one the
	// history catalog excludes, which keeps the fixture cheap: only the
	// positions matter to HasMore.
	logOf := func(n int) []repository.StoredEvent {
		log := make([]repository.StoredEvent, n)
		for i := range log {
			log[i] = repository.StoredEvent{ID: uuid.New(), EventType: "SnapshotCreated", Position: int64(i + 1)}
		}
		return log
	}

	compare := func(t *testing.T, log []repository.StoredEvent, from, to *domain.Snapshot) *SnapshotComparisonResult {
		t.Helper()
		eventStore := &mockEventStoreExt{readBranchFunc: branchReader(log)}
		service := NewSnapshotService(snapshotStore, eventStore, NewHistoryService(eventStore, &mockReadModelStore{}))
		result, err := service.CompareSnapshots(context.Background(), from.ID, to.ID)
		require.NoError(t, err)
		return result
	}

	t.Run("a capped read that stops short of the newer snapshot has more", func(t *testing.T) {
		result := compare(t, logOf(maxComparisonEvents+20), older, newer)
		assert.True(t, result.HasMore)
	})

	t.Run("a capped read that runs past the newer snapshot is complete", func(t *testing.T) {
		mid := &domain.Snapshot{ID: uuid.New(), Name: "Mid", Position: 5}
		snapshotStore.getFunc = func(_ context.Context, id uuid.UUID) (*domain.Snapshot, error) {
			if id == mid.ID {
				return mid, nil
			}
			return older, nil
		}
		result := compare(t, logOf(maxComparisonEvents+20), older, mid)
		assert.False(t, result.HasMore)
	})

	t.Run("an uncapped read is complete", func(t *testing.T) {
		snapshotStore.getFunc = func(_ context.Context, id uuid.UUID) (*domain.Snapshot, error) {
			if id == newer.ID {
				return newer, nil
			}
			return older, nil
		}
		result := compare(t, logOf(3), older, newer)
		assert.False(t, result.HasMore)
	})
}

func (m *mockEventStoreExt) ReadGlobalHistory(ctx context.Context, q repository.GlobalHistoryQuery) (*repository.HistoryPage, error) {
	return m.readGlobalByTime(ctx, q.FromTime, q.ToTime, q.IncludeEventTypes, q.Limit, q.Offset)
}
