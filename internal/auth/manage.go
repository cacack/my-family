package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/repository"
)

// Link lifetimes. An invite is for someone who may take days to get to it; a
// password reset is for someone waiting on it.
const (
	InviteTTL = 7 * 24 * time.Hour
	ResetTTL  = 24 * time.Hour
)

// Errors returned by the account management methods.
var (
	// ErrLastOwner refuses demoting or disabling the tree's only active owner,
	// which would leave no one able to manage it.
	ErrLastOwner = errors.New("the tree's last owner cannot be demoted or disabled")
	// ErrInvalidLink is an invite or reset link that is unknown, expired or
	// already used.
	ErrInvalidLink = errors.New("this link is not valid: it is wrong, expired or already used")
	// ErrUserNotFound is a management action on a user who is not a member.
	ErrUserNotFound = errors.New("user not found")
)

// Actor is who asks for an account change, for the auth audit log: the
// signed-in user (uuid.Nil from the CLI or before sign-in) and their address.
type Actor struct {
	UserID   uuid.UUID
	ClientIP string
}

// Link is a single-use invite or password reset token and when it expires.
// The token is shown once, to whoever issued it; only its hash is stored.
type Link struct {
	Token     string
	ExpiresAt time.Time
}

// audit records action on subject (which may be nil) in the auth audit log.
func (s *Service) audit(ctx context.Context, by Actor, action repository.AuthAction, subject *repository.User, detail string) {
	entry := repository.AuthAuditEntry{Action: action, ActorID: by.UserID, Detail: detail, ClientIP: by.ClientIP}
	if subject != nil {
		entry.SubjectID, entry.Username = subject.ID, subject.Username
	}
	s.auditEntry(ctx, entry)
}

// auditEntry appends entry, stamped now. A failure to record is logged, not
// returned: the action it describes has already happened.
func (s *Service) auditEntry(ctx context.Context, entry repository.AuthAuditEntry) {
	entry.At = s.now().UTC()
	if err := s.store.AppendAuthAudit(ctx, &entry); err != nil {
		slog.Error("auth: could not write the auth audit log", "action", entry.Action, "error", err)
	}
}

// AuthAudit returns up to limit audit entries, newest first, after offset,
// and how many there are in all.
func (s *Service) AuthAudit(ctx context.Context, limit, offset int) ([]repository.AuthAuditEntry, int, error) {
	return s.store.ListAuthAudit(ctx, limit, offset)
}

// Members returns every member of the tree, by username.
func (s *Service) Members(ctx context.Context) ([]repository.Member, error) {
	return s.store.ListMembers(ctx, repository.DefaultTreeID)
}

// Member returns the member with userID, or ErrUserNotFound.
func (s *Service) Member(ctx context.Context, userID uuid.UUID) (*repository.Member, error) {
	members, err := s.Members(ctx)
	if err != nil {
		return nil, err
	}
	for i := range members {
		if members[i].User.ID == userID {
			return &members[i], nil
		}
	}
	return nil, ErrUserNotFound
}

// CreateInvite issues an invite link that creates an account with role.
func (s *Service) CreateInvite(ctx context.Context, by Actor, role repository.Role) (*Link, error) {
	if !role.Valid() {
		return nil, invalid("role must be viewer, editor or owner")
	}
	token, err := newToken()
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	invite := &repository.Invite{
		TokenHash: hashToken(token), TreeID: repository.DefaultTreeID, Role: role,
		CreatedBy: by.UserID, CreatedAt: now, ExpiresAt: now.Add(InviteTTL),
	}
	if err := s.store.CreateInvite(ctx, invite); err != nil {
		return nil, fmt.Errorf("create invite: %w", err)
	}
	s.audit(ctx, by, repository.AuthActionInviteCreated, nil, string(role))
	return &Link{Token: token, ExpiresAt: invite.ExpiresAt}, nil
}

