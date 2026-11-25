package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/cacack/my-family/internal/backup"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/testutil/pgtest"
)

// backupOf writes a backup of a SQLite database holding two persons to a file
// and returns its path.
func backupOf(t *testing.T) string {
	t.Helper()
	cfg := &config.Config{SQLitePath: filepath.Join(t.TempDir(), "source.db")}
	stores, err := Open(cfg, AccessServe)
	if err != nil {
		t.Fatal(err)
	}
	server := newServer(stores)
	createPerson(t, server, "Ada", "Restore")
	createPerson(t, server, "Grace", "Restore")
	if err := stores.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "backup.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := Backup(context.Background(), cfg, f, "test"); err != nil {
		t.Fatal(err)
	}
	return path
}

// assertRestore restores a backup into cfg's empty database, which a server
// then opens and serves.
func assertRestore(t *testing.T, cfg *config.Config) {
	t.Helper()
	result, err := Restore(context.Background(), cfg, backupOf(t))
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if result.Rebuild.Events != int(result.Manifest.EventCount) || result.Manifest.EventCount == 0 {
		t.Errorf("replayed %d events, the archive holds %d", result.Rebuild.Events, result.Manifest.EventCount)
	}
	stores, err := Open(cfg, AccessServe)
	if err != nil {
		t.Fatalf("serve the restored database: %v", err)
	}
	t.Cleanup(func() { _ = stores.Close() })
	if n := countPersons(t, newServer(stores)); n != 2 {
		t.Errorf("person count = %d, want 2", n)
	}
}

func TestRestore_SQLite(t *testing.T) {
	assertRestore(t, &config.Config{SQLitePath: filepath.Join(t.TempDir(), "restored.db")})
}

func TestRestore_Postgres(t *testing.T) {
	assertRestore(t, &config.Config{DatabaseURL: pgtest.NewDatabase(t)})
}

// TestRestore_Refusals: each refusal leaves the target as it was.
func TestRestore_Refusals(t *testing.T) {
	ctx := context.Background()
	archive := backupOf(t)

	t.Run("damaged archive", func(t *testing.T) {
		damaged := filepath.Join(t.TempDir(), "damaged.zip")
		if err := os.WriteFile(damaged, []byte("not a zip"), 0o600); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(t.TempDir(), "restored.db")
		if _, err := Restore(ctx, &config.Config{SQLitePath: target}, damaged); err == nil {
			t.Fatal("Restore of a damaged archive: want error")
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Errorf("a refused restore created the database (stat err = %v)", err)
		}
	})

	t.Run("server running", func(t *testing.T) {
		cfg := &config.Config{SQLitePath: filepath.Join(t.TempDir(), "served.db")}
		stores, err := Open(cfg, AccessServe)
		if err != nil {
			t.Fatal(err)
		}
		defer stores.Close()
		if _, err := Restore(ctx, cfg, archive); !errors.Is(err, ErrServerRunning) {
			t.Fatalf("Restore while a server runs = %v, want ErrServerRunning", err)
		}
		if n := countPersons(t, newServer(stores)); n != 0 {
			t.Errorf("person count = %d, want the database unchanged", n)
		}
	})

	t.Run("database holds data", func(t *testing.T) {
		cfg := &config.Config{SQLitePath: filepath.Join(t.TempDir(), "used.db")}
		stores, err := Open(cfg, AccessServe)
		if err != nil {
			t.Fatal(err)
		}
		createPerson(t, newServer(stores), "Already", "Here")
		if err := stores.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := Restore(ctx, cfg, archive); !errors.Is(err, backup.ErrNotEmpty) {
			t.Fatalf("Restore into a database with data = %v, want ErrNotEmpty", err)
		}
		stores, err = Open(cfg, AccessServe)
		if err != nil {
			t.Fatal(err)
		}
		defer stores.Close()
		if n := countPersons(t, newServer(stores)); n != 1 {
			t.Errorf("person count = %d, want the database unchanged", n)
		}
	})

	t.Run("memory", func(t *testing.T) {
		if _, err := Restore(ctx, &config.Config{DemoMode: true}, archive); !errors.Is(err, ErrRestoreMemory) {
			t.Errorf("Restore = %v, want ErrRestoreMemory", err)
		}
	})
}
