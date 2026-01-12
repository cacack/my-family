package command_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestAddName_Success(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// First create a person
	personInput := command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	}
	personResult, err := handler.CreatePerson(ctx, personInput)
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Add a name
	nameInput := command.AddNameInput{
		PersonID:  personResult.ID,
		GivenName: "Johann",
		Surname:   "Doe",
		NameType:  "immigrant",
		IsPrimary: false,
	}
	result, err := handler.AddName(ctx, nameInput)
	if err != nil {
		t.Fatalf("AddName failed: %v", err)
	}

	if result.PersonID != personResult.ID {
		t.Errorf("Expected PersonID %s, got %s", personResult.ID, result.PersonID)
	}
	if result.ID == uuid.Nil {
		t.Error("Expected non-nil ID")
	}
}

func TestAddName_FirstNameBecomesPrimary(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a person (no names yet in person_names table)
	personInput := command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	}
	personResult, err := handler.CreatePerson(ctx, personInput)
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Add first name - should become primary even if IsPrimary=false
	nameInput := command.AddNameInput{
		PersonID:  personResult.ID,
		GivenName: "John",
		Surname:   "Doe",
		NameType:  "birth",
		IsPrimary: false,
	}
	result, err := handler.AddName(ctx, nameInput)
	if err != nil {
		t.Fatalf("AddName failed: %v", err)
	}

	if !result.IsPrimary {
		t.Error("First name should become primary")
	}
}

func TestAddName_DemoteExistingPrimary(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person
	personInput := command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	}
	personResult, err := handler.CreatePerson(ctx, personInput)
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Add first name (becomes primary)
	_, err = handler.AddName(ctx, command.AddNameInput{
		PersonID:  personResult.ID,
		GivenName: "John",
		Surname:   "Doe",
		NameType:  "birth",
	})
	if err != nil {
		t.Fatalf("AddName failed: %v", err)
	}

	// Add second name as primary (should demote first)
	result, err := handler.AddName(ctx, command.AddNameInput{
		PersonID:  personResult.ID,
		GivenName: "Johann",
		Surname:   "Doe",
		NameType:  "immigrant",
		IsPrimary: true,
	})
	if err != nil {
		t.Fatalf("AddName failed: %v", err)
	}

	if !result.IsPrimary {
		t.Error("New name should be primary")
	}
}

func TestAddName_PersonNotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	nameInput := command.AddNameInput{
		PersonID:  uuid.New(), // Non-existent
		GivenName: "John",
		Surname:   "Doe",
		NameType:  "birth",
	}
	_, err := handler.AddName(ctx, nameInput)
	if err == nil {
		t.Fatal("Expected error for non-existent person")
	}
}

func TestAddName_InvalidInput(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person first
	personInput := command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	}
	personResult, err := handler.CreatePerson(ctx, personInput)
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Empty given name should fail
	nameInput := command.AddNameInput{
		PersonID:  personResult.ID,
		GivenName: "",
		Surname:   "Doe",
		NameType:  "birth",
	}
	_, err = handler.AddName(ctx, nameInput)
	if err == nil {
		t.Fatal("Expected error for empty given_name")
	}
}

func TestUpdateName_Success(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person and add a name
	personInput := command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	}
	personResult, err := handler.CreatePerson(ctx, personInput)
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	nameResult, err := handler.AddName(ctx, command.AddNameInput{
		PersonID:  personResult.ID,
		GivenName: "John",
		Surname:   "Doe",
		NameType:  "birth",
	})
	if err != nil {
		t.Fatalf("AddName failed: %v", err)
	}

	// Update the name
	newGiven := "Johnny"
	updateInput := command.UpdateNameInput{
		PersonID:  personResult.ID,
		NameID:    nameResult.ID,
		GivenName: &newGiven,
	}
	result, err := handler.UpdateName(ctx, updateInput)
	if err != nil {
		t.Fatalf("UpdateName failed: %v", err)
	}

	if result.ID != nameResult.ID {
		t.Errorf("Expected ID %s, got %s", nameResult.ID, result.ID)
	}
}

