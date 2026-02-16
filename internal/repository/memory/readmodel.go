package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
)

// ReadModelStore is an in-memory implementation of repository.ReadModelStore for testing.
type ReadModelStore struct {
	mu             sync.RWMutex
	persons        map[uuid.UUID]*repository.PersonReadModel
	personNames    map[uuid.UUID][]repository.PersonNameReadModel // keyed by person ID
	families       map[uuid.UUID]*repository.FamilyReadModel
	familyChildren map[uuid.UUID][]repository.FamilyChildReadModel // keyed by family ID
	pedigreeEdges  map[uuid.UUID]*repository.PedigreeEdge          // keyed by person ID
	sources        map[uuid.UUID]*repository.SourceReadModel
	citations      map[uuid.UUID]*repository.CitationReadModel
	media          map[uuid.UUID]*repository.MediaReadModel
	events         map[uuid.UUID]*repository.EventReadModel
	attributes     map[uuid.UUID]*repository.AttributeReadModel
	notes          map[uuid.UUID]*repository.NoteReadModel
	submitters     map[uuid.UUID]*repository.SubmitterReadModel
	associations   map[uuid.UUID]*repository.AssociationReadModel
	ldsOrdinances  map[uuid.UUID]*repository.LDSOrdinanceReadModel
}

// NewReadModelStore creates a new in-memory read model store.
func NewReadModelStore() *ReadModelStore {
	return &ReadModelStore{
		persons:        make(map[uuid.UUID]*repository.PersonReadModel),
		personNames:    make(map[uuid.UUID][]repository.PersonNameReadModel),
		families:       make(map[uuid.UUID]*repository.FamilyReadModel),
		familyChildren: make(map[uuid.UUID][]repository.FamilyChildReadModel),
		pedigreeEdges:  make(map[uuid.UUID]*repository.PedigreeEdge),
		sources:        make(map[uuid.UUID]*repository.SourceReadModel),
		citations:      make(map[uuid.UUID]*repository.CitationReadModel),
		media:          make(map[uuid.UUID]*repository.MediaReadModel),
		events:         make(map[uuid.UUID]*repository.EventReadModel),
		attributes:     make(map[uuid.UUID]*repository.AttributeReadModel),
		notes:          make(map[uuid.UUID]*repository.NoteReadModel),
		submitters:     make(map[uuid.UUID]*repository.SubmitterReadModel),
		associations:   make(map[uuid.UUID]*repository.AssociationReadModel),
		ldsOrdinances:  make(map[uuid.UUID]*repository.LDSOrdinanceReadModel),
	}
}

// GetPerson retrieves a person by ID.
func (s *ReadModelStore) GetPerson(ctx context.Context, id uuid.UUID) (*repository.PersonReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, exists := s.persons[id]
	if !exists {
		return nil, nil
	}
	// Return a copy
	result := *p
	return &result, nil
}

// matchesResearchStatusFilter checks if a person matches the research status filter.
func matchesResearchStatusFilter(p *repository.PersonReadModel, filter *string) bool {
	if filter == nil {
		return true
	}
	if *filter == "unset" {
		return p.ResearchStatus == ""
	}
	return string(p.ResearchStatus) == *filter
}

// comparePersons compares two persons based on the sort field.
func comparePersons(a, b *repository.PersonReadModel, sortField string) int {
	switch sortField {
	case "given_name":
		return strings.Compare(a.GivenName, b.GivenName)
	case "birth_date":
		return compareBirthDates(a.BirthDateSort, b.BirthDateSort)
	case "updated_at":
		return compareTimestamps(a.UpdatedAt, b.UpdatedAt)
	default: // surname
		cmp := strings.Compare(a.Surname, b.Surname)
		if cmp == 0 {
			return strings.Compare(a.GivenName, b.GivenName)
		}
		return cmp
	}
}

// compareBirthDates compares two birth date pointers.
func compareBirthDates(a, b *time.Time) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return 1
	}
	if b == nil {
		return -1
	}
	return compareTimestamps(*a, *b)
}

// compareTimestamps compares two timestamps.
func compareTimestamps(a, b time.Time) int {
	if a.Before(b) {
		return -1
	}
	if a.After(b) {
		return 1
	}
	return 0
}

// ListPersons returns a paginated list of persons.
func (s *ReadModelStore) ListPersons(ctx context.Context, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Convert map to slice, applying research_status filter if present
	persons := make([]repository.PersonReadModel, 0, len(s.persons))
	for _, p := range s.persons {
		if matchesResearchStatusFilter(p, opts.ResearchStatus) {
			persons = append(persons, *p)
		}
	}

	// Sort
	sort.Slice(persons, func(i, j int) bool {
		cmp := comparePersons(&persons[i], &persons[j], opts.Sort)
		if opts.Order == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})

	total := len(persons)

	// Paginate
	start := opts.Offset
	if start > len(persons) {
		start = len(persons)
	}
	end := start + opts.Limit
	if end > len(persons) {
		end = len(persons)
	}

	return persons[start:end], total, nil
}

// SearchPersons searches for persons by name, including alternate names.
func (s *ReadModelStore) SearchPersons(ctx context.Context, opts repository.SearchOptions) ([]repository.PersonReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	hasQuery := strings.TrimSpace(opts.Query) != ""
	hasDateFilter := opts.BirthDateFrom != nil || opts.BirthDateTo != nil ||
		opts.DeathDateFrom != nil || opts.DeathDateTo != nil
	hasPlaceFilter := strings.TrimSpace(opts.BirthPlace) != "" || strings.TrimSpace(opts.DeathPlace) != ""
	if !hasQuery && !hasDateFilter && !hasPlaceFilter {
		return nil, nil
	}

	queryLower := strings.ToLower(opts.Query)
	foundIDs := make(map[uuid.UUID]bool)
	var results []repository.PersonReadModel

	// Search in main persons table
	for _, p := range s.persons {
		if !s.matchesSearchFilters(p, opts) {
			continue
		}
		if s.personMatchesQuery(p, queryLower, opts.Soundex) && !foundIDs[p.ID] {
			results = append(results, *p)
			foundIDs[p.ID] = true
		}
	}

	// Search in person_names table for alternate names (only if text query provided)
	if opts.Query != "" {
		s.searchAlternateNames(queryLower, opts, foundIDs, &results)
	}

	// Sort results to match postgres/sqlite behavior
	sortSearchResults(results, opts)

	// Apply limit after sorting
	if opts.Limit > 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	return results, nil
}

