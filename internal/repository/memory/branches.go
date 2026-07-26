package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Compile-time assertion that BranchStore satisfies the interface.
var _ repository.BranchStore = (*BranchStore)(nil)

// BranchStore is an in-memory implementation of repository.BranchStore for testing.
type BranchStore struct {
	mu       sync.RWMutex
	branches map[uuid.UUID]*domain.Branch
}

// NewBranchStore creates a new in-memory branch store.
func NewBranchStore() *BranchStore {
	return &BranchStore{
		branches: make(map[uuid.UUID]*domain.Branch),
	}
}

// Create stores a new branch.
func (s *BranchStore) Create(_ context.Context, branch *domain.Branch) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Make a copy to prevent external mutation
	copied := *branch
	s.branches[branch.ID] = &copied
	return nil
}

// Upsert stores a branch, inserting or replacing any existing entry with the same ID.
func (s *BranchStore) Upsert(_ context.Context, branch *domain.Branch) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Make a copy to prevent external mutation
	copied := *branch
	s.branches[branch.ID] = &copied
	return nil
}

// Get retrieves a branch by ID.
func (s *BranchStore) Get(_ context.Context, id uuid.UUID) (*domain.Branch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	branch, exists := s.branches[id]
	if !exists {
		return nil, repository.ErrBranchNotFound
	}

	// Return a copy to prevent mutation
	copied := *branch
	return &copied, nil
}

// List retrieves all branches ordered by created_at DESC.
func (s *BranchStore) List(_ context.Context) ([]*domain.Branch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*domain.Branch, 0, len(s.branches))
	for _, branch := range s.branches {
		// Make a copy
		copied := *branch
		result = append(result, &copied)
	}

	// Sort by created_at DESC
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	return result, nil
}

// Delete removes a branch by ID.
func (s *BranchStore) Delete(_ context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.branches[id]; !exists {
		return repository.ErrBranchNotFound
	}

	delete(s.branches, id)
	return nil
}

// UpdateStatus changes a branch's status.
func (s *BranchStore) UpdateStatus(_ context.Context, id uuid.UUID, status domain.BranchStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	branch, exists := s.branches[id]
	if !exists {
		return repository.ErrBranchNotFound
	}

	branch.Status = status
	return nil
}

// Reset clears all data (useful for tests).
func (s *BranchStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.branches = make(map[uuid.UUID]*domain.Branch)
}
