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

func TestUpdateFamily_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	newPlace := "New York"
	_, err := handler.UpdateFamily(ctx, command.UpdateFamilyInput{
		ID:            uuid.New(),
		MarriagePlace: &newPlace,
		Version:       1,
	})

	if err != command.ErrFamilyNotFound {
		t.Errorf("Expected ErrFamilyNotFound, got %v", err)
	}
}

func TestUpdateFamily_NoChanges(t *testing.T) {
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

	// Update with no changes
	updateResult, err := handler.UpdateFamily(ctx, command.UpdateFamilyInput{
		ID:      createResult.ID,
		Version: createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateFamily failed: %v", err)
	}

	// Version should remain the same
	if updateResult.Version != createResult.Version {
		t.Errorf("Version = %d, want %d (no changes)", updateResult.Version, createResult.Version)
	}
}

func TestUpdateFamily_ClearMarriageDate(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create family with marriage date
	p1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	createResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID:    &p1.ID,
		MarriageDate:  "1 JAN 2000",
		MarriagePlace: "Boston",
	})

	// Clear marriage date
	emptyDate := ""
	updateResult, err := handler.UpdateFamily(ctx, command.UpdateFamilyInput{
		ID:           createResult.ID,
		MarriageDate: &emptyDate,
		Version:      createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateFamily failed: %v", err)
	}

	if updateResult.Version != 2 {
		t.Errorf("Version = %d, want 2", updateResult.Version)
	}
}

func TestUpdateFamily_AllFields(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create persons and family
	p1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	p3, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Jack",
		Surname:   "Smith",
	})

	createResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &p1.ID,
	})

	// Update all fields
	newRelType := "marriage"
	newMarriageDate := "15 JUN 2000"
	newMarriagePlace := "Chicago"

	updateResult, err := handler.UpdateFamily(ctx, command.UpdateFamilyInput{
		ID:               createResult.ID,
		Partner2ID:       &p3.ID,
		RelationshipType: &newRelType,
		MarriageDate:     &newMarriageDate,
		MarriagePlace:    &newMarriagePlace,
		Version:          createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateFamily failed: %v", err)
	}

	if updateResult.Version != 2 {
		t.Errorf("Version = %d, want 2", updateResult.Version)
	}
}

func TestDeleteFamily_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	err := handler.DeleteFamily(ctx, command.DeleteFamilyInput{
		ID:      uuid.New(),
		Version: 1,
	})

	if err != command.ErrFamilyNotFound {
		t.Errorf("Expected ErrFamilyNotFound, got %v", err)
	}
}

func TestCreateFamily_Partner1NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	nonExistentID := uuid.New()
	_, err := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &nonExistentID,
	})

	if err == nil {
		t.Error("Expected error when partner1 not found")
	}
}

func TestCreateFamily_Partner2NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create one partner
	p1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	nonExistentID := uuid.New()
	_, err := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &p1.ID,
		Partner2ID: &nonExistentID,
	})

	if err == nil {
		t.Error("Expected error when partner2 not found")
	}
}

func TestCircularAncestryDetection_WithPartner2(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create parent and child
	parent1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent1",
		Surname:   "Doe",
	})
	parent2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent2",
		Surname:   "Doe",
	})
	child, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child",
		Surname:   "Doe",
	})

	// Create parent family and link child
	pFamily, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent1.ID,
		Partner2ID: &parent2.ID,
	})
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: pFamily.ID,
		ChildID:  child.ID,
	})

	// Try to make child a parent of parent2 (circular via Partner2)
	cFamily, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &child.ID,
	})
	_, err := handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: cFamily.ID,
		ChildID:  parent2.ID,
	})

	if err != command.ErrCircularAncestry {
		t.Errorf("Expected ErrCircularAncestry for partner2, got %v", err)
	}
}

func TestLinkChild_FamilyNotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create child
	child, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child",
		Surname:   "Doe",
	})

	// Try to link to non-existent family
	_, err := handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: uuid.New(),
		ChildID:  child.ID,
	})

	if err != command.ErrFamilyNotFound {
		t.Errorf("Expected ErrFamilyNotFound, got %v", err)
	}
}

