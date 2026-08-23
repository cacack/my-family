package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
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
		if item.Surname == "" || item.Surname[0] != 'S' {
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

// Cemetery browse tests

// saveCemeteryTestData is a helper that creates persons and burial/cremation events in the read store.
func saveCemeteryTestData(t *testing.T, ctx context.Context, readStore *memory.ReadModelStore, data []struct {
	givenName string
	surname   string
	factType  domain.FactType
	place     string
}) []uuid.UUID {
	t.Helper()
	var ids []uuid.UUID
	for _, d := range data {
		pid := uuid.New()
		ids = append(ids, pid)
		err := readStore.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
			ID:        pid,
			GivenName: d.givenName,
			Surname:   d.surname,
			FullName:  d.givenName + " " + d.surname,
			Version:   1,
			UpdatedAt: time.Now(),
		})
		if err != nil {
			t.Fatalf("SavePerson() failed: %v", err)
		}
		err = readStore.SaveEvent(ctx, &repository.EventReadModel{
			ID:        uuid.New(),
			OwnerType: "person",
			OwnerID:   pid,
			FactType:  d.factType,
			Place:     d.place,
			Version:   1,
			CreatedAt: time.Now(),
		})
		if err != nil {
			t.Fatalf("SaveEvent() failed: %v", err)
		}
	}
	return ids
}

func TestGetCemeteryIndex_EmptyDatabase(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewBrowseService(readStore)
	ctx := context.Background()

	result, err := service.GetCemeteryIndex(ctx)
	if err != nil {
		t.Fatalf("GetCemeteryIndex failed: %v", err)
	}

	if result.Total != 0 {
		t.Errorf("Total = %d, want 0", result.Total)
	}
	if len(result.Items) != 0 {
		t.Errorf("Items count = %d, want 0", len(result.Items))
	}
}

func TestGetCemeteryIndex(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewBrowseService(readStore)
	ctx := context.Background()

	saveCemeteryTestData(t, ctx, readStore, []struct {
		givenName string
		surname   string
		factType  domain.FactType
		place     string
	}{
		{"John", "Smith", domain.FactPersonBurial, "Oakwood Cemetery"},
		{"Jane", "Smith", domain.FactPersonBurial, "Oakwood Cemetery"},
		{"Bob", "Jones", domain.FactPersonCremation, "Rose Hills Memorial"},
		{"Alice", "Brown", domain.FactPersonBurial, "Green Lawn Cemetery"},
	})

	result, err := service.GetCemeteryIndex(ctx)
	if err != nil {
		t.Fatalf("GetCemeteryIndex failed: %v", err)
	}

	if result.Total != 3 {
		t.Errorf("Total = %d, want 3", result.Total)
	}

	// Verify sorted alphabetically
	if len(result.Items) >= 3 {
		if result.Items[0].Place != "Green Lawn Cemetery" {
			t.Errorf("Items[0].Place = %q, want %q", result.Items[0].Place, "Green Lawn Cemetery")
		}
		if result.Items[1].Place != "Oakwood Cemetery" {
			t.Errorf("Items[1].Place = %q, want %q", result.Items[1].Place, "Oakwood Cemetery")
		}
		if result.Items[1].Count != 2 {
			t.Errorf("Items[1].Count = %d, want 2", result.Items[1].Count)
		}
		if result.Items[2].Place != "Rose Hills Memorial" {
			t.Errorf("Items[2].Place = %q, want %q", result.Items[2].Place, "Rose Hills Memorial")
		}
	}
}

func TestGetCemeteryIndex_IncludesCremation(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewBrowseService(readStore)
	ctx := context.Background()

	saveCemeteryTestData(t, ctx, readStore, []struct {
		givenName string
		surname   string
		factType  domain.FactType
		place     string
	}{
		{"John", "Doe", domain.FactPersonCremation, "Memorial Crematorium"},
	})

	result, err := service.GetCemeteryIndex(ctx)
	if err != nil {
		t.Fatalf("GetCemeteryIndex failed: %v", err)
	}

	if result.Total != 1 {
		t.Errorf("Total = %d, want 1", result.Total)
	}
	if len(result.Items) == 1 && result.Items[0].Place != "Memorial Crematorium" {
		t.Errorf("Items[0].Place = %q, want %q", result.Items[0].Place, "Memorial Crematorium")
	}
}

