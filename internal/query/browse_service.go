package query

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// BrowseService provides queries for browsing surnames and places.
//
// Its readStore is the query.PrivacyStore (PV-001), which replaces the
// privacy each read is given with the principal's, so the aggregates below
// pass repository.Privacy{}. Over a raw store they would see everyone.
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
	Breadcrumb []PlaceEntry `json:"breadcrumb,omitempty"`
}

// PlaceEntry is a place in the place browser (ADR-009 §4): its ID, the name
// it shows at its level, its full name, its count and whether places sit
// under it.
type PlaceEntry struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	FullName    string    `json:"full_name"`
	Count       int       `json:"count"`
	HasChildren bool      `json:"has_children"`
}

// GetSurnameIndexInput contains the input for GetSurnameIndex.
type GetSurnameIndexInput struct {
	Letter   string          // Optional: filter by starting letter
	BranchID domain.BranchID // Branch to read; domain.MainBranchID for the mainline
}

// GetSurnameIndex returns the surname index with optional letter filtering.
func (s *BrowseService) GetSurnameIndex(ctx context.Context, input GetSurnameIndexInput) (*SurnameIndexResult, error) {
	if input.Letter != "" {
		// Get surnames for specific letter
		entries, err := s.readStore.GetSurnamesByLetter(ctx, input.BranchID, input.Letter, repository.Privacy{})
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
	entries, letterCounts, err := s.readStore.GetSurnameIndex(ctx, input.BranchID, repository.Privacy{})
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
	Surname  string
	Limit    int
	Offset   int
	BranchID domain.BranchID // Branch to read; domain.MainBranchID for the mainline
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
		Limit:    limit,
		Offset:   offset,
		BranchID: input.BranchID,
	}

	persons, total, err := s.readStore.GetPersonsBySurname(ctx, input.Surname, opts)
	if err != nil {
		return nil, err
	}

	items := make([]Person, len(persons))
	for i, p := range persons {
		items[i] = convertReadModelToPerson(p, livingCutoff(ctx))
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
	Parent   uuid.UUID       // uuid.Nil for the top-level places
	BranchID domain.BranchID // Branch to read; domain.MainBranchID for the mainline
}

// GetPlaceHierarchy returns the places directly under a place in the place
// hierarchy, and the breadcrumb: the path from the top level down to that
// place.
func (s *BrowseService) GetPlaceHierarchy(ctx context.Context, input GetPlaceHierarchyInput) (*PlaceIndexResult, error) {
	level, err := s.readStore.GetPlaceHierarchy(ctx, input.BranchID, input.Parent, repository.Privacy{})
	if err != nil {
		return nil, err
	}
	return &PlaceIndexResult{
		Items:      convertPlaceEntries(level.Places),
		Total:      len(level.Places),
		Breadcrumb: convertPlaceEntries(level.Path),
	}, nil
}

// convertPlaceEntries converts the store's place browser entries.
func convertPlaceEntries(entries []repository.PlaceEntry) []PlaceEntry {
	items := make([]PlaceEntry, len(entries))
	for i, e := range entries {
		items[i] = PlaceEntry{ID: e.ID, Name: e.Name, FullName: e.FullName, Count: e.Count, HasChildren: e.HasChildren}
	}
	return items
}

// GetPersonsByPlaceInput contains the input for GetPersonsByPlace.
type GetPersonsByPlaceInput struct {
	Place    uuid.UUID // the place's ID; an absorbed place's ID names the place it resolves to
	Limit    int
	Offset   int
	BranchID domain.BranchID // Branch to read; domain.MainBranchID for the mainline
}

// GetPersonsByPlace returns the people whose birth or death place is the place
// or a place beneath it in the place hierarchy: the people of the records
// that the place's count in GetPlaceHierarchy counts.
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
		Limit:    limit,
		Offset:   offset,
		BranchID: input.BranchID,
	}

	persons, total, err := s.readStore.GetPersonsByPlace(ctx, input.Place, opts)
	if err != nil {
		return nil, err
	}

	items := make([]Person, len(persons))
	for i, p := range persons {
		items[i] = convertReadModelToPerson(p, livingCutoff(ctx))
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

// GetCemeteryIndex returns the cemetery/burial place index within the branch
// overlay (ADR-005); the zero branchID (MainBranchID) reads the mainline.
func (s *BrowseService) GetCemeteryIndex(ctx context.Context, branchID domain.BranchID) (*CemeteryIndexResult, error) {
	entries, err := s.readStore.GetCemeteryIndex(ctx, branchID, repository.Privacy{})
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
	Place    string
	Limit    int
	Offset   int
	BranchID domain.BranchID // Branch to read; domain.MainBranchID for the mainline
}

// GetPersonsByCemetery returns persons with burial/cremation events at the given place.
//
// Both sides of the join follow input.BranchID's overlay (ADR-005): the burial and
// cremation life events select the owners, and the persons returned are the
// branch's view of them (#757), so the list agrees with GetCemeteryIndex.
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
		Limit:    limit,
		Offset:   offset,
		BranchID: input.BranchID,
	}

	persons, total, err := s.readStore.GetPersonsByCemetery(ctx, input.Place, opts)
	if err != nil {
		return nil, err
	}

	items := make([]Person, len(persons))
	for i, p := range persons {
		items[i] = convertReadModelToPerson(p, livingCutoff(ctx))
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
	// PlaceID is the ID of the place it groups (ADR-009 §9).
	PlaceID   uuid.UUID   `json:"place_id"`
	Place     string      `json:"place"`
	Latitude  float64     `json:"latitude"`
	Longitude float64     `json:"longitude"`
	EventType string      `json:"event_type"`
	Count     int         `json:"count"`
	Persons   []MapPerson `json:"persons"`
	// Origins are the distinct sources of the pairs it groups (#1563).
	Origins []domain.CoordinateSource `json:"origins"`
}