// personMatchesQuery checks if a person matches the text query (or returns true if no query).
func (s *ReadModelStore) personMatchesQuery(p *repository.PersonReadModel, queryLower string, soundex bool) bool {
	if queryLower == "" {
		return true
	}
	if strings.Contains(strings.ToLower(p.FullName), queryLower) ||
		strings.Contains(strings.ToLower(p.GivenName), queryLower) ||
		strings.Contains(strings.ToLower(p.Surname), queryLower) {
		return true
	}
	if soundex {
		for _, word := range strings.Fields(queryLower) {
			if repository.SoundexMatch(word, p.GivenName) || repository.SoundexMatch(word, p.Surname) {
				return true
			}
		}
	}
	return false
}

// searchAlternateNames searches person_names for alternate name matches.
func (s *ReadModelStore) searchAlternateNames(queryLower string, opts repository.SearchOptions, foundIDs map[uuid.UUID]bool, results *[]repository.PersonReadModel) {
	for personID, names := range s.personNames {
		if len(*results) >= opts.Limit {
			break
		}
		if foundIDs[personID] {
			continue
		}
		for _, name := range names {
			if altNameMatches(name, queryLower, opts.Soundex) {
				if p, exists := s.persons[personID]; exists && !foundIDs[personID] && s.matchesSearchFilters(p, opts) {
					*results = append(*results, *p)
					foundIDs[personID] = true
				}
				break
			}
		}
	}
}

// altNameMatches checks if a PersonNameReadModel matches via substring or Soundex.
func altNameMatches(name repository.PersonNameReadModel, queryLower string, soundex bool) bool {
	if nameMatchesQuery(name, queryLower) {
		return true
	}
	if soundex {
		for _, word := range strings.Fields(queryLower) {
			if repository.SoundexMatch(word, name.GivenName) ||
				repository.SoundexMatch(word, name.Surname) ||
				repository.SoundexMatch(word, name.Nickname) {
				return true
			}
		}
	}
	return false
}

// sortSearchResults sorts results by the requested field and direction.
func sortSearchResults(results []repository.PersonReadModel, opts repository.SearchOptions) {
	if opts.Sort == "" || opts.Sort == "relevance" {
		return // keep insertion order for relevance
	}
	desc := strings.EqualFold(opts.Order, "desc")
	sort.SliceStable(results, func(i, j int) bool {
		var cmp int
		switch opts.Sort {
		case "name":
			cmp = strings.Compare(results[i].Surname, results[j].Surname)
			if cmp == 0 {
				cmp = strings.Compare(results[i].GivenName, results[j].GivenName)
			}
		case "birth_date":
			cmp = compareTimePtr(results[i].BirthDateSort, results[j].BirthDateSort)
		case "death_date":
			cmp = compareTimePtr(results[i].DeathDateSort, results[j].DeathDateSort)
		default:
			return false
		}
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
}

// compareTimePtr does a three-way comparison of nullable times. Nil sorts last.
func compareTimePtr(a, b *time.Time) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return 1 // nil sorts last
	}
	if b == nil {
		return -1
	}
	return a.Compare(*b)
}

// nameMatchesQuery checks if a PersonNameReadModel matches the query.
func nameMatchesQuery(name repository.PersonNameReadModel, queryLower string) bool {
	return strings.Contains(strings.ToLower(name.FullName), queryLower) ||
		strings.Contains(strings.ToLower(name.GivenName), queryLower) ||
		strings.Contains(strings.ToLower(name.Surname), queryLower) ||
		strings.Contains(strings.ToLower(name.Nickname), queryLower)
}

// matchesSearchFilters checks if a person matches the date/place filters in SearchOptions.
func (s *ReadModelStore) matchesSearchFilters(p *repository.PersonReadModel, opts repository.SearchOptions) bool {
	if opts.BirthDateFrom != nil && (p.BirthDateSort == nil || p.BirthDateSort.Before(*opts.BirthDateFrom)) {
		return false
	}
	if opts.BirthDateTo != nil && (p.BirthDateSort == nil || p.BirthDateSort.After(*opts.BirthDateTo)) {
		return false
	}
	if opts.DeathDateFrom != nil && (p.DeathDateSort == nil || p.DeathDateSort.Before(*opts.DeathDateFrom)) {
		return false
	}
	if opts.DeathDateTo != nil && (p.DeathDateSort == nil || p.DeathDateSort.After(*opts.DeathDateTo)) {
		return false
	}
	if opts.BirthPlace != "" && !strings.Contains(strings.ToLower(p.BirthPlace), strings.ToLower(opts.BirthPlace)) {
		return false
	}
	if opts.DeathPlace != "" && !strings.Contains(strings.ToLower(p.DeathPlace), strings.ToLower(opts.DeathPlace)) {
		return false
	}
	return true
}

// SavePerson saves or updates a person.
func (s *ReadModelStore) SavePerson(ctx context.Context, person *repository.PersonReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := *person
	s.persons[person.ID] = &result
	return nil
}

// DeletePerson removes a person.
func (s *ReadModelStore) DeletePerson(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.persons, id)
	// Also delete associated person names (cascade behavior)
	delete(s.personNames, id)
	return nil
}

// SavePersonName saves or updates a person name variant.
func (s *ReadModelStore) SavePersonName(ctx context.Context, name *repository.PersonNameReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Compute full name if not set
	fullName := name.FullName
	if fullName == "" {
		fullName = name.GivenName + " " + name.Surname
	}

	names := s.personNames[name.PersonID]
	// Check if already exists and update
	for i, n := range names {
		if n.ID != name.ID {
			continue
		}
		nameCopy := *name
		nameCopy.FullName = fullName
		names[i] = nameCopy
		s.personNames[name.PersonID] = names
		return nil
	}
	// Add new
	nameCopy := *name
	nameCopy.FullName = fullName
	s.personNames[name.PersonID] = append(names, nameCopy)
	return nil
}

// GetPersonName retrieves a person name by ID.
func (s *ReadModelStore) GetPersonName(ctx context.Context, nameID uuid.UUID) (*repository.PersonNameReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, names := range s.personNames {
		for _, n := range names {
			if n.ID == nameID {
				result := n
				return &result, nil
			}
		}
	}
	return nil, nil
}

