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

	// Missing surname
	_, err = handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "",
	})
	if err == nil {
		t.Error("Expected error for empty surname")
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
