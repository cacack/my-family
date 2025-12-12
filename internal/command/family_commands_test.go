package command_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestCreateFamily(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create two persons first
	p1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	})
	p2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Jane",
		Surname:   "Doe",
		Gender:    "female",
	})

	// Create family
	result, err := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID:       &p1.ID,
		Partner2ID:       &p2.ID,
		RelationshipType: "marriage",
		MarriageDate:     "15 JUN 1980",
		MarriagePlace:    "Springfield, IL",
	})

	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}
	if result.ID == uuid.Nil {
		t.Error("Expected non-nil ID")
	}
	if result.Version != 1 {
		t.Errorf("Version = %d, want 1", result.Version)
	}

	// Verify family in read model
	family, _ := readStore.GetFamily(ctx, result.ID)
	if family == nil {
		t.Fatal("Family not found in read model")
	}
	if family.Partner1ID == nil || *family.Partner1ID != p1.ID {
		t.Error("Partner1ID mismatch")
	}
}

func TestCreateFamily_SingleParent(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create one person
	p1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Jane",
		Surname:   "Doe",
		Gender:    "female",
	})

	// Create single-parent family
	result, err := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &p1.ID,
	})

	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}
	if result.ID == uuid.Nil {
		t.Error("Expected non-nil ID")
	}
}

func TestCreateFamily_NoPartners(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	_, err := handler.CreateFamily(ctx, command.CreateFamilyInput{})
	if err == nil {
		t.Error("Expected error for family with no partners")
	}
}

func TestUpdateFamily(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create persons and family
	p1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	createResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &p1.ID,
	})

	// Update family
	newPlace := "New York, NY"
	updateResult, err := handler.UpdateFamily(ctx, command.UpdateFamilyInput{
		ID:            createResult.ID,
		MarriagePlace: &newPlace,
		Version:       createResult.Version,
	})

	if err != nil {
		t.Fatalf("UpdateFamily failed: %v", err)
	}
	if updateResult.Version != 2 {
		t.Errorf("Version = %d, want 2", updateResult.Version)
	}

	// Verify update
	family, _ := readStore.GetFamily(ctx, createResult.ID)
	if family.MarriagePlace != "New York, NY" {
		t.Errorf("MarriagePlace = %s, want New York, NY", family.MarriagePlace)
	}
}

func TestDeleteFamily(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create family
	p1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	createResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &p1.ID,
	})

	// Delete family
	err := handler.DeleteFamily(ctx, command.DeleteFamilyInput{
		ID:      createResult.ID,
		Version: createResult.Version,
	})

	if err != nil {
		t.Fatalf("DeleteFamily failed: %v", err)
	}

	// Verify deletion
	family, _ := readStore.GetFamily(ctx, createResult.ID)
	if family != nil {
		t.Error("Family should be deleted")
	}
}

func TestDeleteFamily_WithChildren(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create family with child
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	child, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Junior",
		Surname:   "Doe",
	})
	createResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent.ID,
	})
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: createResult.ID,
		ChildID:  child.ID,
	})

	// Try to delete family with children
	err := handler.DeleteFamily(ctx, command.DeleteFamilyInput{
		ID:      createResult.ID,
		Version: 2, // Version incremented after adding child
	})

	if err != command.ErrFamilyHasChildren {
		t.Errorf("Expected ErrFamilyHasChildren, got %v", err)
	}
}

func TestLinkChild(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create family and child
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	})
	child, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Junior",
		Surname:   "Doe",
	})
	familyResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent.ID,
	})

	// Link child
	result, err := handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID:     familyResult.ID,
		ChildID:      child.ID,
		RelationType: "biological",
	})

	if err != nil {
		t.Fatalf("LinkChild failed: %v", err)
	}
	if result.FamilyVersion != 2 {
		t.Errorf("FamilyVersion = %d, want 2", result.FamilyVersion)
	}

	// Verify child in family
	childFamily, _ := readStore.GetChildFamily(ctx, child.ID)
	if childFamily == nil {
		t.Fatal("Child not linked to family")
	}
	if childFamily.ID != familyResult.ID {
		t.Error("Child linked to wrong family")
	}
}

func TestLinkChild_AlreadyLinked(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create family and child
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	child, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Junior",
		Surname:   "Doe",
	})
	familyResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent.ID,
	})

	// Link child first time
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: familyResult.ID,
		ChildID:  child.ID,
	})

	// Try to link again
	_, err := handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: familyResult.ID,
		ChildID:  child.ID,
	})

	if err != command.ErrChildAlreadyLinked {
		t.Errorf("Expected ErrChildAlreadyLinked, got %v", err)
	}
}

func TestUnlinkChild(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create family with child
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	child, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Junior",
		Surname:   "Doe",
	})
	familyResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent.ID,
	})
	linkResult, _ := handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: familyResult.ID,
		ChildID:  child.ID,
	})

	// Verify linking worked
	if linkResult.FamilyVersion != 2 {
		t.Fatalf("LinkChild version = %d, want 2", linkResult.FamilyVersion)
	}

	// Unlink child
	err := handler.UnlinkChild(ctx, command.UnlinkChildInput{
		FamilyID: familyResult.ID,
		ChildID:  child.ID,
	})

	if err != nil {
		t.Fatalf("UnlinkChild failed: %v", err)
	}

	// Verify child unlinked
	childFamily, _ := readStore.GetChildFamily(ctx, child.ID)
	if childFamily != nil {
		t.Error("Child should be unlinked")
	}
}

func TestUnlinkChild_NotInFamily(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create family and separate child
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	child, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Junior",
		Surname:   "Doe",
	})
	familyResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent.ID,
	})

	// Try to unlink child not in family
	err := handler.UnlinkChild(ctx, command.UnlinkChildInput{
		FamilyID: familyResult.ID,
		ChildID:  child.ID,
	})

	if err != command.ErrChildNotInFamily {
		t.Errorf("Expected ErrChildNotInFamily, got %v", err)
	}
}

func TestCircularAncestryDetection(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create grandparent, parent, child
	grandparent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Grandpa",
		Surname:   "Doe",
	})
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent",
		Surname:   "Doe",
	})
	child, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child",
		Surname:   "Doe",
	})

	// Create grandparent -> parent relationship
	gpFamily, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &grandparent.ID,
	})
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: gpFamily.ID,
		ChildID:  parent.ID,
	})

	// Create parent -> child relationship
	pFamily, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent.ID,
	})
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: pFamily.ID,
		ChildID:  child.ID,
	})

	// Now try to make child a parent of grandparent (circular)
	cFamily, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &child.ID,
	})
	_, err := handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: cFamily.ID,
		ChildID:  grandparent.ID,
	})

	if err != command.ErrCircularAncestry {
		t.Errorf("Expected ErrCircularAncestry, got %v", err)
	}
}
