package memory

import (
	"context"
	"sort"
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
	apiTokens   map[string]repository.APIToken // by token hash
	invites     map[string]repository.Invite
	resets      map[string]repository.PasswordReset
	audit       []repository.AuthAuditEntry // oldest first
	lastAuditID int64
	settings    map[uuid.UUID]repository.TreeSettings
}

// NewAccountStore creates an empty in-memory account store.
func NewAccountStore() *AccountStore {
	return &AccountStore{
		users:     make(map[uuid.UUID]repository.User),
		sessions:  make(map[string]repository.Session),
		apiTokens: make(map[string]repository.APIToken),
		invites:   make(map[string]repository.Invite),
		resets:    make(map[string]repository.PasswordReset),
		settings:  make(map[uuid.UUID]repository.TreeSettings),
	}
}

// CreateUser stores user and its membership of treeID with role.
func (s *AccountStore) CreateUser(_ context.Context, user *repository.User, treeID uuid.UUID, role repository.Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createUser(user, treeID, role)
}

func (s *AccountStore) createUser(user *repository.User, treeID uuid.UUID, role repository.Role) error {
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

// DeleteUserSessions removes every session of userID but keepTokenHash.
func (s *AccountStore) DeleteUserSessions(_ context.Context, userID uuid.UUID, keepTokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteUserSessions(userID, keepTokenHash)
	return nil
}

func (s *AccountStore) deleteUserSessions(userID uuid.UUID, keepTokenHash string) {
	for hash, session := range s.sessions {
		if session.UserID == userID && hash != keepTokenHash {
			delete(s.sessions, hash)
		}
	}
}

// copyAPIToken returns token with its own LastUsedAt, so a caller cannot
// change the stored row through the pointer.
func copyAPIToken(token repository.APIToken) repository.APIToken {
	if token.LastUsedAt != nil {
		lastUsed := *token.LastUsedAt
		token.LastUsedAt = &lastUsed
	}
	return token
}

// CreateAPIToken stores token.
func (s *AccountStore) CreateAPIToken(_ context.Context, token *repository.APIToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.apiTokens[token.TokenHash] = copyAPIToken(*token)
	return nil
}

// GetAPIToken returns the API token with tokenHash.
func (s *AccountStore) GetAPIToken(_ context.Context, tokenHash string) (*repository.APIToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	token, ok := s.apiTokens[tokenHash]
	if !ok {
		return nil, repository.ErrAPITokenNotFound
	}
	token = copyAPIToken(token)
	return &token, nil
}

// ListAPITokens returns userID's API tokens, newest first.
func (s *AccountStore) ListAPITokens(_ context.Context, userID uuid.UUID) ([]repository.APIToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var tokens []repository.APIToken
	for _, token := range s.apiTokens {
		if token.UserID == userID {
			tokens = append(tokens, copyAPIToken(token))
		}
	}
	sort.Slice(tokens, func(i, j int) bool {
		if !tokens[i].CreatedAt.Equal(tokens[j].CreatedAt) {
			return tokens[i].CreatedAt.After(tokens[j].CreatedAt)
		}
		return tokens[i].ID.String() > tokens[j].ID.String()
	})
	return tokens, nil
}

// TouchAPIToken sets the token's last-used time.
func (s *AccountStore) TouchAPIToken(_ context.Context, tokenHash string, lastUsed time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, ok := s.apiTokens[tokenHash]
	if !ok {
		return repository.ErrAPITokenNotFound
	}
	token.LastUsedAt = &lastUsed
	s.apiTokens[tokenHash] = token
	return nil
}

// DeleteAPIToken removes userID's API token with id.
func (s *AccountStore) DeleteAPIToken(_ context.Context, userID, id uuid.UUID) (*repository.APIToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for hash, token := range s.apiTokens {
		if token.ID == id && token.UserID == userID {
			delete(s.apiTokens, hash)
			return &token, nil
		}
	}
	return nil, repository.ErrAPITokenNotFound
}

// ListMembers returns every member of treeID, by username.
func (s *AccountStore) ListMembers(_ context.Context, treeID uuid.UUID) ([]repository.Member, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	members := []repository.Member{}
	for _, m := range s.memberships {
		if m.treeID == treeID {
			members = append(members, repository.Member{User: s.users[m.userID], Role: m.role})
		}
	}
	sort.Slice(members, func(i, j int) bool { return members[i].User.Username < members[j].User.Username })
	return members, nil
}

// SetMemberRole changes the role userID holds in treeID.
func (s *AccountStore) SetMemberRole(_ context.Context, treeID, userID uuid.UUID, role repository.Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, m := range s.memberships {
		if m.treeID == treeID && m.userID == userID {
			if role != repository.RoleOwner && s.lastActiveOwner(m) {
				return repository.ErrLastOwner
			}
			s.memberships[i].role = role
			return nil
		}
	}
	return repository.ErrNotMember
}

// lastActiveOwner reports whether m makes its user the last active owner of
// its tree. The caller holds s.mu.
func (s *AccountStore) lastActiveOwner(m membership) bool {
	if m.role != repository.RoleOwner || s.users[m.userID].Disabled {
		return false
	}
	for _, o := range s.memberships {
		if o.treeID == m.treeID && o.userID != m.userID && o.role == repository.RoleOwner && !s.users[o.userID].Disabled {
			return false
		}
	}
	return true
}

// UpdateUser stores user's display name, password hash and disabled flag.
func (s *AccountStore) UpdateUser(_ context.Context, user *repository.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.users[user.ID]
	if !ok {
		return repository.ErrUserNotFound
	}
	if user.Disabled {
		for _, m := range s.memberships {
			if m.userID == user.ID && s.lastActiveOwner(m) {
				return repository.ErrLastOwner
			}
		}
	}
	existing.DisplayName, existing.PasswordHash, existing.Disabled = user.DisplayName, user.PasswordHash, user.Disabled
	s.users[user.ID] = existing
	return nil
}

// CreateInvite stores invite.
func (s *AccountStore) CreateInvite(_ context.Context, invite *repository.Invite) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invites[invite.TokenHash] = *invite
	return nil
}

