package command_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestMergePersons_Success(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create survivor
	survivor, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName:  "John",
		Surname:    "Doe",
		BirthDate:  "1 JAN 1850",
		BirthPlace: "Springfield, IL",
	})
	if err != nil {
		t.Fatalf("CreatePerson survivor failed: %v", err)
	}

	// Create person to be merged
	merged, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName:  "Jonathan",
		Surname:    "Doe",
		DeathDate:  "31 DEC 1920",
		DeathPlace: "Chicago, IL",
	})
	if err != nil {
		t.Fatalf("CreatePerson merged failed: %v", err)
	}

	// Merge persons
	result, err := handler.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID:      survivor.ID,
		MergedID:        merged.ID,
		SurvivorVersion: survivor.Version,
		MergedVersion:   merged.Version,
	})
	if err != nil {
		t.Fatalf("MergePersons failed: %v", err)
	}

	// Verify result
	if result.SurvivorID != survivor.ID {
		t.Errorf("SurvivorID = %v, want %v", result.SurvivorID, survivor.ID)
	}
	if result.Version != 2 {
		t.Errorf("Version = %d, want 2", result.Version)
	}
	if result.Summary.MergedPersonName != "Jonathan Doe" {
		t.Errorf("MergedPersonName = %s, want Jonathan Doe", result.Summary.MergedPersonName)
	}

	// Verify survivor has merged data (death info from merged person)
	person, err := readStore.GetPerson(ctx, survivor.ID)
	if err != nil {
		t.Fatalf("GetPerson failed: %v", err)
	}
	if person.DeathDateRaw != "31 DEC 1920" {
		t.Errorf("DeathDateRaw = %s, want 31 DEC 1920", person.DeathDateRaw)
	}
	if person.DeathPlace != "Chicago, IL" {
		t.Errorf("DeathPlace = %s, want Chicago, IL", person.DeathPlace)
	}
	// Survivor's original birth data should be preserved
	if person.BirthDateRaw != "1 JAN 1850" {
		t.Errorf("BirthDateRaw = %s, want 1 JAN 1850", person.BirthDateRaw)
	}
	if person.BirthPlace != "Springfield, IL" {
		t.Errorf("BirthPlace = %s, want Springfield, IL", person.BirthPlace)
	}

	// Verify merged person is deleted
	mergedPerson, _ := readStore.GetPerson(ctx, merged.ID)
	if mergedPerson != nil {
		t.Error("Merged person should be deleted")
	}
}

func TestMergePersons_SamePersonError(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person
	person, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	// Try to merge person with themselves
	_, err := handler.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID:      person.ID,
		MergedID:        person.ID,
		SurvivorVersion: person.Version,
		MergedVersion:   person.Version,
	})
	if err != command.ErrSamePersonMerge {
		t.Errorf("Expected ErrSamePersonMerge, got %v", err)
	}
}

func TestMergePersons_SurvivorNotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create only merged person
	merged, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	_, err := handler.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID:      uuid.New(),
		MergedID:        merged.ID,
		SurvivorVersion: 1,
		MergedVersion:   merged.Version,
	})
	if err == nil {
		t.Error("Expected error for missing survivor")
	}
}

func TestMergePersons_MergedNotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create only survivor
	survivor, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	_, err := handler.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID:      survivor.ID,
		MergedID:        uuid.New(),
		SurvivorVersion: survivor.Version,
		MergedVersion:   1,
	})
	if err == nil {
		t.Error("Expected error for missing merged person")
	}
}

func TestMergePersons_SurvivorVersionConflict(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	survivor, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	merged, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Jane",
		Surname:   "Doe",
	})

	_, err := handler.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID:      survivor.ID,
		MergedID:        merged.ID,
		SurvivorVersion: 999, // Wrong version
		MergedVersion:   merged.Version,
	})
	if err != repository.ErrConcurrencyConflict {
		t.Errorf("Expected ErrConcurrencyConflict, got %v", err)
	}
}

func TestMergePersons_MergedVersionConflict(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	survivor, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	merged, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Jane",
		Surname:   "Doe",
	})

	_, err := handler.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID:      survivor.ID,
		MergedID:        merged.ID,
		SurvivorVersion: survivor.Version,
		MergedVersion:   999, // Wrong version
	})
	if err != repository.ErrConcurrencyConflict {
		t.Errorf("Expected ErrConcurrencyConflict, got %v", err)
	}
}

