package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/cacack/my-family/internal/backup"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository"
)

// ErrRestoreMemory is returned by Restore on the in-memory backend, whose data
// does not outlive the process.
var ErrRestoreMemory = errors.New("the in-memory backend has nothing to restore into; unset DEMO_MODE")

// RestoreResult is what Restore loaded and replayed.
type RestoreResult struct {
	// Manifest is the restored archive's manifest; its event count and last
	// position are the restored log's.
	Manifest *backup.Manifest
	// Rebuild is the replay of the restored log into the read model.
	Rebuild repository.RebuildResult
}

// Restore restores the backup archive at path into the empty database cfg
// selects, following ADR-010's restore path: it verifies the archive before it
// opens the database, opens the database with the lock `rebuild` takes (so it
// refuses while a server holds it), loads the log and the carried tables into
// it in one transaction (refusing a database that holds data), then replays
// the log as `rebuild` does. Every refusal leaves the database unchanged.
//
// If the replay fails, the log is already committed; the error says so, and
// `myfamily rebuild` finishes the same replay.
func Restore(ctx context.Context, cfg *config.Config, path string) (RestoreResult, error) {
	backend := Select(cfg)
	if backend == BackendMemory {
		return RestoreResult{}, ErrRestoreMemory
	}
	archive, err := backup.OpenArchive(path)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("verify %s: %w", path, err)
	}
	defer func() { _ = archive.Close() }()
	result := RestoreResult{Manifest: archive.Manifest}

	stores, err := Open(cfg, AccessRebuild)
	if err != nil {
		return result, err
	}
	defer func() { _ = stores.Close() }()

	dialect := backup.SQLite
	if backend == BackendPostgres {
		dialect = backup.Postgres
	}
	if err := archive.Load(ctx, stores.db, dialect); err != nil {
		return result, fmt.Errorf("restore into %s: %w", stores.Describe(), err)
	}
	if result.Rebuild, err = stores.RebuildReadModel(ctx); err != nil {
		return result, fmt.Errorf("the event log and accounts are restored, but replaying the log into the read model failed; "+
			"fix the cause and run `myfamily rebuild` to finish it: %w", err)
	}
	return result, nil
}
