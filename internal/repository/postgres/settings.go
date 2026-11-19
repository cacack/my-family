package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/repository"
)

// Compile-time assertion that SettingsStore satisfies the interface.
var _ repository.SettingsStore = (*SettingsStore)(nil)

// SettingsStore is a PostgreSQL implementation of repository.SettingsStore. Each
// setting is one row of site_settings, absent while unset.
type SettingsStore struct {
	db *sql.DB
}

// NewSettingsStore creates a new PostgreSQL settings store.
func NewSettingsStore(db *sql.DB) (*SettingsStore, error) {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS site_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)
	`); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}
	return &SettingsStore{db: db}, nil
}

// HomePerson returns the home person, or uuid.Nil when none is set.
func (s *SettingsStore) HomePerson(ctx context.Context) (uuid.UUID, error) {
	var value string
	err := s.db.QueryRowContext(ctx,
		`SELECT value FROM site_settings WHERE key = $1`, repository.SettingHomePerson,
	).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, nil
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("read home person: %w", err)
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, fmt.Errorf("read home person: %w", err)
	}
	return id, nil
}

// SetHomePerson marks id as the home person; uuid.Nil clears it.
func (s *SettingsStore) SetHomePerson(ctx context.Context, id uuid.UUID) error {
	var err error
	if id == uuid.Nil {
		_, err = s.db.ExecContext(ctx, `DELETE FROM site_settings WHERE key = $1`, repository.SettingHomePerson)
	} else {
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO site_settings (key, value) VALUES ($1, $2)
			ON CONFLICT (key) DO UPDATE SET value = excluded.value`,
			repository.SettingHomePerson, id.String())
	}
	if err != nil {
		return fmt.Errorf("save home person: %w", err)
	}
	return nil
}
