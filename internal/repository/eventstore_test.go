package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestEventStore_Append(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	streamID := uuid.New()
	person := domain.NewPerson("John", "Doe")
	event := domain.NewPersonCreated(person)

	// Append first event with expectedVersion -1 (new stream)
	err := store.Append(ctx, streamID, "Person", []domain.Event{event}, -1)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Verify event count
	if store.EventCount() != 1 {
		t.Errorf("EventCount = %d, want 1", store.EventCount())
	}
}

func TestEventStore_ReadStream(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	streamID := uuid.New()
	person := domain.NewPerson("John", "Doe")
	event := domain.NewPersonCreated(person)

	// Append event
	err := store.Append(ctx, streamID, "Person", []domain.Event{event}, -1)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Read stream
	events, err := store.ReadStream(ctx, streamID)
	if err != nil {
		t.Fatalf("ReadStream failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].EventType != "PersonCreated" {
		t.Errorf("EventType = %s, want PersonCreated", events[0].EventType)
	}
	if events[0].Version != 1 {
		t.Errorf("Version = %d, want 1", events[0].Version)
	}
}

func TestEventStore_ConcurrencyConflict(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	streamID := uuid.New()
	person := domain.NewPerson("John", "Doe")
	event := domain.NewPersonCreated(person)

	// Append first event
	err := store.Append(ctx, streamID, "Person", []domain.Event{event}, -1)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Try to append with wrong expected version (should fail)
	event2 := domain.NewPersonUpdated(person.ID, map[string]any{"given_name": "Jane"})
	err = store.Append(ctx, streamID, "Person", []domain.Event{event2}, 0)
	if err != repository.ErrConcurrencyConflict {
		t.Errorf("Expected ErrConcurrencyConflict, got %v", err)
	}

	// Append with correct expected version (should succeed)
	err = store.Append(ctx, streamID, "Person", []domain.Event{event2}, 1)
	if err != nil {
		t.Fatalf("Append with correct version failed: %v", err)
	}
}

func TestEventStore_GetStreamVersion(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	streamID := uuid.New()

	// Non-existent stream should return 0
	version, err := store.GetStreamVersion(ctx, streamID)
	if err != nil {
		t.Fatalf("GetStreamVersion failed: %v", err)
	}
	if version != 0 {
		t.Errorf("Version = %d, want 0 for new stream", version)
	}

	// Append events
	person := domain.NewPerson("John", "Doe")
	err = store.Append(ctx, streamID, "Person", []domain.Event{domain.NewPersonCreated(person)}, -1)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	version, err = store.GetStreamVersion(ctx, streamID)
	if err != nil {
		t.Fatalf("GetStreamVersion failed: %v", err)
	}
	if version != 1 {
		t.Errorf("Version = %d, want 1", version)
	}
}

func TestEventStore_ReadAll(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	// Create multiple streams with events
	for i := 0; i < 3; i++ {
		streamID := uuid.New()
		person := domain.NewPerson("John", "Doe")
		event := domain.NewPersonCreated(person)
		err := store.Append(ctx, streamID, "Person", []domain.Event{event}, -1)
		if err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	// Read all from position 0
	events, err := store.ReadAll(ctx, 0, 10)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("Expected 3 events, got %d", len(events))
	}

	// Read with limit
	events, err = store.ReadAll(ctx, 0, 2)
	if err != nil {
		t.Fatalf("ReadAll with limit failed: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("Expected 2 events with limit, got %d", len(events))
	}

	// Read from position
	events, err = store.ReadAll(ctx, 1, 10)
	if err != nil {
		t.Fatalf("ReadAll from position failed: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("Expected 2 events from position 1, got %d", len(events))
	}
}

func TestStoredEvent_DecodeEvent(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	streamID := uuid.New()
	person := domain.NewPerson("John", "Doe")
	person.Gender = domain.GenderMale
	event := domain.NewPersonCreated(person)

	err := store.Append(ctx, streamID, "Person", []domain.Event{event}, -1)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	events, err := store.ReadStream(ctx, streamID)
	if err != nil {
		t.Fatalf("ReadStream failed: %v", err)
	}

	decoded, err := events[0].DecodeEvent()
	if err != nil {
		t.Fatalf("DecodeEvent failed: %v", err)
	}

	pc, ok := decoded.(domain.PersonCreated)
	if !ok {
		t.Fatalf("Expected PersonCreated, got %T", decoded)
	}

	if pc.GivenName != "John" {
		t.Errorf("GivenName = %s, want John", pc.GivenName)
	}
	if pc.Gender != domain.GenderMale {
		t.Errorf("Gender = %s, want male", pc.Gender)
	}
}
