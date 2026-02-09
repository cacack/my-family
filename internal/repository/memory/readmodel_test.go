package memory_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestNewReadModelStore(t *testing.T) {
	store := memory.NewReadModelStore()
	if store == nil {
		t.Fatal("NewReadModelStore() returned nil")
	}
}

// Person CRUD operations

func TestReadModelStore_SaveAndGetPerson(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	person := &repository.PersonReadModel{
		ID:        uuid.New(),
		GivenName: "John",
		Surname:   "Doe",
		FullName:  "John Doe",
		Gender:    domain.GenderMale,
		Version:   1,
		UpdatedAt: time.Now(),
	}

	// Save person
	err := store.SavePerson(ctx, person)
	if err != nil {
		t.Fatalf("SavePerson() failed: %v", err)
	}

	// Get person
	retrieved, err := store.GetPerson(ctx, person.ID)
	if err != nil {
		t.Fatalf("GetPerson() failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetPerson() returned nil")
	}

	// Verify fields
	if retrieved.ID != person.ID {
		t.Errorf("ID = %v, want %v", retrieved.ID, person.ID)
	}
	if retrieved.GivenName != person.GivenName {
		t.Errorf("GivenName = %s, want %s", retrieved.GivenName, person.GivenName)
	}
	if retrieved.Surname != person.Surname {
		t.Errorf("Surname = %s, want %s", retrieved.Surname, person.Surname)
	}
	if retrieved.Gender != person.Gender {
		t.Errorf("Gender = %s, want %s", retrieved.Gender, person.Gender)
	}
}

func TestReadModelStore_GetPersonNonExistent(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	nonExistentID := uuid.New()

	retrieved, err := store.GetPerson(ctx, nonExistentID)
	if err != nil {
		t.Fatalf("GetPerson() failed: %v", err)
	}

	if retrieved != nil {
		t.Errorf("GetPerson() for non-existent ID = %v, want nil", retrieved)
	}
}

func TestReadModelStore_UpdatePerson(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	personID := uuid.New()
	person := &repository.PersonReadModel{
		ID:        personID,
		GivenName: "John",
		Surname:   "Doe",
		FullName:  "John Doe",
		Version:   1,
		UpdatedAt: time.Now(),
	}

	// Save initial version
	err := store.SavePerson(ctx, person)
	if err != nil {
		t.Fatalf("SavePerson() failed: %v", err)
	}

	// Update person
	person.GivenName = "Jane"
	person.FullName = "Jane Doe"
	person.Version = 2
	person.UpdatedAt = time.Now()

	err = store.SavePerson(ctx, person)
	if err != nil {
		t.Fatalf("SavePerson() update failed: %v", err)
	}

	// Retrieve and verify update
	retrieved, err := store.GetPerson(ctx, personID)
	if err != nil {
		t.Fatalf("GetPerson() failed: %v", err)
	}

	if retrieved.GivenName != "Jane" {
		t.Errorf("GivenName = %s, want Jane", retrieved.GivenName)
	}
	if retrieved.Version != 2 {
		t.Errorf("Version = %d, want 2", retrieved.Version)
	}
}

func TestReadModelStore_DeletePerson(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	person := &repository.PersonReadModel{
		ID:        uuid.New(),
		GivenName: "John",
		Surname:   "Doe",
		FullName:  "John Doe",
		Version:   1,
		UpdatedAt: time.Now(),
	}

	// Save person
	err := store.SavePerson(ctx, person)
	if err != nil {
		t.Fatalf("SavePerson() failed: %v", err)
	}

	// Delete person
	err = store.DeletePerson(ctx, person.ID)
	if err != nil {
		t.Fatalf("DeletePerson() failed: %v", err)
	}

	// Verify person is deleted
	retrieved, err := store.GetPerson(ctx, person.ID)
	if err != nil {
		t.Fatalf("GetPerson() after delete failed: %v", err)
	}

	if retrieved != nil {
		t.Errorf("GetPerson() after delete = %v, want nil", retrieved)
	}
}

func TestReadModelStore_ListPersons(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	// Create test persons
	persons := []*repository.PersonReadModel{
		{
			ID:        uuid.New(),
			GivenName: "Alice",
			Surname:   "Anderson",
			FullName:  "Alice Anderson",
			UpdatedAt: time.Now().Add(-3 * time.Hour),
		},
		{
			ID:        uuid.New(),
			GivenName: "Bob",
			Surname:   "Brown",
			FullName:  "Bob Brown",
			UpdatedAt: time.Now().Add(-2 * time.Hour),
		},
		{
			ID:        uuid.New(),
			GivenName: "Charlie",
			Surname:   "Clark",
			FullName:  "Charlie Clark",
			UpdatedAt: time.Now().Add(-1 * time.Hour),
		},
		{
			ID:        uuid.New(),
			GivenName: "David",
			Surname:   "Brown",
			FullName:  "David Brown",
			UpdatedAt: time.Now(),
		},
	}

	for _, p := range persons {
		err := store.SavePerson(ctx, p)
		if err != nil {
			t.Fatalf("SavePerson() failed: %v", err)
		}
	}

	tests := []struct {
		name       string
		opts       repository.ListOptions
		wantCount  int
		wantTotal  int
		wantFirst  string // surname of first result
		wantSecond string // surname of second result (if applicable)
	}{
		{
			name: "default sort by surname",
			opts: repository.ListOptions{
				Limit:  10,
				Offset: 0,
				Sort:   "surname",
				Order:  "asc",
			},
			wantCount:  4,
			wantTotal:  4,
			wantFirst:  "Anderson",
			wantSecond: "Brown",
		},
		{
			name: "sort by given_name",
			opts: repository.ListOptions{
				Limit:  10,
				Offset: 0,
				Sort:   "given_name",
				Order:  "asc",
			},
			wantCount:  4,
			wantTotal:  4,
			wantFirst:  "Anderson", // Alice Anderson
			wantSecond: "Brown",    // Bob Brown
		},
		{
			name: "sort by updated_at desc",
			opts: repository.ListOptions{
				Limit:  10,
				Offset: 0,
				Sort:   "updated_at",
				Order:  "desc",
			},
			wantCount:  4,
			wantTotal:  4,
			wantFirst:  "Brown", // David Brown (most recent)
			wantSecond: "Clark", // Charlie Clark
		},
		{
			name: "pagination first page",
			opts: repository.ListOptions{
				Limit:  2,
				Offset: 0,
				Sort:   "surname",
				Order:  "asc",
			},
			wantCount:  2,
			wantTotal:  4,
			wantFirst:  "Anderson",
			wantSecond: "Brown",
		},
		{
			name: "pagination second page",
			opts: repository.ListOptions{
				Limit:  2,
				Offset: 2,
				Sort:   "surname",
				Order:  "asc",
			},
			wantCount: 2,
			wantTotal: 4,
			wantFirst: "Brown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := store.ListPersons(ctx, tt.opts)
			if err != nil {
				t.Fatalf("ListPersons() failed: %v", err)
			}

			if len(results) != tt.wantCount {
				t.Errorf("len(results) = %d, want %d", len(results), tt.wantCount)
			}

			if total != tt.wantTotal {
				t.Errorf("total = %d, want %d", total, tt.wantTotal)
			}

			if len(results) > 0 {
				// Verify first result - always check surname
				firstValue := results[0].Surname
				if firstValue != tt.wantFirst {
					t.Errorf("first result surname = %s, want %s", firstValue, tt.wantFirst)
				}
			}

			if len(results) > 1 && tt.wantSecond != "" {
				// Verify second result - always check surname
				secondValue := results[1].Surname
				if secondValue != tt.wantSecond {
					t.Errorf("second result surname = %s, want %s", secondValue, tt.wantSecond)
				}
			}
		})
	}
}

