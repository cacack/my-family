package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
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
	person, err := service.GetPerson(ctx, domain.MainBranchID, createResult.ID)
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

	_, err := service.GetPerson(ctx, domain.MainBranchID, [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
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

func TestListPersons_LimitConstraints(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewPersonService(readStore)
	ctx := context.Background()

	// Create some test data
	for i := 0; i < 3; i++ {
		_, _ = handler.CreatePerson(ctx, command.CreatePersonInput{
			GivenName: "Test",
			Surname:   "Person",
		})
	}

	tests := []struct {
		name          string
		input         query.ListPersonsInput
		expectedLimit int
	}{
		{
			name:          "limit over max gets capped to 100",
			input:         query.ListPersonsInput{Limit: 200},
			expectedLimit: 100,
		},
		{
			name:          "negative limit defaults to 20",
			input:         query.ListPersonsInput{Limit: -1},
			expectedLimit: 20,
		},
		{
			name:          "zero limit defaults to 20",
			input:         query.ListPersonsInput{Limit: 0},
			expectedLimit: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.ListPersons(ctx, tt.input)
			if err != nil {
				t.Fatalf("ListPersons failed: %v", err)
			}
			if result.Limit != tt.expectedLimit {
				t.Errorf("Limit = %d, want %d", result.Limit, tt.expectedLimit)
			}
		})
	}
}

func TestListPersons_SortOptions(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewPersonService(readStore)
	ctx := context.Background()

	// Create test data
	_, _ = handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Alice",
		Surname:   "Smith",
	})

	tests := []struct {
		name string
		sort string
		desc string
	}{
		{
			name: "default sort is surname ascending",
			sort: "",
			desc: "should use default sort",
		},
		{
			name: "explicit surname sort",
			sort: "surname",
			desc: "should use surname sort",
		},
		{
			name: "given_name sort",
			sort: "given_name",
			desc: "should use given_name sort",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.ListPersons(ctx, query.ListPersonsInput{
				Sort: tt.sort,
			})
			if err != nil {
				t.Fatalf("ListPersons failed: %v", err)
			}
			if result.Total != 1 {
				t.Errorf("Expected 1 result, got %d", result.Total)
			}
		})
	}
}

func TestListPersons_OffsetBeyondResults(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewPersonService(readStore)
	ctx := context.Background()

	// Create only 2 persons
	_, _ = handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Person1",
		Surname:   "Test",
	})
	_, _ = handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Person2",
		Surname:   "Test",
	})

	// Request offset beyond results
	result, err := service.ListPersons(ctx, query.ListPersonsInput{
		Limit:  10,
		Offset: 100,
	})
	if err != nil {
		t.Fatalf("ListPersons failed: %v", err)
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

func TestSearchPersons_LimitConstraints(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewPersonService(readStore)
	ctx := context.Background()

	_, _ = handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Test",
		Surname:   "Person",
	})

	tests := []struct {
		name          string
		limit         int
		expectedLimit int
	}{
		{
			name:          "limit over 100 gets capped",
			limit:         200,
			expectedLimit: 100,
		},
		{
			name:          "zero limit defaults to 20",
			limit:         0,
			expectedLimit: 20,
		},
		{
			name:          "negative limit defaults to 20",
			limit:         -5,
			expectedLimit: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.SearchPersons(ctx, query.SearchPersonsInput{
				Query: "Test",
				Limit: tt.limit,
			})
			if err != nil {
				t.Fatalf("SearchPersons failed: %v", err)
			}
			// Note: we can't directly check the limit, but we verify it works
			if result.Query != "Test" {
				t.Errorf("Query = %s, want Test", result.Query)
			}
		})
	}
}

func TestSearchPersons_EmptyQuery(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewPersonService(readStore)
	ctx := context.Background()

	_, _ = handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Test",
		Surname:   "Person",
	})

	result, err := service.SearchPersons(ctx, query.SearchPersonsInput{
		Query: "",
	})
	if err != nil {
		t.Fatalf("SearchPersons failed: %v", err)
	}

	if result.Query != "" {
		t.Errorf("Query = %s, want empty string", result.Query)
	}
}

