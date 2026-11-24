package command_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
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
	if result.Version != 3 {
		t.Errorf("Version = %d, want 3", result.Version)
	}
	if result.Summary.MergedPersonName != "Jonathan Doe" {
		t.Errorf("MergedPersonName = %s, want Jonathan Doe", result.Summary.MergedPersonName)
	}

	// Verify survivor has merged data (death info from merged person)
	person, err := readStore.GetPerson(ctx, domain.MainBranchID, survivor.ID)
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
	mergedPerson, _ := readStore.GetPerson(ctx, domain.MainBranchID, merged.ID)
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
	if !errors.Is(err, repository.ErrConcurrencyConflict) {
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
	if !errors.Is(err, repository.ErrConcurrencyConflict) {
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
	if !errors.Is(err, command.ErrCircularMerge) {
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
	if !errors.Is(err, command.ErrCircularMerge) {
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
	if !errors.Is(err, command.ErrChildFamilyConflict) {
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
	person, _ := readStore.GetPerson(ctx, domain.MainBranchID, survivor.ID)
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
	families, _ := readStore.GetFamiliesForPerson(ctx, domain.MainBranchID, survivor.ID)
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
	person, _ := readStore.GetPerson(ctx, domain.MainBranchID, survivor.ID)
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
	person, _ := readStore.GetPerson(ctx, domain.MainBranchID, survivor.ID)
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

func TestMergePersons_TypeDetailFollowsResolvedEvent(t *testing.T) {
	// The birth/death TYPE goes with the resolved date or place (#1352): an
	// empty merged TYPE replaces the survivor's when the birth date resolves
	// to the merged person, and a death kept from the survivor keeps its TYPE.
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	survivor, _ := handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "John", Surname: "Doe", BirthDate: "1850", DeathDate: "1920"})
	merged, _ := handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Johnny", Surname: "Doe", BirthDate: "1851", DeathDate: "1921"})
	// TYPE is set only by GEDCOM import; set it on the rows the merge reads.
	setTypes := func(id uuid.UUID, birth, death string) {
		p, _ := readStore.GetPerson(ctx, domain.MainBranchID, id)
		p.BirthTypeDetail, p.DeathTypeDetail = birth, death
		if err := readStore.SavePerson(ctx, domain.MainBranchID, p); err != nil {
			t.Fatal(err)
		}
	}
	setTypes(survivor.ID, "Hospital birth", "Accident")
	setTypes(merged.ID, "", "Illness")

	result, err := handler.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID: survivor.ID, MergedID: merged.ID,
		SurvivorVersion: survivor.Version, MergedVersion: merged.Version,
		FieldResolution: map[string]string{"birth_date": "merged"},
	})
	if err != nil {
		t.Fatalf("MergePersons failed: %v", err)
	}
	want := []string{"birth_date", "birth_type_detail"}
	if len(result.Summary.FieldsUpdated) != len(want) || result.Summary.FieldsUpdated[0] != want[0] || result.Summary.FieldsUpdated[1] != want[1] {
		t.Errorf("FieldsUpdated = %v, want %v", result.Summary.FieldsUpdated, want)
	}
	person, _ := readStore.GetPerson(ctx, domain.MainBranchID, survivor.ID)
	if person.BirthDateRaw != "1851" || person.BirthTypeDetail != "" {
		t.Errorf("birth = %q, TYPE %q; want 1851 with no TYPE", person.BirthDateRaw, person.BirthTypeDetail)
	}
	if person.DeathDateRaw != "1920" || person.DeathTypeDetail != "Accident" {
		t.Errorf("death = %q, TYPE %q; want 1920, Accident", person.DeathDateRaw, person.DeathTypeDetail)
	}
}

func TestMergePersons_CoordinatesFollowResolvedPlace(t *testing.T) {
	// A place's coordinate pair goes with the place (#1799): a merged place
	// brings its pair and origin, an empty merged pair clears the survivor's,
	// and a place kept from the survivor keeps its pair.
	coords := func(lat, long string) command.CoordinatesPatch {
		return command.CoordinatesPatch{Latitude: command.Set(lat), Longitude: command.Set(long)}
	}
	tests := []struct {
		name                     string
		survivorPair, mergedPair command.CoordinatesPatch
		resolution               map[string]string
		wantPlace                string
		wantLat, wantLong        string
		wantOrigin               *domain.CoordinateOrigin
		wantRecorded             []string
	}{
		{
			name: "merged place brings its pair", survivorPair: coords("N42.3601", "W71.0589"), mergedPair: coords("N42.5195", "W70.8967"),
			resolution: map[string]string{"birth_place": "merged"}, wantPlace: "Salem",
			wantLat: "N42.5195", wantLong: "W70.8967", wantOrigin: &domain.CoordinateOrigin{Source: domain.CoordinateManual},
			wantRecorded: []string{"birth_place", "birth_place_lat", "birth_place_long"},
		},
		{
			name: "empty merged pair clears the survivor's", survivorPair: coords("N42.3601", "W71.0589"),
			resolution: map[string]string{"birth_place": "merged"}, wantPlace: "Salem",
			wantRecorded: []string{"birth_place", "birth_place_lat", "birth_place_long"},
		},
		{
			name: "survivor's place keeps its pair", survivorPair: coords("N42.3601", "W71.0589"), mergedPair: coords("N42.5195", "W70.8967"),
			wantPlace: "Boston", wantLat: "N42.3601", wantLong: "W71.0589", wantOrigin: &domain.CoordinateOrigin{Source: domain.CoordinateManual},
		},
		{
			name: "same pair records nothing", survivorPair: coords("N42.3601", "W71.0589"), mergedPair: coords("N42.3601", "W71.0589"),
			resolution: map[string]string{"birth_place": "merged"}, wantPlace: "Salem",
			wantLat: "N42.3601", wantLong: "W71.0589", wantOrigin: &domain.CoordinateOrigin{Source: domain.CoordinateManual},
			wantRecorded: []string{"birth_place"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readStore := memory.NewReadModelStore()
			handler := command.NewHandler(memory.NewEventStore(), readStore)
			ctx := context.Background()

			survivor, err := handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "John", Surname: "Doe", BirthPlace: "Boston", BirthPlaceCoordinates: tt.survivorPair})
			if err != nil {
				t.Fatal(err)
			}
			merged, err := handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Johnny", Surname: "Doe", BirthPlace: "Salem", BirthPlaceCoordinates: tt.mergedPair})
			if err != nil {
				t.Fatal(err)
			}
			result, err := handler.MergePersons(ctx, command.MergePersonsInput{
				SurvivorID: survivor.ID, MergedID: merged.ID,
				SurvivorVersion: survivor.Version, MergedVersion: merged.Version,
				FieldResolution: tt.resolution,
			})
			if err != nil {
				t.Fatalf("MergePersons: %v", err)
			}
			if !slices.Equal(result.Summary.FieldsUpdated, tt.wantRecorded) {
				t.Errorf("FieldsUpdated = %v, want %v", result.Summary.FieldsUpdated, tt.wantRecorded)
			}
			got, _ := readStore.GetPerson(ctx, domain.MainBranchID, survivor.ID)
			text := func(p *string) string {
				if p == nil {
					return ""
				}
				return *p
			}
			if got.BirthPlace != tt.wantPlace || text(got.BirthPlaceLat) != tt.wantLat || text(got.BirthPlaceLong) != tt.wantLong {
				t.Errorf("birth = %q at %q %q, want %q at %q %q", got.BirthPlace, text(got.BirthPlaceLat), text(got.BirthPlaceLong), tt.wantPlace, tt.wantLat, tt.wantLong)
			}
			if (got.BirthPlaceOrigin == nil) != (tt.wantOrigin == nil) || (got.BirthPlaceOrigin != nil && *got.BirthPlaceOrigin != *tt.wantOrigin) {
				t.Errorf("birth origin = %v, want %v", got.BirthPlaceOrigin, tt.wantOrigin)
			}
		})
	}
}

