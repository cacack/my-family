package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository/memory"
)

// TestListLDSOrdinances tests listing LDS ordinances with pagination.
func TestListLDSOrdinances(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewLDSOrdinanceService(readStore)
	ctx := context.Background()

	// Create a person for ordinances
	personResult, _ := cmdHandler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	})

	// Create test ordinances
	ordinanceTypes := []domain.LDSOrdinanceType{
		domain.LDSBaptism,
		domain.LDSConfirmation,
		domain.LDSEndowment,
	}
	for _, ordType := range ordinanceTypes {
		_, err := cmdHandler.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{
			Type:     ordType,
			PersonID: &personResult.ID,
		})
		if err != nil {
			t.Fatalf("Failed to create LDS ordinance: %v", err)
		}
	}

	tests := []struct {
		name      string
		input     query.ListLDSOrdinancesInput
		wantCount int
		wantTotal int
	}{
		{
			name: "list all",
			input: query.ListLDSOrdinancesInput{
				Limit: 10,
			},
			wantCount: 3,
			wantTotal: 3,
		},
		{
			name: "with pagination",
			input: query.ListLDSOrdinancesInput{
				Limit:  2,
				Offset: 0,
			},
			wantCount: 2,
			wantTotal: 3,
		},
		{
			name: "second page",
			input: query.ListLDSOrdinancesInput{
				Limit:  2,
				Offset: 2,
			},
			wantCount: 1,
			wantTotal: 3,
		},
		{
			name: "default limit when not specified",
			input: query.ListLDSOrdinancesInput{
				Limit: 0, // should default to 20
			},
			wantCount: 3,
			wantTotal: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := queryService.ListLDSOrdinances(ctx, tt.input)
			if err != nil {
				t.Fatalf("ListLDSOrdinances failed: %v", err)
			}

			if len(result.LDSOrdinances) != tt.wantCount {
				t.Errorf("Got %d ordinances, want %d", len(result.LDSOrdinances), tt.wantCount)
			}

			if result.Total != tt.wantTotal {
				t.Errorf("Total = %d, want %d", result.Total, tt.wantTotal)
			}
		})
	}
}

// TestListLDSOrdinances_LimitEnforcement tests that limits are enforced.
func TestListLDSOrdinances_LimitEnforcement(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewLDSOrdinanceService(readStore)
	ctx := context.Background()

	// Create a person
	personResult, _ := cmdHandler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	})

	// Create 5 ordinances
	for i := 0; i < 5; i++ {
		_, err := cmdHandler.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{
			Type:     domain.LDSBaptism,
			PersonID: &personResult.ID,
		})
		if err != nil {
			t.Fatalf("Failed to create LDS ordinance: %v", err)
		}
	}

	// Request with limit > 100 should be capped at 100
	result, err := queryService.ListLDSOrdinances(ctx, query.ListLDSOrdinancesInput{
		Limit: 150, // Should be capped at 100
	})
	if err != nil {
		t.Fatalf("ListLDSOrdinances failed: %v", err)
	}

	// Since we only have 5 ordinances, we should get 5
	if len(result.LDSOrdinances) != 5 {
		t.Errorf("Got %d ordinances, want 5", len(result.LDSOrdinances))
	}

	// But the limit should be set to 100
	if result.Limit != 100 {
		t.Errorf("Limit = %d, want 100", result.Limit)
	}
}