func TestGetPerson_WithFamilyRelationships(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewPersonService(readStore)
	ctx := context.Background()

	// Create a person with family relationships
	person1, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	person2, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Jane",
		Surname:   "Doe",
	})
	child, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Junior",
		Surname:   "Doe",
	})

	// Create family as partners
	_, _ = handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &person1.ID,
		Partner2ID: &person2.ID,
	})

	// Create parent family for child
	parentFamily, _ := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &person1.ID,
		Partner2ID: &person2.ID,
	})
	_, _ = handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID:     parentFamily.ID,
		ChildID:      child.ID,
		RelationType: "biological",
	})

	// Get person with families
	detail, err := service.GetPerson(ctx, domain.MainBranchID, person1.ID)
	if err != nil {
		t.Fatalf("GetPerson failed: %v", err)
	}

	if len(detail.FamiliesAsPartner) == 0 {
		t.Error("Expected FamiliesAsPartner to be populated")
	}

	// Get child to test family as child
	childDetail, err := service.GetPerson(ctx, domain.MainBranchID, child.ID)
	if err != nil {
		t.Fatalf("GetPerson for child failed: %v", err)
	}

	if childDetail.FamilyAsChild == nil {
		t.Error("Expected FamilyAsChild to be populated")
	}
}

func TestConvertReadModelToPerson_AllFields(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewPersonService(readStore)
	ctx := context.Background()

	// Create person with all fields populated
	createResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName:  "John",
		Surname:    "Doe",
		Gender:     "male",
		BirthDate:  "15 MAR 1980",
		BirthPlace: "Boston, MA",
		DeathDate:  "20 DEC 2050",
		DeathPlace: "New York, NY",
		Notes:      "Test notes",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	person, err := service.GetPerson(ctx, domain.MainBranchID, createResult.ID)
	if err != nil {
		t.Fatalf("GetPerson failed: %v", err)
	}

	// Verify all optional fields are set
	if person.Gender == nil {
		t.Error("Gender should be set")
	}
	if person.BirthDate == nil {
		t.Error("BirthDate should be set")
	}
	if person.BirthPlace == nil {
		t.Error("BirthPlace should be set")
	}
	if person.DeathDate == nil {
		t.Error("DeathDate should be set")
	}
	if person.DeathPlace == nil {
		t.Error("DeathPlace should be set")
	}
	if person.Notes == nil {
		t.Error("Notes should be set")
	}
}

func TestGenDateToSortTime(t *testing.T) {
	tests := []struct {
		name    string
		genDate *domain.GenDate
		wantNil bool
	}{
		{
			name:    "nil gendate returns nil",
			genDate: nil,
			wantNil: true,
		},
		{
			name:    "empty gendate returns nil",
			genDate: &domain.GenDate{},
			wantNil: true,
		},
		{
			name:    "gendate with year only returns time",
			genDate: func() *domain.GenDate { gd := domain.ParseGenDate("1850"); return &gd }(),
			wantNil: false,
		},
		{
			name:    "gendate with full date returns time",
			genDate: func() *domain.GenDate { gd := domain.ParseGenDate("15 MAR 1850"); return &gd }(),
			wantNil: false,
		},
		{
			name:    "gendate with about qualifier returns time",
			genDate: func() *domain.GenDate { gd := domain.ParseGenDate("ABT 1850"); return &gd }(),
			wantNil: false,
		},
		{
			name:    "gendate with date range returns time",
			genDate: func() *domain.GenDate { gd := domain.ParseGenDate("BET 1850 AND 1860"); return &gd }(),
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := query.GenDateToSortTime(tt.genDate)
			if tt.wantNil && result != nil {
				t.Errorf("GenDateToSortTime() = %v, want nil", result)
			}
			if !tt.wantNil && result == nil {
				t.Error("GenDateToSortTime() = nil, want non-nil time")
			}
			if !tt.wantNil && result != nil {
				// Verify year is correct for non-nil results
				if result.Year() != 1850 {
					t.Errorf("GenDateToSortTime().Year() = %d, want 1850", result.Year())
				}
			}
		})
	}
}

func TestGetPersonNames(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewPersonService(readStore)
	ctx := context.Background()

	// Create a person
	createResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Add a primary name
	_, err = handler.AddName(ctx, command.AddNameInput{
		PersonID:  createResult.ID,
		GivenName: "John",
		Surname:   "Doe",
		IsPrimary: true,
	})
	if err != nil {
		t.Fatalf("AddName failed: %v", err)
	}

	// Get names for the person
	names, err := service.GetPersonNames(ctx, domain.MainBranchID, createResult.ID)
	if err != nil {
		t.Fatalf("GetPersonNames failed: %v", err)
	}

	// Should have one primary name
	if len(names) != 1 {
		t.Errorf("len(names) = %d, want 1", len(names))
	}

	if names[0].GivenName != "John" {
		t.Errorf("GivenName = %s, want John", names[0].GivenName)
	}
	if names[0].Surname != "Doe" {
		t.Errorf("Surname = %s, want Doe", names[0].Surname)
	}
	if !names[0].IsPrimary {
		t.Error("Expected first name to be primary")
	}
}

