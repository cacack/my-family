package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// saveArchive writes archive to a file and opens it.
func saveArchive(t *testing.T, archive []byte) *Archive {
	t.Helper()
	path := filepath.Join(t.TempDir(), "backup.zip")
	if err := os.WriteFile(path, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := OpenArchive(path)
	if err != nil {
		t.Fatalf("OpenArchive: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

// assertEmpty fails unless f's log and carried tables hold no row.
func assertEmpty(t *testing.T, f fixture) {
	t.Helper()
	for _, tbl := range append([]table{eventsTable, streamsTable}, carriedTables...) {
		var n int
		if err := f.db.QueryRow("SELECT COUNT(*) FROM " + tbl.name).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%s holds %d rows after a refused restore, want 0", tbl.name, n)
		}
	}
}

// TestLoad_RoundTrip restores a backup into an empty database of the same
// backend and backs that up again: every entry but the manifest is identical,
// so the log (positions included) and every carried row came back as they were.
func TestLoad_RoundTrip(t *testing.T) {
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			ctx := context.Background()
			source := b.open(t)
			seed(t, source)
			// A gap in the positions, which restore must keep.
			if _, err := source.db.Exec("UPDATE events SET position = position + 10 WHERE position > 1"); err != nil {
				t.Fatal(err)
			}
			archive, m := writeBackup(t, source)

			target := b.open(t)
			if err := saveArchive(t, archive).Load(ctx, target.db, target.dialect); err != nil {
				t.Fatalf("Load: %v", err)
			}
			again, m2 := writeBackup(t, target)
			if m2.EventCount != m.EventCount || m2.LastPosition != m.LastPosition {
				t.Errorf("restored manifest = %+v, want %+v", m2, m)
			}
			for _, name := range EntryNames() {
				if want, got := readEntry(t, archive, name), readEntry(t, again, name); !bytes.Equal(want, got) {
					t.Errorf("%s differs after restore\n before: %s\n after:  %s", name, want, got)
				}
			}

			// New writes take fresh keys: the log's next position follows the
			// restored ones, and an audit entry does not collide.
			p := domain.NewPerson("After", "Restore")
			must(t, func() error {
				return target.events.Append(ctx, p.ID, "Person", []domain.Event{domain.NewPersonCreated(p)}, -1, repository.MainScope)
			})
			stored, err := target.events.ReadAll(ctx, m.LastPosition, 10)
			if err != nil || len(stored) != 1 {
				t.Fatalf("events after the restored log = %v, %v; want the new one", stored, err)
			}
			must(t, func() error {
				return target.accounts.AppendAuthAudit(ctx, &repository.AuthAuditEntry{At: time.Now(), Action: repository.AuthActionSignIn, Username: "owner"})
			})
			audit, err := target.accounts.ListAuthAudit(ctx, 10, 0)
			if err != nil || len(audit) != 2 {
				t.Errorf("audit log = %v, %v; want the restored entry and the new one", audit, err)
			}
		})
	}
}

func TestLoad_RefusesADatabaseWithData(t *testing.T) {
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			source := b.open(t)
			seed(t, source)
			archive, _ := writeBackup(t, source)

			target := b.open(t)
			must(t, func() error {
				return target.accounts.SaveTreeSettings(context.Background(), repository.DefaultTreeID,
					repository.TreeSettings{LivingThresholdYears: 90, AnonymousAccess: repository.AnonymousAccessNone})
			})
			err := saveArchive(t, archive).Load(context.Background(), target.db, target.dialect)
			if !errors.Is(err, ErrNotEmpty) || !strings.Contains(err.Error(), "tree_settings") {
				t.Fatalf("Load = %v, want ErrNotEmpty naming tree_settings", err)
			}
			settings, err := target.accounts.GetTreeSettings(context.Background(), repository.DefaultTreeID)
			if err != nil || settings.LivingThresholdYears != 90 {
				t.Errorf("tree settings = %+v, %v; want them unchanged", settings, err)
			}
			var events int
			if err := target.db.QueryRow("SELECT COUNT(*) FROM events").Scan(&events); err != nil || events != 0 {
				t.Errorf("events = %d, %v; want none", events, err)
			}
		})
	}
}

// resum rewrites archive with edit applied to its entries and the manifest's
// sums recomputed, as an archive from another release would be.
func resum(t *testing.T, archive []byte, edit func(name string, data []byte) []byte) []byte {
	t.Helper()
	sums := map[string]string{}
	edited := rezip(t, archive, func(name string, data []byte) ([]byte, bool) {
		if name == ManifestName {
			return data, true
		}
		data = edit(name, data)
		sum := sha256.Sum256(data)
		sums[name] = hex.EncodeToString(sum[:])
		return data, true
	})
	return rezip(t, edited, editManifest(t, func(m *Manifest) {
		for i := range m.Entries {
			m.Entries[i].SHA256 = sums[m.Entries[i].Name]
		}
	}))
}

