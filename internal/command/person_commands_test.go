package command_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestCreatePerson(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

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

	if result.ID == uuid.Nil {
		t.Error("Expected non-nil ID")
	}
	if result.Version != 1 {
		t.Errorf("Version = %d, want 1", result.Version)
	}

	// Verify person in read model
	person, _ := readStore.GetPerson(ctx, result.ID)
	if person == nil {
		t.Fatal("Person not found in read model")
	}
	if person.GivenName != "John" {
		t.Errorf("GivenName = %s, want John", person.GivenName)
	}
}

func TestCreatePerson_Validation(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Missing given name
	_, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "",
		Surname:   "Doe",
	})
	if err == nil {
		t.Error("Expected error for empty given name")
	}
}

func TestCreatePerson_EmptySurname(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	result, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName:      "Madonna",
		Surname:        "",
		Gender:         "female",
		ResearchStatus: "possible",
	})
	if err != nil {
		t.Fatalf("CreatePerson with empty surname failed: %v", err)
	}
	if result.ID == uuid.Nil {
		t.Error("Expected non-nil ID")
	}
	person, _ := readStore.GetPerson(ctx, result.ID)
	if person == nil {
		t.Fatal("Person not found in read model")
	}
	if person.GivenName != "Madonna" {
		t.Errorf("GivenName = %s, want Madonna", person.GivenName)
	}
	if person.Surname != "" {
		t.Errorf("Surname = %s, want empty", person.Surname)
	}
}

func TestUpdatePerson(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person first
	createResult, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	// Update person
	newName := "Jane"
	updateResult, err := handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:        createResult.ID,
		GivenName: &newName,
		Version:   createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdatePerson failed: %v", err)
	}

	if updateResult.Version != 2 {
		t.Errorf("Version = %d, want 2", updateResult.Version)
	}

	// Verify update in read model
	person, _ := readStore.GetPerson(ctx, createResult.ID)
	if person.GivenName != "Jane" {
		t.Errorf("GivenName = %s, want Jane", person.GivenName)
	}
}

func TestUpdatePerson_VersionConflict(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person
	createResult, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	// Try to update with wrong version
	newName := "Jane"
	_, err := handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:        createResult.ID,
		GivenName: &newName,
		Version:   999, // Wrong version
	})
	if err == nil {
		t.Error("Expected concurrency conflict error")
	}
}

func TestUpdatePerson_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	newName := "Jane"
	_, err := handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:        uuid.New(),
		GivenName: &newName,
		Version:   1,
	})
	if err != command.ErrPersonNotFound {
		t.Errorf("Expected ErrPersonNotFound, got %v", err)
	}
}

func TestDeletePerson(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person
	createResult, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	// Delete person
	err := handler.DeletePerson(ctx, command.DeletePersonInput{
		ID:      createResult.ID,
		Version: createResult.Version,
	})
	if err != nil {
		t.Fatalf("DeletePerson failed: %v", err)
	}

	// Verify deletion
	person, _ := readStore.GetPerson(ctx, createResult.ID)
	if person != nil {
		t.Error("Person should be deleted")
	}
}

func TestDeletePerson_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	err := handler.DeletePerson(ctx, command.DeletePersonInput{
		ID:      uuid.New(),
		Version: 1,
	})
	if err != command.ErrPersonNotFound {
		t.Errorf("Expected ErrPersonNotFound, got %v", err)
	}
}

func TestDeletePerson_WithFamiliesAsPartner(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create two persons
	p1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	p2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Jane",
		Surname:   "Doe",
	})

	// Create a family with p1 as partner
	_, _ = handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &p1.ID,
		Partner2ID: &p2.ID,
	})

	// Try to delete p1 (should fail because they're in a family)
	err := handler.DeletePerson(ctx, command.DeletePersonInput{
		ID:      p1.ID,
		Version: p1.Version,
	})
	if err != command.ErrPersonHasFamilies {
		t.Errorf("Expected ErrPersonHasFamilies, got %v", err)
	}
}

func TestDeletePerson_WithFamiliesAsChild(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create parent and child
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent",
		Surname:   "Doe",
	})
	child, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child",
		Surname:   "Doe",
	})

	// Create family and link child
	family, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent.ID,
	})
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: family.ID,
		ChildID:  child.ID,
	})

	// Try to delete child (should fail because they're in a family)
	err := handler.DeletePerson(ctx, command.DeletePersonInput{
		ID:      child.ID,
		Version: child.Version,
	})
	if err != command.ErrPersonHasFamilies {
		t.Errorf("Expected ErrPersonHasFamilies, got %v", err)
	}
}