// TestListLDSOrdinances_SortOrder tests sorting.
func TestListLDSOrdinances_SortOrder(t *testing.T) {
	readStore := memory.NewReadModelStore()
	queryService := query.NewLDSOrdinanceService(readStore)
	ctx := context.Background()

	// Test default order (desc)
	result, err := queryService.ListLDSOrdinances(ctx, query.ListLDSOrdinancesInput{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListLDSOrdinances failed: %v", err)
	}

	// Just verify it doesn't error with empty results
	if result == nil {
		t.Error("Result should not be nil")
	}

	// Test explicit ascending order
	result, err = queryService.ListLDSOrdinances(ctx, query.ListLDSOrdinancesInput{
		Limit:     10,
		SortOrder: "asc",
	})
	if err != nil {
		t.Fatalf("ListLDSOrdinances with asc order failed: %v", err)
	}
	if result == nil {
		t.Error("Result should not be nil")
	}
}

// TestGetLDSOrdinance tests getting an LDS ordinance by ID.
func TestGetLDSOrdinance(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewLDSOrdinanceService(readStore)
	ctx := context.Background()

	// Create a person
	personResult, _ := cmdHandler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	})

	// Create ordinance
	createResult, err := cmdHandler.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{
		Type:     domain.LDSBaptism,
		PersonID: &personResult.ID,
		Temple:   "SLAKE",
		Status:   "COMPLETED",
		Place:    "Salt Lake Temple",
		Date:     "15 JAN 1900",
	})
	if err != nil {
		t.Fatalf("CreateLDSOrdinance failed: %v", err)
	}

	// Get ordinance
	result, err := queryService.GetLDSOrdinance(ctx, createResult.ID)
	if err != nil {
		t.Fatalf("GetLDSOrdinance failed: %v", err)
	}

	if result.ID != createResult.ID {
		t.Errorf("ID = %v, want %v", result.ID, createResult.ID)
	}
	if result.Type != domain.LDSBaptism {
		t.Errorf("Type = %s, want BAPL", result.Type)
	}
	if result.TypeLabel != "Baptism (LDS)" {
		t.Errorf("TypeLabel = %s, want 'Baptism (LDS)'", result.TypeLabel)
	}
	if result.Temple != "SLAKE" {
		t.Errorf("Temple = %s, want SLAKE", result.Temple)
	}
	if result.Status != "COMPLETED" {
		t.Errorf("Status = %s, want COMPLETED", result.Status)
	}
	if result.Place != "Salt Lake Temple" {
		t.Errorf("Place = %s, want Salt Lake Temple", result.Place)
	}
	if result.Version != 1 {
		t.Errorf("Version = %d, want 1", result.Version)
	}
	if result.PersonID == nil || *result.PersonID != personResult.ID {
		t.Errorf("PersonID = %v, want %v", result.PersonID, personResult.ID)
	}
}

// TestGetLDSOrdinance_NotFound tests getting a non-existent ordinance.
func TestGetLDSOrdinance_NotFound(t *testing.T) {
	readStore := memory.NewReadModelStore()
	queryService := query.NewLDSOrdinanceService(readStore)
	ctx := context.Background()

	_, err := queryService.GetLDSOrdinance(ctx, uuid.New())
	if err != query.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

// TestListLDSOrdinancesForPerson tests listing ordinances for a specific person.
func TestListLDSOrdinancesForPerson(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewLDSOrdinanceService(readStore)
	ctx := context.Background()

	// Create two people
	person1, _ := cmdHandler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	})
	person2, _ := cmdHandler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Jane",
		Surname:   "Doe",
		Gender:    "female",
	})

	// Create ordinances for person 1
	for _, ordType := range []domain.LDSOrdinanceType{domain.LDSBaptism, domain.LDSConfirmation} {
		_, _ = cmdHandler.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{
			Type:     ordType,
			PersonID: &person1.ID,
		})
	}

	// Create one ordinance for person 2
	_, _ = cmdHandler.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{
		Type:     domain.LDSBaptism,
		PersonID: &person2.ID,
	})

	// List for person 1
	result, err := queryService.ListLDSOrdinancesForPerson(ctx, person1.ID)
	if err != nil {
		t.Fatalf("ListLDSOrdinancesForPerson failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Got %d ordinances, want 2", len(result))
	}

	// List for person 2
	result, err = queryService.ListLDSOrdinancesForPerson(ctx, person2.ID)
	if err != nil {
		t.Fatalf("ListLDSOrdinancesForPerson failed: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("Got %d ordinances, want 1", len(result))
	}
}

