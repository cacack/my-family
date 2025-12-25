package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestNewEventStore(t *testing.T) {
	store := memory.NewEventStore()
	if store == nil {
		t.Fatal("NewEventStore() returned nil")
	}

	// Should start with zero events
	if count := store.EventCount(); count != 0 {
		t.Errorf("EventCount() = %d, want 0", count)
	}
}

func TestEventStore_AppendNewStream(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	streamID := uuid.New()
	person := domain.NewPerson("John", "Doe")
	event := domain.NewPersonCreated(person)

	// Append to new stream with expectedVersion -1
	err := store.Append(ctx, streamID, "Person", []domain.Event{event}, -1)
	if err != nil {
		t.Fatalf("Append() failed: %v", err)
	}

	// Verify event count
	if count := store.EventCount(); count != 1 {
		t.Errorf("EventCount() = %d, want 1", count)
	}

	// Verify stream version
	version, err := store.GetStreamVersion(ctx, streamID)
	if err != nil {
		t.Fatalf("GetStreamVersion() failed: %v", err)
	}
	if version != 1 {
		t.Errorf("GetStreamVersion() = %d, want 1", version)
	}
}

func TestEventStore_AppendExistingStream(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	streamID := uuid.New()
	person := domain.NewPerson("John", "Doe")

	// Append first event
	event1 := domain.NewPersonCreated(person)
	err := store.Append(ctx, streamID, "Person", []domain.Event{event1}, -1)
	if err != nil {
		t.Fatalf("Append() first event failed: %v", err)
	}

	// Append second event to existing stream
	event2 := domain.NewPersonUpdated(person.ID, map[string]any{"given_name": "Jane"})
	err = store.Append(ctx, streamID, "Person", []domain.Event{event2}, 1)
	if err != nil {
		t.Fatalf("Append() second event failed: %v", err)
	}

	// Verify event count
	if count := store.EventCount(); count != 2 {
		t.Errorf("EventCount() = %d, want 2", count)
	}

	// Verify stream version
	version, err := store.GetStreamVersion(ctx, streamID)
	if err != nil {
		t.Fatalf("GetStreamVersion() failed: %v", err)
	}
	if version != 2 {
		t.Errorf("GetStreamVersion() = %d, want 2", version)
	}
}

func TestEventStore_AppendConcurrencyConflict(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	streamID := uuid.New()
	person := domain.NewPerson("John", "Doe")

	// Append first event
	event1 := domain.NewPersonCreated(person)
	err := store.Append(ctx, streamID, "Person", []domain.Event{event1}, -1)
	if err != nil {
		t.Fatalf("Append() first event failed: %v", err)
	}

	// Try to append with wrong expected version (should fail)
	event2 := domain.NewPersonUpdated(person.ID, map[string]any{"given_name": "Jane"})
	err = store.Append(ctx, streamID, "Person", []domain.Event{event2}, 0)
	if err != repository.ErrConcurrencyConflict {
		t.Errorf("Append() with wrong version = %v, want ErrConcurrencyConflict", err)
	}

	// Verify stream version hasn't changed
	version, err := store.GetStreamVersion(ctx, streamID)
	if err != nil {
		t.Fatalf("GetStreamVersion() failed: %v", err)
	}
	if version != 1 {
		t.Errorf("GetStreamVersion() = %d, want 1 (unchanged)", version)
	}

	// Append with correct expected version (should succeed)
	err = store.Append(ctx, streamID, "Person", []domain.Event{event2}, 1)
	if err != nil {
		t.Fatalf("Append() with correct version failed: %v", err)
	}

	// Verify stream version updated
	version, err = store.GetStreamVersion(ctx, streamID)
	if err != nil {
		t.Fatalf("GetStreamVersion() failed: %v", err)
	}
	if version != 2 {
		t.Errorf("GetStreamVersion() = %d, want 2", version)
	}
}

