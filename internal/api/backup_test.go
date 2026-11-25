package api_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
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

func TestDownloadBackup_Failure(t *testing.T) {
	dir := tempBackups(t)
	f := newAuthFixtureWith(t, accountsConfig, api.WithBackup(func(_ context.Context, w io.Writer) error {
		_, _ = io.WriteString(w, "partial")
		return errors.New("disk on fire")
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
}
