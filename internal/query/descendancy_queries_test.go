package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func setupDescendancyTestData(t *testing.T, readStore *memory.ReadModelStore) (grandparent, parent1, parent2, child1, child2, grandchild uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	// Create persons: grandparent -> parent1 + parent2(spouse) -> child1, child2 -> grandchild
	grandparent = uuid.New()
	parent1 = uuid.New()
	parent2 = uuid.New() // spouse of parent1
	child1 = uuid.New()
	child2 = uuid.New()
	grandchild = uuid.New()

	// Save persons
	persons := []repository.PersonReadModel{
		{ID: grandparent, GivenName: "George", Surname: "Smith", FullName: "George Smith", Gender: domain.GenderMale, BirthDateRaw: "1 JAN 1940"},
		{ID: parent1, GivenName: "John", Surname: "Smith", FullName: "John Smith", Gender: domain.GenderMale, BirthDateRaw: "1 JAN 1970"},
		{ID: parent2, GivenName: "Jane", Surname: "Doe", FullName: "Jane Doe", Gender: domain.GenderFemale, BirthDateRaw: "1 JAN 1975"},
		{ID: child1, GivenName: "Junior", Surname: "Smith", FullName: "Junior Smith", Gender: domain.GenderMale, BirthDateRaw: "1 JAN 2000"},
		{ID: child2, GivenName: "Jenny", Surname: "Smith", FullName: "Jenny Smith", Gender: domain.GenderFemale, BirthDateRaw: "1 JAN 2002"},
		{ID: grandchild, GivenName: "Baby", Surname: "Smith", FullName: "Baby Smith", Gender: domain.GenderMale, BirthDateRaw: "1 JAN 2025"},
	}

	for _, p := range persons {
		pm := p
		if err := readStore.SavePerson(ctx, &pm); err != nil {
			t.Fatal(err)
		}
	}

	// Create families
	// Family 1: grandparent is partner (no spouse in this family - single parent for simplicity)
	family1 := uuid.New()
	err := readStore.SaveFamily(ctx, &repository.FamilyReadModel{
		ID:         family1,
		Partner1ID: &grandparent,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Add parent1 as child of grandparent
	err = readStore.SaveFamilyChild(ctx, &repository.FamilyChildReadModel{
		FamilyID:         family1,
		PersonID:         parent1,
		PersonName:       "John Smith",
		RelationshipType: domain.ChildBiological,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Family 2: parent1 + parent2 (spouse)
	family2 := uuid.New()
	err = readStore.SaveFamily(ctx, &repository.FamilyReadModel{
		ID:              family2,
		Partner1ID:      &parent1,
		Partner1Name:    "John Smith",
		Partner2ID:      &parent2,
		Partner2Name:    "Jane Doe",
		MarriageDateRaw: "15 JUN 1995",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Add child1 and child2 as children of parent1 and parent2
	err = readStore.SaveFamilyChild(ctx, &repository.FamilyChildReadModel{
		FamilyID:         family2,
		PersonID:         child1,
		PersonName:       "Junior Smith",
		RelationshipType: domain.ChildBiological,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = readStore.SaveFamilyChild(ctx, &repository.FamilyChildReadModel{
		FamilyID:         family2,
		PersonID:         child2,
		PersonName:       "Jenny Smith",
		RelationshipType: domain.ChildBiological,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Family 3: child1 has a family with grandchild
	family3 := uuid.New()
	err = readStore.SaveFamily(ctx, &repository.FamilyReadModel{
		ID:         family3,
		Partner1ID: &child1,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = readStore.SaveFamilyChild(ctx, &repository.FamilyChildReadModel{
		FamilyID:         family3,
		PersonID:         grandchild,
		PersonName:       "Baby Smith",
		RelationshipType: domain.ChildBiological,
	})
	if err != nil {
		t.Fatal(err)
	}

	return
}

func TestGetDescendancy(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewDescendancyService(readStore)

	grandparent, parent1, _, child1, child2, grandchild := setupDescendancyTestData(t, readStore)

	ctx := context.Background()
	result, err := svc.GetDescendancy(ctx, query.GetDescendancyInput{
		PersonID:       grandparent,
		MaxGenerations: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify root
	if result.Root == nil {
		t.Fatal("Root should not be nil")
	}
	if result.Root.ID != grandparent {
		t.Errorf("Root ID = %v, want %v", result.Root.ID, grandparent)
	}
	if result.Root.GivenName != "George" {
		t.Errorf("Root given name = %s, want George", result.Root.GivenName)
	}
	if result.Root.Generation != 0 {
		t.Errorf("Root generation = %d, want 0", result.Root.Generation)
	}

	// Verify parent1 is a child of grandparent
	if len(result.Root.Children) == 0 {
		t.Fatal("Root should have children")
	}

	var foundParent1 *query.DescendancyNode
	for _, c := range result.Root.Children {
		if c.ID == parent1 {
			foundParent1 = c
			break
		}
	}
	if foundParent1 == nil {
		t.Fatal("Parent1 should be a child of grandparent")
	}
	if foundParent1.Generation != 1 {
		t.Errorf("Parent1 generation = %d, want 1", foundParent1.Generation)
	}

	// Verify parent1 has spouse info
	if len(foundParent1.Spouses) == 0 {
		t.Fatal("Parent1 should have spouse info")
	}
	if foundParent1.Spouses[0].Name != "Jane Doe" {
		t.Errorf("Spouse name = %s, want Jane Doe", foundParent1.Spouses[0].Name)
	}
	if foundParent1.Spouses[0].MarriageDate == nil {
		t.Error("Marriage date should be set")
	}

	// Verify child1 and child2 are children of parent1
	if len(foundParent1.Children) != 2 {
		t.Errorf("Parent1 should have 2 children, got %d", len(foundParent1.Children))
	}

	var foundChild1, foundChild2 *query.DescendancyNode
	for _, c := range foundParent1.Children {
		if c.ID == child1 {
			foundChild1 = c
		}
		if c.ID == child2 {
			foundChild2 = c
		}
	}
	if foundChild1 == nil {
		t.Error("Child1 should be a child of parent1")
	}
	if foundChild2 == nil {
		t.Error("Child2 should be a child of parent1")
	}
	if foundChild1 != nil && foundChild1.Generation != 2 {
		t.Errorf("Child1 generation = %d, want 2", foundChild1.Generation)
	}

	// Verify grandchild is a child of child1
	if foundChild1 != nil && len(foundChild1.Children) != 1 {
		t.Errorf("Child1 should have 1 child, got %d", len(foundChild1.Children))
	}
	if foundChild1 != nil && foundChild1.Children[0].ID != grandchild {
		t.Error("Grandchild should be a child of child1")
	}
	if foundChild1 != nil && foundChild1.Children[0].Generation != 3 {
		t.Errorf("Grandchild generation = %d, want 3", foundChild1.Children[0].Generation)
	}

	// Verify counts
	if result.TotalDescendants != 4 { // parent1, child1, child2, grandchild
		t.Errorf("TotalDescendants = %d, want 4", result.TotalDescendants)
	}
	if result.MaxGeneration != 3 {
		t.Errorf("MaxGeneration = %d, want 3", result.MaxGeneration)
	}
}

func TestGetDescendancy_NotFound(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewDescendancyService(readStore)

	ctx := context.Background()
	_, err := svc.GetDescendancy(ctx, query.GetDescendancyInput{
		PersonID: uuid.New(),
	})
	if err != query.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestGetDescendancy_MaxGenerations(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewDescendancyService(readStore)

	grandparent, _, _, _, _, _ := setupDescendancyTestData(t, readStore)

	ctx := context.Background()

	// Request only 1 generation
	result, err := svc.GetDescendancy(ctx, query.GetDescendancyInput{
		PersonID:       grandparent,
		MaxGenerations: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should have children (generation 1) but no grandchildren
	if len(result.Root.Children) == 0 {
		t.Error("Should have children at generation 1")
	}
	// Children should not have their own children (generation 2 exceeds limit)
	for _, child := range result.Root.Children {
		if len(child.Children) > 0 {
			t.Error("Grandchildren should not be present when max generation is 1")
		}
	}

	// Max generation should be 1
	if result.MaxGeneration != 1 {
		t.Errorf("MaxGeneration = %d, want 1", result.MaxGeneration)
	}
}

func TestGetDescendancy_NoDescendants(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewDescendancyService(readStore)

	ctx := context.Background()

	// Create a person with no descendants
	leafPerson := uuid.New()
	err := readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        leafPerson,
		GivenName: "Leaf",
		Surname:   "Person",
		FullName:  "Leaf Person",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := svc.GetDescendancy(ctx, query.GetDescendancyInput{
		PersonID: leafPerson,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Root == nil {
		t.Fatal("Root should not be nil")
	}
	if len(result.Root.Children) != 0 {
		t.Error("Should have no children")
	}
	if len(result.Root.Spouses) != 0 {
		t.Error("Should have no spouses")
	}
	if result.TotalDescendants != 0 {
		t.Errorf("TotalDescendants = %d, want 0", result.TotalDescendants)
	}
	if result.MaxGeneration != 0 {
		t.Errorf("MaxGeneration = %d, want 0", result.MaxGeneration)
	}
}

func TestGetDescendancy_DefaultMaxGenerations(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewDescendancyService(readStore)

	grandparent, _, _, _, _, _ := setupDescendancyTestData(t, readStore)

	ctx := context.Background()

	// Request with 0 max generations should default to 4
	result, err := svc.GetDescendancy(ctx, query.GetDescendancyInput{
		PersonID:       grandparent,
		MaxGenerations: 0,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should still include all descendants (only 3 generations in test data)
	if result.TotalDescendants != 4 {
		t.Errorf("TotalDescendants = %d, want 4", result.TotalDescendants)
	}
}

func TestGetDescendancy_MaxGenerationsHardLimit(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewDescendancyService(readStore)

	grandparent, _, _, _, _, _ := setupDescendancyTestData(t, readStore)

	ctx := context.Background()

	// Request with max generations > 10 should be capped to 10
	result, err := svc.GetDescendancy(ctx, query.GetDescendancyInput{
		PersonID:       grandparent,
		MaxGenerations: 100, // Should be capped to 10
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should still work and include all available descendants
	if result.Root == nil {
		t.Fatal("Root should not be nil")
	}
}

func TestGetDescendancy_CycleDetection(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewDescendancyService(readStore)

	ctx := context.Background()

	// Create persons that could form a cycle in the descendancy tree
	person1 := uuid.New()
	person2 := uuid.New()

	// Save persons
	err := readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        person1,
		GivenName: "Person1",
		Surname:   "Test",
		FullName:  "Person1 Test",
		Gender:    domain.GenderMale,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        person2,
		GivenName: "Person2",
		Surname:   "Test",
		FullName:  "Person2 Test",
		Gender:    domain.GenderMale,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create family where person1 is parent of person2
	family1 := uuid.New()
	err = readStore.SaveFamily(ctx, &repository.FamilyReadModel{
		ID:         family1,
		Partner1ID: &person1,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = readStore.SaveFamilyChild(ctx, &repository.FamilyChildReadModel{
		FamilyID:         family1,
		PersonID:         person2,
		PersonName:       "Person2 Test",
		RelationshipType: domain.ChildBiological,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create family where person2 is parent of person1 (cycle!)
	family2 := uuid.New()
	err = readStore.SaveFamily(ctx, &repository.FamilyReadModel{
		ID:         family2,
		Partner1ID: &person2,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = readStore.SaveFamilyChild(ctx, &repository.FamilyChildReadModel{
		FamilyID:         family2,
		PersonID:         person1,
		PersonName:       "Person1 Test",
		RelationshipType: domain.ChildBiological,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should not infinite loop due to cycle detection
	result, err := svc.GetDescendancy(ctx, query.GetDescendancyInput{
		PersonID:       person1,
		MaxGenerations: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Root == nil {
		t.Fatal("Root should not be nil")
	}
	// Verify it handled the cycle gracefully - person2 should be a child
	if len(result.Root.Children) != 1 {
		t.Errorf("Expected 1 child, got %d", len(result.Root.Children))
	}
	// Person2's children should not include person1 again due to cycle detection
	if len(result.Root.Children) > 0 && len(result.Root.Children[0].Children) > 0 {
		t.Error("Cycle should have been detected, person2's children should be empty")
	}
}

func TestGetDescendancy_AllOptionalFields(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewDescendancyService(readStore)

	ctx := context.Background()

	// Create person with all optional fields populated
	person := uuid.New()
	err := readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:           person,
		GivenName:    "John",
		Surname:      "Doe",
		FullName:     "John Doe",
		Gender:       domain.GenderMale,
		BirthDateRaw: "15 MAR 1980",
		DeathDateRaw: "20 DEC 2050",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := svc.GetDescendancy(ctx, query.GetDescendancyInput{
		PersonID:       person,
		MaxGenerations: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Root == nil {
		t.Fatal("Root should not be nil")
	}

	// Verify all optional fields are populated
	if result.Root.Gender == "" {
		t.Error("Gender should be set")
	}
	if result.Root.BirthDate == nil {
		t.Error("BirthDate should be set")
	}
	if result.Root.DeathDate == nil {
		t.Error("DeathDate should be set")
	}
}

func TestCountDescendants_NilNode(t *testing.T) {
	// This tests the service with a person who has no descendants
	readStore := memory.NewReadModelStore()
	svc := query.NewDescendancyService(readStore)

	ctx := context.Background()

	// Create person with no descendants
	person := uuid.New()
	err := readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        person,
		GivenName: "Alone",
		Surname:   "Person",
		FullName:  "Alone Person",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := svc.GetDescendancy(ctx, query.GetDescendancyInput{
		PersonID:       person,
		MaxGenerations: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.TotalDescendants != 0 {
		t.Errorf("TotalDescendants = %d, want 0", result.TotalDescendants)
	}
	if result.MaxGeneration != 0 {
		t.Errorf("MaxGeneration = %d, want 0", result.MaxGeneration)
	}
}

func TestGetDescendancy_NegativeMaxGenerations(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewDescendancyService(readStore)

	grandparent, _, _, _, _, _ := setupDescendancyTestData(t, readStore)

	ctx := context.Background()

	// Negative max generations should default to 4
	result, err := svc.GetDescendancy(ctx, query.GetDescendancyInput{
		PersonID:       grandparent,
		MaxGenerations: -1,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should still include all descendants (only 3 generations in test data)
	if result.TotalDescendants != 4 {
		t.Errorf("TotalDescendants = %d, want 4", result.TotalDescendants)
	}
}

func TestGetDescendancy_MultipleSpouses(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewDescendancyService(readStore)

	ctx := context.Background()

	// Create a person with multiple spouses
	parent := uuid.New()
	spouse1 := uuid.New()
	spouse2 := uuid.New()
	child1 := uuid.New()
	child2 := uuid.New()

	persons := []repository.PersonReadModel{
		{ID: parent, GivenName: "Parent", Surname: "Test", FullName: "Parent Test", Gender: domain.GenderMale},
		{ID: spouse1, GivenName: "Spouse1", Surname: "Test", FullName: "Spouse1 Test", Gender: domain.GenderFemale},
		{ID: spouse2, GivenName: "Spouse2", Surname: "Test", FullName: "Spouse2 Test", Gender: domain.GenderFemale},
		{ID: child1, GivenName: "Child1", Surname: "Test", FullName: "Child1 Test", Gender: domain.GenderMale},
		{ID: child2, GivenName: "Child2", Surname: "Test", FullName: "Child2 Test", Gender: domain.GenderMale},
	}

	for _, p := range persons {
		pm := p
		if err := readStore.SavePerson(ctx, &pm); err != nil {
			t.Fatal(err)
		}
	}

	// Create family 1: parent + spouse1
	family1 := uuid.New()
	err := readStore.SaveFamily(ctx, &repository.FamilyReadModel{
		ID:              family1,
		Partner1ID:      &parent,
		Partner1Name:    "Parent Test",
		Partner2ID:      &spouse1,
		Partner2Name:    "Spouse1 Test",
		MarriageDateRaw: "1 JAN 1990",
	})
	if err != nil {
		t.Fatal(err)
	}

	err = readStore.SaveFamilyChild(ctx, &repository.FamilyChildReadModel{
		FamilyID:         family1,
		PersonID:         child1,
		PersonName:       "Child1 Test",
		RelationshipType: domain.ChildBiological,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create family 2: parent + spouse2
	family2 := uuid.New()
	err = readStore.SaveFamily(ctx, &repository.FamilyReadModel{
		ID:              family2,
		Partner1ID:      &parent,
		Partner1Name:    "Parent Test",
		Partner2ID:      &spouse2,
		Partner2Name:    "Spouse2 Test",
		MarriageDateRaw: "1 JAN 2000",
	})
	if err != nil {
		t.Fatal(err)
	}

	err = readStore.SaveFamilyChild(ctx, &repository.FamilyChildReadModel{
		FamilyID:         family2,
		PersonID:         child2,
		PersonName:       "Child2 Test",
		RelationshipType: domain.ChildBiological,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := svc.GetDescendancy(ctx, query.GetDescendancyInput{
		PersonID:       parent,
		MaxGenerations: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify parent has 2 spouses
	if len(result.Root.Spouses) != 2 {
		t.Errorf("Expected 2 spouses, got %d", len(result.Root.Spouses))
	}

	// Verify parent has 2 children
	if len(result.Root.Children) != 2 {
		t.Errorf("Expected 2 children, got %d", len(result.Root.Children))
	}

	// Verify both spouses are present
	spouseNames := make(map[string]bool)
	for _, s := range result.Root.Spouses {
		spouseNames[s.Name] = true
	}
	if !spouseNames["Spouse1 Test"] {
		t.Error("Spouse1 should be in spouses list")
	}
	if !spouseNames["Spouse2 Test"] {
		t.Error("Spouse2 should be in spouses list")
	}
}
