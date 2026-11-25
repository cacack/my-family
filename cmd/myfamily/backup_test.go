package main

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/backup"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/storage"
)

// servedDatabase opens a SQLite database as `serve` does, logs an event and
// leaves the stores open, as a running server would.
func servedDatabase(t *testing.T) *config.Config {
	t.Helper()
	cfg := &config.Config{SQLitePath: filepath.Join(t.TempDir(), "myfamily.db")}
	stores, err := storage.Open(cfg, storage.AccessServe)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stores.Close() })
	p := domain.NewPerson("Ada", "Lovelace")
	if err := stores.Events.Append(context.Background(), p.ID, "Person", []domain.Event{domain.NewPersonCreated(p)}, -1, repository.MainScope); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestWriteBackupThenVerify(t *testing.T) {
	cfg := servedDatabase(t)
	path := filepath.Join(t.TempDir(), "backup.zip")
	var out bytes.Buffer
	if err := writeBackup(context.Background(), cfg, path, &out); err != nil {
		t.Fatalf("writeBackup: %v", err)
	}
	if !strings.Contains(out.String(), "1 events") || !strings.Contains(out.String(), "keep it as safe as the database") {
		t.Errorf("output = %q", out.String())
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("archive mode = %v, want 0600", perm)
	}

	out.Reset()
	if err := runVerifyBackup([]string{path}, &out); err != nil {
		t.Fatalf("verify-backup of an intact archive: %v", err)
	}
	if !strings.Contains(out.String(), "is intact: 1 events") {
		t.Errorf("verify output = %q", out.String())
	}

	// One changed byte inside the event log is reported, naming the entry.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Name != backup.EventsEntry {
			continue
		}
		offset, err := f.DataOffset()
		if err != nil {
			t.Fatal(err)
		}
		data[offset+int64(f.CompressedSize64)/2] ^= 0x01
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	err = runVerifyBackup([]string{path}, &out)
	if err == nil || !strings.Contains(err.Error(), backup.EventsEntry) {
		t.Errorf("verify-backup of a damaged archive = %v, want an error naming %s", err, backup.EventsEntry)
	}
}

func TestWriteBackup_KeepsAnExistingFile(t *testing.T) {
	cfg := servedDatabase(t)
	path := filepath.Join(t.TempDir(), "backup.zip")
	if err := os.WriteFile(path, []byte("earlier"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeBackup(context.Background(), cfg, path, &bytes.Buffer{}); err == nil {
		t.Fatal("writeBackup over an existing file: want error")
	}
	if got, _ := os.ReadFile(path); string(got) != "earlier" {
		t.Errorf("existing file = %q, want it untouched", got)
	}
}

func TestWriteBackup_FailureLeavesNoFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backup.zip")
	cfg := &config.Config{SQLitePath: filepath.Join(t.TempDir(), "missing.db")}
	if err := writeBackup(context.Background(), cfg, path, &bytes.Buffer{}); err == nil {
		t.Fatal("writeBackup of a missing database: want error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("a failed backup left %s (stat err = %v)", path, err)
	}
}

func TestBackupUsage(t *testing.T) {
	for _, args := range [][]string{nil, {""}, {"a", "b"}} {
		if err := runBackup(args, &bytes.Buffer{}); err == nil || err.Error() != backupUsage {
			t.Errorf("runBackup(%q) = %v, want usage", args, err)
		}
		if err := runVerifyBackup(args, &bytes.Buffer{}); err == nil || err.Error() != verifyBackupUsage {
			t.Errorf("runVerifyBackup(%q) = %v, want usage", args, err)
		}
	}
}

func TestRunBackup_FromEnvironment(t *testing.T) {
	cfg := servedDatabase(t)
	t.Setenv("SQLITE_PATH", cfg.SQLitePath)
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DEMO_MODE", "")
	path := filepath.Join(t.TempDir(), "backup.zip")
	if err := runBackup([]string{path}, &bytes.Buffer{}); err != nil {
		t.Fatalf("runBackup: %v", err)
	}
	if _, err := backup.VerifyFile(path); err != nil {
		t.Errorf("VerifyFile: %v", err)
	}
}
