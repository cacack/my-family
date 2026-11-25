package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/cacack/my-family/internal/repository"
)

// Compile-time assertion that AccountStore satisfies the interface.
var _ repository.AccountStore = (*AccountStore)(nil)

// uniqueViolation is PostgreSQL's SQLSTATE for a broken unique constraint.
const uniqueViolation = "23505"

// AccountStore is a PostgreSQL implementation of repository.AccountStore. Its
// tables are neither event log nor read model: `myfamily rebuild` never
// drops them (ADR-007).
type AccountStore struct {
	db *sql.DB
}

// NewAccountStore returns the store, first bringing the store schema, which
// owns the account tables, up to date (ADR-011).
func NewAccountStore(db *sql.DB) (*AccountStore, error) {
	if err := migrateStoreSchema(db); err != nil {
		return nil, fmt.Errorf("create account tables: %w", err)
	}
	return &AccountStore{db: db}, nil
}

// CreateUser stores user and its membership of treeID with role in one
// transaction.
func (s *AccountStore) CreateUser(ctx context.Context, user *repository.User, treeID uuid.UUID, role repository.Role) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin create user: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := createUser(ctx, tx, user, treeID, role); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit create user: %w", err)
	}
	return nil
}

func createUser(ctx context.Context, tx *sql.Tx, user *repository.User, treeID uuid.UUID, role repository.Role) error {
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO users (id, username, display_name, password_hash, created_at, disabled) VALUES ($1, $2, $3, $4, $5, $6)`,
		user.ID, user.Username, user.DisplayName, user.PasswordHash, user.CreatedAt, user.Disabled); err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == uniqueViolation {
			return repository.ErrUsernameTaken
		}
		return fmt.Errorf("insert user: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO tree_memberships (user_id, tree_id, role) VALUES ($1, $2, $3)`,
		user.ID, treeID, string(role)); err != nil {
		return fmt.Errorf("insert membership: %w", err)
	}
	return nil
}

func (s *AccountStore) getUser(ctx context.Context, where string, arg any) (*repository.User, error) {
	var user repository.User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, display_name, password_hash, created_at, disabled FROM users WHERE `+where+` = $1`, arg,
	).Scan(&user.ID, &user.Username, &user.DisplayName, &user.PasswordHash, &user.CreatedAt, &user.Disabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return &user, nil
}

// GetUser returns the user with id.
func (s *AccountStore) GetUser(ctx context.Context, id uuid.UUID) (*repository.User, error) {
	return s.getUser(ctx, "id", id)
}

// GetUserByUsername returns the user with username.
func (s *AccountStore) GetUserByUsername(ctx context.Context, username string) (*repository.User, error) {
	return s.getUser(ctx, "username", username)
}

// MemberRole returns the role userID holds in treeID.
func (s *AccountStore) MemberRole(ctx context.Context, treeID, userID uuid.UUID) (repository.Role, error) {
	var role string
	err := s.db.QueryRowContext(ctx,
		`SELECT role FROM tree_memberships WHERE tree_id = $1 AND user_id = $2`, treeID, userID,
	).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", repository.ErrNotMember
	}
	if err != nil {
		return "", fmt.Errorf("get member role: %w", err)
	}
	return repository.Role(role), nil
}

// CountMembers returns how many users hold role in treeID.
func (s *AccountStore) CountMembers(ctx context.Context, treeID uuid.UUID, role repository.Role) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tree_memberships WHERE tree_id = $1 AND role = $2`, treeID, string(role),
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("count members: %w", err)
	}
	return count, nil
}

// DisplayNames returns the display name of each of ids that is a user.
func (s *AccountStore) DisplayNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	names := make(map[uuid.UUID]string, len(ids))
	if len(ids) == 0 {
		return names, nil
	}
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = id.String()
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, display_name FROM users WHERE id = ANY($1::uuid[])`, pq.Array(strs))
	if err != nil {
		return nil, fmt.Errorf("list display names: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id   uuid.UUID
			name string
		)
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scan display name: %w", err)
		}
		names[id] = name
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list display names: %w", err)
	}
	return names, nil
}

// CreateSession stores session.
func (s *AccountStore) CreateSession(ctx context.Context, session *repository.Session) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, created_at, last_seen_at) VALUES ($1, $2, $3, $4)`,
		session.TokenHash, session.UserID, session.CreatedAt, session.LastSeenAt); err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// GetSession returns the session with tokenHash.