func TestReadModelStore_ListPersonsWithBirthDates(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	birthDate1 := time.Date(1950, 1, 1, 0, 0, 0, 0, time.UTC)
	birthDate2 := time.Date(1960, 1, 1, 0, 0, 0, 0, time.UTC)

	persons := []*repository.PersonReadModel{
		{
			ID:            uuid.New(),
			GivenName:     "Alice",
			Surname:       "Anderson",
			FullName:      "Alice Anderson",
			BirthDateSort: &birthDate2,
			UpdatedAt:     time.Now(),
		},
		{
			ID:            uuid.New(),
			GivenName:     "Bob",
			Surname:       "Brown",
			FullName:      "Bob Brown",
			BirthDateSort: &birthDate1,
			UpdatedAt:     time.Now(),
		},
		{
			ID:        uuid.New(),
			GivenName: "Charlie",
			Surname:   "Clark",
			FullName:  "Charlie Clark",
			UpdatedAt: time.Now(),
		},
	}

	for _, p := range persons {
		err := store.SavePerson(ctx, p)
		if err != nil {
			t.Fatalf("SavePerson() failed: %v", err)
		}
	}

	// Sort by birth date ascending - nulls should come last
	results, _, err := store.ListPersons(ctx, repository.ListOptions{
		Limit:  10,
		Offset: 0,
		Sort:   "birth_date",
		Order:  "asc",
	})
	if err != nil {
		t.Fatalf("ListPersons() failed: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}

	// Bob (1950) should be first
	if results[0].GivenName != "Bob" {
		t.Errorf("first result = %s, want Bob", results[0].GivenName)
	}

	// Alice (1960) should be second
	if results[1].GivenName != "Alice" {
		t.Errorf("second result = %s, want Alice", results[1].GivenName)
	}

	// Charlie (null) should be last
	if results[2].GivenName != "Charlie" {
		t.Errorf("third result = %s, want Charlie", results[2].GivenName)
	}
}

func TestReadModelStore_SearchPersons(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	// Create test persons
	persons := []*repository.PersonReadModel{
		{
			ID:        uuid.New(),
			GivenName: "John",
			Surname:   "Smith",
			FullName:  "John Smith",
			UpdatedAt: time.Now(),
		},
		{
			ID:        uuid.New(),
			GivenName: "Jane",
			Surname:   "Smith",
			FullName:  "Jane Smith",
			UpdatedAt: time.Now(),
		},
		{
			ID:        uuid.New(),
			GivenName: "Bob",
			Surname:   "Johnson",
			FullName:  "Bob Johnson",
			UpdatedAt: time.Now(),
		},
	}

	for _, p := range persons {
		err := store.SavePerson(ctx, p)
		if err != nil {
			t.Fatalf("SavePerson() failed: %v", err)
		}
	}

	tests := []struct {
		name      string
		query     string
		limit     int
		wantCount int
		wantNames []string
	}{
		{
			name:      "search by surname",
			query:     "Smith",
			limit:     10,
			wantCount: 2,
			wantNames: []string{"John Smith", "Jane Smith"},
		},
		{
			name:      "search by given name",
			query:     "Jane",
			limit:     10,
			wantCount: 1,
			wantNames: []string{"Jane Smith"},
		},
		{
			name:      "search by partial name",
			query:     "jo",
			limit:     10,
			wantCount: 2,
			wantNames: []string{"John Smith", "Bob Johnson"},
		},
		{
			name:      "search case insensitive",
			query:     "SMITH",
			limit:     10,
			wantCount: 2,
			wantNames: []string{"John Smith", "Jane Smith"},
		},
		{
			name:      "search with limit",
			query:     "Smith",
			limit:     1,
			wantCount: 1,
		},
		{
			name:      "search no results",
			query:     "Nonexistent",
			limit:     10,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := store.SearchPersons(ctx, repository.SearchOptions{Query: tt.query, Limit: tt.limit})
			if err != nil {
				t.Fatalf("SearchPersons() failed: %v", err)
			}

			if len(results) != tt.wantCount {
				t.Errorf("len(results) = %d, want %d", len(results), tt.wantCount)
			}

			// Verify expected names are present (order not guaranteed for map iteration)
			if tt.wantNames != nil && len(results) > 0 {
				foundNames := make(map[string]bool)
				for _, r := range results {
					foundNames[r.FullName] = true
				}
				for _, wantName := range tt.wantNames {
					if !foundNames[wantName] {
						t.Errorf("expected to find %s in results", wantName)
					}
				}
			}
		})
	}
}

// Family CRUD operations

func TestReadModelStore_SaveAndGetFamily(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	partner1ID := uuid.New()
	partner2ID := uuid.New()
	family := &repository.FamilyReadModel{
		ID:               uuid.New(),
		Partner1ID:       &partner1ID,
		Partner1Name:     "John Doe",
		Partner2ID:       &partner2ID,
		Partner2Name:     "Jane Doe",
		RelationshipType: domain.RelationMarriage,
		ChildCount:       2,
		Version:          1,
		UpdatedAt:        time.Now(),
	}

	// Save family
	err := store.SaveFamily(ctx, family)
	if err != nil {
		t.Fatalf("SaveFamily() failed: %v", err)
	}

	// Get family
	retrieved, err := store.GetFamily(ctx, family.ID)
	if err != nil {
		t.Fatalf("GetFamily() failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetFamily() returned nil")
	}

	// Verify fields
	if retrieved.ID != family.ID {
		t.Errorf("ID = %v, want %v", retrieved.ID, family.ID)
	}
	if retrieved.Partner1ID == nil || *retrieved.Partner1ID != partner1ID {
		t.Errorf("Partner1ID = %v, want %v", retrieved.Partner1ID, partner1ID)
	}
	if retrieved.Partner2ID == nil || *retrieved.Partner2ID != partner2ID {
		t.Errorf("Partner2ID = %v, want %v", retrieved.Partner2ID, partner2ID)
	}
	if retrieved.RelationshipType != domain.RelationMarriage {
		t.Errorf("RelationshipType = %v, want %v", retrieved.RelationshipType, domain.RelationMarriage)
	}
	if retrieved.ChildCount != 2 {
		t.Errorf("ChildCount = %d, want 2", retrieved.ChildCount)
	}
}

func TestReadModelStore_GetFamilyNonExistent(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	nonExistentID := uuid.New()

	retrieved, err := store.GetFamily(ctx, nonExistentID)
	if err != nil {
		t.Fatalf("GetFamily() failed: %v", err)
	}

	if retrieved != nil {
		t.Errorf("GetFamily() for non-existent ID = %v, want nil", retrieved)
	}
}

func TestReadModelStore_DeleteFamily(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	familyID := uuid.New()
	family := &repository.FamilyReadModel{
		ID:        familyID,
		Version:   1,
		UpdatedAt: time.Now(),
	}

	// Save family with children
	err := store.SaveFamily(ctx, family)
	if err != nil {
		t.Fatalf("SaveFamily() failed: %v", err)
	}

	childID := uuid.New()
	child := &repository.FamilyChildReadModel{
		FamilyID:         familyID,
		PersonID:         childID,
		PersonName:       "Child",
		RelationshipType: domain.ChildBiological,
	}
	err = store.SaveFamilyChild(ctx, child)
	if err != nil {
		t.Fatalf("SaveFamilyChild() failed: %v", err)
	}

	// Delete family
	err = store.DeleteFamily(ctx, familyID)
	if err != nil {
		t.Fatalf("DeleteFamily() failed: %v", err)
	}

	// Verify family is deleted
	retrieved, err := store.GetFamily(ctx, familyID)
	if err != nil {
		t.Fatalf("GetFamily() after delete failed: %v", err)
	}
	if retrieved != nil {
		t.Errorf("GetFamily() after delete = %v, want nil", retrieved)
	}

	// Verify children are also deleted
	children, err := store.GetFamilyChildren(ctx, familyID)
	if err != nil {
		t.Fatalf("GetFamilyChildren() after delete failed: %v", err)
	}
	if len(children) != 0 {
		t.Errorf("len(children) after delete = %d, want 0", len(children))
	}
}

func TestReadModelStore_ListFamilies(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	// Create test families
	for i := 0; i < 5; i++ {
		family := &repository.FamilyReadModel{
			ID:        uuid.New(),
			Version:   1,
			UpdatedAt: time.Now(),
		}
		err := store.SaveFamily(ctx, family)
		if err != nil {
			t.Fatalf("SaveFamily() failed: %v", err)
		}
	}

	tests := []struct {
		name      string
		opts      repository.ListOptions
		wantCount int
		wantTotal int
	}{
		{
			name: "list all",
			opts: repository.ListOptions{
				Limit:  10,
				Offset: 0,
			},
			wantCount: 5,
			wantTotal: 5,
		},
		{
			name: "pagination first page",
			opts: repository.ListOptions{
				Limit:  3,
				Offset: 0,
			},
			wantCount: 3,
			wantTotal: 5,
		},
		{
			name: "pagination second page",
			opts: repository.ListOptions{
				Limit:  3,
				Offset: 3,
			},
			wantCount: 2,
			wantTotal: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := store.ListFamilies(ctx, tt.opts)
			if err != nil {
				t.Fatalf("ListFamilies() failed: %v", err)
			}

			if len(results) != tt.wantCount {
				t.Errorf("len(results) = %d, want %d", len(results), tt.wantCount)
			}

			if total != tt.wantTotal {
				t.Errorf("total = %d, want %d", total, tt.wantTotal)
			}
		})
	}
}

func TestReadModelStore_GetFamiliesForPerson(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	personID := uuid.New()
	otherPersonID := uuid.New()

	// Create families where person is partner
	family1 := &repository.FamilyReadModel{
		ID:         uuid.New(),
		Partner1ID: &personID,
		Version:    1,
		UpdatedAt:  time.Now(),
	}
	err := store.SaveFamily(ctx, family1)
	if err != nil {
		t.Fatalf("SaveFamily() 1 failed: %v", err)
	}

	family2 := &repository.FamilyReadModel{
		ID:         uuid.New(),
		Partner2ID: &personID,
		Version:    1,
		UpdatedAt:  time.Now(),
	}
	err = store.SaveFamily(ctx, family2)
	if err != nil {
		t.Fatalf("SaveFamily() 2 failed: %v", err)
	}

	// Create family where person is not involved
	family3 := &repository.FamilyReadModel{
		ID:         uuid.New(),
		Partner1ID: &otherPersonID,
		Version:    1,
		UpdatedAt:  time.Now(),
	}
	err = store.SaveFamily(ctx, family3)
	if err != nil {
		t.Fatalf("SaveFamily() 3 failed: %v", err)
	}

	// Get families for person
	families, err := store.GetFamiliesForPerson(ctx, personID)
	if err != nil {
		t.Fatalf("GetFamiliesForPerson() failed: %v", err)
	}

	if len(families) != 2 {
		t.Fatalf("len(families) = %d, want 2", len(families))
	}

	// Verify correct families are returned
	foundIDs := make(map[uuid.UUID]bool)
	for _, f := range families {
		foundIDs[f.ID] = true
	}

	if !foundIDs[family1.ID] {
		t.Errorf("expected to find family1 in results")
	}
	if !foundIDs[family2.ID] {
		t.Errorf("expected to find family2 in results")
	}
	if foundIDs[family3.ID] {
		t.Errorf("did not expect to find family3 in results")
	}
}

// FamilyChild operations

func TestReadModelStore_SaveAndGetFamilyChildren(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	familyID := uuid.New()
	childID := uuid.New()

	child := &repository.FamilyChildReadModel{
		FamilyID:         familyID,
		PersonID:         childID,
		PersonName:       "Child Doe",
		RelationshipType: domain.ChildBiological,
	}

	// Save family child
	err := store.SaveFamilyChild(ctx, child)
	if err != nil {
		t.Fatalf("SaveFamilyChild() failed: %v", err)
	}

	// Get family children
	children, err := store.GetFamilyChildren(ctx, familyID)
	if err != nil {
		t.Fatalf("GetFamilyChildren() failed: %v", err)
	}

	if len(children) != 1 {
		t.Fatalf("len(children) = %d, want 1", len(children))
	}

	retrieved := children[0]
	if retrieved.FamilyID != familyID {
		t.Errorf("FamilyID = %v, want %v", retrieved.FamilyID, familyID)
	}
	if retrieved.PersonID != childID {
		t.Errorf("PersonID = %v, want %v", retrieved.PersonID, childID)
	}
	if retrieved.RelationshipType != domain.ChildBiological {
		t.Errorf("RelationshipType = %v, want %v", retrieved.RelationshipType, domain.ChildBiological)
	}
}

