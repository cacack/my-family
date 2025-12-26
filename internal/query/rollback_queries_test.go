package query

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// rollbackMockEventStore implements repository.EventStore for rollback testing.
type rollbackMockEventStore struct {
	readStreamFunc func(ctx context.Context, streamID uuid.UUID) ([]repository.StoredEvent, error)
}

func (m *rollbackMockEventStore) Append(ctx context.Context, streamID uuid.UUID, streamType string, events []domain.Event, expectedVersion int64) error {
	return nil
}

func (m *rollbackMockEventStore) ReadStream(ctx context.Context, streamID uuid.UUID) ([]repository.StoredEvent, error) {
	if m.readStreamFunc != nil {
		return m.readStreamFunc(ctx, streamID)
	}
	return nil, repository.ErrStreamNotFound
}

func (m *rollbackMockEventStore) ReadAll(ctx context.Context, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
	return nil, nil
}

func (m *rollbackMockEventStore) GetStreamVersion(ctx context.Context, streamID uuid.UUID) (int64, error) {
	return 0, nil
}

func (m *rollbackMockEventStore) ReadByStream(ctx context.Context, streamID uuid.UUID, limit, offset int) (*repository.HistoryPage, error) {
	return &repository.HistoryPage{}, nil
}

func (m *rollbackMockEventStore) ReadGlobalByTime(ctx context.Context, fromTime, toTime time.Time, eventTypes []string, limit, offset int) (*repository.HistoryPage, error) {
	return &repository.HistoryPage{}, nil
}

func TestNewRollbackService(t *testing.T) {
	eventStore := &rollbackMockEventStore{}
	readStore := &mockReadModelStore{}

	service := NewRollbackService(eventStore, readStore)

	assert.NotNil(t, service)
	assert.Equal(t, eventStore, service.eventStore)
	assert.Equal(t, readStore, service.readStore)
}

func TestGetRestorePoints(t *testing.T) {
	personID := uuid.New()
	now := time.Now().UTC()

	personCreated := domain.PersonCreated{
		BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: now},
		PersonID:  personID,
		GivenName: "John",
		Surname:   "Smith",
	}
	createdData, _ := json.Marshal(personCreated)

	personUpdated := domain.PersonUpdated{
		BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: now.Add(1 * time.Hour)},
		PersonID:  personID,
		Changes:   map[string]any{"given_name": "Jonathan"},
	}
	updatedData, _ := json.Marshal(personUpdated)

	tests := []struct {
		name              string
		entityType        string
		entityID          uuid.UUID
		limit             int
		offset            int
		mockEvents        []repository.StoredEvent
		mockError         error
		wantRestorePoints int
		wantTotalCount    int
		wantHasMore       bool
		wantError         error
	}{
		{
			name:       "single event returns one restore point",
			entityType: "person",
			entityID:   personID,
			limit:      20,
			offset:     0,
			mockEvents: []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
			},
			wantRestorePoints: 1,
			wantTotalCount:    1,
			wantHasMore:       false,
		},
		{
			name:       "multiple events returns restore points in reverse order",
			entityType: "person",
			entityID:   personID,
			limit:      20,
			offset:     0,
			mockEvents: []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonUpdated",
					Data:       updatedData,
					Version:    2,
					Position:   2,
					Timestamp:  now.Add(1 * time.Hour),
				},
			},
			wantRestorePoints: 2,
			wantTotalCount:    2,
			wantHasMore:       false,
		},
		{
			name:       "pagination with limit",
			entityType: "person",
			entityID:   personID,
			limit:      1,
			offset:     0,
			mockEvents: []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonUpdated",
					Data:       updatedData,
					Version:    2,
					Position:   2,
					Timestamp:  now.Add(1 * time.Hour),
				},
			},
			wantRestorePoints: 1,
			wantTotalCount:    2,
			wantHasMore:       true,
		},
		{
			name:       "pagination with offset",
			entityType: "person",
			entityID:   personID,
			limit:      1,
			offset:     1,
			mockEvents: []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonUpdated",
					Data:       updatedData,
					Version:    2,
					Position:   2,
					Timestamp:  now.Add(1 * time.Hour),
				},
			},
			wantRestorePoints: 1,
			wantTotalCount:    2,
			wantHasMore:       false,
		},
		{
			name:       "offset beyond results",
			entityType: "person",
			entityID:   personID,
			limit:      20,
			offset:     10,
			mockEvents: []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
			},
			wantRestorePoints: 0,
			wantTotalCount:    1,
			wantHasMore:       false,
		},
		{
			name:       "no events returns error",
			entityType: "person",
			entityID:   personID,
			limit:      20,
			offset:     0,
			mockEvents: []repository.StoredEvent{},
			wantError:  ErrNoEvents,
		},
		{
			name:       "stream not found returns error",
			entityType: "person",
			entityID:   personID,
			limit:      20,
			offset:     0,
			mockError:  repository.ErrStreamNotFound,
			wantError:  ErrNoEvents,
		},
		{
			name:       "default limit when zero",
			entityType: "person",
			entityID:   personID,
			limit:      0,
			offset:     0,
			mockEvents: []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
			},
			wantRestorePoints: 1,
			wantTotalCount:    1,
			wantHasMore:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventStore := &rollbackMockEventStore{
				readStreamFunc: func(ctx context.Context, streamID uuid.UUID) ([]repository.StoredEvent, error) {
					if tt.mockError != nil {
						return nil, tt.mockError
					}
					return tt.mockEvents, nil
				},
			}

			service := NewRollbackService(eventStore, &mockReadModelStore{})
			result, err := service.GetRestorePoints(context.Background(), tt.entityType, tt.entityID, tt.limit, tt.offset)

			if tt.wantError != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantRestorePoints, len(result.RestorePoints))
			assert.Equal(t, tt.wantTotalCount, result.TotalCount)
			assert.Equal(t, tt.wantHasMore, result.HasMore)
		})
	}
}

