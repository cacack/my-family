package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// PedigreeService provides pedigree (ancestor tree) queries.
type PedigreeService struct {
	readStore repository.ReadModelStore
}

// NewPedigreeService creates a new pedigree query service.
func NewPedigreeService(readStore repository.ReadModelStore) *PedigreeService {
	return &PedigreeService{readStore: readStore}
}

// PedigreeNode represents a person in the pedigree tree.
type PedigreeNode struct {
	ID         uuid.UUID       `json:"id"`
	GivenName  string          `json:"given_name"`
	Surname    string          `json:"surname"`
	Gender     string          `json:"gender,omitempty"`
	BirthDate  *domain.GenDate `json:"birth_date,omitempty"`
	BirthPlace *string         `json:"birth_place,omitempty"`
	DeathDate  *domain.GenDate `json:"death_date,omitempty"`
	DeathPlace *string         `json:"death_place,omitempty"`
	Generation int             `json:"generation"`
	Father     *PedigreeNode   `json:"father,omitempty"`
	Mother     *PedigreeNode   `json:"mother,omitempty"`
}

// PedigreeResult contains the pedigree tree for a person.
type PedigreeResult struct {
	Root           *PedigreeNode `json:"root"`
	TotalAncestors int           `json:"total_ancestors"`
	MaxGeneration  int           `json:"max_generation"`
}

// GetPedigreeInput contains options for retrieving a pedigree.
type GetPedigreeInput struct {
	PersonID       uuid.UUID
	MaxGenerations int // Maximum generations to traverse (default 5)
}

// GetPedigree returns the ancestor tree for a person.
func (s *PedigreeService) GetPedigree(ctx context.Context, input GetPedigreeInput) (*PedigreeResult, error) {
	// Set default max generations
	maxGen := input.MaxGenerations
	if maxGen <= 0 {
		maxGen = 5
	}
	if maxGen > 10 {
		maxGen = 10 // Hard limit to prevent excessive recursion
	}

	// Get the root person
	person, err := s.readStore.GetPerson(ctx, input.PersonID)
	if err != nil {
		return nil, err
	}
	if person == nil {
		return nil, ErrNotFound
	}

	// Build pedigree tree recursively
	visited := make(map[uuid.UUID]bool)
	root := s.buildNode(ctx, input.PersonID, 0, maxGen, visited)

	// Count total ancestors and max generation
	totalAncestors := 0
	maxGenReached := 0
	countAncestors(root, &totalAncestors, &maxGenReached)

	return &PedigreeResult{
		Root:           root,
		TotalAncestors: totalAncestors,
		MaxGeneration:  maxGenReached,
	}, nil
}

// buildNode recursively builds a pedigree node and its ancestors.
func (s *PedigreeService) buildNode(ctx context.Context, personID uuid.UUID, generation, maxGen int, visited map[uuid.UUID]bool) *PedigreeNode {
	// Check if we've already visited this person (cycle detection)
	if visited[personID] {
		return nil
	}
	visited[personID] = true

	// Get person data
	person, err := s.readStore.GetPerson(ctx, personID)
	if err != nil || person == nil {
		return nil
	}

	node := &PedigreeNode{
		ID:         person.ID,
		GivenName:  person.GivenName,
		Surname:    person.Surname,
		Generation: generation,
	}

	// Set optional fields
	if person.Gender != "" {
		node.Gender = string(person.Gender)
	}
	if person.BirthDateRaw != "" {
		bd := domain.ParseGenDate(person.BirthDateRaw)
		node.BirthDate = &bd
	}
	if person.BirthPlace != "" {
		node.BirthPlace = &person.BirthPlace
	}
	if person.DeathDateRaw != "" {
		dd := domain.ParseGenDate(person.DeathDateRaw)
		node.DeathDate = &dd
	}
	if person.DeathPlace != "" {
		node.DeathPlace = &person.DeathPlace
	}

	// Don't recurse beyond max generations
	if generation >= maxGen {
		return node
	}

	// Get pedigree edge to find parents
	edge, err := s.readStore.GetPedigreeEdge(ctx, personID)
	if err != nil || edge == nil {
		return node
	}

	// Recursively build father's ancestors
	if edge.FatherID != nil {
		node.Father = s.buildNode(ctx, *edge.FatherID, generation+1, maxGen, visited)
	}

	// Recursively build mother's ancestors
	if edge.MotherID != nil {
		node.Mother = s.buildNode(ctx, *edge.MotherID, generation+1, maxGen, visited)
	}

	return node
}

// countAncestors counts total ancestors and finds max generation in the tree.
func countAncestors(node *PedigreeNode, total *int, maxGen *int) {
	if node == nil {
		return
	}

	if node.Generation > *maxGen {
		*maxGen = node.Generation
	}

	if node.Father != nil {
		*total++
		countAncestors(node.Father, total, maxGen)
	}
	if node.Mother != nil {
		*total++
		countAncestors(node.Mother, total, maxGen)
	}
}

// GetAncestors returns a flat list of ancestors up to a certain generation.
// This is useful for simpler queries that don't need the tree structure.
func (s *PedigreeService) GetAncestors(ctx context.Context, personID uuid.UUID, maxGen int) ([]Person, error) {
	if maxGen <= 0 {
		maxGen = 5
	}
	if maxGen > 10 {
		maxGen = 10
	}

	var ancestors []Person
	visited := make(map[uuid.UUID]bool)

	s.collectAncestors(ctx, personID, 0, maxGen, visited, &ancestors)

	return ancestors, nil
}

// collectAncestors recursively collects ancestors into a flat list.
func (s *PedigreeService) collectAncestors(ctx context.Context, personID uuid.UUID, generation, maxGen int, visited map[uuid.UUID]bool, ancestors *[]Person) {
	if generation >= maxGen {
		return
	}
	if visited[personID] {
		return
	}
	visited[personID] = true

	edge, err := s.readStore.GetPedigreeEdge(ctx, personID)
	if err != nil || edge == nil {
		return
	}

	// Add father if exists
	if edge.FatherID != nil {
		father, err := s.readStore.GetPerson(ctx, *edge.FatherID)
		if err == nil && father != nil {
			*ancestors = append(*ancestors, convertReadModelToPerson(*father))
			s.collectAncestors(ctx, *edge.FatherID, generation+1, maxGen, visited, ancestors)
		}
	}

	// Add mother if exists
	if edge.MotherID != nil {
		mother, err := s.readStore.GetPerson(ctx, *edge.MotherID)
		if err == nil && mother != nil {
			*ancestors = append(*ancestors, convertReadModelToPerson(*mother))
			s.collectAncestors(ctx, *edge.MotherID, generation+1, maxGen, visited, ancestors)
		}
	}
}