func TestEventStore_AppendMultipleEventsInBatch(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	streamID := uuid.New()
	person := domain.NewPerson("John", "Doe")

	events := []domain.Event{
		domain.NewPersonCreated(person),
		domain.NewPersonUpdated(person.ID, map[string]any{"notes": "First update"}),
		domain.NewPersonUpdated(person.ID, map[string]any{"notes": "Second update"}),
	}

	// Append batch
	err := store.Append(ctx, streamID, "Person", events, -1)
	if err != nil {
		t.Fatalf("Append() batch failed: %v", err)
	}

	// Verify event count
	if count := store.EventCount(); count != 3 {
		t.Errorf("EventCount() = %d, want 3", count)
	}

	// Verify stream version
	version, err := store.GetStreamVersion(ctx, streamID)
	if err != nil {
		t.Fatalf("GetStreamVersion() failed: %v", err)
	}
	if version != 3 {
		t.Errorf("GetStreamVersion() = %d, want 3", version)
	}

	// Verify all events are stored with correct versions
	storedEvents, err := store.ReadStream(ctx, streamID)
	if err != nil {
		t.Fatalf("ReadStream() failed: %v", err)
	}
	if len(storedEvents) != 3 {
		t.Fatalf("len(storedEvents) = %d, want 3", len(storedEvents))
	}

	for i, event := range storedEvents {
		expectedVersion := int64(i + 1)
		if event.Version != expectedVersion {
			t.Errorf("storedEvents[%d].Version = %d, want %d", i, event.Version, expectedVersion)
		}
	}
}

