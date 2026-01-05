package memory

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// ReadModelStore is an in-memory implementation of repository.ReadModelStore for testing.
type ReadModelStore struct {
	mu             sync.RWMutex
	persons        map[uuid.UUID]*repository.PersonReadModel
	families       map[uuid.UUID]*repository.FamilyReadModel
	familyChildren map[uuid.UUID][]repository.FamilyChildReadModel // keyed by family ID
	pedigreeEdges  map[uuid.UUID]*repository.PedigreeEdge          // keyed by person ID
	sources        map[uuid.UUID]*repository.SourceReadModel
	citations      map[uuid.UUID]*repository.CitationReadModel
	media          map[uuid.UUID]*repository.MediaReadModel
	events         map[uuid.UUID]*repository.EventReadModel
	attributes     map[uuid.UUID]*repository.AttributeReadModel
}

// NewReadModelStore creates a new in-memory read model store.
func NewReadModelStore() *ReadModelStore {
	return &ReadModelStore{
		persons:        make(map[uuid.UUID]*repository.PersonReadModel),
		families:       make(map[uuid.UUID]*repository.FamilyReadModel),
		familyChildren: make(map[uuid.UUID][]repository.FamilyChildReadModel),
		pedigreeEdges:  make(map[uuid.UUID]*repository.PedigreeEdge),
		sources:        make(map[uuid.UUID]*repository.SourceReadModel),
		citations:      make(map[uuid.UUID]*repository.CitationReadModel),
		media:          make(map[uuid.UUID]*repository.MediaReadModel),
		events:         make(map[uuid.UUID]*repository.EventReadModel),
		attributes:     make(map[uuid.UUID]*repository.AttributeReadModel),
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
	copy := *p
	return &copy, nil
}

// ListPersons returns a paginated list of persons.
func (s *ReadModelStore) ListPersons(ctx context.Context, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Convert map to slice
	persons := make([]repository.PersonReadModel, 0, len(s.persons))
	for _, p := range s.persons {
		persons = append(persons, *p)
	}

	// Sort
	sort.Slice(persons, func(i, j int) bool {
		var cmp int
		switch opts.Sort {
		case "given_name":
			cmp = strings.Compare(persons[i].GivenName, persons[j].GivenName)
		case "birth_date":
			if persons[i].BirthDateSort == nil && persons[j].BirthDateSort == nil {
				cmp = 0
			} else if persons[i].BirthDateSort == nil {
				cmp = 1
			} else if persons[j].BirthDateSort == nil {
				cmp = -1
			} else if persons[i].BirthDateSort.Before(*persons[j].BirthDateSort) {
				cmp = -1
			} else if persons[i].BirthDateSort.After(*persons[j].BirthDateSort) {
				cmp = 1
			}
		case "updated_at":
			if persons[i].UpdatedAt.Before(persons[j].UpdatedAt) {
				cmp = -1
			} else if persons[i].UpdatedAt.After(persons[j].UpdatedAt) {
				cmp = 1
			}
		default: // surname
			cmp = strings.Compare(persons[i].Surname, persons[j].Surname)
			if cmp == 0 {
				cmp = strings.Compare(persons[i].GivenName, persons[j].GivenName)
			}
		}
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

// SearchPersons searches for persons by name.
func (s *ReadModelStore) SearchPersons(ctx context.Context, query string, fuzzy bool, limit int) ([]repository.PersonReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query = strings.ToLower(query)
	var results []repository.PersonReadModel

	for _, p := range s.persons {
		fullName := strings.ToLower(p.FullName)
		givenName := strings.ToLower(p.GivenName)
		surname := strings.ToLower(p.Surname)

		// Simple contains matching
		if strings.Contains(fullName, query) ||
			strings.Contains(givenName, query) ||
			strings.Contains(surname, query) {
			results = append(results, *p)
			if len(results) >= limit {
				break
			}
		}
	}

	return results, nil
}

// SavePerson saves or updates a person.
func (s *ReadModelStore) SavePerson(ctx context.Context, person *repository.PersonReadModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	copy := *person
	s.persons[person.ID] = &copy
	return nil
}

// DeletePerson removes a person.
func (s *ReadModelStore) DeletePerson(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.persons, id)
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
	copy := *f
	return &copy, nil
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

	copy := *family
	s.families[family.ID] = &copy
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
					copy := *f
					return &copy, nil
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
	copy := *edge
	return &copy, nil
}

// SavePedigreeEdge saves a pedigree edge.
func (s *ReadModelStore) SavePedigreeEdge(ctx context.Context, edge *repository.PedigreeEdge) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	copy := *edge
	s.pedigreeEdges[edge.PersonID] = &copy
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
	s.families = make(map[uuid.UUID]*repository.FamilyReadModel)
	s.familyChildren = make(map[uuid.UUID][]repository.FamilyChildReadModel)
	s.pedigreeEdges = make(map[uuid.UUID]*repository.PedigreeEdge)
	s.sources = make(map[uuid.UUID]*repository.SourceReadModel)
	s.citations = make(map[uuid.UUID]*repository.CitationReadModel)
	s.media = make(map[uuid.UUID]*repository.MediaReadModel)
	s.events = make(map[uuid.UUID]*repository.EventReadModel)
	s.attributes = make(map[uuid.UUID]*repository.AttributeReadModel)
}

// GetSource retrieves a source by ID.
func (s *ReadModelStore) GetSource(ctx context.Context, id uuid.UUID) (*repository.SourceReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	src, exists := s.sources[id]
	if !exists {
		return nil, nil
	}
	copy := *src
	return &copy, nil
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

	copy := *source
	s.sources[source.ID] = &copy
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
	copy := *cit
	return &copy, nil
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

	copy := *citation
	s.citations[citation.ID] = &copy
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
	copy := *m
	copy.FileData = nil
	copy.ThumbnailData = nil
	return &copy, nil
}

// GetMediaWithData retrieves full media record including FileData and ThumbnailData.
func (s *ReadModelStore) GetMediaWithData(ctx context.Context, id uuid.UUID) (*repository.MediaReadModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, exists := s.media[id]
	if !exists {
		return nil, nil
	}
	copy := *m
	return &copy, nil
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
			copy := *m
			copy.FileData = nil
			copy.ThumbnailData = nil
			results = append(results, copy)
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

	copy := *media
	s.media[media.ID] = &copy
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
		if len(surname) > 0 {
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
	upperLetter := strings.ToUpper(letter)

	for _, p := range s.persons {
		surname := p.Surname
		if len(surname) > 0 && strings.ToUpper(string(surname[0])) == upperLetter {
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
