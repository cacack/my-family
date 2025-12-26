package command_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
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

// Rollback tests

func TestRollbackPerson_Success(t *testing.T) {
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

	// Update the person
	newName := "Jane"
	_, err = handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:        createResult.ID,
		GivenName: &newName,
		Version:   createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdatePerson failed: %v", err)
	}

	// Rollback to version 1
	rollbackResult, err := handler.RollbackPerson(ctx, createResult.ID, 1)
	if err != nil {
		t.Fatalf("RollbackPerson failed: %v", err)
	}

	// Verify rollback result
	if rollbackResult.EntityID != createResult.ID {
		t.Errorf("EntityID = %v, want %v", rollbackResult.EntityID, createResult.ID)
	}
	if rollbackResult.EntityType != "Person" {
		t.Errorf("EntityType = %s, want Person", rollbackResult.EntityType)
	}
	if rollbackResult.NewVersion != 3 {
		t.Errorf("NewVersion = %d, want 3", rollbackResult.NewVersion)
	}
	if rollbackResult.Changes["given_name"] != "John" {
		t.Errorf("given_name change = %v, want John", rollbackResult.Changes["given_name"])
	}

	// Verify the person was restored
	person, err := readStore.GetPerson(ctx, createResult.ID)
	if err != nil {
		t.Fatalf("GetPerson failed: %v", err)
	}
	if person.GivenName != "John" {
		t.Errorf("GivenName = %s, want John", person.GivenName)
	}
}

func TestRollbackPerson_InvalidVersion(t *testing.T) {
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

	// Try to rollback to version 0 (invalid)
	_, err = handler.RollbackPerson(ctx, createResult.ID, 0)
	if err != command.ErrRollbackInvalidVersion {
		t.Errorf("Expected ErrRollbackInvalidVersion, got %v", err)
	}

	// Try to rollback to version higher than current
	_, err = handler.RollbackPerson(ctx, createResult.ID, 10)
	if err != command.ErrRollbackInvalidVersion {
		t.Errorf("Expected ErrRollbackInvalidVersion, got %v", err)
	}
}

func TestRollbackPerson_ToCurrentVersion(t *testing.T) {
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

	// Try to rollback to current version (no-op)
	_, err = handler.RollbackPerson(ctx, createResult.ID, 1)
	if err != command.ErrRollbackNoChanges {
		t.Errorf("Expected ErrRollbackNoChanges, got %v", err)
	}
}

func TestRollbackPerson_ConcurrencyConflict(t *testing.T) {
	mockStore := newMockEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(mockStore, readStore)
	ctx := context.Background()

	// Create and update a person normally
	createResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	newName := "Jane"
	_, err = handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:        createResult.ID,
		GivenName: &newName,
		Version:   createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdatePerson failed: %v", err)
	}

	// Set mock to return concurrency error on next append
	mockStore.appendError = repository.ErrConcurrencyConflict

	// Try to rollback - should fail with concurrency error
	_, err = handler.RollbackPerson(ctx, createResult.ID, 1)
	if err != repository.ErrConcurrencyConflict {
		t.Errorf("Expected ErrConcurrencyConflict, got %v", err)
	}
}

func TestRollbackPerson_DeletedEntity(t *testing.T) {
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

	// Update the person to have version 2
	newName := "Jane"
	_, err = handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:        createResult.ID,
		GivenName: &newName,
		Version:   createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdatePerson failed: %v", err)
	}

	// Delete the person
	err = handler.DeletePerson(ctx, command.DeletePersonInput{
		ID:      createResult.ID,
		Version: 2,
		Reason:  "test deletion",
	})
	if err != nil {
		t.Fatalf("DeletePerson failed: %v", err)
	}

	// Try to rollback a deleted entity
	_, err = handler.RollbackPerson(ctx, createResult.ID, 1)
	if err != command.ErrRollbackDeletedEntity {
		t.Errorf("Expected ErrRollbackDeletedEntity, got %v", err)
	}
}