func TestReadModelStore_SaveFamilyChildUpdate(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	familyID := uuid.New()
	childID := uuid.New()

	child := &repository.FamilyChildReadModel{
		FamilyID:         familyID,
		PersonID:         childID,
		PersonName:       "Child Doe",
		RelationshipType: domain.ChildBiological,
	}

	// Save initial
	err := store.SaveFamilyChild(ctx, child)
	if err != nil {
		t.Fatalf("SaveFamilyChild() failed: %v", err)
	}

	// Update relationship type
	child.RelationshipType = domain.ChildAdopted
	err = store.SaveFamilyChild(ctx, child)
	if err != nil {
		t.Fatalf("SaveFamilyChild() update failed: %v", err)
	}

	// Verify update
	children, err := store.GetFamilyChildren(ctx, familyID)
	if err != nil {
		t.Fatalf("GetFamilyChildren() failed: %v", err)
	}

	if len(children) != 1 {
		t.Fatalf("len(children) = %d, want 1", len(children))
	}

	if children[0].RelationshipType != domain.ChildAdopted {
		t.Errorf("RelationshipType = %v, want %v", children[0].RelationshipType, domain.ChildAdopted)
	}
}

func TestReadModelStore_DeleteFamilyChild(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	familyID := uuid.New()
	child1ID := uuid.New()
	child2ID := uuid.New()

	// Add two children
	child1 := &repository.FamilyChildReadModel{
		FamilyID:         familyID,
		PersonID:         child1ID,
		PersonName:       "Child 1",
		RelationshipType: domain.ChildBiological,
	}
	err := store.SaveFamilyChild(ctx, child1)
	if err != nil {
		t.Fatalf("SaveFamilyChild() 1 failed: %v", err)
	}

	child2 := &repository.FamilyChildReadModel{
		FamilyID:         familyID,
		PersonID:         child2ID,
		PersonName:       "Child 2",
		RelationshipType: domain.ChildBiological,
	}
	err = store.SaveFamilyChild(ctx, child2)
	if err != nil {
		t.Fatalf("SaveFamilyChild() 2 failed: %v", err)
	}

	// Delete one child
	err = store.DeleteFamilyChild(ctx, familyID, child1ID)
	if err != nil {
		t.Fatalf("DeleteFamilyChild() failed: %v", err)
	}

	// Verify only one child remains
	children, err := store.GetFamilyChildren(ctx, familyID)
	if err != nil {
		t.Fatalf("GetFamilyChildren() failed: %v", err)
	}

	if len(children) != 1 {
		t.Fatalf("len(children) = %d, want 1", len(children))
	}

	if children[0].PersonID != child2ID {
		t.Errorf("remaining child PersonID = %v, want %v", children[0].PersonID, child2ID)
	}
}

func TestReadModelStore_GetChildrenOfFamily(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	familyID := uuid.New()
	child1ID := uuid.New()
	child2ID := uuid.New()

	// Save persons
	person1 := &repository.PersonReadModel{
		ID:        child1ID,
		GivenName: "Alice",
		Surname:   "Doe",
		FullName:  "Alice Doe",
		UpdatedAt: time.Now(),
	}
	err := store.SavePerson(ctx, person1)
	if err != nil {
		t.Fatalf("SavePerson() 1 failed: %v", err)
	}

	person2 := &repository.PersonReadModel{
		ID:        child2ID,
		GivenName: "Bob",
		Surname:   "Doe",
		FullName:  "Bob Doe",
		UpdatedAt: time.Now(),
	}
	err = store.SavePerson(ctx, person2)
	if err != nil {
		t.Fatalf("SavePerson() 2 failed: %v", err)
	}

	// Save family children
	child1 := &repository.FamilyChildReadModel{
		FamilyID:         familyID,
		PersonID:         child1ID,
		PersonName:       "Alice Doe",
		RelationshipType: domain.ChildBiological,
	}
	err = store.SaveFamilyChild(ctx, child1)
	if err != nil {
		t.Fatalf("SaveFamilyChild() 1 failed: %v", err)
	}

	child2 := &repository.FamilyChildReadModel{
		FamilyID:         familyID,
		PersonID:         child2ID,
		PersonName:       "Bob Doe",
		RelationshipType: domain.ChildBiological,
	}
	err = store.SaveFamilyChild(ctx, child2)
	if err != nil {
		t.Fatalf("SaveFamilyChild() 2 failed: %v", err)
	}

	// Get children of family
	children, err := store.GetChildrenOfFamily(ctx, familyID)
	if err != nil {
		t.Fatalf("GetChildrenOfFamily() failed: %v", err)
	}

	if len(children) != 2 {
		t.Fatalf("len(children) = %d, want 2", len(children))
	}

	// Verify both persons are returned
	foundIDs := make(map[uuid.UUID]bool)
	for _, c := range children {
		foundIDs[c.ID] = true
	}

	if !foundIDs[child1ID] {
		t.Errorf("expected to find child1 in results")
	}
	if !foundIDs[child2ID] {
		t.Errorf("expected to find child2 in results")
	}
}

func TestReadModelStore_GetChildFamily(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	familyID := uuid.New()
	childID := uuid.New()
	otherChildID := uuid.New()

	// Save family
	family := &repository.FamilyReadModel{
		ID:        familyID,
		Version:   1,
		UpdatedAt: time.Now(),
	}
	err := store.SaveFamily(ctx, family)
	if err != nil {
		t.Fatalf("SaveFamily() failed: %v", err)
	}

	// Save family child
	child := &repository.FamilyChildReadModel{
		FamilyID:         familyID,
		PersonID:         childID,
		PersonName:       "Child Doe",
		RelationshipType: domain.ChildBiological,
	}
	err = store.SaveFamilyChild(ctx, child)
	if err != nil {
		t.Fatalf("SaveFamilyChild() failed: %v", err)
	}

	// Get child family
	retrievedFamily, err := store.GetChildFamily(ctx, childID)
	if err != nil {
		t.Fatalf("GetChildFamily() failed: %v", err)
	}

	if retrievedFamily == nil {
		t.Fatal("GetChildFamily() returned nil")
	}

	if retrievedFamily.ID != familyID {
		t.Errorf("Family ID = %v, want %v", retrievedFamily.ID, familyID)
	}

	// Get child family for person not in any family
	retrievedFamily, err = store.GetChildFamily(ctx, otherChildID)
	if err != nil {
		t.Fatalf("GetChildFamily() for non-child failed: %v", err)
	}

	if retrievedFamily != nil {
		t.Errorf("GetChildFamily() for non-child = %v, want nil", retrievedFamily)
	}
}

// PedigreeEdge operations

func TestReadModelStore_SaveAndGetPedigreeEdge(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	personID := uuid.New()
	fatherID := uuid.New()
	motherID := uuid.New()

	edge := &repository.PedigreeEdge{
		PersonID:   personID,
		FatherID:   &fatherID,
		MotherID:   &motherID,
		FatherName: "John Doe",
		MotherName: "Jane Doe",
	}

	// Save pedigree edge
	err := store.SavePedigreeEdge(ctx, edge)
	if err != nil {
		t.Fatalf("SavePedigreeEdge() failed: %v", err)
	}

	// Get pedigree edge
	retrieved, err := store.GetPedigreeEdge(ctx, personID)
	if err != nil {
		t.Fatalf("GetPedigreeEdge() failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetPedigreeEdge() returned nil")
	}

	// Verify fields
	if retrieved.PersonID != personID {
		t.Errorf("PersonID = %v, want %v", retrieved.PersonID, personID)
	}
	if retrieved.FatherID == nil || *retrieved.FatherID != fatherID {
		t.Errorf("FatherID = %v, want %v", retrieved.FatherID, fatherID)
	}
	if retrieved.MotherID == nil || *retrieved.MotherID != motherID {
		t.Errorf("MotherID = %v, want %v", retrieved.MotherID, motherID)
	}
	if retrieved.FatherName != "John Doe" {
		t.Errorf("FatherName = %s, want John Doe", retrieved.FatherName)
	}
	if retrieved.MotherName != "Jane Doe" {
		t.Errorf("MotherName = %s, want Jane Doe", retrieved.MotherName)
	}
}

func TestReadModelStore_GetPedigreeEdgeNonExistent(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	nonExistentID := uuid.New()

	retrieved, err := store.GetPedigreeEdge(ctx, nonExistentID)
	if err != nil {
		t.Fatalf("GetPedigreeEdge() failed: %v", err)
	}

	if retrieved != nil {
		t.Errorf("GetPedigreeEdge() for non-existent ID = %v, want nil", retrieved)
	}
}

func TestReadModelStore_DeletePedigreeEdge(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	personID := uuid.New()
	fatherID := uuid.New()

	edge := &repository.PedigreeEdge{
		PersonID: personID,
		FatherID: &fatherID,
	}

	// Save pedigree edge
	err := store.SavePedigreeEdge(ctx, edge)
	if err != nil {
		t.Fatalf("SavePedigreeEdge() failed: %v", err)
	}

	// Delete pedigree edge
	err = store.DeletePedigreeEdge(ctx, personID)
	if err != nil {
		t.Fatalf("DeletePedigreeEdge() failed: %v", err)
	}

	// Verify edge is deleted
	retrieved, err := store.GetPedigreeEdge(ctx, personID)
	if err != nil {
		t.Fatalf("GetPedigreeEdge() after delete failed: %v", err)
	}

	if retrieved != nil {
		t.Errorf("GetPedigreeEdge() after delete = %v, want nil", retrieved)
	}
}

// Reset operation

