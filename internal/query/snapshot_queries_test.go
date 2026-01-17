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

func TestSnapshotService_CreateSnapshot(t *testing.T) {
	tests := []struct {
		name         string
		snapshotName string
		description  string
		maxPosition  int64
		wantErr      bool
	}{
		{
			name:         "valid snapshot",
			snapshotName: "Pre-DNA results",
			description:  "Before DNA test",
			maxPosition:  42,
			wantErr:      false,
		},
		{
			name:         "valid snapshot without description",
			snapshotName: "Milestone",
			description:  "",
			maxPosition:  10,
			wantErr:      false,
		},
		{
			name:         "empty name returns error",
			snapshotName: "",
			description:  "Description",
			maxPosition:  5,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var createdSnapshot *domain.Snapshot
			snapshotStore := &mockSnapshotStore{
				getMaxPositionFunc: func(ctx context.Context) (int64, error) {
					return tt.maxPosition, nil
				},
				createFunc: func(ctx context.Context, snapshot *domain.Snapshot) error {
					createdSnapshot = snapshot
					return nil
				},
			}

			service := NewSnapshotService(snapshotStore, &mockEventStore{}, &HistoryService{})
			snapshot, err := service.CreateSnapshot(context.Background(), tt.snapshotName, tt.description)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, snapshot)
			assert.Equal(t, tt.snapshotName, snapshot.Name)
			assert.Equal(t, tt.description, snapshot.Description)
			assert.Equal(t, tt.maxPosition, snapshot.Position)
			assert.NotEqual(t, uuid.Nil, snapshot.ID)
			assert.NotZero(t, snapshot.CreatedAt)

			// Verify the snapshot was passed to Create
			assert.Equal(t, snapshot, createdSnapshot)
		})
	}
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

func TestSnapshotService_DeleteSnapshot(t *testing.T) {
	snapshotID := uuid.New()

	snapshotStore := &mockSnapshotStore{
		deleteFunc: func(ctx context.Context, id uuid.UUID) error {
			if id == snapshotID {
				return nil
			}
			return repository.ErrSnapshotNotFound
		},
	}

	service := NewSnapshotService(snapshotStore, &mockEventStore{}, &HistoryService{})

	t.Run("success", func(t *testing.T) {
		err := service.DeleteSnapshot(context.Background(), snapshotID)
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		err := service.DeleteSnapshot(context.Background(), uuid.New())
		assert.ErrorIs(t, err, repository.ErrSnapshotNotFound)
	})
}

// mockEventStoreExt extends mockEventStore with ReadAll support for snapshot comparison tests.
type mockEventStoreExt struct {
	readByStreamFunc     func(ctx context.Context, streamID uuid.UUID, limit, offset int) (*repository.HistoryPage, error)
	readGlobalByTimeFunc func(ctx context.Context, fromTime, toTime time.Time, eventTypes []string, limit, offset int) (*repository.HistoryPage, error)
	readAllFunc          func(ctx context.Context, fromPosition int64, limit int) ([]repository.StoredEvent, error)
}

func (m *mockEventStoreExt) Append(ctx context.Context, streamID uuid.UUID, streamType string, events []domain.Event, expectedVersion int64) error {
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

func (m *mockEventStoreExt) GetStreamVersion(ctx context.Context, streamID uuid.UUID) (int64, error) {
	return 0, nil
}

func (m *mockEventStoreExt) ReadByStream(ctx context.Context, streamID uuid.UUID, limit, offset int) (*repository.HistoryPage, error) {
	if m.readByStreamFunc != nil {
		return m.readByStreamFunc(ctx, streamID, limit, offset)
	}
	return &repository.HistoryPage{}, nil
}

func (m *mockEventStoreExt) ReadGlobalByTime(ctx context.Context, fromTime, toTime time.Time, eventTypes []string, limit, offset int) (*repository.HistoryPage, error) {
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

	eventStore := &mockEventStoreExt{
		readAllFunc: func(ctx context.Context, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
			var result []repository.StoredEvent
			for _, e := range events {
				if e.Position > fromPosition {
					result = append(result, e)
				}
			}
			return result, nil
		},
	}

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
