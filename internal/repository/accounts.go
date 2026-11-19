package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// DefaultTreeID is the one tree an instance holds (ADR-007, sub-decision 2B).
// Memberships carry it so that several trees later add rows, not a migration.
var DefaultTreeID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// Role is what a membership lets a user do in a tree (ADR-007). Each role
// includes the abilities of the one before it.
type Role string

// The roles, least first.
const (
	// RoleViewer reads the tree.
	RoleViewer Role = "viewer"
	// RoleEditor also changes tree data, works in research branches and
	// exports.
	RoleEditor Role = "editor"
	// RoleOwner also manages users and settings, imports GEDCOM and rolls
	// records back.
	RoleOwner Role = "owner"
)

// roleRank orders the roles; a role absent from it ranks 0 and includes none.
var roleRank = map[Role]int{RoleViewer: 1, RoleEditor: 2, RoleOwner: 3}

// Valid reports whether r is one of the roles.
func (r Role) Valid() bool {
	return roleRank[r] > 0
}

// Includes reports whether r may do what other may: r is a role and ranks at
// least as high.
func (r Role) Includes(other Role) bool {
	return r.Valid() && roleRank[r] >= roleRank[other]
}

// User is an account. It is not tree data: it lives in plain tables, never in
// the event log, and `myfamily rebuild` leaves it alone (ADR-007, 3B).
type User struct {
	ID uuid.UUID
	// Username is the sign-in name, stored normalized (see auth); unique.
	Username    string
	DisplayName string
	// PasswordHash is an argon2id hash in PHC string form, never the password.
	PasswordHash string
	CreatedAt    time.Time
}

// Session is a signed-in browser. The store keeps only a hash of the token
// the cookie carries, so a leaked table cannot be replayed as a cookie.
type Session struct {
	TokenHash  string
	UserID     uuid.UUID
	CreatedAt  time.Time
	LastSeenAt time.Time
}

// Errors returned by AccountStore.
var (
	ErrUserNotFound    = errors.New("user not found")
	ErrUsernameTaken   = errors.New("username already taken")
	ErrSessionNotFound = errors.New("session not found")
	ErrNotMember       = errors.New("user is not a member of the tree")
)

// AccountStore keeps users, their tree memberships and their sessions
// (ADR-007). Every backend implements it over plain tables beside the event
// log; none of it is event-sourced or rebuilt.
type AccountStore interface {
	// CreateUser stores user and its membership of treeID with role, both or
	// neither. It returns ErrUsernameTaken when the username is in use.
	CreateUser(ctx context.Context, user *User, treeID uuid.UUID, role Role) error

	// GetUser returns the user with id, or ErrUserNotFound.
	GetUser(ctx context.Context, id uuid.UUID) (*User, error)

	// GetUserByUsername returns the user with username, or ErrUserNotFound.
	GetUserByUsername(ctx context.Context, username string) (*User, error)

	// MemberRole returns the role userID holds in treeID, or ErrNotMember.
	MemberRole(ctx context.Context, treeID, userID uuid.UUID) (Role, error)

	// CountMembers returns how many users hold role in treeID.
	CountMembers(ctx context.Context, treeID uuid.UUID, role Role) (int, error)

	// DisplayNames returns the display name of each of ids that is a user.
	// An id with no user is absent from the result.
	DisplayNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error)

	// CreateSession stores session.
	CreateSession(ctx context.Context, session *Session) error

	// GetSession returns the session with tokenHash, or ErrSessionNotFound.
	GetSession(ctx context.Context, tokenHash string) (*Session, error)

	// TouchSession sets the session's last-seen time, or returns
	// ErrSessionNotFound.
	TouchSession(ctx context.Context, tokenHash string, lastSeen time.Time) error

	// DeleteSession removes the session with tokenHash. Removing one that is
	// not there is not an error: signing out twice signs out.
	DeleteSession(ctx context.Context, tokenHash string) error

	// DeleteExpiredSessions removes every session last seen before idleBefore
	// or created before createdBefore, returning how many it removed.
	DeleteExpiredSessions(ctx context.Context, idleBefore, createdBefore time.Time) (int, error)
}