// AcceptInvite uses up the invite with tokenHash and creates user from it.
func (s *AccountStore) AcceptInvite(_ context.Context, tokenHash string, now time.Time, user *repository.User) (*repository.Invite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	invite, ok := s.invites[tokenHash]
	if !ok || !invite.ExpiresAt.After(now) {
		return nil, repository.ErrLinkNotFound
	}
	if err := s.createUser(user, invite.TreeID, invite.Role); err != nil {
		return nil, err
	}
	delete(s.invites, tokenHash)
	return &invite, nil
}

// ListInvites returns the invites to treeID that expire after now, newest
// first.
func (s *AccountStore) ListInvites(_ context.Context, treeID uuid.UUID, now time.Time) ([]repository.Invite, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	invites := []repository.Invite{}
	for _, invite := range s.invites {
		if invite.TreeID == treeID && invite.ExpiresAt.After(now) {
			invites = append(invites, invite)
		}
	}
	sort.Slice(invites, func(i, j int) bool {
		return newerLink(invites[i].CreatedAt, invites[i].TokenHash, invites[j].CreatedAt, invites[j].TokenHash)
	})
	return invites, nil
}

// RevokeInvite removes the invite with tokenHash, if it expires after now.
func (s *AccountStore) RevokeInvite(_ context.Context, tokenHash string, now time.Time) (*repository.Invite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	invite, ok := s.invites[tokenHash]
	if !ok || !invite.ExpiresAt.After(now) {
		return nil, repository.ErrLinkNotFound
	}
	delete(s.invites, tokenHash)
	return &invite, nil
}

// newerLink orders links newest first, then by token hash, as the SQL
// backends do.
func newerLink(aCreated time.Time, aHash string, bCreated time.Time, bHash string) bool {
	if !aCreated.Equal(bCreated) {
		return aCreated.After(bCreated)
	}
	return aHash < bHash
}

// DeleteLinksIssuedBy removes every invite and password reset userID issued.
func (s *AccountStore) DeleteLinksIssuedBy(_ context.Context, userID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for hash, invite := range s.invites {
		if invite.CreatedBy == userID {
			delete(s.invites, hash)
		}
	}
	for hash, reset := range s.resets {
		if reset.CreatedBy == userID {
			delete(s.resets, hash)
		}
	}
	return nil
}

