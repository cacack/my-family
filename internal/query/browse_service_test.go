package query_test

import (
	"context"
	"testing"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestGetSurnameIndex(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewBrowseService(readStore)
	ctx := context.Background()

	// Create test persons with various surnames
	testData := []struct {
		givenName string
		surname   string
	}{
		{"John", "Smith"},
		{"Jane", "Smith"},
		{"Bob", "Smith"},
		{"Alice", "Anderson"},
		{"Charlie", "Brown"},
	}

	for _, td := range testData {
		_, err := handler.CreatePerson(ctx, command.CreatePersonInput{
			GivenName: td.givenName,
			Surname:   td.surname,
		})
		if err != nil {
			t.Fatalf("CreatePerson failed: %v", err)
		}
	}

	// Get full surname index
	result, err := service.GetSurnameIndex(ctx, query.GetSurnameIndexInput{})
	if err != nil {
		t.Fatalf("GetSurnameIndex failed: %v", err)
	}

	if result.Total != 3 { // Smith, Anderson, Brown
		t.Errorf("Total = %d, want 3", result.Total)
	}

	// Verify letter counts
	if len(result.LetterCounts) == 0 {
		t.Error("Expected letter counts to be populated")
	}

	// Check that we have counts for A, B, S
	letterMap := make(map[string]int)
	for _, lc := range result.LetterCounts {
		letterMap[lc.Letter] = lc.Count
	}

	if letterMap["A"] != 1 {
		t.Errorf("Letter A count = %d, want 1", letterMap["A"])
	}
	if letterMap["B"] != 1 {
		t.Errorf("Letter B count = %d, want 1", letterMap["B"])
	}
	if letterMap["S"] != 1 {
		t.Errorf("Letter S count = %d, want 1", letterMap["S"])
	}
}

func TestGetSurnamesByLetter(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewBrowseService(readStore)
	ctx := context.Background()

	// Create persons with surnames starting with S
	surnames := []string{"Smith", "Simpson", "Sanders"}
	for i, surname := range surnames {
		_, err := handler.CreatePerson(ctx, command.CreatePersonInput{
			GivenName: "Person",
			Surname:   surname,
		})
		if err != nil {
			t.Fatalf("CreatePerson %d failed: %v", i, err)
		}
	}

	// Also create a person with a different starting letter
	_, _ = handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Jones",
	})

	// Get surnames starting with S
	result, err := service.GetSurnameIndex(ctx, query.GetSurnameIndexInput{Letter: "S"})
	if err != nil {
		t.Fatalf("GetSurnameIndex with letter failed: %v", err)
	}

	if result.Total != 3 {
		t.Errorf("Total = %d, want 3", result.Total)
	}

	// Verify all items start with S
	for _, item := range result.Items {
		if len(item.Surname) == 0 || item.Surname[0] != 'S' {
			t.Errorf("Surname %s does not start with S", item.Surname)
		}
	}
}

func TestGetPersonsBySurname(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewBrowseService(readStore)
	ctx := context.Background()

	// Create multiple Smiths
	givenNames := []string{"Alice", "Bob", "Charlie"}
	for _, name := range givenNames {
		_, err := handler.CreatePerson(ctx, command.CreatePersonInput{
			GivenName: name,
			Surname:   "Smith",
		})
		if err != nil {
			t.Fatalf("CreatePerson failed: %v", err)
		}
	}

	// Create a Jones
	_, _ = handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Jones",
	})

	// Get Smiths
	result, err := service.GetPersonsBySurname(ctx, query.GetPersonsBySurnameInput{
		Surname: "Smith",
	})
	if err != nil {
		t.Fatalf("GetPersonsBySurname failed: %v", err)
	}

	if result.Total != 3 {
		t.Errorf("Total = %d, want 3", result.Total)
	}
	if len(result.Items) != 3 {
		t.Errorf("Items count = %d, want 3", len(result.Items))
	}

	// All should be Smiths
	for _, person := range result.Items {
		if person.Surname != "Smith" {
			t.Errorf("Got surname %s, want Smith", person.Surname)
		}
	}
}