// TestLoad_ByColumnName: a column the archive lacks takes its default, and a
// column this release does not know fails the restore, which rolls back.
func TestLoad_ByColumnName(t *testing.T) {
	ctx := context.Background()
	source := openSQLite(t)
	seed(t, source)
	archive, _ := writeBackup(t, source)
	users := TableEntry("users")

	t.Run("missing column takes its default", func(t *testing.T) {
		older := resum(t, archive, func(name string, data []byte) []byte {
			if name == users {
				return bytes.Replace(data, []byte(`,"disabled":false`), nil, 1)
			}
			return data
		})
		target := openSQLite(t)
		if err := saveArchive(t, older).Load(ctx, target.db, target.dialect); err != nil {
			t.Fatalf("Load: %v", err)
		}
		user, err := target.accounts.GetUserByUsername(ctx, "owner")
		if err != nil || user == nil || user.Disabled {
			t.Errorf("user = %+v, %v; want an enabled owner", user, err)
		}
	})

	t.Run("unknown column fails", func(t *testing.T) {
		newer := resum(t, archive, func(name string, data []byte) []byte {
			if name == users {
				return bytes.Replace(data, []byte(`"disabled":false`), []byte(`"disabled":false,"shoe_size":9`), 1)
			}
			return data
		})
		target := openSQLite(t)
		err := saveArchive(t, newer).Load(ctx, target.db, target.dialect)
		if err == nil || !strings.Contains(err.Error(), "shoe_size") {
			t.Fatalf("Load = %v, want an error naming shoe_size", err)
		}
		assertEmpty(t, target)
	})
}

// TestLoad_ChangedAfterVerify: an entry whose content no longer matches the
// manifest is not loaded, and what was loaded before it rolls back.
func TestLoad_ChangedAfterVerify(t *testing.T) {
	source := openSQLite(t)
	seed(t, source)
	archive, _ := writeBackup(t, source)
	a := saveArchive(t, archive)
	a.sums[TableEntry("api_tokens")] = "0"
	target := openSQLite(t)
	err := a.Load(context.Background(), target.db, target.dialect)
	if err == nil || !strings.Contains(err.Error(), "changed after it was verified") {
		t.Fatalf("Load = %v, want a refusal", err)
	}
	assertEmpty(t, target)
}

func TestOpenArchive_Refusals(t *testing.T) {
	if _, err := OpenArchive(filepath.Join(t.TempDir(), "missing.zip")); err == nil {
		t.Error("OpenArchive of a missing file: want error")
	}
	path := filepath.Join(t.TempDir(), "damaged.zip")
	if err := os.WriteFile(path, []byte("not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenArchive(path); err == nil || !strings.Contains(err.Error(), "not a readable zip") {
		t.Errorf("OpenArchive of a damaged file = %v, want a verification error", err)
	}
}

func TestStoredValue(t *testing.T) {
	when := time.Date(2026, 1, 2, 3, 4, 5, 600000000, time.UTC)
	tests := []struct {
		name    string
		dialect Dialect
		kind    kind
		raw     string
		want    any
		err     bool
	}{
		{"null", SQLite, kindText, `null`, nil, false},
		{"text", Postgres, kindText, `"a"`, "a", false},
		{"int", SQLite, kindInt, `7`, int64(7), false},
		{"sqlite bool", SQLite, kindBool, `true`, int64(1), false},
		{"sqlite false", SQLite, kindBool, `false`, int64(0), false},
		{"postgres bool", Postgres, kindBool, `true`, true, false},
		{"bad bool", SQLite, kindBool, `1`, nil, true},
		{"sqlite account time", SQLite, kindTime, `"2026-01-02T03:04:05.600000000Z"`, "2026-01-02T03:04:05.600000000Z", false},
		{"sqlite log time", SQLite, kindLogTime, `"2026-01-02T03:04:05.600000000Z"`, "2026-01-02T03:04:05.6Z", false},
		{"sqlite datetime", SQLite, kindDatetime, `"2026-01-02T03:04:05.000000000Z"`, "2026-01-02 03:04:05", false},
		{"postgres time", Postgres, kindTime, `"2026-01-02T03:04:05.600000000Z"`, when, false},
		{"bad time", SQLite, kindTime, `"yesterday"`, nil, true},
		{"time not a string", SQLite, kindTime, `5`, nil, true},
		{"json", Postgres, kindJSON, `{"a":1}`, `{"a":1}`, false},
		{"unknown kind", SQLite, kind(99), `1`, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := storedValue(tt.dialect, tt.kind, json.RawMessage(tt.raw))
			if (err != nil) != tt.err {
				t.Fatalf("err = %v, want error %v", err, tt.err)
			}
			if tt.err {
				return
			}
			if wt, ok := tt.want.(time.Time); ok {
				if gt, ok := got.(time.Time); !ok || !gt.Equal(wt) {
					t.Errorf("got %v, want %v", got, wt)
				}
				return
			}
			if got != tt.want {
				t.Errorf("got %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}