func TestMergePersons_CircularMerge_SurvivorIsAncestor(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create parent (survivor)
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent",
		Surname:   "Doe",
		Gender:    "male",
	})

	// Create child (to be merged)
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

	// Try to merge parent into child (parent is ancestor of child)
	_, err := handler.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID:      child.ID,
		MergedID:        parent.ID,
		SurvivorVersion: child.Version,
		MergedVersion:   parent.Version,
	})
	if err != command.ErrCircularMerge {
		t.Errorf("Expected ErrCircularMerge, got %v", err)
	}
}

func TestMergePersons_CircularMerge_MergedIsAncestor(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create parent (to be merged)
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent",
		Surname:   "Doe",
		Gender:    "male",
	})

	// Create child (survivor)
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

	// Try to merge child into parent (child is descendant of parent -> parent is ancestor of child)
	_, err := handler.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID:      parent.ID,
		MergedID:        child.ID,
		SurvivorVersion: parent.Version,
		MergedVersion:   child.Version,
	})
	if err != command.ErrCircularMerge {
		t.Errorf("Expected ErrCircularMerge, got %v", err)
	}
}

func TestMergePersons_ChildFamilyConflict(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create two separate parent persons
	parent1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent1",
		Surname:   "Smith",
		Gender:    "male",
	})
	parent2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent2",
		Surname:   "Jones",
		Gender:    "male",
	})

	// Create two children to merge
	child1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child1",
		Surname:   "Smith",
	})
	child2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child2",
		Surname:   "Jones",
	})

	// Create two separate families
	family1, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent1.ID,
	})
	family2, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent2.ID,
	})

	// Link each child to different family
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: family1.ID,
		ChildID:  child1.ID,
	})
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: family2.ID,
		ChildID:  child2.ID,
	})

	// Try to merge - should fail because both are children in different families
	_, err := handler.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID:      child1.ID,
		MergedID:        child2.ID,
		SurvivorVersion: child1.Version,
		MergedVersion:   child2.Version,
	})
	if err != command.ErrChildFamilyConflict {
		t.Errorf("Expected ErrChildFamilyConflict, got %v", err)
	}
}

func TestMergePersons_WithFieldResolution(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create survivor with birth info
	survivor, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName:  "John",
		Surname:    "Doe",
		BirthDate:  "1 JAN 1850",
		BirthPlace: "Springfield, IL",
	})

	// Create merged person with different birth info
	merged, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName:  "Jonathan",
		Surname:    "Doe",
		BirthDate:  "5 JAN 1850",
		BirthPlace: "Boston, MA",
	})

	// Merge with explicit field resolution preferring merged person's birth data
	result, err := handler.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID:      survivor.ID,
		MergedID:        merged.ID,
		SurvivorVersion: survivor.Version,
		MergedVersion:   merged.Version,
		FieldResolution: map[string]string{
			"birth_date":  "merged",
			"birth_place": "merged",
		},
	})
	if err != nil {
		t.Fatalf("MergePersons failed: %v", err)
	}

	// Verify fields were updated
	if len(result.Summary.FieldsUpdated) == 0 {
		t.Error("Expected fields to be updated")
	}

	// Verify survivor has merged person's birth data
	person, _ := readStore.GetPerson(ctx, survivor.ID)
	if person.BirthDateRaw != "5 JAN 1850" {
		t.Errorf("BirthDateRaw = %s, want 5 JAN 1850", person.BirthDateRaw)
	}
	if person.BirthPlace != "Boston, MA" {
		t.Errorf("BirthPlace = %s, want Boston, MA", person.BirthPlace)
	}
}

func TestMergePersons_WithFamilies(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create survivor
	survivor, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	})

	// Create merged person
	merged, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Jonathan",
		Surname:   "Doe",
		Gender:    "male",
	})

	// Create spouse
	spouse, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Jane",
		Surname:   "Smith",
		Gender:    "female",
	})

	// Create family with merged person as partner
	_, _ = handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &merged.ID,
		Partner2ID: &spouse.ID,
	})

	// Merge persons
	result, err := handler.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID:      survivor.ID,
		MergedID:        merged.ID,
		SurvivorVersion: survivor.Version,
		MergedVersion:   merged.Version,
	})
	if err != nil {
		t.Fatalf("MergePersons failed: %v", err)
	}

	// Verify family was updated
	if result.Summary.FamiliesUpdated != 1 {
		t.Errorf("FamiliesUpdated = %d, want 1", result.Summary.FamiliesUpdated)
	}

	// Verify family now has survivor as partner
	families, _ := readStore.GetFamiliesForPerson(ctx, survivor.ID)
	if len(families) != 1 {
		t.Fatalf("Expected 1 family for survivor, got %d", len(families))
	}
	if families[0].Partner1ID == nil || *families[0].Partner1ID != survivor.ID {
		t.Error("Family should have survivor as partner1")
	}
}

