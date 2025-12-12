package memory

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/repository"
)

// ReadModelStore is an in-memory implementation of repository.ReadModelStore for testing.
type ReadModelStore struct {
	mu             sync.RWMutex
	persons        map[uuid.UUID]*repository.PersonReadModel
	families       map[uuid.UUID]*repository.FamilyReadModel
	familyChildren map[uuid.UUID][]repository.FamilyChildReadModel // keyed by family ID
	pedigreeEdges  map[uuid.UUID]*repository.PedigreeEdge          // keyed by person ID
}

// NewReadModelStore creates a new in-memory read model store.
func NewReadModelStore() *ReadModelStore {
	return &ReadModelStore{
		persons:        make(map[uuid.UUID]*repository.PersonReadModel),
		families:       make(map[uuid.UUID]*repository.FamilyReadModel),
		familyChildren: make(map[uuid.UUID][]repository.FamilyChildReadModel),
		pedigreeEdges:  make(map[uuid.UUID]*repository.PedigreeEdge),
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
}