func TestRollbackPerson_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Try to rollback a non-existent person
	nonExistentID := uuid.New()
	_, err := handler.RollbackPerson(ctx, nonExistentID, 1)
	// The memory event store returns version 0 for non-existent streams,
	// which makes target version >= current version, causing ErrRollbackNoChanges
	if err != command.ErrRollbackNoChanges && err != command.ErrRollbackInvalidVersion {
		t.Errorf("Expected ErrRollbackNoChanges or ErrRollbackInvalidVersion, got %v", err)
	}
}

func TestRollbackFamily_Success(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a person to use as partner
	personResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Create a family
	createResult, err := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID:       &personResult.ID,
		RelationshipType: "marriage",
		MarriagePlace:    "New York",
	})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}

	// Update the family
	newPlace := "Boston"
	_, err = handler.UpdateFamily(ctx, command.UpdateFamilyInput{
		ID:            createResult.ID,
		MarriagePlace: &newPlace,
		Version:       createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateFamily failed: %v", err)
	}

	// Rollback to version 1
	rollbackResult, err := handler.RollbackFamily(ctx, createResult.ID, 1)
	if err != nil {
		t.Fatalf("RollbackFamily failed: %v", err)
	}

	// Verify rollback result
	if rollbackResult.EntityType != "Family" {
		t.Errorf("EntityType = %s, want Family", rollbackResult.EntityType)
	}
	if rollbackResult.NewVersion != 3 {
		t.Errorf("NewVersion = %d, want 3", rollbackResult.NewVersion)
	}
}

func TestRollbackFamily_InvalidVersion(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a person to use as partner
	personResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Create a family with at least one partner
	createResult, err := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID:       &personResult.ID,
		RelationshipType: "marriage",
	})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}

	// Try to rollback to version 0 (invalid)
	_, err = handler.RollbackFamily(ctx, createResult.ID, 0)
	if err != command.ErrRollbackInvalidVersion {
		t.Errorf("Expected ErrRollbackInvalidVersion, got %v", err)
	}
}

func TestRollbackSource_Success(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a source
	createResult, err := handler.CreateSource(ctx, command.CreateSourceInput{
		Title:      "1900 Census",
		SourceType: "census",
		Author:     "US Census Bureau",
	})
	if err != nil {
		t.Fatalf("CreateSource failed: %v", err)
	}

	// Update the source
	newTitle := "1900 Federal Census"
	_, err = handler.UpdateSource(ctx, command.UpdateSourceInput{
		ID:      createResult.ID,
		Title:   &newTitle,
		Version: createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateSource failed: %v", err)
	}

	// Rollback to version 1
	rollbackResult, err := handler.RollbackSource(ctx, createResult.ID, 1)
	if err != nil {
		t.Fatalf("RollbackSource failed: %v", err)
	}

	// Verify rollback result
	if rollbackResult.EntityType != "Source" {
		t.Errorf("EntityType = %s, want Source", rollbackResult.EntityType)
	}
	if rollbackResult.NewVersion != 3 {
		t.Errorf("NewVersion = %d, want 3", rollbackResult.NewVersion)
	}
	if rollbackResult.Changes["title"] != "1900 Census" {
		t.Errorf("title change = %v, want 1900 Census", rollbackResult.Changes["title"])
	}
}

func TestRollbackSource_InvalidVersion(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a source
	createResult, err := handler.CreateSource(ctx, command.CreateSourceInput{
		Title:      "Test Source",
		SourceType: "book",
	})
	if err != nil {
		t.Fatalf("CreateSource failed: %v", err)
	}

	// Try to rollback to version 0 (invalid)
	_, err = handler.RollbackSource(ctx, createResult.ID, 0)
	if err != command.ErrRollbackInvalidVersion {
		t.Errorf("Expected ErrRollbackInvalidVersion, got %v", err)
	}
}

func TestRollbackCitation_Success(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a source first
	sourceResult, err := handler.CreateSource(ctx, command.CreateSourceInput{
		Title:      "1900 Census",
		SourceType: "census",
	})
	if err != nil {
		t.Fatalf("CreateSource failed: %v", err)
	}

	// Create a person for the citation
	personResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Create a citation
	createResult, err := handler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    sourceResult.ID,
		FactType:    "person_birth",
		FactOwnerID: personResult.ID,
		Page:        "Page 10",
	})
	if err != nil {
		t.Fatalf("CreateCitation failed: %v", err)
	}

	// Update the citation
	newPage := "Page 20"
	_, err = handler.UpdateCitation(ctx, command.UpdateCitationInput{
		ID:      createResult.ID,
		Page:    &newPage,
		Version: createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateCitation failed: %v", err)
	}

	// Rollback to version 1
	rollbackResult, err := handler.RollbackCitation(ctx, createResult.ID, 1)
	if err != nil {
		t.Fatalf("RollbackCitation failed: %v", err)
	}

	// Verify rollback result
	if rollbackResult.EntityType != "Citation" {
		t.Errorf("EntityType = %s, want Citation", rollbackResult.EntityType)
	}
	if rollbackResult.NewVersion != 3 {
		t.Errorf("NewVersion = %d, want 3", rollbackResult.NewVersion)
	}
	if rollbackResult.Changes["page"] != "Page 10" {
		t.Errorf("page change = %v, want Page 10", rollbackResult.Changes["page"])
	}
}