func (s *AccountStore) GetSession(ctx context.Context, tokenHash string) (*repository.Session, error) {
	var session repository.Session
	err := s.db.QueryRowContext(ctx,
		`SELECT token_hash, user_id, created_at, last_seen_at FROM sessions WHERE token_hash = $1`, tokenHash,
	).Scan(&session.TokenHash, &session.UserID, &session.CreatedAt, &session.LastSeenAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	return &session, nil
}

// TouchSession sets the session's last-seen time.
func (s *AccountStore) TouchSession(ctx context.Context, tokenHash string, lastSeen time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at = $1 WHERE token_hash = $2`, lastSeen, tokenHash)
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	if n, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("touch session: %w", err)
	} else if n == 0 {
		return repository.ErrSessionNotFound
	}
	return nil
}

// DeleteSession removes the session with tokenHash, if there is one.
func (s *AccountStore) DeleteSession(ctx context.Context, tokenHash string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// The cleanup deletes, each served by an index created in NewAccountStore.
const (
	deleteExpiredSessionsSQL = `DELETE FROM sessions WHERE last_seen_at < $1 OR created_at < $2`
	deleteOldAuthAuditSQL    = `DELETE FROM auth_audit WHERE at < $1`
)

// DeleteExpiredSessions removes every session idle since before idleBefore or
// created before createdBefore.
func (s *AccountStore) DeleteExpiredSessions(ctx context.Context, idleBefore, createdBefore time.Time) (int, error) {
	result, err := s.db.ExecContext(ctx, deleteExpiredSessionsSQL, idleBefore, createdBefore)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return int(n), nil
}

// DeleteUserSessions removes every session of userID but keepTokenHash.
func (s *AccountStore) DeleteUserSessions(ctx context.Context, userID uuid.UUID, keepTokenHash string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1 AND token_hash <> $2`,
		userID, keepTokenHash); err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}
	return nil
}