func TestLinkChild_ChildNotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create parent and family
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent",
		Surname:   "Doe",
	})
	family, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent.ID,
	})

	// Try to link non-existent child
	_, err := handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: family.ID,
		ChildID:  uuid.New(),
	})

	if err != command.ErrPersonNotFound {
		t.Errorf("Expected ErrPersonNotFound, got %v", err)
	}
}

func TestUnlinkChild_FamilyNotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create child
	child, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child",
		Surname:   "Doe",
	})

	// Try to unlink from non-existent family
	err := handler.UnlinkChild(ctx, command.UnlinkChildInput{
		FamilyID: uuid.New(),
		ChildID:  child.ID,
	})

	if err != command.ErrFamilyNotFound {
		t.Errorf("Expected ErrFamilyNotFound, got %v", err)
	}
}

func TestDeleteFamily_ConcurrencyConflict(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create family
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent",
		Surname:   "Doe",
	})
	family, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent.ID,
	})

	// Try to delete with wrong version
	err := handler.DeleteFamily(ctx, command.DeleteFamilyInput{
		ID:      family.ID,
		Version: 999, // Wrong version
	})

	if err == nil {
		t.Error("Expected concurrency conflict error")
	}
}

func TestUpdateFamily_ConcurrencyConflict(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create family
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent",
		Surname:   "Doe",
	})
	family, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent.ID,
	})

	newPlace := "New York"
	// Try to update with wrong version
	_, err := handler.UpdateFamily(ctx, command.UpdateFamilyInput{
		ID:            family.ID,
		MarriagePlace: &newPlace,
		Version:       999, // Wrong version
	})

	if err == nil {
		t.Error("Expected concurrency conflict error")
	}
}

func TestLinkChild_WithRelationType(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create parent, family, and child
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent",
		Surname:   "Doe",
	})
	family, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent.ID,
	})
	child, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child",
		Surname:   "Doe",
	})

	// Link child with adopted relationship type
	_, err := handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID:     family.ID,
		ChildID:      child.ID,
		RelationType: "adopted",
	})

	if err != nil {
		t.Fatalf("LinkChild with adopted relation type failed: %v", err)
	}
}

func TestIsAncestor_DirectParent(t *testing.T) {
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

	// Try to make parent a child of child (direct circular ancestry)
	childFamily, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &child.ID,
	})

	_, err := handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: childFamily.ID,
		ChildID:  parent.ID,
	})

	if err != command.ErrCircularAncestry {
		t.Errorf("Expected ErrCircularAncestry, got %v", err)
	}
}

func TestUnlinkChild_ChildInDifferentFamily(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create two families
	parent1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent1",
		Surname:   "Doe",
	})
	parent2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent2",
		Surname:   "Smith",
	})
	child, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child",
		Surname:   "Doe",
	})

	family1, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent1.ID,
	})
	family2, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent2.ID,
	})

	// Link child to family1
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: family1.ID,
		ChildID:  child.ID,
	})

	// Try to unlink from family2 (child is not in this family)
	err := handler.UnlinkChild(ctx, command.UnlinkChildInput{
		FamilyID: family2.ID,
		ChildID:  child.ID,
	})

	if err != command.ErrChildNotInFamily {
		t.Errorf("Expected ErrChildNotInFamily, got %v", err)
	}
}

func TestUnlinkChild_ChildNotLinkedToAny(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create parent, family, and child
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent",
		Surname:   "Doe",
	})
	family, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent.ID,
	})
	child, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child",
		Surname:   "Doe",
	})

	// Try to unlink child that was never linked
	err := handler.UnlinkChild(ctx, command.UnlinkChildInput{
		FamilyID: family.ID,
		ChildID:  child.ID,
	})

	if err != command.ErrChildNotInFamily {
		t.Errorf("Expected ErrChildNotInFamily when child not linked, got %v", err)
	}
}

func TestLinkChild_DefaultRelationType(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create parent, family, and child
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent",
		Surname:   "Doe",
	})
	family, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent.ID,
	})
	child, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child",
		Surname:   "Doe",
	})

	// Link child without specifying relation type (should default to biological)
	_, err := handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: family.ID,
		ChildID:  child.ID,
	})

	if err != nil {
		t.Fatalf("LinkChild failed: %v", err)
	}

	// Verify child is linked
	childFamily, _ := readStore.GetChildFamily(ctx, child.ID)
	if childFamily == nil {
		t.Fatal("Child should be linked to family")
	}
	if childFamily.ID != family.ID {
		t.Error("Child linked to wrong family")
	}
}