// GetPersonNames retrieves all name variants for a person.
func (s *ReadModelStore) GetPersonNames(ctx context.Context, personID uuid.UUID) ([]repository.PersonNameReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := s.personNames[personID]
	if names == nil {
		return nil, nil
	}
	result := make([]repository.PersonNameReadModel, len(names))
	copy(result, names)

	// Sort by is_primary DESC, then name_type
	sort.Slice(result, func(i, j int) bool {
		if result[i].IsPrimary != result[j].IsPrimary {
			return result[i].IsPrimary // true comes before false
		}
		return result[i].NameType < result[j].NameType
	})

	return result, nil
}

// DeletePersonName removes a person name.
func (s *ReadModelStore) DeletePersonName(ctx context.Context, nameID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for personID, names := range s.personNames {
		for i, n := range names {
			if n.ID == nameID {
				s.personNames[personID] = append(names[:i], names[i+1:]...)
				return nil
			}
		}
	}
	return nil
}

// GetFamily retrieves a family by ID.
func (s *ReadModelStore) GetFamily(ctx context.Context, id uuid.UUID) (*repository.FamilyReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	f, exists := s.families[id]
	if !exists {
		return nil, nil
	}
	result := *f
	return &result, nil
}

// ListFamilies returns a paginated list of families.
func (s *ReadModelStore) ListFamilies(ctx context.Context, opts repository.ListOptions) ([]repository.FamilyReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	families := make([]repository.FamilyReadModel, 0, len(s.families))
	for _, f := range s.families {
		families = append(families, *f)
	}

	total := len(families)

	// Paginate
	start := opts.Offset
	if start > len(families) {
		start = len(families)
	}
	end := start + opts.Limit
	if end > len(families) {
		end = len(families)
	}

	return families[start:end], total, nil
}

// GetFamiliesForPerson returns all families where the person is a partner.
func (s *ReadModelStore) GetFamiliesForPerson(ctx context.Context, personID uuid.UUID) ([]repository.FamilyReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.FamilyReadModel
	for _, f := range s.families {
		if (f.Partner1ID != nil && *f.Partner1ID == personID) ||
			(f.Partner2ID != nil && *f.Partner2ID == personID) {
			results = append(results, *f)
		}
	}
	return results, nil
}

// SaveFamily saves or updates a family.
func (s *ReadModelStore) SaveFamily(ctx context.Context, family *repository.FamilyReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := *family
	s.families[family.ID] = &result
	return nil
}

// DeleteFamily removes a family.
func (s *ReadModelStore) DeleteFamily(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.families, id)
	delete(s.familyChildren, id)
	return nil
}

// GetFamilyChildren returns all children for a family.
func (s *ReadModelStore) GetFamilyChildren(ctx context.Context, familyID uuid.UUID) ([]repository.FamilyChildReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	children := s.familyChildren[familyID]
	if children == nil {
		return nil, nil
	}
	result := make([]repository.FamilyChildReadModel, len(children))
	copy(result, children)
	return result, nil
}

// GetChildrenOfFamily returns person read models for all children in a family.
func (s *ReadModelStore) GetChildrenOfFamily(ctx context.Context, familyID uuid.UUID) ([]repository.PersonReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	children := s.familyChildren[familyID]
	var result []repository.PersonReadModel
	for _, child := range children {
		if p, exists := s.persons[child.PersonID]; exists {
			result = append(result, *p)
		}
	}
	return result, nil
}

// GetChildFamily returns the family where the person is a child.
func (s *ReadModelStore) GetChildFamily(ctx context.Context, personID uuid.UUID) (*repository.FamilyReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for familyID, children := range s.familyChildren {
		for _, child := range children {
			if child.PersonID == personID {
				if f, exists := s.families[familyID]; exists {
					result := *f
					return &result, nil
				}
			}
		}
	}
	return nil, nil
}

// SaveFamilyChild saves a family child relationship.
func (s *ReadModelStore) SaveFamilyChild(ctx context.Context, child *repository.FamilyChildReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	children := s.familyChildren[child.FamilyID]
	// Check if already exists and update
	for i, c := range children {
		if c.PersonID == child.PersonID {
			children[i] = *child
			s.familyChildren[child.FamilyID] = children
			return nil
		}
	}
	// Add new
	s.familyChildren[child.FamilyID] = append(children, *child)
	return nil
}

// DeleteFamilyChild removes a family child relationship.
func (s *ReadModelStore) DeleteFamilyChild(ctx context.Context, familyID, personID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	children := s.familyChildren[familyID]
	for i, c := range children {
		if c.PersonID == personID {
			s.familyChildren[familyID] = append(children[:i], children[i+1:]...)
			return nil
		}
	}
	return nil
}

// GetPedigreeEdge returns the pedigree edge for a person.
func (s *ReadModelStore) GetPedigreeEdge(ctx context.Context, personID uuid.UUID) (*repository.PedigreeEdge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	edge, exists := s.pedigreeEdges[personID]
	if !exists {
		return nil, nil
	}
	result := *edge
	return &result, nil
}

// SavePedigreeEdge saves a pedigree edge.
func (s *ReadModelStore) SavePedigreeEdge(ctx context.Context, edge *repository.PedigreeEdge) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := *edge
	s.pedigreeEdges[edge.PersonID] = &result
	return nil
}

// DeletePedigreeEdge removes a pedigree edge.
func (s *ReadModelStore) DeletePedigreeEdge(ctx context.Context, personID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.pedigreeEdges, personID)
	return nil
}

// Reset clears all data.
func (s *ReadModelStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.persons = make(map[uuid.UUID]*repository.PersonReadModel)
	s.personNames = make(map[uuid.UUID][]repository.PersonNameReadModel)
	s.families = make(map[uuid.UUID]*repository.FamilyReadModel)
	s.familyChildren = make(map[uuid.UUID][]repository.FamilyChildReadModel)
	s.pedigreeEdges = make(map[uuid.UUID]*repository.PedigreeEdge)
	s.sources = make(map[uuid.UUID]*repository.SourceReadModel)
	s.citations = make(map[uuid.UUID]*repository.CitationReadModel)
	s.media = make(map[uuid.UUID]*repository.MediaReadModel)
	s.events = make(map[uuid.UUID]*repository.EventReadModel)
	s.attributes = make(map[uuid.UUID]*repository.AttributeReadModel)
	s.notes = make(map[uuid.UUID]*repository.NoteReadModel)
	s.submitters = make(map[uuid.UUID]*repository.SubmitterReadModel)
	s.associations = make(map[uuid.UUID]*repository.AssociationReadModel)
	s.ldsOrdinances = make(map[uuid.UUID]*repository.LDSOrdinanceReadModel)
}