func TestGetPersonsByCemetery(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewBrowseService(readStore)
	ctx := context.Background()

	saveCemeteryTestData(t, ctx, readStore, []struct {
		givenName string
		surname   string
		factType  domain.FactType
		place     string
	}{
		{"Alice", "Smith", domain.FactPersonBurial, "Oakwood Cemetery"},
		{"Bob", "Jones", domain.FactPersonBurial, "Oakwood Cemetery"},
		{"Charlie", "Brown", domain.FactPersonCremation, "Rose Hills Memorial"},
	})

	result, err := service.GetPersonsByCemetery(ctx, query.GetPersonsByCemeteryInput{
		Place: "Oakwood Cemetery",
	})
	if err != nil {
		t.Fatalf("GetPersonsByCemetery failed: %v", err)
	}

	if result.Total != 2 {
		t.Errorf("Total = %d, want 2", result.Total)
	}
	if len(result.Items) != 2 {
		t.Errorf("Items count = %d, want 2", len(result.Items))
	}

	// Should be sorted by surname
	if len(result.Items) == 2 {
		if result.Items[0].Surname != "Jones" {
			t.Errorf("Items[0].Surname = %q, want %q", result.Items[0].Surname, "Jones")
		}
		if result.Items[1].Surname != "Smith" {
			t.Errorf("Items[1].Surname = %q, want %q", result.Items[1].Surname, "Smith")
		}
	}
}

func TestGetPersonsByCemetery_NotFound(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewBrowseService(readStore)
	ctx := context.Background()

	result, err := service.GetPersonsByCemetery(ctx, query.GetPersonsByCemeteryInput{
		Place: "Nonexistent Cemetery",
	})
	if err != nil {
		t.Fatalf("GetPersonsByCemetery failed: %v", err)
	}

	if result.Total != 0 {
		t.Errorf("Total = %d, want 0", result.Total)
	}
}

func TestGetPersonsByCemetery_CaseInsensitive(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewBrowseService(readStore)
	ctx := context.Background()

	saveCemeteryTestData(t, ctx, readStore, []struct {
		givenName string
		surname   string
		factType  domain.FactType
		place     string
	}{
		{"John", "Doe", domain.FactPersonBurial, "Oakwood Cemetery"},
	})

	// Search with lowercase
	result, err := service.GetPersonsByCemetery(ctx, query.GetPersonsByCemeteryInput{
		Place: "oakwood cemetery",
	})
	if err != nil {
		t.Fatalf("GetPersonsByCemetery failed: %v", err)
	}

	if result.Total != 1 {
		t.Errorf("Total = %d, want 1 (case-insensitive)", result.Total)
	}
}

func TestGetPersonsByCemetery_Pagination(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewBrowseService(readStore)
	ctx := context.Background()

	// Create 5 persons at the same cemetery
	for i := 0; i < 5; i++ {
		pid := uuid.New()
		_ = readStore.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
			ID:        pid,
			GivenName: "Person",
			Surname:   "Test",
			FullName:  "Person Test",
			Version:   1,
			UpdatedAt: time.Now(),
		})
		_ = readStore.SaveEvent(ctx, &repository.EventReadModel{
			ID:        uuid.New(),
			OwnerType: "person",
			OwnerID:   pid,
			FactType:  domain.FactPersonBurial,
			Place:     "Central Cemetery",
			Version:   1,
			CreatedAt: time.Now(),
		})
	}

	// First page
	result, err := service.GetPersonsByCemetery(ctx, query.GetPersonsByCemeteryInput{
		Place:  "Central Cemetery",
		Limit:  2,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("GetPersonsByCemetery failed: %v", err)
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

	// Second page
	result2, err := service.GetPersonsByCemetery(ctx, query.GetPersonsByCemeteryInput{
		Place:  "Central Cemetery",
		Limit:  2,
		Offset: 2,
	})
	if err != nil {
		t.Fatalf("GetPersonsByCemetery page 2 failed: %v", err)
	}
	if len(result2.Items) != 2 {
		t.Errorf("Page 2 items = %d, want 2", len(result2.Items))
	}
}

func TestGetPersonsByCemetery_LimitCapping(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewBrowseService(readStore)
	ctx := context.Background()

	pid := uuid.New()
	_ = readStore.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
		ID:        pid,
		GivenName: "John",
		Surname:   "Doe",
		FullName:  "John Doe",
		Version:   1,
		UpdatedAt: time.Now(),
	})
	_ = readStore.SaveEvent(ctx, &repository.EventReadModel{
		ID:        uuid.New(),
		OwnerType: "person",
		OwnerID:   pid,
		FactType:  domain.FactPersonBurial,
		Place:     "Test Cemetery",
		Version:   1,
		CreatedAt: time.Now(),
	})

	// Test limit > 100 gets capped to 100
	result, err := service.GetPersonsByCemetery(ctx, query.GetPersonsByCemeteryInput{
		Place: "Test Cemetery",
		Limit: 200,
	})
	if err != nil {
		t.Fatalf("GetPersonsByCemetery failed: %v", err)
	}
	if result.Limit != 100 {
		t.Errorf("Limit = %d, want 100 (capped)", result.Limit)
	}

	// Test negative offset defaults to 0
	result2, err := service.GetPersonsByCemetery(ctx, query.GetPersonsByCemeteryInput{
		Place:  "Test Cemetery",
		Offset: -5,
	})
	if err != nil {
		t.Fatalf("GetPersonsByCemetery failed: %v", err)
	}
	if result2.Offset != 0 {
		t.Errorf("Offset = %d, want 0 (defaulted)", result2.Offset)
	}
}

