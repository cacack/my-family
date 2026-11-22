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
	// Disabled refuses the user sign-in and any session; an owner sets it.
	Disabled bool
}

// Member is a user and the role they hold in a tree.
type Member struct {
	User User
	Role Role
}

// Invite is an owner-issued link that creates an account with Role when it
// is accepted (ADR-007). The store keeps only a hash of the link's token; an
// accepted invite is deleted, so each is used at most once.
type Invite struct {
	// TokenHash doubles as the invite's ID in owner-facing listings, which is
	// safe only while redemption hashes the presented token before lookup:
	// never accept or compare a TokenHash as a token.
	TokenHash string
	TreeID    uuid.UUID
	Role      Role
	CreatedBy uuid.UUID
	CreatedAt time.Time
	ExpiresAt time.Time
}

// PasswordReset is an owner- or CLI-issued link that sets a user's password.
// Like an invite it is stored as a token hash and deleted once used.
type PasswordReset struct {
	// TokenHash doubles as the reset's ID, like Invite.TokenHash: never
	// accept or compare it as a token.
	TokenHash string
	UserID    uuid.UUID
	// CreatedBy is the owner who issued it, or uuid.Nil from the CLI.
	CreatedBy uuid.UUID
	CreatedAt time.Time
	ExpiresAt time.Time
}

// AuthAction is what an auth audit entry records.
type AuthAction string

// The audited actions.
const (
	AuthActionSignIn          AuthAction = "sign_in"
	AuthActionSignInFailed    AuthAction = "sign_in_failed"
	AuthActionUserCreated     AuthAction = "user_created"
	AuthActionInviteCreated   AuthAction = "invite_created"
	AuthActionInviteAccepted  AuthAction = "invite_accepted"
	AuthActionInviteRevoked   AuthAction = "invite_revoked"
	AuthActionRoleChanged     AuthAction = "role_changed"
	AuthActionUserDisabled    AuthAction = "user_disabled"
	AuthActionUserEnabled     AuthAction = "user_enabled"
	AuthActionResetIssued     AuthAction = "password_reset_issued"
	AuthActionResetRevoked    AuthAction = "password_reset_revoked"
	AuthActionPasswordReset   AuthAction = "password_reset"
	AuthActionPasswordChanged AuthAction = "password_changed"
	// AuthActionAnonymousAccessChanged is an owner changing the tree's
	// anonymous access; the detail is the new value.
	AuthActionAnonymousAccessChanged AuthAction = "anonymous_access_changed"
)

// AuthAuditEntry is one row of the append-only auth audit log. The log is
// only ever appended to and read: the store has no way to change or remove a
// row.
type AuthAuditEntry struct {
	// ID orders the log: a later entry has a larger ID. The store assigns it
	// on append; ListAuthAudit pages by it.
	ID     int64
	At     time.Time
	Action AuthAction
	// ActorID is who did it, or uuid.Nil (a failed sign-in, the CLI).
	ActorID uuid.UUID
	// SubjectID is the account it was done to, or uuid.Nil.
	SubjectID uuid.UUID
	// Username is the name a sign-in was attempted with, or the subject's.
	Username string
	// Detail adds what the action alone does not say, such as a new role.
	Detail   string
	ClientIP string
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
	// ErrLinkNotFound is an invite or password reset that is unknown,
	// expired or already used.
	ErrLinkNotFound = errors.New("link is unknown, expired or already used")
	// ErrLastOwner refuses a change that would leave a tree with no active
	// owner: one who holds RoleOwner and is not disabled.
	ErrLastOwner = errors.New("the tree's last active owner cannot be demoted or disabled")
)

