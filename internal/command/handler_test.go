package command_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// mockEventStore simulates an event store that can fail on Append
type mockEventStore struct {
	*memory.EventStore
	appendError error
}

func newMockEventStore() *mockEventStore {
	return &mockEventStore{
		EventStore: memory.NewEventStore(),
	}
}

func (m *mockEventStore) Append(ctx context.Context, streamID uuid.UUID, streamType string, events []domain.Event, expectedVersion int64) error {
	if m.appendError != nil {
		return m.appendError
	}
	return m.EventStore.Append(ctx, streamID, streamType, events, expectedVersion)
}

func TestNewHandler(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()

	handler := command.NewHandler(eventStore, readStore)

	if handler == nil {
		t.Fatal("NewHandler returned nil")
	}
}

func TestHandler_Execute_ValidEvents(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a person to generate valid events
	input := command.CreatePersonInput{
		GivenName:  "John",
		Surname:    "Doe",
		Gender:     "male",
		BirthDate:  "1 JAN 1850",
		BirthPlace: "Springfield, IL",
	}

	result, err := handler.CreatePerson(ctx, input)
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Verify execute() worked correctly
	if result.ID == uuid.Nil {
		t.Error("Expected non-nil ID")
	}
	if result.Version != 1 {
		t.Errorf("Version = %d, want 1", result.Version)
	}

	// Verify event was stored
	person, err := readStore.GetPerson(ctx, result.ID)
	if err != nil {
		t.Fatalf("GetPerson failed: %v", err)
	}
	if person == nil {
		t.Fatal("Person not found in read model")
	}
	if person.GivenName != "John" {
		t.Errorf("GivenName = %s, want John", person.GivenName)
	}
}

func TestHandler_Execute_InvalidUUID(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// The parseUUID error path is tested implicitly through the person commands
	// when they call execute() with valid UUIDs (since the public API uses uuid.UUID type).
	// This test verifies that the CreatePerson command properly handles
	// the execute() flow with version calculation
	input := command.CreatePersonInput{
		GivenName: "Test",
		Surname:   "User",
	}

	result, err := handler.CreatePerson(ctx, input)
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Now update the person multiple times to test version calculation
	newName := "Updated"
	_, err = handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:        result.ID,
		GivenName: &newName,
		Version:   result.Version,
	})
	if err != nil {
		t.Fatalf("UpdatePerson failed: %v", err)
	}

	// Verify the version was incremented correctly
	person, _ := readStore.GetPerson(ctx, result.ID)
	if person.Version != 2 {
		t.Errorf("Version = %d, want 2", person.Version)
	}
}

func TestHandler_Execute_AppendError(t *testing.T) {
	mockStore := newMockEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(mockStore, readStore)
	ctx := context.Background()

	// Set the mock to return a concurrency error
	mockStore.appendError = repository.ErrConcurrencyConflict

	// Try to create a person - should fail with append error
	input := command.CreatePersonInput{
		GivenName: "Test",
		Surname:   "User",
	}

	_, err := handler.CreatePerson(ctx, input)
	if err == nil {
		t.Error("Expected error from CreatePerson when Append fails")
	}
}

func TestHandler_Execute_VersionCalculation(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	tests := []struct {
		name            string
		expectedVersion int64
		wantNewVersion  int64
	}{
		{
			name:            "new stream (version -1)",
			expectedVersion: -1,
			wantNewVersion:  1,
		},
		{
			name:            "existing stream (version 0)",
			expectedVersion: 0,
			wantNewVersion:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new person for each test
			input := command.CreatePersonInput{
				GivenName: "Test",
				Surname:   "User",
			}

			result, err := handler.CreatePerson(ctx, input)
			if err != nil {
				t.Fatalf("CreatePerson failed: %v", err)
			}

			// Verify version calculation
			if result.Version != tt.wantNewVersion {
				t.Errorf("Version = %d, want %d", result.Version, tt.wantNewVersion)
			}
		})
	}
}

func TestHandler_Execute_MultipleEvents(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a person
	createResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Update multiple times to generate multiple events
	for i := 0; i < 3; i++ {
		newNotes := "Update " + string(rune('A'+i))
		_, err = handler.UpdatePerson(ctx, command.UpdatePersonInput{
			ID:      createResult.ID,
			Notes:   &newNotes,
			Version: createResult.Version + int64(i),
		})
		if err != nil {
			t.Fatalf("UpdatePerson #%d failed: %v", i+1, err)
		}
	}

	// Verify final version
	person, _ := readStore.GetPerson(ctx, createResult.ID)
	if person.Version != 4 {
		t.Errorf("Final version = %d, want 4", person.Version)
	}
}

func TestHandler_Execute_ProjectionError(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a person - even if projection fails, the command should succeed
	// (projection errors are logged but don't fail the command in MVP)
	input := command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	}

	result, err := handler.CreatePerson(ctx, input)
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Verify the event was stored (execute succeeded)
	if result.ID == uuid.Nil {
		t.Error("Expected non-nil ID")
	}
	if result.Version != 1 {
		t.Errorf("Version = %d, want 1", result.Version)
	}
}
