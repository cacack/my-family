package sqlite_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

func setupTestDB(t *testing.T) (*sqlite.EventStore, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "myfamily-test-*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()

	db, err := sqlite.OpenDB(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("open database: %v", err)
	}

	store, err := sqlite.NewEventStore(db)
	if err != nil {
		db.Close()
		os.Remove(tmpFile.Name())
		t.Fatalf("create event store: %v", err)
	}

	return store, func() {
		store.Close()
		os.Remove(tmpFile.Name())
	}
}

func TestEventStore_AppendAndRead(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	streamID := uuid.New()

	// Append first event
	event1 := domain.PersonCreated{
		BaseEvent: domain.BaseEvent{
			ID:        uuid.New(),
			Timestamp: time.Now(),
		},
		PersonID:  streamID,
		GivenName: "John",
		Surname:   "Doe",
	}

	err := store.Append(ctx, streamID, "Person", []domain.Event{event1}, -1)
	if err != nil {
		t.Fatalf("append first event: %v", err)
	}

	// Read stream
	events, err := store.ReadStream(ctx, streamID)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].EventType != "PersonCreated" {
		t.Errorf("expected PersonCreated, got %s", events[0].EventType)
	}
	if events[0].Version != 1 {
		t.Errorf("expected version 1, got %d", events[0].Version)
	}

	// Append second event
	event2 := domain.PersonUpdated{
		BaseEvent: domain.BaseEvent{
			ID:        uuid.New(),
			Timestamp: time.Now(),
		},
		PersonID: streamID,
		Changes:  map[string]any{"given_name": "Jane"},
	}

	err = store.Append(ctx, streamID, "Person", []domain.Event{event2}, 1)
	if err != nil {
		t.Fatalf("append second event: %v", err)
	}

	// Read stream again
	events, err = store.ReadStream(ctx, streamID)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	if events[1].EventType != "PersonUpdated" {
		t.Errorf("expected PersonUpdated, got %s", events[1].EventType)
	}
	if events[1].Version != 2 {
		t.Errorf("expected version 2, got %d", events[1].Version)
	}
}

func TestEventStore_ConcurrencyConflict(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	streamID := uuid.New()

	// Append first event
	event1 := domain.PersonCreated{
		BaseEvent: domain.BaseEvent{
			ID:        uuid.New(),
			Timestamp: time.Now(),
		},
		PersonID:  streamID,
		GivenName: "John",
		Surname:   "Doe",
	}

	err := store.Append(ctx, streamID, "Person", []domain.Event{event1}, -1)
	if err != nil {
		t.Fatalf("append first event: %v", err)
	}

	// Try to append with wrong version
	event2 := domain.PersonUpdated{
		BaseEvent: domain.BaseEvent{
			ID:        uuid.New(),
			Timestamp: time.Now(),
		},
		PersonID: streamID,
		Changes:  map[string]any{"given_name": "Jane"},
	}

	err = store.Append(ctx, streamID, "Person", []domain.Event{event2}, 0)
	if err != repository.ErrConcurrencyConflict {
		t.Errorf("expected ErrConcurrencyConflict, got %v", err)
	}
}

func TestEventStore_ReadAll(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple streams with events
	for i := 0; i < 3; i++ {
		streamID := uuid.New()
		event := domain.PersonCreated{
			BaseEvent: domain.BaseEvent{
				ID:        uuid.New(),
				Timestamp: time.Now(),
			},
			PersonID:  streamID,
			GivenName: "Person",
			Surname:   "Test",
		}
		err := store.Append(ctx, streamID, "Person", []domain.Event{event}, -1)
		if err != nil {
			t.Fatalf("append event %d: %v", i, err)
		}
	}

	// Read all events
	events, err := store.ReadAll(ctx, 0, 10)
	if err != nil {
		t.Fatalf("read all: %v", err)
	}

	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}

	// Verify positions are sequential
	for i, e := range events {
		expectedPosition := int64(i + 1)
		if e.Position != expectedPosition {
			t.Errorf("event %d: expected position %d, got %d", i, expectedPosition, e.Position)
		}
	}
}

func TestEventStore_GetStreamVersion(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	streamID := uuid.New()

	// Non-existent stream should return 0
	version, err := store.GetStreamVersion(ctx, streamID)
	if err != nil {
		t.Fatalf("get version: %v", err)
	}
	if version != 0 {
		t.Errorf("expected version 0 for non-existent stream, got %d", version)
	}

	// Append events
	event := domain.PersonCreated{
		BaseEvent: domain.BaseEvent{
			ID:        uuid.New(),
			Timestamp: time.Now(),
		},
		PersonID:  streamID,
		GivenName: "John",
		Surname:   "Doe",
	}
	err = store.Append(ctx, streamID, "Person", []domain.Event{event}, -1)
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	// Version should now be 1
	version, err = store.GetStreamVersion(ctx, streamID)
	if err != nil {
		t.Fatalf("get version: %v", err)
	}
	if version != 1 {
		t.Errorf("expected version 1, got %d", version)
	}
}