// ListMembers returns every member of treeID, by username. COLLATE "C" sorts
// byte for byte, as SQLite does, rather than by the database locale.
func (s *AccountStore) ListMembers(ctx context.Context, treeID uuid.UUID) ([]repository.Member, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.username, u.display_name, u.password_hash, u.created_at, u.disabled, m.role
		FROM users u JOIN tree_memberships m ON m.user_id = u.id
		WHERE m.tree_id = $1 ORDER BY u.username COLLATE "C"`, treeID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()
	members := []repository.Member{}
	for rows.Next() {
		var (
			m    repository.Member
			role string
		)
		if err := rows.Scan(&m.User.ID, &m.User.Username, &m.User.DisplayName, &m.User.PasswordHash,
			&m.User.CreatedAt, &m.User.Disabled, &role); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		m.Role = repository.Role(role)
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	return members, nil
}

// ownersLockKey is the transaction-scoped advisory lock (pg_advisory_xact_lock)
// every guarded owner change takes first. Under READ COMMITTED two
// transactions demoting or disabling two different owners would each still
// see the other's owner as active and both commit, leaving none; under the
// lock the second sees the first's committed change. One key for all trees
// keeps it simple and deadlock-free: owner changes are rare. The value is
// arbitrary ("owners" in ASCII).
const ownersLockKey int64 = 0x6f776e657273

// SetMemberRole changes the role userID holds in treeID, unless that demotes
// the tree's last active owner.
func (s *AccountStore) SetMemberRole(ctx context.Context, treeID, userID uuid.UUID, role repository.Role) error {
	return s.guardedUpdate(ctx, repository.ErrNotMember,
		`UPDATE tree_memberships SET role = $3 WHERE tree_id = $1 AND user_id = $2
		AND (role <> 'owner' OR $3 = 'owner' OR (SELECT disabled FROM users WHERE id = $2)
			OR EXISTS (SELECT 1 FROM tree_memberships o JOIN users u ON u.id = o.user_id
				WHERE o.tree_id = $1 AND o.user_id <> $2 AND o.role = 'owner' AND NOT u.disabled))`,
		[]any{treeID, userID, string(role)},
		`SELECT 1 FROM tree_memberships WHERE tree_id = $1 AND user_id = $2`, treeID, userID)
}

// UpdateUser stores user's display name, password hash and disabled flag,
// unless that disables the last active owner of a tree.
func (s *AccountStore) UpdateUser(ctx context.Context, user *repository.User) error {
	return s.guardedUpdate(ctx, repository.ErrUserNotFound,
		`UPDATE users SET display_name = $2, password_hash = $3, disabled = $4 WHERE id = $1
		AND (NOT $4 OR disabled OR NOT EXISTS (SELECT 1 FROM tree_memberships m
			WHERE m.user_id = $1 AND m.role = 'owner'
			AND NOT EXISTS (SELECT 1 FROM tree_memberships o JOIN users u ON u.id = o.user_id
				WHERE o.tree_id = m.tree_id AND o.user_id <> $1 AND o.role = 'owner' AND NOT u.disabled)))`,
		[]any{user.ID, user.DisplayName, user.PasswordHash, user.Disabled},
		`SELECT 1 FROM users WHERE id = $1`, user.ID)
}

// guardedUpdate runs update, an UPDATE of one row whose WHERE clause carries
// a last-owner guard, under ownersLockKey. When it changes no row it returns
// ErrLastOwner if the row the exists query looks for is there (so the guard
// refused it), else notFound.
func (s *AccountStore) guardedUpdate(ctx context.Context, notFound error, update string, args []any, exists string, existsArgs ...any) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin owner change: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, ownersLockKey); err != nil {
		return fmt.Errorf("lock owners: %w", err)
	}
	result, err := tx.ExecContext(ctx, update, args...)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	if err := requireRow(result, notFound); errors.Is(err, notFound) {
		var one int
		switch err := tx.QueryRowContext(ctx, exists, existsArgs...).Scan(&one); {
		case errors.Is(err, sql.ErrNoRows):
			return notFound
		case err != nil:
			return fmt.Errorf("check row: %w", err)
		}
		return repository.ErrLastOwner
	} else if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit owner change: %w", err)
	}
	return nil
}

// requireRow returns notFound when result changed no row.
func requireRow(result sql.Result, notFound error) error {
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return notFound
	}
	return nil
}

// CreateInvite stores invite.
func (s *AccountStore) CreateInvite(ctx context.Context, invite *repository.Invite) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO invites (token_hash, tree_id, role, created_by, created_at, expires_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		invite.TokenHash, invite.TreeID, string(invite.Role), invite.CreatedBy, invite.CreatedAt, invite.ExpiresAt); err != nil {
		return fmt.Errorf("insert invite: %w", err)
	}
	return nil
}

// AcceptInvite uses up the invite with tokenHash and creates user from it.
func (s *AccountStore) AcceptInvite(ctx context.Context, tokenHash string, now time.Time, user *repository.User) (*repository.Invite, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin accept invite: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	invite, err := scanInvite(tx.QueryRowContext(ctx,
		`DELETE FROM invites WHERE token_hash = $1 AND expires_at > $2 RETURNING `+inviteColumns,
		tokenHash, now))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrLinkNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("use invite: %w", err)
	}
	if err := createUser(ctx, tx, user, invite.TreeID, invite.Role); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit accept invite: %w", err)
	}
	return invite, nil
}

// apiTokenColumns are the columns scanAPIToken reads, in its order.
const apiTokenColumns = `id, token_hash, user_id, name, created_at, last_used_at` // #nosec G101 -- column names, not a credential.

// scanAPIToken reads an api_tokens row of apiTokenColumns.
func scanAPIToken(row interface{ Scan(...any) error }) (*repository.APIToken, error) {
	var token repository.APIToken
	var lastUsed sql.NullTime
	if err := row.Scan(&token.ID, &token.TokenHash, &token.UserID, &token.Name, &token.CreatedAt, &lastUsed); err != nil {
		return nil, err
	}
	if lastUsed.Valid {
		token.LastUsedAt = &lastUsed.Time
	}
	return &token, nil
}

// CreateAPIToken stores token.
func (s *AccountStore) CreateAPIToken(ctx context.Context, token *repository.APIToken) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO api_tokens (`+apiTokenColumns+`) VALUES ($1, $2, $3, $4, $5, $6)`,
		token.ID, token.TokenHash, token.UserID, token.Name, token.CreatedAt, token.LastUsedAt); err != nil {
		return fmt.Errorf("insert API token: %w", err)
	}
	return nil
}

