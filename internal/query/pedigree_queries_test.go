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

func setupPedigreeTestData(t *testing.T, readStore *memory.ReadModelStore) (child, father, mother, grandfather, grandmother uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	// Create persons: child -> father -> grandfather, grandmother
	//                      -> mother
	child = uuid.New()
	father = uuid.New()
	mother = uuid.New()
	grandfather = uuid.New()
	grandmother = uuid.New()

	// Save persons
	persons := []repository.PersonReadModel{
		{ID: child, GivenName: "Junior", Surname: "Smith", FullName: "Junior Smith", Gender: domain.GenderMale},
		{ID: father, GivenName: "John", Surname: "Smith", FullName: "John Smith", Gender: domain.GenderMale},
		{ID: mother, GivenName: "Jane", Surname: "Doe", FullName: "Jane Doe", Gender: domain.GenderFemale},
		{ID: grandfather, GivenName: "George", Surname: "Smith", FullName: "George Smith", Gender: domain.GenderMale},
		{ID: grandmother, GivenName: "Mary", Surname: "Jones", FullName: "Mary Jones", Gender: domain.GenderFemale},
	}

	for _, p := range persons {
		pm := p
		if err := readStore.SavePerson(ctx, &pm); err != nil {
			t.Fatal(err)
		}
	}

	// Create pedigree edges
	// Child's parents are father and mother
	childEdge := &repository.PedigreeEdge{
		PersonID:   child,
		FatherID:   &father,
		FatherName: "John Smith",
		MotherID:   &mother,
		MotherName: "Jane Doe",
	}
	if err := readStore.SavePedigreeEdge(ctx, childEdge); err != nil {
		t.Fatal(err)
	}

	// Father's parents are grandfather and grandmother
	fatherEdge := &repository.PedigreeEdge{
		PersonID:   father,
		FatherID:   &grandfather,
		FatherName: "George Smith",
		MotherID:   &grandmother,
		MotherName: "Mary Jones",
	}
	if err := readStore.SavePedigreeEdge(ctx, fatherEdge); err != nil {
		t.Fatal(err)
	}

	return
}