func TestUpdatePerson_NoChanges(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person
	createResult, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	// Update with no changes
	updateResult, err := handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:      createResult.ID,
		Version: createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdatePerson failed: %v", err)
	}

	// Version should remain the same
	if updateResult.Version != createResult.Version {
		t.Errorf("Version = %d, want %d (no changes)", updateResult.Version, createResult.Version)
	}
}

func TestUpdatePerson_AllFields(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person
	createResult, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	// Update all fields
	newGiven := "Jane"
	newSurname := "Smith"
	newGender := "female"
	newBirthDate := "1 JAN 1900"
	newBirthPlace := "New York"
	newDeathDate := "31 DEC 1980"
	newDeathPlace := "Florida"
	newNotes := "Test notes"

	updateResult, err := handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:         createResult.ID,
		GivenName:  &newGiven,
		Surname:    &newSurname,
		Gender:     &newGender,
		BirthDate:  &newBirthDate,
		BirthPlace: &newBirthPlace,
		DeathDate:  &newDeathDate,
		DeathPlace: &newDeathPlace,
		Notes:      &newNotes,
		Version:    createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdatePerson failed: %v", err)
	}

	if updateResult.Version != 2 {
		t.Errorf("Version = %d, want 2", updateResult.Version)
	}

	// Verify all updates in read model
	person, _ := readStore.GetPerson(ctx, createResult.ID)
	if person.GivenName != "Jane" {
		t.Errorf("GivenName = %s, want Jane", person.GivenName)
	}
	if person.Surname != "Smith" {
		t.Errorf("Surname = %s, want Smith", person.Surname)
	}
	if person.Gender != "female" {
		t.Errorf("Gender = %s, want female", person.Gender)
	}
}

func TestCreatePerson_WithOptionalFields(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	input := command.CreatePersonInput{
		GivenName:  "John",
		Surname:    "Doe",
		Gender:     "male",
		BirthDate:  "ABT 1850",
		BirthPlace: "Springfield, IL",
		DeathDate:  "BEF 1920",
		DeathPlace: "Chicago, IL",
		Notes:      "Test person with all fields",
	}

	result, err := handler.CreatePerson(ctx, input)
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Verify all fields in read model
	person, _ := readStore.GetPerson(ctx, result.ID)
	if person.Notes != "Test person with all fields" {
		t.Errorf("Notes = %s, want 'Test person with all fields'", person.Notes)
	}
	if person.BirthPlace != "Springfield, IL" {
		t.Errorf("BirthPlace = %s, want Springfield, IL", person.BirthPlace)
	}
	if person.DeathPlace != "Chicago, IL" {
		t.Errorf("DeathPlace = %s, want Chicago, IL", person.DeathPlace)
	}
}

// ============================================================================
// Optimistic Locking / Version Conflict Tests for Person
// ============================================================================

func TestPersonUpdateStaleVersion(t *testing.T) {
	// Tests that updating with a stale version fails with version conflict error
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person (version 1)
	createResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	if createResult.Version != 1 {
		t.Fatalf("Expected initial version 1, got %d", createResult.Version)
	}

	// First update succeeds (version 1 -> 2)
	newName := "Jane"
	updateResult, err := handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:        createResult.ID,
		GivenName: &newName,
		Version:   1,
	})
	if err != nil {
		t.Fatalf("First update failed: %v", err)
	}
	if updateResult.Version != 2 {
		t.Fatalf("Expected version 2 after first update, got %d", updateResult.Version)
	}

	// Attempt update with stale version 1 (current is 2)
	staleName := "Stale"
	_, err = handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:        createResult.ID,
		GivenName: &staleName,
		Version:   1, // Stale version
	})
	if err == nil {
		t.Fatal("Expected version conflict error for stale version update")
	}

	// Verify the person still has the name from the successful update
	person, _ := readStore.GetPerson(ctx, createResult.ID)
	if person.GivenName != "Jane" {
		t.Errorf("Person name = %s, want Jane (stale update should not have applied)", person.GivenName)
	}
}

func TestPersonConcurrentModificationScenario(t *testing.T) {
	// Simulates two concurrent updates to the same entity with the same base version
	// The second update should fail because the first one incremented the version
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

	// Simulate two "concurrent" readers getting the same version
	baseVersion := createResult.Version // Both readers see version 1

	// First "concurrent" update succeeds
	name1 := "Alice"
	_, err = handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:        createResult.ID,
		GivenName: &name1,
		Version:   baseVersion,
	})
	if err != nil {
		t.Fatalf("First concurrent update failed: %v", err)
	}

	// Second "concurrent" update with same base version should fail
	name2 := "Bob"
	_, err = handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:        createResult.ID,
		GivenName: &name2,
		Version:   baseVersion, // Same stale version
	})
	if err == nil {
		t.Fatal("Expected version conflict error for second concurrent update")
	}

	// Verify only the first update was applied
	person, _ := readStore.GetPerson(ctx, createResult.ID)
	if person.GivenName != "Alice" {
		t.Errorf("Person name = %s, want Alice", person.GivenName)
	}
}