// DeleteExpiredLinks removes every invite and password reset that expires at
// or before now.
func (s *AccountStore) DeleteExpiredLinks(_ context.Context, now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for hash, invite := range s.invites {
		if !invite.ExpiresAt.After(now) {
			delete(s.invites, hash)
			removed++
		}
	}
	for hash, reset := range s.resets {
		if !reset.ExpiresAt.After(now) {
			delete(s.resets, hash)
			removed++
		}
	}
	return removed, nil
}

// CreatePasswordReset stores reset.
func (s *AccountStore) CreatePasswordReset(_ context.Context, reset *repository.PasswordReset) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resets[reset.TokenHash] = *reset
	return nil
}

// UsePasswordReset uses up the reset with tokenHash and sets the password.
func (s *AccountStore) UsePasswordReset(_ context.Context, tokenHash string, now time.Time, passwordHash string) (uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reset, ok := s.resets[tokenHash]
	if !ok || !reset.ExpiresAt.After(now) {
		return uuid.Nil, repository.ErrLinkNotFound
	}
	user, ok := s.users[reset.UserID]
	if !ok {
		return uuid.Nil, repository.ErrLinkNotFound
	}
	user.PasswordHash = passwordHash
	s.users[user.ID] = user
	for hash, r := range s.resets {
		if r.UserID == user.ID {
			delete(s.resets, hash)
		}
	}
	s.deleteUserSessions(user.ID, "")
	return user.ID, nil
}

// ListPasswordResets returns the password resets that expire after now,
// newest first.
func (s *AccountStore) ListPasswordResets(_ context.Context, now time.Time) ([]repository.PasswordReset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	resets := []repository.PasswordReset{}
	for _, reset := range s.resets {
		if reset.ExpiresAt.After(now) {
			resets = append(resets, reset)
		}
	}
	sort.Slice(resets, func(i, j int) bool {
		return newerLink(resets[i].CreatedAt, resets[i].TokenHash, resets[j].CreatedAt, resets[j].TokenHash)
	})
	return resets, nil
}

// RevokePasswordReset removes the reset with tokenHash, if it expires after
// now.
func (s *AccountStore) RevokePasswordReset(_ context.Context, tokenHash string, now time.Time) (*repository.PasswordReset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reset, ok := s.resets[tokenHash]
	if !ok || !reset.ExpiresAt.After(now) {
		return nil, repository.ErrLinkNotFound
	}
	delete(s.resets, tokenHash)
	return &reset, nil
}

// AppendAuthAudit adds entry to the auth audit log.
func (s *AccountStore) AppendAuthAudit(_ context.Context, entry *repository.AuthAuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastAuditID++
	stored := *entry
	stored.ID = s.lastAuditID
	s.audit = append(s.audit, stored)
	return nil
}

// ListAuthAudit returns up to limit entries with an ID below before (any, for
// 0), newest first.
func (s *AccountStore) ListAuthAudit(_ context.Context, limit int, before int64) ([]repository.AuthAuditEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := []repository.AuthAuditEntry{}
	for i := len(s.audit) - 1; i >= 0 && len(entries) < limit; i-- {
		if before == 0 || s.audit[i].ID < before {
			entries = append(entries, s.audit[i])
		}
	}
	return entries, nil
}

// DeleteAuthAuditBefore removes every audit entry recorded before before.
func (s *AccountStore) DeleteAuthAuditBefore(_ context.Context, before time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.audit[:0]
	for _, entry := range s.audit {
		if !entry.At.Before(before) {
			kept = append(kept, entry)
		}
	}
	removed := len(s.audit) - len(kept)
	clear(s.audit[len(kept):])
	s.audit = kept
	return removed, nil
}

// GetTreeSettings returns treeID's settings, or the defaults.
func (s *AccountStore) GetTreeSettings(_ context.Context, treeID uuid.UUID) (repository.TreeSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if settings, ok := s.settings[treeID]; ok {
		return settings, nil
	}
	return repository.DefaultTreeSettings(), nil
}

// SaveTreeSettings stores treeID's settings.
func (s *AccountStore) SaveTreeSettings(_ context.Context, treeID uuid.UUID, settings repository.TreeSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings[treeID] = settings
	return nil
}