func TestLinkChild_ConcurrencyConflict(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create parent and family
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent",
		Surname:   "Doe",
	})
	family, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent.ID,
	})
	child1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child1",
		Surname:   "Doe",
	})
	child2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child2",
		Surname:   "Doe",
	})

	// Link first child
	_, err := handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: family.ID,
		ChildID:  child1.ID,
	})
	if err != nil {
		t.Fatalf("First LinkChild failed: %v", err)
	}

	// Link second child (should succeed since it uses current version)
	_, err = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: family.ID,
		ChildID:  child2.ID,
	})
	if err != nil {
		t.Fatalf("Second LinkChild failed: %v", err)
	}

	// Verify both children are linked
	children, _ := readStore.GetChildrenOfFamily(ctx, family.ID)
	if len(children) != 2 {
		t.Errorf("Expected 2 children, got %d", len(children))
	}
}

func TestUnlinkChild_ConcurrencyConflict(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create parent, family, and children
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent",
		Surname:   "Doe",
	})
	family, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent.ID,
	})
	child1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child1",
		Surname:   "Doe",
	})
	child2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child2",
		Surname:   "Doe",
	})

	// Link both children
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: family.ID,
		ChildID:  child1.ID,
	})
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: family.ID,
		ChildID:  child2.ID,
	})

	// Unlink first child
	err := handler.UnlinkChild(ctx, command.UnlinkChildInput{
		FamilyID: family.ID,
		ChildID:  child1.ID,
	})
	if err != nil {
		t.Fatalf("First UnlinkChild failed: %v", err)
	}

	// Verify only one child remains
	children, _ := readStore.GetChildrenOfFamily(ctx, family.ID)
	if len(children) != 1 {
		t.Errorf("Expected 1 child after unlink, got %d", len(children))
	}
}

func TestCreateFamily_WithRelationshipType(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create persons
	p1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Partner1",
		Surname:   "Test",
	})
	p2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Partner2",
		Surname:   "Test",
	})

	// Create family with partnership relationship type
	result, err := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID:       &p1.ID,
		Partner2ID:       &p2.ID,
		RelationshipType: "partnership",
		MarriageDate:     "ABT 2000",
		MarriagePlace:    "Somewhere",
	})

	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}
	if result.ID == uuid.Nil {
		t.Error("Expected non-nil ID")
	}

	// Verify family in read model
	family, _ := readStore.GetFamily(ctx, result.ID)
	if family == nil {
		t.Fatal("Family not found")
	}
}

func TestUpdateFamily_Partner1And2(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create persons
	p1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Partner1",
		Surname:   "Test",
	})
	p2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Partner2",
		Surname:   "Test",
	})
	p3, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Partner3",
		Surname:   "New",
	})

	// Create family with one partner
	createResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &p1.ID,
	})

	// Update to add partner2 and change partner1
	updateResult, err := handler.UpdateFamily(ctx, command.UpdateFamilyInput{
		ID:         createResult.ID,
		Partner1ID: &p3.ID,
		Partner2ID: &p2.ID,
		Version:    createResult.Version,
	})

	if err != nil {
		t.Fatalf("UpdateFamily failed: %v", err)
	}
	if updateResult.Version != 2 {
		t.Errorf("Version = %d, want 2", updateResult.Version)
	}
}

func TestIsAncestor_Partner2Path(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create grandparent, parent1, parent2, and child
	grandparent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Grandparent",
		Surname:   "Test",
	})
	parent1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent1",
		Surname:   "Test",
	})
	parent2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent2",
		Surname:   "Other",
	})
	child, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child",
		Surname:   "Test",
	})

	// Create grandparent family with parent2 as partner2
	gpFamily, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &grandparent.ID,
		Partner2ID: &parent2.ID,
	})
	// Link parent1 as child of grandparent family
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: gpFamily.ID,
		ChildID:  parent1.ID,
	})

	// Create parent family
	parentFamily, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent1.ID,
	})
	// Link child
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: parentFamily.ID,
		ChildID:  child.ID,
	})

	// Now try to make child a parent of grandparent (circular through partner2 path)
	childFamily, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &child.ID,
	})
	// This should fail - child can't be parent of grandparent
	_, err := handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: childFamily.ID,
		ChildID:  grandparent.ID,
	})

	if err != command.ErrCircularAncestry {
		t.Errorf("Expected ErrCircularAncestry, got %v", err)
	}
}