func TestGetPersonsBySurname_Pagination(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewBrowseService(readStore)
	ctx := context.Background()

	// Create 5 Smiths
	for i := 0; i < 5; i++ {
		_, _ = handler.CreatePerson(ctx, command.CreatePersonInput{
			GivenName: "Person",
			Surname:   "Smith",
		})
	}

	// Get first page
	result, err := service.GetPersonsBySurname(ctx, query.GetPersonsBySurnameInput{
		Surname: "Smith",
		Limit:   2,
		Offset:  0,
	})
	if err != nil {
		t.Fatalf("GetPersonsBySurname failed: %v", err)
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
	result2, _ := service.GetPersonsBySurname(ctx, query.GetPersonsBySurnameInput{
		Surname: "Smith",
		Limit:   2,
		Offset:  2,
	})

	if len(result2.Items) != 2 {
		t.Errorf("Page 2 items = %d, want 2", len(result2.Items))
	}
}

func TestGetPlaceHierarchy(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewBrowseService(readStore)
	ctx := context.Background()

	// Create persons with places
	places := []string{
		"New York, NY, USA",
		"Boston, MA, USA",
		"London, England, UK",
	}
	for i, place := range places {
		_, err := handler.CreatePerson(ctx, command.CreatePersonInput{
			GivenName:  "Person",
			Surname:    "Test",
			BirthPlace: place,
		})
		if err != nil {
			t.Fatalf("CreatePerson %d failed: %v", i, err)
		}
	}

	// Get top-level places (should be simplified for memory store)
	result, err := service.GetPlaceHierarchy(ctx, query.GetPlaceHierarchyInput{})
	if err != nil {
		t.Fatalf("GetPlaceHierarchy failed: %v", err)
	}

	if result.Total == 0 {
		t.Error("Expected some places")
	}
}

func TestGetPersonsByPlace(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewBrowseService(readStore)
	ctx := context.Background()

	// Create persons from USA
	for i := 0; i < 3; i++ {
		_, _ = handler.CreatePerson(ctx, command.CreatePersonInput{
			GivenName:  "Person",
			Surname:    "USA",
			BirthPlace: "New York, USA",
		})
	}

	// Create person from UK
	_, _ = handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName:  "Person",
		Surname:    "UK",
		BirthPlace: "London, UK",
	})

	// Get persons from USA
	result, err := service.GetPersonsByPlace(ctx, query.GetPersonsByPlaceInput{
		Place: "USA",
	})
	if err != nil {
		t.Fatalf("GetPersonsByPlace failed: %v", err)
	}

	if result.Total != 3 {
		t.Errorf("Total = %d, want 3", result.Total)
	}
}

func TestGetPersonsByPlace_DeathPlace(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewBrowseService(readStore)
	ctx := context.Background()

	// Create person with death place
	_, _ = handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName:  "John",
		Surname:    "Doe",
		DeathPlace: "Paris, France",
	})

	// Get persons from France
	result, err := service.GetPersonsByPlace(ctx, query.GetPersonsByPlaceInput{
		Place: "France",
	})
	if err != nil {
		t.Fatalf("GetPersonsByPlace failed: %v", err)
	}

	if result.Total != 1 {
		t.Errorf("Total = %d, want 1", result.Total)
	}
}

func TestGetPersonsBySurname_CaseInsensitive(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewBrowseService(readStore)
	ctx := context.Background()

	// Create person
	_, _ = handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Smith",
	})

	// Search with lowercase
	result, err := service.GetPersonsBySurname(ctx, query.GetPersonsBySurnameInput{
		Surname: "smith",
	})
	if err != nil {
		t.Fatalf("GetPersonsBySurname failed: %v", err)
	}

	if result.Total != 1 {
		t.Errorf("Total = %d, want 1 (case-insensitive)", result.Total)
	}
}

func TestGetSurnameIndex_EmptyDatabase(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewBrowseService(readStore)
	ctx := context.Background()

	result, err := service.GetSurnameIndex(ctx, query.GetSurnameIndexInput{})
	if err != nil {
		t.Fatalf("GetSurnameIndex failed: %v", err)
	}

	if result.Total != 0 {
		t.Errorf("Total = %d, want 0", result.Total)
	}
	if len(result.Items) != 0 {
		t.Errorf("Items count = %d, want 0", len(result.Items))
	}
}

func TestGetPlaceHierarchy_EmptyDatabase(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewBrowseService(readStore)
	ctx := context.Background()

	result, err := service.GetPlaceHierarchy(ctx, query.GetPlaceHierarchyInput{})
	if err != nil {
		t.Fatalf("GetPlaceHierarchy failed: %v", err)
	}

	if result.Total != 0 {
		t.Errorf("Total = %d, want 0", result.Total)
	}
}

