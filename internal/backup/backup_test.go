package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/postgres"
	"github.com/cacack/my-family/internal/repository/sqlite"
	"github.com/cacack/my-family/internal/testutil/pgtest"
)

// leftOutTables are the tables a backup does not carry (ADR-010 §2): the read
// model, the registries the log rebuilds, the bookkeeping, and the
// credentials that are issued again. Every table a backend creates is either
// here or carried, so a new table fails TestEveryTableIsCarriedOrLeftOut
// until it is placed.
var leftOutTables = []string{
	"persons", "families", "family_children", "pedigree_edges", "person_names", "person_external_ids",
	"family_external_ids", "life_events", "attributes", "associations", "sources", "source_external_ids",
	"citations", "citation_external_ids", "notes", "media", "evidence_analyses", "evidence_conflicts",
	"research_logs", "proof_summaries", "duplicate_dismissals", "submitters", "repositories",
	"repository_external_ids", "lds_ordinances", "places", "place_aliases", "projection_failures",
	"branches", "snapshots",
	"sessions", "password_resets", "invites",
	"rebuild_state", "read_model_schema", "store_schema",
}

// fixture is an opened database with every store constructed.
type fixture struct {
	db       *sql.DB
	dialect  Dialect
	events   repository.EventStore
	accounts repository.AccountStore
	settings repository.SettingsStore
}