func TestIsAncestor_SamePersonCheck(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a person
	person, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Self",
		Surname:   "Test",
	})

	// Create a family with this person as a partner
	family, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &person.ID,
	})

	// Try to link the person as their own child (should fail)
	_, err := handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: family.ID,
		ChildID:  person.ID,
	})

	// This should fail because person == person (same ID check in isAncestor)
	if err != command.ErrCircularAncestry {
		t.Errorf("Expected ErrCircularAncestry when linking person as own child, got %v", err)
	}
}

func TestIsAncestor_NoParentFamily(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create two persons with no parent families
	person1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Person1",
		Surname:   "Test",
	})
	person2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Person2",
		Surname:   "Test",
	})

	// Create a family with person1 as partner
	family, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &person1.ID,
	})

	// Link person2 as child (should succeed - no ancestry to check)
	_, err := handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: family.ID,
		ChildID:  person2.ID,
	})

	if err != nil {
		t.Fatalf("LinkChild failed: %v", err)
	}
}

func TestIsAncestor_DeepAncestryCheck(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a deep ancestry chain: ggp -> gp -> parent -> child
	ggp, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "GreatGrandparent",
		Surname:   "Test",
	})
	gp, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Grandparent",
		Surname:   "Test",
	})
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent",
		Surname:   "Test",
	})
	child, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child",
		Surname:   "Test",
	})

	// ggp -> gp
	ggpFamily, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &ggp.ID,
	})
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: ggpFamily.ID,
		ChildID:  gp.ID,
	})

	// gp -> parent
	gpFamily, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &gp.ID,
	})
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: gpFamily.ID,
		ChildID:  parent.ID,
	})

	// parent -> child
	parentFamily, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent.ID,
	})
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: parentFamily.ID,
		ChildID:  child.ID,
	})

	// Now try to make child a parent of ggp (circular across 4 generations)
	childFamily, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &child.ID,
	})
	_, err := handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: childFamily.ID,
		ChildID:  ggp.ID,
	})

	if err != command.ErrCircularAncestry {
		t.Errorf("Expected ErrCircularAncestry for deep ancestry, got %v", err)
	}
}

func TestCircularAncestryDetection_WithBothPartners(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create grandparent, two parents, and a child
	grandparent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Grandparent",
		Surname:   "Test",
	})
	parent1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent1",
		Surname:   "Test",
	})
	parent2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent2",
		Surname:   "Test",
	})
	child, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child",
		Surname:   "Test",
	})

	// grandparent -> parent1
	gpFamily, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &grandparent.ID,
	})
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: gpFamily.ID,
		ChildID:  parent1.ID,
	})

	// parent1 + parent2 -> child
	parentFamily, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent1.ID,
		Partner2ID: &parent2.ID,
	})
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: parentFamily.ID,
		ChildID:  child.ID,
	})

	// Now try to make child a parent of parent2 (circular via Partner2)
	childFamily, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &child.ID,
	})
	_, err := handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: childFamily.ID,
		ChildID:  parent2.ID,
	})

	if err != command.ErrCircularAncestry {
		t.Errorf("Expected ErrCircularAncestry for Partner2 path, got %v", err)
	}
}