// The origin goes with the pair it describes, and the death pair follows the
// death place the same way (#1799).
func TestMergePersons_DeathCoordinatesCarryTheirOrigin(t *testing.T) {
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)
	ctx := context.Background()
	geocoded := domain.CoordinateOrigin{Source: domain.CoordinateGeocoded, Provider: "Nominatim", Precision: domain.MatchPartial}

	survivor, err := handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "John", Surname: "Doe", DeathPlace: "Boston",
		DeathPlaceCoordinates: command.CoordinatesPatch{Latitude: command.Set("N42.3601"), Longitude: command.Set("W71.0589")}})
	if err != nil {
		t.Fatal(err)
	}
	merged, err := handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Johnny", Surname: "Doe", DeathPlace: "Salem",
		DeathPlaceCoordinates: command.CoordinatesPatch{Latitude: command.Set("N42.5195"), Longitude: command.Set("W70.8967"), Origin: &geocoded}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handler.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID: survivor.ID, MergedID: merged.ID,
		SurvivorVersion: survivor.Version, MergedVersion: merged.Version,
		FieldResolution: map[string]string{"death_place": "merged"},
	}); err != nil {
		t.Fatalf("MergePersons: %v", err)
	}
	got, _ := readStore.GetPerson(ctx, domain.MainBranchID, survivor.ID)
	if got.DeathPlace != "Salem" || got.DeathPlaceLat == nil || *got.DeathPlaceLat != "N42.5195" || got.DeathPlaceLong == nil || *got.DeathPlaceLong != "W70.8967" {
		t.Errorf("death = %q at %v %v, want Salem at N42.5195 W70.8967", got.DeathPlace, got.DeathPlaceLat, got.DeathPlaceLong)
	}
	if got.DeathPlaceOrigin == nil || *got.DeathPlaceOrigin != geocoded {
		t.Errorf("death origin = %v, want %v", got.DeathPlaceOrigin, geocoded)
	}
}

