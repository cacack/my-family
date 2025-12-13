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
