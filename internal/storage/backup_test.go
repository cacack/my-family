package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/backup"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/sqlite"
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

// readModelOnlyValues write, straight to the read model or the snapshot
// registry, a value no event records, as releases before #680 did (ADR-010
// §6). Each is given a person created through the server.
var readModelOnlyValues = []struct {
	name  string
	write func(ctx context.Context, s *Stores, person uuid.UUID) error
}{
	{"coordinate", func(ctx context.Context, s *Stores, person uuid.UUID) error {
		p, err := s.ReadModel.GetPerson(ctx, domain.MainBranchID, person)
		if err != nil {
			return err
		}
		lat := "N1.5"
		p.BirthPlaceLat = &lat
		return s.ReadModel.SavePerson(ctx, domain.MainBranchID, p)
	}},
	{"exid", func(ctx context.Context, s *Stores, person uuid.UUID) error {
		return s.ReadModel.ReplacePersonExternalIDs(ctx, domain.MainBranchID, person,
			[]repository.PersonExternalIDReadModel{{PersonID: person, Value: "P-1"}})
	}},
	{"brick wall", func(ctx context.Context, s *Stores, person uuid.UUID) error {
		p, err := s.ReadModel.GetPerson(ctx, domain.MainBranchID, person)
		if err != nil {
			return err
		}
		since := time.Date(2020, 3, 1, 12, 0, 0, 0, time.UTC)
		p.BrickWallNote, p.BrickWallSince = "stuck", &since
		return s.ReadModel.SavePerson(ctx, domain.MainBranchID, p)
	}},
	{"snapshot", func(ctx context.Context, s *Stores, _ uuid.UUID) error {
		snap, err := domain.NewSnapshot("before #624", "", 1)
		if err != nil {
			return err
		}
		return s.Snapshots.Create(ctx, snap)
	}},
}

// openWithReadModelOnly opens cfg's stamped database, as a server does, and
// gives it one value only the read model holds.
func openWithReadModelOnly(t *testing.T, cfg *config.Config, write func(context.Context, *Stores, uuid.UUID) error) *Stores {
	t.Helper()
	stores, err := Open(cfg, AccessServe)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = stores.Close() })
	person := uuid.MustParse(createPerson(t, newServer(stores), "Ada", "Legacy"))
	if err := write(context.Background(), stores, person); err != nil {
		t.Fatalf("write the read-model-only value: %v", err)
	}
	return stores
}

// assertBackupRefusesReadModelOnly expects both the command's backup and the
// server's (GET /api/v1/backup) to refuse each read-model-only value of a
// stamped database, naming the rebuild.
func assertBackupRefusesReadModelOnly(t *testing.T, newConfig func(*testing.T) *config.Config) {
	for _, v := range readModelOnlyValues {
		t.Run(v.name, func(t *testing.T) {
			cfg := newConfig(t)
			stores := openWithReadModelOnly(t, cfg, v.write)
			ctx := context.Background()
			if _, err := stores.Backup(ctx, &bytes.Buffer{}, "test"); err == nil || !strings.Contains(err.Error(), "myfamily rebuild") {
				t.Errorf("Stores.Backup = %v, want a refusal naming myfamily rebuild", err)
			}
			if _, err := Backup(ctx, cfg, &bytes.Buffer{}, "test"); err == nil || !strings.Contains(err.Error(), "myfamily rebuild") {
				t.Errorf("Backup = %v, want a refusal naming myfamily rebuild", err)
			}
		})
	}
}

func TestBackup_SQLiteRefusesReadModelOnlyValues(t *testing.T) {
	assertBackupRefusesReadModelOnly(t, func(t *testing.T) *config.Config {
		return &config.Config{SQLitePath: filepath.Join(t.TempDir(), "myfamily.db")}
	})
}

func TestBackup_PostgresRefusesReadModelOnlyValues(t *testing.T) {
	assertBackupRefusesReadModelOnly(t, func(t *testing.T) *config.Config {
		return &config.Config{DatabaseURL: pgtest.NewDatabase(t)}
	})
}

// TestBackup_SQLiteCheckWritesNothing: the backup's checks run no DDL and
// write nothing, whether they refuse or pass. Closing the last connection
// checkpoints the WAL, so any write would reach the database file.
func TestBackup_SQLiteCheckWritesNothing(t *testing.T) {
	fileSum := func(t *testing.T, path string) [sha256.Size]byte {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return sha256.Sum256(data)
	}
	for _, tt := range []struct {
		name    string
		write   func(context.Context, *Stores, uuid.UUID) error
		refused bool
	}{
		{"refused", readModelOnlyValues[0].write, true},
		{"passed", func(context.Context, *Stores, uuid.UUID) error { return nil }, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{SQLitePath: filepath.Join(t.TempDir(), "myfamily.db")}
			stores := openWithReadModelOnly(t, cfg, tt.write)
			if err := stores.Close(); err != nil {
				t.Fatal(err)
			}
			before := fileSum(t, cfg.SQLitePath)
			_, err := Backup(context.Background(), cfg, &bytes.Buffer{}, "test")
			if (err != nil) != tt.refused {
				t.Fatalf("Backup = %v, want refused = %v", err, tt.refused)
			}
			if fileSum(t, cfg.SQLitePath) != before {
				t.Error("the backup changed the database file")
			}
		})
	}
}

// TestBackup_SQLiteStoreSchemaStamp: the backup refuses a store schema newer
// than this binary's, naming both versions, and accepts an unstamped database
// while the binary knows only the baseline (ADR-011 §4). An older stamp is
// covered by StoreMigrator.CheckCurrent's own test, as this binary has no
// migration after the baseline yet.
func TestBackup_SQLiteStoreSchemaStamp(t *testing.T) {
	for _, tt := range []struct {
		name, stamp string
		newer       bool
	}{
		{"newer", `UPDATE store_schema SET version = 99, name = 'from_the_future'`, true},
		{"unstamped", `DROP TABLE store_schema`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{SQLitePath: filepath.Join(t.TempDir(), "myfamily.db")}
			stores := openWithReadModelOnly(t, cfg, func(context.Context, *Stores, uuid.UUID) error { return nil })
			if err := stores.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := sqlite.OpenDB(cfg.SQLitePath)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tt.stamp); err != nil {
				t.Fatal(err)
			}
			_ = db.Close()

			_, err = Backup(context.Background(), cfg, &bytes.Buffer{}, "test")
			if !tt.newer {
				if err != nil {
					t.Errorf("Backup of an unstamped database = %v, want it accepted", err)
				}
				return
			}
			if !errors.Is(err, repository.ErrStoreSchemaNewer) {
				t.Fatalf("Backup = %v, want ErrStoreSchemaNewer", err)
			}
			for _, want := range []string{"version 99 (from_the_future)", "up to 1 (baseline)"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not say %q", err, want)
				}
			}
		})
	}
}