func TestCircularAncestryDetection_ThroughPartner2Ancestry(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create persons: gp -> parent2, parent1 + parent2 -> child
	// Then try to make child a parent of gp via the Partner2 ancestry chain
	gp, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "GP",
		Surname:   "Test",
	})
	parent1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent1",
		Surname:   "Test",
	})
	parent2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent2",
		Surname:   "Test",
	})
	child, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child",
		Surname:   "Test",
	})

	// gp -> parent2
	gpFamily, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &gp.ID,
	})
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: gpFamily.ID,
		ChildID:  parent2.ID,
	})

	// parent1 + parent2 -> child
	parentFamily, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent1.ID,
		Partner2ID: &parent2.ID,
	})
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: parentFamily.ID,
		ChildID:  child.ID,
	})

	// Now try to make child's family and add gp as child (circular through parent2's ancestry)
	childFamily, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &child.ID,
	})
	_, err := handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: childFamily.ID,
		ChildID:  gp.ID,
	})

	if err != command.ErrCircularAncestry {
		t.Errorf("Expected ErrCircularAncestry through Partner2's ancestry, got %v", err)
	}
}

// ============================================================================
// Optimistic Locking / Version Conflict Tests for Family
// ============================================================================

func TestFamilyUpdateStaleVersion(t *testing.T) {
	// Tests that updating a family with a stale version fails
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person and family
	p1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	createResult, err := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &p1.ID,
	})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}
	if createResult.Version != 1 {
		t.Fatalf("Expected initial version 1, got %d", createResult.Version)
	}

	// First update succeeds (version 1 -> 2)
	newPlace := "Boston"
	updateResult, err := handler.UpdateFamily(ctx, command.UpdateFamilyInput{
		ID:            createResult.ID,
		MarriagePlace: &newPlace,
		Version:       1,
	})
	if err != nil {
		t.Fatalf("First update failed: %v", err)
	}
	if updateResult.Version != 2 {
		t.Fatalf("Expected version 2 after first update, got %d", updateResult.Version)
	}

	// Attempt update with stale version 1 (current is 2)
	stalePlace := "Chicago"
	_, err = handler.UpdateFamily(ctx, command.UpdateFamilyInput{
		ID:            createResult.ID,
		MarriagePlace: &stalePlace,
		Version:       1, // Stale version
	})
	if err == nil {
		t.Fatal("Expected version conflict error for stale version update")
	}

	// Verify the family still has the value from the successful update
	family, _ := readStore.GetFamily(ctx, createResult.ID)
	if family.MarriagePlace != "Boston" {
		t.Errorf("Family MarriagePlace = %s, want Boston", family.MarriagePlace)
	}
}

func TestFamilyConcurrentModificationScenario(t *testing.T) {
	// Simulates two concurrent updates to the same family with the same base version
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person and family
	p1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	createResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &p1.ID,
	})

	// Simulate two "concurrent" readers getting the same version
	baseVersion := createResult.Version // Both readers see version 1

	// First "concurrent" update succeeds
	place1 := "Boston"
	_, err := handler.UpdateFamily(ctx, command.UpdateFamilyInput{
		ID:            createResult.ID,
		MarriagePlace: &place1,
		Version:       baseVersion,
	})
	if err != nil {
		t.Fatalf("First concurrent update failed: %v", err)
	}

	// Second "concurrent" update with same base version should fail
	place2 := "Chicago"
	_, err = handler.UpdateFamily(ctx, command.UpdateFamilyInput{
		ID:            createResult.ID,
		MarriagePlace: &place2,
		Version:       baseVersion, // Same stale version
	})
	if err == nil {
		t.Fatal("Expected version conflict error for second concurrent update")
	}

	// Verify only the first update was applied
	family, _ := readStore.GetFamily(ctx, createResult.ID)
	if family.MarriagePlace != "Boston" {
		t.Errorf("Family MarriagePlace = %s, want Boston", family.MarriagePlace)
	}
}