// ============================================================================
// Branch scope plumbing (sub-issue A of #676, issue #756)
// ============================================================================

// branchScopeSpy records the branch scope each browse/map store call receives.
// It embeds the interface so only the methods under test need overriding; any
// other call would panic on the nil embedded value, which keeps the spy honest.
type branchScopeSpy struct {
	repository.ReadModelStore

	surnameIndex      []domain.BranchID
	surnamesByLetter  []domain.BranchID
	personsBySurname  []domain.BranchID
	placeHierarchy    []domain.BranchID
	personsByPlace    []domain.BranchID
	personsByCemetery []domain.BranchID
	mapLocations      []domain.BranchID

	// The main-only methods carry no scope at all — the counters just prove the
	// service still reaches them unchanged.
	cemeteryIndexCalls    int
	brickWallCalls        int
	setBrickWallCalls     int
	resolveBrickWallCalls int
}

func (s *branchScopeSpy) GetSurnameIndex(_ context.Context, branchID domain.BranchID) ([]repository.SurnameEntry, []repository.LetterCount, error) {
	s.surnameIndex = append(s.surnameIndex, branchID)
	return nil, nil, nil
}

func (s *branchScopeSpy) GetSurnamesByLetter(_ context.Context, branchID domain.BranchID, _ string) ([]repository.SurnameEntry, error) {
	s.surnamesByLetter = append(s.surnamesByLetter, branchID)
	return nil, nil
}

func (s *branchScopeSpy) GetPersonsBySurname(_ context.Context, _ string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	s.personsBySurname = append(s.personsBySurname, opts.BranchID)
	return nil, 0, nil
}

func (s *branchScopeSpy) GetPlaceHierarchy(_ context.Context, branchID domain.BranchID, _ string) ([]repository.PlaceEntry, error) {
	s.placeHierarchy = append(s.placeHierarchy, branchID)
	return nil, nil
}

func (s *branchScopeSpy) GetPersonsByPlace(_ context.Context, _ string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	s.personsByPlace = append(s.personsByPlace, opts.BranchID)
	return nil, 0, nil
}

func (s *branchScopeSpy) GetPersonsByCemetery(_ context.Context, _ string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	s.personsByCemetery = append(s.personsByCemetery, opts.BranchID)
	return nil, 0, nil
}

func (s *branchScopeSpy) GetMapLocations(_ context.Context, branchID domain.BranchID) ([]repository.MapLocation, error) {
	s.mapLocations = append(s.mapLocations, branchID)
	return nil, nil
}

func (s *branchScopeSpy) GetCemeteryIndex(_ context.Context) ([]repository.CemeteryEntry, error) {
	s.cemeteryIndexCalls++
	return nil, nil
}

func (s *branchScopeSpy) GetBrickWalls(_ context.Context, _ bool) ([]repository.BrickWallEntry, error) {
	s.brickWallCalls++
	return nil, nil
}

func (s *branchScopeSpy) SetBrickWall(_ context.Context, _ uuid.UUID, _ string) error {
	s.setBrickWallCalls++
	return nil
}

func (s *branchScopeSpy) ResolveBrickWall(_ context.Context, _ uuid.UUID) error {
	s.resolveBrickWallCalls++
	return nil
}