// GetAPIToken returns the API token with tokenHash.
func (s *AccountStore) GetAPIToken(ctx context.Context, tokenHash string) (*repository.APIToken, error) {
	token, err := scanAPIToken(s.db.QueryRowContext(ctx,
		`SELECT `+apiTokenColumns+` FROM api_tokens WHERE token_hash = $1`, tokenHash))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrAPITokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get API token: %w", err)
	}
	return token, nil
}

// ListAPITokens returns userID's API tokens, newest first.
func (s *AccountStore) ListAPITokens(ctx context.Context, userID uuid.UUID) ([]repository.APIToken, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+apiTokenColumns+` FROM api_tokens WHERE user_id = $1 ORDER BY created_at DESC, id::text DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list API tokens: %w", err)
	}
	defer rows.Close()
	var tokens []repository.APIToken
	for rows.Next() {
		token, err := scanAPIToken(rows)
		if err != nil {
			return nil, fmt.Errorf("scan API token: %w", err)
		}
		tokens = append(tokens, *token)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list API tokens: %w", err)
	}
	return tokens, nil
}

// TouchAPIToken sets the token's last-used time.
func (s *AccountStore) TouchAPIToken(ctx context.Context, tokenHash string, lastUsed time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = $1 WHERE token_hash = $2`, lastUsed, tokenHash)
	if err != nil {
		return fmt.Errorf("touch API token: %w", err)
	}
	if n, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("touch API token: %w", err)
	} else if n == 0 {
		return repository.ErrAPITokenNotFound
	}
	return nil
}

// DeleteAPIToken removes userID's API token with id.
func (s *AccountStore) DeleteAPIToken(ctx context.Context, userID, id uuid.UUID) (*repository.APIToken, error) {
	token, err := scanAPIToken(s.db.QueryRowContext(ctx,
		`DELETE FROM api_tokens WHERE id = $1 AND user_id = $2 RETURNING `+apiTokenColumns, id, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrAPITokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("delete API token: %w", err)
	}
	return token, nil
}

// inviteColumns are the columns scanInvite reads, in its order.
const inviteColumns = `token_hash, tree_id, role, created_by, created_at, expires_at`

// scanInvite reads an invite row of inviteColumns.
func scanInvite(row interface{ Scan(...any) error }) (*repository.Invite, error) {
	var invite repository.Invite
	var role string
	if err := row.Scan(&invite.TokenHash, &invite.TreeID, &role, &invite.CreatedBy, &invite.CreatedAt, &invite.ExpiresAt); err != nil {
		return nil, err
	}
	invite.Role = repository.Role(role)
	return &invite, nil
}

// ListInvites returns the invites to treeID that expire after now, newest
// first.
func (s *AccountStore) ListInvites(ctx context.Context, treeID uuid.UUID, now time.Time) ([]repository.Invite, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+inviteColumns+` FROM invites WHERE tree_id = $1 AND expires_at > $2 ORDER BY created_at DESC, token_hash`,
		treeID, now)
	if err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}
	defer rows.Close()
	invites := []repository.Invite{}
	for rows.Next() {
		invite, err := scanInvite(rows)
		if err != nil {
			return nil, fmt.Errorf("scan invite: %w", err)
		}
		invites = append(invites, *invite)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}
	return invites, nil
}

// RevokeInvite removes the invite with tokenHash, if it expires after now.
func (s *AccountStore) RevokeInvite(ctx context.Context, tokenHash string, now time.Time) (*repository.Invite, error) {
	invite, err := scanInvite(s.db.QueryRowContext(ctx,
		`DELETE FROM invites WHERE token_hash = $1 AND expires_at > $2 RETURNING `+inviteColumns,
		tokenHash, now))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrLinkNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("revoke invite: %w", err)
	}
	return invite, nil
}

// nullableID stores uuid.Nil as NULL.
func nullableID(id uuid.UUID) uuid.NullUUID {
	return uuid.NullUUID{UUID: id, Valid: id != uuid.Nil}
}