// GetSource retrieves a source by ID.
func (s *ReadModelStore) GetSource(ctx context.Context, id uuid.UUID) (*repository.SourceReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	src, exists := s.sources[id]
	if !exists {
		return nil, nil
	}
	result := *src
	return &result, nil
}

// ListSources returns a paginated list of sources.
func (s *ReadModelStore) ListSources(ctx context.Context, opts repository.ListOptions) ([]repository.SourceReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sources := make([]repository.SourceReadModel, 0, len(s.sources))
	for _, src := range s.sources {
		sources = append(sources, *src)
	}

	// Sort by title
	sort.Slice(sources, func(i, j int) bool {
		cmp := strings.Compare(sources[i].Title, sources[j].Title)
		if opts.Order == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})

	total := len(sources)

	// Paginate
	start := opts.Offset
	if start > len(sources) {
		start = len(sources)
	}
	end := start + opts.Limit
	if end > len(sources) {
		end = len(sources)
	}

	return sources[start:end], total, nil
}

// SearchSources searches for sources by title.
func (s *ReadModelStore) SearchSources(ctx context.Context, query string, limit int) ([]repository.SourceReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query = strings.ToLower(query)
	var results []repository.SourceReadModel

	for _, src := range s.sources {
		title := strings.ToLower(src.Title)
		author := strings.ToLower(src.Author)

		if strings.Contains(title, query) || strings.Contains(author, query) {
			results = append(results, *src)
			if len(results) >= limit {
				break
			}
		}
	}

	return results, nil
}

// SaveSource saves or updates a source.
func (s *ReadModelStore) SaveSource(ctx context.Context, source *repository.SourceReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := *source
	s.sources[source.ID] = &result
	return nil
}

// DeleteSource removes a source.
func (s *ReadModelStore) DeleteSource(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sources, id)
	return nil
}

// GetCitation retrieves a citation by ID.
func (s *ReadModelStore) GetCitation(ctx context.Context, id uuid.UUID) (*repository.CitationReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cit, exists := s.citations[id]
	if !exists {
		return nil, nil
	}
	result := *cit
	return &result, nil
}

// ListCitations returns all citations with pagination.
func (s *ReadModelStore) ListCitations(ctx context.Context, opts repository.ListOptions) ([]repository.CitationReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	citations := make([]repository.CitationReadModel, 0, len(s.citations))
	for _, cit := range s.citations {
		citations = append(citations, *cit)
	}

	// Sort by source title, then by fact type, then by ID for deterministic ordering
	sort.Slice(citations, func(i, j int) bool {
		cmp := strings.Compare(citations[i].SourceTitle, citations[j].SourceTitle)
		if cmp == 0 {
			cmp = strings.Compare(string(citations[i].FactType), string(citations[j].FactType))
		}
		if cmp == 0 {
			cmp = strings.Compare(citations[i].ID.String(), citations[j].ID.String())
		}
		if opts.Order == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})

	total := len(citations)

	// Paginate
	start := opts.Offset
	if start > len(citations) {
		start = len(citations)
	}
	end := start + opts.Limit
	if end > len(citations) {
		end = len(citations)
	}

	return citations[start:end], total, nil
}

// GetCitationsForSource returns all citations for a source.
func (s *ReadModelStore) GetCitationsForSource(ctx context.Context, sourceID uuid.UUID) ([]repository.CitationReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.CitationReadModel
	for _, cit := range s.citations {
		if cit.SourceID == sourceID {
			results = append(results, *cit)
		}
	}
	return results, nil
}

// GetCitationsForPerson returns all citations for a person.
func (s *ReadModelStore) GetCitationsForPerson(ctx context.Context, personID uuid.UUID) ([]repository.CitationReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.CitationReadModel
	for _, cit := range s.citations {
		if cit.FactOwnerID == personID && strings.HasPrefix(string(cit.FactType), "person_") {
			results = append(results, *cit)
		}
	}
	return results, nil
}

// GetCitationsForFact returns all citations for a specific fact.
func (s *ReadModelStore) GetCitationsForFact(ctx context.Context, factType domain.FactType, factOwnerID uuid.UUID) ([]repository.CitationReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.CitationReadModel
	for _, cit := range s.citations {
		if cit.FactType == factType && cit.FactOwnerID == factOwnerID {
			results = append(results, *cit)
		}
	}
	return results, nil
}

// SaveCitation saves or updates a citation.
func (s *ReadModelStore) SaveCitation(ctx context.Context, citation *repository.CitationReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := *citation
	s.citations[citation.ID] = &result
	return nil
}

// DeleteCitation removes a citation.
func (s *ReadModelStore) DeleteCitation(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.citations, id)
	return nil
}

// GetMedia retrieves media metadata by ID (excludes FileData and ThumbnailData).
func (s *ReadModelStore) GetMedia(ctx context.Context, id uuid.UUID) (*repository.MediaReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, exists := s.media[id]
	if !exists {
		return nil, nil
	}
	// Return copy without binary data
	result := *m
	result.FileData = nil
	result.ThumbnailData = nil
	return &result, nil
}

// GetMediaWithData retrieves full media record including FileData and ThumbnailData.
func (s *ReadModelStore) GetMediaWithData(ctx context.Context, id uuid.UUID) (*repository.MediaReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, exists := s.media[id]
	if !exists {
		return nil, nil
	}
	result := *m
	return &result, nil
}

// GetMediaThumbnail retrieves just the thumbnail bytes.
func (s *ReadModelStore) GetMediaThumbnail(ctx context.Context, id uuid.UUID) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, exists := s.media[id]
	if !exists {
		return nil, nil
	}
	return m.ThumbnailData, nil
}

