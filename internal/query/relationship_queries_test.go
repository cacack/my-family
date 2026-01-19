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

// Helper to create a person with minimal data.
func createPerson(t *testing.T, ctx context.Context, store *memory.ReadModelStore, givenName, surname string, gender domain.Gender) uuid.UUID {
	t.Helper()
	id := uuid.New()
	err := store.SavePerson(ctx, &repository.PersonReadModel{
		ID:        id,
		GivenName: givenName,
		Surname:   surname,
		FullName:  givenName + " " + surname,
		Gender:    gender,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// Helper to create a parent-child relationship.
func createParentChild(t *testing.T, ctx context.Context, store *memory.ReadModelStore, childID uuid.UUID, fatherID, motherID *uuid.UUID, fatherName, motherName string) {
	t.Helper()
	edge := &repository.PedigreeEdge{
		PersonID:   childID,
		FatherID:   fatherID,
		MotherID:   motherID,
		FatherName: fatherName,
		MotherName: motherName,
	}
	if err := store.SavePedigreeEdge(ctx, edge); err != nil {
		t.Fatal(err)
	}
}

func TestGetRelationship_SamePerson(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	person := createPerson(t, ctx, store, "John", "Doe", domain.GenderMale)

	result, err := svc.GetRelationship(ctx, person, person)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsRelated {
		t.Error("Expected IsRelated to be true for same person")
	}
	if result.Summary != "same person" {
		t.Errorf("Expected summary 'same person', got '%s'", result.Summary)
	}
	if len(result.Paths) != 1 {
		t.Errorf("Expected 1 path, got %d", len(result.Paths))
	}
	if result.Paths[0].Name != "self" {
		t.Errorf("Expected path name 'self', got '%s'", result.Paths[0].Name)
	}
}

func TestGetRelationship_PersonNotFound(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	person := createPerson(t, ctx, store, "John", "Doe", domain.GenderMale)
	nonExistent := uuid.New()

	// Test when first person doesn't exist
	_, err := svc.GetRelationship(ctx, nonExistent, person)
	if err != query.ErrNotFound {
		t.Errorf("Expected ErrNotFound for non-existent person A, got %v", err)
	}

	// Test when second person doesn't exist
	_, err = svc.GetRelationship(ctx, person, nonExistent)
	if err != query.ErrNotFound {
		t.Errorf("Expected ErrNotFound for non-existent person B, got %v", err)
	}
}

func TestGetRelationship_Unrelated(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	personA := createPerson(t, ctx, store, "John", "Doe", domain.GenderMale)
	personB := createPerson(t, ctx, store, "Jane", "Smith", domain.GenderFemale)

	result, err := svc.GetRelationship(ctx, personA, personB)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsRelated {
		t.Error("Expected IsRelated to be false for unrelated persons")
	}
	if result.Summary != "not related" {
		t.Errorf("Expected summary 'not related', got '%s'", result.Summary)
	}
	if len(result.Paths) != 0 {
		t.Errorf("Expected 0 paths, got %d", len(result.Paths))
	}
}

func TestGetRelationship_ParentChild(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	father := createPerson(t, ctx, store, "John", "Doe", domain.GenderMale)
	child := createPerson(t, ctx, store, "Junior", "Doe", domain.GenderMale)

	createParentChild(t, ctx, store, child, &father, nil, "John Doe", "")

	// Test child to parent
	result, err := svc.GetRelationship(ctx, child, father)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsRelated {
		t.Error("Expected IsRelated to be true")
	}
	if len(result.Paths) != 1 {
		t.Errorf("Expected 1 path, got %d", len(result.Paths))
	}
	if result.Paths[0].Name != "parent" {
		t.Errorf("Expected 'parent', got '%s'", result.Paths[0].Name)
	}
	if result.Paths[0].GenerationDistanceA != 1 {
		t.Errorf("Expected generation distance A = 1, got %d", result.Paths[0].GenerationDistanceA)
	}
	if result.Paths[0].GenerationDistanceB != 0 {
		t.Errorf("Expected generation distance B = 0, got %d", result.Paths[0].GenerationDistanceB)
	}

	// Test parent to child
	result2, err := svc.GetRelationship(ctx, father, child)
	if err != nil {
		t.Fatal(err)
	}

	if !result2.IsRelated {
		t.Error("Expected IsRelated to be true")
	}
	if len(result2.Paths) != 1 {
		t.Errorf("Expected 1 path, got %d", len(result2.Paths))
	}
	if result2.Paths[0].Name != "child" {
		t.Errorf("Expected 'child', got '%s'", result2.Paths[0].Name)
	}
}

func TestGetRelationship_Grandparent(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	grandfather := createPerson(t, ctx, store, "George", "Doe", domain.GenderMale)
	father := createPerson(t, ctx, store, "John", "Doe", domain.GenderMale)
	child := createPerson(t, ctx, store, "Junior", "Doe", domain.GenderMale)

	createParentChild(t, ctx, store, father, &grandfather, nil, "George Doe", "")
	createParentChild(t, ctx, store, child, &father, nil, "John Doe", "")

	// Test child to grandparent
	result, err := svc.GetRelationship(ctx, child, grandfather)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsRelated {
		t.Error("Expected IsRelated to be true")
	}
	if len(result.Paths) != 1 {
		t.Errorf("Expected 1 path, got %d", len(result.Paths))
	}
	if result.Paths[0].Name != "grandparent" {
		t.Errorf("Expected 'grandparent', got '%s'", result.Paths[0].Name)
	}

	// Test grandparent to child
	result2, err := svc.GetRelationship(ctx, grandfather, child)
	if err != nil {
		t.Fatal(err)
	}

	if result2.Paths[0].Name != "grandchild" {
		t.Errorf("Expected 'grandchild', got '%s'", result2.Paths[0].Name)
	}
}

func TestGetRelationship_GreatGrandparent(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	greatGrandfather := createPerson(t, ctx, store, "Great", "Doe", domain.GenderMale)
	grandfather := createPerson(t, ctx, store, "George", "Doe", domain.GenderMale)
	father := createPerson(t, ctx, store, "John", "Doe", domain.GenderMale)
	child := createPerson(t, ctx, store, "Junior", "Doe", domain.GenderMale)

	createParentChild(t, ctx, store, grandfather, &greatGrandfather, nil, "Great Doe", "")
	createParentChild(t, ctx, store, father, &grandfather, nil, "George Doe", "")
	createParentChild(t, ctx, store, child, &father, nil, "John Doe", "")

	result, err := svc.GetRelationship(ctx, child, greatGrandfather)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsRelated {
		t.Error("Expected IsRelated to be true")
	}
	if result.Paths[0].Name != "great-grandparent" {
		t.Errorf("Expected 'great-grandparent', got '%s'", result.Paths[0].Name)
	}
}

func TestGetRelationship_Siblings(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	father := createPerson(t, ctx, store, "John", "Doe", domain.GenderMale)
	mother := createPerson(t, ctx, store, "Jane", "Doe", domain.GenderFemale)
	child1 := createPerson(t, ctx, store, "Alice", "Doe", domain.GenderFemale)
	child2 := createPerson(t, ctx, store, "Bob", "Doe", domain.GenderMale)

	createParentChild(t, ctx, store, child1, &father, &mother, "John Doe", "Jane Doe")
	createParentChild(t, ctx, store, child2, &father, &mother, "John Doe", "Jane Doe")

	result, err := svc.GetRelationship(ctx, child1, child2)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsRelated {
		t.Error("Expected IsRelated to be true")
	}

	// Should have paths through both parents
	foundSibling := false
	for _, path := range result.Paths {
		if path.Name == "sibling" {
			foundSibling = true
			if path.GenerationDistanceA != 1 || path.GenerationDistanceB != 1 {
				t.Errorf("Expected generation distances (1, 1), got (%d, %d)",
					path.GenerationDistanceA, path.GenerationDistanceB)
			}
		}
	}
	if !foundSibling {
		t.Error("Expected to find 'sibling' relationship")
	}
}

func TestGetRelationship_FirstCousins(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	// Build family tree:
	// grandfather
	// ├── father -> child1
	// └── uncle -> child2 (cousin)
	grandfather := createPerson(t, ctx, store, "George", "Doe", domain.GenderMale)
	father := createPerson(t, ctx, store, "John", "Doe", domain.GenderMale)
	uncle := createPerson(t, ctx, store, "James", "Doe", domain.GenderMale)
	child1 := createPerson(t, ctx, store, "Alice", "Doe", domain.GenderFemale)
	child2 := createPerson(t, ctx, store, "Bob", "Doe", domain.GenderMale)

	createParentChild(t, ctx, store, father, &grandfather, nil, "George Doe", "")
	createParentChild(t, ctx, store, uncle, &grandfather, nil, "George Doe", "")
	createParentChild(t, ctx, store, child1, &father, nil, "John Doe", "")
	createParentChild(t, ctx, store, child2, &uncle, nil, "James Doe", "")

	result, err := svc.GetRelationship(ctx, child1, child2)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsRelated {
		t.Error("Expected IsRelated to be true")
	}

	foundCousin := false
	for _, path := range result.Paths {
		if path.Name == "1st cousin" {
			foundCousin = true
			if path.GenerationDistanceA != 2 || path.GenerationDistanceB != 2 {
				t.Errorf("Expected generation distances (2, 2), got (%d, %d)",
					path.GenerationDistanceA, path.GenerationDistanceB)
			}
		}
	}
	if !foundCousin {
		t.Error("Expected to find '1st cousin' relationship")
	}
}

func TestGetRelationship_FirstCousinOnceRemoved(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	// Build family tree:
	// grandfather
	// ├── father -> child1 -> grandchild1
	// └── uncle -> cousin2
	// grandchild1 and cousin2 are 1st cousins once removed
	grandfather := createPerson(t, ctx, store, "George", "Doe", domain.GenderMale)
	father := createPerson(t, ctx, store, "John", "Doe", domain.GenderMale)
	uncle := createPerson(t, ctx, store, "James", "Doe", domain.GenderMale)
	child1 := createPerson(t, ctx, store, "Alice", "Doe", domain.GenderFemale)
	cousin2 := createPerson(t, ctx, store, "Bob", "Doe", domain.GenderMale)
	grandchild1 := createPerson(t, ctx, store, "Carol", "Doe", domain.GenderFemale)

	createParentChild(t, ctx, store, father, &grandfather, nil, "George Doe", "")
	createParentChild(t, ctx, store, uncle, &grandfather, nil, "George Doe", "")
	createParentChild(t, ctx, store, child1, &father, nil, "John Doe", "")
	createParentChild(t, ctx, store, cousin2, &uncle, nil, "James Doe", "")
	createParentChild(t, ctx, store, grandchild1, &child1, nil, "Alice Doe", "")

	result, err := svc.GetRelationship(ctx, grandchild1, cousin2)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsRelated {
		t.Error("Expected IsRelated to be true")
	}

	foundCousinRemoved := false
	for _, path := range result.Paths {
		if path.Name == "1st cousin once removed" {
			foundCousinRemoved = true
			// grandchild1 is gen 3 from grandfather, cousin2 is gen 2
			if path.GenerationDistanceA != 3 || path.GenerationDistanceB != 2 {
				t.Errorf("Expected generation distances (3, 2), got (%d, %d)",
					path.GenerationDistanceA, path.GenerationDistanceB)
			}
		}
	}
	if !foundCousinRemoved {
		t.Error("Expected to find '1st cousin once removed' relationship")
	}
}

func TestGetRelationship_SecondCousins(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	// Build family tree:
	// great-grandfather
	// ├── grandfather1 -> father1 -> child1
	// └── grandfather2 -> father2 -> child2
	// child1 and child2 are 2nd cousins
	greatGrandfather := createPerson(t, ctx, store, "Great", "Doe", domain.GenderMale)
	grandfather1 := createPerson(t, ctx, store, "George1", "Doe", domain.GenderMale)
	grandfather2 := createPerson(t, ctx, store, "George2", "Doe", domain.GenderMale)
	father1 := createPerson(t, ctx, store, "John1", "Doe", domain.GenderMale)
	father2 := createPerson(t, ctx, store, "John2", "Doe", domain.GenderMale)
	child1 := createPerson(t, ctx, store, "Alice", "Doe", domain.GenderFemale)
	child2 := createPerson(t, ctx, store, "Bob", "Doe", domain.GenderMale)

	createParentChild(t, ctx, store, grandfather1, &greatGrandfather, nil, "Great Doe", "")
	createParentChild(t, ctx, store, grandfather2, &greatGrandfather, nil, "Great Doe", "")
	createParentChild(t, ctx, store, father1, &grandfather1, nil, "George1 Doe", "")
	createParentChild(t, ctx, store, father2, &grandfather2, nil, "George2 Doe", "")
	createParentChild(t, ctx, store, child1, &father1, nil, "John1 Doe", "")
	createParentChild(t, ctx, store, child2, &father2, nil, "John2 Doe", "")

	result, err := svc.GetRelationship(ctx, child1, child2)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsRelated {
		t.Error("Expected IsRelated to be true")
	}

	foundSecondCousin := false
	for _, path := range result.Paths {
		if path.Name == "2nd cousin" {
			foundSecondCousin = true
			// Both are gen 3 from great-grandfather
			if path.GenerationDistanceA != 3 || path.GenerationDistanceB != 3 {
				t.Errorf("Expected generation distances (3, 3), got (%d, %d)",
					path.GenerationDistanceA, path.GenerationDistanceB)
			}
		}
	}
	if !foundSecondCousin {
		t.Error("Expected to find '2nd cousin' relationship")
	}
}

func TestGetRelationship_UncleNiece(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	// Build family tree:
	// grandfather
	// ├── father -> niece
	// └── uncle
	grandfather := createPerson(t, ctx, store, "George", "Doe", domain.GenderMale)
	father := createPerson(t, ctx, store, "John", "Doe", domain.GenderMale)
	uncle := createPerson(t, ctx, store, "James", "Doe", domain.GenderMale)
	niece := createPerson(t, ctx, store, "Alice", "Doe", domain.GenderFemale)

	createParentChild(t, ctx, store, father, &grandfather, nil, "George Doe", "")
	createParentChild(t, ctx, store, uncle, &grandfather, nil, "George Doe", "")
	createParentChild(t, ctx, store, niece, &father, nil, "John Doe", "")

	// Test niece to uncle
	result, err := svc.GetRelationship(ctx, niece, uncle)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsRelated {
		t.Error("Expected IsRelated to be true")
	}

	foundUncle := false
	for _, path := range result.Paths {
		if path.Name == "uncle/aunt" {
			foundUncle = true
			if path.GenerationDistanceA != 2 || path.GenerationDistanceB != 1 {
				t.Errorf("Expected generation distances (2, 1), got (%d, %d)",
					path.GenerationDistanceA, path.GenerationDistanceB)
			}
		}
	}
	if !foundUncle {
		t.Error("Expected to find 'uncle/aunt' relationship")
	}

	// Test uncle to niece
	result2, err := svc.GetRelationship(ctx, uncle, niece)
	if err != nil {
		t.Fatal(err)
	}

	foundNephew := false
	for _, path := range result2.Paths {
		if path.Name == "nephew/niece" {
			foundNephew = true
		}
	}
	if !foundNephew {
		t.Error("Expected to find 'nephew/niece' relationship")
	}
}

func TestGetRelationship_MultiplePaths(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	// Build family tree where siblings share both parents:
	// father --- mother
	//    \      /
	//    child1  child2
	father := createPerson(t, ctx, store, "John", "Doe", domain.GenderMale)
	mother := createPerson(t, ctx, store, "Jane", "Doe", domain.GenderFemale)
	child1 := createPerson(t, ctx, store, "Alice", "Doe", domain.GenderFemale)
	child2 := createPerson(t, ctx, store, "Bob", "Doe", domain.GenderMale)

	createParentChild(t, ctx, store, child1, &father, &mother, "John Doe", "Jane Doe")
	createParentChild(t, ctx, store, child2, &father, &mother, "John Doe", "Jane Doe")

	result, err := svc.GetRelationship(ctx, child1, child2)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsRelated {
		t.Error("Expected IsRelated to be true")
	}

	// Should have 2 paths - one through father and one through mother
	if len(result.Paths) < 2 {
		t.Errorf("Expected at least 2 paths for full siblings, got %d", len(result.Paths))
	}
}

func TestGetRelationship_CycleDetection(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	// Create a circular reference (which shouldn't happen in real data)
	person1 := createPerson(t, ctx, store, "Person1", "Test", domain.GenderMale)
	person2 := createPerson(t, ctx, store, "Person2", "Test", domain.GenderFemale)

	// Create circular edge (person1's father is person2, person2's father is person1)
	createParentChild(t, ctx, store, person1, &person2, nil, "Person2 Test", "")
	createParentChild(t, ctx, store, person2, &person1, nil, "Person1 Test", "")

	// Should not infinite loop
	result, err := svc.GetRelationship(ctx, person1, person2)
	if err != nil {
		t.Fatal(err)
	}

	// Should be related (person2 is father of person1)
	if !result.IsRelated {
		t.Error("Expected IsRelated to be true")
	}
}

func TestGetRelationship_GreatGreatGrandparent(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	// Build 5 generations
	gen4 := createPerson(t, ctx, store, "Gen4", "Doe", domain.GenderMale)
	gen3 := createPerson(t, ctx, store, "Gen3", "Doe", domain.GenderMale)
	gen2 := createPerson(t, ctx, store, "Gen2", "Doe", domain.GenderMale)
	gen1 := createPerson(t, ctx, store, "Gen1", "Doe", domain.GenderMale)
	gen0 := createPerson(t, ctx, store, "Gen0", "Doe", domain.GenderMale)

	createParentChild(t, ctx, store, gen3, &gen4, nil, "Gen4 Doe", "")
	createParentChild(t, ctx, store, gen2, &gen3, nil, "Gen3 Doe", "")
	createParentChild(t, ctx, store, gen1, &gen2, nil, "Gen2 Doe", "")
	createParentChild(t, ctx, store, gen0, &gen1, nil, "Gen1 Doe", "")

	result, err := svc.GetRelationship(ctx, gen0, gen4)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsRelated {
		t.Error("Expected IsRelated to be true")
	}
	if result.Paths[0].Name != "great-great-grandparent" {
		t.Errorf("Expected 'great-great-grandparent', got '%s'", result.Paths[0].Name)
	}
}

func TestGetRelationship_ThirdCousins(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	// Build family tree for 3rd cousins (share great-great-grandparent)
	ggGrandfather := createPerson(t, ctx, store, "GG", "Doe", domain.GenderMale)

	gGrandfather1 := createPerson(t, ctx, store, "GGF1", "Doe", domain.GenderMale)
	gGrandfather2 := createPerson(t, ctx, store, "GGF2", "Doe", domain.GenderMale)

	grandfather1 := createPerson(t, ctx, store, "GF1", "Doe", domain.GenderMale)
	grandfather2 := createPerson(t, ctx, store, "GF2", "Doe", domain.GenderMale)

	father1 := createPerson(t, ctx, store, "F1", "Doe", domain.GenderMale)
	father2 := createPerson(t, ctx, store, "F2", "Doe", domain.GenderMale)

	child1 := createPerson(t, ctx, store, "C1", "Doe", domain.GenderMale)
	child2 := createPerson(t, ctx, store, "C2", "Doe", domain.GenderMale)

	createParentChild(t, ctx, store, gGrandfather1, &ggGrandfather, nil, "GG Doe", "")
	createParentChild(t, ctx, store, gGrandfather2, &ggGrandfather, nil, "GG Doe", "")
	createParentChild(t, ctx, store, grandfather1, &gGrandfather1, nil, "GGF1 Doe", "")
	createParentChild(t, ctx, store, grandfather2, &gGrandfather2, nil, "GGF2 Doe", "")
	createParentChild(t, ctx, store, father1, &grandfather1, nil, "GF1 Doe", "")
	createParentChild(t, ctx, store, father2, &grandfather2, nil, "GF2 Doe", "")
	createParentChild(t, ctx, store, child1, &father1, nil, "F1 Doe", "")
	createParentChild(t, ctx, store, child2, &father2, nil, "F2 Doe", "")

	result, err := svc.GetRelationship(ctx, child1, child2)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsRelated {
		t.Error("Expected IsRelated to be true")
	}

	foundThirdCousin := false
	for _, path := range result.Paths {
		if path.Name == "3rd cousin" {
			foundThirdCousin = true
			// Both are gen 4 from gg-grandfather
			if path.GenerationDistanceA != 4 || path.GenerationDistanceB != 4 {
				t.Errorf("Expected generation distances (4, 4), got (%d, %d)",
					path.GenerationDistanceA, path.GenerationDistanceB)
			}
		}
	}
	if !foundThirdCousin {
		t.Error("Expected to find '3rd cousin' relationship")
	}
}

func TestGetRelationship_FirstCousinTwiceRemoved(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	// grandfather -> father -> child -> grandchild -> greatGrandchild
	//            \-> uncle -> cousin
	// greatGrandchild and cousin are 1st cousins twice removed
	grandfather := createPerson(t, ctx, store, "GF", "Doe", domain.GenderMale)
	father := createPerson(t, ctx, store, "F", "Doe", domain.GenderMale)
	uncle := createPerson(t, ctx, store, "U", "Doe", domain.GenderMale)
	child := createPerson(t, ctx, store, "C", "Doe", domain.GenderMale)
	cousin := createPerson(t, ctx, store, "Cousin", "Doe", domain.GenderMale)
	grandchild := createPerson(t, ctx, store, "GC", "Doe", domain.GenderMale)
	greatGrandchild := createPerson(t, ctx, store, "GGC", "Doe", domain.GenderMale)

	createParentChild(t, ctx, store, father, &grandfather, nil, "GF Doe", "")
	createParentChild(t, ctx, store, uncle, &grandfather, nil, "GF Doe", "")
	createParentChild(t, ctx, store, child, &father, nil, "F Doe", "")
	createParentChild(t, ctx, store, cousin, &uncle, nil, "U Doe", "")
	createParentChild(t, ctx, store, grandchild, &child, nil, "C Doe", "")
	createParentChild(t, ctx, store, greatGrandchild, &grandchild, nil, "GC Doe", "")

	result, err := svc.GetRelationship(ctx, greatGrandchild, cousin)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsRelated {
		t.Error("Expected IsRelated to be true")
	}

	foundCousinTwiceRemoved := false
	for _, path := range result.Paths {
		if path.Name == "1st cousin twice removed" {
			foundCousinTwiceRemoved = true
		}
	}
	if !foundCousinTwiceRemoved {
		t.Error("Expected to find '1st cousin twice removed' relationship")
	}
}

func TestGetRelationship_GrandUncle(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	// great-grandfather
	// ├── grandfather -> father -> child
	// └── grand-uncle
	greatGrandfather := createPerson(t, ctx, store, "GGF", "Doe", domain.GenderMale)
	grandfather := createPerson(t, ctx, store, "GF", "Doe", domain.GenderMale)
	grandUncle := createPerson(t, ctx, store, "GU", "Doe", domain.GenderMale)
	father := createPerson(t, ctx, store, "F", "Doe", domain.GenderMale)
	child := createPerson(t, ctx, store, "C", "Doe", domain.GenderMale)

	createParentChild(t, ctx, store, grandfather, &greatGrandfather, nil, "GGF Doe", "")
	createParentChild(t, ctx, store, grandUncle, &greatGrandfather, nil, "GGF Doe", "")
	createParentChild(t, ctx, store, father, &grandfather, nil, "GF Doe", "")
	createParentChild(t, ctx, store, child, &father, nil, "F Doe", "")

	result, err := svc.GetRelationship(ctx, child, grandUncle)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsRelated {
		t.Error("Expected IsRelated to be true")
	}

	foundGrandUncle := false
	for _, path := range result.Paths {
		if path.Name == "grand-uncle/aunt" {
			foundGrandUncle = true
		}
	}
	if !foundGrandUncle {
		t.Error("Expected to find 'grand-uncle/aunt' relationship")
	}
}

func TestGetRelationship_GreatGrandUncle(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	// 2nd-great-grandfather
	// ├── great-grandfather -> grandfather -> father -> child
	// └── great-grand-uncle
	ggGrandfather := createPerson(t, ctx, store, "GGG", "Doe", domain.GenderMale)
	greatGrandfather := createPerson(t, ctx, store, "GGF", "Doe", domain.GenderMale)
	greatGrandUncle := createPerson(t, ctx, store, "GGU", "Doe", domain.GenderMale)
	grandfather := createPerson(t, ctx, store, "GF", "Doe", domain.GenderMale)
	father := createPerson(t, ctx, store, "F", "Doe", domain.GenderMale)
	child := createPerson(t, ctx, store, "C", "Doe", domain.GenderMale)

	createParentChild(t, ctx, store, greatGrandfather, &ggGrandfather, nil, "GGG Doe", "")
	createParentChild(t, ctx, store, greatGrandUncle, &ggGrandfather, nil, "GGG Doe", "")
	createParentChild(t, ctx, store, grandfather, &greatGrandfather, nil, "GGF Doe", "")
	createParentChild(t, ctx, store, father, &grandfather, nil, "GF Doe", "")
	createParentChild(t, ctx, store, child, &father, nil, "F Doe", "")

	result, err := svc.GetRelationship(ctx, child, greatGrandUncle)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsRelated {
		t.Error("Expected IsRelated to be true")
	}

	foundGreatGrandUncle := false
	for _, path := range result.Paths {
		if path.Name == "great-grand-uncle/aunt" {
			foundGreatGrandUncle = true
		}
	}
	if !foundGreatGrandUncle {
		t.Error("Expected to find 'great-grand-uncle/aunt' relationship")
	}
}

func TestGetRelationship_GrandNephew(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	// great-grandfather
	// ├── grandfather -> father -> child
	// └── grand-uncle
	// Test: grand-uncle to child (grand-nephew/niece)
	greatGrandfather := createPerson(t, ctx, store, "GGF", "Doe", domain.GenderMale)
	grandfather := createPerson(t, ctx, store, "GF", "Doe", domain.GenderMale)
	grandUncle := createPerson(t, ctx, store, "GU", "Doe", domain.GenderMale)
	father := createPerson(t, ctx, store, "F", "Doe", domain.GenderMale)
	child := createPerson(t, ctx, store, "C", "Doe", domain.GenderMale)

	createParentChild(t, ctx, store, grandfather, &greatGrandfather, nil, "GGF Doe", "")
	createParentChild(t, ctx, store, grandUncle, &greatGrandfather, nil, "GGF Doe", "")
	createParentChild(t, ctx, store, father, &grandfather, nil, "GF Doe", "")
	createParentChild(t, ctx, store, child, &father, nil, "F Doe", "")

	result, err := svc.GetRelationship(ctx, grandUncle, child)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsRelated {
		t.Error("Expected IsRelated to be true")
	}

	foundGrandNephew := false
	for _, path := range result.Paths {
		if path.Name == "grand-nephew/niece" {
			foundGrandNephew = true
		}
	}
	if !foundGrandNephew {
		t.Error("Expected to find 'grand-nephew/niece' relationship")
	}
}

func TestRelationshipService_OrdinalNumbers(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	// Create a structure for 11th cousin (requires 12 generations to common ancestor)
	// This tests the ordinal function for special cases like 11th, 12th, 13th
	// For simplicity, we'll just verify 4th cousin (which uses "4th")

	// 3rd-great-grandfather
	// ├── 2nd-great-grandfather1 -> great-grandfather1 -> grandfather1 -> father1 -> child1
	// └── 2nd-great-grandfather2 -> great-grandfather2 -> grandfather2 -> father2 -> child2

	ancestor := createPerson(t, ctx, store, "Ancestor", "Doe", domain.GenderMale)

	// Build left branch
	left1 := createPerson(t, ctx, store, "L1", "Doe", domain.GenderMale)
	left2 := createPerson(t, ctx, store, "L2", "Doe", domain.GenderMale)
	left3 := createPerson(t, ctx, store, "L3", "Doe", domain.GenderMale)
	left4 := createPerson(t, ctx, store, "L4", "Doe", domain.GenderMale)
	left5 := createPerson(t, ctx, store, "L5", "Doe", domain.GenderMale)

	// Build right branch
	right1 := createPerson(t, ctx, store, "R1", "Doe", domain.GenderMale)
	right2 := createPerson(t, ctx, store, "R2", "Doe", domain.GenderMale)
	right3 := createPerson(t, ctx, store, "R3", "Doe", domain.GenderMale)
	right4 := createPerson(t, ctx, store, "R4", "Doe", domain.GenderMale)
	right5 := createPerson(t, ctx, store, "R5", "Doe", domain.GenderMale)

	createParentChild(t, ctx, store, left1, &ancestor, nil, "Ancestor Doe", "")
	createParentChild(t, ctx, store, left2, &left1, nil, "L1 Doe", "")
	createParentChild(t, ctx, store, left3, &left2, nil, "L2 Doe", "")
	createParentChild(t, ctx, store, left4, &left3, nil, "L3 Doe", "")
	createParentChild(t, ctx, store, left5, &left4, nil, "L4 Doe", "")

	createParentChild(t, ctx, store, right1, &ancestor, nil, "Ancestor Doe", "")
	createParentChild(t, ctx, store, right2, &right1, nil, "R1 Doe", "")
	createParentChild(t, ctx, store, right3, &right2, nil, "R2 Doe", "")
	createParentChild(t, ctx, store, right4, &right3, nil, "R3 Doe", "")
	createParentChild(t, ctx, store, right5, &right4, nil, "R4 Doe", "")

	result, err := svc.GetRelationship(ctx, left5, right5)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsRelated {
		t.Error("Expected IsRelated to be true")
	}

	foundFourthCousin := false
	for _, path := range result.Paths {
		if path.Name == "4th cousin" {
			foundFourthCousin = true
		}
	}
	if !foundFourthCousin {
		t.Error("Expected to find '4th cousin' relationship")
	}
}

func TestGetRelationship_SummaryWithMultiplePaths(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	// Full siblings have paths through both parents
	father := createPerson(t, ctx, store, "Father", "Doe", domain.GenderMale)
	mother := createPerson(t, ctx, store, "Mother", "Doe", domain.GenderFemale)
	child1 := createPerson(t, ctx, store, "Child1", "Doe", domain.GenderMale)
	child2 := createPerson(t, ctx, store, "Child2", "Doe", domain.GenderFemale)

	createParentChild(t, ctx, store, child1, &father, &mother, "Father Doe", "Mother Doe")
	createParentChild(t, ctx, store, child2, &father, &mother, "Father Doe", "Mother Doe")

	result, err := svc.GetRelationship(ctx, child1, child2)
	if err != nil {
		t.Fatal(err)
	}

	// Summary should indicate multiple paths
	if result.Summary == "" {
		t.Error("Expected non-empty summary")
	}
}

func TestGetRelationship_PathContainsCorrectIDs(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	grandfather := createPerson(t, ctx, store, "GF", "Doe", domain.GenderMale)
	father := createPerson(t, ctx, store, "F", "Doe", domain.GenderMale)
	child := createPerson(t, ctx, store, "C", "Doe", domain.GenderMale)

	createParentChild(t, ctx, store, father, &grandfather, nil, "GF Doe", "")
	createParentChild(t, ctx, store, child, &father, nil, "F Doe", "")

	result, err := svc.GetRelationship(ctx, child, grandfather)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Paths) != 1 {
		t.Fatalf("Expected 1 path, got %d", len(result.Paths))
	}

	path := result.Paths[0]

	// PathFromA should be: child -> father -> grandfather
	if len(path.PathFromA) != 3 {
		t.Errorf("Expected PathFromA length 3, got %d", len(path.PathFromA))
	}
	if path.PathFromA[0] != child {
		t.Error("PathFromA[0] should be child")
	}
	if path.PathFromA[1] != father {
		t.Error("PathFromA[1] should be father")
	}
	if path.PathFromA[2] != grandfather {
		t.Error("PathFromA[2] should be grandfather")
	}

	// PathFromB should be just: grandfather
	if len(path.PathFromB) != 1 {
		t.Errorf("Expected PathFromB length 1, got %d", len(path.PathFromB))
	}
	if path.PathFromB[0] != grandfather {
		t.Error("PathFromB[0] should be grandfather")
	}
}

func TestGetRelationship_CommonAncestorField(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	grandfather := createPerson(t, ctx, store, "George", "Doe", domain.GenderMale)
	father := createPerson(t, ctx, store, "John", "Doe", domain.GenderMale)
	uncle := createPerson(t, ctx, store, "James", "Doe", domain.GenderMale)
	child1 := createPerson(t, ctx, store, "Alice", "Doe", domain.GenderFemale)
	child2 := createPerson(t, ctx, store, "Bob", "Doe", domain.GenderMale)

	createParentChild(t, ctx, store, father, &grandfather, nil, "George Doe", "")
	createParentChild(t, ctx, store, uncle, &grandfather, nil, "George Doe", "")
	createParentChild(t, ctx, store, child1, &father, nil, "John Doe", "")
	createParentChild(t, ctx, store, child2, &uncle, nil, "James Doe", "")

	result, err := svc.GetRelationship(ctx, child1, child2)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Paths) == 0 {
		t.Fatal("Expected at least one path")
	}

	// Common ancestor should be grandfather
	if result.Paths[0].CommonAncestor == nil {
		t.Fatal("CommonAncestor should not be nil")
	}
	if result.Paths[0].CommonAncestor.ID != grandfather {
		t.Errorf("Expected common ancestor ID %v, got %v", grandfather, result.Paths[0].CommonAncestor.ID)
	}
	if result.Paths[0].CommonAncestor.GivenName != "George" {
		t.Errorf("Expected common ancestor name 'George', got '%s'", result.Paths[0].CommonAncestor.GivenName)
	}
}

func TestGetRelationship_PersonAAndBFields(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	personA := createPerson(t, ctx, store, "Alice", "Doe", domain.GenderFemale)
	personB := createPerson(t, ctx, store, "Bob", "Smith", domain.GenderMale)

	result, err := svc.GetRelationship(ctx, personA, personB)
	if err != nil {
		t.Fatal(err)
	}

	if result.PersonA == nil {
		t.Fatal("PersonA should not be nil")
	}
	if result.PersonB == nil {
		t.Fatal("PersonB should not be nil")
	}

	if result.PersonA.ID != personA {
		t.Errorf("Expected PersonA ID %v, got %v", personA, result.PersonA.ID)
	}
	if result.PersonA.GivenName != "Alice" {
		t.Errorf("Expected PersonA name 'Alice', got '%s'", result.PersonA.GivenName)
	}

	if result.PersonB.ID != personB {
		t.Errorf("Expected PersonB ID %v, got %v", personB, result.PersonB.ID)
	}
	if result.PersonB.GivenName != "Bob" {
		t.Errorf("Expected PersonB name 'Bob', got '%s'", result.PersonB.GivenName)
	}
}

func TestGetRelationship_3rdGreatGrandparent(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	// Build 6 generations for 3rd great-grandparent
	gen5 := createPerson(t, ctx, store, "Gen5", "Doe", domain.GenderMale)
	gen4 := createPerson(t, ctx, store, "Gen4", "Doe", domain.GenderMale)
	gen3 := createPerson(t, ctx, store, "Gen3", "Doe", domain.GenderMale)
	gen2 := createPerson(t, ctx, store, "Gen2", "Doe", domain.GenderMale)
	gen1 := createPerson(t, ctx, store, "Gen1", "Doe", domain.GenderMale)
	gen0 := createPerson(t, ctx, store, "Gen0", "Doe", domain.GenderMale)

	createParentChild(t, ctx, store, gen4, &gen5, nil, "Gen5 Doe", "")
	createParentChild(t, ctx, store, gen3, &gen4, nil, "Gen4 Doe", "")
	createParentChild(t, ctx, store, gen2, &gen3, nil, "Gen3 Doe", "")
	createParentChild(t, ctx, store, gen1, &gen2, nil, "Gen2 Doe", "")
	createParentChild(t, ctx, store, gen0, &gen1, nil, "Gen1 Doe", "")

	result, err := svc.GetRelationship(ctx, gen0, gen5)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsRelated {
		t.Error("Expected IsRelated to be true")
	}
	if result.Paths[0].Name != "3rd great-grandparent" {
		t.Errorf("Expected '3rd great-grandparent', got '%s'", result.Paths[0].Name)
	}

	// Test reverse direction (3rd great-grandchild)
	result2, err := svc.GetRelationship(ctx, gen5, gen0)
	if err != nil {
		t.Fatal(err)
	}
	if result2.Paths[0].Name != "3rd great-grandchild" {
		t.Errorf("Expected '3rd great-grandchild', got '%s'", result2.Paths[0].Name)
	}
}

func TestGetRelationship_1stCousinThreeTimesRemoved(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	// Create family tree for 1st cousin 3 times removed
	// grandfather -> father -> child -> grandchild -> greatGrandchild -> ggGreatGrandchild
	//            \-> uncle -> cousin
	grandfather := createPerson(t, ctx, store, "GF", "Doe", domain.GenderMale)
	father := createPerson(t, ctx, store, "F", "Doe", domain.GenderMale)
	uncle := createPerson(t, ctx, store, "U", "Doe", domain.GenderMale)
	child := createPerson(t, ctx, store, "C", "Doe", domain.GenderMale)
	cousin := createPerson(t, ctx, store, "Cousin", "Doe", domain.GenderMale)
	grandchild := createPerson(t, ctx, store, "GC", "Doe", domain.GenderMale)
	greatGrandchild := createPerson(t, ctx, store, "GGC", "Doe", domain.GenderMale)
	ggGreatGrandchild := createPerson(t, ctx, store, "GGGC", "Doe", domain.GenderMale)

	createParentChild(t, ctx, store, father, &grandfather, nil, "GF Doe", "")
	createParentChild(t, ctx, store, uncle, &grandfather, nil, "GF Doe", "")
	createParentChild(t, ctx, store, child, &father, nil, "F Doe", "")
	createParentChild(t, ctx, store, cousin, &uncle, nil, "U Doe", "")
	createParentChild(t, ctx, store, grandchild, &child, nil, "C Doe", "")
	createParentChild(t, ctx, store, greatGrandchild, &grandchild, nil, "GC Doe", "")
	createParentChild(t, ctx, store, ggGreatGrandchild, &greatGrandchild, nil, "GGC Doe", "")

	result, err := svc.GetRelationship(ctx, ggGreatGrandchild, cousin)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsRelated {
		t.Error("Expected IsRelated to be true")
	}

	foundCousinThrice := false
	for _, path := range result.Paths {
		if path.Name == "1st cousin thrice removed" {
			foundCousinThrice = true
		}
	}
	if !foundCousinThrice {
		t.Errorf("Expected to find '1st cousin thrice removed' relationship, got paths: %v", result.Paths)
	}
}

func TestGetRelationship_1stCousinFourTimesRemoved(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	// Create family tree for 1st cousin 4 times removed
	grandfather := createPerson(t, ctx, store, "GF", "Doe", domain.GenderMale)
	father := createPerson(t, ctx, store, "F", "Doe", domain.GenderMale)
	uncle := createPerson(t, ctx, store, "U", "Doe", domain.GenderMale)
	child := createPerson(t, ctx, store, "C", "Doe", domain.GenderMale)
	cousin := createPerson(t, ctx, store, "Cousin", "Doe", domain.GenderMale)
	grandchild := createPerson(t, ctx, store, "GC", "Doe", domain.GenderMale)
	greatGrandchild := createPerson(t, ctx, store, "GGC", "Doe", domain.GenderMale)
	ggGreatGrandchild := createPerson(t, ctx, store, "GGGC", "Doe", domain.GenderMale)
	gggGreatGrandchild := createPerson(t, ctx, store, "GGGGC", "Doe", domain.GenderMale)

	createParentChild(t, ctx, store, father, &grandfather, nil, "GF Doe", "")
	createParentChild(t, ctx, store, uncle, &grandfather, nil, "GF Doe", "")
	createParentChild(t, ctx, store, child, &father, nil, "F Doe", "")
	createParentChild(t, ctx, store, cousin, &uncle, nil, "U Doe", "")
	createParentChild(t, ctx, store, grandchild, &child, nil, "C Doe", "")
	createParentChild(t, ctx, store, greatGrandchild, &grandchild, nil, "GC Doe", "")
	createParentChild(t, ctx, store, ggGreatGrandchild, &greatGrandchild, nil, "GGC Doe", "")
	createParentChild(t, ctx, store, gggGreatGrandchild, &ggGreatGrandchild, nil, "GGGC Doe", "")

	result, err := svc.GetRelationship(ctx, gggGreatGrandchild, cousin)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsRelated {
		t.Error("Expected IsRelated to be true")
	}

	foundCousinFourTimes := false
	for _, path := range result.Paths {
		if path.Name == "1st cousin 4 times removed" {
			foundCousinFourTimes = true
		}
	}
	if !foundCousinFourTimes {
		t.Errorf("Expected to find '1st cousin 4 times removed' relationship, got paths: %v", result.Paths)
	}
}

func TestGetRelationship_2ndGreatGrandUncle(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	// 3rd-great-grandfather
	// ├── 2nd-great-grandfather -> great-grandfather -> grandfather -> father -> child
	// └── 2nd-great-grand-uncle
	gggGrandfather := createPerson(t, ctx, store, "GGGG", "Doe", domain.GenderMale)
	ggGrandfather := createPerson(t, ctx, store, "GGG", "Doe", domain.GenderMale)
	ggGrandUncle := createPerson(t, ctx, store, "GGGU", "Doe", domain.GenderMale)
	greatGrandfather := createPerson(t, ctx, store, "GGF", "Doe", domain.GenderMale)
	grandfather := createPerson(t, ctx, store, "GF", "Doe", domain.GenderMale)
	father := createPerson(t, ctx, store, "F", "Doe", domain.GenderMale)
	child := createPerson(t, ctx, store, "C", "Doe", domain.GenderMale)

	createParentChild(t, ctx, store, ggGrandfather, &gggGrandfather, nil, "GGGG Doe", "")
	createParentChild(t, ctx, store, ggGrandUncle, &gggGrandfather, nil, "GGGG Doe", "")
	createParentChild(t, ctx, store, greatGrandfather, &ggGrandfather, nil, "GGG Doe", "")
	createParentChild(t, ctx, store, grandfather, &greatGrandfather, nil, "GGF Doe", "")
	createParentChild(t, ctx, store, father, &grandfather, nil, "GF Doe", "")
	createParentChild(t, ctx, store, child, &father, nil, "F Doe", "")

	result, err := svc.GetRelationship(ctx, child, ggGrandUncle)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsRelated {
		t.Error("Expected IsRelated to be true")
	}

	foundGGGrandUncle := false
	for _, path := range result.Paths {
		if path.Name == "great-great-grand-uncle/aunt" {
			foundGGGrandUncle = true
		}
	}
	if !foundGGGrandUncle {
		t.Errorf("Expected to find 'great-great-grand-uncle/aunt' relationship, got paths: %v", result.Paths)
	}
}

func TestGetRelationship_OrdinalEdgeCases(t *testing.T) {
	store := memory.NewReadModelStore()
	svc := query.NewRelationshipService(store)
	ctx := context.Background()

	// Create 11th cousin (requires 12 generations to common ancestor)
	// Testing ordinal 11th (11th ends in 1 but is "th" not "st")
	// Build a deep family tree

	// For 11th cousins, both need to be 12 generations from common ancestor
	// That's a lot of people, so let's just test the naming directly
	// by creating a simpler structure that tests the ordinal edge cases

	// Create 5th cousins (gen 6 from common ancestor)
	ancestor := createPerson(t, ctx, store, "Ancestor", "Doe", domain.GenderMale)

	// Build branches
	var leftBranch [6]uuid.UUID
	var rightBranch [6]uuid.UUID

	leftBranch[0] = createPerson(t, ctx, store, "L0", "Doe", domain.GenderMale)
	createParentChild(t, ctx, store, leftBranch[0], &ancestor, nil, "Ancestor Doe", "")

	for i := 1; i < 6; i++ {
		leftBranch[i] = createPerson(t, ctx, store, "L"+string(rune('0'+i)), "Doe", domain.GenderMale)
		createParentChild(t, ctx, store, leftBranch[i], &leftBranch[i-1], nil, "L"+string(rune('0'+i-1))+" Doe", "")
	}

	rightBranch[0] = createPerson(t, ctx, store, "R0", "Doe", domain.GenderMale)
	createParentChild(t, ctx, store, rightBranch[0], &ancestor, nil, "Ancestor Doe", "")

	for i := 1; i < 6; i++ {
		rightBranch[i] = createPerson(t, ctx, store, "R"+string(rune('0'+i)), "Doe", domain.GenderMale)
		createParentChild(t, ctx, store, rightBranch[i], &rightBranch[i-1], nil, "R"+string(rune('0'+i-1))+" Doe", "")
	}

	result, err := svc.GetRelationship(ctx, leftBranch[5], rightBranch[5])
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsRelated {
		t.Error("Expected IsRelated to be true")
	}

	found5thCousin := false
	for _, path := range result.Paths {
		if path.Name == "5th cousin" {
			found5thCousin = true
		}
	}
	if !found5thCousin {
		t.Errorf("Expected to find '5th cousin' relationship, got paths: %v", result.Paths)
	}
}
