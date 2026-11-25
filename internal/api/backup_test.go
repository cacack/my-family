package api_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/backup"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository"
)

var accountsConfig = &config.Config{Port: 8080, LogFormat: "text", AuthMode: config.AuthModeAccounts}

// tempBackups points os.CreateTemp at a fresh directory and returns it, so a
// test can see the server's temporary backup files.
func tempBackups(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	return dir
}

func TestDownloadBackup_Owner(t *testing.T) {
	dir := tempBackups(t)
	const archive = "PK fake archive"
	f := newAuthFixtureWith(t, accountsConfig, api.WithBackup(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, archive)
		return err
	}))
	f.createOwner(t)
	rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/backup", cookie: f.signIn(t)})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if got := rec.Body.String(); got != archive {
		t.Errorf("body = %q, want the archive", got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, `attachment; filename="myfamily-backup-`) || !strings.HasSuffix(cd, `.zip"`) {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if rec.Header().Get("Content-Length") != "15" {
		t.Errorf("Content-Length = %q, want 15", rec.Header().Get("Content-Length"))
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "myfamily-backup-*")); len(left) != 0 {
		t.Errorf("temporary backup files left behind: %v", left)
	}
}

func TestDownloadBackup_OwnerOnly(t *testing.T) {
	called := false
	f := newAuthFixtureWith(t, accountsConfig, api.WithBackup(func(context.Context, io.Writer) error {
		called = true
		return nil
	}))
	f.createOwner(t)
	for _, role := range []repository.Role{repository.RoleEditor, repository.RoleViewer} {
		rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/backup", cookie: f.addMember(t, role)})
		if !forbidden(rec) {
			t.Errorf("as %s: %d %s, want forbidden", role, rec.Code, rec.Body)
		}
	}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/backup"}); rec.Code != http.StatusUnauthorized {
		t.Errorf("signed out: status = %d, want 401", rec.Code)
	}
	if called {
		t.Error("a backup was written for someone who is not the owner")
	}
}

func TestDownloadBackup_Unavailable(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/backup", cookie: f.signIn(t)})
	if rec.Code != http.StatusConflict || errorCodeOf(rec) != "backup_unavailable" {
		t.Errorf("without a backup writer: %d %s, want 409 backup_unavailable", rec.Code, rec.Body)
	}
}

// TestDownloadBackup_Failure: a backup that fails for any reason but a
// refusal the owner can fix is a server error, and nothing partial is sent.
func TestDownloadBackup_Failure(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"io", errors.New("disk on fire")},
		{"canceled", fmt.Errorf("check the read model for values no event records: %w", context.Canceled)},
		{"failed check", errors.New("check the read model for values no event records (if it awaits `myfamily rebuild`, run that first): boom")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := tempBackups(t)
			f := newAuthFixtureWith(t, accountsConfig, api.WithBackup(func(_ context.Context, w io.Writer) error {
				_, _ = io.WriteString(w, "partial")
				return tc.err
			}))
			f.createOwner(t)
			rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/backup", cookie: f.signIn(t)})
			if rec.Code != http.StatusInternalServerError {
				t.Errorf("status = %d, want 500", rec.Code)
			}
			if strings.Contains(rec.Body.String(), "partial") {
				t.Errorf("a failed backup was sent: %s", rec.Body)
			}
			if left, _ := filepath.Glob(filepath.Join(dir, "myfamily-backup-*")); len(left) != 0 {
				t.Errorf("temporary backup files left behind: %v", left)
			}
		})
	}
}

// TestDownloadBackup_Refused: each reason storage refuses a backup for is a
// 409 naming the fix, matched by sentinel however it is wrapped, not a failed
// download (#1830, #1834).
func TestDownloadBackup_Refused(t *testing.T) {
	for _, tc := range []struct {
		name, code, fix string
		err             error
	}{
		{"store schema newer", "backup_schema_newer", "newer release", repository.ErrStoreSchemaNewer},
		{"store schema older", "backup_needs_restart", "Restart the server", repository.ErrStoreSchemaOlder},
		{"older table shape", "backup_needs_restart", "Restart the server", backup.ErrSourceShape},
		{"unstamped read model", "backup_needs_rebuild", "myfamily rebuild", backup.ErrReadModelUnstamped},
		{"read-model-only values", "backup_needs_rebuild", "myfamily rebuild", backup.ErrReadModelOnlyValues},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := tempBackups(t)
			f := newAuthFixtureWith(t, accountsConfig, api.WithBackup(func(_ context.Context, w io.Writer) error {
				_, _ = io.WriteString(w, "partial")
				return fmt.Errorf("back up SQLite database %q: %w: detail", "/secret/path.db", tc.err)
			}))
			f.createOwner(t)
			rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/backup", cookie: f.signIn(t)})
			if rec.Code != http.StatusConflict || errorCodeOf(rec) != tc.code {
				t.Fatalf("%d %s, want 409 %s", rec.Code, rec.Body, tc.code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, tc.fix) {
				t.Errorf("message does not name the fix %q: %s", tc.fix, body)
			}
			if strings.Contains(body, "partial") || strings.Contains(body, "/secret/path.db") {
				t.Errorf("refusal leaks the archive or the database path: %s", body)
			}
			if left, _ := filepath.Glob(filepath.Join(dir, "myfamily-backup-*")); len(left) != 0 {
				t.Errorf("temporary backup files left behind: %v", left)
			}
		})
	}
}
