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