func TestEventStore_ReadStream(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	streamID := uuid.New()
	person := domain.NewPerson("John", "Doe")
	person.Gender = domain.GenderMale

	event := domain.NewPersonCreated(person)
	err := store.Append(ctx, streamID, "Person", []domain.Event{event}, -1)
	if err != nil {
		t.Fatalf("Append() failed: %v", err)
	}

	// Read stream
	events, err := store.ReadStream(ctx, streamID)
	if err != nil {
		t.Fatalf("ReadStream() failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}

	// Verify stored event fields
	storedEvent := events[0]
	if storedEvent.StreamID != streamID {
		t.Errorf("StreamID = %v, want %v", storedEvent.StreamID, streamID)
	}
	if storedEvent.StreamType != "Person" {
		t.Errorf("StreamType = %s, want Person", storedEvent.StreamType)
	}
	if storedEvent.EventType != "PersonCreated" {
		t.Errorf("EventType = %s, want PersonCreated", storedEvent.EventType)
	}
	if storedEvent.Version != 1 {
		t.Errorf("Version = %d, want 1", storedEvent.Version)
	}
	if storedEvent.Position != 1 {
		t.Errorf("Position = %d, want 1", storedEvent.Position)
	}

	// Verify event can be decoded
	decoded, err := storedEvent.DecodeEvent()
	if err != nil {
		t.Fatalf("DecodeEvent() failed: %v", err)
	}

	personCreated, ok := decoded.(domain.PersonCreated)
	if !ok {
		t.Fatalf("decoded event type = %T, want domain.PersonCreated", decoded)
	}

	if personCreated.GivenName != "John" {
		t.Errorf("GivenName = %s, want John", personCreated.GivenName)
	}
	if personCreated.Surname != "Doe" {
		t.Errorf("Surname = %s, want Doe", personCreated.Surname)
	}
	if personCreated.Gender != domain.GenderMale {
		t.Errorf("Gender = %s, want male", personCreated.Gender)
	}
}

func TestEventStore_ReadStreamNonExistent(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	nonExistentStreamID := uuid.New()

	// Read non-existent stream should return empty slice
	events, err := store.ReadStream(ctx, nonExistentStreamID)
	if err != nil {
		t.Fatalf("ReadStream() failed: %v", err)
	}

	if events != nil && len(events) != 0 {
		t.Errorf("len(events) = %d, want 0 for non-existent stream", len(events))
	}
}

func TestEventStore_ReadAll(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	// Create multiple streams with events
	streamIDs := make([]uuid.UUID, 5)
	for i := 0; i < 5; i++ {
		streamID := uuid.New()
		streamIDs[i] = streamID
		person := domain.NewPerson("Person", "Test")
		event := domain.NewPersonCreated(person)
		err := store.Append(ctx, streamID, "Person", []domain.Event{event}, -1)
		if err != nil {
			t.Fatalf("Append() event %d failed: %v", i, err)
		}
	}

	tests := []struct {
		name         string
		fromPosition int64
		limit        int
		wantCount    int
	}{
		{
			name:         "read all from position 0",
			fromPosition: 0,
			limit:        10,
			wantCount:    5,
		},
		{
			name:         "read with limit",
			fromPosition: 0,
			limit:        3,
			wantCount:    3,
		},
		{
			name:         "read from position 2",
			fromPosition: 2,
			limit:        10,
			wantCount:    3,
		},
		{
			name:         "read from position beyond events",
			fromPosition: 10,
			limit:        10,
			wantCount:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := store.ReadAll(ctx, tt.fromPosition, tt.limit)
			if err != nil {
				t.Fatalf("ReadAll() failed: %v", err)
			}

			if len(events) != tt.wantCount {
				t.Errorf("len(events) = %d, want %d", len(events), tt.wantCount)
			}

			// Verify positions are sequential and greater than fromPosition
			for i, event := range events {
				if event.Position <= tt.fromPosition {
					t.Errorf("events[%d].Position = %d, want > %d", i, event.Position, tt.fromPosition)
				}
				if i > 0 && event.Position <= events[i-1].Position {
					t.Errorf("events[%d].Position = %d, want > events[%d].Position = %d",
						i, event.Position, i-1, events[i-1].Position)
				}
			}
		})
	}
}

func TestEventStore_GetStreamVersion(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	streamID := uuid.New()

	// Non-existent stream should return version 0
	version, err := store.GetStreamVersion(ctx, streamID)
	if err != nil {
		t.Fatalf("GetStreamVersion() failed: %v", err)
	}
	if version != 0 {
		t.Errorf("GetStreamVersion() for new stream = %d, want 0", version)
	}

	// Append events and verify version increments
	person := domain.NewPerson("John", "Doe")
	for i := 1; i <= 3; i++ {
		var event domain.Event
		if i == 1 {
			event = domain.NewPersonCreated(person)
		} else {
			event = domain.NewPersonUpdated(person.ID, map[string]any{"notes": "Update"})
		}

		err = store.Append(ctx, streamID, "Person", []domain.Event{event}, int64(i-1))
		if err != nil {
			t.Fatalf("Append() event %d failed: %v", i, err)
		}

		version, err = store.GetStreamVersion(ctx, streamID)
		if err != nil {
			t.Fatalf("GetStreamVersion() failed: %v", err)
		}
		if version != int64(i) {
			t.Errorf("GetStreamVersion() after %d events = %d, want %d", i, version, i)
		}
	}
}

func TestEventStore_Reset(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	// Add some events
	streamID := uuid.New()
	person := domain.NewPerson("John", "Doe")
	event := domain.NewPersonCreated(person)
	err := store.Append(ctx, streamID, "Person", []domain.Event{event}, -1)
	if err != nil {
		t.Fatalf("Append() failed: %v", err)
	}

	// Verify event exists
	if count := store.EventCount(); count != 1 {
		t.Errorf("EventCount() before reset = %d, want 1", count)
	}

	// Reset
	store.Reset()

	// Verify everything is cleared
	if count := store.EventCount(); count != 0 {
		t.Errorf("EventCount() after reset = %d, want 0", count)
	}

	version, err := store.GetStreamVersion(ctx, streamID)
	if err != nil {
		t.Fatalf("GetStreamVersion() after reset failed: %v", err)
	}
	if version != 0 {
		t.Errorf("GetStreamVersion() after reset = %d, want 0", version)
	}

	events, err := store.ReadStream(ctx, streamID)
	if err != nil {
		t.Fatalf("ReadStream() after reset failed: %v", err)
	}
	if events != nil && len(events) != 0 {
		t.Errorf("len(events) after reset = %d, want 0", len(events))
	}
}

func TestEventStore_EventCount(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	// Initial count should be 0
	if count := store.EventCount(); count != 0 {
		t.Errorf("EventCount() initially = %d, want 0", count)
	}

	// Add events across multiple streams
	for i := 0; i < 3; i++ {
		streamID := uuid.New()
		person := domain.NewPerson("Person", "Test")
		events := []domain.Event{
			domain.NewPersonCreated(person),
			domain.NewPersonUpdated(person.ID, map[string]any{"notes": "Update"}),
		}
		err := store.Append(ctx, streamID, "Person", events, -1)
		if err != nil {
			t.Fatalf("Append() failed: %v", err)
		}
	}

	// Count should reflect all events
	expectedCount := 3 * 2 // 3 streams, 2 events each
	if count := store.EventCount(); count != expectedCount {
		t.Errorf("EventCount() = %d, want %d", count, expectedCount)
	}
}

func TestEventStore_ConcurrentAccess(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	streamID := uuid.New()
	person := domain.NewPerson("John", "Doe")

	// Initialize stream
	event := domain.NewPersonCreated(person)
	err := store.Append(ctx, streamID, "Person", []domain.Event{event}, -1)
	if err != nil {
		t.Fatalf("Append() initial event failed: %v", err)
	}

	// Simulate concurrent reads
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := store.ReadStream(ctx, streamID)
			if err != nil {
				t.Errorf("ReadStream() concurrent read failed: %v", err)
			}
			done <- true
		}()
	}

	// Wait for all reads to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify stream is still intact
	events, err := store.ReadStream(ctx, streamID)
	if err != nil {
		t.Fatalf("ReadStream() after concurrent reads failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("len(events) after concurrent reads = %d, want 1", len(events))
	}
}