func TestMergePersons_KeepsTheMoreProtectivePrivacyMarks(t *testing.T) {
	yes, no := true, false
	tests := []struct {
		name                             string
		survivorPrivate, mergedPrivate   *bool
		survivorOverride, mergedOverride string
		wantPrivate                      bool
		wantOverride                     domain.LivingOverride
		wantRecorded                     []string
	}{
		{name: "merged private", mergedPrivate: &yes, wantPrivate: true, wantRecorded: []string{"restriction"}},
		{name: "survivor private", survivorPrivate: &yes, mergedPrivate: &no, wantPrivate: true},
		{name: "merged held living", survivorOverride: "deceased", mergedOverride: "living", wantOverride: domain.LivingOverrideLiving, wantRecorded: []string{"living_override"}},
		{name: "auto over deceased", survivorOverride: "deceased", wantOverride: domain.LivingOverrideAuto, wantRecorded: []string{"living_override"}},
		{name: "survivor deceased kept over merged deceased", survivorOverride: "deceased", mergedOverride: "deceased", wantOverride: domain.LivingOverrideDeceased},
		{name: "survivor living kept", survivorOverride: "living", mergedOverride: "deceased", wantOverride: domain.LivingOverrideLiving},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventStore := memory.NewEventStore()
			readStore := memory.NewReadModelStore()
			handler := command.NewHandler(eventStore, readStore)
			ctx := context.Background()
			create := func(given string, private *bool, override string) (uuid.UUID, int64) {
				p, err := handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: given, Surname: "Doe"})
				if err != nil {
					t.Fatalf("CreatePerson: %v", err)
				}
				if private == nil && override == "" {
					return p.ID, p.Version
				}
				input := command.UpdatePersonInput{ID: p.ID, Private: private, Version: p.Version}
				if override != "" {
					input.LivingOverride = command.Set(override)
				}
				updated, err := handler.UpdatePerson(ctx, input)
				if err != nil {
					t.Fatalf("UpdatePerson: %v", err)
				}
				return p.ID, updated.Version
			}
			survivorID, survivorVersion := create("John", tt.survivorPrivate, tt.survivorOverride)
			mergedID, mergedVersion := create("Jon", tt.mergedPrivate, tt.mergedOverride)

			result, err := handler.MergePersons(ctx, command.MergePersonsInput{
				SurvivorID: survivorID, MergedID: mergedID,
				SurvivorVersion: survivorVersion, MergedVersion: mergedVersion,
			})
			if err != nil {
				t.Fatalf("MergePersons: %v", err)
			}
			person, _ := readStore.GetPerson(ctx, domain.MainBranchID, survivorID)
			if got := person.Restriction.IsPrivate(); got != tt.wantPrivate {
				t.Errorf("private = %v, want %v (restriction %q)", got, tt.wantPrivate, person.Restriction)
			}
			if person.LivingOverride != tt.wantOverride {
				t.Errorf("living override = %q, want %q", person.LivingOverride, tt.wantOverride)
			}
			for _, field := range tt.wantRecorded {
				if !slices.Contains(result.Summary.FieldsUpdated, field) {
					t.Errorf("FieldsUpdated = %v, want it to hold %s", result.Summary.FieldsUpdated, field)
				}
			}
		})
	}
}

