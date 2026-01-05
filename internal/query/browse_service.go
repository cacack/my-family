package query

import (
	"context"
	"strings"

	"github.com/cacack/my-family/internal/repository"
)

// BrowseService provides queries for browsing surnames and places.
type BrowseService struct {
	readStore repository.ReadModelStore
}

// NewBrowseService creates a new BrowseService.
func NewBrowseService(readStore repository.ReadModelStore) *BrowseService {
	return &BrowseService{readStore: readStore}
}

// SurnameIndexResult contains the surname index response.
type SurnameIndexResult struct {
	Items        []SurnameEntry `json:"items"`
	Total        int            `json:"total"`
	LetterCounts []LetterCount  `json:"letter_counts,omitempty"`
}

// SurnameEntry represents a surname with count.
type SurnameEntry struct {
	Surname string `json:"surname"`
	Count   int    `json:"count"`
}

// LetterCount represents count of surnames by starting letter.
type LetterCount struct {
	Letter string `json:"letter"`
	Count  int    `json:"count"`
}

// PlaceIndexResult contains the place index response.
type PlaceIndexResult struct {
	Items      []PlaceEntry `json:"items"`
	Total      int          `json:"total"`
	Breadcrumb []string     `json:"breadcrumb,omitempty"`
}

// PlaceEntry represents a place with count and hierarchy info.
type PlaceEntry struct {
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Count       int    `json:"count"`
	HasChildren bool   `json:"has_children"`
}

// GetSurnameIndexInput contains the input for GetSurnameIndex.
type GetSurnameIndexInput struct {
	Letter string // Optional: filter by starting letter
}

// GetSurnameIndex returns the surname index with optional letter filtering.
func (s *BrowseService) GetSurnameIndex(ctx context.Context, input GetSurnameIndexInput) (*SurnameIndexResult, error) {
	if input.Letter != "" {
		// Get surnames for specific letter
		entries, err := s.readStore.GetSurnamesByLetter(ctx, input.Letter)
		if err != nil {
			return nil, err
		}

		items := make([]SurnameEntry, len(entries))
		for i, e := range entries {
			items[i] = SurnameEntry{
				Surname: e.Surname,
				Count:   e.Count,
			}
		}

		return &SurnameIndexResult{
			Items: items,
			Total: len(items),
		}, nil
	}

	// Get full index
	entries, letterCounts, err := s.readStore.GetSurnameIndex(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]SurnameEntry, len(entries))
	for i, e := range entries {
		items[i] = SurnameEntry{
			Surname: e.Surname,
			Count:   e.Count,
		}
	}

	letters := make([]LetterCount, len(letterCounts))
	for i, lc := range letterCounts {
		letters[i] = LetterCount{
			Letter: lc.Letter,
			Count:  lc.Count,
		}
	}

	return &SurnameIndexResult{
		Items:        items,
		Total:        len(items),
		LetterCounts: letters,
	}, nil
}

// GetPersonsBySurnameInput contains the input for GetPersonsBySurname.
type GetPersonsBySurnameInput struct {
	Surname string
	Limit   int
	Offset  int
}

// GetPersonsBySurname returns persons with a specific surname.
func (s *BrowseService) GetPersonsBySurname(ctx context.Context, input GetPersonsBySurnameInput) (*PersonListResult, error) {
	// Apply defaults
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}

	offset := input.Offset
	if offset < 0 {
		offset = 0
	}

	opts := repository.ListOptions{
		Limit:  limit,
		Offset: offset,
	}

	persons, total, err := s.readStore.GetPersonsBySurname(ctx, input.Surname, opts)
	if err != nil {
		return nil, err
	}

	items := make([]Person, len(persons))
	for i, p := range persons {
		items[i] = convertReadModelToPerson(p)
	}

	return &PersonListResult{
		Items:  items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

// GetPlaceHierarchyInput contains the input for GetPlaceHierarchy.
type GetPlaceHierarchyInput struct {
	Parent string // Empty for top-level places
}

// GetPlaceHierarchy returns places at a given level in the hierarchy.
func (s *BrowseService) GetPlaceHierarchy(ctx context.Context, input GetPlaceHierarchyInput) (*PlaceIndexResult, error) {
	entries, err := s.readStore.GetPlaceHierarchy(ctx, input.Parent)
	if err != nil {
		return nil, err
	}

	items := make([]PlaceEntry, len(entries))
	for i, e := range entries {
		items[i] = PlaceEntry{
			Name:        e.Name,
			FullName:    e.FullName,
			Count:       e.Count,
			HasChildren: e.HasChildren,
		}
	}

	// Build breadcrumb from parent
	var breadcrumb []string
	if input.Parent != "" {
		// Parse parent path (comma-separated, rightmost is most general)
		parts := strings.Split(input.Parent, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			breadcrumb = append(breadcrumb, strings.TrimSpace(parts[i]))
		}
	}

	return &PlaceIndexResult{
		Items:      items,
		Total:      len(items),
		Breadcrumb: breadcrumb,
	}, nil
}

// GetPersonsByPlaceInput contains the input for GetPersonsByPlace.
type GetPersonsByPlaceInput struct {
	Place  string
	Limit  int
	Offset int
}

// GetPersonsByPlace returns persons associated with a place.
func (s *BrowseService) GetPersonsByPlace(ctx context.Context, input GetPersonsByPlaceInput) (*PersonListResult, error) {
	// Apply defaults
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}

	offset := input.Offset
	if offset < 0 {
		offset = 0
	}

	opts := repository.ListOptions{
		Limit:  limit,
		Offset: offset,
	}

	persons, total, err := s.readStore.GetPersonsByPlace(ctx, input.Place, opts)
	if err != nil {
		return nil, err
	}

	items := make([]Person, len(persons))
	for i, p := range persons {
		items[i] = convertReadModelToPerson(p)
	}

	return &PersonListResult{
		Items:  items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}