func TestPersonSequentialUpdatesSucceed(t *testing.T) {
	// Tests that sequential updates with correct version increments all succeed
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person (version 1)
	createResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Sequential updates with correct versions
	updates := []struct {
		name            string
		expectedVersion int64
	}{
		{"Jane", 2},
		{"Alice", 3},
		{"Bob", 4},
	}

	currentVersion := createResult.Version
	for _, update := range updates {
		newName := update.name
		result, err := handler.UpdatePerson(ctx, command.UpdatePersonInput{
			ID:        createResult.ID,
			GivenName: &newName,
			Version:   currentVersion,
		})
		if err != nil {
			t.Fatalf("Sequential update to %s failed: %v", update.name, err)
		}
		if result.Version != update.expectedVersion {
			t.Errorf("After update to %s: version = %d, want %d", update.name, result.Version, update.expectedVersion)
		}
		currentVersion = result.Version
	}

	// Verify final state
	person, _ := readStore.GetPerson(ctx, createResult.ID)
	if person.GivenName != "Bob" {
		t.Errorf("Final name = %s, want Bob", person.GivenName)
	}
	if person.Version != 4 {
		t.Errorf("Final version = %d, want 4", person.Version)
	}
}

func TestPersonDeleteVersionConflict(t *testing.T) {
	// Tests that deleting with a stale version fails
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

	// Update person (version 1 -> 2)
	newName := "Jane"
	_, err = handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:        createResult.ID,
		GivenName: &newName,
		Version:   1,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Attempt delete with stale version 1
	err = handler.DeletePerson(ctx, command.DeletePersonInput{
		ID:      createResult.ID,
		Version: 1, // Stale version
	})
	if err == nil {
		t.Fatal("Expected version conflict error for stale version delete")
	}

	// Verify person still exists
	person, _ := readStore.GetPerson(ctx, createResult.ID)
	if person == nil {
		t.Error("Person should still exist after failed delete")
	}
}

func TestPersonDeleteWithCorrectVersion(t *testing.T) {
	// Tests that delete succeeds with correct version after updates
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

	// Update person (version 1 -> 2)
	newName := "Jane"
	updateResult, err := handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:        createResult.ID,
		GivenName: &newName,
		Version:   1,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Delete with correct version
	err = handler.DeletePerson(ctx, command.DeletePersonInput{
		ID:      createResult.ID,
		Version: updateResult.Version,
	})
	if err != nil {
		t.Fatalf("Delete with correct version failed: %v", err)
	}

	// Verify person is deleted
	person, _ := readStore.GetPerson(ctx, createResult.ID)
	if person != nil {
		t.Error("Person should be deleted")
	}
}

func TestPersonVersionConflictNoPartialState(t *testing.T) {
	// Tests that version conflict leaves no partial state changes
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person with initial values
	createResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName:  "John",
		Surname:    "Doe",
		BirthPlace: "Boston",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Update to change values (version 1 -> 2)
	newPlace := "New York"
	_, err = handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:         createResult.ID,
		BirthPlace: &newPlace,
		Version:    1,
	})
	if err != nil {
		t.Fatalf("First update failed: %v", err)
	}

	// Attempt update with stale version that would change multiple fields
	staleName := "Stale"
	stalePlace := "Chicago"
	staleNotes := "Should not appear"
	_, err = handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:         createResult.ID,
		GivenName:  &staleName,
		BirthPlace: &stalePlace,
		Notes:      &staleNotes,
		Version:    1, // Stale version
	})
	if err == nil {
		t.Fatal("Expected version conflict error")
	}

	// Verify no partial changes were applied
	person, _ := readStore.GetPerson(ctx, createResult.ID)
	if person.GivenName != "John" {
		t.Errorf("GivenName = %s, want John (no partial change)", person.GivenName)
	}
	if person.BirthPlace != "New York" {
		t.Errorf("BirthPlace = %s, want New York (from successful update)", person.BirthPlace)
	}
	if person.Notes != "" {
		t.Errorf("Notes = %s, want empty (stale update should not apply)", person.Notes)
	}
}