// TestListLDSOrdinancesForPerson_NotFound tests listing for non-existent person.
func TestListLDSOrdinancesForPerson_NotFound(t *testing.T) {
	readStore := memory.NewReadModelStore()
	queryService := query.NewLDSOrdinanceService(readStore)
	ctx := context.Background()

	_, err := queryService.ListLDSOrdinancesForPerson(ctx, uuid.New())
	if err != query.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

// TestListLDSOrdinancesForFamily tests listing ordinances for a specific family.
func TestListLDSOrdinancesForFamily(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewLDSOrdinanceService(readStore)
	ctx := context.Background()

	// Create persons for families
	person1, _ := cmdHandler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	})
	person2, _ := cmdHandler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Jane",
		Surname:   "Doe",
		Gender:    "female",
	})
	person3, _ := cmdHandler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Bob",
		Surname:   "Smith",
		Gender:    "male",
	})
	person4, _ := cmdHandler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Alice",
		Surname:   "Smith",
		Gender:    "female",
	})

	// Create two families
	family1, _ := cmdHandler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &person1.ID,
		Partner2ID: &person2.ID,
	})
	family2, _ := cmdHandler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &person3.ID,
		Partner2ID: &person4.ID,
	})

	// Create spouse sealing for family 1
	_, _ = cmdHandler.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{
		Type:     domain.LDSSealingSpouse,
		FamilyID: &family1.ID,
	})

	// Create spouse sealing for family 2
	_, _ = cmdHandler.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{
		Type:     domain.LDSSealingSpouse,
		FamilyID: &family2.ID,
	})

	// List for family 1
	result, err := queryService.ListLDSOrdinancesForFamily(ctx, family1.ID)
	if err != nil {
		t.Fatalf("ListLDSOrdinancesForFamily failed: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("Got %d ordinances, want 1", len(result))
	}
}

// TestListLDSOrdinancesForFamily_NotFound tests listing for non-existent family.
func TestListLDSOrdinancesForFamily_NotFound(t *testing.T) {
	readStore := memory.NewReadModelStore()
	queryService := query.NewLDSOrdinanceService(readStore)
	ctx := context.Background()

	_, err := queryService.ListLDSOrdinancesForFamily(ctx, uuid.New())
	if err != query.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

// TestListLDSOrdinances_Empty tests listing with no ordinances.
func TestListLDSOrdinances_Empty(t *testing.T) {
	readStore := memory.NewReadModelStore()
	queryService := query.NewLDSOrdinanceService(readStore)
	ctx := context.Background()

	result, err := queryService.ListLDSOrdinances(ctx, query.ListLDSOrdinancesInput{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListLDSOrdinances failed: %v", err)
	}

	if len(result.LDSOrdinances) != 0 {
		t.Errorf("Got %d ordinances, want 0", len(result.LDSOrdinances))
	}
	if result.Total != 0 {
		t.Errorf("Total = %d, want 0", result.Total)
	}
}

// TestGetLDSOrdinance_WithDate tests date parsing in query result.
func TestGetLDSOrdinance_WithDate(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewLDSOrdinanceService(readStore)
	ctx := context.Background()

	// Create a person
	personResult, _ := cmdHandler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	})

	// Create ordinance with date
	createResult, _ := cmdHandler.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{
		Type:     domain.LDSBaptism,
		PersonID: &personResult.ID,
		Date:     "15 JAN 1900",
	})

	// Get ordinance
	result, err := queryService.GetLDSOrdinance(ctx, createResult.ID)
	if err != nil {
		t.Fatalf("GetLDSOrdinance failed: %v", err)
	}

	if result.Date == nil {
		t.Error("Date should not be nil")
	}
}

// TestGetLDSOrdinance_NoDate tests ordinance without date.
func TestGetLDSOrdinance_NoDate(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewLDSOrdinanceService(readStore)
	ctx := context.Background()

	// Create a person
	personResult, _ := cmdHandler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	})

	// Create ordinance without date
	createResult, _ := cmdHandler.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{
		Type:     domain.LDSBaptism,
		PersonID: &personResult.ID,
	})

	// Get ordinance
	result, err := queryService.GetLDSOrdinance(ctx, createResult.ID)
	if err != nil {
		t.Fatalf("GetLDSOrdinance failed: %v", err)
	}

	if result.Date != nil {
		t.Errorf("Date should be nil, got %v", result.Date)
	}
}
