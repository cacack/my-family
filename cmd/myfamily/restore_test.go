package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/storage"
)

func TestRestoreBackup(t *testing.T) {
	source := servedDatabase(t)
	archive := filepath.Join(t.TempDir(), "backup.zip")
	if err := writeBackup(context.Background(), source, archive, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	target := &config.Config{SQLitePath: filepath.Join(t.TempDir(), "restored.db")}
	var out bytes.Buffer
	if err := restoreBackup(context.Background(), target, archive, &out); err != nil {
		t.Fatalf("restoreBackup: %v", err)
	}
	if !strings.Contains(out.String(), "1 events") || !strings.Contains(out.String(), "Everyone signs in again") {
		t.Errorf("output = %q", out.String())
	}

	// A second restore finds the database holds data, and the source is
	// still held by its server.
	if err := restoreBackup(context.Background(), target, archive, &bytes.Buffer{}); err == nil {
		t.Error("restore into the restored database: want error")
	}
	if err := restoreBackup(context.Background(), source, archive, &bytes.Buffer{}); !errors.Is(err, storage.ErrServerRunning) {
		t.Errorf("restore into a served database = %v, want ErrServerRunning", err)
	}
}

func TestRunRestore(t *testing.T) {
	for _, args := range [][]string{nil, {""}, {"a", "b"}} {
		if err := runRestore(args, &bytes.Buffer{}); err == nil || err.Error() != restoreUsage {
			t.Errorf("runRestore(%q) = %v, want usage", args, err)
		}
	}
	t.Setenv("SQLITE_PATH", filepath.Join(t.TempDir(), "restored.db"))
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DEMO_MODE", "")
	if err := runRestore([]string{filepath.Join(t.TempDir(), "missing.zip")}, &bytes.Buffer{}); err == nil {
		t.Error("runRestore of a missing archive: want error")
	}
}