// DeleteExpiredLinks removes every invite and password reset that expires at
// or before now.
func (s *AccountStore) DeleteExpiredLinks(ctx context.Context, now time.Time) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin delete expired links: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	removed := 0
	for _, query := range []string{
		`DELETE FROM invites WHERE expires_at <= $1`,
		`DELETE FROM password_resets WHERE expires_at <= $1`,
	} {
		result, err := tx.ExecContext(ctx, query, now)
		if err != nil {
			return 0, fmt.Errorf("delete expired links: %w", err)
		}
		n, err := result.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("delete expired links: %w", err)
		}
		removed += int(n)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit delete expired links: %w", err)
	}
	return removed, nil
}

// CreatePasswordReset stores reset.
func (s *AccountStore) CreatePasswordReset(ctx context.Context, reset *repository.PasswordReset) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO password_resets (token_hash, user_id, created_by, created_at, expires_at) VALUES ($1, $2, $3, $4, $5)`,
		reset.TokenHash, reset.UserID, nullableID(reset.CreatedBy), reset.CreatedAt, reset.ExpiresAt); err != nil {
		return fmt.Errorf("insert password reset: %w", err)
	}
	return nil
}

// DeleteLinksIssuedBy removes every invite and password reset userID issued.
func (s *AccountStore) DeleteLinksIssuedBy(ctx context.Context, userID uuid.UUID) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete links: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, query := range []string{
		`DELETE FROM invites WHERE created_by = $1`,
		`DELETE FROM password_resets WHERE created_by = $1`,
	} {
		if _, err := tx.ExecContext(ctx, query, userID); err != nil {
			return fmt.Errorf("delete links: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete links: %w", err)
	}
	return nil
}

// UsePasswordReset uses up the reset with tokenHash and sets the password.
func (s *AccountStore) UsePasswordReset(ctx context.Context, tokenHash string, now time.Time, passwordHash string) (uuid.UUID, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin password reset: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var userID uuid.UUID
	err = tx.QueryRowContext(ctx,
		`DELETE FROM password_resets WHERE token_hash = $1 AND expires_at > $2 RETURNING user_id`,
		tokenHash, now).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, repository.ErrLinkNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("use password reset: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2`, passwordHash, userID); err != nil {
		return uuid.Nil, fmt.Errorf("set password: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM password_resets WHERE user_id = $1`, userID); err != nil {
		return uuid.Nil, fmt.Errorf("delete password resets: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID); err != nil {
		return uuid.Nil, fmt.Errorf("delete sessions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return uuid.Nil, fmt.Errorf("commit password reset: %w", err)
	}
	return userID, nil
}

// resetColumns are the columns scanPasswordReset reads, in its order.
const resetColumns = `token_hash, user_id, created_by, created_at, expires_at`

// scanPasswordReset reads a password reset row of resetColumns.
func scanPasswordReset(row interface{ Scan(...any) error }) (*repository.PasswordReset, error) {
	var reset repository.PasswordReset
	var createdBy uuid.NullUUID
	if err := row.Scan(&reset.TokenHash, &reset.UserID, &createdBy, &reset.CreatedAt, &reset.ExpiresAt); err != nil {
		return nil, err
	}
	reset.CreatedBy = createdBy.UUID
	return &reset, nil
}

// ListPasswordResets returns the password resets that expire after now,
// newest first.
func (s *AccountStore) ListPasswordResets(ctx context.Context, now time.Time) ([]repository.PasswordReset, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+resetColumns+` FROM password_resets WHERE expires_at > $1 ORDER BY created_at DESC, token_hash`, now)
	if err != nil {
		return nil, fmt.Errorf("list password resets: %w", err)
	}
	defer rows.Close()
	resets := []repository.PasswordReset{}
	for rows.Next() {
		reset, err := scanPasswordReset(rows)
		if err != nil {
			return nil, fmt.Errorf("scan password reset: %w", err)
		}
		resets = append(resets, *reset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list password resets: %w", err)
	}
	return resets, nil
}

// RevokePasswordReset removes the reset with tokenHash, if it expires after
// now.
func (s *AccountStore) RevokePasswordReset(ctx context.Context, tokenHash string, now time.Time) (*repository.PasswordReset, error) {
	reset, err := scanPasswordReset(s.db.QueryRowContext(ctx,
		`DELETE FROM password_resets WHERE token_hash = $1 AND expires_at > $2 RETURNING `+resetColumns,
		tokenHash, now))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrLinkNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("revoke password reset: %w", err)
	}
	return reset, nil
}

// AppendAuthAudit adds entry to the auth audit log.
func (s *AccountStore) AppendAuthAudit(ctx context.Context, entry *repository.AuthAuditEntry) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO auth_audit (at, action, actor_id, subject_id, username, detail, client_ip) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		entry.At, string(entry.Action), nullableID(entry.ActorID), nullableID(entry.SubjectID),
		entry.Username, entry.Detail, entry.ClientIP); err != nil {
		return fmt.Errorf("append auth audit: %w", err)
	}
	return nil
}