func TestReadModelStore_Reset(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	// Add data
	personID := uuid.New()
	person := &repository.PersonReadModel{
		ID:        personID,
		GivenName: "John",
		Surname:   "Doe",
		FullName:  "John Doe",
		UpdatedAt: time.Now(),
	}
	err := store.SavePerson(ctx, person)
	if err != nil {
		t.Fatalf("SavePerson() failed: %v", err)
	}

	familyID := uuid.New()
	family := &repository.FamilyReadModel{
		ID:        familyID,
		Version:   1,
		UpdatedAt: time.Now(),
	}
	err = store.SaveFamily(ctx, family)
	if err != nil {
		t.Fatalf("SaveFamily() failed: %v", err)
	}

	child := &repository.FamilyChildReadModel{
		FamilyID:         familyID,
		PersonID:         personID,
		PersonName:       "John Doe",
		RelationshipType: domain.ChildBiological,
	}
	err = store.SaveFamilyChild(ctx, child)
	if err != nil {
		t.Fatalf("SaveFamilyChild() failed: %v", err)
	}

	edge := &repository.PedigreeEdge{
		PersonID: personID,
	}
	err = store.SavePedigreeEdge(ctx, edge)
	if err != nil {
		t.Fatalf("SavePedigreeEdge() failed: %v", err)
	}

	// Reset
	store.Reset()

	// Verify everything is cleared
	retrievedPerson, err := store.GetPerson(ctx, personID)
	if err != nil {
		t.Fatalf("GetPerson() after reset failed: %v", err)
	}
	if retrievedPerson != nil {
		t.Errorf("GetPerson() after reset = %v, want nil", retrievedPerson)
	}

	retrievedFamily, err := store.GetFamily(ctx, familyID)
	if err != nil {
		t.Fatalf("GetFamily() after reset failed: %v", err)
	}
	if retrievedFamily != nil {
		t.Errorf("GetFamily() after reset = %v, want nil", retrievedFamily)
	}

	children, err := store.GetFamilyChildren(ctx, familyID)
	if err != nil {
		t.Fatalf("GetFamilyChildren() after reset failed: %v", err)
	}
	if len(children) != 0 {
		t.Errorf("len(children) after reset = %d, want 0", len(children))
	}

	retrievedEdge, err := store.GetPedigreeEdge(ctx, personID)
	if err != nil {
		t.Fatalf("GetPedigreeEdge() after reset failed: %v", err)
	}
	if retrievedEdge != nil {
		t.Errorf("GetPedigreeEdge() after reset = %v, want nil", retrievedEdge)
	}
}

// Source CRUD operations

func TestReadModelStore_SaveAndGetSource(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	source := &repository.SourceReadModel{
		ID:         uuid.New(),
		SourceType: domain.SourceBook,
		Title:      "Census of 1900",
		Author:     "US Census Bureau",
		Publisher:  "NARA",
		Version:    1,
		UpdatedAt:  time.Now(),
	}

	// Save source
	err := store.SaveSource(ctx, source)
	if err != nil {
		t.Fatalf("SaveSource() failed: %v", err)
	}

	// Get source
	retrieved, err := store.GetSource(ctx, source.ID)
	if err != nil {
		t.Fatalf("GetSource() failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetSource() returned nil")
	}

	// Verify fields
	if retrieved.ID != source.ID {
		t.Errorf("ID = %v, want %v", retrieved.ID, source.ID)
	}
	if retrieved.Title != source.Title {
		t.Errorf("Title = %s, want %s", retrieved.Title, source.Title)
	}
	if retrieved.SourceType != source.SourceType {
		t.Errorf("SourceType = %s, want %s", retrieved.SourceType, source.SourceType)
	}
}

func TestReadModelStore_GetSourceNonExistent(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	nonExistentID := uuid.New()

	retrieved, err := store.GetSource(ctx, nonExistentID)
	if err != nil {
		t.Fatalf("GetSource() failed: %v", err)
	}

	if retrieved != nil {
		t.Errorf("GetSource() for non-existent ID = %v, want nil", retrieved)
	}
}

func TestReadModelStore_ListSources(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	// Create test sources
	sources := []*repository.SourceReadModel{
		{
			ID:         uuid.New(),
			SourceType: domain.SourceBook,
			Title:      "Book A",
			UpdatedAt:  time.Now().Add(-3 * time.Hour),
		},
		{
			ID:         uuid.New(),
			SourceType: domain.SourceCensus,
			Title:      "Census 1900",
			UpdatedAt:  time.Now().Add(-2 * time.Hour),
		},
		{
			ID:         uuid.New(),
			SourceType: domain.SourceBook,
			Title:      "Book B",
			UpdatedAt:  time.Now().Add(-1 * time.Hour),
		},
	}

	for _, s := range sources {
		err := store.SaveSource(ctx, s)
		if err != nil {
			t.Fatalf("SaveSource() failed: %v", err)
		}
	}

	tests := []struct {
		name      string
		opts      repository.ListOptions
		wantCount int
		wantTotal int
	}{
		{
			name: "list all",
			opts: repository.ListOptions{
				Limit:  10,
				Offset: 0,
				Sort:   "title",
				Order:  "asc",
			},
			wantCount: 3,
			wantTotal: 3,
		},
		{
			name: "pagination first page",
			opts: repository.ListOptions{
				Limit:  2,
				Offset: 0,
				Sort:   "title",
				Order:  "asc",
			},
			wantCount: 2,
			wantTotal: 3,
		},
		{
			name: "pagination second page",
			opts: repository.ListOptions{
				Limit:  2,
				Offset: 2,
				Sort:   "title",
				Order:  "asc",
			},
			wantCount: 1,
			wantTotal: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := store.ListSources(ctx, tt.opts)
			if err != nil {
				t.Fatalf("ListSources() failed: %v", err)
			}

			if len(results) != tt.wantCount {
				t.Errorf("len(results) = %d, want %d", len(results), tt.wantCount)
			}

			if total != tt.wantTotal {
				t.Errorf("total = %d, want %d", total, tt.wantTotal)
			}
		})
	}
}

func TestReadModelStore_SearchSources(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	// Create test sources
	sources := []*repository.SourceReadModel{
		{
			ID:         uuid.New(),
			SourceType: domain.SourceBook,
			Title:      "Census of 1900",
			Author:     "US Census Bureau",
			UpdatedAt:  time.Now(),
		},
		{
			ID:         uuid.New(),
			SourceType: domain.SourceBook,
			Title:      "History of Springfield",
			Author:     "John Smith",
			UpdatedAt:  time.Now(),
		},
		{
			ID:         uuid.New(),
			SourceType: domain.SourceCensus,
			Title:      "Census of 1910",
			UpdatedAt:  time.Now(),
		},
	}

	for _, s := range sources {
		err := store.SaveSource(ctx, s)
		if err != nil {
			t.Fatalf("SaveSource() failed: %v", err)
		}
	}

	tests := []struct {
		name      string
		query     string
		limit     int
		wantCount int
	}{
		{
			name:      "search by title",
			query:     "Census",
			limit:     10,
			wantCount: 2,
		},
		{
			name:      "search by author",
			query:     "Smith",
			limit:     10,
			wantCount: 1,
		},
		{
			name:      "search case insensitive",
			query:     "CENSUS",
			limit:     10,
			wantCount: 2,
		},
		{
			name:      "search with limit",
			query:     "Census",
			limit:     1,
			wantCount: 1,
		},
		{
			name:      "search no results",
			query:     "Nonexistent",
			limit:     10,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := store.SearchSources(ctx, tt.query, tt.limit)
			if err != nil {
				t.Fatalf("SearchSources() failed: %v", err)
			}

			if len(results) != tt.wantCount {
				t.Errorf("len(results) = %d, want %d", len(results), tt.wantCount)
			}
		})
	}
}

func TestReadModelStore_DeleteSource(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	source := &repository.SourceReadModel{
		ID:         uuid.New(),
		SourceType: domain.SourceBook,
		Title:      "Test Source",
		UpdatedAt:  time.Now(),
	}

	// Save source
	err := store.SaveSource(ctx, source)
	if err != nil {
		t.Fatalf("SaveSource() failed: %v", err)
	}

	// Delete source
	err = store.DeleteSource(ctx, source.ID)
	if err != nil {
		t.Fatalf("DeleteSource() failed: %v", err)
	}

	// Verify deleted
	retrieved, err := store.GetSource(ctx, source.ID)
	if err != nil {
		t.Fatalf("GetSource() after delete failed: %v", err)
	}

	if retrieved != nil {
		t.Errorf("GetSource() after delete = %v, want nil", retrieved)
	}
}

// Citation CRUD operations

func TestReadModelStore_SaveAndGetCitation(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	sourceID := uuid.New()
	factOwnerID := uuid.New()

	citation := &repository.CitationReadModel{
		ID:            uuid.New(),
		SourceID:      sourceID,
		FactType:      domain.FactPersonBirth,
		FactOwnerID:   factOwnerID,
		Page:          "123",
		SourceQuality: domain.SourceOriginal,
		Version:       1,
	}

	// Save citation
	err := store.SaveCitation(ctx, citation)
	if err != nil {
		t.Fatalf("SaveCitation() failed: %v", err)
	}

	// Get citation
	retrieved, err := store.GetCitation(ctx, citation.ID)
	if err != nil {
		t.Fatalf("GetCitation() failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetCitation() returned nil")
	}

	// Verify fields
	if retrieved.ID != citation.ID {
		t.Errorf("ID = %v, want %v", retrieved.ID, citation.ID)
	}
	if retrieved.SourceID != sourceID {
		t.Errorf("SourceID = %v, want %v", retrieved.SourceID, sourceID)
	}
	if retrieved.FactType != domain.FactPersonBirth {
		t.Errorf("FactType = %s, want %s", retrieved.FactType, domain.FactPersonBirth)
	}
}

func TestReadModelStore_GetCitationNonExistent(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	nonExistentID := uuid.New()

	retrieved, err := store.GetCitation(ctx, nonExistentID)
	if err != nil {
		t.Fatalf("GetCitation() failed: %v", err)
	}

	if retrieved != nil {
		t.Errorf("GetCitation() for non-existent ID = %v, want nil", retrieved)
	}
}