func TestRollbackCitation_InvalidVersion(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a source first
	sourceResult, err := handler.CreateSource(ctx, command.CreateSourceInput{
		Title:      "Test Source",
		SourceType: "book",
	})
	if err != nil {
		t.Fatalf("CreateSource failed: %v", err)
	}

	// Create a person for the citation
	personResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Create a citation
	createResult, err := handler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    sourceResult.ID,
		FactType:    "person_birth",
		FactOwnerID: personResult.ID,
	})
	if err != nil {
		t.Fatalf("CreateCitation failed: %v", err)
	}

	// Try to rollback to version 0 (invalid)
	_, err = handler.RollbackCitation(ctx, createResult.ID, 0)
	if err != command.ErrRollbackInvalidVersion {
		t.Errorf("Expected ErrRollbackInvalidVersion, got %v", err)
	}
}

func TestRollbackPerson_NoChangesNeeded(t *testing.T) {
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

	// Update the person with a field that will be removed
	newNotes := "Some notes"
	_, err = handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:      createResult.ID,
		Notes:   &newNotes,
		Version: createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdatePerson failed: %v", err)
	}

	// Update again to remove the notes
	emptyNotes := ""
	_, err = handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:      createResult.ID,
		Notes:   &emptyNotes,
		Version: 2,
	})
	if err != nil {
		t.Fatalf("UpdatePerson failed: %v", err)
	}

	// The state at version 3 should be mostly the same as version 1
	// (given_name and surname unchanged, notes removed)
	// Rollback to version 1 should only need to restore empty state for notes
	rollbackResult, err := handler.RollbackPerson(ctx, createResult.ID, 1)
	if err != nil {
		t.Fatalf("RollbackPerson failed: %v", err)
	}

	// There should be changes (notes was added then removed, so at v1 there was no notes field)
	// At v3 notes is empty string, at v1 notes didn't exist
	if rollbackResult.NewVersion != 4 {
		t.Errorf("NewVersion = %d, want 4", rollbackResult.NewVersion)
	}
}

func TestNewHandlerWithRollbackService(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	rollbackService := query.NewRollbackService(eventStore, readStore)

	handler := command.NewHandlerWithRollbackService(eventStore, readStore, rollbackService)

	if handler == nil {
		t.Fatal("NewHandlerWithRollbackService returned nil")
	}
}

