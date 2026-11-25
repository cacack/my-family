package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	sqlitedriver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/cacack/my-family/internal/repository"
)

// Compile-time assertion that AccountStore satisfies the interface.
var _ repository.AccountStore = (*AccountStore)(nil)

// sessionTimeLayout stores session times at a fixed width in UTC, so the
// expiry sweep can compare them as text.
const sessionTimeLayout = "2006-01-02T15:04:05.000000000Z"

// AccountStore is a SQLite implementation of repository.AccountStore. Its
// tables are neither event log nor read model: `myfamily rebuild` never
// drops them (ADR-007).
type AccountStore struct {
	db *sql.DB
}

// NewAccountStore returns the store, first bringing the store schema, which
// owns the account tables, up to date (ADR-011). That runs every store
// migration, the event log's included, so it fails as NewEventStore does: with
// repository.ErrStoreSchemaNewer on a database stamped newer than this binary,
// and with ErrReadModelEventsTable on a pre-#733 database whose read model
// store has not been constructed first (DB-007).
func NewAccountStore(db *sql.DB) (*AccountStore, error) {
	if err := migrateStoreSchema(db); err != nil {
		return nil, fmt.Errorf("migrate store schema: %w", err)
	}
	return &AccountStore{db: db}, nil
}

func sessionTime(t time.Time) string {
	return t.UTC().Format(sessionTimeLayout)
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
	// The UNIQUE constraint on users.username is the only username check, as
	// on PostgreSQL: a separate pre-check would race a concurrent create.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO users (id, username, display_name, password_hash, created_at, disabled) VALUES (?, ?, ?, ?, ?, ?)`,
		user.ID.String(), user.Username, user.DisplayName, user.PasswordHash, formatTimestamp(user.CreatedAt), user.Disabled); err != nil {
		var sqliteErr *sqlitedriver.Error
		if errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
			return repository.ErrUsernameTaken
		}
		return fmt.Errorf("insert user: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO tree_memberships (user_id, tree_id, role) VALUES (?, ?, ?)`,
		user.ID.String(), treeID.String(), string(role)); err != nil {
		return fmt.Errorf("insert membership: %w", err)
	}
	return nil
}