func TestFamilySequentialUpdatesSucceed(t *testing.T) {
	// Tests that sequential updates with correct version increments all succeed
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person and family
	p1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	createResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &p1.ID,
	})

	// Sequential updates with correct versions
	updates := []struct {
		place           string
		expectedVersion int64
	}{
		{"Boston", 2},
		{"Chicago", 3},
		{"New York", 4},
	}

	currentVersion := createResult.Version
	for _, update := range updates {
		newPlace := update.place
		result, err := handler.UpdateFamily(ctx, command.UpdateFamilyInput{
			ID:            createResult.ID,
			MarriagePlace: &newPlace,
			Version:       currentVersion,
		})
		if err != nil {
			t.Fatalf("Sequential update to %s failed: %v", update.place, err)
		}
		if result.Version != update.expectedVersion {
			t.Errorf("After update to %s: version = %d, want %d", update.place, result.Version, update.expectedVersion)
		}
		currentVersion = result.Version
	}

	// Verify final state - data should be updated
	family, _ := readStore.GetFamily(ctx, createResult.ID)
	if family.MarriagePlace != "New York" {
		t.Errorf("Final MarriagePlace = %s, want New York", family.MarriagePlace)
	}

	// Verify event store version tracking (source of truth for optimistic locking)
	eventStoreVersion, _ := eventStore.GetStreamVersion(ctx, createResult.ID)
	if eventStoreVersion != 4 {
		t.Errorf("Event store version = %d, want 4", eventStoreVersion)
	}
}

func TestFamilyDeleteStaleVersion(t *testing.T) {
	// Tests that deleting a family with a stale version fails
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person and family
	p1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	createResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &p1.ID,
	})

	// Update family (version 1 -> 2)
	newPlace := "Boston"
	_, err := handler.UpdateFamily(ctx, command.UpdateFamilyInput{
		ID:            createResult.ID,
		MarriagePlace: &newPlace,
		Version:       1,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Attempt delete with stale version 1
	err = handler.DeleteFamily(ctx, command.DeleteFamilyInput{
		ID:      createResult.ID,
		Version: 1, // Stale version
	})
	if err == nil {
		t.Fatal("Expected version conflict error for stale version delete")
	}

	// Verify family still exists
	family, _ := readStore.GetFamily(ctx, createResult.ID)
	if family == nil {
		t.Error("Family should still exist after failed delete")
	}
}

func TestFamilyDeleteWithCorrectVersion(t *testing.T) {
	// Tests that delete succeeds with correct version after updates
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person and family
	p1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	createResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &p1.ID,
	})

	// Update family (version 1 -> 2)
	newPlace := "Boston"
	updateResult, err := handler.UpdateFamily(ctx, command.UpdateFamilyInput{
		ID:            createResult.ID,
		MarriagePlace: &newPlace,
		Version:       1,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Delete with correct version
	err = handler.DeleteFamily(ctx, command.DeleteFamilyInput{
		ID:      createResult.ID,
		Version: updateResult.Version,
	})
	if err != nil {
		t.Fatalf("Delete with correct version failed: %v", err)
	}

	// Verify family is deleted
	family, _ := readStore.GetFamily(ctx, createResult.ID)
	if family != nil {
		t.Error("Family should be deleted")
	}
}

func TestFamilyLinkChildVersionTracking(t *testing.T) {
	// Tests that linking children properly tracks versions through the event store
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create parent and family
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent",
		Surname:   "Doe",
	})
	createResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent.ID,
	})

	// Create two children
	child1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child1",
		Surname:   "Doe",
	})
	child2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child2",
		Surname:   "Doe",
	})

	// Link first child (increments event store version from 1 to 2)
	linkResult1, err := handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: createResult.ID,
		ChildID:  child1.ID,
	})
	if err != nil {
		t.Fatalf("First LinkChild failed: %v", err)
	}
	if linkResult1.FamilyVersion != 2 {
		t.Errorf("Expected family version 2 after first LinkChild, got %d", linkResult1.FamilyVersion)
	}

	// Link second child (should succeed as it reads current version)
	linkResult2, err := handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: createResult.ID,
		ChildID:  child2.ID,
	})
	if err != nil {
		t.Fatalf("Second LinkChild failed: %v", err)
	}
	if linkResult2.FamilyVersion != 3 {
		t.Errorf("Expected family version 3 after second LinkChild, got %d", linkResult2.FamilyVersion)
	}

	// Verify both children are linked
	children, _ := readStore.GetChildrenOfFamily(ctx, createResult.ID)
	if len(children) != 2 {
		t.Errorf("Expected 2 children, got %d", len(children))
	}
}