func TestGetRestorePoints_Properties(t *testing.T) {
	personID := uuid.New()
	now := time.Now().UTC()

	personCreated := domain.PersonCreated{
		BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: now},
		PersonID:  personID,
		GivenName: "John",
		Surname:   "Smith",
	}
	createdData, _ := json.Marshal(personCreated)

	personUpdated := domain.PersonUpdated{
		BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: now.Add(1 * time.Hour)},
		PersonID:  personID,
		Changes:   map[string]any{"given_name": "Jonathan"},
	}
	updatedData, _ := json.Marshal(personUpdated)

	eventStore := &rollbackMockEventStore{
		readStreamFunc: func(ctx context.Context, streamID uuid.UUID) ([]repository.StoredEvent, error) {
			return []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonUpdated",
					Data:       updatedData,
					Version:    2,
					Position:   2,
					Timestamp:  now.Add(1 * time.Hour),
				},
			}, nil
		},
	}

	service := NewRollbackService(eventStore, &mockReadModelStore{})
	result, err := service.GetRestorePoints(context.Background(), "person", personID, 20, 0)

	require.NoError(t, err)
	require.Equal(t, 2, len(result.RestorePoints))

	// Current version should be marked as current and not restorable
	currentPoint := result.RestorePoints[0]
	assert.Equal(t, int64(2), currentPoint.Version)
	assert.True(t, currentPoint.IsCurrent)
	assert.False(t, currentPoint.IsRestorable)
	assert.Equal(t, "updated", currentPoint.Action)

	// Previous version should be restorable
	prevPoint := result.RestorePoints[1]
	assert.Equal(t, int64(1), prevPoint.Version)
	assert.False(t, prevPoint.IsCurrent)
	assert.True(t, prevPoint.IsRestorable)
	assert.Equal(t, "created", prevPoint.Action)
}

func TestGetStateAtVersion(t *testing.T) {
	personID := uuid.New()
	now := time.Now().UTC()

	personCreated := domain.PersonCreated{
		BaseEvent:  domain.BaseEvent{ID: uuid.New(), Timestamp: now},
		PersonID:   personID,
		GivenName:  "John",
		Surname:    "Smith",
		Gender:     domain.GenderMale,
		BirthPlace: "New York",
	}
	createdData, _ := json.Marshal(personCreated)

	personUpdated := domain.PersonUpdated{
		BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: now.Add(1 * time.Hour)},
		PersonID:  personID,
		Changes:   map[string]any{"given_name": "Jonathan", "birth_place": "Boston"},
	}
	updatedData, _ := json.Marshal(personUpdated)

	tests := []struct {
		name          string
		entityType    string
		entityID      uuid.UUID
		version       int64
		mockEvents    []repository.StoredEvent
		mockError     error
		wantState     map[string]any
		wantIsDeleted bool
		wantError     error
	}{
		{
			name:       "state at creation",
			entityType: "person",
			entityID:   personID,
			version:    1,
			mockEvents: []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonUpdated",
					Data:       updatedData,
					Version:    2,
					Position:   2,
					Timestamp:  now.Add(1 * time.Hour),
				},
			},
			wantState: map[string]any{
				"id":          personID.String(),
				"given_name":  "John",
				"surname":     "Smith",
				"gender":      "male",
				"birth_place": "New York",
			},
			wantIsDeleted: false,
		},
		{
			name:       "state after update",
			entityType: "person",
			entityID:   personID,
			version:    2,
			mockEvents: []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonUpdated",
					Data:       updatedData,
					Version:    2,
					Position:   2,
					Timestamp:  now.Add(1 * time.Hour),
				},
			},
			wantState: map[string]any{
				"id":          personID.String(),
				"given_name":  "Jonathan",
				"surname":     "Smith",
				"gender":      "male",
				"birth_place": "Boston",
			},
			wantIsDeleted: false,
		},
		{
			name:       "invalid version - zero",
			entityType: "person",
			entityID:   personID,
			version:    0,
			mockEvents: []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
			},
			wantError: ErrInvalidVersion,
		},
		{
			name:       "invalid version - exceeds current",
			entityType: "person",
			entityID:   personID,
			version:    10,
			mockEvents: []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
			},
			wantError: ErrInvalidVersion,
		},
		{
			name:       "no events",
			entityType: "person",
			entityID:   personID,
			version:    1,
			mockEvents: []repository.StoredEvent{},
			wantError:  ErrNoEvents,
		},
		{
			name:       "stream not found",
			entityType: "person",
			entityID:   personID,
			version:    1,
			mockError:  repository.ErrStreamNotFound,
			wantError:  ErrNoEvents,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventStore := &rollbackMockEventStore{
				readStreamFunc: func(ctx context.Context, streamID uuid.UUID) ([]repository.StoredEvent, error) {
					if tt.mockError != nil {
						return nil, tt.mockError
					}
					return tt.mockEvents, nil
				},
			}

			service := NewRollbackService(eventStore, &mockReadModelStore{})
			result, err := service.GetStateAtVersion(context.Background(), tt.entityType, tt.entityID, tt.version)

			if tt.wantError != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.entityType, result.EntityType)
			assert.Equal(t, tt.entityID, result.EntityID)
			assert.Equal(t, tt.version, result.Version)
			assert.Equal(t, tt.wantIsDeleted, result.IsDeleted)

			for key, wantValue := range tt.wantState {
				assert.Equal(t, wantValue, result.State[key], "field %s mismatch", key)
			}
		})
	}
}

