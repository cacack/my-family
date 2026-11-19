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
	mu         sync.RWMutex
	homePerson uuid.UUID
}

// NewSettingsStore creates a new in-memory settings store.
func NewSettingsStore() *SettingsStore {
	return &SettingsStore{}
}

// HomePerson returns the home person, or uuid.Nil when none is set.
func (s *SettingsStore) HomePerson(_ context.Context) (uuid.UUID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.homePerson, nil
}

// SetHomePerson marks id as the home person; uuid.Nil clears it.
func (s *SettingsStore) SetHomePerson(_ context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.homePerson = id
	return nil
}
