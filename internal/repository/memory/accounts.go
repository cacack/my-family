package memory

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/repository"
)

// Compile-time assertion that AccountStore satisfies the interface.
var _ repository.AccountStore = (*AccountStore)(nil)

// membership is one row of the memory tree_memberships table.
type membership struct {
	userID uuid.UUID
	treeID uuid.UUID
	role   repository.Role
}

// AccountStore is an in-memory implementation of repository.AccountStore.
type AccountStore struct {
	mu          sync.RWMutex
	users       map[uuid.UUID]repository.User
	memberships []membership
	sessions    map[string]repository.Session
}

// NewAccountStore creates an empty in-memory account store.
func NewAccountStore() *AccountStore {
	return &AccountStore{
		users:    make(map[uuid.UUID]repository.User),
		sessions: make(map[string]repository.Session),
	}
}

// CreateUser stores user and its membership of treeID with role.
func (s *AccountStore) CreateUser(_ context.Context, user *repository.User, treeID uuid.UUID, role repository.Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.users {
		if existing.Username == user.Username {
			return repository.ErrUsernameTaken
		}
	}
	s.users[user.ID] = *user
	s.memberships = append(s.memberships, membership{userID: user.ID, treeID: treeID, role: role})
	return nil
}

// GetUser returns the user with id.
func (s *AccountStore) GetUser(_ context.Context, id uuid.UUID) (*repository.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.users[id]
	if !ok {
		return nil, repository.ErrUserNotFound
	}
	return &user, nil
}

// GetUserByUsername returns the user with username.
func (s *AccountStore) GetUserByUsername(_ context.Context, username string) (*repository.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, user := range s.users {
		if user.Username == username {
			return &user, nil
		}
	}
	return nil, repository.ErrUserNotFound
}

// MemberRole returns the role userID holds in treeID.
func (s *AccountStore) MemberRole(_ context.Context, treeID, userID uuid.UUID) (repository.Role, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, m := range s.memberships {
		if m.treeID == treeID && m.userID == userID {
			return m.role, nil
		}
	}
	return "", repository.ErrNotMember
}

// CountMembers returns how many users hold role in treeID.
func (s *AccountStore) CountMembers(_ context.Context, treeID uuid.UUID, role repository.Role) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, m := range s.memberships {
		if m.treeID == treeID && m.role == role {
			count++
		}
	}
	return count, nil
}

// DisplayNames returns the display name of each of ids that is a user.
func (s *AccountStore) DisplayNames(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make(map[uuid.UUID]string, len(ids))
	for _, id := range ids {
		if user, ok := s.users[id]; ok {
			names[id] = user.DisplayName
		}
	}
	return names, nil
}

// CreateSession stores session.
func (s *AccountStore) CreateSession(_ context.Context, session *repository.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.TokenHash] = *session
	return nil
}

// GetSession returns the session with tokenHash.
func (s *AccountStore) GetSession(_ context.Context, tokenHash string) (*repository.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[tokenHash]
	if !ok {
		return nil, repository.ErrSessionNotFound
	}
	return &session, nil
}

// TouchSession sets the session's last-seen time.
func (s *AccountStore) TouchSession(_ context.Context, tokenHash string, lastSeen time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[tokenHash]
	if !ok {
		return repository.ErrSessionNotFound
	}
	session.LastSeenAt = lastSeen
	s.sessions[tokenHash] = session
	return nil
}

// DeleteSession removes the session with tokenHash, if there is one.
func (s *AccountStore) DeleteSession(_ context.Context, tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, tokenHash)
	return nil
}

// DeleteExpiredSessions removes every session idle since before idleBefore or
// created before createdBefore.
func (s *AccountStore) DeleteExpiredSessions(_ context.Context, idleBefore, createdBefore time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for hash, session := range s.sessions {
		if session.LastSeenAt.Before(idleBefore) || session.CreatedAt.Before(createdBefore) {
			delete(s.sessions, hash)
			removed++
		}
	}
	return removed, nil
}