// AcceptInvite creates an account from an invite link, with the invite's
// role, and signs it in, returning the account, its role and the session
// token. clientIP is the caller's address.
func (s *Service) AcceptInvite(ctx context.Context, token, username, displayName, password, clientIP string) (*repository.User, repository.Role, string, error) {
	user, err := s.newUser(username, displayName, password)
	if err != nil {
		return nil, "", "", err
	}
	invite, err := s.store.AcceptInvite(ctx, hashToken(strings.TrimSpace(token)), s.now().UTC(), user)
	switch {
	case errors.Is(err, repository.ErrLinkNotFound):
		return nil, "", "", ErrInvalidLink
	case errors.Is(err, repository.ErrUsernameTaken):
		return nil, "", "", invalid("username %q is already taken", user.Username)
	case err != nil:
		return nil, "", "", fmt.Errorf("accept invite: %w", err)
	}
	s.audit(ctx, Actor{UserID: user.ID, ClientIP: clientIP}, repository.AuthActionInviteAccepted, user, string(invite.Role))
	sessionToken, err := s.startSession(ctx, user.ID)
	if err != nil {
		return nil, "", "", err
	}
	return user, invite.Role, sessionToken, nil
}

// activeOwners counts the members who are owners and not disabled.
func activeOwners(members []repository.Member) int {
	n := 0
	for _, m := range members {
		if m.Role == repository.RoleOwner && !m.User.Disabled {
			n++
		}
	}
	return n
}

// lockMember takes manageMu and returns the member with userID and whether
// they are the last active owner. The caller unlocks manageMu.
func (s *Service) lockMember(ctx context.Context, userID uuid.UUID) (*repository.Member, bool, error) {
	s.manageMu.Lock()
	members, err := s.Members(ctx)
	if err != nil {
		return nil, false, err
	}
	for i := range members {
		m := &members[i]
		if m.User.ID == userID {
			last := m.Role == repository.RoleOwner && !m.User.Disabled && activeOwners(members) == 1
			return m, last, nil
		}
	}
	return nil, false, ErrUserNotFound
}

// SetRole changes the role userID holds. The last active owner keeps theirs.
func (s *Service) SetRole(ctx context.Context, by Actor, userID uuid.UUID, role repository.Role) (*repository.Member, error) {
	if !role.Valid() {
		return nil, invalid("role must be viewer, editor or owner")
	}
	m, lastOwner, err := s.lockMember(ctx, userID)
	defer s.manageMu.Unlock()
	if err != nil {
		return nil, err
	}
	if m.Role == role {
		return m, nil
	}
	if lastOwner {
		return nil, ErrLastOwner
	}
	if err := s.store.SetMemberRole(ctx, repository.DefaultTreeID, userID, role); err != nil {
		return nil, fmt.Errorf("set role: %w", err)
	}
	s.audit(ctx, by, repository.AuthActionRoleChanged, &m.User, fmt.Sprintf("%s -> %s", m.Role, role))
	m.Role = role
	return m, nil
}

// SetDisabled disables or re-enables userID's account. Disabling ends all
// their sessions. The last active owner cannot be disabled.
func (s *Service) SetDisabled(ctx context.Context, by Actor, userID uuid.UUID, disabled bool) (*repository.Member, error) {
	m, lastOwner, err := s.lockMember(ctx, userID)
	defer s.manageMu.Unlock()
	if err != nil {
		return nil, err
	}
	if m.User.Disabled == disabled {
		return m, nil
	}
	if disabled && lastOwner {
		return nil, ErrLastOwner
	}
	m.User.Disabled = disabled
	if err := s.store.UpdateUser(ctx, &m.User); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}
	action := repository.AuthActionUserEnabled
	if disabled {
		action = repository.AuthActionUserDisabled
		if err := s.store.DeleteUserSessions(ctx, userID, ""); err != nil {
			return nil, fmt.Errorf("end sessions: %w", err)
		}
	}
	s.audit(ctx, by, action, &m.User, "")
	return m, nil
}