func TestMergePersons_NoChangesWhenSurvivorComplete(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create complete survivor
	survivor, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName:  "John",
		Surname:    "Doe",
		Gender:     "male",
		BirthDate:  "1 JAN 1850",
		BirthPlace: "Springfield, IL",
		DeathDate:  "31 DEC 1920",
		DeathPlace: "Chicago, IL",
	})

	// Create merged person with same/less info
	merged, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Jonathan",
		Surname:   "Doe",
	})

	// Merge persons
	result, err := handler.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID:      survivor.ID,
		MergedID:        merged.ID,
		SurvivorVersion: survivor.Version,
		MergedVersion:   merged.Version,
	})
	if err != nil {
		t.Fatalf("MergePersons failed: %v", err)
	}

	// Verify no fields were updated (survivor already has all data)
	if len(result.Summary.FieldsUpdated) != 0 {
		t.Errorf("Expected 0 fields updated, got %d: %v", len(result.Summary.FieldsUpdated), result.Summary.FieldsUpdated)
	}

	// Verify survivor data unchanged
	person, _ := readStore.GetPerson(ctx, survivor.ID)
	if person.GivenName != "John" {
		t.Errorf("GivenName = %s, want John", person.GivenName)
	}
	if person.BirthDateRaw != "1 JAN 1850" {
		t.Errorf("BirthDateRaw = %s, want 1 JAN 1850", person.BirthDateRaw)
	}
}

func TestMergePersons_TransfersEmptyFieldsFromMerged(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create survivor with minimal info
	survivor, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	// Create merged person with more info
	merged, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName:  "Jonathan",
		Surname:    "Doe",
		Gender:     "male",
		BirthDate:  "1 JAN 1850",
		BirthPlace: "Boston, MA",
		DeathDate:  "31 DEC 1920",
		DeathPlace: "Chicago, IL",
		Notes:      "Important person",
	})

	// Merge persons - should auto-fill empty fields from merged
	result, err := handler.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID:      survivor.ID,
		MergedID:        merged.ID,
		SurvivorVersion: survivor.Version,
		MergedVersion:   merged.Version,
	})
	if err != nil {
		t.Fatalf("MergePersons failed: %v", err)
	}

	// Verify fields were updated
	if len(result.Summary.FieldsUpdated) == 0 {
		t.Error("Expected fields to be updated")
	}

	// Verify survivor now has merged person's data
	person, _ := readStore.GetPerson(ctx, survivor.ID)
	if person.Gender != "male" {
		t.Errorf("Gender = %s, want male", person.Gender)
	}
	if person.BirthDateRaw != "1 JAN 1850" {
		t.Errorf("BirthDateRaw = %s, want 1 JAN 1850", person.BirthDateRaw)
	}
	if person.DeathPlace != "Chicago, IL" {
		t.Errorf("DeathPlace = %s, want Chicago, IL", person.DeathPlace)
	}
	if person.Notes != "Important person" {
		t.Errorf("Notes = %s, want Important person", person.Notes)
	}
}

func TestMergePersons_SameChildFamily_NoConflict(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create parent
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Parent",
		Surname:   "Doe",
		Gender:    "male",
	})

	// Create two children that are duplicates (same family)
	child1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Child",
		Surname:   "Doe",
	})
	// Note: Only one child can be linked to the family, so we create second without linking
	child2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Same Child",
		Surname:   "Doe",
	})

	// Create family and link only child1
	family, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent.ID,
	})
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID: family.ID,
		ChildID:  child1.ID,
	})

	// Merge should succeed because child2 is not in any family
	_, err := handler.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID:      child1.ID,
		MergedID:        child2.ID,
		SurvivorVersion: child1.Version,
		MergedVersion:   child2.Version,
	})
	if err != nil {
		t.Errorf("MergePersons should succeed when merged has no child family, got %v", err)
	}
}
