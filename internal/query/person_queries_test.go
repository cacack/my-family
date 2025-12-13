package query_test

import (
	"context"
	"testing"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestListPersons(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewPersonService(readStore)
	ctx := context.Background()

	// Create some persons
	for _, name := range []string{"Alice", "Bob", "Charlie"} {
		_, err := handler.CreatePerson(ctx, command.CreatePersonInput{
			GivenName: name,
			Surname:   "Smith",
		})
		if err != nil {
			t.Fatalf("CreatePerson failed: %v", err)
		}
	}

	// List persons
	result, err := service.ListPersons(ctx, query.ListPersonsInput{})
	if err != nil {
		t.Fatalf("ListPersons failed: %v", err)
	}

	if result.Total != 3 {
		t.Errorf("Total = %d, want 3", result.Total)
	}
	if len(result.Items) != 3 {
		t.Errorf("Items count = %d, want 3", len(result.Items))
	}
}

func TestListPersons_Pagination(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewPersonService(readStore)
	ctx := context.Background()

	// Create 5 persons
	for i := 0; i < 5; i++ {
		_, err := handler.CreatePerson(ctx, command.CreatePersonInput{
			GivenName: "Person",
			Surname:   "Test",
		})
		if err != nil {
			t.Fatalf("CreatePerson failed: %v", err)
		}
	}

	// Get first page
	result, err := service.ListPersons(ctx, query.ListPersonsInput{
		Limit:  2,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("ListPersons failed: %v", err)
	}

	if result.Total != 5 {
		t.Errorf("Total = %d, want 5", result.Total)
	}
	if len(result.Items) != 2 {
		t.Errorf("Items count = %d, want 2", len(result.Items))
	}
	if result.Limit != 2 {
		t.Errorf("Limit = %d, want 2", result.Limit)
	}

	// Get second page
	result2, err := service.ListPersons(ctx, query.ListPersonsInput{
		Limit:  2,
		Offset: 2,
	})
	if err != nil {
		t.Fatalf("ListPersons failed: %v", err)
	}

	if len(result2.Items) != 2 {
		t.Errorf("Page 2 Items count = %d, want 2", len(result2.Items))
	}
	if result2.Offset != 2 {
		t.Errorf("Offset = %d, want 2", result2.Offset)
	}
}

func TestGetPerson(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewPersonService(readStore)
	ctx := context.Background()

	// Create a person
	createResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName:  "John",
		Surname:    "Doe",
		Gender:     "male",
		BirthDate:  "15 MAR 1990",
		BirthPlace: "New York, NY",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Get the person
	person, err := service.GetPerson(ctx, createResult.ID)
	if err != nil {
		t.Fatalf("GetPerson failed: %v", err)
	}

	if person.GivenName != "John" {
		t.Errorf("GivenName = %s, want John", person.GivenName)
	}
	if person.Surname != "Doe" {
		t.Errorf("Surname = %s, want Doe", person.Surname)
	}
	if person.Gender == nil || *person.Gender != "male" {
		t.Errorf("Gender = %v, want male", person.Gender)
	}
	if person.BirthPlace == nil || *person.BirthPlace != "New York, NY" {
		t.Errorf("BirthPlace = %v, want New York, NY", person.BirthPlace)
	}
	if person.BirthDate == nil || person.BirthDate.Raw != "15 MAR 1990" {
		t.Errorf("BirthDate = %v, want 15 MAR 1990", person.BirthDate)
	}
}

func TestGetPerson_NotFound(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewPersonService(readStore)
	ctx := context.Background()

	_, err := service.GetPerson(ctx, [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	if err != query.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestSearchPersons(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewPersonService(readStore)
	ctx := context.Background()

	// Create some persons
	_, _ = handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Smith",
	})
	_, _ = handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Jane",
		Surname:   "Smith",
	})
	_, _ = handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Bob",
		Surname:   "Johnson",
	})

	// Search for "Smith"
	result, err := service.SearchPersons(ctx, query.SearchPersonsInput{
		Query: "Smith",
	})
	if err != nil {
		t.Fatalf("SearchPersons failed: %v", err)
	}

	if result.Total != 2 {
		t.Errorf("Total = %d, want 2", result.Total)
	}
	if result.Query != "Smith" {
		t.Errorf("Query = %s, want Smith", result.Query)
	}
}

func TestSearchPersons_NoResults(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewPersonService(readStore)
	ctx := context.Background()

	_, _ = handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Smith",
	})

	result, err := service.SearchPersons(ctx, query.SearchPersonsInput{
		Query: "NonExistent",
	})
	if err != nil {
		t.Fatalf("SearchPersons failed: %v", err)
	}

	if result.Total != 0 {
		t.Errorf("Total = %d, want 0", result.Total)
	}
}