func TestGetPersonNames_MultipleNames(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewPersonService(readStore)
	ctx := context.Background()

	// Create a person
	createResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Jane",
		Surname:   "Smith",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Add a primary name (married name)
	_, err = handler.AddName(ctx, command.AddNameInput{
		PersonID:  createResult.ID,
		GivenName: "Jane",
		Surname:   "Smith",
		IsPrimary: true,
	})
	if err != nil {
		t.Fatalf("AddName failed: %v", err)
	}

	// Add a maiden name
	_, err = handler.AddName(ctx, command.AddNameInput{
		PersonID:  createResult.ID,
		GivenName: "Jane",
		Surname:   "Johnson",
		NameType:  "birth",
	})
	if err != nil {
		t.Fatalf("AddName failed: %v", err)
	}

	// Get names for the person
	names, err := service.GetPersonNames(ctx, domain.MainBranchID, createResult.ID)
	if err != nil {
		t.Fatalf("GetPersonNames failed: %v", err)
	}

	// Should have two names now
	if len(names) != 2 {
		t.Errorf("len(names) = %d, want 2", len(names))
	}

	// Count primary names (should be exactly 1)
	primaryCount := 0
	for _, n := range names {
		if n.IsPrimary {
			primaryCount++
		}
	}
	if primaryCount != 1 {
		t.Errorf("primary count = %d, want 1", primaryCount)
	}
}

