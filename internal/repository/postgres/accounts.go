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
			created_at TIMESTAMPTZ NOT NULL
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

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO users (id, username, display_name, password_hash, created_at) VALUES ($1, $2, $3, $4, $5)`,
		user.ID, user.Username, user.DisplayName, user.PasswordHash, user.CreatedAt); err != nil {
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
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit create user: %w", err)
	}
	return nil
}

func (s *AccountStore) getUser(ctx context.Context, where string, arg any) (*repository.User, error) {
	var user repository.User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, display_name, password_hash, created_at FROM users WHERE `+where+` = $1`, arg,
	).Scan(&user.ID, &user.Username, &user.DisplayName, &user.PasswordHash, &user.CreatedAt)
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
