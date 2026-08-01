package memory

import (
	"context"
	"sort"
	"sync"
	"time"

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

// copyBranch returns a deep copy of branch. Branch holds a *time.Time
// (MergedAt), so a plain struct copy would still share that pointer with the
// caller; this keeps the store's "no external mutation" contract true.
func copyBranch(branch *domain.Branch) *domain.Branch {
	copied := *branch
	if branch.MergedAt != nil {
		mergedAt := *branch.MergedAt
		copied.MergedAt = &mergedAt
	}
	return &copied
}

// Create stores a new branch.
func (s *BranchStore) Create(_ context.Context, branch *domain.Branch) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Make a copy to prevent external mutation
	s.branches[branch.ID] = copyBranch(branch)
	return nil
}

// Upsert stores a branch, inserting or replacing any existing entry with the same ID.
func (s *BranchStore) Upsert(_ context.Context, branch *domain.Branch) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Make a copy to prevent external mutation
	s.branches[branch.ID] = copyBranch(branch)
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
	return copyBranch(branch), nil
}

// List retrieves all branches ordered by created_at DESC.
func (s *BranchStore) List(_ context.Context) ([]*domain.Branch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*domain.Branch, 0, len(s.branches))
	for _, branch := range s.branches {
		// Make a copy
		result = append(result, copyBranch(branch))
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

// MarkMerged records the merge: status, timestamp and note in one write.
func (s *BranchStore) MarkMerged(_ context.Context, id uuid.UUID, mergedAt time.Time, note string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	branch, exists := s.branches[id]
	if !exists {
		return repository.ErrBranchNotFound
	}

	branch.Status = domain.BranchStatusMerged
	branch.MergedAt = &mergedAt
	branch.MergeNote = note
	return nil
}

// Reset clears all data (useful for tests).
func (s *BranchStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.branches = make(map[uuid.UUID]*domain.Branch)
}
