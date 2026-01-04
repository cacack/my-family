package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestListFamilies(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewFamilyService(readStore)
	ctx := context.Background()

	// Create some families
	p1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	p2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Jane",
		Surname:   "Doe",
	})
	p3, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Bob",
		Surname:   "Smith",
	})

	_, _ = handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &p1.ID,
		Partner2ID: &p2.ID,
	})
	_, _ = handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &p3.ID,
	})

	// List families
	result, err := service.ListFamilies(ctx, query.ListFamiliesInput{})
	if err != nil {
		t.Fatalf("ListFamilies failed: %v", err)
	}

	if result.Total != 2 {
		t.Errorf("Total = %d, want 2", result.Total)
	}
	if len(result.Items) != 2 {
		t.Errorf("Items count = %d, want 2", len(result.Items))
	}
}

func TestGetFamily(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewFamilyService(readStore)
	ctx := context.Background()

	// Create family with partners
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
	familyResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID:       &p1.ID,
		Partner2ID:       &p2.ID,
		RelationshipType: "marriage",
		MarriageDate:     "15 JUN 1980",
	})

	// Get family
	family, err := service.GetFamily(ctx, familyResult.ID)
	if err != nil {
		t.Fatalf("GetFamily failed: %v", err)
	}

	if family.Partner1ID == nil || *family.Partner1ID != p1.ID {
		t.Error("Partner1ID mismatch")
	}
	if family.Partner2ID == nil || *family.Partner2ID != p2.ID {
		t.Error("Partner2ID mismatch")
	}
	if family.RelationshipType == nil || *family.RelationshipType != "marriage" {
		t.Errorf("RelationshipType = %v, want marriage", family.RelationshipType)
	}
	if family.MarriageDate == nil || family.MarriageDate.Raw != "15 JUN 1980" {
		t.Errorf("MarriageDate = %v, want 15 JUN 1980", family.MarriageDate)
	}
}

func TestGetFamily_WithChildren(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewFamilyService(readStore)
	ctx := context.Background()

	// Create family with children
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	child1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Alice",
		Surname:   "Doe",
	})
	child2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Bob",
		Surname:   "Doe",
	})

	familyResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent.ID,
	})
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID:     familyResult.ID,
		ChildID:      child1.ID,
		RelationType: "biological",
	})
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID:     familyResult.ID,
		ChildID:      child2.ID,
		RelationType: "adopted",
	})

	// Get family with children
	family, err := service.GetFamily(ctx, familyResult.ID)
	if err != nil {
		t.Fatalf("GetFamily failed: %v", err)
	}

	if family.ChildCount != 2 {
		t.Errorf("ChildCount = %d, want 2", family.ChildCount)
	}
	if len(family.Children) != 2 {
		t.Errorf("Children count = %d, want 2", len(family.Children))
	}
}

func TestGetFamily_NotFound(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewFamilyService(readStore)
	ctx := context.Background()

	_, err := service.GetFamily(ctx, uuid.New())
	if err != query.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestGetFamiliesForPerson(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewFamilyService(readStore)
	ctx := context.Background()

	// Create person with multiple families (remarriage scenario)
	p1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	p2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Jane",
		Surname:   "Doe",
	})
	p3, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Mary",
		Surname:   "Smith",
	})

	_, _ = handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &p1.ID,
		Partner2ID: &p2.ID,
	})
	_, _ = handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &p1.ID,
		Partner2ID: &p3.ID,
	})

	// Get families for person
	families, err := service.GetFamiliesForPerson(ctx, p1.ID)
	if err != nil {
		t.Fatalf("GetFamiliesForPerson failed: %v", err)
	}

	if len(families) != 2 {
		t.Errorf("Families count = %d, want 2", len(families))
	}
}

