package sqlite

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

// SettingsStore is a SQLite implementation of repository.SettingsStore. Each
// site setting is one row of site_settings, and each account's one row of
// account_settings, absent until the account first chooses and empty once it
// clears its choice.
type SettingsStore struct {
	db *sql.DB
}

// NewSettingsStore returns the store, first bringing the store schema, which
// owns the settings tables, up to date (ADR-011). That runs every store
// migration, the event log's included, so it fails as NewEventStore does: with
// repository.ErrStoreSchemaNewer on a database stamped newer than this binary,
// and with ErrReadModelEventsTable on a pre-#733 database whose read model
// store has not been constructed first (DB-007).
func NewSettingsStore(db *sql.DB) (*SettingsStore, error) {
	if err := migrateStoreSchema(db); err != nil {
		return nil, fmt.Errorf("migrate store schema: %w", err)
	}
	return &SettingsStore{db: db}, nil
}

// HomePerson returns accountID's home person, or uuid.Nil when none is set.
func (s *SettingsStore) HomePerson(ctx context.Context, accountID uuid.UUID) (uuid.UUID, error) {
	var value string
	var err error
	if accountID == repository.SiteWide {
		err = s.db.QueryRowContext(ctx,
			`SELECT value FROM site_settings WHERE key = ?`, repository.SettingHomePerson,
		).Scan(&value)
	} else {
		err = s.db.QueryRowContext(ctx,
			`SELECT value FROM account_settings WHERE user_id = ? AND key = ?`, accountID.String(), repository.SettingHomePerson,
		).Scan(&value)
	}
	if errors.Is(err, sql.ErrNoRows) || (err == nil && value == "") {
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

// SetHomePerson marks personID as accountID's home person; uuid.Nil clears it.
func (s *SettingsStore) SetHomePerson(ctx context.Context, accountID, personID uuid.UUID) error {
	var err error
	switch {
	case accountID == repository.SiteWide && personID == uuid.Nil:
		_, err = s.db.ExecContext(ctx, `DELETE FROM site_settings WHERE key = ?`, repository.SettingHomePerson)
	case accountID == repository.SiteWide:
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO site_settings (key, value) VALUES (?, ?)
			ON CONFLICT (key) DO UPDATE SET value = excluded.value`,
			repository.SettingHomePerson, personID.String())
	default:
		// An account's clear keeps its row, empty, so SeedHomePerson can tell
		// a cleared choice from none ever made.
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO account_settings (user_id, key, value) VALUES (?, ?, ?)
			ON CONFLICT (user_id, key) DO UPDATE SET value = excluded.value`,
			accountID.String(), repository.SettingHomePerson, accountValue(personID))
	}
	if err != nil {
		return fmt.Errorf("save home person: %w", err)
	}
	return nil
}

// accountValue is the stored form of an account's home person: its ID, or ""
// once the account has cleared its choice.
func accountValue(personID uuid.UUID) string {
	if personID == uuid.Nil {
		return ""
	}
	return personID.String()
}

// SeedHomePerson gives accountID the site's home person unless accountID has
// a row, chosen or cleared.
func (s *SettingsStore) SeedHomePerson(ctx context.Context, accountID uuid.UUID) error {
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO account_settings (user_id, key, value)
		SELECT ?, key, value FROM site_settings WHERE key = ?
		ON CONFLICT (user_id, key) DO NOTHING`,
		accountID.String(), repository.SettingHomePerson); err != nil {
		return fmt.Errorf("seed home person: %w", err)
	}
	return nil
}

// MoveHomePersons makes survivors[p] the home person of the site and of every
// account whose home person is p, all or nothing. Every home person is read
// first and looked up in survivors once, so a chain of merges is not followed
// whatever order the map yields.
func (s *SettingsStore) MoveHomePersons(ctx context.Context, survivors map[uuid.UUID]uuid.UUID) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("move home person: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// homes maps each account's user_id, and "" for the site, to its home person.
	homes := make(map[string]string)
	var site string
	switch err := tx.QueryRowContext(ctx, `SELECT value FROM site_settings WHERE key = ?`, repository.SettingHomePerson).Scan(&site); {
	case err == nil:
		homes[""] = site
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("move home person: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT user_id, value FROM account_settings WHERE key = ?`, repository.SettingHomePerson)
	if err != nil {
		return fmt.Errorf("move home person: %w", err)
	}
	for rows.Next() {
		var userID, value string
		if err := rows.Scan(&userID, &value); err != nil {
			_ = rows.Close()
			return fmt.Errorf("move home person: %w", err)
		}
		homes[userID] = value
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("move home person: %w", err)
	}

	for account, value := range homes {
		home, err := uuid.Parse(value)
		if err != nil {
			continue // a cleared or corrupt value is no merged person
		}
		survivor, merged := survivors[home]
		if !merged {
			continue
		}
		if account == "" {
			_, err = tx.ExecContext(ctx, `UPDATE site_settings SET value = ? WHERE key = ?`, survivor.String(), repository.SettingHomePerson)
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE account_settings SET value = ? WHERE user_id = ? AND key = ?`, survivor.String(), account, repository.SettingHomePerson)
		}
		if err != nil {
			return fmt.Errorf("move home person: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("move home person: %w", err)
	}
	return nil
}
