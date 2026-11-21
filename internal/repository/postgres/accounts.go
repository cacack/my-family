package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

// NewAccountStore creates the account tables if needed and returns the store.
func NewAccountStore(db *sql.DB) (*AccountStore, error) {
	store := &AccountStore{db: db}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			disabled BOOLEAN NOT NULL DEFAULT FALSE
		);

		CREATE TABLE IF NOT EXISTS tree_memberships (
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			tree_id UUID NOT NULL,
			role TEXT NOT NULL,
			PRIMARY KEY (user_id, tree_id)
		);

		CREATE TABLE IF NOT EXISTS sessions (
			token_hash TEXT PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL,
			last_seen_at TIMESTAMPTZ NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);

		CREATE TABLE IF NOT EXISTS invites (
			token_hash TEXT PRIMARY KEY,
			tree_id UUID NOT NULL,
			role TEXT NOT NULL,
			created_by UUID NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL
		);

		CREATE TABLE IF NOT EXISTS password_resets (
			token_hash TEXT PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_by UUID,
			created_at TIMESTAMPTZ NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL
		);

		CREATE TABLE IF NOT EXISTS auth_audit (
			id BIGSERIAL PRIMARY KEY,
			at TIMESTAMPTZ NOT NULL,
			action TEXT NOT NULL,
			actor_id UUID,
			subject_id UUID,
			username TEXT NOT NULL,
			detail TEXT NOT NULL,
			client_ip TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS tree_settings (
			tree_id UUID PRIMARY KEY,
			living_threshold_years INTEGER NOT NULL,
			viewers_see_living BOOLEAN NOT NULL,
			anonymous_access TEXT NOT NULL
		);
	`); err != nil {
		return nil, fmt.Errorf("create account tables: %w", err)
	}
	return store, nil
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

// DeleteExpiredSessions removes every session idle since before idleBefore or
// created before createdBefore.
func (s *AccountStore) DeleteExpiredSessions(ctx context.Context, idleBefore, createdBefore time.Time) (int, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE last_seen_at < $1 OR created_at < $2`, idleBefore, createdBefore)
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

// SetMemberRole changes the role userID holds in treeID.
func (s *AccountStore) SetMemberRole(ctx context.Context, treeID, userID uuid.UUID, role repository.Role) error {
	result, err := s.db.ExecContext(ctx, `UPDATE tree_memberships SET role = $1 WHERE tree_id = $2 AND user_id = $3`,
		string(role), treeID, userID)
	if err != nil {
		return fmt.Errorf("set member role: %w", err)
	}
	return requireRow(result, repository.ErrNotMember)
}

// UpdateUser stores user's display name, password hash and disabled flag.
func (s *AccountStore) UpdateUser(ctx context.Context, user *repository.User) error {
	result, err := s.db.ExecContext(ctx, `UPDATE users SET display_name = $1, password_hash = $2, disabled = $3 WHERE id = $4`,
		user.DisplayName, user.PasswordHash, user.Disabled, user.ID)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return requireRow(result, repository.ErrUserNotFound)
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

// ListAuthAudit returns up to limit entries, newest first, after offset.
func (s *AccountStore) ListAuthAudit(ctx context.Context, limit, offset int) ([]repository.AuthAuditEntry, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM auth_audit`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count auth audit: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT at, action, actor_id, subject_id, username, detail, client_ip
		FROM auth_audit ORDER BY id DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list auth audit: %w", err)
	}
	defer rows.Close()
	entries := []repository.AuthAuditEntry{}
	for rows.Next() {
		var (
			e                  repository.AuthAuditEntry
			action             string
			actorID, subjectID uuid.NullUUID
		)
		if err := rows.Scan(&e.At, &action, &actorID, &subjectID, &e.Username, &e.Detail, &e.ClientIP); err != nil {
			return nil, 0, fmt.Errorf("scan auth audit: %w", err)
		}
		e.Action, e.ActorID, e.SubjectID = repository.AuthAction(action), actorID.UUID, subjectID.UUID
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list auth audit: %w", err)
	}
	return entries, total, nil
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