func TestListFamilies_LimitConstraints(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewFamilyService(readStore)
	ctx := context.Background()

	// Create some test data
	p1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	_, _ = handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &p1.ID,
	})

	tests := []struct {
		name          string
		input         query.ListFamiliesInput
		expectedLimit int
	}{
		{
			name:          "limit over max gets capped to 100",
			input:         query.ListFamiliesInput{Limit: 200},
			expectedLimit: 100,
		},
		{
			name:          "negative limit defaults to 20",
			input:         query.ListFamiliesInput{Limit: -1},
			expectedLimit: 20,
		},
		{
			name:          "zero limit defaults to 20",
			input:         query.ListFamiliesInput{Limit: 0},
			expectedLimit: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.ListFamilies(ctx, tt.input)
			if err != nil {
				t.Fatalf("ListFamilies failed: %v", err)
			}
			if result.Limit != tt.expectedLimit {
				t.Errorf("Limit = %d, want %d", result.Limit, tt.expectedLimit)
			}
		})
	}
}

func TestListFamilies_Pagination(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewFamilyService(readStore)
	ctx := context.Background()

	// Create 5 families
	for i := 0; i < 5; i++ {
		p, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
			GivenName: "Person",
			Surname:   "Test",
		})
		_, _ = handler.CreateFamily(ctx, command.CreateFamilyInput{
			Partner1ID: &p.ID,
		})
	}

	// Get first page
	result, err := service.ListFamilies(ctx, query.ListFamiliesInput{
		Limit:  2,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("ListFamilies failed: %v", err)
	}

	if result.Total != 5 {
		t.Errorf("Total = %d, want 5", result.Total)
	}
	if len(result.Items) != 2 {
		t.Errorf("Items count = %d, want 2", len(result.Items))
	}
	if result.Offset != 0 {
		t.Errorf("Offset = %d, want 0", result.Offset)
	}
}

func TestListFamilies_OffsetBeyondResults(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewFamilyService(readStore)
	ctx := context.Background()

	// Create only 2 families
	for i := 0; i < 2; i++ {
		p, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
			GivenName: "Person",
			Surname:   "Test",
		})
		_, _ = handler.CreateFamily(ctx, command.CreateFamilyInput{
			Partner1ID: &p.ID,
		})
	}

	// Request offset beyond results
	result, err := service.ListFamilies(ctx, query.ListFamiliesInput{
		Limit:  10,
		Offset: 100,
	})
	if err != nil {
		t.Fatalf("ListFamilies failed: %v", err)
	}

	if result.Total != 2 {
		t.Errorf("Total = %d, want 2", result.Total)
	}
	if len(result.Items) != 0 {
		t.Errorf("Items count = %d, want 0 (offset beyond results)", len(result.Items))
	}
	if result.Offset != 100 {
		t.Errorf("Offset = %d, want 100", result.Offset)
	}
}

func TestGetFamily_AllOptionalFields(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewFamilyService(readStore)
	ctx := context.Background()

	// Create family with all optional fields
	p1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	p2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Jane",
		Surname:   "Smith",
	})
	familyResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID:       &p1.ID,
		Partner2ID:       &p2.ID,
		RelationshipType: "marriage",
		MarriageDate:     "15 JUN 1980",
		MarriagePlace:    "Boston, MA",
	})

	// Get family to verify conversion of all fields
	family, err := service.GetFamily(ctx, familyResult.ID)
	if err != nil {
		t.Fatalf("GetFamily failed: %v", err)
	}

	if family.Partner1Name == nil {
		t.Error("Partner1Name should be set")
	}
	if family.Partner2Name == nil {
		t.Error("Partner2Name should be set")
	}
	if family.RelationshipType == nil {
		t.Error("RelationshipType should be set")
	}
	if family.MarriageDate == nil {
		t.Error("MarriageDate should be set")
	}
	if family.MarriagePlace == nil {
		t.Error("MarriagePlace should be set")
	}
}

func TestGetFamiliesForPerson_EmptyResult(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewFamilyService(readStore)
	ctx := context.Background()

	// Create person with no families
	p, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Lonely",
		Surname:   "Person",
	})

	families, err := service.GetFamiliesForPerson(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetFamiliesForPerson failed: %v", err)
	}

	if len(families) != 0 {
		t.Errorf("Expected no families, got %d", len(families))
	}
}

