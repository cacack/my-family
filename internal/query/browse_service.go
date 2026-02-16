package query

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

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

// CemeteryIndexResult contains the cemetery index response.
type CemeteryIndexResult struct {
	Items []CemeteryEntry `json:"items"`
	Total int             `json:"total"`
}

// CemeteryEntry represents a burial/cremation place with person count.
type CemeteryEntry struct {
	Place string `json:"place"`
	Count int    `json:"count"`
}

// GetCemeteryIndex returns the cemetery/burial place index.
func (s *BrowseService) GetCemeteryIndex(ctx context.Context) (*CemeteryIndexResult, error) {
	entries, err := s.readStore.GetCemeteryIndex(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]CemeteryEntry, len(entries))
	for i, e := range entries {
		items[i] = CemeteryEntry{
			Place: e.Place,
			Count: e.Count,
		}
	}

	return &CemeteryIndexResult{
		Items: items,
		Total: len(items),
	}, nil
}

// GetPersonsByCemeteryInput contains the input for GetPersonsByCemetery.
type GetPersonsByCemeteryInput struct {
	Place  string
	Limit  int
	Offset int
}

// GetPersonsByCemetery returns persons with burial/cremation events at the given place.
func (s *BrowseService) GetPersonsByCemetery(ctx context.Context, input GetPersonsByCemeteryInput) (*PersonListResult, error) {
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

	persons, total, err := s.readStore.GetPersonsByCemetery(ctx, input.Place, opts)
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

// MapLocationsResult contains the map locations response.
type MapLocationsResult struct {
	Items []MapLocation `json:"items"`
	Total int           `json:"total"`
}

// MapLocation represents a geographic location for map display.
type MapLocation struct {
	Place     string   `json:"place"`
	Latitude  float64  `json:"latitude"`
	Longitude float64  `json:"longitude"`
	EventType string   `json:"event_type"`
	Count     int      `json:"count"`
	PersonIDs []string `json:"person_ids"`
}

// GetMapLocations returns aggregated geographic locations for map visualization.
func (s *BrowseService) GetMapLocations(ctx context.Context) (*MapLocationsResult, error) {
	locations, err := s.readStore.GetMapLocations(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]MapLocation, len(locations))
	for i, loc := range locations {
		personIDs := make([]string, len(loc.PersonIDs))
		for j, id := range loc.PersonIDs {
			personIDs[j] = id.String()
		}
		items[i] = MapLocation{
			Place:     loc.Place,
			Latitude:  loc.Latitude,
			Longitude: loc.Longitude,
			EventType: loc.EventType,
			Count:     loc.Count,
			PersonIDs: personIDs,
		}
	}

	return &MapLocationsResult{
		Items: items,
		Total: len(items),
	}, nil
}

// BrickWallsResult contains the brick wall list response.
type BrickWallsResult struct {
	Items         []BrickWallItem `json:"items"`
	ActiveCount   int             `json:"active_count"`
	ResolvedCount int             `json:"resolved_count"`
}

// BrickWallItem represents a person with a brick wall status.
type BrickWallItem struct {
	PersonID   uuid.UUID  `json:"person_id"`
	PersonName string     `json:"person_name"`
	Note       string     `json:"note"`
	Since      time.Time  `json:"since"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}

// GetBrickWalls returns brick wall entries.
func (s *BrowseService) GetBrickWalls(ctx context.Context, includeResolved bool) (*BrickWallsResult, error) {
	entries, err := s.readStore.GetBrickWalls(ctx, includeResolved)
	if err != nil {
		return nil, err
	}

	items := make([]BrickWallItem, len(entries))
	activeCount := 0
	resolvedCount := 0
	for i, e := range entries {
		items[i] = BrickWallItem{
			PersonID:   e.PersonID,
			PersonName: e.PersonName,
			Note:       e.Note,
			Since:      e.Since,
			ResolvedAt: e.ResolvedAt,
		}
		if e.ResolvedAt != nil {
			resolvedCount++
		} else {
			activeCount++
		}
	}

	return &BrickWallsResult{
		Items:         items,
		ActiveCount:   activeCount,
		ResolvedCount: resolvedCount,
	}, nil
}

// SetBrickWall marks a person as a brick wall with a note.
func (s *BrowseService) SetBrickWall(ctx context.Context, personID uuid.UUID, note string) error {
	return s.readStore.SetBrickWall(ctx, personID, note)
}

// ResolveBrickWall resolves a brick wall (marks as broken through).
func (s *BrowseService) ResolveBrickWall(ctx context.Context, personID uuid.UUID) error {
	return s.readStore.ResolveBrickWall(ctx, personID)
}