// TestBrowseService_BranchScopeReachesStore checks that the scope the caller
// supplies arrives at the store for each of the six branch-aware browse/map
// reads, and that omitting it means the mainline.
func TestBrowseService_BranchScopeReachesStore(t *testing.T) {
	branchID := domain.BranchID(uuid.New())

	tests := []struct {
		name string
		call func(*query.BrowseService, domain.BranchID) error
		got  func(*branchScopeSpy) []domain.BranchID
	}{
		{
			name: "GetSurnameIndex",
			call: func(s *query.BrowseService, b domain.BranchID) error {
				_, err := s.GetSurnameIndex(context.Background(), query.GetSurnameIndexInput{BranchID: b})
				return err
			},
			got: func(s *branchScopeSpy) []domain.BranchID { return s.surnameIndex },
		},
		{
			name: "GetSurnameIndex_ByLetter",
			call: func(s *query.BrowseService, b domain.BranchID) error {
				_, err := s.GetSurnameIndex(context.Background(), query.GetSurnameIndexInput{Letter: "S", BranchID: b})
				return err
			},
			got: func(s *branchScopeSpy) []domain.BranchID { return s.surnamesByLetter },
		},
		{
			name: "GetPersonsBySurname",
			call: func(s *query.BrowseService, b domain.BranchID) error {
				_, err := s.GetPersonsBySurname(context.Background(), query.GetPersonsBySurnameInput{Surname: "Smith", BranchID: b})
				return err
			},
			got: func(s *branchScopeSpy) []domain.BranchID { return s.personsBySurname },
		},
		{
			name: "GetPlaceHierarchy",
			call: func(s *query.BrowseService, b domain.BranchID) error {
				_, err := s.GetPlaceHierarchy(context.Background(), query.GetPlaceHierarchyInput{BranchID: b})
				return err
			},
			got: func(s *branchScopeSpy) []domain.BranchID { return s.placeHierarchy },
		},
		{
			name: "GetPersonsByPlace",
			call: func(s *query.BrowseService, b domain.BranchID) error {
				_, err := s.GetPersonsByPlace(context.Background(), query.GetPersonsByPlaceInput{Place: "Ohio", BranchID: b})
				return err
			},
			got: func(s *branchScopeSpy) []domain.BranchID { return s.personsByPlace },
		},
		{
			name: "GetPersonsByCemetery",
			call: func(s *query.BrowseService, b domain.BranchID) error {
				_, err := s.GetPersonsByCemetery(context.Background(), query.GetPersonsByCemeteryInput{Place: "Oak Grove", BranchID: b})
				return err
			},
			got: func(s *branchScopeSpy) []domain.BranchID { return s.personsByCemetery },
		},
		{
			name: "GetMapLocations",
			call: func(s *query.BrowseService, b domain.BranchID) error {
				_, err := s.GetMapLocations(context.Background(), b)
				return err
			},
			got: func(s *branchScopeSpy) []domain.BranchID { return s.mapLocations },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spy := &branchScopeSpy{}
			service := query.NewBrowseService(spy)

			if err := tt.call(service, branchID); err != nil {
				t.Fatalf("branch-scoped call failed: %v", err)
			}
			if err := tt.call(service, domain.MainBranchID); err != nil {
				t.Fatalf("mainline call failed: %v", err)
			}

			scopes := tt.got(spy)
			if len(scopes) != 2 {
				t.Fatalf("store received %d calls, want 2", len(scopes))
			}
			if scopes[0] != branchID {
				t.Errorf("branch-scoped call passed %v, want %v", scopes[0], branchID)
			}
			if scopes[1] != domain.MainBranchID {
				t.Errorf("mainline call passed %v, want MainBranchID", scopes[1])
			}
		})
	}
}

// TestBrowseService_MainOnlyPathsUnscoped pins the four reads that stay on the
// mainline this cycle: their store methods take no branch scope at all, so
// there is nothing for a caller to redirect (sub-issues B/#757 and F/#761).
func TestBrowseService_MainOnlyPathsUnscoped(t *testing.T) {
	spy := &branchScopeSpy{}
	service := query.NewBrowseService(spy)
	ctx := context.Background()

	if _, err := service.GetCemeteryIndex(ctx); err != nil {
		t.Fatalf("GetCemeteryIndex failed: %v", err)
	}
	if _, err := service.GetBrickWalls(ctx, true); err != nil {
		t.Fatalf("GetBrickWalls failed: %v", err)
	}
	if err := service.SetBrickWall(ctx, uuid.New(), "note"); err != nil {
		t.Fatalf("SetBrickWall failed: %v", err)
	}
	if err := service.ResolveBrickWall(ctx, uuid.New()); err != nil {
		t.Fatalf("ResolveBrickWall failed: %v", err)
	}

	if spy.cemeteryIndexCalls != 1 || spy.brickWallCalls != 1 ||
		spy.setBrickWallCalls != 1 || spy.resolveBrickWallCalls != 1 {
		t.Errorf("main-only calls = %d/%d/%d/%d, want 1 each",
			spy.cemeteryIndexCalls, spy.brickWallCalls, spy.setBrickWallCalls, spy.resolveBrickWallCalls)
	}
}