// Group Sheet tests

func TestGetGroupSheet_Basic(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewFamilyService(readStore)
	ctx := context.Background()

	// Create husband
	husband, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName:  "John",
		Surname:    "Doe",
		Gender:     "male",
		BirthDate:  "1 JAN 1950",
		BirthPlace: "Boston, MA",
		DeathDate:  "15 DEC 2020",
		DeathPlace: "New York, NY",
	})

	// Create wife
	wife, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName:  "Jane",
		Surname:    "Smith",
		Gender:     "female",
		BirthDate:  "15 MAR 1952",
		BirthPlace: "Chicago, IL",
	})

	// Create family with marriage
	familyResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID:       &husband.ID,
		Partner2ID:       &wife.ID,
		RelationshipType: "marriage",
		MarriageDate:     "20 JUN 1975",
		MarriagePlace:    "Boston, MA",
	})

	// Get group sheet
	gs, err := service.GetGroupSheet(ctx, familyResult.ID)
	if err != nil {
		t.Fatalf("GetGroupSheet failed: %v", err)
	}

	// Verify husband
	if gs.Husband == nil {
		t.Fatal("Husband should not be nil")
	}
	if gs.Husband.GivenName != "John" {
		t.Errorf("Husband GivenName = %q, want 'John'", gs.Husband.GivenName)
	}
	if gs.Husband.Surname != "Doe" {
		t.Errorf("Husband Surname = %q, want 'Doe'", gs.Husband.Surname)
	}
	if gs.Husband.Birth == nil {
		t.Error("Husband Birth should not be nil")
	} else {
		if gs.Husband.Birth.Date != "1 JAN 1950" {
			t.Errorf("Husband Birth Date = %q, want '1 JAN 1950'", gs.Husband.Birth.Date)
		}
		if gs.Husband.Birth.Place != "Boston, MA" {
			t.Errorf("Husband Birth Place = %q, want 'Boston, MA'", gs.Husband.Birth.Place)
		}
	}
	if gs.Husband.Death == nil {
		t.Error("Husband Death should not be nil")
	}

	// Verify wife
	if gs.Wife == nil {
		t.Fatal("Wife should not be nil")
	}
	if gs.Wife.GivenName != "Jane" {
		t.Errorf("Wife GivenName = %q, want 'Jane'", gs.Wife.GivenName)
	}

	// Verify marriage
	if gs.Marriage == nil {
		t.Fatal("Marriage should not be nil")
	}
	if gs.Marriage.Date != "20 JUN 1975" {
		t.Errorf("Marriage Date = %q, want '20 JUN 1975'", gs.Marriage.Date)
	}
	if gs.Marriage.Place != "Boston, MA" {
		t.Errorf("Marriage Place = %q, want 'Boston, MA'", gs.Marriage.Place)
	}
}

func TestGetGroupSheet_WithChildren(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewFamilyService(readStore)
	ctx := context.Background()

	// Create parents
	father, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	})
	mother, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Jane",
		Surname:   "Doe",
		Gender:    "female",
	})

	// Create children
	child1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName:  "Alice",
		Surname:    "Doe",
		Gender:     "female",
		BirthDate:  "1 JAN 1980",
		BirthPlace: "Boston, MA",
	})
	child2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName:  "Bob",
		Surname:    "Doe",
		Gender:     "male",
		BirthDate:  "15 MAR 1982",
		BirthPlace: "New York, NY",
	})

	// Create family
	familyResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &father.ID,
		Partner2ID: &mother.ID,
	})

	// Link children
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID:     familyResult.ID,
		ChildID:      child1.ID,
		RelationType: "biological",
	})
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID:     familyResult.ID,
		ChildID:      child2.ID,
		RelationType: "biological",
	})

	// Get group sheet
	gs, err := service.GetGroupSheet(ctx, familyResult.ID)
	if err != nil {
		t.Fatalf("GetGroupSheet failed: %v", err)
	}

	// Verify children
	if len(gs.Children) != 2 {
		t.Fatalf("Children count = %d, want 2", len(gs.Children))
	}

	// Check first child
	found := false
	for _, child := range gs.Children {
		if child.GivenName == "Alice" {
			found = true
			if child.Birth == nil {
				t.Error("Alice's Birth should not be nil")
			} else if child.Birth.Date != "1 JAN 1980" {
				t.Errorf("Alice's Birth Date = %q, want '1 JAN 1980'", child.Birth.Date)
			}
		}
	}
	if !found {
		t.Error("Child Alice not found in group sheet")
	}
}