// A rollback across a person merge restores the privacy marks the merge
// resolved, but never lowers them: back to just after the merge a later unmark
// is undone, and back to before it the survivor keeps the marks it took from
// the merged person, whose transfers the rollback does not undo (ADR-008).
func TestRollbackPerson_AcrossAMergeRestoresThePrivacyMarks(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()
	yes, no := true, false

	survivor, err := handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "John", Surname: "Doe"})
	if err != nil {
		t.Fatalf("CreatePerson: %v", err)
	}
	merged, err := handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Jon", Surname: "Doe"})
	if err != nil {
		t.Fatalf("CreatePerson: %v", err)
	}
	marked, err := handler.UpdatePerson(ctx, command.UpdatePersonInput{ID: merged.ID, Version: merged.Version,
		Private: &yes, LivingOverride: command.Set(string(domain.LivingOverrideLiving))})
	if err != nil {
		t.Fatalf("UpdatePerson: %v", err)
	}
	result, err := handler.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID: survivor.ID, MergedID: merged.ID,
		SurvivorVersion: survivor.Version, MergedVersion: marked.Version,
	})
	if err != nil {
		t.Fatalf("MergePersons: %v", err)
	}
	for _, field := range []string{"restriction", "living_override"} {
		if !slices.Contains(result.Summary.FieldsUpdated, field) {
			t.Fatalf("FieldsUpdated = %v, want it to hold %s", result.Summary.FieldsUpdated, field)
		}
	}
	mergeVersion := result.Version
	unmarked, err := handler.UpdatePerson(ctx, command.UpdatePersonInput{ID: survivor.ID, Version: mergeVersion,
		Private: &no, LivingOverride: command.Set(string(domain.LivingOverrideAuto))})
	if err != nil {
		t.Fatalf("UpdatePerson unmark: %v", err)
	}

	marks := func() (bool, domain.LivingOverride) {
		t.Helper()
		p, err := readStore.GetPerson(ctx, domain.MainBranchID, survivor.ID)
		if err != nil || p == nil {
			t.Fatalf("GetPerson: %v, %v", p, err)
		}
		return p.Restriction.IsPrivate(), p.LivingOverride
	}
	if private, override := marks(); private || override != domain.LivingOverrideAuto {
		t.Fatalf("after unmarking: private %v, override %q", private, override)
	}

	if _, err := handler.RollbackPerson(ctx, survivor.ID, mergeVersion); err != nil {
		t.Fatalf("RollbackPerson to the merge (v%d): %v", mergeVersion, err)
	}
	if private, override := marks(); !private || override != domain.LivingOverrideLiving {
		t.Errorf("rolled back to the merge: private %v, override %q; want private and living", private, override)
	}

	if unmarked.Version <= mergeVersion {
		t.Fatalf("unmark version %d not after the merge's %d", unmarked.Version, mergeVersion)
	}
	if _, err := handler.RollbackPerson(ctx, survivor.ID, survivor.Version); err != nil {
		t.Fatalf("RollbackPerson to before the merge (v%d): %v", survivor.Version, err)
	}
	if private, override := marks(); !private || override != domain.LivingOverrideLiving {
		t.Errorf("rolled back to before the merge: private %v, override %q; want private and living", private, override)
	}
	p, err := readStore.GetPerson(ctx, domain.MainBranchID, survivor.ID)
	if err != nil || p == nil || p.GivenName != "John" {
		t.Errorf("rolled back to before the merge: given name %v (%v), want John", p, err)
	}
}