// ListAuthAudit returns up to limit entries with an ID below before (any, for
// 0), newest first. It walks the primary key, so every page costs the same.
func (s *AccountStore) ListAuthAudit(ctx context.Context, limit int, before int64) ([]repository.AuthAuditEntry, error) {
	if before == 0 {
		before = math.MaxInt64 // one plan for every page: id < before
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, at, action, actor_id, subject_id, username, detail, client_ip
		FROM auth_audit WHERE id < $1 ORDER BY id DESC LIMIT $2`, before, limit)
	if err != nil {
		return nil, fmt.Errorf("list auth audit: %w", err)
	}
	defer rows.Close()
	entries := []repository.AuthAuditEntry{}
	for rows.Next() {
		var (
			e                  repository.AuthAuditEntry
			action             string
			actorID, subjectID uuid.NullUUID
		)
		if err := rows.Scan(&e.ID, &e.At, &action, &actorID, &subjectID, &e.Username, &e.Detail, &e.ClientIP); err != nil {
			return nil, fmt.Errorf("scan auth audit: %w", err)
		}
		e.Action, e.ActorID, e.SubjectID = repository.AuthAction(action), actorID.UUID, subjectID.UUID
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list auth audit: %w", err)
	}
	return entries, nil
}

// DeleteAuthAuditBefore removes every audit entry recorded before before.
func (s *AccountStore) DeleteAuthAuditBefore(ctx context.Context, before time.Time) (int, error) {
	result, err := s.db.ExecContext(ctx, deleteOldAuthAuditSQL, before)
	if err != nil {
		return 0, fmt.Errorf("delete old auth audit: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("delete old auth audit: %w", err)
	}
	return int(n), nil
}

// GetTreeSettings returns treeID's settings, or the defaults when none were
// saved.
func (s *AccountStore) GetTreeSettings(ctx context.Context, treeID uuid.UUID) (repository.TreeSettings, error) {
	var settings repository.TreeSettings
	err := s.db.QueryRowContext(ctx, `
		SELECT living_threshold_years, viewers_see_living, anonymous_access FROM tree_settings WHERE tree_id = $1`, treeID).
		Scan(&settings.LivingThresholdYears, &settings.ViewersSeeLiving, &settings.AnonymousAccess)
	if errors.Is(err, sql.ErrNoRows) {
		return repository.DefaultTreeSettings(), nil
	}
	if err != nil {
		return repository.TreeSettings{}, fmt.Errorf("get tree settings: %w", err)
	}
	return settings, nil
}

// SaveTreeSettings stores treeID's settings, replacing any saved before.
func (s *AccountStore) SaveTreeSettings(ctx context.Context, treeID uuid.UUID, settings repository.TreeSettings) error {
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO tree_settings (tree_id, living_threshold_years, viewers_see_living, anonymous_access) VALUES ($1, $2, $3, $4)
		ON CONFLICT (tree_id) DO UPDATE SET living_threshold_years = EXCLUDED.living_threshold_years,
			viewers_see_living = EXCLUDED.viewers_see_living, anonymous_access = EXCLUDED.anonymous_access`,
		treeID, settings.LivingThresholdYears, settings.ViewersSeeLiving, settings.AnonymousAccess); err != nil {
		return fmt.Errorf("save tree settings: %w", err)
	}
	return nil
}