func TestFamilyUnlinkChildVersionConflict(t *testing.T) {
	// Tests that unlinking a child works with proper version tracking
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create parent and family
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent",
		Surname:   "Doe",
	})
	family, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent.ID,
	})

	// Create and link two children
	child1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child1",
		Surname:   "Doe",
	})
	child2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child2",
		Surname:   "Doe",
	})

	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: family.ID,
		ChildID:  child1.ID,
	})
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: family.ID,
		ChildID:  child2.ID,
	})

	// Unlink first child
	err := handler.UnlinkChild(ctx, command.UnlinkChildInput{
		FamilyID: family.ID,
		ChildID:  child1.ID,
	})
	if err != nil {
		t.Fatalf("First UnlinkChild failed: %v", err)
	}

	// Unlink second child (should succeed as it reads current version)
	err = handler.UnlinkChild(ctx, command.UnlinkChildInput{
		FamilyID: family.ID,
		ChildID:  child2.ID,
	})
	if err != nil {
		t.Fatalf("Second UnlinkChild failed: %v", err)
	}

	// Verify both children are unlinked
	children, _ := readStore.GetChildrenOfFamily(ctx, family.ID)
	if len(children) != 0 {
		t.Errorf("Expected 0 children, got %d", len(children))
	}
}

func TestFamilyVersionConflictNoPartialState(t *testing.T) {
	// Tests that version conflict leaves no partial state changes for family
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create persons
	p1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Partner1",
		Surname:   "Doe",
	})
	p2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Partner2",
		Surname:   "Smith",
	})

	// Create family with initial values
	createResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID:    &p1.ID,
		MarriagePlace: "Boston",
	})

	// Update to change values (version 1 -> 2)
	newPlace := "New York"
	_, err := handler.UpdateFamily(ctx, command.UpdateFamilyInput{
		ID:            createResult.ID,
		MarriagePlace: &newPlace,
		Version:       1,
	})
	if err != nil {
		t.Fatalf("First update failed: %v", err)
	}

	// Attempt update with stale version that would change multiple fields
	stalePlace := "Chicago"
	staleDate := "1 JAN 2000"
	_, err = handler.UpdateFamily(ctx, command.UpdateFamilyInput{
		ID:            createResult.ID,
		Partner2ID:    &p2.ID,
		MarriagePlace: &stalePlace,
		MarriageDate:  &staleDate,
		Version:       1, // Stale version
	})
	if err == nil {
		t.Fatal("Expected version conflict error")
	}

	// Verify no partial changes were applied
	family, _ := readStore.GetFamily(ctx, createResult.ID)
	if family.MarriagePlace != "New York" {
		t.Errorf("MarriagePlace = %s, want New York (from successful update)", family.MarriagePlace)
	}
	if family.Partner2ID != nil {
		t.Error("Partner2ID should be nil (stale update should not apply)")
	}
}

func TestFamilyMultipleUpdatesVersionTracking(t *testing.T) {
	// Tests version tracking across multiple UpdateFamily operations
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create parent
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent",
		Surname:   "Doe",
	})

	// Create family (event store version 1)
	createResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent.ID,
	})
	if createResult.Version != 1 {
		t.Errorf("After CreateFamily: version = %d, want 1", createResult.Version)
	}

	// Sequential updates tracking versions through returned results
	updates := []struct {
		place           string
		inputVersion    int64
		expectedVersion int64
	}{
		{"Boston", 1, 2},
		{"Chicago", 2, 3},
		{"New York", 3, 4},
	}

	for _, update := range updates {
		newPlace := update.place
		result, err := handler.UpdateFamily(ctx, command.UpdateFamilyInput{
			ID:            createResult.ID,
			MarriagePlace: &newPlace,
			Version:       update.inputVersion,
		})
		if err != nil {
			t.Fatalf("UpdateFamily to %s failed: %v", update.place, err)
		}
		if result.Version != update.expectedVersion {
			t.Errorf("After update to %s: version = %d, want %d", update.place, result.Version, update.expectedVersion)
		}
	}

	// Verify final state
	family, _ := readStore.GetFamily(ctx, createResult.ID)
	if family.MarriagePlace != "New York" {
		t.Errorf("Final MarriagePlace = %s, want New York", family.MarriagePlace)
	}

	// Verify event store version tracking
	eventStoreVersion, _ := eventStore.GetStreamVersion(ctx, createResult.ID)
	if eventStoreVersion != 4 {
		t.Errorf("Event store version = %d, want 4", eventStoreVersion)
	}
}