func TestUpdateName_SetAsPrimary(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person
	personInput := command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	}
	personResult, err := handler.CreatePerson(ctx, personInput)
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Add primary name
	_, err = handler.AddName(ctx, command.AddNameInput{
		PersonID:  personResult.ID,
		GivenName: "John",
		Surname:   "Doe",
		NameType:  "birth",
		IsPrimary: true,
	})
	if err != nil {
		t.Fatalf("AddName failed: %v", err)
	}

	// Add secondary name
	secondName, err := handler.AddName(ctx, command.AddNameInput{
		PersonID:  personResult.ID,
		GivenName: "Johann",
		Surname:   "Doe",
		NameType:  "immigrant",
		IsPrimary: false,
	})
	if err != nil {
		t.Fatalf("AddName failed: %v", err)
	}

	// Update second name to be primary
	isPrimary := true
	result, err := handler.UpdateName(ctx, command.UpdateNameInput{
		PersonID:  personResult.ID,
		NameID:    secondName.ID,
		IsPrimary: &isPrimary,
	})
	if err != nil {
		t.Fatalf("UpdateName failed: %v", err)
	}

	if !result.IsPrimary {
		t.Error("Updated name should be primary")
	}
}

func TestUpdateName_NameNotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person
	personInput := command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	}
	personResult, err := handler.CreatePerson(ctx, personInput)
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	newGiven := "Johnny"
	_, err = handler.UpdateName(ctx, command.UpdateNameInput{
		PersonID:  personResult.ID,
		NameID:    uuid.New(), // Non-existent
		GivenName: &newGiven,
	})
	if err == nil {
		t.Fatal("Expected error for non-existent name")
	}
}

func TestUpdateName_PersonNotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	newGiven := "Johnny"
	_, err := handler.UpdateName(ctx, command.UpdateNameInput{
		PersonID:  uuid.New(), // Non-existent
		NameID:    uuid.New(),
		GivenName: &newGiven,
	})
	if err == nil {
		t.Fatal("Expected error for non-existent person")
	}
}

func TestDeleteName_Success(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person
	personInput := command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	}
	personResult, err := handler.CreatePerson(ctx, personInput)
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Add primary name
	_, err = handler.AddName(ctx, command.AddNameInput{
		PersonID:  personResult.ID,
		GivenName: "John",
		Surname:   "Doe",
		NameType:  "birth",
		IsPrimary: true,
	})
	if err != nil {
		t.Fatalf("AddName failed: %v", err)
	}

	// Add secondary name
	secondName, err := handler.AddName(ctx, command.AddNameInput{
		PersonID:  personResult.ID,
		GivenName: "Johann",
		Surname:   "Doe",
		NameType:  "immigrant",
		IsPrimary: false,
	})
	if err != nil {
		t.Fatalf("AddName failed: %v", err)
	}

	// Delete secondary name
	err = handler.DeleteName(ctx, command.DeleteNameInput{
		PersonID: personResult.ID,
		NameID:   secondName.ID,
	})
	if err != nil {
		t.Fatalf("DeleteName failed: %v", err)
	}
}

func TestDeleteName_CannotDeleteLast(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person
	personInput := command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	}
	personResult, err := handler.CreatePerson(ctx, personInput)
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Add one name
	nameResult, err := handler.AddName(ctx, command.AddNameInput{
		PersonID:  personResult.ID,
		GivenName: "John",
		Surname:   "Doe",
		NameType:  "birth",
	})
	if err != nil {
		t.Fatalf("AddName failed: %v", err)
	}

	// Try to delete the only name
	err = handler.DeleteName(ctx, command.DeleteNameInput{
		PersonID: personResult.ID,
		NameID:   nameResult.ID,
	})
	if err == nil {
		t.Fatal("Expected error when deleting last name")
	}
}