func TestReadModelStore_GetCitationsForSource(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	sourceID := uuid.New()
	otherSourceID := uuid.New()
	factOwnerID := uuid.New()

	// Create citations for source
	citation1 := &repository.CitationReadModel{
		ID:          uuid.New(),
		SourceID:    sourceID,
		FactType:    domain.FactPersonBirth,
		FactOwnerID: factOwnerID,
	}
	citation2 := &repository.CitationReadModel{
		ID:          uuid.New(),
		SourceID:    sourceID,
		FactType:    domain.FactPersonDeath,
		FactOwnerID: factOwnerID,
	}
	citation3 := &repository.CitationReadModel{
		ID:          uuid.New(),
		SourceID:    otherSourceID,
		FactType:    domain.FactPersonBirth,
		FactOwnerID: factOwnerID,
	}

	store.SaveCitation(ctx, citation1)
	store.SaveCitation(ctx, citation2)
	store.SaveCitation(ctx, citation3)

	// Get citations for source
	citations, err := store.GetCitationsForSource(ctx, sourceID)
	if err != nil {
		t.Fatalf("GetCitationsForSource() failed: %v", err)
	}

	if len(citations) != 2 {
		t.Errorf("len(citations) = %d, want 2", len(citations))
	}

	// Verify correct citations returned
	foundIDs := make(map[uuid.UUID]bool)
	for _, c := range citations {
		foundIDs[c.ID] = true
	}

	if !foundIDs[citation1.ID] {
		t.Error("expected to find citation1")
	}
	if !foundIDs[citation2.ID] {
		t.Error("expected to find citation2")
	}
	if foundIDs[citation3.ID] {
		t.Error("did not expect to find citation3")
	}
}

func TestReadModelStore_GetCitationsForPerson(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	sourceID := uuid.New()
	personID := uuid.New()
	otherPersonID := uuid.New()

	// Create citations for person
	citation1 := &repository.CitationReadModel{
		ID:          uuid.New(),
		SourceID:    sourceID,
		FactType:    domain.FactPersonBirth,
		FactOwnerID: personID,
	}
	citation2 := &repository.CitationReadModel{
		ID:          uuid.New(),
		SourceID:    sourceID,
		FactType:    domain.FactPersonDeath,
		FactOwnerID: personID,
	}
	citation3 := &repository.CitationReadModel{
		ID:          uuid.New(),
		SourceID:    sourceID,
		FactType:    domain.FactPersonBirth,
		FactOwnerID: otherPersonID,
	}

	store.SaveCitation(ctx, citation1)
	store.SaveCitation(ctx, citation2)
	store.SaveCitation(ctx, citation3)

	// Get citations for person
	citations, err := store.GetCitationsForPerson(ctx, personID)
	if err != nil {
		t.Fatalf("GetCitationsForPerson() failed: %v", err)
	}

	if len(citations) != 2 {
		t.Errorf("len(citations) = %d, want 2", len(citations))
	}
}

func TestReadModelStore_GetCitationsForFact(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	sourceID := uuid.New()
	personID := uuid.New()
	otherPersonID := uuid.New()

	// Create citations for person's birth fact
	citation1 := &repository.CitationReadModel{
		ID:          uuid.New(),
		SourceID:    sourceID,
		FactType:    domain.FactPersonBirth,
		FactOwnerID: personID,
		Page:        "10",
	}
	citation2 := &repository.CitationReadModel{
		ID:          uuid.New(),
		SourceID:    sourceID,
		FactType:    domain.FactPersonBirth,
		FactOwnerID: personID,
		Page:        "11",
	}
	// Create citation for same person but different fact
	citation3 := &repository.CitationReadModel{
		ID:          uuid.New(),
		SourceID:    sourceID,
		FactType:    domain.FactPersonDeath,
		FactOwnerID: personID,
	}
	// Create citation for different person's birth
	citation4 := &repository.CitationReadModel{
		ID:          uuid.New(),
		SourceID:    sourceID,
		FactType:    domain.FactPersonBirth,
		FactOwnerID: otherPersonID,
	}

	store.SaveCitation(ctx, citation1)
	store.SaveCitation(ctx, citation2)
	store.SaveCitation(ctx, citation3)
	store.SaveCitation(ctx, citation4)

	// Get citations for person's birth fact
	citations, err := store.GetCitationsForFact(ctx, domain.FactPersonBirth, personID)
	if err != nil {
		t.Fatalf("GetCitationsForFact() failed: %v", err)
	}

	if len(citations) != 2 {
		t.Errorf("len(citations) = %d, want 2", len(citations))
	}

	// Verify correct citations returned
	foundIDs := make(map[uuid.UUID]bool)
	for _, c := range citations {
		foundIDs[c.ID] = true
		// All returned citations should match the fact type and owner
		if c.FactType != domain.FactPersonBirth {
			t.Errorf("FactType = %s, want %s", c.FactType, domain.FactPersonBirth)
		}
		if c.FactOwnerID != personID {
			t.Errorf("FactOwnerID = %v, want %v", c.FactOwnerID, personID)
		}
	}

	if !foundIDs[citation1.ID] {
		t.Error("expected to find citation1")
	}
	if !foundIDs[citation2.ID] {
		t.Error("expected to find citation2")
	}
	if foundIDs[citation3.ID] {
		t.Error("did not expect to find citation3 (different fact type)")
	}
	if foundIDs[citation4.ID] {
		t.Error("did not expect to find citation4 (different person)")
	}
}

func TestReadModelStore_DeleteCitation(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	citation := &repository.CitationReadModel{
		ID:          uuid.New(),
		SourceID:    uuid.New(),
		FactType:    domain.FactPersonBirth,
		FactOwnerID: uuid.New(),
	}

	// Save citation
	err := store.SaveCitation(ctx, citation)
	if err != nil {
		t.Fatalf("SaveCitation() failed: %v", err)
	}

	// Delete citation
	err = store.DeleteCitation(ctx, citation.ID)
	if err != nil {
		t.Fatalf("DeleteCitation() failed: %v", err)
	}

	// Verify deleted
	retrieved, err := store.GetCitation(ctx, citation.ID)
	if err != nil {
		t.Fatalf("GetCitation() after delete failed: %v", err)
	}

	if retrieved != nil {
		t.Errorf("GetCitation() after delete = %v, want nil", retrieved)
	}
}

// Media CRUD operations

