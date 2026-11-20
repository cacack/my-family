package memory

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/repository"
)

// Compile-time assertion that SettingsStore satisfies the interface.
var _ repository.SettingsStore = (*SettingsStore)(nil)

// SettingsStore is an in-memory implementation of repository.SettingsStore.
type SettingsStore struct {
	mu sync.RWMutex
	// homePersons maps an account (or repository.SiteWide) to its home
	// person. One that cleared its choice maps to uuid.Nil; one that never
	// chose is absent.
	homePersons map[uuid.UUID]uuid.UUID
}

// NewSettingsStore creates a new in-memory settings store.
func NewSettingsStore() *SettingsStore {
	return &SettingsStore{homePersons: make(map[uuid.UUID]uuid.UUID)}
}

// HomePerson returns accountID's home person, or uuid.Nil when none is set.
func (s *SettingsStore) HomePerson(_ context.Context, accountID uuid.UUID) (uuid.UUID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.homePersons[accountID], nil
}

// SetHomePerson marks personID as accountID's home person; uuid.Nil clears it.
func (s *SettingsStore) SetHomePerson(_ context.Context, accountID, personID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.homePersons[accountID] = personID
	return nil
}

// SeedHomePerson gives accountID the site's home person unless accountID has
// chosen or cleared its own.
func (s *SettingsStore) SeedHomePerson(_ context.Context, accountID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, chosen := s.homePersons[accountID]; chosen {
		return nil
	}
	if site := s.homePersons[repository.SiteWide]; site != uuid.Nil {
		s.homePersons[accountID] = site
	}
	return nil
}

// MoveHomePersons makes survivors[p] the home person of every account whose
// home person is p.
func (s *SettingsStore) MoveHomePersons(_ context.Context, survivors map[uuid.UUID]uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for account, person := range s.homePersons {
		if survivor, merged := survivors[person]; merged {
			s.homePersons[account] = survivor
		}
	}
	return nil
}

// Reset clears all settings (demo mode).
func (s *SettingsStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.homePersons)
}