func TestGetStateAtVersion_DeletedEntity(t *testing.T) {
	personID := uuid.New()
	now := time.Now().UTC()

	personCreated := domain.PersonCreated{
		BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: now},
		PersonID:  personID,
		GivenName: "John",
		Surname:   "Smith",
	}
	createdData, _ := json.Marshal(personCreated)

	personDeleted := domain.PersonDeleted{
		BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: now.Add(1 * time.Hour)},
		PersonID:  personID,
		Reason:    "duplicate",
	}
	deletedData, _ := json.Marshal(personDeleted)

	eventStore := &rollbackMockEventStore{
		readStreamFunc: func(ctx context.Context, streamID uuid.UUID) ([]repository.StoredEvent, error) {
			return []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonDeleted",
					Data:       deletedData,
					Version:    2,
					Position:   2,
					Timestamp:  now.Add(1 * time.Hour),
				},
			}, nil
		},
	}

	service := NewRollbackService(eventStore, &mockReadModelStore{})

	// State before deletion
	result, err := service.GetStateAtVersion(context.Background(), "person", personID, 1)
	require.NoError(t, err)
	assert.False(t, result.IsDeleted)
	assert.Equal(t, "John", result.State["given_name"])

	// State at deletion
	result, err = service.GetStateAtVersion(context.Background(), "person", personID, 2)
	require.NoError(t, err)
	assert.True(t, result.IsDeleted)
	// State is preserved for potential restoration
	assert.Equal(t, "John", result.State["given_name"])
}

func TestComputeRollbackChanges(t *testing.T) {
	personID := uuid.New()
	now := time.Now().UTC()

	personCreated := domain.PersonCreated{
		BaseEvent:  domain.BaseEvent{ID: uuid.New(), Timestamp: now},
		PersonID:   personID,
		GivenName:  "John",
		Surname:    "Smith",
		BirthPlace: "New York",
	}
	createdData, _ := json.Marshal(personCreated)

	personUpdated := domain.PersonUpdated{
		BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: now.Add(1 * time.Hour)},
		PersonID:  personID,
		Changes:   map[string]any{"given_name": "Jonathan", "notes": "Some notes"},
	}
	updatedData, _ := json.Marshal(personUpdated)

	tests := []struct {
		name             string
		entityType       string
		entityID         uuid.UUID
		targetVersion    int64
		mockEvents       []repository.StoredEvent
		mockError        error
		wantChanges      map[string]any
		wantIsRecreation bool
		wantError        error
	}{
		{
			name:          "rollback to previous version",
			entityType:    "person",
			entityID:      personID,
			targetVersion: 1,
			mockEvents: []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonUpdated",
					Data:       updatedData,
					Version:    2,
					Position:   2,
					Timestamp:  now.Add(1 * time.Hour),
				},
			},
			wantChanges: map[string]any{
				"given_name": "John",
				"notes":      nil, // Field didn't exist at version 1
			},
			wantIsRecreation: false,
		},
		{
			name:          "same version returns empty changes",
			entityType:    "person",
			entityID:      personID,
			targetVersion: 1,
			mockEvents: []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
			},
			wantChanges:      map[string]any{},
			wantIsRecreation: false,
		},
		{
			name:          "invalid version - zero",
			entityType:    "person",
			entityID:      personID,
			targetVersion: 0,
			mockEvents: []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
			},
			wantError: ErrInvalidVersion,
		},
		{
			name:          "invalid version - exceeds current",
			entityType:    "person",
			entityID:      personID,
			targetVersion: 10,
			mockEvents: []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
			},
			wantError: ErrInvalidVersion,
		},
		{
			name:          "no events",
			entityType:    "person",
			entityID:      personID,
			targetVersion: 1,
			mockEvents:    []repository.StoredEvent{},
			wantError:     ErrNoEvents,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventStore := &rollbackMockEventStore{
				readStreamFunc: func(ctx context.Context, streamID uuid.UUID) ([]repository.StoredEvent, error) {
					if tt.mockError != nil {
						return nil, tt.mockError
					}
					return tt.mockEvents, nil
				},
			}

			service := NewRollbackService(eventStore, &mockReadModelStore{})
			result, err := service.ComputeRollbackChanges(context.Background(), tt.entityType, tt.entityID, tt.targetVersion)

			if tt.wantError != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.entityType, result.EntityType)
			assert.Equal(t, tt.entityID, result.EntityID)
			assert.Equal(t, tt.targetVersion, result.TargetVersion)
			assert.Equal(t, tt.wantIsRecreation, result.IsRecreation)

			for key, wantValue := range tt.wantChanges {
				assert.Equal(t, wantValue, result.Changes[key], "field %s mismatch", key)
			}
		})
	}
}

func TestComputeRollbackChanges_Resurrection(t *testing.T) {
	personID := uuid.New()
	now := time.Now().UTC()

	personCreated := domain.PersonCreated{
		BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: now},
		PersonID:  personID,
		GivenName: "John",
		Surname:   "Smith",
	}
	createdData, _ := json.Marshal(personCreated)

	personDeleted := domain.PersonDeleted{
		BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: now.Add(1 * time.Hour)},
		PersonID:  personID,
		Reason:    "duplicate",
	}
	deletedData, _ := json.Marshal(personDeleted)

	eventStore := &rollbackMockEventStore{
		readStreamFunc: func(ctx context.Context, streamID uuid.UUID) ([]repository.StoredEvent, error) {
			return []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonDeleted",
					Data:       deletedData,
					Version:    2,
					Position:   2,
					Timestamp:  now.Add(1 * time.Hour),
				},
			}, nil
		},
	}

	service := NewRollbackService(eventStore, &mockReadModelStore{})
	result, err := service.ComputeRollbackChanges(context.Background(), "person", personID, 1)

	require.NoError(t, err)
	assert.True(t, result.IsRecreation, "should be marked as recreation when rolling back from deleted state")
	assert.Equal(t, int64(2), result.CurrentVersion)
	assert.Equal(t, int64(1), result.TargetVersion)
}