// ListMediaForEntity returns a paginated list of media for an entity.
func (s *ReadModelStore) ListMediaForEntity(ctx context.Context, entityType string, entityID uuid.UUID, opts repository.ListOptions) ([]repository.MediaReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.MediaReadModel
	for _, m := range s.media {
		if m.EntityType == entityType && m.EntityID == entityID {
			result := *m
			result.FileData = nil
			result.ThumbnailData = nil
			results = append(results, result)
		}
	}

	total := len(results)

	// Sort by created_at DESC
	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	// Apply pagination
	if opts.Offset > 0 {
		if opts.Offset >= len(results) {
			results = nil
		} else {
			results = results[opts.Offset:]
		}
	}

	if opts.Limit > 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	return results, total, nil
}

// SaveMedia saves or updates a media record.
func (s *ReadModelStore) SaveMedia(ctx context.Context, media *repository.MediaReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := *media
	s.media[media.ID] = &result
	return nil
}

// DeleteMedia removes a media record.
func (s *ReadModelStore) DeleteMedia(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.media, id)
	return nil
}

// GetEvent retrieves an event by ID.
func (s *ReadModelStore) GetEvent(ctx context.Context, id uuid.UUID) (*repository.EventReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	e, exists := s.events[id]
	if !exists {
		return nil, nil
	}
	eventCopy := *e
	return &eventCopy, nil
}

// ListEventsForPerson returns all events for a person.
func (s *ReadModelStore) ListEventsForPerson(ctx context.Context, personID uuid.UUID) ([]repository.EventReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.EventReadModel
	for _, e := range s.events {
		if e.OwnerType == "person" && e.OwnerID == personID {
			results = append(results, *e)
		}
	}
	return results, nil
}

// ListEventsForFamily returns all events for a family.
func (s *ReadModelStore) ListEventsForFamily(ctx context.Context, familyID uuid.UUID) ([]repository.EventReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.EventReadModel
	for _, e := range s.events {
		if e.OwnerType == "family" && e.OwnerID == familyID {
			results = append(results, *e)
		}
	}
	return results, nil
}

// ListEvents returns all events with pagination.
func (s *ReadModelStore) ListEvents(ctx context.Context, opts repository.ListOptions) ([]repository.EventReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	events := make([]repository.EventReadModel, 0, len(s.events))
	for _, e := range s.events {
		events = append(events, *e)
	}

	// Sort by fact type, then by date, then by ID for deterministic ordering
	sort.Slice(events, func(i, j int) bool {
		cmp := strings.Compare(string(events[i].FactType), string(events[j].FactType))
		if cmp == 0 {
			// Sort by date if same fact type
			if events[i].DateSort != nil && events[j].DateSort != nil {
				cmp = events[i].DateSort.Compare(*events[j].DateSort)
			} else if events[i].DateSort == nil && events[j].DateSort != nil {
				cmp = 1 // nil dates sort after non-nil
			} else if events[i].DateSort != nil && events[j].DateSort == nil {
				cmp = -1
			}
			// Both nil: cmp stays 0
		}
		if cmp == 0 {
			cmp = strings.Compare(events[i].ID.String(), events[j].ID.String())
		}
		if opts.Order == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})

	total := len(events)

	// Paginate
	start := opts.Offset
	if start > len(events) {
		start = len(events)
	}
	end := start + opts.Limit
	if end > len(events) {
		end = len(events)
	}

	return events[start:end], total, nil
}

// SaveEvent saves or updates an event.
func (s *ReadModelStore) SaveEvent(ctx context.Context, event *repository.EventReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	eventCopy := *event
	s.events[event.ID] = &eventCopy
	return nil
}

// DeleteEvent removes an event.
func (s *ReadModelStore) DeleteEvent(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.events, id)
	return nil
}

// GetAttribute retrieves an attribute by ID.
func (s *ReadModelStore) GetAttribute(ctx context.Context, id uuid.UUID) (*repository.AttributeReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	a, exists := s.attributes[id]
	if !exists {
		return nil, nil
	}
	attrCopy := *a
	return &attrCopy, nil
}

// ListAttributesForPerson returns all attributes for a person.
func (s *ReadModelStore) ListAttributesForPerson(ctx context.Context, personID uuid.UUID) ([]repository.AttributeReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.AttributeReadModel
	for _, a := range s.attributes {
		if a.PersonID == personID {
			results = append(results, *a)
		}
	}
	return results, nil
}

// ListAttributes returns all attributes with pagination.
func (s *ReadModelStore) ListAttributes(ctx context.Context, opts repository.ListOptions) ([]repository.AttributeReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	attributes := make([]repository.AttributeReadModel, 0, len(s.attributes))
	for _, a := range s.attributes {
		attributes = append(attributes, *a)
	}

	// Sort by fact type, then by value, then by ID for deterministic ordering
	sort.Slice(attributes, func(i, j int) bool {
		cmp := strings.Compare(string(attributes[i].FactType), string(attributes[j].FactType))
		if cmp == 0 {
			cmp = strings.Compare(attributes[i].Value, attributes[j].Value)
		}
		if cmp == 0 {
			cmp = strings.Compare(attributes[i].ID.String(), attributes[j].ID.String())
		}
		if opts.Order == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})

	total := len(attributes)

	// Paginate
	start := opts.Offset
	if start > len(attributes) {
		start = len(attributes)
	}
	end := start + opts.Limit
	if end > len(attributes) {
		end = len(attributes)
	}

	return attributes[start:end], total, nil
}

// SaveAttribute saves or updates an attribute.
func (s *ReadModelStore) SaveAttribute(ctx context.Context, attribute *repository.AttributeReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	attrCopy := *attribute
	s.attributes[attribute.ID] = &attrCopy
	return nil
}

// DeleteAttribute removes an attribute.
func (s *ReadModelStore) DeleteAttribute(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.attributes, id)
	return nil
}