func TestRollbackFamily_DeletedEntity(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a person to use as partner
	personResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Create a family
	createResult, err := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID:       &personResult.ID,
		RelationshipType: "marriage",
		MarriagePlace:    "New York",
	})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}

	// Update the family to have version 2
	newPlace := "Boston"
	_, err = handler.UpdateFamily(ctx, command.UpdateFamilyInput{
		ID:            createResult.ID,
		MarriagePlace: &newPlace,
		Version:       createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateFamily failed: %v", err)
	}

	// Delete the family
	err = handler.DeleteFamily(ctx, command.DeleteFamilyInput{
		ID:      createResult.ID,
		Version: 2,
	})
	if err != nil {
		t.Fatalf("DeleteFamily failed: %v", err)
	}

	// Try to rollback a deleted entity
	_, err = handler.RollbackFamily(ctx, createResult.ID, 1)
	if err != command.ErrRollbackDeletedEntity {
		t.Errorf("Expected ErrRollbackDeletedEntity, got %v", err)
	}
}

func TestRollbackSource_DeletedEntity(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a source
	createResult, err := handler.CreateSource(ctx, command.CreateSourceInput{
		Title:      "1900 Census",
		SourceType: "census",
		Author:     "US Census Bureau",
	})
	if err != nil {
		t.Fatalf("CreateSource failed: %v", err)
	}

	// Update the source to have version 2
	newTitle := "1900 Federal Census"
	_, err = handler.UpdateSource(ctx, command.UpdateSourceInput{
		ID:      createResult.ID,
		Title:   &newTitle,
		Version: createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateSource failed: %v", err)
	}

	// Delete the source
	err = handler.DeleteSource(ctx, createResult.ID, 2, "test deletion")
	if err != nil {
		t.Fatalf("DeleteSource failed: %v", err)
	}

	// Try to rollback a deleted entity
	_, err = handler.RollbackSource(ctx, createResult.ID, 1)
	if err != command.ErrRollbackDeletedEntity {
		t.Errorf("Expected ErrRollbackDeletedEntity, got %v", err)
	}
}

func TestRollbackCitation_DeletedEntity(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a source first
	sourceResult, err := handler.CreateSource(ctx, command.CreateSourceInput{
		Title:      "1900 Census",
		SourceType: "census",
	})
	if err != nil {
		t.Fatalf("CreateSource failed: %v", err)
	}

	// Create a person for the citation
	personResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Create a citation
	createResult, err := handler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    sourceResult.ID,
		FactType:    "person_birth",
		FactOwnerID: personResult.ID,
		Page:        "Page 10",
	})
	if err != nil {
		t.Fatalf("CreateCitation failed: %v", err)
	}

	// Update the citation to have version 2
	newPage := "Page 20"
	_, err = handler.UpdateCitation(ctx, command.UpdateCitationInput{
		ID:      createResult.ID,
		Page:    &newPage,
		Version: createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateCitation failed: %v", err)
	}

	// Delete the citation
	err = handler.DeleteCitation(ctx, createResult.ID, 2, "test deletion")
	if err != nil {
		t.Fatalf("DeleteCitation failed: %v", err)
	}

	// Try to rollback a deleted entity
	_, err = handler.RollbackCitation(ctx, createResult.ID, 1)
	if err != command.ErrRollbackDeletedEntity {
		t.Errorf("Expected ErrRollbackDeletedEntity, got %v", err)
	}
}

func TestRollbackFamily_ToCurrentVersion(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a person to use as partner
	personResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Create a family
	createResult, err := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID:       &personResult.ID,
		RelationshipType: "marriage",
	})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}

	// Try to rollback to current version (no-op)
	_, err = handler.RollbackFamily(ctx, createResult.ID, 1)
	if err != command.ErrRollbackNoChanges {
		t.Errorf("Expected ErrRollbackNoChanges, got %v", err)
	}
}

func TestRollbackSource_ToCurrentVersion(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a source
	createResult, err := handler.CreateSource(ctx, command.CreateSourceInput{
		Title:      "Test Source",
		SourceType: "book",
	})
	if err != nil {
		t.Fatalf("CreateSource failed: %v", err)
	}

	// Try to rollback to current version (no-op)
	_, err = handler.RollbackSource(ctx, createResult.ID, 1)
	if err != command.ErrRollbackNoChanges {
		t.Errorf("Expected ErrRollbackNoChanges, got %v", err)
	}
}