func TestApplyEventToState_FamilyEvents(t *testing.T) {
	familyID := uuid.New()
	partner1ID := uuid.New()
	partner2ID := uuid.New()
	childID := uuid.New()
	now := time.Now().UTC()

	familyCreated := domain.FamilyCreated{
		BaseEvent:        domain.BaseEvent{ID: uuid.New(), Timestamp: now},
		FamilyID:         familyID,
		Partner1ID:       &partner1ID,
		Partner2ID:       &partner2ID,
		RelationshipType: domain.RelationMarriage,
		MarriagePlace:    "New York",
	}
	createdData, _ := json.Marshal(familyCreated)

	childLinked := domain.ChildLinkedToFamily{
		BaseEvent:        domain.BaseEvent{ID: uuid.New(), Timestamp: now.Add(1 * time.Hour)},
		FamilyID:         familyID,
		PersonID:         childID,
		RelationshipType: domain.ChildBiological,
	}
	linkedData, _ := json.Marshal(childLinked)

	childUnlinked := domain.ChildUnlinkedFromFamily{
		BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: now.Add(2 * time.Hour)},
		FamilyID:  familyID,
		PersonID:  childID,
	}
	unlinkedData, _ := json.Marshal(childUnlinked)

	eventStore := &rollbackMockEventStore{
		readStreamFunc: func(ctx context.Context, streamID uuid.UUID) ([]repository.StoredEvent, error) {
			return []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   familyID,
					StreamType: "family",
					EventType:  "FamilyCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
				{
					ID:         uuid.New(),
					StreamID:   familyID,
					StreamType: "family",
					EventType:  "ChildLinkedToFamily",
					Data:       linkedData,
					Version:    2,
					Position:   2,
					Timestamp:  now.Add(1 * time.Hour),
				},
				{
					ID:         uuid.New(),
					StreamID:   familyID,
					StreamType: "family",
					EventType:  "ChildUnlinkedFromFamily",
					Data:       unlinkedData,
					Version:    3,
					Position:   3,
					Timestamp:  now.Add(2 * time.Hour),
				},
			}, nil
		},
	}

	service := NewRollbackService(eventStore, &mockReadModelStore{})

	// State at creation
	result, err := service.GetStateAtVersion(context.Background(), "family", familyID, 1)
	require.NoError(t, err)
	assert.Equal(t, partner1ID.String(), result.State["partner1_id"])
	assert.Equal(t, partner2ID.String(), result.State["partner2_id"])
	assert.Equal(t, "marriage", result.State["relationship_type"])

	// State after child linked
	result, err = service.GetStateAtVersion(context.Background(), "family", familyID, 2)
	require.NoError(t, err)
	children, ok := result.State["children"].([]string)
	require.True(t, ok)
	assert.Contains(t, children, childID.String())

	// State after child unlinked
	result, err = service.GetStateAtVersion(context.Background(), "family", familyID, 3)
	require.NoError(t, err)
	children, ok = result.State["children"].([]string)
	require.True(t, ok)
	assert.NotContains(t, children, childID.String())
}

func TestApplyEventToState_SourceEvents(t *testing.T) {
	sourceID := uuid.New()
	now := time.Now().UTC()

	sourceCreated := domain.SourceCreated{
		BaseEvent:  domain.BaseEvent{ID: uuid.New(), Timestamp: now},
		SourceID:   sourceID,
		SourceType: domain.SourceCensus,
		Title:      "1900 Census",
		Author:     "US Census Bureau",
	}
	createdData, _ := json.Marshal(sourceCreated)

	eventStore := &rollbackMockEventStore{
		readStreamFunc: func(ctx context.Context, streamID uuid.UUID) ([]repository.StoredEvent, error) {
			return []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   sourceID,
					StreamType: "source",
					EventType:  "SourceCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
			}, nil
		},
	}

	service := NewRollbackService(eventStore, &mockReadModelStore{})
	result, err := service.GetStateAtVersion(context.Background(), "source", sourceID, 1)

	require.NoError(t, err)
	assert.Equal(t, sourceID.String(), result.State["id"])
	assert.Equal(t, "1900 Census", result.State["title"])
	assert.Equal(t, "census", result.State["source_type"])
	assert.Equal(t, "US Census Bureau", result.State["author"])
}

func TestApplyEventToState_CitationEvents(t *testing.T) {
	citationID := uuid.New()
	sourceID := uuid.New()
	personID := uuid.New()
	now := time.Now().UTC()

	citationCreated := domain.CitationCreated{
		BaseEvent:     domain.BaseEvent{ID: uuid.New(), Timestamp: now},
		CitationID:    citationID,
		SourceID:      sourceID,
		FactType:      domain.FactPersonBirth,
		FactOwnerID:   personID,
		Page:          "123",
		SourceQuality: domain.SourceOriginal,
	}
	createdData, _ := json.Marshal(citationCreated)

	eventStore := &rollbackMockEventStore{
		readStreamFunc: func(ctx context.Context, streamID uuid.UUID) ([]repository.StoredEvent, error) {
			return []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   citationID,
					StreamType: "citation",
					EventType:  "CitationCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
			}, nil
		},
	}

	service := NewRollbackService(eventStore, &mockReadModelStore{})
	result, err := service.GetStateAtVersion(context.Background(), "citation", citationID, 1)

	require.NoError(t, err)
	assert.Equal(t, citationID.String(), result.State["id"])
	assert.Equal(t, sourceID.String(), result.State["source_id"])
	assert.Equal(t, "person_birth", result.State["fact_type"])
	assert.Equal(t, "123", result.State["page"])
	assert.Equal(t, "original", result.State["source_quality"])
}

