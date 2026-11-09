package storage

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/config"
)

// Rebuild / serve exclusion (#1232).

func TestSQLite_RebuildAndServeExcludeEachOther(t *testing.T) {
	assertRebuildAndServeExcludeEachOther(t, &config.Config{SQLitePath: filepath.Join(t.TempDir(), "myfamily.db")})
}

func TestPostgres_RebuildAndServeExcludeEachOther(t *testing.T) {
	assertRebuildAndServeExcludeEachOther(t, &config.Config{DatabaseURL: postgresDatabase(t)})
}

// assertRebuildAndServeExcludeEachOther proves the lock Open takes: servers
// share it, a rebuild holds it alone, and each refusal names the holder and
// leaves the data untouched.
func assertRebuildAndServeExcludeEachOther(t *testing.T, cfg *config.Config) {
	t.Helper()
	ctx := context.Background()

	serving, err := Open(cfg, AccessServe)
	if err != nil {
		t.Fatalf("Open for serve: %v", err)
	}
	createPerson(t, newServer(serving), "Ada", "Locked")

	// A second server may share the database.
	second, err := Open(cfg, AccessServe)
	if err != nil {
		t.Fatalf("second Open for serve: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("close second server: %v", err)
	}

	// A rebuild is refused while a server holds the database, and changes nothing.
	if _, err := Open(cfg, AccessRebuild); !errors.Is(err, ErrServerRunning) || !strings.Contains(err.Error(), "server is running") {
		t.Fatalf("Open for rebuild while serving: err = %v, want ErrServerRunning", err)
	}
	if n := countPersons(t, newServer(serving)); n != 1 {
		t.Errorf("persons after the refused rebuild = %d, want 1", n)
	}
	if err := serving.Close(); err != nil {
		t.Fatalf("close server: %v", err)
	}

	// Once the server is gone the rebuild runs, and holds the database alone.
	rebuilding, err := Open(cfg, AccessRebuild)
	if err != nil {
		t.Fatalf("Open for rebuild after the server stopped: %v", err)
	}
	if _, err := Open(cfg, AccessServe); !errors.Is(err, ErrRebuildRunning) || !strings.Contains(err.Error(), "rebuild is in progress") {
		t.Fatalf("Open for serve during a rebuild: err = %v, want ErrRebuildRunning", err)
	}
	if _, err := Open(cfg, AccessRebuild); !errors.Is(err, ErrRebuildRunning) {
		t.Fatalf("second Open for rebuild: err = %v, want ErrRebuildRunning", err)
	}
	if _, err := rebuilding.RebuildReadModel(ctx); err != nil {
		t.Fatalf("RebuildReadModel: %v", err)
	}
	if err := rebuilding.Close(); err != nil {
		t.Fatalf("close rebuild: %v", err)
	}

	// Closing releases the lock: the server starts again and sees the data.
	again, err := Open(cfg, AccessServe)
	if err != nil {
		t.Fatalf("Open for serve after the rebuild: %v", err)
	}
	t.Cleanup(func() { _ = again.Close() })
	if n := countPersons(t, newServer(again)); n != 1 {
		t.Errorf("persons after the rebuild = %d, want 1", n)
	}
}

// fakeLock grants the modes in grant and fails as configured.
type fakeLock struct {
	grantShared, grantExclusive bool
	lockErr, unlockErr          error
	unlocked                    bool
}

func (l *fakeLock) tryLock(exclusive bool) (bool, error) {
	if l.lockErr != nil {
		return false, l.lockErr
	}
	if exclusive {
		return l.grantExclusive, nil
	}
	return l.grantShared, nil
}

func (l *fakeLock) unlock() error { l.unlocked = true; return l.unlockErr }
func (l *fakeLock) close() error  { return nil }

func TestAcquire(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name   string
		lock   fakeLock
		access Access
		want   error
	}{
		{"serve gets the shared lock", fakeLock{grantShared: true}, AccessServe, nil},
		{"rebuild gets the exclusive lock", fakeLock{grantExclusive: true}, AccessRebuild, nil},
		{"serve during a rebuild", fakeLock{}, AccessServe, ErrRebuildRunning},
		{"rebuild while servers hold it shared", fakeLock{grantShared: true}, AccessRebuild, ErrServerRunning},
		{"rebuild during another rebuild", fakeLock{}, AccessRebuild, ErrRebuildRunning},
		{"lock error", fakeLock{lockErr: boom}, AccessServe, boom},
		{"unlock error after the probe", fakeLock{grantShared: true, unlockErr: boom}, AccessRebuild, boom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lock := tt.lock
			err := acquire(&lock, tt.access)
			if tt.want == nil && err != nil || tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("acquire = %v, want %v", err, tt.want)
			}
			if tt.want == ErrServerRunning && !lock.unlocked {
				t.Error("the shared probe lock was not released")
			}
		})
	}
}

// TestRebuildReadModel_RequiresRebuildAccess keeps the rebuild behind the
// exclusive lock: stores opened for serve refuse it.
func TestRebuildReadModel_RequiresRebuildAccess(t *testing.T) {
	stores, err := OpenSQLite(filepath.Join(t.TempDir(), "myfamily.db"), AccessServe)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = stores.Close() })
	if _, err := stores.RebuildReadModel(context.Background()); !errors.Is(err, ErrRebuildNotExclusive) {
		t.Fatalf("RebuildReadModel on serve stores = %v, want ErrRebuildNotExclusive", err)
	}
}