// GetSurnameIndex returns a list of unique surnames with counts and letter distribution.
func (s *ReadModelStore) GetSurnameIndex(ctx context.Context) ([]repository.SurnameEntry, []repository.LetterCount, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	surnameCount := make(map[string]int)
	surnamesByLetter := make(map[string]map[string]bool) // letter -> set of surnames

	for _, p := range s.persons {
		surname := p.Surname
		surnameCount[surname]++
		if surname != "" {
			letter := strings.ToUpper(string(surname[0]))
			if surnamesByLetter[letter] == nil {
				surnamesByLetter[letter] = make(map[string]bool)
			}
			surnamesByLetter[letter][surname] = true
		}
	}

	surnames := make([]repository.SurnameEntry, 0, len(surnameCount))
	for name, count := range surnameCount {
		surnames = append(surnames, repository.SurnameEntry{Surname: name, Count: count})
	}
	sort.Slice(surnames, func(i, j int) bool {
		return surnames[i].Surname < surnames[j].Surname
	})

	letters := make([]repository.LetterCount, 0, len(surnamesByLetter))
	for letter, surnameSet := range surnamesByLetter {
		letters = append(letters, repository.LetterCount{Letter: letter, Count: len(surnameSet)})
	}
	sort.Slice(letters, func(i, j int) bool {
		return letters[i].Letter < letters[j].Letter
	})

	return surnames, letters, nil
}

// GetSurnamesByLetter returns surnames starting with a specific letter.
func (s *ReadModelStore) GetSurnamesByLetter(ctx context.Context, letter string) ([]repository.SurnameEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	surnameCount := make(map[string]int)
	for _, p := range s.persons {
		surname := p.Surname
		if surname != "" && strings.EqualFold(string(surname[0]), letter) {
			surnameCount[surname]++
		}
	}

	surnames := make([]repository.SurnameEntry, 0, len(surnameCount))
	for name, count := range surnameCount {
		surnames = append(surnames, repository.SurnameEntry{Surname: name, Count: count})
	}
	sort.Slice(surnames, func(i, j int) bool {
		return surnames[i].Surname < surnames[j].Surname
	})

	return surnames, nil
}

// GetPersonsBySurname returns persons with a specific surname.
func (s *ReadModelStore) GetPersonsBySurname(ctx context.Context, surname string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.PersonReadModel
	for _, p := range s.persons {
		if strings.EqualFold(p.Surname, surname) {
			results = append(results, *p)
		}
	}

	total := len(results)

	// Sort by GivenName
	sort.Slice(results, func(i, j int) bool {
		return results[i].GivenName < results[j].GivenName
	})

	// Apply pagination
	if opts.Offset > 0 && opts.Offset < len(results) {
		results = results[opts.Offset:]
	} else if opts.Offset >= len(results) {
		results = nil
	}
	if opts.Limit > 0 && opts.Limit < len(results) {
		results = results[:opts.Limit]
	}

	return results, total, nil
}