func openSQLite(t *testing.T) fixture {
	t.Helper()
	db, err := sqlite.OpenDB(filepath.Join(t.TempDir(), "backup.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	f := fixture{db: db, dialect: SQLite}
	must(t, func() (err error) { _, err = sqlite.NewReadModelStore(db); return })
	must(t, func() (err error) { f.events, err = sqlite.NewEventStore(db); return })
	must(t, func() (err error) { _, err = sqlite.NewSnapshotStore(db); return })
	must(t, func() (err error) { _, err = sqlite.NewBranchStore(db); return })
	must(t, func() (err error) { f.settings, err = sqlite.NewSettingsStore(db); return })
	must(t, func() (err error) { f.accounts, err = sqlite.NewAccountStore(db); return })
	return f
}

func openPostgres(t *testing.T) fixture {
	t.Helper()
	db, err := postgres.OpenDB(pgtest.NewDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	f := fixture{db: db, dialect: Postgres}
	must(t, func() (err error) { _, err = postgres.NewReadModelStore(db); return })
	must(t, func() (err error) { f.events, err = postgres.NewEventStore(db); return })
	must(t, func() (err error) { _, err = postgres.NewSnapshotStore(db); return })
	must(t, func() (err error) { _, err = postgres.NewBranchStore(db); return })
	must(t, func() (err error) { f.settings, err = postgres.NewSettingsStore(db); return })
	must(t, func() (err error) { f.accounts, err = postgres.NewAccountStore(db); return })
	return f
}

func must(t *testing.T, fn func() error) {
	t.Helper()
	if err := fn(); err != nil {
		t.Fatal(err)
	}
}

var backends = []struct {
	name string
	open func(*testing.T) fixture
}{
	{"sqlite", openSQLite},
	{"postgres", openPostgres},
}

// seed fills every carried and left-out account table with one row, and
// logs a few events.
func seed(t *testing.T, f fixture) {
	t.Helper()
	ctx := context.Background()
	for _, name := range []string{"Ada", "Grace"} {
		p := domain.NewPerson(name, "Lovelace")
		must(t, func() error {
			return f.events.Append(ctx, p.ID, "Person", []domain.Event{domain.NewPersonCreated(p)}, -1, repository.MainScope)
		})
	}
	now := time.Date(2026, 11, 24, 12, 30, 45, 123456789, time.UTC)
	owner := &repository.User{ID: uuid.New(), Username: "owner", DisplayName: "Owner", PasswordHash: "$argon2id$hash", CreatedAt: now}
	must(t, func() error { return f.accounts.CreateUser(ctx, owner, repository.DefaultTreeID, repository.RoleOwner) })
	must(t, func() error {
		return f.accounts.CreateSession(ctx, &repository.Session{TokenHash: "session-hash", UserID: owner.ID, CreatedAt: now, LastSeenAt: now})
	})
	must(t, func() error {
		return f.accounts.CreateAPIToken(ctx, &repository.APIToken{ID: uuid.New(), TokenHash: "token-hash", UserID: owner.ID, Name: "script", CreatedAt: now})
	})
	must(t, func() error {
		return f.accounts.CreateInvite(ctx, &repository.Invite{TokenHash: "invite-hash", TreeID: repository.DefaultTreeID, Role: repository.RoleViewer,
			CreatedBy: owner.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
	})
	must(t, func() error {
		return f.accounts.CreatePasswordReset(ctx, &repository.PasswordReset{TokenHash: "reset-hash", UserID: owner.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
	})
	must(t, func() error {
		return f.accounts.AppendAuthAudit(ctx, &repository.AuthAuditEntry{At: now, Action: repository.AuthActionSignIn, ActorID: owner.ID, Username: "owner"})
	})
	must(t, func() error {
		return f.accounts.SaveTreeSettings(ctx, repository.DefaultTreeID, repository.TreeSettings{LivingThresholdYears: 100, ViewersSeeLiving: true, AnonymousAccess: repository.AnonymousAccessNone})
	})
	must(t, func() error { return f.settings.SetHomePerson(ctx, repository.SiteWide, uuid.New()) })
	must(t, func() error { return f.settings.SetHomePerson(ctx, owner.ID, uuid.New()) })
}

func writeBackup(t *testing.T, f fixture) ([]byte, *Manifest) {
	t.Helper()
	var buf bytes.Buffer
	m, err := Write(context.Background(), f.db, f.dialect, &buf, "test-version")
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	return buf.Bytes(), m
}

func readEntry(t *testing.T, archive []byte, name string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	rc, err := zr.Open(name)
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestWrite(t *testing.T) {
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			f := b.open(t)
			seed(t, f)
			archive, m := writeBackup(t, f)

			stored, err := f.events.ReadAll(context.Background(), 0, 1000)
			if err != nil {
				t.Fatal(err)
			}
			if m.EventCount != int64(len(stored)) || m.EventCount == 0 {
				t.Errorf("event_count = %d, store holds %d", m.EventCount, len(stored))
			}
			if last := stored[len(stored)-1].Position; m.LastPosition != last {
				t.Errorf("last_position = %d, store's last position is %d", m.LastPosition, last)
			}
			if m.FormatVersion != FormatVersion || m.AppVersion != "test-version" {
				t.Errorf("manifest = %+v", m)
			}

			// The manifest in the archive is the one returned.
			var inArchive Manifest
			if err := json.Unmarshal(readEntry(t, archive, ManifestName), &inArchive); err != nil {
				t.Fatal(err)
			}
			if inArchive.EventCount != m.EventCount || inArchive.LastPosition != m.LastPosition || len(inArchive.Entries) != len(EntryNames()) {
				t.Errorf("archived manifest = %+v, want %+v", inArchive, m)
			}

			// Each event line keeps every column, with data as JSON.
			lines := strings.Split(strings.TrimSpace(string(readEntry(t, archive, EventsEntry))), "\n")
			var first map[string]json.RawMessage
			if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
				t.Fatal(err)
			}
			for _, c := range eventsTable.columns {
				if _, ok := first[c.name]; !ok {
					t.Errorf("event line lacks %s: %s", c.name, lines[0])
				}
			}
			if !strings.HasPrefix(string(first["data"]), "{") {
				t.Errorf("data is not embedded as JSON: %s", first["data"])
			}

			// Values are logical: the same on either backend.
			var users []map[string]any
			if err := json.Unmarshal(readEntry(t, archive, TableEntry("users")), &users); err != nil {
				t.Fatal(err)
			}
			if len(users) != 1 || users[0]["created_at"] != "2026-11-24T12:30:45.123456789Z" || users[0]["disabled"] != false {
				t.Errorf("users = %v", users)
			}
			var trees []map[string]any
			if err := json.Unmarshal(readEntry(t, archive, TableEntry("tree_settings")), &trees); err != nil {
				t.Fatal(err)
			}
			if len(trees) != 1 || trees[0]["viewers_see_living"] != true || trees[0]["living_threshold_years"] != float64(100) {
				t.Errorf("tree_settings = %v", trees)
			}
			for _, carriedName := range []string{"tree_memberships", "site_settings", "account_settings", "auth_audit", "api_tokens"} {
				var rows []map[string]any
				if err := json.Unmarshal(readEntry(t, archive, TableEntry(carriedName)), &rows); err != nil {
					t.Fatal(err)
				}
				if len(rows) != 1 {
					t.Errorf("%s holds %d rows, want 1", carriedName, len(rows))
				}
			}

			// No row of the left-out credential tables is anywhere in it.
			for _, secret := range []string{"session-hash", "invite-hash", "reset-hash"} {
				for _, name := range EntryNames() {
					if bytes.Contains(readEntry(t, archive, name), []byte(secret)) {
						t.Errorf("%s holds %q", name, secret)
					}
				}
			}

			if _, err := Verify(bytes.NewReader(archive), int64(len(archive))); err != nil {
				t.Errorf("Verify: %v", err)
			}
		})
	}
}

func TestWrite_EmptyDatabase(t *testing.T) {
	f := openSQLite(t)
	archive, m := writeBackup(t, f)
	if m.EventCount != 0 || m.LastPosition != 0 {
		t.Errorf("manifest = %+v, want an empty log", m)
	}
	if got := string(readEntry(t, archive, TableEntry("users"))); got != "[]\n" {
		t.Errorf("users = %q, want an empty array", got)
	}
	if _, err := Verify(bytes.NewReader(archive), int64(len(archive))); err != nil {
		t.Errorf("Verify: %v", err)
	}
}

// TestWrite_ConcurrentAppend appends while the backup reads: the archive is a
// consistent snapshot either way.
func TestWrite_ConcurrentAppend(t *testing.T) {
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			f := b.open(t)
			seed(t, f)
			done := make(chan error)
			go func() {
				p := domain.NewPerson("Late", "Writer")
				done <- f.events.Append(context.Background(), p.ID, "Person", []domain.Event{domain.NewPersonCreated(p)}, -1, repository.MainScope)
			}()
			archive, _ := writeBackup(t, f)
			if err := <-done; err != nil {
				t.Fatalf("append beside the backup: %v", err)
			}
			if _, err := Verify(bytes.NewReader(archive), int64(len(archive))); err != nil {
				t.Errorf("Verify: %v", err)
			}
		})
	}
}

func TestVerify_DamagedByte(t *testing.T) {
	f := openSQLite(t)
	seed(t, f)
	archive, _ := writeBackup(t, f)
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range zr.File {
		t.Run(file.Name, func(t *testing.T) {
			offset, err := file.DataOffset()
			if err != nil {
				t.Fatal(err)
			}
			damaged := slices.Clone(archive)
			damaged[offset+int64(file.CompressedSize64)/2] ^= 0xff
			_, err = Verify(bytes.NewReader(damaged), int64(len(damaged)))
			if err == nil {
				t.Fatal("Verify accepted a damaged archive")
			}
			if !strings.Contains(err.Error(), file.Name) {
				t.Errorf("error %q does not name %s", err, file.Name)
			}
		})
	}
}

// rezip rewrites archive with edit applied to its manifest and entries.
func rezip(t *testing.T, archive []byte, edit func(name string, data []byte) ([]byte, bool)) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, file := range zr.File {
		data := readEntry(t, archive, file.Name)
		data, keep := edit(file.Name, data)
		if !keep {
			continue
		}
		w, err := zw.Create(file.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func editManifest(t *testing.T, change func(*Manifest)) func(string, []byte) ([]byte, bool) {
	return func(name string, data []byte) ([]byte, bool) {
		if name != ManifestName {
			return data, true
		}
		var m Manifest
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		change(&m)
		out, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		return out, true
	}
}

func TestVerify_Refusals(t *testing.T) {
	f := openSQLite(t)
	seed(t, f)
	archive, _ := writeBackup(t, f)
	users := TableEntry("users")

	tests := []struct {
		name    string
		archive []byte
		want    string
	}{
		{"not a zip", []byte("not a zip"), "not a readable zip"},
		{"no manifest", rezip(t, archive, func(name string, data []byte) ([]byte, bool) { return data, name != ManifestName }), "manifest.json is missing"},
		{"listed entry missing", rezip(t, archive, func(name string, data []byte) ([]byte, bool) { return data, name != users }), users + " is listed in the manifest but missing"},
		{"unlisted entry", rezip(t, archive, editManifest(t, func(m *Manifest) {
			m.Entries = slices.DeleteFunc(m.Entries, func(e Entry) bool { return e.Name == users })
		})), users + " is not listed"},
		{"events unlisted", rezip(t, archive, func(name string, data []byte) ([]byte, bool) {
			if name == EventsEntry {
				return data, false
			}
			return editManifest(t, func(m *Manifest) {
				m.Entries = slices.DeleteFunc(m.Entries, func(e Entry) bool { return e.Name == EventsEntry })
			})(name, data)
		}), EventsEntry + " is missing from the manifest"},
		{"newer format", rezip(t, archive, editManifest(t, func(m *Manifest) { m.FormatVersion = FormatVersion + 1 })), "newer than this release reads"},
		{"no format", rezip(t, archive, editManifest(t, func(m *Manifest) { m.FormatVersion = 0 })), "format_version 0"},
		{"wrong count", rezip(t, archive, editManifest(t, func(m *Manifest) { m.EventCount++ })), "the manifest says"},
		{"wrong last position", rezip(t, archive, editManifest(t, func(m *Manifest) { m.LastPosition++ })), "ends at position"},
		{"manifest not JSON", rezip(t, archive, func(name string, data []byte) ([]byte, bool) {
			if name == ManifestName {
				return []byte("{"), true
			}
			return data, true
		}), "manifest.json is damaged"},
		{"event line not an event", rezip(t, archive, func(name string, data []byte) ([]byte, bool) {
			if name == EventsEntry {
				return append(data, []byte("{}\n")...), true
			}
			return data, true
		}), "line 3 is not an event"},
		{"events out of order", rezip(t, archive, func(name string, data []byte) ([]byte, bool) {
			if name == EventsEntry {
				lines := strings.SplitAfter(string(data), "\n")
				return []byte(lines[1] + lines[0]), true
			}
			return data, true
		}), "out of position order"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Verify(bytes.NewReader(tt.archive), int64(len(tt.archive)))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Verify = %v, want an error containing %q", err, tt.want)
			}
		})
	}
}

func TestVerifyFile(t *testing.T) {
	if _, err := VerifyFile(filepath.Join(t.TempDir(), "missing.zip")); err == nil {
		t.Error("VerifyFile of a missing file: want error")
	}
}

// TestEveryTableIsCarriedOrLeftOut is the ADR-010 §2 checklist: every table a
// backend creates is carried by the archive or deliberately left out.
func TestEveryTableIsCarriedOrLeftOut(t *testing.T) {
	// SQLite's full-text indexes (persons_fts and the like, with their shadow
	// tables) are read model too.
	queries := map[Dialect]string{
		SQLite:   `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE '%_fts%'`,
		Postgres: `SELECT table_name FROM information_schema.tables WHERE table_schema = 'public'`,
	}
	known := map[string]bool{eventsTable.name: true, streamsTable.name: true}
	for _, ct := range carriedTables {
		known[ct.name] = true
	}
	for _, name := range leftOutTables {
		known[name] = true
	}
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			f := b.open(t)
			for _, name := range queryStrings(t, f.db, queries[f.dialect]) {
				if !known[name] {
					t.Errorf("table %s is neither carried nor left out; place it in ADR-010 and here", name)
				}
			}
		})
	}
}