func (s *AccountStore) getUser(ctx context.Context, where string, arg any) (*repository.User, error) {
	var (
		user              repository.User
		idStr, createdStr string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, display_name, password_hash, created_at, disabled FROM users WHERE `+where+` = ?`, arg,
	).Scan(&idStr, &user.Username, &user.DisplayName, &user.PasswordHash, &createdStr, &user.Disabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if user.ID, err = uuid.Parse(idStr); err != nil {
		return nil, fmt.Errorf("parse user id: %w", err)
	}
	if user.CreatedAt, err = parseTimestamp(createdStr); err != nil {
		return nil, fmt.Errorf("parse user created_at: %w", err)
	}
	return &user, nil
}

// GetUser returns the user with id.
func (s *AccountStore) GetUser(ctx context.Context, id uuid.UUID) (*repository.User, error) {
	return s.getUser(ctx, "id", id.String())
}

// GetUserByUsername returns the user with username.
func (s *AccountStore) GetUserByUsername(ctx context.Context, username string) (*repository.User, error) {
	return s.getUser(ctx, "username", username)
}

// MemberRole returns the role userID holds in treeID.
func (s *AccountStore) MemberRole(ctx context.Context, treeID, userID uuid.UUID) (repository.Role, error) {
	var role string
	err := s.db.QueryRowContext(ctx,
		`SELECT role FROM tree_memberships WHERE tree_id = ? AND user_id = ?`, treeID.String(), userID.String(),
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
		`SELECT COUNT(*) FROM tree_memberships WHERE tree_id = ? AND role = ?`, treeID.String(), string(role),
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
	// One JSON-array parameter, as the batched read-model lookups do, so no
	// bind-variable limit applies however many ids a page names.
	idsJSON, err := json.Marshal(strs)
	if err != nil {
		return nil, fmt.Errorf("encode user ids: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, display_name FROM users WHERE id IN (SELECT value FROM json_each(?))`, string(idsJSON))
	if err != nil {
		return nil, fmt.Errorf("list display names: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var idStr, name string
		if err := rows.Scan(&idStr, &name); err != nil {
			return nil, fmt.Errorf("scan display name: %w", err)
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return nil, fmt.Errorf("parse user id: %w", err)
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
		`INSERT INTO sessions (token_hash, user_id, created_at, last_seen_at) VALUES (?, ?, ?, ?)`,
		session.TokenHash, session.UserID.String(), sessionTime(session.CreatedAt), sessionTime(session.LastSeenAt)); err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// GetSession returns the session with tokenHash.
func (s *AccountStore) GetSession(ctx context.Context, tokenHash string) (*repository.Session, error) {
	var (
		session                            repository.Session
		userIDStr, createdStr, lastSeenStr string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT token_hash, user_id, created_at, last_seen_at FROM sessions WHERE token_hash = ?`, tokenHash,
	).Scan(&session.TokenHash, &userIDStr, &createdStr, &lastSeenStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if session.UserID, err = uuid.Parse(userIDStr); err != nil {
		return nil, fmt.Errorf("parse session user id: %w", err)
	}
	if session.CreatedAt, err = parseTimestamp(createdStr); err != nil {
		return nil, fmt.Errorf("parse session created_at: %w", err)
	}
	if session.LastSeenAt, err = parseTimestamp(lastSeenStr); err != nil {
		return nil, fmt.Errorf("parse session last_seen_at: %w", err)
	}
	return &session, nil
}

// TouchSession sets the session's last-seen time.
func (s *AccountStore) TouchSession(ctx context.Context, tokenHash string, lastSeen time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ? WHERE token_hash = ?`, sessionTime(lastSeen), tokenHash)
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
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// The cleanup deletes, each served by an index created in NewAccountStore.
const (
	deleteExpiredSessionsSQL = `DELETE FROM sessions WHERE last_seen_at < ? OR created_at < ?`
	deleteOldAuthAuditSQL    = `DELETE FROM auth_audit WHERE at < ?`
)

// DeleteExpiredSessions removes every session idle since before idleBefore or
// created before createdBefore.
func (s *AccountStore) DeleteExpiredSessions(ctx context.Context, idleBefore, createdBefore time.Time) (int, error) {
	result, err := s.db.ExecContext(ctx, deleteExpiredSessionsSQL,
		sessionTime(idleBefore), sessionTime(createdBefore))
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
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND token_hash <> ?`,
		userID.String(), keepTokenHash); err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}
	return nil
}

// apiTokenColumns are the columns scanAPIToken reads, in order.
const apiTokenColumns = `id, token_hash, user_id, name, created_at, last_used_at` // #nosec G101 -- column names, not a credential.

// scanAPIToken reads one api_tokens row selected as apiTokenColumns.
func scanAPIToken(row interface{ Scan(...any) error }) (*repository.APIToken, error) {
	var (
		token                        repository.APIToken
		idStr, userIDStr, createdStr string
		lastUsed                     sql.NullString
	)
	if err := row.Scan(&idStr, &token.TokenHash, &userIDStr, &token.Name, &createdStr, &lastUsed); err != nil {
		return nil, err
	}
	var err error
	if token.ID, err = uuid.Parse(idStr); err != nil {
		return nil, fmt.Errorf("parse API token id: %w", err)
	}
	if token.UserID, err = uuid.Parse(userIDStr); err != nil {
		return nil, fmt.Errorf("parse API token user id: %w", err)
	}
	if token.CreatedAt, err = parseTimestamp(createdStr); err != nil {
		return nil, fmt.Errorf("parse API token created_at: %w", err)
	}
	if lastUsed.Valid {
		t, err := parseTimestamp(lastUsed.String)
		if err != nil {
			return nil, fmt.Errorf("parse API token last_used_at: %w", err)
		}
		token.LastUsedAt = &t
	}
	return &token, nil
}

// CreateAPIToken stores token.
func (s *AccountStore) CreateAPIToken(ctx context.Context, token *repository.APIToken) error {
	var lastUsed sql.NullString
	if token.LastUsedAt != nil {
		lastUsed = sql.NullString{String: sessionTime(*token.LastUsedAt), Valid: true}
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO api_tokens (`+apiTokenColumns+`) VALUES (?, ?, ?, ?, ?, ?)`,
		token.ID.String(), token.TokenHash, token.UserID.String(), token.Name, sessionTime(token.CreatedAt), lastUsed); err != nil {
		return fmt.Errorf("insert API token: %w", err)
	}
	return nil
}

// GetAPIToken returns the API token with tokenHash.
func (s *AccountStore) GetAPIToken(ctx context.Context, tokenHash string) (*repository.APIToken, error) {
	token, err := scanAPIToken(s.db.QueryRowContext(ctx,
		`SELECT `+apiTokenColumns+` FROM api_tokens WHERE token_hash = ?`, tokenHash))
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
		`SELECT `+apiTokenColumns+` FROM api_tokens WHERE user_id = ? ORDER BY created_at DESC, id DESC`, userID.String())
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
	result, err := s.db.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = ? WHERE token_hash = ?`, sessionTime(lastUsed), tokenHash)
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
		`DELETE FROM api_tokens WHERE id = ? AND user_id = ? RETURNING `+apiTokenColumns, id.String(), userID.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrAPITokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("delete API token: %w", err)
	}
	return token, nil
}

// ListMembers returns every member of treeID, by username.
func (s *AccountStore) ListMembers(ctx context.Context, treeID uuid.UUID) ([]repository.Member, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.username, u.display_name, u.password_hash, u.created_at, u.disabled, m.role
		FROM users u JOIN tree_memberships m ON m.user_id = u.id
		WHERE m.tree_id = ? ORDER BY u.username`, treeID.String())
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()
	members := []repository.Member{}
	for rows.Next() {
		var (
			m                       repository.Member
			idStr, createdStr, role string
		)
		if err := rows.Scan(&idStr, &m.User.Username, &m.User.DisplayName, &m.User.PasswordHash, &createdStr, &m.User.Disabled, &role); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		if m.User.ID, err = uuid.Parse(idStr); err != nil {
			return nil, fmt.Errorf("parse user id: %w", err)
		}
		if m.User.CreatedAt, err = parseTimestamp(createdStr); err != nil {
			return nil, fmt.Errorf("parse user created_at: %w", err)
		}
		m.Role = repository.Role(role)
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	return members, nil
}

// The last-owner guards below are part of the UPDATE they guard: SQLite
// takes the write lock before a write statement reads, so no other writer, in
// this process or another, can change the owners between the check and the
// write. Each guard's EXISTS asks whether the tree has another active owner.

// SetMemberRole changes the role userID holds in treeID, unless that demotes
// the tree's last active owner.
func (s *AccountStore) SetMemberRole(ctx context.Context, treeID, userID uuid.UUID, role repository.Role) error {
	result, err := s.db.ExecContext(ctx, `UPDATE tree_memberships SET role = ?3 WHERE tree_id = ?1 AND user_id = ?2
		AND (role <> 'owner' OR ?3 = 'owner' OR (SELECT disabled FROM users WHERE id = ?2)
			OR EXISTS (SELECT 1 FROM tree_memberships o JOIN users u ON u.id = o.user_id
				WHERE o.tree_id = ?1 AND o.user_id <> ?2 AND o.role = 'owner' AND NOT u.disabled))`,
		treeID.String(), userID.String(), string(role))
	if err != nil {
		return fmt.Errorf("set member role: %w", err)
	}
	return s.refusedOr(ctx, result, repository.ErrNotMember,
		`SELECT 1 FROM tree_memberships WHERE tree_id = ? AND user_id = ?`, treeID.String(), userID.String())
}

// UpdateUser stores user's display name, password hash and disabled flag,
// unless that disables the last active owner of a tree.
func (s *AccountStore) UpdateUser(ctx context.Context, user *repository.User) error {
	result, err := s.db.ExecContext(ctx, `UPDATE users SET display_name = ?2, password_hash = ?3, disabled = ?4 WHERE id = ?1
		AND (NOT ?4 OR disabled OR NOT EXISTS (SELECT 1 FROM tree_memberships m
			WHERE m.user_id = ?1 AND m.role = 'owner'
			AND NOT EXISTS (SELECT 1 FROM tree_memberships o JOIN users u ON u.id = o.user_id
				WHERE o.tree_id = m.tree_id AND o.user_id <> ?1 AND o.role = 'owner' AND NOT u.disabled)))`,
		user.ID.String(), user.DisplayName, user.PasswordHash, user.Disabled)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return s.refusedOr(ctx, result, repository.ErrUserNotFound, `SELECT 1 FROM users WHERE id = ?`, user.ID.String())
}

// refusedOr explains a guarded UPDATE's result: nil when it changed a row,
// ErrLastOwner when the row the exists query looks for is there (so the
// last-owner guard refused it), else notFound.
func (s *AccountStore) refusedOr(ctx context.Context, result sql.Result, notFound error, exists string, args ...any) error {
	err := requireRow(result, notFound)
	if !errors.Is(err, notFound) {
		return err
	}
	var one int
	switch err := s.db.QueryRowContext(ctx, exists, args...).Scan(&one); {
	case errors.Is(err, sql.ErrNoRows):
		return notFound
	case err != nil:
		return fmt.Errorf("check row: %w", err)
	}
	return repository.ErrLastOwner
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
		`INSERT INTO invites (token_hash, tree_id, role, created_by, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		invite.TokenHash, invite.TreeID.String(), string(invite.Role), invite.CreatedBy.String(),
		sessionTime(invite.CreatedAt), sessionTime(invite.ExpiresAt)); err != nil {
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
		`DELETE FROM invites WHERE token_hash = ? AND expires_at > ? RETURNING `+inviteColumns,
		tokenHash, sessionTime(now)))
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

// inviteColumns are the columns scanInvite reads, in its order.
const inviteColumns = `token_hash, tree_id, role, created_by, created_at, expires_at`

// scanInvite reads an invite row of inviteColumns.
func scanInvite(row interface{ Scan(...any) error }) (*repository.Invite, error) {
	var invite repository.Invite
	var treeStr, role, createdByStr, createdStr, expiresStr string
	if err := row.Scan(&invite.TokenHash, &treeStr, &role, &createdByStr, &createdStr, &expiresStr); err != nil {
		return nil, err
	}
	invite.Role = repository.Role(role)
	var err error
	if invite.TreeID, err = uuid.Parse(treeStr); err != nil {
		return nil, fmt.Errorf("parse invite tree id: %w", err)
	}
	if invite.CreatedBy, err = uuid.Parse(createdByStr); err != nil {
		return nil, fmt.Errorf("parse invite created_by: %w", err)
	}
	if invite.CreatedAt, err = parseTimestamp(createdStr); err != nil {
		return nil, fmt.Errorf("parse invite created_at: %w", err)
	}
	if invite.ExpiresAt, err = parseTimestamp(expiresStr); err != nil {
		return nil, fmt.Errorf("parse invite expires_at: %w", err)
	}
	return &invite, nil
}

// ListInvites returns the invites to treeID that expire after now, newest
// first.
func (s *AccountStore) ListInvites(ctx context.Context, treeID uuid.UUID, now time.Time) ([]repository.Invite, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+inviteColumns+` FROM invites WHERE tree_id = ? AND expires_at > ? ORDER BY created_at DESC, token_hash`,
		treeID.String(), sessionTime(now))
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
		`DELETE FROM invites WHERE token_hash = ? AND expires_at > ? RETURNING `+inviteColumns,
		tokenHash, sessionTime(now)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrLinkNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("revoke invite: %w", err)
	}
	return invite, nil
}

// DeleteLinksIssuedBy removes every invite and password reset userID issued.
func (s *AccountStore) DeleteLinksIssuedBy(ctx context.Context, userID uuid.UUID) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete links: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, query := range []string{
		`DELETE FROM invites WHERE created_by = ?`,
		`DELETE FROM password_resets WHERE created_by = ?`,
	} {
		if _, err := tx.ExecContext(ctx, query, userID.String()); err != nil {
			return fmt.Errorf("delete links: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete links: %w", err)
	}
	return nil
}

// nullableID stores uuid.Nil as NULL.
func nullableID(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id.String()
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
		`DELETE FROM invites WHERE expires_at <= ?`,
		`DELETE FROM password_resets WHERE expires_at <= ?`,
	} {
		result, err := tx.ExecContext(ctx, query, sessionTime(now))
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
		`INSERT INTO password_resets (token_hash, user_id, created_by, created_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		reset.TokenHash, reset.UserID.String(), nullableID(reset.CreatedBy),
		sessionTime(reset.CreatedAt), sessionTime(reset.ExpiresAt)); err != nil {
		return fmt.Errorf("insert password reset: %w", err)
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

	var userStr string
	err = tx.QueryRowContext(ctx,
		`SELECT user_id FROM password_resets WHERE token_hash = ? AND expires_at > ?`,
		tokenHash, sessionTime(now)).Scan(&userStr)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, repository.ErrLinkNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("get password reset: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, userStr); err != nil {
		return uuid.Nil, fmt.Errorf("set password: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM password_resets WHERE user_id = ?`, userStr); err != nil {
		return uuid.Nil, fmt.Errorf("delete password resets: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userStr); err != nil {
		return uuid.Nil, fmt.Errorf("delete sessions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return uuid.Nil, fmt.Errorf("commit password reset: %w", err)
	}
	userID, err := uuid.Parse(userStr)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse password reset user id: %w", err)
	}
	return userID, nil
}

// resetColumns are the columns scanPasswordReset reads, in its order.
const resetColumns = `token_hash, user_id, created_by, created_at, expires_at`

// scanPasswordReset reads a password reset row of resetColumns.
func scanPasswordReset(row interface{ Scan(...any) error }) (*repository.PasswordReset, error) {
	var reset repository.PasswordReset
	var userStr, createdStr, expiresStr string
	var createdBy sql.NullString
	if err := row.Scan(&reset.TokenHash, &userStr, &createdBy, &createdStr, &expiresStr); err != nil {
		return nil, err
	}
	var err error
	if reset.UserID, err = uuid.Parse(userStr); err != nil {
		return nil, fmt.Errorf("parse password reset user id: %w", err)
	}
	if createdBy.Valid {
		if reset.CreatedBy, err = uuid.Parse(createdBy.String); err != nil {
			return nil, fmt.Errorf("parse password reset created_by: %w", err)
		}
	}
	if reset.CreatedAt, err = parseTimestamp(createdStr); err != nil {
		return nil, fmt.Errorf("parse password reset created_at: %w", err)
	}
	if reset.ExpiresAt, err = parseTimestamp(expiresStr); err != nil {
		return nil, fmt.Errorf("parse password reset expires_at: %w", err)
	}
	return &reset, nil
}

// ListPasswordResets returns the password resets that expire after now,
// newest first.
func (s *AccountStore) ListPasswordResets(ctx context.Context, now time.Time) ([]repository.PasswordReset, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+resetColumns+` FROM password_resets WHERE expires_at > ? ORDER BY created_at DESC, token_hash`,
		sessionTime(now))
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
		`DELETE FROM password_resets WHERE token_hash = ? AND expires_at > ? RETURNING `+resetColumns,
		tokenHash, sessionTime(now)))
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
		`INSERT INTO auth_audit (at, action, actor_id, subject_id, username, detail, client_ip) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sessionTime(entry.At), string(entry.Action), nullableID(entry.ActorID), nullableID(entry.SubjectID),
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
		FROM auth_audit WHERE id < ? ORDER BY id DESC LIMIT ?`, before, limit)
	if err != nil {
		return nil, fmt.Errorf("list auth audit: %w", err)
	}
	defer rows.Close()
	entries := []repository.AuthAuditEntry{}
	for rows.Next() {
		var (
			e                  repository.AuthAuditEntry
			atStr, action      string
			actorID, subjectID sql.NullString
		)
		if err := rows.Scan(&e.ID, &atStr, &action, &actorID, &subjectID, &e.Username, &e.Detail, &e.ClientIP); err != nil {
			return nil, fmt.Errorf("scan auth audit: %w", err)
		}
		e.Action = repository.AuthAction(action)
		if e.At, err = parseTimestamp(atStr); err != nil {
			return nil, fmt.Errorf("parse auth audit at: %w", err)
		}
		if e.ActorID, err = parseNullableID(actorID); err != nil {
			return nil, fmt.Errorf("parse auth audit actor: %w", err)
		}
		if e.SubjectID, err = parseNullableID(subjectID); err != nil {
			return nil, fmt.Errorf("parse auth audit subject: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list auth audit: %w", err)
	}
	return entries, nil
}

// DeleteAuthAuditBefore removes every audit entry recorded before before.
func (s *AccountStore) DeleteAuthAuditBefore(ctx context.Context, before time.Time) (int, error) {
	result, err := s.db.ExecContext(ctx, deleteOldAuthAuditSQL, sessionTime(before))
	if err != nil {
		return 0, fmt.Errorf("delete old auth audit: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("delete old auth audit: %w", err)
	}
	return int(n), nil
}

func parseNullableID(s sql.NullString) (uuid.UUID, error) {
	if !s.Valid {
		return uuid.Nil, nil
	}
	return uuid.Parse(s.String)
}

// GetTreeSettings returns treeID's settings, or the defaults when none were
// saved.
func (s *AccountStore) GetTreeSettings(ctx context.Context, treeID uuid.UUID) (repository.TreeSettings, error) {
	var settings repository.TreeSettings
	err := s.db.QueryRowContext(ctx, `
		SELECT living_threshold_years, viewers_see_living, anonymous_access FROM tree_settings WHERE tree_id = ?`, treeID.String()).
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
		INSERT INTO tree_settings (tree_id, living_threshold_years, viewers_see_living, anonymous_access) VALUES (?, ?, ?, ?)
		ON CONFLICT (tree_id) DO UPDATE SET living_threshold_years = excluded.living_threshold_years,
			viewers_see_living = excluded.viewers_see_living, anonymous_access = excluded.anonymous_access`,
		treeID.String(), settings.LivingThresholdYears, settings.ViewersSeeLiving, settings.AnonymousAccess); err != nil {
		return fmt.Errorf("save tree settings: %w", err)
	}
	return nil
}
