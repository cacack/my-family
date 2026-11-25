package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
)

// StoreMigration is one numbered change to the tables a rebuild does not
// re-create: the event log, accounts and settings (ADR-011). Its version is its
// position in a backend's list, from 1. Migrations are never edited once
// released; a change to one of those tables is a new migration at the end of
// both backends' lists (DB-010).
type StoreMigration struct {
	// Name identifies the migration in logs, errors and the stamp. Both
	// backends' lists carry the same names in the same order.
	Name string
	// Apply makes the change inside tx, which the runner commits together
	// with the new stamp.
	Apply func(ctx context.Context, tx *sql.Tx) error
	// ForeignKeysOff runs the migration with foreign-key enforcement off. A
	// SQLite migration that copies a table to change its key needs it, and
	// SQLite only allows the switch outside a transaction.
	ForeignKeysOff bool
}

// StoreSchemaStamp is the last migration applied to a database. Version 0,
// with no name, is a database no migration has been applied to.
type StoreSchemaStamp struct {
	Version int
	Name    string
}

// RowQuerier is what reading a stamp needs: *sql.DB, *sql.Conn and *sql.Tx all
// provide it.
type RowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// StoreSchemaBackend is the part of running migrations that differs between
// SQL dialects.
type StoreSchemaBackend interface {
	// BeginLocked begins a transaction on conn that holds the database's write
	// lock, so a migration another process applies meanwhile is seen once the
	// stamp is re-read, not run twice. The stamp table exists inside it.
	BeginLocked(ctx context.Context, conn *sql.Conn) (*sql.Tx, error)
	// ReadStamp returns the stamp, or version 0 when there is no stamp table
	// or no row in it. It never creates anything.
	ReadStamp(ctx context.Context, q RowQuerier) (StoreSchemaStamp, error)
	// WriteStamp records stamp in tx.
	WriteStamp(ctx context.Context, tx *sql.Tx, stamp StoreSchemaStamp) error
	// SetForeignKeys turns foreign-key enforcement on conn on or off, outside
	// any transaction.
	SetForeignKeys(ctx context.Context, conn *sql.Conn, on bool) error
}

// ErrStoreSchemaNewer is returned when a database's store schema is stamped
// with a migration this binary does not know: a newer release wrote it.
var ErrStoreSchemaNewer = errors.New("the database's store schema is newer than this version of myfamily")

// StoreMigrator applies a backend's migrations to one database (ADR-011 §2).
type StoreMigrator struct {
	DB         *sql.DB
	Backend    StoreSchemaBackend
	Migrations []StoreMigration
	// Logger receives one line per migration, before it runs; nil is
	// slog.Default().
	Logger *slog.Logger
}

// Check refuses a database stamped above the last migration this binary
// knows, wrapping ErrStoreSchemaNewer. It only reads.
func (m *StoreMigrator) Check(ctx context.Context) error {
	stamp, err := m.Backend.ReadStamp(ctx, m.DB)
	if err != nil {
		return fmt.Errorf("read the store schema version: %w", err)
	}
	return m.checkNotNewer(stamp)
}

// Run applies, in order, every migration after the database's stamp, each in
// its own transaction with its stamp update. A migration that fails is rolled
// back, leaving the stamp and the tables at the last good version, and Run
// returns an error naming it. A database stamped above the list is refused
// (ErrStoreSchemaNewer) before anything is applied.
func (m *StoreMigrator) Run(ctx context.Context) error {
	stamp, err := m.Backend.ReadStamp(ctx, m.DB)
	if err != nil {
		return fmt.Errorf("read the store schema version: %w", err)
	}
	if err := m.checkNotNewer(stamp); err != nil {
		return err
	}
	if stamp.Version == len(m.Migrations) {
		return nil
	}
	conn, err := m.DB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("store schema migration: connect: %w", err)
	}
	defer func() { _ = conn.Close() }()
	for i := stamp.Version; i < len(m.Migrations); i++ {
		if err := m.apply(ctx, conn, i+1, m.Migrations[i]); err != nil {
			return fmt.Errorf("store schema migration %d (%s): %w", i+1, m.Migrations[i].Name, err)
		}
	}
	return nil
}

// apply runs migration version unless the stamp, re-read under the lock, says
// another process already has.
func (m *StoreMigrator) apply(ctx context.Context, conn *sql.Conn, version int, mig StoreMigration) (err error) {
	if mig.ForeignKeysOff {
		if err := m.Backend.SetForeignKeys(ctx, conn, false); err != nil {
			return fmt.Errorf("turn foreign keys off: %w", err)
		}
		defer func() {
			if onErr := m.Backend.SetForeignKeys(context.WithoutCancel(ctx), conn, true); onErr != nil {
				err = errors.Join(err, fmt.Errorf("turn foreign keys back on: %w", onErr))
			}
		}()
	}
	tx, err := m.Backend.BeginLocked(ctx, conn)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stamp, err := m.Backend.ReadStamp(ctx, tx)
	if err != nil {
		return fmt.Errorf("re-read the stamp: %w", err)
	}
	if err := m.checkNotNewer(stamp); err != nil {
		return err
	}
	if stamp.Version >= version {
		return nil
	}

	m.logger().Info("applying store schema migration", "version", version, "name", mig.Name)
	if err := mig.Apply(ctx, tx); err != nil {
		return err
	}
	if err := m.Backend.WriteStamp(ctx, tx, StoreSchemaStamp{Version: version, Name: mig.Name}); err != nil {
		return fmt.Errorf("write the stamp: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (m *StoreMigrator) checkNotNewer(stamp StoreSchemaStamp) error {
	known := len(m.Migrations)
	if stamp.Version <= known {
		return nil
	}
	last := "none"
	if known > 0 {
		last = m.Migrations[known-1].Name
	}
	return fmt.Errorf("%w: the database is at store schema version %d (%s) and this binary knows versions up to %d (%s); "+
		"restore the backup taken before the upgrade, or run the newer release",
		ErrStoreSchemaNewer, stamp.Version, stamp.Name, known, last)
}

func (m *StoreMigrator) logger() *slog.Logger {
	if m.Logger != nil {
		return m.Logger
	}
	return slog.Default()
}