// AccountStore keeps users, their tree memberships and sessions, invites,
// password resets, the auth audit log (ADR-007) and the tree settings
// (ADR-008). Every backend implements it over plain tables beside the event
// log; none of it is event-sourced or rebuilt.
type AccountStore interface {
	TreeSettingsStore

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

	// DeleteUserSessions removes every session of userID except the one with
	// keepTokenHash ("" keeps none).
	DeleteUserSessions(ctx context.Context, userID uuid.UUID, keepTokenHash string) error

	// ListMembers returns every member of treeID, by username compared byte
	// for byte.
	ListMembers(ctx context.Context, treeID uuid.UUID) ([]Member, error)

	// SetMemberRole changes the role userID holds in treeID, or returns
	// ErrNotMember. It returns ErrLastOwner, changing nothing, when userID is
	// treeID's last active owner and role is not RoleOwner. The check and the
	// write are one atomic step, so concurrent writers, even in other
	// processes, cannot together remove the last owner.
	SetMemberRole(ctx context.Context, treeID, userID uuid.UUID, role Role) error

	// UpdateUser stores user's display name, password hash and disabled flag,
	// or returns ErrUserNotFound. Its username and creation time are fixed.
	// It returns ErrLastOwner, changing nothing, when it would disable the
	// last active owner of a tree, atomically as SetMemberRole does.
	UpdateUser(ctx context.Context, user *User) error

	// CreateInvite stores invite.
	CreateInvite(ctx context.Context, invite *Invite) error

	// AcceptInvite uses up the invite with tokenHash, if it expires after now,
	// and creates user with the invite's tree and role, all or nothing. It
	// returns the invite, ErrLinkNotFound, or ErrUsernameTaken (leaving the
	// invite unused).
	AcceptInvite(ctx context.Context, tokenHash string, now time.Time, user *User) (*Invite, error)

	// ListInvites returns the invites to treeID that expire after now, newest
	// first.
	ListInvites(ctx context.Context, treeID uuid.UUID, now time.Time) ([]Invite, error)

	// RevokeInvite removes the invite with tokenHash, if it expires after now,
	// and returns it, or ErrLinkNotFound.
	RevokeInvite(ctx context.Context, tokenHash string, now time.Time) (*Invite, error)

	// DeleteLinksIssuedBy removes every invite and password reset userID
	// issued, so links from an owner who is disabled or demoted stop working.
	DeleteLinksIssuedBy(ctx context.Context, userID uuid.UUID) error

	// DeleteExpiredLinks removes every invite and password reset that expires
	// at or before now, returning how many it removed.
	DeleteExpiredLinks(ctx context.Context, now time.Time) (int, error)

	// CreatePasswordReset stores reset.
	CreatePasswordReset(ctx context.Context, reset *PasswordReset) error

	// UsePasswordReset uses up the reset with tokenHash, if it expires after
	// now: it sets the user's password hash and removes all their sessions
	// and resets, all or nothing. It returns the user's ID or ErrLinkNotFound.
	UsePasswordReset(ctx context.Context, tokenHash string, now time.Time, passwordHash string) (uuid.UUID, error)

	// ListPasswordResets returns the password resets that expire after now,
	// newest first.
	ListPasswordResets(ctx context.Context, now time.Time) ([]PasswordReset, error)

	// RevokePasswordReset removes the reset with tokenHash, if it expires
	// after now, and returns it, or ErrLinkNotFound.
	RevokePasswordReset(ctx context.Context, tokenHash string, now time.Time) (*PasswordReset, error)

	// AppendAuthAudit adds entry to the auth audit log. Callers append after
	// the action succeeds, outside its transaction: the log is best-effort
	// (AC-005).
	AppendAuthAudit(ctx context.Context, entry *AuthAuditEntry) error

	// ListAuthAudit returns up to limit entries with an ID below before,
	// newest first; a before of 0 starts at the newest entry. Paging by ID
	// costs the same on every page and skips or repeats no entry, even when
	// entries share a timestamp.
	ListAuthAudit(ctx context.Context, limit int, before int64) ([]AuthAuditEntry, error)

	// DeleteAuthAuditBefore removes every audit entry recorded before before,
	// returning how many it removed. It is the retention prune, the only
	// removal from the log.
	DeleteAuthAuditBefore(ctx context.Context, before time.Time) (int, error)
}