func TestEventTypeToAction(t *testing.T) {
	service := &RollbackService{}

	tests := []struct {
		eventType  string
		wantAction string
	}{
		{"PersonCreated", "created"},
		{"PersonUpdated", "updated"},
		{"PersonDeleted", "deleted"},
		{"FamilyCreated", "created"},
		{"FamilyUpdated", "updated"},
		{"FamilyDeleted", "deleted"},
		{"ChildLinkedToFamily", "linked"},
		{"ChildUnlinkedFromFamily", "unlinked"},
		{"SourceCreated", "created"},
		{"SourceUpdated", "updated"},
		{"SourceDeleted", "deleted"},
		{"CitationCreated", "created"},
		{"CitationUpdated", "updated"},
		{"CitationDeleted", "deleted"},
		{"UnknownEvent", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			action := service.eventTypeToAction(tt.eventType)
			assert.Equal(t, tt.wantAction, action)
		})
	}
}

func TestIsDeletedEvent(t *testing.T) {
	service := &RollbackService{}

	tests := []struct {
		eventType   string
		wantDeleted bool
	}{
		{"PersonDeleted", true},
		{"FamilyDeleted", true},
		{"SourceDeleted", true},
		{"CitationDeleted", true},
		{"PersonCreated", false},
		{"PersonUpdated", false},
		{"ChildLinkedToFamily", false},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			isDeleted := service.isDeletedEvent(tt.eventType)
			assert.Equal(t, tt.wantDeleted, isDeleted)
		})
	}
}