func TestGetPersonsBySurname_NotFound(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewBrowseService(readStore)
	ctx := context.Background()

	result, err := service.GetPersonsBySurname(ctx, query.GetPersonsBySurnameInput{
		Surname: "NonExistent",
	})
	if err != nil {
		t.Fatalf("GetPersonsBySurname failed: %v", err)
	}

	if result.Total != 0 {
		t.Errorf("Total = %d, want 0", result.Total)
	}
}

func TestGetPersonsByPlace_NotFound(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewBrowseService(readStore)
	ctx := context.Background()

	result, err := service.GetPersonsByPlace(ctx, query.GetPersonsByPlaceInput{
		Place: "Narnia",
	})
	if err != nil {
		t.Fatalf("GetPersonsByPlace failed: %v", err)
	}

	if result.Total != 0 {
		t.Errorf("Total = %d, want 0", result.Total)
	}
}

func TestGetPersonsBySurname_LimitCapping(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewBrowseService(readStore)
	ctx := context.Background()

	// Create a person
	_, _ = handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Smith",
	})

	// Test limit > 100 gets capped to 100
	result, err := service.GetPersonsBySurname(ctx, query.GetPersonsBySurnameInput{
		Surname: "Smith",
		Limit:   200,
	})
	if err != nil {
		t.Fatalf("GetPersonsBySurname failed: %v", err)
	}
	if result.Limit != 100 {
		t.Errorf("Limit = %d, want 100 (capped)", result.Limit)
	}

	// Test negative offset defaults to 0
	result2, err := service.GetPersonsBySurname(ctx, query.GetPersonsBySurnameInput{
		Surname: "Smith",
		Offset:  -5,
	})
	if err != nil {
		t.Fatalf("GetPersonsBySurname failed: %v", err)
	}
	if result2.Offset != 0 {
		t.Errorf("Offset = %d, want 0 (defaulted)", result2.Offset)
	}
}

func TestGetPersonsByPlace_LimitCapping(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewBrowseService(readStore)
	ctx := context.Background()

	// Create a person
	_, _ = handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName:  "John",
		Surname:    "Doe",
		BirthPlace: "New York, USA",
	})

	// Test limit > 100 gets capped to 100
	result, err := service.GetPersonsByPlace(ctx, query.GetPersonsByPlaceInput{
		Place: "USA",
		Limit: 200,
	})
	if err != nil {
		t.Fatalf("GetPersonsByPlace failed: %v", err)
	}
	if result.Limit != 100 {
		t.Errorf("Limit = %d, want 100 (capped)", result.Limit)
	}

	// Test negative offset defaults to 0
	result2, err := service.GetPersonsByPlace(ctx, query.GetPersonsByPlaceInput{
		Place:  "USA",
		Offset: -10,
	})
	if err != nil {
		t.Fatalf("GetPersonsByPlace failed: %v", err)
	}
	if result2.Offset != 0 {
		t.Errorf("Offset = %d, want 0 (defaulted)", result2.Offset)
	}
}

func TestGetPlaceHierarchy_WithBreadcrumb(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewBrowseService(readStore)
	ctx := context.Background()

	// Create persons with hierarchical places
	_, _ = handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName:  "John",
		Surname:    "Doe",
		BirthPlace: "Springfield, Illinois, USA",
	})

	// Get places with parent filter to test breadcrumb
	result, err := service.GetPlaceHierarchy(ctx, query.GetPlaceHierarchyInput{
		Parent: "Illinois, USA",
	})
	if err != nil {
		t.Fatalf("GetPlaceHierarchy failed: %v", err)
	}

	// Should have breadcrumb from parent
	if len(result.Breadcrumb) == 0 {
		t.Error("Expected breadcrumb to be populated when parent is specified")
	}

	// Breadcrumb should be in reverse order (most general first)
	// "Illinois, USA" -> ["USA", "Illinois"]
	if len(result.Breadcrumb) >= 2 {
		if result.Breadcrumb[0] != "USA" {
			t.Errorf("Breadcrumb[0] = %s, want USA", result.Breadcrumb[0])
		}
		if result.Breadcrumb[1] != "Illinois" {
			t.Errorf("Breadcrumb[1] = %s, want Illinois", result.Breadcrumb[1])
		}
	}
}