func TestDeleteName_CannotDeletePrimary(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person
	personInput := command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	}
	personResult, err := handler.CreatePerson(ctx, personInput)
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Add primary name
	primaryName, err := handler.AddName(ctx, command.AddNameInput{
		PersonID:  personResult.ID,
		GivenName: "John",
		Surname:   "Doe",
		NameType:  "birth",
		IsPrimary: true,
	})
	if err != nil {
		t.Fatalf("AddName failed: %v", err)
	}

	// Add secondary name
	_, err = handler.AddName(ctx, command.AddNameInput{
		PersonID:  personResult.ID,
		GivenName: "Johann",
		Surname:   "Doe",
		NameType:  "immigrant",
		IsPrimary: false,
	})
	if err != nil {
		t.Fatalf("AddName failed: %v", err)
	}

	// Try to delete primary name
	err = handler.DeleteName(ctx, command.DeleteNameInput{
		PersonID: personResult.ID,
		NameID:   primaryName.ID,
	})
	if err == nil {
		t.Fatal("Expected error when deleting primary name")
	}
}

func TestDeleteName_PersonNotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	err := handler.DeleteName(ctx, command.DeleteNameInput{
		PersonID: uuid.New(), // Non-existent
		NameID:   uuid.New(),
	})
	if err == nil {
		t.Fatal("Expected error for non-existent person")
	}
}

func TestDeleteName_NameNotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person
	personInput := command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	}
	personResult, err := handler.CreatePerson(ctx, personInput)
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Add a name first so there's at least one
	_, err = handler.AddName(ctx, command.AddNameInput{
		PersonID:  personResult.ID,
		GivenName: "John",
		Surname:   "Doe",
		NameType:  "birth",
	})
	if err != nil {
		t.Fatalf("AddName failed: %v", err)
	}

	// Try to delete non-existent name
	err = handler.DeleteName(ctx, command.DeleteNameInput{
		PersonID: personResult.ID,
		NameID:   uuid.New(), // Non-existent
	})
	if err == nil {
		t.Fatal("Expected error for non-existent name")
	}
}

func TestAddName_WithAllFields(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person
	personInput := command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	}
	personResult, err := handler.CreatePerson(ctx, personInput)
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Add name with all fields
	nameInput := command.AddNameInput{
		PersonID:      personResult.ID,
		GivenName:     "Johannes",
		Surname:       "von Doe",
		NamePrefix:    "Dr.",
		NameSuffix:    "Jr.",
		SurnamePrefix: "von",
		Nickname:      "Johnny",
		NameType:      "birth",
		IsPrimary:     true,
	}
	result, err := handler.AddName(ctx, nameInput)
	if err != nil {
		t.Fatalf("AddName failed: %v", err)
	}

	if result.ID == uuid.Nil {
		t.Error("Expected non-nil ID")
	}
	if !result.IsPrimary {
		t.Error("Expected primary to be true")
	}
}

func TestUpdateName_AllFields(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person and add a name
	personInput := command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	}
	personResult, err := handler.CreatePerson(ctx, personInput)
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	nameResult, err := handler.AddName(ctx, command.AddNameInput{
		PersonID:  personResult.ID,
		GivenName: "John",
		Surname:   "Doe",
		NameType:  "birth",
	})
	if err != nil {
		t.Fatalf("AddName failed: %v", err)
	}

	// Update all fields
	givenName := "Johannes"
	surname := "von Doe"
	namePrefix := "Dr."
	nameSuffix := "Jr."
	surnamePrefix := "von"
	nickname := "Johnny"
	nameType := "married"

	updateInput := command.UpdateNameInput{
		PersonID:      personResult.ID,
		NameID:        nameResult.ID,
		GivenName:     &givenName,
		Surname:       &surname,
		NamePrefix:    &namePrefix,
		NameSuffix:    &nameSuffix,
		SurnamePrefix: &surnamePrefix,
		Nickname:      &nickname,
		NameType:      &nameType,
	}
	result, err := handler.UpdateName(ctx, updateInput)
	if err != nil {
		t.Fatalf("UpdateName failed: %v", err)
	}

	if result.ID != nameResult.ID {
		t.Errorf("Expected ID %s, got %s", nameResult.ID, result.ID)
	}
}