func TestBuildChangeSummary(t *testing.T) {
	service := &RollbackService{}
	personID := uuid.New()
	sourceID := uuid.New()
	citationID := uuid.New()
	familyID := uuid.New()

	tests := []struct {
		name        string
		event       repository.StoredEvent
		wantSummary string
	}{
		{
			name: "person created",
			event: repository.StoredEvent{
				EventType: "PersonCreated",
				Data: mustMarshal(domain.PersonCreated{
					PersonID:  personID,
					GivenName: "John",
					Surname:   "Smith",
				}),
			},
			wantSummary: "created John Smith",
		},
		{
			name: "person updated with few fields",
			event: repository.StoredEvent{
				EventType: "PersonUpdated",
				Data: mustMarshal(domain.PersonUpdated{
					PersonID: personID,
					Changes:  map[string]any{"given_name": "Jonathan"},
				}),
			},
			wantSummary: "updated given_name",
		},
		{
			name: "person updated with many fields",
			event: repository.StoredEvent{
				EventType: "PersonUpdated",
				Data: mustMarshal(domain.PersonUpdated{
					PersonID: personID,
					Changes: map[string]any{
						"given_name":  "Jonathan",
						"surname":     "Smithson",
						"birth_place": "Boston",
						"notes":       "Some notes",
					},
				}),
			},
			wantSummary: "updated birth_place, given_name, notes and 1 more",
		},
		{
			name: "person deleted with reason",
			event: repository.StoredEvent{
				EventType: "PersonDeleted",
				Data: mustMarshal(domain.PersonDeleted{
					PersonID: personID,
					Reason:   "duplicate entry",
				}),
			},
			wantSummary: "deleted: duplicate entry",
		},
		{
			name: "person deleted without reason",
			event: repository.StoredEvent{
				EventType: "PersonDeleted",
				Data: mustMarshal(domain.PersonDeleted{
					PersonID: personID,
				}),
			},
			wantSummary: "deleted",
		},
		{
			name: "source created",
			event: repository.StoredEvent{
				EventType: "SourceCreated",
				Data: mustMarshal(domain.SourceCreated{
					SourceID: sourceID,
					Title:    "1900 United States Federal Census",
				}),
			},
			wantSummary: "created source: 1900 United States Federal Census",
		},
		{
			name: "source created with long title",
			event: repository.StoredEvent{
				EventType: "SourceCreated",
				Data: mustMarshal(domain.SourceCreated{
					SourceID: sourceID,
					Title:    "1900 United States Federal Census for the State of New York, County of Kings",
				}),
			},
			wantSummary: "created source: 1900 United States Federal Census for...",
		},
		{
			name: "citation created",
			event: repository.StoredEvent{
				EventType: "CitationCreated",
				Data: mustMarshal(domain.CitationCreated{
					CitationID: citationID,
					FactType:   domain.FactPersonBirth,
				}),
			},
			wantSummary: "created citation for person_birth",
		},
		{
			name: "family created",
			event: repository.StoredEvent{
				EventType: "FamilyCreated",
				Data: mustMarshal(domain.FamilyCreated{
					FamilyID: familyID,
				}),
			},
			wantSummary: "created family",
		},
		{
			name: "child linked",
			event: repository.StoredEvent{
				EventType: "ChildLinkedToFamily",
				Data: mustMarshal(domain.ChildLinkedToFamily{
					FamilyID: familyID,
					PersonID: personID,
				}),
			},
			wantSummary: "linked child " + personID.String()[:8],
		},
		{
			name: "child unlinked",
			event: repository.StoredEvent{
				EventType: "ChildUnlinkedFromFamily",
				Data: mustMarshal(domain.ChildUnlinkedFromFamily{
					FamilyID: familyID,
					PersonID: personID,
				}),
			},
			wantSummary: "unlinked child " + personID.String()[:8],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := service.buildChangeSummary(tt.event)
			assert.Equal(t, tt.wantSummary, summary)
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 8, "hello..."},
		{"abc", 3, "abc"},
		{"abcd", 3, "..."},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := truncate(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValuesEqual(t *testing.T) {
	tests := []struct {
		name string
		a    any
		b    any
		want bool
	}{
		{"both nil", nil, nil, true},
		{"first nil", nil, "value", false},
		{"second nil", "value", nil, false},
		{"equal strings", "hello", "hello", true},
		{"different strings", "hello", "world", false},
		{"equal numbers", 42, 42, true},
		{"different numbers", 42, 43, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := valuesEqual(tt.a, tt.b)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestNormalizeValue(t *testing.T) {
	id := uuid.New()

	tests := []struct {
		name  string
		input any
		want  any
	}{
		{"uuid", id, id.String()},
		{"uuid pointer", &id, id.String()},
		{"nil uuid pointer", (*uuid.UUID)(nil), nil},
		{"string", "hello", "hello"},
		{"int", 42, 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeValue(tt.input)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestComputeStateDiff(t *testing.T) {
	service := &RollbackService{}

	tests := []struct {
		name        string
		current     map[string]any
		target      map[string]any
		wantChanges map[string]any
	}{
		{
			name:        "no differences",
			current:     map[string]any{"a": "1", "b": "2"},
			target:      map[string]any{"a": "1", "b": "2"},
			wantChanges: map[string]any{},
		},
		{
			name:        "value changed",
			current:     map[string]any{"a": "1"},
			target:      map[string]any{"a": "2"},
			wantChanges: map[string]any{"a": "2"},
		},
		{
			name:        "field added in target",
			current:     map[string]any{"a": "1"},
			target:      map[string]any{"a": "1", "b": "2"},
			wantChanges: map[string]any{"b": "2"},
		},
		{
			name:        "field removed in target",
			current:     map[string]any{"a": "1", "b": "2"},
			target:      map[string]any{"a": "1"},
			wantChanges: map[string]any{"b": nil},
		},
		{
			name:        "multiple changes",
			current:     map[string]any{"a": "1", "b": "2", "c": "3"},
			target:      map[string]any{"a": "changed", "d": "new"},
			wantChanges: map[string]any{"a": "changed", "b": nil, "c": nil, "d": "new"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.computeStateDiff(tt.current, tt.target)
			assert.Equal(t, len(tt.wantChanges), len(result))
			for key, wantValue := range tt.wantChanges {
				assert.Equal(t, wantValue, result[key], "field %s mismatch", key)
			}
		})
	}
}

func TestGetRestorePoints_DeletedEntity(t *testing.T) {
	personID := uuid.New()
	now := time.Now().UTC()

	personCreated := domain.PersonCreated{
		BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: now},
		PersonID:  personID,
		GivenName: "John",
		Surname:   "Smith",
	}
	createdData, _ := json.Marshal(personCreated)

	personDeleted := domain.PersonDeleted{
		BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: now.Add(1 * time.Hour)},
		PersonID:  personID,
	}
	deletedData, _ := json.Marshal(personDeleted)

	eventStore := &rollbackMockEventStore{
		readStreamFunc: func(ctx context.Context, streamID uuid.UUID) ([]repository.StoredEvent, error) {
			return []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonDeleted",
					Data:       deletedData,
					Version:    2,
					Position:   2,
					Timestamp:  now.Add(1 * time.Hour),
				},
			}, nil
		},
	}

	service := NewRollbackService(eventStore, &mockReadModelStore{})
	result, err := service.GetRestorePoints(context.Background(), "person", personID, 20, 0)

	require.NoError(t, err)
	require.Equal(t, 2, len(result.RestorePoints))

	// When deleted, no version should be restorable (can't update a deleted entity)
	for _, point := range result.RestorePoints {
		assert.False(t, point.IsRestorable, "version %d should not be restorable on deleted entity", point.Version)
	}
}

func TestApplyEventToState_SourceDeletedAndUpdated(t *testing.T) {
	sourceID := uuid.New()
	now := time.Now().UTC()

	sourceCreated := domain.SourceCreated{
		BaseEvent:      domain.BaseEvent{ID: uuid.New(), Timestamp: now},
		SourceID:       sourceID,
		SourceType:     domain.SourceCensus,
		Title:          "1900 Census",
		Author:         "US Census Bureau",
		Publisher:      "Government",
		URL:            "http://example.com",
		RepositoryName: "NARA",
		CollectionName: "Census Records",
		CallNumber:     "T623",
		Notes:          "Important notes",
	}
	createdData, _ := json.Marshal(sourceCreated)

	sourceUpdated := domain.SourceUpdated{
		BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: now.Add(1 * time.Hour)},
		SourceID:  sourceID,
		Changes:   map[string]any{"title": "1900 Federal Census", "notes": nil},
	}
	updatedData, _ := json.Marshal(sourceUpdated)

	sourceDeleted := domain.SourceDeleted{
		BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: now.Add(2 * time.Hour)},
		SourceID:  sourceID,
	}
	deletedData, _ := json.Marshal(sourceDeleted)

	eventStore := &rollbackMockEventStore{
		readStreamFunc: func(ctx context.Context, streamID uuid.UUID) ([]repository.StoredEvent, error) {
			return []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   sourceID,
					StreamType: "source",
					EventType:  "SourceCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
				{
					ID:         uuid.New(),
					StreamID:   sourceID,
					StreamType: "source",
					EventType:  "SourceUpdated",
					Data:       updatedData,
					Version:    2,
					Position:   2,
					Timestamp:  now.Add(1 * time.Hour),
				},
				{
					ID:         uuid.New(),
					StreamID:   sourceID,
					StreamType: "source",
					EventType:  "SourceDeleted",
					Data:       deletedData,
					Version:    3,
					Position:   3,
					Timestamp:  now.Add(2 * time.Hour),
				},
			}, nil
		},
	}

	service := NewRollbackService(eventStore, &mockReadModelStore{})

	// State at creation
	result, err := service.GetStateAtVersion(context.Background(), "source", sourceID, 1)
	require.NoError(t, err)
	assert.False(t, result.IsDeleted)
	assert.Equal(t, "1900 Census", result.State["title"])
	assert.Equal(t, "US Census Bureau", result.State["author"])
	assert.Equal(t, "Government", result.State["publisher"])
	assert.Equal(t, "http://example.com", result.State["url"])
	assert.Equal(t, "NARA", result.State["repository_name"])
	assert.Equal(t, "Census Records", result.State["collection_name"])
	assert.Equal(t, "T623", result.State["call_number"])
	assert.Equal(t, "Important notes", result.State["notes"])

	// State after update (notes field removed)
	result, err = service.GetStateAtVersion(context.Background(), "source", sourceID, 2)
	require.NoError(t, err)
	assert.False(t, result.IsDeleted)
	assert.Equal(t, "1900 Federal Census", result.State["title"])
	_, hasNotes := result.State["notes"]
	assert.False(t, hasNotes, "notes should have been deleted")

	// State at deletion
	result, err = service.GetStateAtVersion(context.Background(), "source", sourceID, 3)
	require.NoError(t, err)
	assert.True(t, result.IsDeleted)
}

func TestApplyEventToState_CitationDeletedAndUpdated(t *testing.T) {
	citationID := uuid.New()
	sourceID := uuid.New()
	personID := uuid.New()
	now := time.Now().UTC()

	citationCreated := domain.CitationCreated{
		BaseEvent:     domain.BaseEvent{ID: uuid.New(), Timestamp: now},
		CitationID:    citationID,
		SourceID:      sourceID,
		FactType:      domain.FactPersonBirth,
		FactOwnerID:   personID,
		Page:          "123",
		Volume:        "Vol 1",
		SourceQuality: domain.SourceOriginal,
		InformantType: domain.InformantPrimary,
		EvidenceType:  domain.EvidenceDirect,
		QuotedText:    "Born on this date",
		Analysis:      "Reliable record",
		TemplateID:    "template-1",
	}
	createdData, _ := json.Marshal(citationCreated)

	citationUpdated := domain.CitationUpdated{
		BaseEvent:  domain.BaseEvent{ID: uuid.New(), Timestamp: now.Add(1 * time.Hour)},
		CitationID: citationID,
		Changes:    map[string]any{"page": "456", "analysis": "Updated analysis"},
	}
	updatedData, _ := json.Marshal(citationUpdated)

	citationDeleted := domain.CitationDeleted{
		BaseEvent:  domain.BaseEvent{ID: uuid.New(), Timestamp: now.Add(2 * time.Hour)},
		CitationID: citationID,
	}
	deletedData, _ := json.Marshal(citationDeleted)

	eventStore := &rollbackMockEventStore{
		readStreamFunc: func(ctx context.Context, streamID uuid.UUID) ([]repository.StoredEvent, error) {
			return []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   citationID,
					StreamType: "citation",
					EventType:  "CitationCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
				{
					ID:         uuid.New(),
					StreamID:   citationID,
					StreamType: "citation",
					EventType:  "CitationUpdated",
					Data:       updatedData,
					Version:    2,
					Position:   2,
					Timestamp:  now.Add(1 * time.Hour),
				},
				{
					ID:         uuid.New(),
					StreamID:   citationID,
					StreamType: "citation",
					EventType:  "CitationDeleted",
					Data:       deletedData,
					Version:    3,
					Position:   3,
					Timestamp:  now.Add(2 * time.Hour),
				},
			}, nil
		},
	}

	service := NewRollbackService(eventStore, &mockReadModelStore{})

	// State at creation
	result, err := service.GetStateAtVersion(context.Background(), "citation", citationID, 1)
	require.NoError(t, err)
	assert.False(t, result.IsDeleted)
	assert.Equal(t, "123", result.State["page"])
	assert.Equal(t, "Vol 1", result.State["volume"])
	assert.Equal(t, "original", result.State["source_quality"])
	assert.Equal(t, "primary", result.State["informant_type"])
	assert.Equal(t, "direct", result.State["evidence_type"])
	assert.Equal(t, "Born on this date", result.State["quoted_text"])
	assert.Equal(t, "Reliable record", result.State["analysis"])
	assert.Equal(t, "template-1", result.State["template_id"])

	// State after update
	result, err = service.GetStateAtVersion(context.Background(), "citation", citationID, 2)
	require.NoError(t, err)
	assert.False(t, result.IsDeleted)
	assert.Equal(t, "456", result.State["page"])
	assert.Equal(t, "Updated analysis", result.State["analysis"])

	// State at deletion
	result, err = service.GetStateAtVersion(context.Background(), "citation", citationID, 3)
	require.NoError(t, err)
	assert.True(t, result.IsDeleted)
}

func TestApplyEventToState_FamilyUpdatedAndDeleted(t *testing.T) {
	familyID := uuid.New()
	partner1ID := uuid.New()
	now := time.Now().UTC()

	familyCreated := domain.FamilyCreated{
		BaseEvent:        domain.BaseEvent{ID: uuid.New(), Timestamp: now},
		FamilyID:         familyID,
		Partner1ID:       &partner1ID,
		RelationshipType: domain.RelationMarriage,
		MarriagePlace:    "New York",
	}
	createdData, _ := json.Marshal(familyCreated)

	familyUpdated := domain.FamilyUpdated{
		BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: now.Add(1 * time.Hour)},
		FamilyID:  familyID,
		Changes:   map[string]any{"marriage_place": "Boston"},
	}
	updatedData, _ := json.Marshal(familyUpdated)

	familyDeleted := domain.FamilyDeleted{
		BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: now.Add(2 * time.Hour)},
		FamilyID:  familyID,
	}
	deletedData, _ := json.Marshal(familyDeleted)

	eventStore := &rollbackMockEventStore{
		readStreamFunc: func(ctx context.Context, streamID uuid.UUID) ([]repository.StoredEvent, error) {
			return []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   familyID,
					StreamType: "family",
					EventType:  "FamilyCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
				{
					ID:         uuid.New(),
					StreamID:   familyID,
					StreamType: "family",
					EventType:  "FamilyUpdated",
					Data:       updatedData,
					Version:    2,
					Position:   2,
					Timestamp:  now.Add(1 * time.Hour),
				},
				{
					ID:         uuid.New(),
					StreamID:   familyID,
					StreamType: "family",
					EventType:  "FamilyDeleted",
					Data:       deletedData,
					Version:    3,
					Position:   3,
					Timestamp:  now.Add(2 * time.Hour),
				},
			}, nil
		},
	}

	service := NewRollbackService(eventStore, &mockReadModelStore{})

	// State at creation
	result, err := service.GetStateAtVersion(context.Background(), "family", familyID, 1)
	require.NoError(t, err)
	assert.Equal(t, "New York", result.State["marriage_place"])

	// State after update
	result, err = service.GetStateAtVersion(context.Background(), "family", familyID, 2)
	require.NoError(t, err)
	assert.Equal(t, "Boston", result.State["marriage_place"])

	// State at deletion
	result, err = service.GetStateAtVersion(context.Background(), "family", familyID, 3)
	require.NoError(t, err)
	assert.True(t, result.IsDeleted)
}

