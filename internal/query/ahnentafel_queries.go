package query

import (
	"context"
	"sort"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

// AhnentafelEntry represents a single entry in an Ahnentafel report.
// The Ahnentafel numbering system assigns each ancestor a unique number:
// - Subject = 1
// - Father of person N = 2N
// - Mother of person N = 2N + 1
type AhnentafelEntry struct {
	Number     int             `json:"number"`
	Generation int             `json:"generation"`
	ID         uuid.UUID       `json:"id"`
	GivenName  string          `json:"given_name"`
	Surname    string          `json:"surname"`
	Gender     string          `json:"gender,omitempty"`
	BirthDate  *domain.GenDate `json:"birth_date,omitempty"`
	BirthPlace *string         `json:"birth_place,omitempty"`
	DeathDate  *domain.GenDate `json:"death_date,omitempty"`
	DeathPlace *string         `json:"death_place,omitempty"`
}

// AhnentafelResult contains the complete Ahnentafel report for a person.
type AhnentafelResult struct {
	Entries       []AhnentafelEntry `json:"entries"`        // Sorted by Ahnentafel number
	TotalEntries  int               `json:"total_entries"`  // Number of entries (including subject)
	MaxGeneration int               `json:"max_generation"` // Highest generation reached
}

// AhnentafelService provides Ahnentafel query operations.
type AhnentafelService struct {
	pedigreeService *PedigreeService
}

// NewAhnentafelService creates a new Ahnentafel query service.
func NewAhnentafelService(pedigreeService *PedigreeService) *AhnentafelService {
	return &AhnentafelService{pedigreeService: pedigreeService}
}

// GetAhnentafelInput contains options for retrieving an Ahnentafel report.
type GetAhnentafelInput struct {
	PersonID       uuid.UUID
	MaxGenerations int // Maximum generations to include (default 5)
}

// GetAhnentafel returns the Ahnentafel (numbered ancestor list) for a person.
// Missing ancestors result in gaps in the numbering, which is standard and expected.
func (s *AhnentafelService) GetAhnentafel(ctx context.Context, input GetAhnentafelInput) (*AhnentafelResult, error) {
	// Get the pedigree tree from the pedigree service
	pedigreeResult, err := s.pedigreeService.GetPedigree(ctx, GetPedigreeInput(input))
	if err != nil {
		return nil, err
	}

	// Convert the tree to Ahnentafel entries
	var entries []AhnentafelEntry
	s.traverseTree(pedigreeResult.Root, 1, &entries)

	// Sort entries by Ahnentafel number
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Number < entries[j].Number
	})

	// Find max generation
	maxGen := 0
	for _, entry := range entries {
		if entry.Generation > maxGen {
			maxGen = entry.Generation
		}
	}

	return &AhnentafelResult{
		Entries:       entries,
		TotalEntries:  len(entries),
		MaxGeneration: maxGen,
	}, nil
}

// traverseTree recursively traverses the pedigree tree and collects Ahnentafel entries.
// The Ahnentafel number is calculated as:
// - Subject = 1
// - Father of person N = 2N
// - Mother of person N = 2N + 1
func (s *AhnentafelService) traverseTree(node *PedigreeNode, ahnentafelNum int, entries *[]AhnentafelEntry) {
	if node == nil {
		return
	}

	// Create entry for this person
	entry := AhnentafelEntry{
		Number:     ahnentafelNum,
		Generation: node.Generation,
		ID:         node.ID,
		GivenName:  node.GivenName,
		Surname:    node.Surname,
		Gender:     node.Gender,
		BirthDate:  node.BirthDate,
		BirthPlace: node.BirthPlace,
		DeathDate:  node.DeathDate,
		DeathPlace: node.DeathPlace,
	}
	*entries = append(*entries, entry)

	// Recursively process father (2N) and mother (2N + 1)
	s.traverseTree(node.Father, 2*ahnentafelNum, entries)
	s.traverseTree(node.Mother, 2*ahnentafelNum+1, entries)
}
