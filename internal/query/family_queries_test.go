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