func TestApplyEventToState_UnknownEventType(t *testing.T) {
	entityID := uuid.New()
	now := time.Now().UTC()

	eventStore := &rollbackMockEventStore{
		readStreamFunc: func(ctx context.Context, streamID uuid.UUID) ([]repository.StoredEvent, error) {
			return []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   entityID,
					StreamType: "unknown",
					EventType:  "UnknownEvent",
					Data:       []byte(`{}`),
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
			}, nil
		},
	}

	service := NewRollbackService(eventStore, &mockReadModelStore{})

	// Unknown event types should be handled gracefully (state remains empty, not deleted)
	_, err := service.GetStateAtVersion(context.Background(), "unknown", entityID, 1)
	require.Error(t, err) // DecodeEvent will fail for unknown event types
}

func TestBuildChangeSummary_AdditionalCases(t *testing.T) {
	service := &RollbackService{}
	familyID := uuid.New()
	sourceID := uuid.New()
	citationID := uuid.New()

	t.Run("family deleted", func(t *testing.T) {
		evt := repository.StoredEvent{
			EventType: "FamilyDeleted",
			Data: mustMarshal(domain.FamilyDeleted{
				FamilyID: familyID,
			}),
		}
		summary := service.buildChangeSummary(evt)
		assert.Equal(t, "deleted", summary)
	})

	t.Run("source updated", func(t *testing.T) {
		evt := repository.StoredEvent{
			EventType: "SourceUpdated",
			Data: mustMarshal(domain.SourceUpdated{
				SourceID: sourceID,
				Changes:  map[string]any{"title": "New Title"},
			}),
		}
		summary := service.buildChangeSummary(evt)
		assert.Equal(t, "updated title", summary)
	})

	t.Run("source deleted", func(t *testing.T) {
		evt := repository.StoredEvent{
			EventType: "SourceDeleted",
			Data: mustMarshal(domain.SourceDeleted{
				SourceID: sourceID,
			}),
		}
		summary := service.buildChangeSummary(evt)
		assert.Equal(t, "deleted", summary)
	})

	t.Run("citation updated", func(t *testing.T) {
		evt := repository.StoredEvent{
			EventType: "CitationUpdated",
			Data: mustMarshal(domain.CitationUpdated{
				CitationID: citationID,
				Changes:    map[string]any{"page": "200"},
			}),
		}
		summary := service.buildChangeSummary(evt)
		assert.Equal(t, "updated page", summary)
	})

	t.Run("citation deleted", func(t *testing.T) {
		evt := repository.StoredEvent{
			EventType: "CitationDeleted",
			Data: mustMarshal(domain.CitationDeleted{
				CitationID: citationID,
			}),
		}
		summary := service.buildChangeSummary(evt)
		assert.Equal(t, "deleted", summary)
	})

	t.Run("invalid event data", func(t *testing.T) {
		evt := repository.StoredEvent{
			EventType: "PersonCreated",
			Data:      []byte(`invalid json`),
		}
		summary := service.buildChangeSummary(evt)
		assert.Equal(t, "unknown change", summary)
	})

	t.Run("unknown event type", func(t *testing.T) {
		evt := repository.StoredEvent{
			EventType: "SomeNewEventType",
			Data:      []byte(`{}`),
		}
		summary := service.buildChangeSummary(evt)
		assert.Equal(t, "unknown change", summary)
	})
}