// IssuePasswordReset issues a link that sets userID's password.
func (s *Service) IssuePasswordReset(ctx context.Context, by Actor, userID uuid.UUID) (*Link, error) {
	m, err := s.Member(ctx, userID)
	if err != nil {
		return nil, err
	}
	token, err := newToken()
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	reset := &repository.PasswordReset{
		TokenHash: hashToken(token), UserID: userID, CreatedBy: by.UserID, CreatedAt: now, ExpiresAt: now.Add(ResetTTL),
	}
	if err := s.store.CreatePasswordReset(ctx, reset); err != nil {
		return nil, fmt.Errorf("create password reset: %w", err)
	}
	s.audit(ctx, by, repository.AuthActionResetIssued, &m.User, "")
	return &Link{Token: token, ExpiresAt: reset.ExpiresAt}, nil
}

// IssuePasswordResetFor issues a reset link for the user named username, for
// the CLI.
func (s *Service) IssuePasswordResetFor(ctx context.Context, by Actor, username string) (*repository.User, *Link, error) {
	user, err := s.store.GetUserByUsername(ctx, NormalizeUsername(username))
	if errors.Is(err, repository.ErrUserNotFound) {
		return nil, nil, ErrUserNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("find user: %w", err)
	}
	link, err := s.IssuePasswordReset(ctx, by, user.ID)
	if err != nil {
		return nil, nil, err
	}
	return user, link, nil
}

// ResetPassword sets a password from a reset link and ends every session of
// its user. It signs no one in.
func (s *Service) ResetPassword(ctx context.Context, token, password, clientIP string) error {
	if err := validatePassword(password); err != nil {
		return err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	userID, err := s.store.UsePasswordReset(ctx, hashToken(strings.TrimSpace(token)), s.now().UTC(), hash)
	if errors.Is(err, repository.ErrLinkNotFound) {
		return ErrInvalidLink
	}
	if err != nil {
		return fmt.Errorf("reset password: %w", err)
	}
	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		user = &repository.User{ID: userID}
	}
	// Whoever holds the link acts as its user.
	s.audit(ctx, Actor{UserID: userID, ClientIP: clientIP}, repository.AuthActionPasswordReset, user, "")
	return nil
}

// AccountChange is what a user changes on their own account. A nil field is
// left as it is; a new password needs the current one.
type AccountChange struct {
	DisplayName     *string
	CurrentPassword string
	NewPassword     *string
}

// UpdateAccount applies change to the signed-in user's own account.
// Changing the password ends every other session of theirs, keeping the one
// with sessionToken.
func (s *Service) UpdateAccount(ctx context.Context, by Actor, sessionToken string, change AccountChange) (*repository.User, error) {
	user, err := s.store.GetUser(ctx, by.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if change.DisplayName != nil {
		name := strings.TrimSpace(*change.DisplayName)
		if err := validateDisplayName(name); err != nil {
			return nil, err
		}
		user.DisplayName = name
	}
	if change.NewPassword != nil {
		if err := validatePassword(*change.NewPassword); err != nil {
			return nil, err
		}
		now := s.now()
		key := "password:" + user.ID.String()
		if !s.limiter.acquire(now, limit{key, maxFailuresPerName}) {
			return nil, ErrRateLimited
		}
		if ok, _ := VerifyPassword(change.CurrentPassword, user.PasswordHash); !ok {
			return nil, ErrInvalidCredentials // stays counted as a failure
		}
		s.limiter.reset(key)
		if user.PasswordHash, err = HashPassword(*change.NewPassword); err != nil {
			return nil, err
		}
	}
	if err := s.store.UpdateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("update account: %w", err)
	}
	if change.NewPassword != nil {
		if err := s.store.DeleteUserSessions(ctx, user.ID, hashToken(sessionToken)); err != nil {
			return nil, fmt.Errorf("end other sessions: %w", err)
		}
		s.audit(ctx, by, repository.AuthActionPasswordChanged, user, "")
	}
	return user, nil
}