func TestEventStore_DecodeEvents(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	streamID := uuid.New()

	// Create a person with all fields
	birthDate := domain.ParseGenDate("1 JAN 1850")
	event := domain.PersonCreated{
		BaseEvent: domain.BaseEvent{
			ID:        uuid.New(),
			Timestamp: time.Now(),
		},
		PersonID:   streamID,
		GivenName:  "John",
		Surname:    "Doe",
		Gender:     domain.GenderMale,
		BirthDate:  &birthDate,
		BirthPlace: "Springfield, IL, USA",
	}

	err := store.Append(ctx, streamID, "Person", []domain.Event{event}, -1)
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	// Read and decode
	events, err := store.ReadStream(ctx, streamID)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}

	decoded, err := events[0].DecodeEvent()
	if err != nil {
		t.Fatalf("decode event: %v", err)
	}

	personCreated, ok := decoded.(domain.PersonCreated)
	if !ok {
		t.Fatalf("expected PersonCreated, got %T", decoded)
	}

	if personCreated.GivenName != "John" {
		t.Errorf("expected GivenName John, got %s", personCreated.GivenName)
	}
	if personCreated.Surname != "Doe" {
		t.Errorf("expected Surname Doe, got %s", personCreated.Surname)
	}
	if personCreated.Gender != domain.GenderMale {
		t.Errorf("expected Gender male, got %s", personCreated.Gender)
	}
	if personCreated.BirthPlace != "Springfield, IL, USA" {
		t.Errorf("expected BirthPlace Springfield, IL, USA, got %s", personCreated.BirthPlace)
	}
}

func TestEventStore_MultipleEventsInBatch(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	streamID := uuid.New()

	// Append multiple events in one call
	events := []domain.Event{
		domain.PersonCreated{
			BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: time.Now()},
			PersonID:  streamID,
			GivenName: "John",
			Surname:   "Doe",
		},
		domain.PersonUpdated{
			BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: time.Now()},
			PersonID:  streamID,
			Changes:   map[string]any{"notes": "First update"},
		},
		domain.PersonUpdated{
			BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: time.Now()},
			PersonID:  streamID,
			Changes:   map[string]any{"notes": "Second update"},
		},
	}

	err := store.Append(ctx, streamID, "Person", events, -1)
	if err != nil {
		t.Fatalf("append batch: %v", err)
	}

	// Read and verify
	storedEvents, err := store.ReadStream(ctx, streamID)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}

	if len(storedEvents) != 3 {
		t.Fatalf("expected 3 events, got %d", len(storedEvents))
	}

	// Verify versions
	for i, e := range storedEvents {
		expectedVersion := int64(i + 1)
		if e.Version != expectedVersion {
			t.Errorf("event %d: expected version %d, got %d", i, expectedVersion, e.Version)
		}
	}
}

func TestEventStore_ReadStream_Empty(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	nonExistentStreamID := uuid.New()

	// Read non-existent stream
	events, err := store.ReadStream(ctx, nonExistentStreamID)
	if err != nil {
		t.Fatalf("read empty stream: %v", err)
	}

	if len(events) != 0 {
		t.Errorf("expected 0 events for non-existent stream, got %d", len(events))
	}
}

func TestEventStore_ReadAll_Pagination(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create 5 events across different streams
	for i := 0; i < 5; i++ {
		streamID := uuid.New()
		event := domain.PersonCreated{
			BaseEvent: domain.BaseEvent{
				ID:        uuid.New(),
				Timestamp: time.Now(),
			},
			PersonID:  streamID,
			GivenName: "Person",
			Surname:   "Test",
		}
		err := store.Append(ctx, streamID, "Person", []domain.Event{event}, -1)
		if err != nil {
			t.Fatalf("append event %d: %v", i, err)
		}
	}

	// Read first 2 events
	events, err := store.ReadAll(ctx, 0, 2)
	if err != nil {
		t.Fatalf("read all (first page): %v", err)
	}

	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}

	// Read next 2 events
	events, err = store.ReadAll(ctx, 2, 2)
	if err != nil {
		t.Fatalf("read all (second page): %v", err)
	}

	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}

	// Read beyond available events
	events, err = store.ReadAll(ctx, 10, 10)
	if err != nil {
		t.Fatalf("read all (beyond): %v", err)
	}

	if len(events) != 0 {
		t.Errorf("expected 0 events beyond range, got %d", len(events))
	}
}