func TestNormalizeValue_GenDate(t *testing.T) {
	genDate := domain.ParseGenDate("1900-01-15")

	result := normalizeValue(genDate)
	assert.Equal(t, "1900-01-15", result)

	result = normalizeValue(&genDate)
	assert.Equal(t, "1900-01-15", result)

	result = normalizeValue((*domain.GenDate)(nil))
	assert.Nil(t, result)
}

func TestSummarizeChanges_EmptyChanges(t *testing.T) {
	service := &RollbackService{}

	result := service.summarizeChanges("updated", map[string]any{})
	assert.Equal(t, "updated", result)
}

func TestGetRestorePoints_LimitConstraints(t *testing.T) {
	personID := uuid.New()
	now := time.Now().UTC()

	personCreated := domain.PersonCreated{
		BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: now},
		PersonID:  personID,
		GivenName: "John",
		Surname:   "Smith",
	}
	createdData, _ := json.Marshal(personCreated)

	eventStore := &rollbackMockEventStore{
		readStreamFunc: func(ctx context.Context, streamID uuid.UUID) ([]repository.StoredEvent, error) {
			return []repository.StoredEvent{
				{
					ID:         uuid.New(),
					StreamID:   personID,
					StreamType: "person",
					EventType:  "PersonCreated",
					Data:       createdData,
					Version:    1,
					Position:   1,
					Timestamp:  now,
				},
			}, nil
		},
	}

	service := NewRollbackService(eventStore, &mockReadModelStore{})

	// Test limit over 100 gets capped
	result, err := service.GetRestorePoints(context.Background(), "person", personID, 200, 0)
	require.NoError(t, err)
	assert.Equal(t, 100, result.Limit)

	// Test negative offset defaults to 0
	result, err = service.GetRestorePoints(context.Background(), "person", personID, 20, -5)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Offset)
}
