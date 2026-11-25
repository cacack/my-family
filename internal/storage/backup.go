package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/cacack/my-family/internal/backup"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

// ErrBackupMemory is returned by Backup on the in-memory backend, whose data
// does not outlive the process.
var ErrBackupMemory = errors.New("the in-memory backend has nothing to back up; unset DEMO_MODE")

// Backup writes a backup archive (ADR-010) of the database cfg selects to w
// and returns its manifest. A server may keep running on the database: Backup
// takes the lock `serve` shares, so it waits on no server but refuses while a
// rebuild runs. It does not construct the stores as Open does, so it runs no
// schema DDL and no read-model version check (ADR-010 §6). backup.Write also
// refuses a database it could not copy whole, such as one that predates the
// read-model version stamp and needs `myfamily rebuild` first.
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
	switch s.Backend {
	case BackendMemory:
		return nil, ErrBackupMemory
	case BackendPostgres:
		return backup.Write(ctx, s.db, backup.Postgres, w, appVersion)
	default:
		// A connection of its own: the stores' pool has a single connection,
		// which the backup's long read would hold from every request.
		db, err := sqlite.OpenDB(s.Location)
		if err != nil {
			return nil, fmt.Errorf("back up SQLite database %q: %w", s.Location, err)
		}
		defer func() { _ = db.Close() }()
		return backup.Write(ctx, db, backup.SQLite, w, appVersion)
	}
}