func TestReadModelStore_SaveAndGetMedia(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	entityID := uuid.New()
	media := &repository.MediaReadModel{
		ID:            uuid.New(),
		EntityType:    "person",
		EntityID:      entityID,
		Title:         "Test Photo",
		Description:   "A test photo",
		MimeType:      "image/jpeg",
		MediaType:     domain.MediaPhoto,
		Filename:      "test.jpg",
		FileSize:      1024,
		FileData:      []byte("fake image data"),
		ThumbnailData: []byte("fake thumbnail"),
		Version:       1,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Save media
	err := store.SaveMedia(ctx, media)
	if err != nil {
		t.Fatalf("SaveMedia() failed: %v", err)
	}

	// Get media (metadata only)
	retrieved, err := store.GetMedia(ctx, media.ID)
	if err != nil {
		t.Fatalf("GetMedia() failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetMedia() returned nil")
	}

	if retrieved.Title != media.Title {
		t.Errorf("Title = %s, want %s", retrieved.Title, media.Title)
	}

	// GetMedia should NOT include binary data
	if len(retrieved.FileData) > 0 {
		t.Error("GetMedia() should not include FileData")
	}
	if len(retrieved.ThumbnailData) > 0 {
		t.Error("GetMedia() should not include ThumbnailData")
	}
}

func TestReadModelStore_GetMediaWithData(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	entityID := uuid.New()
	media := &repository.MediaReadModel{
		ID:            uuid.New(),
		EntityType:    "person",
		EntityID:      entityID,
		Title:         "Test Photo",
		MimeType:      "image/jpeg",
		FileData:      []byte("fake image data"),
		ThumbnailData: []byte("fake thumbnail"),
		Version:       1,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Save media
	err := store.SaveMedia(ctx, media)
	if err != nil {
		t.Fatalf("SaveMedia() failed: %v", err)
	}

	// Get media with data
	retrieved, err := store.GetMediaWithData(ctx, media.ID)
	if err != nil {
		t.Fatalf("GetMediaWithData() failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetMediaWithData() returned nil")
	}

	// Should include binary data
	if len(retrieved.FileData) == 0 {
		t.Error("GetMediaWithData() should include FileData")
	}
	if len(retrieved.ThumbnailData) == 0 {
		t.Error("GetMediaWithData() should include ThumbnailData")
	}
}

func TestReadModelStore_GetMediaThumbnail(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	entityID := uuid.New()
	thumbnailData := []byte("fake thumbnail data")
	media := &repository.MediaReadModel{
		ID:            uuid.New(),
		EntityType:    "person",
		EntityID:      entityID,
		Title:         "Test Photo",
		ThumbnailData: thumbnailData,
		Version:       1,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Save media
	err := store.SaveMedia(ctx, media)
	if err != nil {
		t.Fatalf("SaveMedia() failed: %v", err)
	}

	// Get thumbnail
	retrieved, err := store.GetMediaThumbnail(ctx, media.ID)
	if err != nil {
		t.Fatalf("GetMediaThumbnail() failed: %v", err)
	}

	if !bytes.Equal(retrieved, thumbnailData) {
		t.Errorf("GetMediaThumbnail() = %s, want %s", retrieved, thumbnailData)
	}

	// Non-existent media
	retrieved, err = store.GetMediaThumbnail(ctx, uuid.New())
	if err != nil {
		t.Fatalf("GetMediaThumbnail() for non-existent failed: %v", err)
	}
	if retrieved != nil {
		t.Error("GetMediaThumbnail() for non-existent should return nil")
	}
}

func TestReadModelStore_ListMediaForEntity(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	entityID := uuid.New()
	otherEntityID := uuid.New()

	// Create media for entity
	for i := 0; i < 5; i++ {
		media := &repository.MediaReadModel{
			ID:         uuid.New(),
			EntityType: "person",
			EntityID:   entityID,
			Title:      "Photo " + string(rune('A'+i)),
			Version:    1,
			CreatedAt:  time.Now().Add(time.Duration(i) * time.Hour),
			UpdatedAt:  time.Now(),
		}
		_ = store.SaveMedia(ctx, media)
	}

	// Create media for different entity
	otherMedia := &repository.MediaReadModel{
		ID:         uuid.New(),
		EntityType: "person",
		EntityID:   otherEntityID,
		Title:      "Other Photo",
		Version:    1,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	_ = store.SaveMedia(ctx, otherMedia)

	// List all for entity
	results, total, err := store.ListMediaForEntity(ctx, "person", entityID, repository.ListOptions{})
	if err != nil {
		t.Fatalf("ListMediaForEntity() failed: %v", err)
	}

	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}

	if len(results) != 5 {
		t.Errorf("len(results) = %d, want 5", len(results))
	}

	// Results should not include binary data
	for _, r := range results {
		if len(r.FileData) > 0 {
			t.Error("ListMediaForEntity() should not include FileData")
		}
	}

	// List with pagination
	results, total, err = store.ListMediaForEntity(ctx, "person", entityID, repository.ListOptions{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf("ListMediaForEntity() with pagination failed: %v", err)
	}

	if total != 5 {
		t.Errorf("total with pagination = %d, want 5", total)
	}

	if len(results) != 2 {
		t.Errorf("len(results) with limit = %d, want 2", len(results))
	}
}

func TestReadModelStore_DeleteMedia(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	media := &repository.MediaReadModel{
		ID:         uuid.New(),
		EntityType: "person",
		EntityID:   uuid.New(),
		Title:      "To Delete",
		Version:    1,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Save media
	err := store.SaveMedia(ctx, media)
	if err != nil {
		t.Fatalf("SaveMedia() failed: %v", err)
	}

	// Delete media
	err = store.DeleteMedia(ctx, media.ID)
	if err != nil {
		t.Fatalf("DeleteMedia() failed: %v", err)
	}

	// Verify deleted
	retrieved, err := store.GetMedia(ctx, media.ID)
	if err != nil {
		t.Fatalf("GetMedia() after delete failed: %v", err)
	}

	if retrieved != nil {
		t.Errorf("GetMedia() after delete = %v, want nil", retrieved)
	}
}

func TestReadModelStore_GetMedia_NotFound(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	// Get non-existent media
	retrieved, err := store.GetMedia(ctx, uuid.New())
	if err != nil {
		t.Fatalf("GetMedia() for non-existent failed: %v", err)
	}

	if retrieved != nil {
		t.Error("GetMedia() for non-existent should return nil")
	}

	// GetMediaWithData for non-existent
	retrieved, err = store.GetMediaWithData(ctx, uuid.New())
	if err != nil {
		t.Fatalf("GetMediaWithData() for non-existent failed: %v", err)
	}

	if retrieved != nil {
		t.Error("GetMediaWithData() for non-existent should return nil")
	}
}

// Event CRUD operations

func TestReadModelStore_SaveAndGetEvent(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	personID := uuid.New()
	event := &repository.EventReadModel{
		ID:          uuid.New(),
		OwnerType:   "person",
		OwnerID:     personID,
		FactType:    domain.FactPersonBurial,
		DateRaw:     "15 JAN 1920",
		Place:       "Springfield Cemetery, IL",
		Description: "Burial service",
		Cause:       "Natural causes",
		Version:     1,
		CreatedAt:   time.Now(),
	}

	// Save event
	err := store.SaveEvent(ctx, event)
	if err != nil {
		t.Fatalf("SaveEvent() failed: %v", err)
	}

	// Get event
	retrieved, err := store.GetEvent(ctx, event.ID)
	if err != nil {
		t.Fatalf("GetEvent() failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetEvent() returned nil")
	}

	// Verify fields
	if retrieved.ID != event.ID {
		t.Errorf("ID = %v, want %v", retrieved.ID, event.ID)
	}
	if retrieved.OwnerType != "person" {
		t.Errorf("OwnerType = %s, want person", retrieved.OwnerType)
	}
	if retrieved.OwnerID != personID {
		t.Errorf("OwnerID = %v, want %v", retrieved.OwnerID, personID)
	}
	if retrieved.FactType != domain.FactPersonBurial {
		t.Errorf("FactType = %s, want %s", retrieved.FactType, domain.FactPersonBurial)
	}
	if retrieved.DateRaw != "15 JAN 1920" {
		t.Errorf("DateRaw = %s, want 15 JAN 1920", retrieved.DateRaw)
	}
	if retrieved.Place != "Springfield Cemetery, IL" {
		t.Errorf("Place = %s, want Springfield Cemetery, IL", retrieved.Place)
	}
	if retrieved.Cause != "Natural causes" {
		t.Errorf("Cause = %s, want Natural causes", retrieved.Cause)
	}
}

func TestReadModelStore_GetEventNonExistent(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	nonExistentID := uuid.New()

	retrieved, err := store.GetEvent(ctx, nonExistentID)
	if err != nil {
		t.Fatalf("GetEvent() failed: %v", err)
	}

	if retrieved != nil {
		t.Errorf("GetEvent() for non-existent ID = %v, want nil", retrieved)
	}
}

func TestReadModelStore_ListEventsForPerson(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	personID := uuid.New()
	otherPersonID := uuid.New()

	// Create events for person
	events := []*repository.EventReadModel{
		{
			ID:        uuid.New(),
			OwnerType: "person",
			OwnerID:   personID,
			FactType:  domain.FactPersonBurial,
			DateRaw:   "15 JAN 1920",
			Place:     "Springfield Cemetery",
			Version:   1,
			CreatedAt: time.Now(),
		},
		{
			ID:        uuid.New(),
			OwnerType: "person",
			OwnerID:   personID,
			FactType:  domain.FactPersonBaptism,
			DateRaw:   "1 FEB 1850",
			Place:     "First Church",
			Version:   1,
			CreatedAt: time.Now(),
		},
		{
			ID:        uuid.New(),
			OwnerType: "person",
			OwnerID:   otherPersonID,
			FactType:  domain.FactPersonBurial,
			DateRaw:   "20 MAR 1925",
			Version:   1,
			CreatedAt: time.Now(),
		},
	}

	for _, e := range events {
		err := store.SaveEvent(ctx, e)
		if err != nil {
			t.Fatalf("SaveEvent() failed: %v", err)
		}
	}

	// List events for person
	results, err := store.ListEventsForPerson(ctx, personID)
	if err != nil {
		t.Fatalf("ListEventsForPerson() failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}

	// Verify all returned events belong to the person
	for _, e := range results {
		if e.OwnerID != personID {
			t.Errorf("Event OwnerID = %v, want %v", e.OwnerID, personID)
		}
	}
}

func TestReadModelStore_ListEventsForFamily(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	familyID := uuid.New()
	otherFamilyID := uuid.New()

	// Create events for family
	events := []*repository.EventReadModel{
		{
			ID:        uuid.New(),
			OwnerType: "family",
			OwnerID:   familyID,
			FactType:  domain.FactFamilyMarriageBann,
			DateRaw:   "1 JUN 1875",
			Place:     "Springfield Church",
			Version:   1,
			CreatedAt: time.Now(),
		},
		{
			ID:        uuid.New(),
			OwnerType: "family",
			OwnerID:   familyID,
			FactType:  domain.FactFamilyAnnulment,
			DateRaw:   "1 JAN 1890",
			Version:   1,
			CreatedAt: time.Now(),
		},
		{
			ID:        uuid.New(),
			OwnerType: "family",
			OwnerID:   otherFamilyID,
			FactType:  domain.FactFamilyEngagement,
			DateRaw:   "15 MAR 1880",
			Version:   1,
			CreatedAt: time.Now(),
		},
	}

	for _, e := range events {
		err := store.SaveEvent(ctx, e)
		if err != nil {
			t.Fatalf("SaveEvent() failed: %v", err)
		}
	}

	// List events for family
	results, err := store.ListEventsForFamily(ctx, familyID)
	if err != nil {
		t.Fatalf("ListEventsForFamily() failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}

	// Verify all returned events belong to the family
	for _, e := range results {
		if e.OwnerID != familyID {
			t.Errorf("Event OwnerID = %v, want %v", e.OwnerID, familyID)
		}
	}
}

func TestReadModelStore_UpdateEvent(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	eventID := uuid.New()
	event := &repository.EventReadModel{
		ID:        eventID,
		OwnerType: "person",
		OwnerID:   uuid.New(),
		FactType:  domain.FactPersonBurial,
		DateRaw:   "15 JAN 1920",
		Place:     "Old Cemetery",
		Version:   1,
		CreatedAt: time.Now(),
	}

	// Save initial version
	err := store.SaveEvent(ctx, event)
	if err != nil {
		t.Fatalf("SaveEvent() failed: %v", err)
	}

	// Update event
	event.Place = "New Cemetery"
	event.Version = 2

	err = store.SaveEvent(ctx, event)
	if err != nil {
		t.Fatalf("SaveEvent() update failed: %v", err)
	}

	// Retrieve and verify update
	retrieved, err := store.GetEvent(ctx, eventID)
	if err != nil {
		t.Fatalf("GetEvent() failed: %v", err)
	}

	if retrieved.Place != "New Cemetery" {
		t.Errorf("Place = %s, want New Cemetery", retrieved.Place)
	}
	if retrieved.Version != 2 {
		t.Errorf("Version = %d, want 2", retrieved.Version)
	}
}

func TestReadModelStore_DeleteEvent(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	event := &repository.EventReadModel{
		ID:        uuid.New(),
		OwnerType: "person",
		OwnerID:   uuid.New(),
		FactType:  domain.FactPersonBurial,
		Version:   1,
		CreatedAt: time.Now(),
	}

	// Save event
	err := store.SaveEvent(ctx, event)
	if err != nil {
		t.Fatalf("SaveEvent() failed: %v", err)
	}

	// Delete event
	err = store.DeleteEvent(ctx, event.ID)
	if err != nil {
		t.Fatalf("DeleteEvent() failed: %v", err)
	}

	// Verify event is deleted
	retrieved, err := store.GetEvent(ctx, event.ID)
	if err != nil {
		t.Fatalf("GetEvent() after delete failed: %v", err)
	}

	if retrieved != nil {
		t.Errorf("GetEvent() after delete = %v, want nil", retrieved)
	}
}

// Attribute CRUD operations

func TestReadModelStore_SaveAndGetAttribute(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	personID := uuid.New()
	attribute := &repository.AttributeReadModel{
		ID:        uuid.New(),
		PersonID:  personID,
		FactType:  domain.FactPersonOccupation,
		Value:     "Blacksmith",
		DateRaw:   "1880",
		Place:     "Springfield, IL",
		Version:   1,
		CreatedAt: time.Now(),
	}

	// Save attribute
	err := store.SaveAttribute(ctx, attribute)
	if err != nil {
		t.Fatalf("SaveAttribute() failed: %v", err)
	}

	// Get attribute
	retrieved, err := store.GetAttribute(ctx, attribute.ID)
	if err != nil {
		t.Fatalf("GetAttribute() failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetAttribute() returned nil")
	}

	// Verify fields
	if retrieved.ID != attribute.ID {
		t.Errorf("ID = %v, want %v", retrieved.ID, attribute.ID)
	}
	if retrieved.PersonID != personID {
		t.Errorf("PersonID = %v, want %v", retrieved.PersonID, personID)
	}
	if retrieved.FactType != domain.FactPersonOccupation {
		t.Errorf("FactType = %s, want %s", retrieved.FactType, domain.FactPersonOccupation)
	}
	if retrieved.Value != "Blacksmith" {
		t.Errorf("Value = %s, want Blacksmith", retrieved.Value)
	}
	if retrieved.DateRaw != "1880" {
		t.Errorf("DateRaw = %s, want 1880", retrieved.DateRaw)
	}
	if retrieved.Place != "Springfield, IL" {
		t.Errorf("Place = %s, want Springfield, IL", retrieved.Place)
	}
}

func TestReadModelStore_GetAttributeNonExistent(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	nonExistentID := uuid.New()

	retrieved, err := store.GetAttribute(ctx, nonExistentID)
	if err != nil {
		t.Fatalf("GetAttribute() failed: %v", err)
	}

	if retrieved != nil {
		t.Errorf("GetAttribute() for non-existent ID = %v, want nil", retrieved)
	}
}

func TestReadModelStore_ListAttributesForPerson(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	personID := uuid.New()
	otherPersonID := uuid.New()

	// Create attributes for person
	attributes := []*repository.AttributeReadModel{
		{
			ID:        uuid.New(),
			PersonID:  personID,
			FactType:  domain.FactPersonOccupation,
			Value:     "Blacksmith",
			Version:   1,
			CreatedAt: time.Now(),
		},
		{
			ID:        uuid.New(),
			PersonID:  personID,
			FactType:  domain.FactPersonResidence,
			Value:     "123 Main St",
			DateRaw:   "1880",
			Place:     "Springfield, IL",
			Version:   1,
			CreatedAt: time.Now(),
		},
		{
			ID:        uuid.New(),
			PersonID:  personID,
			FactType:  domain.FactPersonEducation,
			Value:     "Grammar School",
			Version:   1,
			CreatedAt: time.Now(),
		},
		{
			ID:        uuid.New(),
			PersonID:  otherPersonID,
			FactType:  domain.FactPersonOccupation,
			Value:     "Farmer",
			Version:   1,
			CreatedAt: time.Now(),
		},
	}

	for _, a := range attributes {
		err := store.SaveAttribute(ctx, a)
		if err != nil {
			t.Fatalf("SaveAttribute() failed: %v", err)
		}
	}

	// List attributes for person
	results, err := store.ListAttributesForPerson(ctx, personID)
	if err != nil {
		t.Fatalf("ListAttributesForPerson() failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("len(results) = %d, want 3", len(results))
	}

	// Verify all returned attributes belong to the person
	for _, a := range results {
		if a.PersonID != personID {
			t.Errorf("Attribute PersonID = %v, want %v", a.PersonID, personID)
		}
	}
}

func TestReadModelStore_UpdateAttribute(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	attrID := uuid.New()
	attribute := &repository.AttributeReadModel{
		ID:        attrID,
		PersonID:  uuid.New(),
		FactType:  domain.FactPersonOccupation,
		Value:     "Farmer",
		Version:   1,
		CreatedAt: time.Now(),
	}

	// Save initial version
	err := store.SaveAttribute(ctx, attribute)
	if err != nil {
		t.Fatalf("SaveAttribute() failed: %v", err)
	}

	// Update attribute
	attribute.Value = "Merchant"
	attribute.Version = 2

	err = store.SaveAttribute(ctx, attribute)
	if err != nil {
		t.Fatalf("SaveAttribute() update failed: %v", err)
	}

	// Retrieve and verify update
	retrieved, err := store.GetAttribute(ctx, attrID)
	if err != nil {
		t.Fatalf("GetAttribute() failed: %v", err)
	}

	if retrieved.Value != "Merchant" {
		t.Errorf("Value = %s, want Merchant", retrieved.Value)
	}
	if retrieved.Version != 2 {
		t.Errorf("Version = %d, want 2", retrieved.Version)
	}
}

func TestReadModelStore_DeleteAttribute(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	attribute := &repository.AttributeReadModel{
		ID:        uuid.New(),
		PersonID:  uuid.New(),
		FactType:  domain.FactPersonOccupation,
		Value:     "Farmer",
		Version:   1,
		CreatedAt: time.Now(),
	}

	// Save attribute
	err := store.SaveAttribute(ctx, attribute)
	if err != nil {
		t.Fatalf("SaveAttribute() failed: %v", err)
	}

	// Delete attribute
	err = store.DeleteAttribute(ctx, attribute.ID)
	if err != nil {
		t.Fatalf("DeleteAttribute() failed: %v", err)
	}

	// Verify attribute is deleted
	retrieved, err := store.GetAttribute(ctx, attribute.ID)
	if err != nil {
		t.Fatalf("GetAttribute() after delete failed: %v", err)
	}

	if retrieved != nil {
		t.Errorf("GetAttribute() after delete = %v, want nil", retrieved)
	}
}

func TestReadModelStore_MultipleAttributeTypes(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	personID := uuid.New()

	// Test all attribute types
	attributeTypes := []domain.FactType{
		domain.FactPersonOccupation,
		domain.FactPersonResidence,
		domain.FactPersonEducation,
		domain.FactPersonReligion,
		domain.FactPersonTitle,
	}

	for i, factType := range attributeTypes {
		attr := &repository.AttributeReadModel{
			ID:        uuid.New(),
			PersonID:  personID,
			FactType:  factType,
			Value:     "Test Value " + string(rune('A'+i)),
			Version:   1,
			CreatedAt: time.Now(),
		}
		err := store.SaveAttribute(ctx, attr)
		if err != nil {
			t.Fatalf("SaveAttribute() for %s failed: %v", factType, err)
		}
	}

	// List all attributes
	results, err := store.ListAttributesForPerson(ctx, personID)
	if err != nil {
		t.Fatalf("ListAttributesForPerson() failed: %v", err)
	}

	if len(results) != len(attributeTypes) {
		t.Errorf("len(results) = %d, want %d", len(results), len(attributeTypes))
	}

	// Verify each type is present
	foundTypes := make(map[domain.FactType]bool)
	for _, a := range results {
		foundTypes[a.FactType] = true
	}

	for _, factType := range attributeTypes {
		if !foundTypes[factType] {
			t.Errorf("expected to find attribute with FactType %s", factType)
		}
	}
}

func TestReadModelStore_MultipleEventTypes(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	personID := uuid.New()
	familyID := uuid.New()

	// Test person event types
	personEventTypes := []domain.FactType{
		domain.FactPersonBurial,
		domain.FactPersonCremation,
		domain.FactPersonBaptism,
		domain.FactPersonChristening,
		domain.FactPersonEmigration,
		domain.FactPersonImmigration,
		domain.FactPersonNaturalization,
		domain.FactPersonCensus,
	}

	for i, factType := range personEventTypes {
		event := &repository.EventReadModel{
			ID:        uuid.New(),
			OwnerType: "person",
			OwnerID:   personID,
			FactType:  factType,
			DateRaw:   "1 JAN 18" + string(rune('5'+i%5)) + "0",
			Version:   1,
			CreatedAt: time.Now(),
		}
		err := store.SaveEvent(ctx, event)
		if err != nil {
			t.Fatalf("SaveEvent() for person %s failed: %v", factType, err)
		}
	}

	// Test family event types
	familyEventTypes := []domain.FactType{
		domain.FactFamilyMarriageBann,
		domain.FactFamilyMarriageContract,
		domain.FactFamilyMarriageLicense,
		domain.FactFamilyMarriageSettlement,
		domain.FactFamilyAnnulment,
		domain.FactFamilyEngagement,
	}

	for i, factType := range familyEventTypes {
		event := &repository.EventReadModel{
			ID:        uuid.New(),
			OwnerType: "family",
			OwnerID:   familyID,
			FactType:  factType,
			DateRaw:   "1 JUN 187" + string(rune('0'+i)),
			Version:   1,
			CreatedAt: time.Now(),
		}
		err := store.SaveEvent(ctx, event)
		if err != nil {
			t.Fatalf("SaveEvent() for family %s failed: %v", factType, err)
		}
	}

	// List person events
	personResults, err := store.ListEventsForPerson(ctx, personID)
	if err != nil {
		t.Fatalf("ListEventsForPerson() failed: %v", err)
	}

	if len(personResults) != len(personEventTypes) {
		t.Errorf("len(personResults) = %d, want %d", len(personResults), len(personEventTypes))
	}

	// List family events
	familyResults, err := store.ListEventsForFamily(ctx, familyID)
	if err != nil {
		t.Fatalf("ListEventsForFamily() failed: %v", err)
	}

	if len(familyResults) != len(familyEventTypes) {
		t.Errorf("len(familyResults) = %d, want %d", len(familyResults), len(familyEventTypes))
	}
}

// Browse tests

func TestReadModelStore_GetSurnameIndex(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	// Add persons with various surnames
	persons := []struct {
		surname string
		count   int
	}{
		{"Smith", 3},
		{"Anderson", 2},
		{"Brown", 1},
	}

	for _, p := range persons {
		for i := 0; i < p.count; i++ {
			person := &repository.PersonReadModel{
				ID:        uuid.New(),
				GivenName: "Person",
				Surname:   p.surname,
				FullName:  "Person " + p.surname,
				Version:   1,
				UpdatedAt: time.Now(),
			}
			err := store.SavePerson(ctx, person)
			if err != nil {
				t.Fatalf("SavePerson() failed: %v", err)
			}
		}
	}

	// Get surname index
	entries, letterCounts, err := store.GetSurnameIndex(ctx)
	if err != nil {
		t.Fatalf("GetSurnameIndex() failed: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("len(entries) = %d, want 3", len(entries))
	}

	if len(letterCounts) == 0 {
		t.Error("letterCounts should not be empty")
	}
}

func TestReadModelStore_GetSurnamesByLetter(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	// Add persons starting with 'S'
	surnames := []string{"Smith", "Simpson", "Sanders"}
	for _, surname := range surnames {
		person := &repository.PersonReadModel{
			ID:        uuid.New(),
			GivenName: "Person",
			Surname:   surname,
			FullName:  "Person " + surname,
			Version:   1,
			UpdatedAt: time.Now(),
		}
		err := store.SavePerson(ctx, person)
		if err != nil {
			t.Fatalf("SavePerson() failed: %v", err)
		}
	}

	// Add person starting with 'J'
	person := &repository.PersonReadModel{
		ID:        uuid.New(),
		GivenName: "John",
		Surname:   "Jones",
		FullName:  "John Jones",
		Version:   1,
		UpdatedAt: time.Now(),
	}
	_ = store.SavePerson(ctx, person)

	// Get surnames starting with S
	entries, err := store.GetSurnamesByLetter(ctx, "S")
	if err != nil {
		t.Fatalf("GetSurnamesByLetter() failed: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("len(entries) = %d, want 3", len(entries))
	}
}

func TestReadModelStore_GetPersonsBySurname(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	// Add multiple Smiths
	for i := 0; i < 3; i++ {
		person := &repository.PersonReadModel{
			ID:        uuid.New(),
			GivenName: "Person",
			Surname:   "Smith",
			FullName:  "Person Smith",
			Version:   1,
			UpdatedAt: time.Now(),
		}
		err := store.SavePerson(ctx, person)
		if err != nil {
			t.Fatalf("SavePerson() failed: %v", err)
		}
	}

	// Add a Jones
	jones := &repository.PersonReadModel{
		ID:        uuid.New(),
		GivenName: "John",
		Surname:   "Jones",
		FullName:  "John Jones",
		Version:   1,
		UpdatedAt: time.Now(),
	}
	_ = store.SavePerson(ctx, jones)

	// Get Smiths
	opts := repository.ListOptions{Limit: 10}
	persons, total, err := store.GetPersonsBySurname(ctx, "Smith", opts)
	if err != nil {
		t.Fatalf("GetPersonsBySurname() failed: %v", err)
	}

	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(persons) != 3 {
		t.Errorf("len(persons) = %d, want 3", len(persons))
	}
}

func TestReadModelStore_GetPlaceHierarchy(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	// Add persons with places
	places := []string{
		"New York, NY, USA",
		"Boston, MA, USA",
		"London, England, UK",
	}
	for _, place := range places {
		person := &repository.PersonReadModel{
			ID:         uuid.New(),
			GivenName:  "Person",
			Surname:    "Test",
			FullName:   "Person Test",
			BirthPlace: place,
			Version:    1,
			UpdatedAt:  time.Now(),
		}
		err := store.SavePerson(ctx, person)
		if err != nil {
			t.Fatalf("SavePerson() failed: %v", err)
		}
	}

	// Get top-level places
	entries, err := store.GetPlaceHierarchy(ctx, "")
	if err != nil {
		t.Fatalf("GetPlaceHierarchy() failed: %v", err)
	}

	if len(entries) == 0 {
		t.Error("entries should not be empty")
	}
}

func TestReadModelStore_GetPersonsByPlace(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	// Add persons from USA
	for i := 0; i < 3; i++ {
		person := &repository.PersonReadModel{
			ID:         uuid.New(),
			GivenName:  "Person",
			Surname:    "USA",
			FullName:   "Person USA",
			BirthPlace: "New York, USA",
			Version:    1,
			UpdatedAt:  time.Now(),
		}
		err := store.SavePerson(ctx, person)
		if err != nil {
			t.Fatalf("SavePerson() failed: %v", err)
		}
	}

	// Add person from UK
	ukPerson := &repository.PersonReadModel{
		ID:         uuid.New(),
		GivenName:  "Person",
		Surname:    "UK",
		FullName:   "Person UK",
		BirthPlace: "London, UK",
		Version:    1,
		UpdatedAt:  time.Now(),
	}
	_ = store.SavePerson(ctx, ukPerson)

	// Get persons from USA
	opts := repository.ListOptions{Limit: 10}
	persons, total, err := store.GetPersonsByPlace(ctx, "USA", opts)
	if err != nil {
		t.Fatalf("GetPersonsByPlace() failed: %v", err)
	}

	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(persons) != 3 {
		t.Errorf("len(persons) = %d, want 3", len(persons))
	}
}

// Global List methods for export

func TestReadModelStore_ListEvents(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	personID := uuid.New()
	familyID := uuid.New()

	// Create some events for different owners
	events := []*repository.EventReadModel{
		{
			ID:        uuid.New(),
			OwnerType: "person",
			OwnerID:   personID,
			FactType:  domain.FactPersonBurial,
			DateRaw:   "15 JAN 1920",
			Place:     "Springfield Cemetery",
			Version:   1,
			CreatedAt: time.Now(),
		},
		{
			ID:        uuid.New(),
			OwnerType: "person",
			OwnerID:   personID,
			FactType:  domain.FactPersonBaptism,
			DateRaw:   "25 DEC 1850",
			Place:     "St. Mary's Church",
			Version:   1,
			CreatedAt: time.Now(),
		},
		{
			ID:        uuid.New(),
			OwnerType: "family",
			OwnerID:   familyID,
			FactType:  domain.FactFamilyMarriage,
			DateRaw:   "10 JUN 1875",
			Place:     "Springfield, IL",
			Version:   1,
			CreatedAt: time.Now(),
		},
	}

	for _, e := range events {
		err := store.SaveEvent(ctx, e)
		if err != nil {
			t.Fatalf("SaveEvent() failed: %v", err)
		}
	}

	// Test ListEvents returns all events
	results, total, err := store.ListEvents(ctx, repository.ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("ListEvents() failed: %v", err)
	}

	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(results) != 3 {
		t.Errorf("len(results) = %d, want 3", len(results))
	}

	// Test with limit
	results, total, err = store.ListEvents(ctx, repository.ListOptions{Limit: 2})
	if err != nil {
		t.Fatalf("ListEvents() with limit failed: %v", err)
	}

	if total != 3 {
		t.Errorf("total with limit = %d, want 3", total)
	}
	if len(results) != 2 {
		t.Errorf("len(results) with limit = %d, want 2", len(results))
	}
}

func TestReadModelStore_ListCitations(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	sourceID := uuid.New()
	personID := uuid.New()

	// Create some citations
	citations := []*repository.CitationReadModel{
		{
			ID:          uuid.New(),
			SourceID:    sourceID,
			SourceTitle: "Census 1850",
			FactType:    domain.FactPersonBirth,
			FactOwnerID: personID,
			Page:        "p. 42",
			Version:     1,
			CreatedAt:   time.Now(),
		},
		{
			ID:          uuid.New(),
			SourceID:    sourceID,
			SourceTitle: "Census 1850",
			FactType:    domain.FactPersonDeath,
			FactOwnerID: personID,
			Page:        "p. 100",
			Version:     1,
			CreatedAt:   time.Now(),
		},
	}

	for _, c := range citations {
		err := store.SaveCitation(ctx, c)
		if err != nil {
			t.Fatalf("SaveCitation() failed: %v", err)
		}
	}

	// Test ListCitations returns all citations
	results, total, err := store.ListCitations(ctx, repository.ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("ListCitations() failed: %v", err)
	}

	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}

	// Test with limit
	results, total, err = store.ListCitations(ctx, repository.ListOptions{Limit: 1})
	if err != nil {
		t.Fatalf("ListCitations() with limit failed: %v", err)
	}

	if total != 2 {
		t.Errorf("total with limit = %d, want 2", total)
	}
	if len(results) != 1 {
		t.Errorf("len(results) with limit = %d, want 1", len(results))
	}
}

func TestReadModelStore_ListAttributes(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()

	personID := uuid.New()
	person2ID := uuid.New()

	// Create some attributes for different persons
	attributes := []*repository.AttributeReadModel{
		{
			ID:        uuid.New(),
			PersonID:  personID,
			FactType:  domain.FactPersonOccupation,
			Value:     "Farmer",
			Version:   1,
			CreatedAt: time.Now(),
		},
		{
			ID:        uuid.New(),
			PersonID:  personID,
			FactType:  domain.FactPersonReligion,
			Value:     "Methodist",
			Version:   1,
			CreatedAt: time.Now(),
		},
		{
			ID:        uuid.New(),
			PersonID:  person2ID,
			FactType:  domain.FactPersonOccupation,
			Value:     "Merchant",
			Version:   1,
			CreatedAt: time.Now(),
		},
	}

	for _, a := range attributes {
		err := store.SaveAttribute(ctx, a)
		if err != nil {
			t.Fatalf("SaveAttribute() failed: %v", err)
		}
	}

	// Test ListAttributes returns all attributes
	results, total, err := store.ListAttributes(ctx, repository.ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("ListAttributes() failed: %v", err)
	}

	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(results) != 3 {
		t.Errorf("len(results) = %d, want 3", len(results))
	}

	// Test with limit
	results, total, err = store.ListAttributes(ctx, repository.ListOptions{Limit: 2})
	if err != nil {
		t.Fatalf("ListAttributes() with limit failed: %v", err)
	}

	if total != 3 {
		t.Errorf("total with limit = %d, want 3", total)
	}
	if len(results) != 2 {
		t.Errorf("len(results) with limit = %d, want 2", len(results))
	}
}
