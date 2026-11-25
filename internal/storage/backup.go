package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/cacack/my-family/internal/backup"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/postgres"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

// ErrBackupMemory is returned by Backup on the in-memory backend, whose data
// does not outlive the process.
var ErrBackupMemory = errors.New("the in-memory backend has nothing to back up; unset DEMO_MODE")

// Backup writes a backup archive (ADR-010) of the database cfg selects to w
// and returns its manifest. A server may keep running on the database: Backup
// takes the lock `serve` shares, so it waits on no server but refuses while a
// rebuild runs. It does not construct the stores as Open does, so it runs no
// schema DDL and no read-model version check (ADR-010 §6). It refuses a
// database it could not copy whole (checkBackupSource): one whose store schema
// is not this binary's, or whose read model holds values no event records and
// needs `myfamily rebuild` first.
func Backup(ctx context.Context, cfg *config.Config, w io.Writer, appVersion string) (*backup.Manifest, error) {
	switch Select(cfg) {
	case BackendMemory:
		return nil, ErrBackupMemory
	case BackendPostgres:
		db, lock, location, err := lockPostgres(cfg.DatabaseURL, AccessServe)
		if err != nil {
			return nil, err
		}
		defer func() { _ = lock.close(); _ = db.Close() }()
		if err := checkBackupSource(ctx, db, BackendPostgres); err != nil {
			return nil, fmt.Errorf("back up PostgreSQL database %s: %w", location, scrubError(err, cfg.DatabaseURL))
		}
		m, err := backup.Write(ctx, db, backup.Postgres, w, appVersion)
		if err != nil {
			return nil, fmt.Errorf("back up PostgreSQL database %s: %w", location, scrubError(err, cfg.DatabaseURL))
		}
		return m, nil
	default:
		path := cfg.SQLitePath
		// Opening a missing file would create an empty database.
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("back up SQLite database %q: %w", path, err)
		}
		db, lock, err := lockSQLite(path, AccessServe)
		if err != nil {
			return nil, err
		}
		defer func() { _ = lock.close(); _ = db.Close() }()
		if err := checkBackupSource(ctx, db, BackendSQLite); err != nil {
			return nil, fmt.Errorf("back up SQLite database %q: %w", path, err)
		}
		m, err := backup.Write(ctx, db, backup.SQLite, w, appVersion)
		if err != nil {
			return nil, fmt.Errorf("back up SQLite database %q: %w", path, err)
		}
		return m, nil
	}
}

// Backup writes a backup archive of the opened stores' database to w, as
// the package-level Backup does, for a server that already holds the lock.
func (s *Stores) Backup(ctx context.Context, w io.Writer, appVersion string) (*backup.Manifest, error) {
	var m *backup.Manifest
	err := s.backupSource(ctx, func(db *sql.DB, dialect backup.Dialect) error {
		var err error
		m, err = backup.Write(ctx, db, dialect, w, appVersion)
		return err
	})
	return m, err
}

// CheckBackup refuses the opened stores' database as Backup would, and
// writes no archive, so a server can name the fix before it starts a
// download (#1833).
func (s *Stores) CheckBackup(ctx context.Context) error {
	return s.backupSource(ctx, func(*sql.DB, backup.Dialect) error { return nil })
}

// backupSource refuses the opened stores' database as checkBackupSource
// does, then runs write on it.
func (s *Stores) backupSource(ctx context.Context, write func(*sql.DB, backup.Dialect) error) error {
	switch s.Backend {
	case BackendMemory:
		return ErrBackupMemory
	case BackendPostgres:
		if err := checkBackupSource(ctx, s.db, s.Backend); err != nil {
			return err
		}
		return write(s.db, backup.Postgres)
	default:
		// A connection of its own: the stores' pool has a single connection,
		// which the check's many reads and the backup's long one would take
		// from every request.
		db, err := sqlite.OpenDB(s.Location)
		if err != nil {
			return fmt.Errorf("back up SQLite database %q: %w", s.Location, err)
		}
		defer func() { _ = db.Close() }()
		if err := checkBackupSource(ctx, db, s.Backend); err != nil {
			return fmt.Errorf("back up SQLite database %q: %w", s.Location, err)
		}
		return write(db, backup.SQLite)
	}
}

// checkBackupSource refuses a database the archive could not copy whole. It
// only reads, and runs before backup.Write's read transaction.
//
// The store schema must be this binary's (ADR-011 §4). Then backup.CheckSource
// refuses tables an older release last shaped and a read model that predates
// its version stamp. Last, the read model must hold no value that no event
// records, by the rule a rebuild logs them by (repository.PendingBackfills):
// a stamped database holds some if a release before #680 wrote to it after a
// downgrade (ADR-010 §6).
func checkBackupSource(ctx context.Context, db *sql.DB, backend Backend) error {
	var migrator *repository.StoreMigrator
	var dialect backup.Dialect
	var events repository.EventStore
	var readModel repository.ReadModelStore
	var branches repository.BranchStore
	var snapshots repository.SnapshotStore
	if backend == BackendPostgres {
		migrator, dialect = postgres.NewStoreMigrator(db), backup.Postgres
		events, readModel, branches, snapshots = postgres.ReadOnlyStores(db)
	} else {
		migrator, dialect = sqlite.NewStoreMigrator(db), backup.SQLite
		events, readModel, branches, snapshots = sqlite.ReadOnlyStores(db)
	}
	if err := migrator.CheckCurrent(ctx); err != nil {
		return err
	}
	if err := backup.CheckSource(ctx, db, dialect); err != nil {
		return err
	}
	pending, err := repository.PendingBackfills(ctx, events, readModel, branches, snapshots)
	if ctx.Err() != nil {
		// Canceled or timed out, not a read model the rebuild would fix.
		return fmt.Errorf("check the read model for values no event records: %w", ctx.Err())
	}
	if err != nil {
		return fmt.Errorf("check the read model for values no event records (if it awaits `myfamily rebuild`, "+
			"run that first): %w", err)
	}
	if pending.Len() > 0 {
		return fmt.Errorf("%w: coordinates or EXIDs of %d entities, %d brick walls and %d snapshots; "+
			"stop every server and run `myfamily rebuild`, which logs them, then back up",
			backup.ErrReadModelOnlyValues, len(pending.ImportedValues), len(pending.BrickWalls), len(pending.Snapshots))
	}
	return nil
}