func TestGetPedigree(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewPedigreeService(readStore)

	child, father, mother, grandfather, _ := setupPedigreeTestData(t, readStore)

	ctx := context.Background()
	result, err := svc.GetPedigree(ctx, query.GetPedigreeInput{
		PersonID:       child,
		MaxGenerations: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify root
	if result.Root == nil {
		t.Fatal("Root should not be nil")
	}
	if result.Root.ID != child {
		t.Errorf("Root ID = %v, want %v", result.Root.ID, child)
	}
	if result.Root.GivenName != "Junior" {
		t.Errorf("Root given name = %s, want Junior", result.Root.GivenName)
	}
	if result.Root.Generation != 0 {
		t.Errorf("Root generation = %d, want 0", result.Root.Generation)
	}

	// Verify father
	if result.Root.Father == nil {
		t.Fatal("Father should not be nil")
	}
	if result.Root.Father.ID != father {
		t.Errorf("Father ID = %v, want %v", result.Root.Father.ID, father)
	}
	if result.Root.Father.Generation != 1 {
		t.Errorf("Father generation = %d, want 1", result.Root.Father.Generation)
	}

	// Verify mother
	if result.Root.Mother == nil {
		t.Fatal("Mother should not be nil")
	}
	if result.Root.Mother.ID != mother {
		t.Errorf("Mother ID = %v, want %v", result.Root.Mother.ID, mother)
	}

	// Verify grandfather (father's father)
	if result.Root.Father.Father == nil {
		t.Fatal("Grandfather should not be nil")
	}
	if result.Root.Father.Father.ID != grandfather {
		t.Errorf("Grandfather ID = %v, want %v", result.Root.Father.Father.ID, grandfather)
	}
	if result.Root.Father.Father.Generation != 2 {
		t.Errorf("Grandfather generation = %d, want 2", result.Root.Father.Father.Generation)
	}

	// Mother has no parents in test data
	if result.Root.Mother.Father != nil {
		t.Error("Mother's father should be nil (no data)")
	}

	// Verify counts
	if result.TotalAncestors != 4 {
		t.Errorf("TotalAncestors = %d, want 4", result.TotalAncestors)
	}
	if result.MaxGeneration != 2 {
		t.Errorf("MaxGeneration = %d, want 2", result.MaxGeneration)
	}
}

func TestGetPedigree_NotFound(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewPedigreeService(readStore)

	ctx := context.Background()
	_, err := svc.GetPedigree(ctx, query.GetPedigreeInput{
		PersonID: uuid.New(),
	})
	if err != query.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestGetPedigree_MaxGenerations(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewPedigreeService(readStore)

	child, _, _, _, _ := setupPedigreeTestData(t, readStore)

	ctx := context.Background()

	// Request only 1 generation
	result, err := svc.GetPedigree(ctx, query.GetPedigreeInput{
		PersonID:       child,
		MaxGenerations: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should have parents (generation 1) but no grandparents
	if result.Root.Father == nil {
		t.Error("Father should be present at generation 1")
	}
	if result.Root.Father.Father != nil {
		t.Error("Grandfather should be nil when max generation is 1")
	}
}

func TestGetPedigree_NoParents(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewPedigreeService(readStore)

	ctx := context.Background()

	// Create a person with no parent data
	orphan := uuid.New()
	err := readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        orphan,
		GivenName: "Orphan",
		Surname:   "Child",
		FullName:  "Orphan Child",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := svc.GetPedigree(ctx, query.GetPedigreeInput{
		PersonID: orphan,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Root == nil {
		t.Fatal("Root should not be nil")
	}
	if result.Root.Father != nil {
		t.Error("Father should be nil")
	}
	if result.Root.Mother != nil {
		t.Error("Mother should be nil")
	}
	if result.TotalAncestors != 0 {
		t.Errorf("TotalAncestors = %d, want 0", result.TotalAncestors)
	}
}

func TestGetAncestors(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewPedigreeService(readStore)

	child, father, mother, grandfather, grandmother := setupPedigreeTestData(t, readStore)

	ctx := context.Background()
	ancestors, err := svc.GetAncestors(ctx, child, 5)
	if err != nil {
		t.Fatal(err)
	}

	// Should have 4 ancestors: father, mother, grandfather, grandmother
	if len(ancestors) != 4 {
		t.Errorf("len(ancestors) = %d, want 4", len(ancestors))
	}

	// Check that all expected ancestors are present
	ancestorIDs := make(map[uuid.UUID]bool)
	for _, a := range ancestors {
		ancestorIDs[a.ID] = true
	}

	for _, expectedID := range []uuid.UUID{father, mother, grandfather, grandmother} {
		if !ancestorIDs[expectedID] {
			t.Errorf("Expected ancestor %v not found", expectedID)
		}
	}
}

func TestGetPedigree_DefaultMaxGenerations(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewPedigreeService(readStore)

	child, _, _, _, _ := setupPedigreeTestData(t, readStore)

	ctx := context.Background()

	// Request with 0 max generations should default to 5
	result, err := svc.GetPedigree(ctx, query.GetPedigreeInput{
		PersonID:       child,
		MaxGenerations: 0,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should still include all ancestors (only 2 generations in test data)
	if result.TotalAncestors != 4 {
		t.Errorf("TotalAncestors = %d, want 4", result.TotalAncestors)
	}
}

func TestGetPedigree_MaxGenerationsHardLimit(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewPedigreeService(readStore)

	child, _, _, _, _ := setupPedigreeTestData(t, readStore)

	ctx := context.Background()

	// Request with max generations > 10 should be capped to 10
	result, err := svc.GetPedigree(ctx, query.GetPedigreeInput{
		PersonID:       child,
		MaxGenerations: 100, // Should be capped to 10
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should still work and include all available ancestors
	if result.Root == nil {
		t.Fatal("Root should not be nil")
	}
}

func TestGetPedigree_CycleDetection(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewPedigreeService(readStore)

	ctx := context.Background()

	// Create a circular reference (which shouldn't happen in real data but we should handle it)
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
		Gender:    domain.GenderFemale,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create circular edge (person1's father is person2, and person2's father is person1)
	err = readStore.SavePedigreeEdge(ctx, &repository.PedigreeEdge{
		PersonID:   person1,
		FatherID:   &person2,
		FatherName: "Person2 Test",
	})
	if err != nil {
		t.Fatal(err)
	}

	err = readStore.SavePedigreeEdge(ctx, &repository.PedigreeEdge{
		PersonID:   person2,
		FatherID:   &person1,
		FatherName: "Person1 Test",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should not infinite loop due to cycle detection
	result, err := svc.GetPedigree(ctx, query.GetPedigreeInput{
		PersonID:       person1,
		MaxGenerations: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Root == nil {
		t.Fatal("Root should not be nil")
	}
	// Verify it handled the cycle gracefully
	if result.Root.Father == nil {
		t.Error("Should have father node")
	}
	// The cycle should be detected, so person2's father should be nil
	if result.Root.Father.Father != nil {
		t.Error("Cycle should have been detected, father's father should be nil")
	}
}

func TestGetAncestors_MaxGenerationsHardLimit(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewPedigreeService(readStore)

	child, _, _, _, _ := setupPedigreeTestData(t, readStore)

	ctx := context.Background()

	// Request with 0 max generations should default to 5
	ancestors, err := svc.GetAncestors(ctx, child, 0)
	if err != nil {
		t.Fatal(err)
	}

	if len(ancestors) != 4 {
		t.Errorf("Expected 4 ancestors with default limit, got %d", len(ancestors))
	}

	// Request with > 10 should be capped to 10
	ancestors2, err := svc.GetAncestors(ctx, child, 100)
	if err != nil {
		t.Fatal(err)
	}

	if len(ancestors2) != 4 {
		t.Errorf("Expected 4 ancestors with capped limit, got %d", len(ancestors2))
	}
}

func TestGetAncestors_NegativeMaxGenerations(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewPedigreeService(readStore)

	child, _, _, _, _ := setupPedigreeTestData(t, readStore)

	ctx := context.Background()

	// Negative max generations should default to 5
	ancestors, err := svc.GetAncestors(ctx, child, -1)
	if err != nil {
		t.Fatal(err)
	}

	if len(ancestors) != 4 {
		t.Errorf("Expected 4 ancestors with negative (defaulted) limit, got %d", len(ancestors))
	}
}

func TestGetAncestors_NoAncestors(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewPedigreeService(readStore)

	ctx := context.Background()

	// Create person with no parents
	orphan := uuid.New()
	err := readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        orphan,
		GivenName: "Orphan",
		Surname:   "Child",
		FullName:  "Orphan Child",
	})
	if err != nil {
		t.Fatal(err)
	}

	ancestors, err := svc.GetAncestors(ctx, orphan, 5)
	if err != nil {
		t.Fatal(err)
	}

	if len(ancestors) != 0 {
		t.Errorf("Expected 0 ancestors for orphan, got %d", len(ancestors))
	}
}

func TestGetAncestors_CycleDetection(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewPedigreeService(readStore)

	ctx := context.Background()

	// Create circular reference
	person1 := uuid.New()
	person2 := uuid.New()

	err := readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        person1,
		GivenName: "Person1",
		Surname:   "Test",
		FullName:  "Person1 Test",
	})
	if err != nil {
		t.Fatal(err)
	}

	err = readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        person2,
		GivenName: "Person2",
		Surname:   "Test",
		FullName:  "Person2 Test",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create circular edge
	err = readStore.SavePedigreeEdge(ctx, &repository.PedigreeEdge{
		PersonID:   person1,
		FatherID:   &person2,
		FatherName: "Person2 Test",
	})
	if err != nil {
		t.Fatal(err)
	}

	err = readStore.SavePedigreeEdge(ctx, &repository.PedigreeEdge{
		PersonID:   person2,
		FatherID:   &person1,
		FatherName: "Person1 Test",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should not infinite loop
	ancestors, err := svc.GetAncestors(ctx, person1, 5)
	if err != nil {
		t.Fatal(err)
	}

	// With cycle detection, we should get both person2 and person1 once each
	// The cycle prevents infinite recursion
	if len(ancestors) > 2 {
		t.Errorf("Expected at most 2 ancestors (cycle detected), got %d", len(ancestors))
	}
}

func TestBuildNode_AllOptionalFields(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewPedigreeService(readStore)

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
		BirthPlace:   "Boston, MA",
		DeathDateRaw: "20 DEC 2050",
		DeathPlace:   "New York, NY",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := svc.GetPedigree(ctx, query.GetPedigreeInput{
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
	if result.Root.BirthPlace == nil {
		t.Error("BirthPlace should be set")
	}
	if result.Root.DeathDate == nil {
		t.Error("DeathDate should be set")
	}
	if result.Root.DeathPlace == nil {
		t.Error("DeathPlace should be set")
	}
}

func TestCountAncestors_NilNode(t *testing.T) {
	// This tests the countAncestors function with a nil node
	// which is an edge case that should be handled gracefully
	readStore := memory.NewReadModelStore()
	svc := query.NewPedigreeService(readStore)

	ctx := context.Background()

	// Create person with no parents
	orphan := uuid.New()
	err := readStore.SavePerson(ctx, &repository.PersonReadModel{
		ID:        orphan,
		GivenName: "Orphan",
		Surname:   "Child",
		FullName:  "Orphan Child",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := svc.GetPedigree(ctx, query.GetPedigreeInput{
		PersonID:       orphan,
		MaxGenerations: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	// countAncestors will be called with nil nodes (father/mother)
	if result.TotalAncestors != 0 {
		t.Errorf("TotalAncestors = %d, want 0", result.TotalAncestors)
	}
	if result.MaxGeneration != 0 {
		t.Errorf("MaxGeneration = %d, want 0", result.MaxGeneration)
	}
}