// GetPlaceHierarchy returns places at a given level of hierarchy.
func (s *ReadModelStore) GetPlaceHierarchy(ctx context.Context, parent string) ([]repository.PlaceEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Simple implementation: extract unique places
	placeCount := make(map[string]int)

	for _, p := range s.persons {
		for _, place := range []string{p.BirthPlace, p.DeathPlace} {
			if place != "" {
				placeCount[place]++
			}
		}
	}

	entries := make([]repository.PlaceEntry, 0, len(placeCount))
	for place, count := range placeCount {
		entries = append(entries, repository.PlaceEntry{
			Name:     place,
			FullName: place,
			Count:    count,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	return entries, nil
}

// GetPersonsByPlace returns persons associated with a specific place.
func (s *ReadModelStore) GetPersonsByPlace(ctx context.Context, place string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.PersonReadModel
	for _, p := range s.persons {
		if strings.Contains(p.BirthPlace, place) || strings.Contains(p.DeathPlace, place) {
			results = append(results, *p)
		}
	}

	total := len(results)

	// Sort by surname
	sort.Slice(results, func(i, j int) bool {
		if results[i].Surname != results[j].Surname {
			return results[i].Surname < results[j].Surname
		}
		return results[i].GivenName < results[j].GivenName
	})

	// Apply pagination
	if opts.Offset > 0 && opts.Offset < len(results) {
		results = results[opts.Offset:]
	} else if opts.Offset >= len(results) {
		results = nil
	}
	if opts.Limit > 0 && opts.Limit < len(results) {
		results = results[:opts.Limit]
	}

	return results, total, nil
}

// GetCemeteryIndex returns unique burial/cremation places with person counts.
func (s *ReadModelStore) GetCemeteryIndex(ctx context.Context) ([]repository.CemeteryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Count distinct persons per place for burial/cremation events
	placePersons := make(map[string]map[uuid.UUID]struct{})
	for _, e := range s.events {
		if e.Place == "" {
			continue
		}
		if e.FactType != domain.FactPersonBurial && e.FactType != domain.FactPersonCremation {
			continue
		}
		if _, ok := placePersons[e.Place]; !ok {
			placePersons[e.Place] = make(map[uuid.UUID]struct{})
		}
		placePersons[e.Place][e.OwnerID] = struct{}{}
	}

	entries := make([]repository.CemeteryEntry, 0, len(placePersons))
	for place, persons := range placePersons {
		entries = append(entries, repository.CemeteryEntry{
			Place: place,
			Count: len(persons),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Place < entries[j].Place
	})

	return entries, nil
}

// GetPersonsByCemetery returns persons with burial/cremation events at the given place.
func (s *ReadModelStore) GetPersonsByCemetery(ctx context.Context, place string, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Find distinct person IDs with matching burial/cremation events (exact case-insensitive match)
	matchedIDs := make(map[uuid.UUID]struct{})
	for _, e := range s.events {
		if e.FactType != domain.FactPersonBurial && e.FactType != domain.FactPersonCremation {
			continue
		}
		if strings.EqualFold(e.Place, place) {
			matchedIDs[e.OwnerID] = struct{}{}
		}
	}

	var results []repository.PersonReadModel
	for _, p := range s.persons {
		if _, ok := matchedIDs[p.ID]; ok {
			results = append(results, *p)
		}
	}

	total := len(results)

	// Sort by surname, then given name
	sort.Slice(results, func(i, j int) bool {
		if results[i].Surname != results[j].Surname {
			return results[i].Surname < results[j].Surname
		}
		return results[i].GivenName < results[j].GivenName
	})

	// Apply pagination
	if opts.Offset > 0 && opts.Offset < len(results) {
		results = results[opts.Offset:]
	} else if opts.Offset >= len(results) {
		results = nil
	}
	if opts.Limit > 0 && opts.Limit < len(results) {
		results = results[:opts.Limit]
	}

	return results, total, nil
}

// GetMapLocations returns aggregated geographic locations from person birth/death coordinates.
func (s *ReadModelStore) GetMapLocations(ctx context.Context) ([]repository.MapLocation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Key by "place|eventType" to aggregate
	type locKey struct {
		place     string
		eventType string
	}
	type locData struct {
		lat       float64
		lon       float64
		personIDs []uuid.UUID
	}

	agg := make(map[locKey]*locData)

	for _, p := range s.persons {
		// Birth location
		if p.BirthPlaceLat != nil && p.BirthPlaceLong != nil && *p.BirthPlaceLat != "" && *p.BirthPlaceLong != "" {
			lat, errLat := gedcom.ParseGEDCOMCoordinate(*p.BirthPlaceLat)
			lon, errLon := gedcom.ParseGEDCOMCoordinate(*p.BirthPlaceLong)
			if errLat == nil && errLon == nil {
				key := locKey{place: p.BirthPlace, eventType: "birth"}
				if d, ok := agg[key]; ok {
					d.personIDs = append(d.personIDs, p.ID)
				} else {
					agg[key] = &locData{lat: lat, lon: lon, personIDs: []uuid.UUID{p.ID}}
				}
			}
		}
		// Death location
		if p.DeathPlaceLat != nil && p.DeathPlaceLong != nil && *p.DeathPlaceLat != "" && *p.DeathPlaceLong != "" {
			lat, errLat := gedcom.ParseGEDCOMCoordinate(*p.DeathPlaceLat)
			lon, errLon := gedcom.ParseGEDCOMCoordinate(*p.DeathPlaceLong)
			if errLat == nil && errLon == nil {
				key := locKey{place: p.DeathPlace, eventType: "death"}
				if d, ok := agg[key]; ok {
					d.personIDs = append(d.personIDs, p.ID)
				} else {
					agg[key] = &locData{lat: lat, lon: lon, personIDs: []uuid.UUID{p.ID}}
				}
			}
		}
	}

	results := make([]repository.MapLocation, 0, len(agg))
	for key, data := range agg {
		results = append(results, repository.MapLocation{
			Place:     key.place,
			Latitude:  data.lat,
			Longitude: data.lon,
			EventType: key.eventType,
			Count:     len(data.personIDs),
			PersonIDs: data.personIDs,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Place != results[j].Place {
			return results[i].Place < results[j].Place
		}
		return results[i].EventType < results[j].EventType
	})

	return results, nil
}

// SetBrickWall marks a person as a brick wall with a note.
func (s *ReadModelStore) SetBrickWall(ctx context.Context, personID uuid.UUID, note string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, exists := s.persons[personID]
	if !exists {
		return nil
	}
	now := time.Now()
	p.BrickWallNote = note
	p.BrickWallSince = &now
	p.BrickWallResolvedAt = nil
	return nil
}

// ResolveBrickWall marks a brick wall as resolved.
func (s *ReadModelStore) ResolveBrickWall(ctx context.Context, personID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, exists := s.persons[personID]
	if !exists {
		return nil
	}
	now := time.Now()
	p.BrickWallResolvedAt = &now
	return nil
}

// GetBrickWalls returns persons with brick wall status.
func (s *ReadModelStore) GetBrickWalls(ctx context.Context, includeResolved bool) ([]repository.BrickWallEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var entries []repository.BrickWallEntry
	for _, p := range s.persons {
		if p.BrickWallSince == nil {
			continue
		}
		if !includeResolved && p.BrickWallResolvedAt != nil {
			continue
		}
		entries = append(entries, repository.BrickWallEntry{
			PersonID:   p.ID,
			PersonName: p.FullName,
			Note:       p.BrickWallNote,
			Since:      *p.BrickWallSince,
			ResolvedAt: p.BrickWallResolvedAt,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Since.After(entries[j].Since)
	})

	return entries, nil
}

// GetNote retrieves a note by ID.
func (s *ReadModelStore) GetNote(ctx context.Context, id uuid.UUID) (*repository.NoteReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	n, exists := s.notes[id]
	if !exists {
		return nil, nil
	}
	result := *n
	return &result, nil
}

// ListNotes returns a paginated list of notes.
func (s *ReadModelStore) ListNotes(ctx context.Context, opts repository.ListOptions) ([]repository.NoteReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.NoteReadModel
	for _, n := range s.notes {
		results = append(results, *n)
	}

	total := len(results)

	// Sort by updated_at
	asc := opts.Order == "asc"
	sort.Slice(results, func(i, j int) bool {
		if asc {
			return results[i].UpdatedAt.Before(results[j].UpdatedAt)
		}
		return results[i].UpdatedAt.After(results[j].UpdatedAt)
	})

	// Apply pagination
	if opts.Offset > 0 && opts.Offset < len(results) {
		results = results[opts.Offset:]
	} else if opts.Offset >= len(results) {
		results = nil
	}
	if opts.Limit > 0 && opts.Limit < len(results) {
		results = results[:opts.Limit]
	}

	return results, total, nil
}

// SaveNote saves or updates a note.
func (s *ReadModelStore) SaveNote(ctx context.Context, note *repository.NoteReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := *note
	s.notes[note.ID] = &result
	return nil
}

// DeleteNote removes a note.
func (s *ReadModelStore) DeleteNote(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.notes, id)
	return nil
}

// GetSubmitter retrieves a submitter by ID.
func (s *ReadModelStore) GetSubmitter(ctx context.Context, id uuid.UUID) (*repository.SubmitterReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sub, exists := s.submitters[id]
	if !exists {
		return nil, nil
	}
	result := *sub
	return &result, nil
}

// ListSubmitters returns a paginated list of submitters.
func (s *ReadModelStore) ListSubmitters(ctx context.Context, opts repository.ListOptions) ([]repository.SubmitterReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.SubmitterReadModel
	for _, sub := range s.submitters {
		results = append(results, *sub)
	}

	total := len(results)

	// Sort by name
	asc := opts.Order == "asc"
	sort.Slice(results, func(i, j int) bool {
		if asc {
			return results[i].Name < results[j].Name
		}
		return results[i].Name > results[j].Name
	})

	// Apply pagination
	if opts.Offset > 0 && opts.Offset < len(results) {
		results = results[opts.Offset:]
	} else if opts.Offset >= len(results) {
		results = nil
	}
	if opts.Limit > 0 && opts.Limit < len(results) {
		results = results[:opts.Limit]
	}

	return results, total, nil
}

// SaveSubmitter saves or updates a submitter.
func (s *ReadModelStore) SaveSubmitter(ctx context.Context, submitter *repository.SubmitterReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := *submitter
	s.submitters[submitter.ID] = &result
	return nil
}

// DeleteSubmitter removes a submitter.
func (s *ReadModelStore) DeleteSubmitter(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.submitters, id)
	return nil
}

// GetAssociation retrieves an association by ID.
func (s *ReadModelStore) GetAssociation(ctx context.Context, id uuid.UUID) (*repository.AssociationReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	assoc, exists := s.associations[id]
	if !exists {
		return nil, nil
	}
	result := *assoc
	return &result, nil
}

// ListAssociations returns a paginated list of associations.
func (s *ReadModelStore) ListAssociations(ctx context.Context, opts repository.ListOptions) ([]repository.AssociationReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.AssociationReadModel
	for _, assoc := range s.associations {
		results = append(results, *assoc)
	}

	total := len(results)

	// Sort by role or updated_at
	sortField := opts.Sort
	if sortField == "" {
		sortField = "updated_at"
	}
	asc := opts.Order == "asc"
	sort.Slice(results, func(i, j int) bool {
		var cmp int
		if sortField == "role" {
			cmp = strings.Compare(results[i].Role, results[j].Role)
		} else {
			cmp = compareTimestamps(results[i].UpdatedAt, results[j].UpdatedAt)
		}
		if asc {
			return cmp < 0
		}
		return cmp > 0
	})

	// Apply pagination
	if opts.Offset > 0 && opts.Offset < len(results) {
		results = results[opts.Offset:]
	} else if opts.Offset >= len(results) {
		results = nil
	}
	if opts.Limit > 0 && opts.Limit < len(results) {
		results = results[:opts.Limit]
	}

	return results, total, nil
}

// ListAssociationsForPerson returns all associations for a given person.
func (s *ReadModelStore) ListAssociationsForPerson(ctx context.Context, personID uuid.UUID) ([]repository.AssociationReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.AssociationReadModel
	for _, assoc := range s.associations {
		if assoc.PersonID == personID || assoc.AssociateID == personID {
			results = append(results, *assoc)
		}
	}

	// Sort by role, then updated_at
	sort.Slice(results, func(i, j int) bool {
		cmp := strings.Compare(results[i].Role, results[j].Role)
		if cmp != 0 {
			return cmp < 0
		}
		return results[i].UpdatedAt.After(results[j].UpdatedAt)
	})

	return results, nil
}

// SaveAssociation saves or updates an association.
func (s *ReadModelStore) SaveAssociation(ctx context.Context, assoc *repository.AssociationReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := *assoc
	s.associations[assoc.ID] = &result
	return nil
}

// DeleteAssociation removes an association.
func (s *ReadModelStore) DeleteAssociation(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.associations, id)
	return nil
}

// GetLDSOrdinance retrieves an LDS ordinance by ID.
func (s *ReadModelStore) GetLDSOrdinance(ctx context.Context, id uuid.UUID) (*repository.LDSOrdinanceReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ord, exists := s.ldsOrdinances[id]
	if !exists {
		return nil, nil
	}
	result := *ord
	return &result, nil
}

// ListLDSOrdinances returns a paginated list of LDS ordinances.
func (s *ReadModelStore) ListLDSOrdinances(ctx context.Context, opts repository.ListOptions) ([]repository.LDSOrdinanceReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.LDSOrdinanceReadModel
	for _, ord := range s.ldsOrdinances {
		results = append(results, *ord)
	}

	total := len(results)

	// Sort by type or updated_at
	sortField := opts.Sort
	if sortField == "" {
		sortField = "updated_at"
	}
	asc := opts.Order == "asc"
	sort.Slice(results, func(i, j int) bool {
		var cmp int
		if sortField == "type" {
			cmp = strings.Compare(string(results[i].Type), string(results[j].Type))
		} else {
			cmp = compareTimestamps(results[i].UpdatedAt, results[j].UpdatedAt)
		}
		if asc {
			return cmp < 0
		}
		return cmp > 0
	})

	// Apply pagination
	if opts.Offset > 0 && opts.Offset < len(results) {
		results = results[opts.Offset:]
	} else if opts.Offset >= len(results) {
		results = nil
	}
	if opts.Limit > 0 && opts.Limit < len(results) {
		results = results[:opts.Limit]
	}

	return results, total, nil
}

// ListLDSOrdinancesForPerson returns all LDS ordinances for a given person.
func (s *ReadModelStore) ListLDSOrdinancesForPerson(ctx context.Context, personID uuid.UUID) ([]repository.LDSOrdinanceReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.LDSOrdinanceReadModel
	for _, ord := range s.ldsOrdinances {
		if ord.PersonID != nil && *ord.PersonID == personID {
			results = append(results, *ord)
		}
	}

	// Sort by type
	sort.Slice(results, func(i, j int) bool {
		return string(results[i].Type) < string(results[j].Type)
	})

	return results, nil
}

// ListLDSOrdinancesForFamily returns all LDS ordinances for a given family.
func (s *ReadModelStore) ListLDSOrdinancesForFamily(ctx context.Context, familyID uuid.UUID) ([]repository.LDSOrdinanceReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []repository.LDSOrdinanceReadModel
	for _, ord := range s.ldsOrdinances {
		if ord.FamilyID != nil && *ord.FamilyID == familyID {
			results = append(results, *ord)
		}
	}

	// Sort by type
	sort.Slice(results, func(i, j int) bool {
		return string(results[i].Type) < string(results[j].Type)
	})

	return results, nil
}

// SaveLDSOrdinance saves or updates an LDS ordinance.
func (s *ReadModelStore) SaveLDSOrdinance(ctx context.Context, ord *repository.LDSOrdinanceReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := *ord
	s.ldsOrdinances[ord.ID] = &result
	return nil
}

// DeleteLDSOrdinance removes an LDS ordinance.
func (s *ReadModelStore) DeleteLDSOrdinance(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.ldsOrdinances, id)
	return nil
}