func TestEventStore_ReadByStream(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	streamID := uuid.New()

	// Create multiple events for the stream
	baseTime := time.Now()
	events := []domain.Event{
		domain.PersonCreated{
			BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: baseTime},
			PersonID:  streamID,
			GivenName: "John",
			Surname:   "Doe",
		},
		domain.PersonUpdated{
			BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: baseTime.Add(time.Minute)},
			PersonID:  streamID,
			Changes:   map[string]any{"notes": "Update 1"},
		},
		domain.PersonUpdated{
			BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: baseTime.Add(2 * time.Minute)},
			PersonID:  streamID,
			Changes:   map[string]any{"notes": "Update 2"},
		},
	}

	err := store.Append(ctx, streamID, "Person", events, -1)
	if err != nil {
		t.Fatalf("append events: %v", err)
	}

	// Read first page
	page, err := store.ReadByStream(ctx, streamID, 2, 0)
	if err != nil {
		t.Fatalf("read by stream (first page): %v", err)
	}

	if len(page.Events) != 2 {
		t.Errorf("expected 2 events, got %d", len(page.Events))
	}
	if page.TotalCount != 3 {
		t.Errorf("expected total count 3, got %d", page.TotalCount)
	}
	if !page.HasMore {
		t.Error("expected HasMore to be true")
	}

	// Verify events are ordered by version ascending
	if page.Events[0].Version != 1 {
		t.Errorf("expected first event version 1, got %d", page.Events[0].Version)
	}
	if page.Events[1].Version != 2 {
		t.Errorf("expected second event version 2, got %d", page.Events[1].Version)
	}

	// Read second page
	page, err = store.ReadByStream(ctx, streamID, 2, 2)
	if err != nil {
		t.Fatalf("read by stream (second page): %v", err)
	}

	if len(page.Events) != 1 {
		t.Errorf("expected 1 event, got %d", len(page.Events))
	}
	if page.TotalCount != 3 {
		t.Errorf("expected total count 3, got %d", page.TotalCount)
	}
	if page.HasMore {
		t.Error("expected HasMore to be false")
	}
	if page.Events[0].Version != 3 {
		t.Errorf("expected event version 3, got %d", page.Events[0].Version)
	}
}

func TestEventStore_ReadByStream_Empty(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	nonExistentStreamID := uuid.New()

	// Read non-existent stream
	page, err := store.ReadByStream(ctx, nonExistentStreamID, 10, 0)
	if err != nil {
		t.Fatalf("read by stream (empty): %v", err)
	}

	if len(page.Events) != 0 {
		t.Errorf("expected 0 events, got %d", len(page.Events))
	}
	if page.TotalCount != 0 {
		t.Errorf("expected total count 0, got %d", page.TotalCount)
	}
	if page.HasMore {
		t.Error("expected HasMore to be false")
	}
}