func TestGetGroupSheet_NotFound(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewFamilyService(readStore)
	ctx := context.Background()

	_, err := service.GetGroupSheet(ctx, uuid.New())
	if err != query.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestGetGroupSheet_NoMarriageInfo(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewFamilyService(readStore)
	ctx := context.Background()

	// Create family without marriage info
	person, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	familyResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &person.ID,
	})

	gs, err := service.GetGroupSheet(ctx, familyResult.ID)
	if err != nil {
		t.Fatalf("GetGroupSheet failed: %v", err)
	}

	if gs.Marriage != nil {
		t.Error("Marriage should be nil when no marriage info")
	}
}

func TestGetGroupSheet_ChildWithSpouse(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewFamilyService(readStore)
	ctx := context.Background()

	// Create parent
	parent, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	// Create child
	child, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Alice",
		Surname:   "Doe",
	})

	// Create child's spouse
	spouse, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Bob",
		Surname:   "Smith",
	})

	// Create parent's family
	parentFamily, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &parent.ID,
	})
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID:     parentFamily.ID,
		ChildID:      child.ID,
		RelationType: "biological",
	})

	// Create child's marriage family
	_, _ = handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &child.ID,
		Partner2ID: &spouse.ID,
	})

	// Get group sheet for parent's family
	gs, err := service.GetGroupSheet(ctx, parentFamily.ID)
	if err != nil {
		t.Fatalf("GetGroupSheet failed: %v", err)
	}

	// Verify child has spouse info
	if len(gs.Children) != 1 {
		t.Fatalf("Children count = %d, want 1", len(gs.Children))
	}

	if gs.Children[0].SpouseName != "Bob Smith" {
		t.Errorf("Child SpouseName = %q, want 'Bob Smith'", gs.Children[0].SpouseName)
	}
	if gs.Children[0].SpouseID == nil {
		t.Error("Child SpouseID should not be nil")
	}
}

func TestGetGroupSheet_PersonWithParents(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewFamilyService(readStore)
	ctx := context.Background()

	// Create grandparent family
	grandfather, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Robert",
		Surname:   "Doe",
		Gender:    "male",
	})
	grandmother, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Mary",
		Surname:   "Doe",
		Gender:    "female",
	})
	grandparentFamily, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &grandfather.ID,
		Partner2ID: &grandmother.ID,
	})

	// Create child who will be a parent
	father, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	})
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID:     grandparentFamily.ID,
		ChildID:      father.ID,
		RelationType: "biological",
	})

	// Create father's spouse
	mother, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Jane",
		Surname:   "Smith",
		Gender:    "female",
	})

	// Create family
	familyResult, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &father.ID,
		Partner2ID: &mother.ID,
	})

	// Get group sheet
	gs, err := service.GetGroupSheet(ctx, familyResult.ID)
	if err != nil {
		t.Fatalf("GetGroupSheet failed: %v", err)
	}

	// Verify husband has parent info
	if gs.Husband == nil {
		t.Fatal("Husband should not be nil")
	}
	if gs.Husband.FatherName != "Robert Doe" {
		t.Errorf("Husband FatherName = %q, want 'Robert Doe'", gs.Husband.FatherName)
	}
	if gs.Husband.MotherName != "Mary Doe" {
		t.Errorf("Husband MotherName = %q, want 'Mary Doe'", gs.Husband.MotherName)
	}
	if gs.Husband.FatherID == nil {
		t.Error("Husband FatherID should not be nil")
	}
	if gs.Husband.MotherID == nil {
		t.Error("Husband MotherID should not be nil")
	}
}
