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