func TestEventStore_ReadGlobalByTime(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create events at different times
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Person 1 events
	streamID1 := uuid.New()
	events1 := []domain.Event{
		domain.PersonCreated{
			BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: baseTime},
			PersonID:  streamID1,
			GivenName: "John",
			Surname:   "Doe",
		},
	}
	err := store.Append(ctx, streamID1, "Person", events1, -1)
	if err != nil {
		t.Fatalf("append person 1: %v", err)
	}

	// Family event
	streamID2 := uuid.New()
	events2 := []domain.Event{
		domain.FamilyCreated{
			BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: baseTime.Add(time.Hour)},
			FamilyID:  streamID2,
		},
	}
	err = store.Append(ctx, streamID2, "Family", events2, -1)
	if err != nil {
		t.Fatalf("append family: %v", err)
	}

	// Person 2 events
	streamID3 := uuid.New()
	events3 := []domain.Event{
		domain.PersonCreated{
			BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: baseTime.Add(2 * time.Hour)},
			PersonID:  streamID3,
			GivenName: "Jane",
			Surname:   "Smith",
		},
	}
	err = store.Append(ctx, streamID3, "Person", events3, -1)
	if err != nil {
		t.Fatalf("append person 2: %v", err)
	}

	// Test 1: Read all events (no filter)
	page, err := store.ReadGlobalByTime(ctx, time.Time{}, time.Time{}, nil, 10, 0)
	if err != nil {
		t.Fatalf("read global (all): %v", err)
	}
	if len(page.Events) != 3 {
		t.Errorf("expected 3 events, got %d", len(page.Events))
	}
	if page.TotalCount != 3 {
		t.Errorf("expected total count 3, got %d", page.TotalCount)
	}

	// Verify events are ordered by timestamp ascending
	if !page.Events[0].Timestamp.Equal(baseTime) {
		t.Errorf("expected first event at baseTime, got %v", page.Events[0].Timestamp)
	}
	if !page.Events[1].Timestamp.Equal(baseTime.Add(time.Hour)) {
		t.Errorf("expected second event at baseTime+1h, got %v", page.Events[1].Timestamp)
	}
	if !page.Events[2].Timestamp.Equal(baseTime.Add(2 * time.Hour)) {
		t.Errorf("expected third event at baseTime+2h, got %v", page.Events[2].Timestamp)
	}

	// Test 2: Filter by time range
	fromTime := baseTime.Add(30 * time.Minute)
	toTime := baseTime.Add(90 * time.Minute)
	page, err = store.ReadGlobalByTime(ctx, fromTime, toTime, nil, 10, 0)
	if err != nil {
		t.Fatalf("read global (time range): %v", err)
	}
	if len(page.Events) != 1 {
		t.Errorf("expected 1 event in time range, got %d", len(page.Events))
	}
	if page.Events[0].EventType != "FamilyCreated" {
		t.Errorf("expected FamilyCreated, got %s", page.Events[0].EventType)
	}

	// Test 3: Filter by event types
	page, err = store.ReadGlobalByTime(ctx, time.Time{}, time.Time{}, []string{"PersonCreated"}, 10, 0)
	if err != nil {
		t.Fatalf("read global (event types): %v", err)
	}
	if len(page.Events) != 2 {
		t.Errorf("expected 2 PersonCreated events, got %d", len(page.Events))
	}
	for _, e := range page.Events {
		if e.EventType != "PersonCreated" {
			t.Errorf("expected PersonCreated, got %s", e.EventType)
		}
	}

	// Test 4: Pagination
	page, err = store.ReadGlobalByTime(ctx, time.Time{}, time.Time{}, nil, 2, 0)
	if err != nil {
		t.Fatalf("read global (page 1): %v", err)
	}
	if len(page.Events) != 2 {
		t.Errorf("expected 2 events on page 1, got %d", len(page.Events))
	}
	if !page.HasMore {
		t.Error("expected HasMore to be true")
	}

	page, err = store.ReadGlobalByTime(ctx, time.Time{}, time.Time{}, nil, 2, 2)
	if err != nil {
		t.Fatalf("read global (page 2): %v", err)
	}
	if len(page.Events) != 1 {
		t.Errorf("expected 1 event on page 2, got %d", len(page.Events))
	}
	if page.HasMore {
		t.Error("expected HasMore to be false")
	}
}

func TestEventStore_ReadGlobalByTime_Empty(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Query with no events
	page, err := store.ReadGlobalByTime(ctx, time.Time{}, time.Time{}, nil, 10, 0)
	if err != nil {
		t.Fatalf("read global (empty): %v", err)
	}

	if len(page.Events) != 0 {
		t.Errorf("expected 0 events, got %d", len(page.Events))
	}
	if page.TotalCount != 0 {
		t.Errorf("expected total count 0, got %d", page.TotalCount)
	}
	if page.HasMore {
		t.Error("expected HasMore to be false")
	}
}

func TestEventStore_ReadGlobalByTime_MultipleEventTypes(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	baseTime := time.Now()

	// Create different event types
	streamID1 := uuid.New()
	err := store.Append(ctx, streamID1, "Person", []domain.Event{
		domain.PersonCreated{
			BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: baseTime},
			PersonID:  streamID1,
		},
	}, -1)
	if err != nil {
		t.Fatalf("append PersonCreated: %v", err)
	}

	streamID2 := uuid.New()
	err = store.Append(ctx, streamID2, "Family", []domain.Event{
		domain.FamilyCreated{
			BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: baseTime.Add(time.Minute)},
			FamilyID:  streamID2,
		},
	}, -1)
	if err != nil {
		t.Fatalf("append FamilyCreated: %v", err)
	}

	streamID3 := uuid.New()
	err = store.Append(ctx, streamID3, "Source", []domain.Event{
		domain.SourceCreated{
			BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: baseTime.Add(2 * time.Minute)},
			SourceID:  streamID3,
		},
	}, -1)
	if err != nil {
		t.Fatalf("append SourceCreated: %v", err)
	}

	// Filter by multiple event types
	page, err := store.ReadGlobalByTime(ctx, time.Time{}, time.Time{}, []string{"PersonCreated", "FamilyCreated"}, 10, 0)
	if err != nil {
		t.Fatalf("read global (multiple types): %v", err)
	}

	if len(page.Events) != 2 {
		t.Errorf("expected 2 events, got %d", len(page.Events))
	}
	if page.TotalCount != 2 {
		t.Errorf("expected total count 2, got %d", page.TotalCount)
	}

	// Verify only PersonCreated and FamilyCreated are returned
	for _, e := range page.Events {
		if e.EventType != "PersonCreated" && e.EventType != "FamilyCreated" {
			t.Errorf("unexpected event type: %s", e.EventType)
		}
	}
}
