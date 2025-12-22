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