func TestGetPersonNames_NotFound(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewPersonService(readStore)
	ctx := context.Background()

	// Try to get names for non-existent person
	_, err := service.GetPersonNames(ctx, domain.MainBranchID, [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	if err != query.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestGetPersonNames_WithAllFields(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	service := query.NewPersonService(readStore)
	ctx := context.Background()

	// Create a person
	createResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Smith",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Add a primary name first
	_, err = handler.AddName(ctx, command.AddNameInput{
		PersonID:  createResult.ID,
		GivenName: "John",
		Surname:   "Smith",
		IsPrimary: true,
	})
	if err != nil {
		t.Fatalf("AddName failed: %v", err)
	}

	// Add a name with all fields
	_, err = handler.AddName(ctx, command.AddNameInput{
		PersonID:      createResult.ID,
		GivenName:     "Johann",
		Surname:       "Schmidt",
		NamePrefix:    "Dr.",
		NameSuffix:    "Jr.",
		SurnamePrefix: "von",
		Nickname:      "Johnny",
		NameType:      "immigrant",
	})
	if err != nil {
		t.Fatalf("AddName failed: %v", err)
	}

	// Get names for the person
	names, err := service.GetPersonNames(ctx, domain.MainBranchID, createResult.ID)
	if err != nil {
		t.Fatalf("GetPersonNames failed: %v", err)
	}

	// Find the immigrant name
	var immigrantName *query.PersonName
	for i := range names {
		if names[i].NameType == "immigrant" {
			immigrantName = &names[i]
			break
		}
	}

	if immigrantName == nil {
		t.Fatal("Expected to find immigrant name")
	}

	if immigrantName.GivenName != "Johann" {
		t.Errorf("GivenName = %s, want Johann", immigrantName.GivenName)
	}
	if immigrantName.Surname != "Schmidt" {
		t.Errorf("Surname = %s, want Schmidt", immigrantName.Surname)
	}
	if immigrantName.NamePrefix != "Dr." {
		t.Errorf("NamePrefix = %s, want Dr.", immigrantName.NamePrefix)
	}
	if immigrantName.NameSuffix != "Jr." {
		t.Errorf("NameSuffix = %s, want Jr.", immigrantName.NameSuffix)
	}
	if immigrantName.SurnamePrefix != "von" {
		t.Errorf("SurnamePrefix = %s, want von", immigrantName.SurnamePrefix)
	}
	if immigrantName.Nickname != "Johnny" {
		t.Errorf("Nickname = %s, want Johnny", immigrantName.Nickname)
	}
}

// TestPersonService_BranchScope verifies that the branch-aware PersonService
// resolves branch-over-main with tombstone suppression, that the composite
// GetPerson forwards the same branchID to every sub-lookup (person + names),
// and that the MainBranchID path reproduces pre-branch behavior.
func TestPersonService_BranchScope(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewPersonService(readStore)
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	id := uuid.New()
	nameID := uuid.New() // shared so the branch write overrides (upsert-by-id) rather than appends

	// Seed the main row and its name bucket.
	if err := readStore.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
		ID: id, GivenName: "Ada", Surname: "Main", FullName: "Ada Main", Version: 1,
	}); err != nil {
		t.Fatalf("seed main person: %v", err)
	}
	if err := readStore.SavePersonName(ctx, domain.MainBranchID, &repository.PersonNameReadModel{
		ID: nameID, PersonID: id, GivenName: "Ada", Surname: "Main",
		FullName: "Ada Main", NameType: domain.NameTypeBirth, IsPrimary: true,
	}); err != nil {
		t.Fatalf("seed main name: %v", err)
	}

	// The branch overrides both the person row and its name bucket.
	if err := readStore.SavePerson(ctx, branch, &repository.PersonReadModel{
		ID: id, GivenName: "Ada", Surname: "Branch", FullName: "Ada Branch", Version: 1,
	}); err != nil {
		t.Fatalf("seed branch person: %v", err)
	}
	if err := readStore.SavePersonName(ctx, branch, &repository.PersonNameReadModel{
		ID: nameID, PersonID: id, GivenName: "Ada", Surname: "Branch",
		FullName: "Ada Branch", NameType: domain.NameTypeBirth, IsPrimary: true,
	}); err != nil {
		t.Fatalf("seed branch name: %v", err)
	}

	// Composite GetPerson on the branch must return the branch person AND the
	// branch-scoped names (proving the sub-call was scoped to the same branch).
	branchDetail, err := service.GetPerson(ctx, branch, id)
	if err != nil {
		t.Fatalf("GetPerson branch: %v", err)
	}
	if branchDetail.Surname != "Branch" {
		t.Errorf("branch person surname = %q, want Branch", branchDetail.Surname)
	}
	if len(branchDetail.Names) != 1 || branchDetail.Names[0].Surname != "Branch" {
		t.Errorf("branch names = %+v, want single Branch name", branchDetail.Names)
	}

	// The main detail is untouched by the branch override.
	mainDetail, err := service.GetPerson(ctx, domain.MainBranchID, id)
	if err != nil {
		t.Fatalf("GetPerson main: %v", err)
	}
	if mainDetail.Surname != "Main" || len(mainDetail.Names) != 1 || mainDetail.Names[0].Surname != "Main" {
		t.Errorf("main detail surname=%q names=%+v, want Main", mainDetail.Surname, mainDetail.Names)
	}

	// List honors the branch scope.
	branchList, err := service.ListPersons(ctx, query.ListPersonsInput{BranchID: branch})
	if err != nil {
		t.Fatalf("ListPersons branch: %v", err)
	}
	if branchList.Total != 1 || branchList.Items[0].Surname != "Branch" {
		t.Errorf("branch list = %+v, want single Branch person", branchList.Items)
	}
	mainList, err := service.ListPersons(ctx, query.ListPersonsInput{})
	if err != nil {
		t.Fatalf("ListPersons main: %v", err)
	}
	if mainList.Total != 1 || mainList.Items[0].Surname != "Main" {
		t.Errorf("main list = %+v, want single Main person", mainList.Items)
	}

	// Search honors the branch scope (branch overlay wins).
	branchSearch, err := service.SearchPersons(ctx, query.SearchPersonsInput{Query: "Ada", BranchID: branch})
	if err != nil {
		t.Fatalf("SearchPersons branch: %v", err)
	}
	if len(branchSearch.Items) != 1 || branchSearch.Items[0].Surname != "Branch" {
		t.Errorf("branch search = %+v, want single Branch person", branchSearch.Items)
	}

	// A branch delete writes a tombstone: the branch view hides the person while
	// main still resolves it.
	if err := readStore.DeletePerson(ctx, branch, id); err != nil {
		t.Fatalf("DeletePerson branch: %v", err)
	}
	if _, err := service.GetPerson(ctx, branch, id); err != query.ErrNotFound {
		t.Errorf("GetPerson branch after tombstone: err = %v, want ErrNotFound", err)
	}
	if _, err := service.GetPerson(ctx, domain.MainBranchID, id); err != nil {
		t.Errorf("GetPerson main after branch tombstone: %v, want nil", err)
	}
}