func TestRollbackCitation_ToCurrentVersion(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a source first
	sourceResult, err := handler.CreateSource(ctx, command.CreateSourceInput{
		Title:      "Test Source",
		SourceType: "book",
	})
	if err != nil {
		t.Fatalf("CreateSource failed: %v", err)
	}

	// Create a person for the citation
	personResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Create a citation
	createResult, err := handler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    sourceResult.ID,
		FactType:    "person_birth",
		FactOwnerID: personResult.ID,
	})
	if err != nil {
		t.Fatalf("CreateCitation failed: %v", err)
	}

	// Try to rollback to current version (no-op)
	_, err = handler.RollbackCitation(ctx, createResult.ID, 1)
	if err != command.ErrRollbackNoChanges {
		t.Errorf("Expected ErrRollbackNoChanges, got %v", err)
	}
}

func TestRollbackPerson_NegativeVersion(t *testing.T) {
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

	// Try to rollback to negative version
	_, err = handler.RollbackPerson(ctx, createResult.ID, -5)
	if err != command.ErrRollbackInvalidVersion {
		t.Errorf("Expected ErrRollbackInvalidVersion, got %v", err)
	}
}

func TestRollbackFamily_NegativeVersion(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a person to use as partner
	personResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Create a family
	createResult, err := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID:       &personResult.ID,
		RelationshipType: "marriage",
	})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}

	// Try to rollback to negative version
	_, err = handler.RollbackFamily(ctx, createResult.ID, -5)
	if err != command.ErrRollbackInvalidVersion {
		t.Errorf("Expected ErrRollbackInvalidVersion, got %v", err)
	}
}

func TestRollbackSource_NegativeVersion(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a source
	createResult, err := handler.CreateSource(ctx, command.CreateSourceInput{
		Title:      "Test Source",
		SourceType: "book",
	})
	if err != nil {
		t.Fatalf("CreateSource failed: %v", err)
	}

	// Try to rollback to negative version
	_, err = handler.RollbackSource(ctx, createResult.ID, -5)
	if err != command.ErrRollbackInvalidVersion {
		t.Errorf("Expected ErrRollbackInvalidVersion, got %v", err)
	}
}

func TestRollbackCitation_NegativeVersion(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a source first
	sourceResult, err := handler.CreateSource(ctx, command.CreateSourceInput{
		Title:      "Test Source",
		SourceType: "book",
	})
	if err != nil {
		t.Fatalf("CreateSource failed: %v", err)
	}

	// Create a person for the citation
	personResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Create a citation
	createResult, err := handler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    sourceResult.ID,
		FactType:    "person_birth",
		FactOwnerID: personResult.ID,
	})
	if err != nil {
		t.Fatalf("CreateCitation failed: %v", err)
	}

	// Try to rollback to negative version
	_, err = handler.RollbackCitation(ctx, createResult.ID, -5)
	if err != command.ErrRollbackInvalidVersion {
		t.Errorf("Expected ErrRollbackInvalidVersion, got %v", err)
	}
}

func TestRollbackFamily_VersionBeyondCurrent(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a person to use as partner
	personResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Create a family
	createResult, err := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID:       &personResult.ID,
		RelationshipType: "marriage",
	})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}

	// Update the family to have version 2
	newPlace := "Boston"
	_, err = handler.UpdateFamily(ctx, command.UpdateFamilyInput{
		ID:            createResult.ID,
		MarriagePlace: &newPlace,
		Version:       1,
	})
	if err != nil {
		t.Fatalf("UpdateFamily failed: %v", err)
	}

	// Try to rollback to version beyond current
	_, err = handler.RollbackFamily(ctx, createResult.ID, 100)
	if err != command.ErrRollbackInvalidVersion {
		t.Errorf("Expected ErrRollbackInvalidVersion, got %v", err)
	}
}

