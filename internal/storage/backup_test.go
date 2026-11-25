package storage

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/cacack/my-family/internal/backup"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/testutil/pgtest"
)

// assertBackupWhileServing backs up a database a server holds and is still
// writing to, through the command's path (Backup) and the server's
// (Stores.Backup), and checks the archive matches the log.
func assertBackupWhileServing(t *testing.T, cfg *config.Config) {
	t.Helper()
	ctx := context.Background()
	stores, err := Open(cfg, AccessServe)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = stores.Close() })
	server := newServer(stores)
	createPerson(t, server, "Ada", "Backup")
	createPerson(t, server, "Grace", "Backup")

	var buf bytes.Buffer
	m, err := Backup(ctx, cfg, &buf, "test")
	if err != nil {
		t.Fatalf("Backup while a server holds the database: %v", err)
	}
	stored, err := stores.Events.ReadAll(ctx, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if m.EventCount != int64(len(stored)) || m.LastPosition != stored[len(stored)-1].Position {
		t.Errorf("manifest holds %d events to %d, the log %d to %d",
			m.EventCount, m.LastPosition, len(stored), stored[len(stored)-1].Position)
	}
	if _, err := backup.Verify(bytes.NewReader(buf.Bytes()), int64(buf.Len())); err != nil {
		t.Errorf("Verify: %v", err)
	}

	// The server keeps writing, and its own backup sees the new event.
	createPerson(t, server, "Late", "Backup")
	buf.Reset()
	again, err := stores.Backup(ctx, &buf, "test")
	if err != nil {
		t.Fatalf("Stores.Backup: %v", err)
	}
	if again.EventCount <= m.EventCount || again.LastPosition <= m.LastPosition {
		t.Errorf("second backup = %+v, want more events than the first, %+v", again, m)
	}
	// The server still serves after its backup.
	if n := countPersons(t, server); n != 3 {
		t.Errorf("person count = %d, want 3", n)
	}
}

func assertBackupRefusedDuringRebuild(t *testing.T, cfg *config.Config) {
	t.Helper()
	stores, err := Open(cfg, AccessRebuild)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = stores.Close() })
	if _, err := Backup(context.Background(), cfg, &bytes.Buffer{}, "test"); !errors.Is(err, ErrRebuildRunning) {
		t.Errorf("Backup during a rebuild = %v, want ErrRebuildRunning", err)
	}
}

func TestBackup_SQLiteWhileServing(t *testing.T) {
	assertBackupWhileServing(t, &config.Config{SQLitePath: filepath.Join(t.TempDir(), "myfamily.db")})
}

func TestBackup_PostgresWhileServing(t *testing.T) {
	assertBackupWhileServing(t, &config.Config{DatabaseURL: pgtest.NewDatabase(t)})
}

func TestBackup_SQLiteRefusedDuringRebuild(t *testing.T) {
	assertBackupRefusedDuringRebuild(t, &config.Config{SQLitePath: filepath.Join(t.TempDir(), "myfamily.db")})
}

func TestBackup_PostgresRefusedDuringRebuild(t *testing.T) {
	assertBackupRefusedDuringRebuild(t, &config.Config{DatabaseURL: pgtest.NewDatabase(t)})
}

func TestBackup_SQLiteMissingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	if _, err := Backup(context.Background(), &config.Config{SQLitePath: path}, &bytes.Buffer{}, "test"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Backup of a missing database = %v, want not exist", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("Backup created the database (stat err = %v)", err)
	}
}

func TestBackup_PostgresUnreachable(t *testing.T) {
	cfg := &config.Config{DatabaseURL: "postgres://u:" + "secret@127.0.0.1:1/db?sslmode=disable"}
	if _, err := Backup(context.Background(), cfg, &bytes.Buffer{}, "test"); err == nil {
		t.Error("Backup of an unreachable database: want error")
	}
}

func TestBackup_SQLiteNoSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.db")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Backup(context.Background(), &config.Config{SQLitePath: path}, &bytes.Buffer{}, "test"); err == nil {
		t.Error("Backup of a database with no tables: want error")
	}
}

func TestBackup_Memory(t *testing.T) {
	if _, err := Backup(context.Background(), &config.Config{DemoMode: true}, &bytes.Buffer{}, "test"); !errors.Is(err, ErrBackupMemory) {
		t.Errorf("Backup = %v, want ErrBackupMemory", err)
	}
	if _, err := OpenMemory().Backup(context.Background(), &bytes.Buffer{}, "test"); !errors.Is(err, ErrBackupMemory) {
		t.Errorf("Stores.Backup = %v, want ErrBackupMemory", err)
	}
}
