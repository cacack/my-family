package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

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

// NewAccountStore creates the account tables if needed and returns the store.
func NewAccountStore(db *sql.DB) (*AccountStore, error) {
	store := &AccountStore{db: db}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			created_at TEXT NOT NULL,
			disabled INTEGER NOT NULL DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS tree_memberships (
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			tree_id TEXT NOT NULL,
			role TEXT NOT NULL,
			PRIMARY KEY (user_id, tree_id)
		);

		CREATE TABLE IF NOT EXISTS sessions (
			token_hash TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);

		CREATE TABLE IF NOT EXISTS invites (
			token_hash TEXT PRIMARY KEY,
			tree_id TEXT NOT NULL,
			role TEXT NOT NULL,
			created_by TEXT NOT NULL,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS password_resets (
			token_hash TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_by TEXT,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS auth_audit (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			at TEXT NOT NULL,
			action TEXT NOT NULL,
			actor_id TEXT,
			subject_id TEXT,
			username TEXT NOT NULL,
			detail TEXT NOT NULL,
			client_ip TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS tree_settings (
			tree_id TEXT PRIMARY KEY,
			living_threshold_years INTEGER NOT NULL,
			viewers_see_living INTEGER NOT NULL,
			anonymous_access TEXT NOT NULL
		);
	`); err != nil {
		return nil, fmt.Errorf("create account tables: %w", err)
	}
	return store, nil
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
	var taken int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE username = ?`, user.Username).Scan(&taken); err != nil {
		return fmt.Errorf("check username: %w", err)
	}
	if taken > 0 {
		return repository.ErrUsernameTaken
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO users (id, username, display_name, password_hash, created_at, disabled) VALUES (?, ?, ?, ?, ?, ?)`,
		user.ID.String(), user.Username, user.DisplayName, user.PasswordHash, formatTimestamp(user.CreatedAt), user.Disabled); err != nil {
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
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id.String()
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	// #nosec G202 -- only "?" placeholders are concatenated; the ids are bound.
	rows, err := s.db.QueryContext(ctx, `SELECT id, display_name FROM users WHERE id IN (`+placeholders+`)`, args...)
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

// DeleteExpiredSessions removes every session idle since before idleBefore or
// created before createdBefore.
func (s *AccountStore) DeleteExpiredSessions(ctx context.Context, idleBefore, createdBefore time.Time) (int, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE last_seen_at < ? OR created_at < ?`,
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

// SetMemberRole changes the role userID holds in treeID.
func (s *AccountStore) SetMemberRole(ctx context.Context, treeID, userID uuid.UUID, role repository.Role) error {
	result, err := s.db.ExecContext(ctx, `UPDATE tree_memberships SET role = ? WHERE tree_id = ? AND user_id = ?`,
		string(role), treeID.String(), userID.String())
	if err != nil {
		return fmt.Errorf("set member role: %w", err)
	}
	return requireRow(result, repository.ErrNotMember)
}

// UpdateUser stores user's display name, password hash and disabled flag.
func (s *AccountStore) UpdateUser(ctx context.Context, user *repository.User) error {
	result, err := s.db.ExecContext(ctx, `UPDATE users SET display_name = ?, password_hash = ?, disabled = ? WHERE id = ?`,
		user.DisplayName, user.PasswordHash, user.Disabled, user.ID.String())
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

	invite := repository.Invite{TokenHash: tokenHash}
	var treeStr, role, createdByStr, createdStr, expiresStr string
	err = tx.QueryRowContext(ctx,
		`DELETE FROM invites WHERE token_hash = ? AND expires_at > ? RETURNING tree_id, role, created_by, created_at, expires_at`,
		tokenHash, sessionTime(now)).Scan(&treeStr, &role, &createdByStr, &createdStr, &expiresStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrLinkNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("use invite: %w", err)
	}
	invite.Role = repository.Role(role)
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
	if err := createUser(ctx, tx, user, invite.TreeID, invite.Role); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit accept invite: %w", err)
	}
	return &invite, nil
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

// ListAuthAudit returns up to limit entries, newest first, after offset.
func (s *AccountStore) ListAuthAudit(ctx context.Context, limit, offset int) ([]repository.AuthAuditEntry, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM auth_audit`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count auth audit: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT at, action, actor_id, subject_id, username, detail, client_ip
		FROM auth_audit ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list auth audit: %w", err)
	}
	defer rows.Close()
	entries := []repository.AuthAuditEntry{}
	for rows.Next() {
		var (
			e                  repository.AuthAuditEntry
			atStr, action      string
			actorID, subjectID sql.NullString
		)
		if err := rows.Scan(&atStr, &action, &actorID, &subjectID, &e.Username, &e.Detail, &e.ClientIP); err != nil {
			return nil, 0, fmt.Errorf("scan auth audit: %w", err)
		}
		e.Action = repository.AuthAction(action)
		if e.At, err = parseTimestamp(atStr); err != nil {
			return nil, 0, fmt.Errorf("parse auth audit at: %w", err)
		}
		if e.ActorID, err = parseNullableID(actorID); err != nil {
			return nil, 0, fmt.Errorf("parse auth audit actor: %w", err)
		}
		if e.SubjectID, err = parseNullableID(subjectID); err != nil {
			return nil, 0, fmt.Errorf("parse auth audit subject: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list auth audit: %w", err)
	}
	return entries, total, nil
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
