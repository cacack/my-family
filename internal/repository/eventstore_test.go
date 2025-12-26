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

func TestStoredEvent_DecodeEvent_AllTypes(t *testing.T) {
	store := memory.NewEventStore()
	ctx := context.Background()

	tests := []struct {
		name      string
		event     domain.Event
		eventType string
		validate  func(t *testing.T, decoded domain.Event)
	}{
		{
			name:      "PersonCreated",
			event:     domain.NewPersonCreated(domain.NewPerson("John", "Doe")),
			eventType: "PersonCreated",
			validate: func(t *testing.T, decoded domain.Event) {
				e, ok := decoded.(domain.PersonCreated)
				if !ok {
					t.Fatalf("Expected PersonCreated, got %T", decoded)
				}
				if e.GivenName != "John" {
					t.Errorf("GivenName = %s, want John", e.GivenName)
				}
			},
		},
		{
			name:      "PersonUpdated",
			event:     domain.NewPersonUpdated(uuid.New(), map[string]any{"given_name": "Jane"}),
			eventType: "PersonUpdated",
			validate: func(t *testing.T, decoded domain.Event) {
				e, ok := decoded.(domain.PersonUpdated)
				if !ok {
					t.Fatalf("Expected PersonUpdated, got %T", decoded)
				}
				if e.Changes["given_name"] != "Jane" {
					t.Errorf("Changes[given_name] = %v, want Jane", e.Changes["given_name"])
				}
			},
		},
		{
			name:      "PersonDeleted",
			event:     domain.NewPersonDeleted(uuid.New(), "test reason"),
			eventType: "PersonDeleted",
			validate: func(t *testing.T, decoded domain.Event) {
				e, ok := decoded.(domain.PersonDeleted)
				if !ok {
					t.Fatalf("Expected PersonDeleted, got %T", decoded)
				}
				if e.Reason != "test reason" {
					t.Errorf("Reason = %s, want test reason", e.Reason)
				}
			},
		},
		{
			name:      "FamilyCreated",
			event:     domain.NewFamilyCreated(domain.NewFamily()),
			eventType: "FamilyCreated",
			validate: func(t *testing.T, decoded domain.Event) {
				_, ok := decoded.(domain.FamilyCreated)
				if !ok {
					t.Fatalf("Expected FamilyCreated, got %T", decoded)
				}
			},
		},
		{
			name:      "FamilyUpdated",
			event:     domain.NewFamilyUpdated(uuid.New(), map[string]any{"marriage_place": "Springfield"}),
			eventType: "FamilyUpdated",
			validate: func(t *testing.T, decoded domain.Event) {
				e, ok := decoded.(domain.FamilyUpdated)
				if !ok {
					t.Fatalf("Expected FamilyUpdated, got %T", decoded)
				}
				if e.Changes["marriage_place"] != "Springfield" {
					t.Errorf("Changes[marriage_place] = %v, want Springfield", e.Changes["marriage_place"])
				}
			},
		},
		{
			name:      "ChildLinkedToFamily",
			event:     domain.NewChildLinkedToFamily(domain.NewFamilyChild(uuid.New(), uuid.New(), domain.ChildBiological)),
			eventType: "ChildLinkedToFamily",
			validate: func(t *testing.T, decoded domain.Event) {
				e, ok := decoded.(domain.ChildLinkedToFamily)
				if !ok {
					t.Fatalf("Expected ChildLinkedToFamily, got %T", decoded)
				}
				if e.RelationshipType != domain.ChildBiological {
					t.Errorf("RelationshipType = %v, want biological", e.RelationshipType)
				}
			},
		},
		{
			name:      "ChildUnlinkedFromFamily",
			event:     domain.NewChildUnlinkedFromFamily(uuid.New(), uuid.New()),
			eventType: "ChildUnlinkedFromFamily",
			validate: func(t *testing.T, decoded domain.Event) {
				_, ok := decoded.(domain.ChildUnlinkedFromFamily)
				if !ok {
					t.Fatalf("Expected ChildUnlinkedFromFamily, got %T", decoded)
				}
			},
		},
		{
			name:      "FamilyDeleted",
			event:     domain.NewFamilyDeleted(uuid.New(), "test reason"),
			eventType: "FamilyDeleted",
			validate: func(t *testing.T, decoded domain.Event) {
				e, ok := decoded.(domain.FamilyDeleted)
				if !ok {
					t.Fatalf("Expected FamilyDeleted, got %T", decoded)
				}
				if e.Reason != "test reason" {
					t.Errorf("Reason = %s, want test reason", e.Reason)
				}
			},
		},
		{
			name:      "GedcomImported",
			event:     domain.NewGedcomImported("test.ged", 100, 10, 5, nil, nil),
			eventType: "GedcomImported",
			validate: func(t *testing.T, decoded domain.Event) {
				e, ok := decoded.(domain.GedcomImported)
				if !ok {
					t.Fatalf("Expected GedcomImported, got %T", decoded)
				}
				if e.Filename != "test.ged" {
					t.Errorf("Filename = %s, want test.ged", e.Filename)
				}
			},
		},
		{
			name:      "SourceCreated",
			event:     domain.NewSourceCreated(domain.NewSource("Test Source", domain.SourceBook)),
			eventType: "SourceCreated",
			validate: func(t *testing.T, decoded domain.Event) {
				e, ok := decoded.(domain.SourceCreated)
				if !ok {
					t.Fatalf("Expected SourceCreated, got %T", decoded)
				}
				if e.Title != "Test Source" {
					t.Errorf("Title = %s, want Test Source", e.Title)
				}
				if e.SourceType != domain.SourceBook {
					t.Errorf("SourceType = %v, want book", e.SourceType)
				}
			},
		},
		{
			name:      "SourceUpdated",
			event:     domain.NewSourceUpdated(uuid.New(), map[string]any{"title": "Updated Title", "author": "New Author"}),
			eventType: "SourceUpdated",
			validate: func(t *testing.T, decoded domain.Event) {
				e, ok := decoded.(domain.SourceUpdated)
				if !ok {
					t.Fatalf("Expected SourceUpdated, got %T", decoded)
				}
				if e.Changes["title"] != "Updated Title" {
					t.Errorf("Changes[title] = %v, want Updated Title", e.Changes["title"])
				}
				if e.Changes["author"] != "New Author" {
					t.Errorf("Changes[author] = %v, want New Author", e.Changes["author"])
				}
			},
		},
		{
			name:      "SourceDeleted",
			event:     domain.NewSourceDeleted(uuid.New(), "no longer needed"),
			eventType: "SourceDeleted",
			validate: func(t *testing.T, decoded domain.Event) {
				e, ok := decoded.(domain.SourceDeleted)
				if !ok {
					t.Fatalf("Expected SourceDeleted, got %T", decoded)
				}
				if e.Reason != "no longer needed" {
					t.Errorf("Reason = %s, want no longer needed", e.Reason)
				}
			},
		},
		{
			name:      "CitationCreated",
			event:     domain.NewCitationCreated(domain.NewCitation(uuid.New(), domain.FactPersonBirth, uuid.New())),
			eventType: "CitationCreated",
			validate: func(t *testing.T, decoded domain.Event) {
				e, ok := decoded.(domain.CitationCreated)
				if !ok {
					t.Fatalf("Expected CitationCreated, got %T", decoded)
				}
				if e.FactType != domain.FactPersonBirth {
					t.Errorf("FactType = %v, want person_birth", e.FactType)
				}
			},
		},
		{
			name:      "CitationUpdated",
			event:     domain.NewCitationUpdated(uuid.New(), map[string]any{"page": "123", "evidence_type": "direct"}),
			eventType: "CitationUpdated",
			validate: func(t *testing.T, decoded domain.Event) {
				e, ok := decoded.(domain.CitationUpdated)
				if !ok {
					t.Fatalf("Expected CitationUpdated, got %T", decoded)
				}
				if e.Changes["page"] != "123" {
					t.Errorf("Changes[page] = %v, want 123", e.Changes["page"])
				}
				if e.Changes["evidence_type"] != "direct" {
					t.Errorf("Changes[evidence_type] = %v, want direct", e.Changes["evidence_type"])
				}
			},
		},
		{
			name:      "CitationDeleted",
			event:     domain.NewCitationDeleted(uuid.New(), "duplicate"),
			eventType: "CitationDeleted",
			validate: func(t *testing.T, decoded domain.Event) {
				e, ok := decoded.(domain.CitationDeleted)
				if !ok {
					t.Fatalf("Expected CitationDeleted, got %T", decoded)
				}
				if e.Reason != "duplicate" {
					t.Errorf("Reason = %s, want duplicate", e.Reason)
				}
			},
		},
		{
			name: "MediaCreated",
			event: func() domain.Event {
				m := domain.NewMedia("Test Photo", "person", uuid.New())
				m.MimeType = "image/jpeg"
				return domain.NewMediaCreated(m)
			}(),
			eventType: "MediaCreated",
			validate: func(t *testing.T, decoded domain.Event) {
				e, ok := decoded.(domain.MediaCreated)
				if !ok {
					t.Fatalf("Expected MediaCreated, got %T", decoded)
				}
				if e.Title != "Test Photo" {
					t.Errorf("Title = %s, want Test Photo", e.Title)
				}
			},
		},
		{
			name:      "MediaUpdated",
			event:     domain.NewMediaUpdated(uuid.New(), map[string]any{"title": "New Title"}),
			eventType: "MediaUpdated",
			validate: func(t *testing.T, decoded domain.Event) {
				e, ok := decoded.(domain.MediaUpdated)
				if !ok {
					t.Fatalf("Expected MediaUpdated, got %T", decoded)
				}
				if e.Changes["title"] != "New Title" {
					t.Errorf("Changes[title] = %v, want New Title", e.Changes["title"])
				}
			},
		},
		{
			name:      "MediaDeleted",
			event:     domain.NewMediaDeleted(uuid.New(), "user request"),
			eventType: "MediaDeleted",
			validate: func(t *testing.T, decoded domain.Event) {
				e, ok := decoded.(domain.MediaDeleted)
				if !ok {
					t.Fatalf("Expected MediaDeleted, got %T", decoded)
				}
				if e.Reason != "user request" {
					t.Errorf("Reason = %s, want user request", e.Reason)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streamID := uuid.New()
			err := store.Append(ctx, streamID, "Test", []domain.Event{tt.event}, -1)
			if err != nil {
				t.Fatalf("Append failed: %v", err)
			}

			events, err := store.ReadStream(ctx, streamID)
			if err != nil {
				t.Fatalf("ReadStream failed: %v", err)
			}

			if len(events) != 1 {
				t.Fatalf("Expected 1 event, got %d", len(events))
			}

			if events[0].EventType != tt.eventType {
				t.Errorf("EventType = %s, want %s", events[0].EventType, tt.eventType)
			}

			decoded, err := events[0].DecodeEvent()
			if err != nil {
				t.Fatalf("DecodeEvent failed: %v", err)
			}

			tt.validate(t, decoded)
		})
	}
}

func TestStoredEvent_DecodeEvent_UnknownType(t *testing.T) {
	// Create a StoredEvent with unknown event type
	stored := repository.StoredEvent{
		ID:         uuid.New(),
		StreamID:   uuid.New(),
		StreamType: "Test",
		EventType:  "UnknownEventType",
		Data:       []byte(`{}`),
		Version:    1,
		Position:   1,
	}

	_, err := stored.DecodeEvent()
	if err == nil {
		t.Fatal("Expected error for unknown event type, got nil")
	}
	if err.Error() != "unknown event type: UnknownEventType" {
		t.Errorf("Error message = %s, want 'unknown event type: UnknownEventType'", err.Error())
	}
}

func TestStoredEvent_DecodeEvent_InvalidJSON(t *testing.T) {
	stored := repository.StoredEvent{
		ID:         uuid.New(),
		StreamID:   uuid.New(),
		StreamType: "Person",
		EventType:  "PersonCreated",
		Data:       []byte(`{invalid json`),
		Version:    1,
		Position:   1,
	}

	_, err := stored.DecodeEvent()
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestStoredEvent_DecodeEvent_InvalidJSON_AllTypes(t *testing.T) {
	invalidJSON := []byte(`{invalid json`)

	eventTypes := []string{
		"PersonCreated", "PersonUpdated", "PersonDeleted",
		"FamilyCreated", "FamilyUpdated", "FamilyDeleted",
		"ChildLinkedToFamily", "ChildUnlinkedFromFamily",
		"GedcomImported",
		"SourceCreated", "SourceUpdated", "SourceDeleted",
		"CitationCreated", "CitationUpdated", "CitationDeleted",
	}

	for _, eventType := range eventTypes {
		t.Run(eventType, func(t *testing.T) {
			stored := repository.StoredEvent{
				ID:         uuid.New(),
				StreamID:   uuid.New(),
				StreamType: "Test",
				EventType:  eventType,
				Data:       invalidJSON,
				Version:    1,
				Position:   1,
			}

			_, err := stored.DecodeEvent()
			if err == nil {
				t.Fatalf("Expected error for invalid JSON in %s, got nil", eventType)
			}
		})
	}
}

func TestEncodeEvent(t *testing.T) {
	streamID := uuid.New()
	person := domain.NewPerson("John", "Doe")
	person.Gender = domain.GenderMale
	event := domain.NewPersonCreated(person)

	stored, err := repository.EncodeEvent(streamID, "Person", event, 1, 1)
	if err != nil {
		t.Fatalf("EncodeEvent failed: %v", err)
	}

	if stored.StreamID != streamID {
		t.Errorf("StreamID = %v, want %v", stored.StreamID, streamID)
	}
	if stored.StreamType != "Person" {
		t.Errorf("StreamType = %s, want Person", stored.StreamType)
	}
	if stored.EventType != "PersonCreated" {
		t.Errorf("EventType = %s, want PersonCreated", stored.EventType)
	}
	if stored.Version != 1 {
		t.Errorf("Version = %d, want 1", stored.Version)
	}
	if stored.Position != 1 {
		t.Errorf("Position = %d, want 1", stored.Position)
	}
	if stored.Timestamp != event.OccurredAt() {
		t.Errorf("Timestamp = %v, want %v", stored.Timestamp, event.OccurredAt())
	}

	// Verify data can be decoded
	decoded, err := stored.DecodeEvent()
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
}

func TestErrorTypes(t *testing.T) {
	// Test that error types are properly defined
	if repository.ErrStreamNotFound == nil {
		t.Error("ErrStreamNotFound should not be nil")
	}
	if repository.ErrConcurrencyConflict == nil {
		t.Error("ErrConcurrencyConflict should not be nil")
	}
	if repository.ErrEventNotFound == nil {
		t.Error("ErrEventNotFound should not be nil")
	}

	// Test error messages
	if repository.ErrStreamNotFound.Error() != "stream not found" {
		t.Errorf("ErrStreamNotFound message = %s, want 'stream not found'", repository.ErrStreamNotFound.Error())
	}
	if repository.ErrConcurrencyConflict.Error() != "concurrency conflict: expected version mismatch" {
		t.Errorf("ErrConcurrencyConflict message = %s", repository.ErrConcurrencyConflict.Error())
	}
	if repository.ErrEventNotFound.Error() != "event not found" {
		t.Errorf("ErrEventNotFound message = %s, want 'event not found'", repository.ErrEventNotFound.Error())
	}
}