func TestEventStore_EventTimestamps(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	streamID := uuid.New()
	person := domain.NewPerson("John", "Doe")

	before := time.Now()
	event := domain.NewPersonCreated(person)
	err := store.Append(ctx, streamID, "Person", []domain.Event{event}, -1)
	if err != nil {
		t.Fatalf("Append() failed: %v", err)
	}
	after := time.Now()

	events, err := store.ReadStream(ctx, streamID)
	if err != nil {
		t.Fatalf("ReadStream() failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}

	timestamp := events[0].Timestamp
	if timestamp.Before(before) || timestamp.After(after) {
		t.Errorf("Timestamp = %v, want between %v and %v", timestamp, before, after)
	}
}

func TestEventStore_FamilyEvents(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	// Test with family events
	streamID := uuid.New()
	family := domain.NewFamily()

	event := domain.NewFamilyCreated(family)
	err := store.Append(ctx, streamID, "Family", []domain.Event{event}, -1)
	if err != nil {
		t.Fatalf("Append() family event failed: %v", err)
	}

	events, err := store.ReadStream(ctx, streamID)
	if err != nil {
		t.Fatalf("ReadStream() failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}

	if events[0].StreamType != "Family" {
		t.Errorf("StreamType = %s, want Family", events[0].StreamType)
	}
	if events[0].EventType != "FamilyCreated" {
		t.Errorf("EventType = %s, want FamilyCreated", events[0].EventType)
	}

	// Verify event can be decoded
	decoded, err := events[0].DecodeEvent()
	if err != nil {
		t.Fatalf("DecodeEvent() failed: %v", err)
	}

	_, ok := decoded.(domain.FamilyCreated)
	if !ok {
		t.Fatalf("decoded event type = %T, want domain.FamilyCreated", decoded)
	}
}

func TestEventStore_ReadByStream_EmptyStream(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	streamID := uuid.New()

	page, err := store.ReadByStream(ctx, streamID, 10, 0)
	if err != nil {
		t.Fatalf("ReadByStream() failed: %v", err)
	}

	if page.TotalCount != 0 {
		t.Errorf("TotalCount = %d, want 0", page.TotalCount)
	}
	if len(page.Events) != 0 {
		t.Errorf("len(Events) = %d, want 0", len(page.Events))
	}
	if page.HasMore {
		t.Errorf("HasMore = %v, want false", page.HasMore)
	}
}

func TestEventStore_ReadByStream_SinglePage(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	streamID := uuid.New()
	person := domain.NewPerson("John", "Doe")

	// Add 3 events
	events := []domain.Event{
		domain.NewPersonCreated(person),
		domain.NewPersonUpdated(person.ID, map[string]any{"notes": "Update 1"}),
		domain.NewPersonUpdated(person.ID, map[string]any{"notes": "Update 2"}),
	}
	err := store.Append(ctx, streamID, "Person", events, -1)
	if err != nil {
		t.Fatalf("Append() failed: %v", err)
	}

	page, err := store.ReadByStream(ctx, streamID, 10, 0)
	if err != nil {
		t.Fatalf("ReadByStream() failed: %v", err)
	}

	if page.TotalCount != 3 {
		t.Errorf("TotalCount = %d, want 3", page.TotalCount)
	}
	if len(page.Events) != 3 {
		t.Errorf("len(Events) = %d, want 3", len(page.Events))
	}
	if page.HasMore {
		t.Errorf("HasMore = %v, want false", page.HasMore)
	}

	// Verify events are in correct order
	for i, event := range page.Events {
		if event.Version != int64(i+1) {
			t.Errorf("Events[%d].Version = %d, want %d", i, event.Version, i+1)
		}
	}
}

func TestEventStore_ReadByStream_Pagination(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	streamID := uuid.New()
	person := domain.NewPerson("John", "Doe")

	// Add 5 events
	events := []domain.Event{
		domain.NewPersonCreated(person),
		domain.NewPersonUpdated(person.ID, map[string]any{"notes": "Update 1"}),
		domain.NewPersonUpdated(person.ID, map[string]any{"notes": "Update 2"}),
		domain.NewPersonUpdated(person.ID, map[string]any{"notes": "Update 3"}),
		domain.NewPersonUpdated(person.ID, map[string]any{"notes": "Update 4"}),
	}
	err := store.Append(ctx, streamID, "Person", events, -1)
	if err != nil {
		t.Fatalf("Append() failed: %v", err)
	}

	// First page
	page1, err := store.ReadByStream(ctx, streamID, 2, 0)
	if err != nil {
		t.Fatalf("ReadByStream() page 1 failed: %v", err)
	}
	if page1.TotalCount != 5 {
		t.Errorf("page1.TotalCount = %d, want 5", page1.TotalCount)
	}
	if len(page1.Events) != 2 {
		t.Errorf("len(page1.Events) = %d, want 2", len(page1.Events))
	}
	if !page1.HasMore {
		t.Errorf("page1.HasMore = %v, want true", page1.HasMore)
	}
	if page1.Events[0].Version != 1 {
		t.Errorf("page1.Events[0].Version = %d, want 1", page1.Events[0].Version)
	}

	// Second page
	page2, err := store.ReadByStream(ctx, streamID, 2, 2)
	if err != nil {
		t.Fatalf("ReadByStream() page 2 failed: %v", err)
	}
	if page2.TotalCount != 5 {
		t.Errorf("page2.TotalCount = %d, want 5", page2.TotalCount)
	}
	if len(page2.Events) != 2 {
		t.Errorf("len(page2.Events) = %d, want 2", len(page2.Events))
	}
	if !page2.HasMore {
		t.Errorf("page2.HasMore = %v, want true", page2.HasMore)
	}
	if page2.Events[0].Version != 3 {
		t.Errorf("page2.Events[0].Version = %d, want 3", page2.Events[0].Version)
	}

	// Third page (partial)
	page3, err := store.ReadByStream(ctx, streamID, 2, 4)
	if err != nil {
		t.Fatalf("ReadByStream() page 3 failed: %v", err)
	}
	if page3.TotalCount != 5 {
		t.Errorf("page3.TotalCount = %d, want 5", page3.TotalCount)
	}
	if len(page3.Events) != 1 {
		t.Errorf("len(page3.Events) = %d, want 1", len(page3.Events))
	}
	if page3.HasMore {
		t.Errorf("page3.HasMore = %v, want false", page3.HasMore)
	}
	if page3.Events[0].Version != 5 {
		t.Errorf("page3.Events[0].Version = %d, want 5", page3.Events[0].Version)
	}
}

func TestEventStore_ReadGlobalByTime_EmptyResults(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	fromTime := time.Now()
	toTime := fromTime.Add(1 * time.Hour)

	page, err := store.ReadGlobalByTime(ctx, fromTime, toTime, nil, 10, 0)
	if err != nil {
		t.Fatalf("ReadGlobalByTime() failed: %v", err)
	}

	if page.TotalCount != 0 {
		t.Errorf("TotalCount = %d, want 0", page.TotalCount)
	}
	if len(page.Events) != 0 {
		t.Errorf("len(Events) = %d, want 0", len(page.Events))
	}
	if page.HasMore {
		t.Errorf("HasMore = %v, want false", page.HasMore)
	}
}

func TestEventStore_ReadGlobalByTime_TimeFiltering(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	baseTime := time.Now()

	// Create events at different times
	for i := 0; i < 5; i++ {
		streamID := uuid.New()
		person := domain.NewPerson("Person", "Test")
		event := domain.NewPersonCreated(person)
		// Manually set timestamp for testing
		event.Timestamp = baseTime.Add(time.Duration(i) * time.Hour)
		err := store.Append(ctx, streamID, "Person", []domain.Event{event}, -1)
		if err != nil {
			t.Fatalf("Append() event %d failed: %v", i, err)
		}
	}

	// Query middle range (hours 1-3)
	fromTime := baseTime.Add(1 * time.Hour)
	toTime := baseTime.Add(3 * time.Hour)

	page, err := store.ReadGlobalByTime(ctx, fromTime, toTime, nil, 10, 0)
	if err != nil {
		t.Fatalf("ReadGlobalByTime() failed: %v", err)
	}

	if page.TotalCount != 3 {
		t.Errorf("TotalCount = %d, want 3", page.TotalCount)
	}
	if len(page.Events) != 3 {
		t.Errorf("len(Events) = %d, want 3", len(page.Events))
	}
	if page.HasMore {
		t.Errorf("HasMore = %v, want false", page.HasMore)
	}
}

func TestEventStore_ReadGlobalByTime_EventTypeFiltering(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	baseTime := time.Now()

	// Create mixed event types
	streamID1 := uuid.New()
	person := domain.NewPerson("Person", "Test")
	event1 := domain.NewPersonCreated(person)
	event1.Timestamp = baseTime
	err := store.Append(ctx, streamID1, "Person", []domain.Event{event1}, -1)
	if err != nil {
		t.Fatalf("Append() PersonCreated failed: %v", err)
	}

	streamID2 := uuid.New()
	family := domain.NewFamily()
	event2 := domain.NewFamilyCreated(family)
	event2.Timestamp = baseTime.Add(1 * time.Second)
	err = store.Append(ctx, streamID2, "Family", []domain.Event{event2}, -1)
	if err != nil {
		t.Fatalf("Append() FamilyCreated failed: %v", err)
	}

	streamID3 := uuid.New()
	person2 := domain.NewPerson("Person2", "Test")
	event3 := domain.NewPersonCreated(person2)
	event3.Timestamp = baseTime.Add(2 * time.Second)
	err = store.Append(ctx, streamID3, "Person", []domain.Event{event3}, -1)
	if err != nil {
		t.Fatalf("Append() PersonCreated2 failed: %v", err)
	}

	// Query only PersonCreated events
	fromTime := baseTime.Add(-1 * time.Second)
	toTime := baseTime.Add(3 * time.Second)

	page, err := store.ReadGlobalByTime(ctx, fromTime, toTime, []string{"PersonCreated"}, 10, 0)
	if err != nil {
		t.Fatalf("ReadGlobalByTime() failed: %v", err)
	}

	if page.TotalCount != 2 {
		t.Errorf("TotalCount = %d, want 2", page.TotalCount)
	}
	if len(page.Events) != 2 {
		t.Errorf("len(Events) = %d, want 2", len(page.Events))
	}
	for i, event := range page.Events {
		if event.EventType != "PersonCreated" {
			t.Errorf("Events[%d].EventType = %s, want PersonCreated", i, event.EventType)
		}
	}
}

func TestEventStore_ReadGlobalByTime_Pagination(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	baseTime := time.Now()

	// Create 5 events
	for i := 0; i < 5; i++ {
		streamID := uuid.New()
		person := domain.NewPerson("Person", "Test")
		event := domain.NewPersonCreated(person)
		event.Timestamp = baseTime.Add(time.Duration(i) * time.Second)
		err := store.Append(ctx, streamID, "Person", []domain.Event{event}, -1)
		if err != nil {
			t.Fatalf("Append() event %d failed: %v", i, err)
		}
	}

	fromTime := baseTime.Add(-1 * time.Second)
	toTime := baseTime.Add(10 * time.Second)

	// First page
	page1, err := store.ReadGlobalByTime(ctx, fromTime, toTime, nil, 2, 0)
	if err != nil {
		t.Fatalf("ReadGlobalByTime() page 1 failed: %v", err)
	}
	if page1.TotalCount != 5 {
		t.Errorf("page1.TotalCount = %d, want 5", page1.TotalCount)
	}
	if len(page1.Events) != 2 {
		t.Errorf("len(page1.Events) = %d, want 2", len(page1.Events))
	}
	if !page1.HasMore {
		t.Errorf("page1.HasMore = %v, want true", page1.HasMore)
	}

	// Second page
	page2, err := store.ReadGlobalByTime(ctx, fromTime, toTime, nil, 2, 2)
	if err != nil {
		t.Fatalf("ReadGlobalByTime() page 2 failed: %v", err)
	}
	if page2.TotalCount != 5 {
		t.Errorf("page2.TotalCount = %d, want 5", page2.TotalCount)
	}
	if len(page2.Events) != 2 {
		t.Errorf("len(page2.Events) = %d, want 2", len(page2.Events))
	}
	if !page2.HasMore {
		t.Errorf("page2.HasMore = %v, want true", page2.HasMore)
	}

	// Third page (partial)
	page3, err := store.ReadGlobalByTime(ctx, fromTime, toTime, nil, 2, 4)
	if err != nil {
		t.Fatalf("ReadGlobalByTime() page 3 failed: %v", err)
	}
	if page3.TotalCount != 5 {
		t.Errorf("page3.TotalCount = %d, want 5", page3.TotalCount)
	}
	if len(page3.Events) != 1 {
		t.Errorf("len(page3.Events) = %d, want 1", len(page3.Events))
	}
	if page3.HasMore {
		t.Errorf("page3.HasMore = %v, want false", page3.HasMore)
	}
}
