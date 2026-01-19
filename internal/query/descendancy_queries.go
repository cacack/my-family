package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// DescendancyService provides descendancy (descendant tree) queries.
type DescendancyService struct {
	readStore repository.ReadModelStore
}

// NewDescendancyService creates a new descendancy query service.
func NewDescendancyService(readStore repository.ReadModelStore) *DescendancyService {
	return &DescendancyService{readStore: readStore}
}

// SpouseInfo represents spouse information in a descendancy node.
type SpouseInfo struct {
	ID           uuid.UUID       `json:"id"`
	Name         string          `json:"name"`
	MarriageDate *domain.GenDate `json:"marriage_date,omitempty"`
}

// DescendancyNode represents a person in the descendancy tree.
type DescendancyNode struct {
	ID         uuid.UUID          `json:"id"`
	GivenName  string             `json:"given_name"`
	Surname    string             `json:"surname"`
	Gender     string             `json:"gender,omitempty"`
	BirthDate  *domain.GenDate    `json:"birth_date,omitempty"`
	DeathDate  *domain.GenDate    `json:"death_date,omitempty"`
	Spouses    []SpouseInfo       `json:"spouses,omitempty"`
	Children   []*DescendancyNode `json:"children,omitempty"`
	Generation int                `json:"generation"`
}

// DescendancyResult contains the descendancy tree for a person.
type DescendancyResult struct {
	Root             *DescendancyNode `json:"root"`
	TotalDescendants int              `json:"total_descendants"`
	MaxGeneration    int              `json:"max_generation"`
}

// GetDescendancyInput contains options for retrieving a descendancy.
type GetDescendancyInput struct {
	PersonID       uuid.UUID
	MaxGenerations int // Maximum generations to traverse (default 4)
}

// GetDescendancy returns the descendant tree for a person.
func (s *DescendancyService) GetDescendancy(ctx context.Context, input GetDescendancyInput) (*DescendancyResult, error) {
	// Set default max generations
	maxGen := input.MaxGenerations
	if maxGen <= 0 {
		maxGen = 4
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

	// Build descendancy tree recursively
	visited := make(map[uuid.UUID]bool)
	root := s.buildDescendancyNode(ctx, input.PersonID, 0, maxGen, visited)

	// Count total descendants and max generation
	totalDescendants := 0
	maxGenReached := 0
	countDescendants(root, &totalDescendants, &maxGenReached)

	return &DescendancyResult{
		Root:             root,
		TotalDescendants: totalDescendants,
		MaxGeneration:    maxGenReached,
	}, nil
}

// buildDescendancyNode recursively builds a descendancy node and its descendants.
func (s *DescendancyService) buildDescendancyNode(ctx context.Context, personID uuid.UUID, generation, maxGen int, visited map[uuid.UUID]bool) *DescendancyNode {
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

	node := &DescendancyNode{
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
	if person.DeathDateRaw != "" {
		dd := domain.ParseGenDate(person.DeathDateRaw)
		node.DeathDate = &dd
	}

	// Get families where this person is a partner
	families, err := s.readStore.GetFamiliesForPerson(ctx, personID)
	if err != nil {
		return node
	}

	// Process each family to get spouses and children
	for _, family := range families {
		// Get spouse information
		spouse := s.getSpouseInfo(family, personID)
		if spouse != nil {
			node.Spouses = append(node.Spouses, *spouse)
		}

		// Don't recurse beyond max generations
		if generation >= maxGen {
			continue
		}

		// Get children and recursively process them
		children, err := s.readStore.GetFamilyChildren(ctx, family.ID)
		if err != nil {
			continue
		}

		for _, child := range children {
			childNode := s.buildDescendancyNode(ctx, child.PersonID, generation+1, maxGen, visited)
			if childNode != nil {
				node.Children = append(node.Children, childNode)
			}
		}
	}

	return node
}

// getSpouseInfo extracts spouse information from a family record.
func (s *DescendancyService) getSpouseInfo(family repository.FamilyReadModel, personID uuid.UUID) *SpouseInfo {
	var spouseID *uuid.UUID
	var spouseName string

	// Find the other partner
	if family.Partner1ID != nil && *family.Partner1ID != personID {
		spouseID = family.Partner1ID
		spouseName = family.Partner1Name
	} else if family.Partner2ID != nil && *family.Partner2ID != personID {
		spouseID = family.Partner2ID
		spouseName = family.Partner2Name
	}

	if spouseID == nil {
		return nil
	}

	info := &SpouseInfo{
		ID:   *spouseID,
		Name: spouseName,
	}

	// Add marriage date if available
	if family.MarriageDateRaw != "" {
		md := domain.ParseGenDate(family.MarriageDateRaw)
		info.MarriageDate = &md
	}

	return info
}

// countDescendants counts total descendants and finds max generation in the tree.
func countDescendants(node *DescendancyNode, total *int, maxGen *int) {
	if node == nil {
		return
	}

	if node.Generation > *maxGen {
		*maxGen = node.Generation
	}

	for _, child := range node.Children {
		*total++
		countDescendants(child, total, maxGen)
	}
}