// MapPerson is a person pinned at a MapLocation.
type MapPerson struct {
	ID        uuid.UUID `json:"id"`
	GivenName string    `json:"given_name"`
	Surname   string    `json:"surname"`
	Living    bool      `json:"living"`            // living rule verdict today (ADR-008 §1)
	Private   bool      `json:"private,omitempty"` // record marked private (ADR-008 §2)
}

// GetMapLocations returns aggregated geographic locations for map visualization.
// branchID selects the branch to read; domain.MainBranchID is the mainline.
func (s *BrowseService) GetMapLocations(ctx context.Context, branchID domain.BranchID) (*MapLocationsResult, error) {
	locations, err := s.readStore.GetMapLocations(ctx, branchID, repository.Privacy{})
	if err != nil {
		return nil, err
	}

	cutoff := livingCutoff(ctx)
	items := make([]MapLocation, len(locations))
	for i, loc := range locations {
		persons := make([]MapPerson, len(loc.Persons))
		for j, p := range loc.Persons {
			persons[j] = MapPerson{
				ID: p.ID, GivenName: p.GivenName, Surname: p.Surname,
				Living: p.LivingFacts.IsLiving(cutoff), Private: p.Restriction.IsPrivate(),
			}
		}
		items[i] = MapLocation{
			PlaceID:   loc.PlaceID,
			Place:     loc.Place,
			Latitude:  loc.Latitude,
			Longitude: loc.Longitude,
			EventType: loc.EventType,
			Count:     loc.Count,
			Persons:   persons,
			Origins:   loc.Origins,
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
//
// Main-only: brick walls are event-sourced (#802) but only main writes them, so
// they have no branch overlay yet (branch-scoping them is follow-up under #676).
func (s *BrowseService) GetBrickWalls(ctx context.Context, includeResolved bool) (*BrickWallsResult, error) {
	// Always fetch all entries so counts are accurate regardless of filter
	entries, err := s.readStore.GetBrickWalls(ctx, true, repository.Privacy{})
	if err != nil {
		return nil, err
	}

	activeCount := 0
	resolvedCount := 0
	var filtered []repository.BrickWallEntry
	for _, e := range entries {
		if e.ResolvedAt != nil {
			resolvedCount++
		} else {
			activeCount++
		}
		if includeResolved || e.ResolvedAt == nil {
			filtered = append(filtered, e)
		}
	}

	items := make([]BrickWallItem, len(filtered))
	for i, e := range filtered {
		items[i] = BrickWallItem{
			PersonID:   e.PersonID,
			PersonName: e.PersonName,
			Note:       e.Note,
			Since:      e.Since,
			ResolvedAt: e.ResolvedAt,
		}
	}

	return &BrickWallsResult{
		Items:         items,
		ActiveCount:   activeCount,
		ResolvedCount: resolvedCount,
	}, nil
}

// ListPlaces returns one page of the places in use within the branch overlay
// (ADR-009 §2, §6), sorted by name, and the number of places in all. An
// absorbed place is not an entry of its own: it is listed among its target's
// aliases (ADR-009 §3).
func (s *BrowseService) ListPlaces(ctx context.Context, branchID domain.BranchID, limit, offset int) ([]repository.PlaceReadModel, int, error) {
	places, err := s.readStore.ListPlaces(ctx, branchID, repository.Privacy{})
	if err != nil {
		return nil, 0, err
	}
	places = slices.DeleteFunc(places, func(p repository.PlaceReadModel) bool { return p.ResolvesTo != nil })
	return repository.Page(places, offset, limit), len(places), nil
}

// ListUnmappedPlaces returns the no-coordinates worklist within the branch
// overlay (ADR-009 §9): every place with a record that has no effective
// coordinates, with the pairs other records there carry as suggestions.
func (s *BrowseService) ListUnmappedPlaces(ctx context.Context, branchID domain.BranchID) ([]repository.UnmappedPlace, error) {
	return s.readStore.ListUnmappedPlaces(ctx, branchID, repository.Privacy{})
}

// GetPlace returns the place in use with ID id within the branch overlay, or
// nil when there is none. It reads ListPlaces, which holds every place the
// place browser shows. A place no visible record resolves to or sits beneath
// is not in use for the principal, so it is nil exactly as an ID never in use
// is (ADR-009 §10).
// An absorbed place is returned as itself, with its own facts and the ID it
// resolves to, not as its target (ADR-009 §3).
func (s *BrowseService) GetPlace(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.PlaceReadModel, error) {
	places, err := s.readStore.ListPlaces(ctx, branchID, repository.Privacy{})
	if err != nil {
		return nil, err
	}
	for i := range places {
		if places[i].ID == id {
			return &places[i], nil
		}
	}
	return nil, nil
}