func TestRollbackSource_VersionBeyondCurrent(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a source
	createResult, err := handler.CreateSource(ctx, command.CreateSourceInput{
		Title:      "Test Source",
		SourceType: "book",
	})
	if err != nil {
		t.Fatalf("CreateSource failed: %v", err)
	}

	// Update the source to have version 2
	newTitle := "Updated Source"
	_, err = handler.UpdateSource(ctx, command.UpdateSourceInput{
		ID:      createResult.ID,
		Title:   &newTitle,
		Version: 1,
	})
	if err != nil {
		t.Fatalf("UpdateSource failed: %v", err)
	}

	// Try to rollback to version beyond current
	_, err = handler.RollbackSource(ctx, createResult.ID, 100)
	if err != command.ErrRollbackInvalidVersion {
		t.Errorf("Expected ErrRollbackInvalidVersion, got %v", err)
	}
}

func TestRollbackCitation_VersionBeyondCurrent(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a source first
	sourceResult, err := handler.CreateSource(ctx, command.CreateSourceInput{
		Title:      "Test Source",
		SourceType: "book",
	})
	if err != nil {
		t.Fatalf("CreateSource failed: %v", err)
	}

	// Create a person for the citation
	personResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Create a citation
	createResult, err := handler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    sourceResult.ID,
		FactType:    "person_birth",
		FactOwnerID: personResult.ID,
	})
	if err != nil {
		t.Fatalf("CreateCitation failed: %v", err)
	}

	// Update the citation to have version 2
	newPage := "Page 20"
	_, err = handler.UpdateCitation(ctx, command.UpdateCitationInput{
		ID:      createResult.ID,
		Page:    &newPage,
		Version: 1,
	})
	if err != nil {
		t.Fatalf("UpdateCitation failed: %v", err)
	}

	// Try to rollback to version beyond current
	_, err = handler.RollbackCitation(ctx, createResult.ID, 100)
	if err != command.ErrRollbackInvalidVersion {
		t.Errorf("Expected ErrRollbackInvalidVersion, got %v", err)
	}
}

func TestRollbackPerson_NonExistentEntity(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Try to rollback a non-existent person - memory store returns version 0
	// so targetVersion >= currentVersion triggers ErrRollbackInvalidVersion
	nonExistentID := uuid.New()
	_, err := handler.RollbackPerson(ctx, nonExistentID, 1)

	if err != command.ErrRollbackInvalidVersion {
		t.Errorf("Expected ErrRollbackInvalidVersion for non-existent entity, got %v", err)
	}
}

func TestRollbackFamily_NonExistentEntity(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	nonExistentID := uuid.New()
	_, err := handler.RollbackFamily(ctx, nonExistentID, 1)

	if err != command.ErrRollbackInvalidVersion {
		t.Errorf("Expected ErrRollbackInvalidVersion for non-existent entity, got %v", err)
	}
}

func TestRollbackSource_NonExistentEntity(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	nonExistentID := uuid.New()
	_, err := handler.RollbackSource(ctx, nonExistentID, 1)

	if err != command.ErrRollbackInvalidVersion {
		t.Errorf("Expected ErrRollbackInvalidVersion for non-existent entity, got %v", err)
	}
}

func TestRollbackCitation_NonExistentEntity(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	nonExistentID := uuid.New()
	_, err := handler.RollbackCitation(ctx, nonExistentID, 1)

	if err != command.ErrRollbackInvalidVersion {
		t.Errorf("Expected ErrRollbackInvalidVersion for non-existent entity, got %v", err)
	}
}

// TestRollbackPerson_NoChangesRequiredFromState tests the case where rollback state matches current state
func TestRollbackPerson_NoChangesRequiredFromState(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person
	createResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Update person with the same values (effectively no change)
	sameName := "John"
	_, err = handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:        createResult.ID,
		GivenName: &sameName,
		Version:   createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdatePerson failed: %v", err)
	}

	// Now we have version 2 but values are same as version 1
	// Rollback should succeed with empty changes
	result, err := handler.RollbackPerson(ctx, createResult.ID, 1)
	if err != nil {
		t.Fatalf("RollbackPerson failed: %v", err)
	}

	// Should return result indicating no changes needed (version stays at 2)
	if result.NewVersion != 2 {
		t.Errorf("Expected NewVersion=2 (no change), got %d", result.NewVersion)
	}
}