// TestCarriedColumnsMatchSchema keeps each carried table's column list equal
// to the backend's, so no column is silently left out.
func TestCarriedColumnsMatchSchema(t *testing.T) {
	queries := map[Dialect]string{
		SQLite:   `SELECT name FROM pragma_table_info($1)`,
		Postgres: `SELECT column_name FROM information_schema.columns WHERE table_schema = 'public' AND table_name = $1`,
	}
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			f := b.open(t)
			for _, tbl := range append([]table{eventsTable, streamsTable}, carriedTables...) {
				got := queryStrings(t, f.db, queries[f.dialect], tbl.name)
				var want []string
				for _, c := range tbl.columns {
					want = append(want, c.name)
				}
				sort.Strings(got)
				sort.Strings(want)
				if !slices.Equal(got, want) {
					t.Errorf("%s columns = %v, the archive carries %v", tbl.name, got, want)
				}
			}
		})
	}
}

func queryStrings(t *testing.T, db *sql.DB, query string, args ...any) []string {
	t.Helper()
	rows, err := db.Query(query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestLogical(t *testing.T) {
	when := time.Date(2026, 1, 2, 3, 4, 5, 6, time.FixedZone("x", 3600))
	tests := []struct {
		name string
		kind kind
		in   any
		want any
		err  bool
	}{
		{"null", kindText, nil, nil, false},
		{"text bytes", kindText, []byte("a"), "a", false},
		{"int", kindInt, int64(7), int64(7), false},
		{"bool int", kindBool, int64(1), true, false},
		{"bool", kindBool, false, false, false},
		{"bool out of range", kindBool, int64(2), nil, true},
		{"time", kindTime, when, "2026-01-02T02:04:05.000000006Z", false},
		{"sqlite datetime", kindTime, "2026-01-02 03:04:05", "2026-01-02T03:04:05.000000000Z", false},
		{"rfc3339 offset", kindTime, "2026-01-02T03:04:05+01:00", "2026-01-02T02:04:05.000000000Z", false},
		{"bad time", kindTime, "yesterday", nil, true},
		{"json", kindJSON, []byte(`{"a":1}`), json.RawMessage(`{"a":1}`), false},
		{"bad json", kindJSON, "{", nil, true},
		{"wrong type", kindInt, "7", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := logical(tt.kind, tt.in)
			if (err != nil) != tt.err {
				t.Fatalf("err = %v, want error %v", err, tt.err)
			}
			if gb, ok := got.(json.RawMessage); ok {
				if string(gb) != string(tt.want.(json.RawMessage)) {
					t.Errorf("got %s, want %s", gb, tt.want)
				}
				return
			}
			if got != tt.want {
				t.Errorf("got %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}
