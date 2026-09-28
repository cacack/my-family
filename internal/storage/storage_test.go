package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository"
)

// The credentials in these tests are fake. They are assembled from pieces so
// that secret scanners do not report credential-shaped literals in this file.
const pwKey = "pass" + "word"

// userinfo returns the "user:password@" part of a PostgreSQL URL.
func userinfo(user, password string) string { return user + ":" + password + "@" }

// ============================================================================
// Selection
// ============================================================================

func TestSelect(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Config
		want Backend
	}{
		{"default is SQLite", config.Config{SQLitePath: "./myfamily.db"}, BackendSQLite},
		{"DATABASE_URL selects PostgreSQL", config.Config{DatabaseURL: "postgres://h/db", SQLitePath: "./myfamily.db"}, BackendPostgres},
		{"demo mode is memory", config.Config{DemoMode: true, SQLitePath: "./myfamily.db"}, BackendMemory},
		{"demo mode wins over DATABASE_URL", config.Config{DemoMode: true, DatabaseURL: "postgres://h/db"}, BackendMemory},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg
			if got := Select(&cfg); got != tt.want {
				t.Errorf("Select() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSelect_FromEnvironment proves the selection over the real config surface:
// the env vars `serve` reads, through config.Load.
func TestSelect_FromEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("SQLITE_PATH", "")
	t.Setenv("DEMO_MODE", "")
	if got := Select(config.Load()); got != BackendSQLite {
		t.Errorf("no env: Select() = %q, want sqlite", got)
	}

	t.Setenv("DATABASE_URL", "postgres://user@localhost/db")
	if got := Select(config.Load()); got != BackendPostgres {
		t.Errorf("DATABASE_URL: Select() = %q, want postgres", got)
	}

	t.Setenv("DEMO_MODE", "true")
	if got := Select(config.Load()); got != BackendMemory {
		t.Errorf("DEMO_MODE: Select() = %q, want memory", got)
	}
}

// ============================================================================
// Memory
// ============================================================================

func TestOpen_Memory(t *testing.T) {
	stores, err := Open(&config.Config{DemoMode: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if stores.Backend != BackendMemory {
		t.Errorf("Backend = %q, want memory", stores.Backend)
	}
	if stores.Memory == nil {
		t.Fatal("Memory stores are nil for the memory backend")
	}
	assertComplete(t, stores)
	if got := stores.Describe(); !strings.Contains(got, "In-memory") {
		t.Errorf("Describe() = %q, want it to say in-memory", got)
	}
	if err := stores.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}

	// A fresh memory store starts empty every time: that is what demo mode is.
	server := newServer(stores)
	createPerson(t, server, "Ada", "Memory")
	again := OpenMemory()
	if n := countPersons(t, newServer(again)); n != 0 {
		t.Errorf("fresh memory store holds %d persons, want 0", n)
	}
}

// ============================================================================
// SQLite
// ============================================================================

func TestOpen_SQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "myfamily.db")
	stores, err := Open(&config.Config{SQLitePath: path})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = stores.Close() })

	if stores.Backend != BackendSQLite {
		t.Errorf("Backend = %q, want sqlite", stores.Backend)
	}
	if stores.Memory != nil {
		t.Error("Memory stores are set for the SQLite backend")
	}
	assertComplete(t, stores)
	if got, want := stores.Describe(), "SQLite ("+path+")"; got != want {
		t.Errorf("Describe() = %q, want %q", got, want)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("database file was not created: %v", err)
	}
}

// TestSQLite_Restart is the persistence proof: data written through the real
// HTTP API survives closing every store and opening the same file again.
func TestSQLite_Restart(t *testing.T) {
	cfg := &config.Config{SQLitePath: filepath.Join(t.TempDir(), "myfamily.db")}
	assertSurvivesRestart(t, cfg)
}

func TestOpenSQLite_WithoutCgo(t *testing.T) {
	prev := sqliteAvailable
	sqliteAvailable = false
	t.Cleanup(func() { sqliteAvailable = prev })

	path := filepath.Join(t.TempDir(), "myfamily.db")
	_, err := Open(&config.Config{SQLitePath: path})
	if !errors.Is(err, ErrSQLiteUnavailable) {
		t.Fatalf("Open without cgo: err = %v, want ErrSQLiteUnavailable", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("a refused open must not create %s (stat err = %v)", path, statErr)
	}
}

func TestOpenSQLite_EmptyPath(t *testing.T) {
	if _, err := OpenSQLite("  "); err == nil || !strings.Contains(err.Error(), "SQLITE_PATH") {
		t.Fatalf("OpenSQLite(\"  \"): err = %v, want an error naming SQLITE_PATH", err)
	}
}

func TestOpenSQLite_MissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-dir", "myfamily.db")
	_, err := OpenSQLite(path)
	if err == nil {
		t.Fatal("OpenSQLite in a missing directory succeeded, want an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name the path", err)
	}
}

// TestOpenSQLite_SchemaFailureClosesDatabase covers a store whose DDL fails:
// Open must report which store failed and must not leave the database open.
func TestOpenSQLite_SchemaFailureClosesDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "myfamily.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// A view named like a read-model table makes that store's CREATE fail.
	if _, err := db.Exec(`CREATE VIEW persons AS SELECT 1 AS id`); err != nil {
		t.Fatalf("create view: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	_, err = OpenSQLite(path)
	if err == nil {
		t.Fatal("OpenSQLite over a conflicting schema succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "read model store") {
		t.Errorf("error %q does not name the failing store", err)
	}
}

// ============================================================================
// PostgreSQL
// ============================================================================

func TestOpen_Postgres(t *testing.T) {
	dsn := postgresDatabase(t)
	stores, err := Open(&config.Config{DatabaseURL: dsn, SQLitePath: "unused.db"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = stores.Close() })

	if stores.Backend != BackendPostgres {
		t.Errorf("Backend = %q, want postgres", stores.Backend)
	}
	assertComplete(t, stores)
	if got := stores.Describe(); !strings.HasPrefix(got, "PostgreSQL (") {
		t.Errorf("Describe() = %q", got)
	}
	if _, err := os.Stat("unused.db"); !os.IsNotExist(err) {
		t.Errorf("PostgreSQL mode touched SQLITE_PATH (stat err = %v)", err)
	}
}

func TestPostgres_Restart(t *testing.T) {
	assertSurvivesRestart(t, &config.Config{DatabaseURL: postgresDatabase(t)})
}

func TestOpenPostgres_Unreachable(t *testing.T) {
	// Port 1 on localhost refuses immediately; nothing listens there.
	_, err := OpenPostgres("postgres://" + userinfo("user", "s3cret") + "127.0.0.1:1/db?sslmode=disable&connect_timeout=2")
	if err == nil {
		t.Fatal("OpenPostgres against a closed port succeeded, want an error")
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Errorf("error leaks the password: %q", err)
	}
	if !strings.Contains(err.Error(), "127.0.0.1:1") {
		t.Errorf("error %q does not say where it tried to connect", err)
	}
}

func TestOpenPostgres_MalformedURLDoesNotLeakPassword(t *testing.T) {
	tests := []struct{ name, dsn, secret string }{
		// An unescaped % in the password: lib/pq's own error quotes the DSN.
		{"percent in password", "postgres://" + userinfo("app", "50%off") + "db.example:5432/myfamily", "50%off"},
		{"space in host", "postgres://" + userinfo("u", "secret") + "local host/db", "secret"},
		{"bad escape in query password", "postgres://u@h/db?" + pwKey + "=pa%zzss", "pa%zzss"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := OpenPostgres(tt.dsn)
			if err == nil {
				t.Fatal("OpenPostgres succeeded on a malformed URL, want an error")
			}
			msg := err.Error()
			// "%of"/"%zz" are fragments of the passwords that url.Error
			// would quote as the invalid escape.
			for _, leak := range []string{tt.secret, tt.dsn, "%of", "%zz"} {
				if strings.Contains(msg, leak) {
					t.Errorf("error leaks %q: %q", leak, msg)
				}
			}
			if !strings.Contains(msg, "DATABASE_URL is not a valid URL") {
				t.Errorf("error %q does not explain the problem", msg)
			}
		})
	}
}

func TestOpenPostgres_KeyValueUnreachableDoesNotLeakPassword(t *testing.T) {
	_, err := OpenPostgres("host=127.0.0.1 port=1 user=u " + pwKey + "='s3cret pw' dbname=db sslmode=disable connect_timeout=2")
	if err == nil {
		t.Fatal("OpenPostgres against a closed port succeeded, want an error")
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Errorf("error leaks the password: %q", err)
	}
}

func TestScrubError(t *testing.T) {
	base := errors.New("base")
	tests := []struct {
		name, dsn string
		errText   string
		secrets   []string
	}{
		{
			"url userinfo, raw and escaped",
			"postgres://" + userinfo("u", "p%40ss") + "h/db",
			`dial "postgres://` + userinfo("u", "p%40ss") + `h/db": auth failed for p@ss and p%40ss`,
			[]string{"p@ss", "p%40ss", "postgres://u:"},
		},
		{
			"query password",
			"postgres://u@h/db?" + pwKey + "=hunter2&sslmode=disable",
			"bad " + pwKey + " hunter2",
			[]string{"hunter2"},
		},
		{
			"key=value quoted",
			"host=h " + pwKey + "='top secret' dbname=d",
			"conn host=h " + pwKey + "='top secret' dbname=d failed; saw top secret",
			[]string{"top secret"},
		},
		{
			"key=value bare",
			"host=h " + pwKey + "=bare1 dbname=d",
			"failed near bare1",
			[]string{"bare1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := scrubError(fmt.Errorf("%s: %w", tt.errText, base), tt.dsn)
			for _, s := range tt.secrets {
				if strings.Contains(err.Error(), s) {
					t.Errorf("scrubbed error %q still contains %q", err, s)
				}
			}
			if !errors.Is(err, base) {
				t.Error("scrubbed error no longer unwraps to the original")
			}
		})
	}
}

// ============================================================================
// Helpers under test
// ============================================================================

func TestRedactPostgres(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"postgres://" + userinfo("user", "pw") + "host:5432/db", "postgres://" + userinfo("user", "xxxxx") + "host:5432/db"},
		{"postgresql://user@host/db?sslmode=disable", "postgresql://user@host/db?sslmode=disable"},
		{"postgres://host/db?" + pwKey + "=pw&sslmode=disable", "postgres://host/db?" + pwKey + "=xxxxx&sslmode=disable"},
		{"host=localhost " + pwKey + "=pw dbname=db", "connection string not shown"},
		{"://bad", "connection string not shown"},
		{"postgres://u@h/db?" + pwKey + "=pa%zzss", "connection string not shown"},
	}
	for _, tt := range tests {
		if got := redactPostgres(tt.in); got != tt.want {
			t.Errorf("redactPostgres(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBuild_ClosesDatabaseOnEachFailure(t *testing.T) {
	boom := errors.New("boom")
	stages := []string{"read model", "event", "snapshot", "branch"}
	for i, stage := range stages {
		t.Run(stage, func(t *testing.T) {
			db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "x.db"))
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			failAt := func(n int) error {
				if n == i {
					return boom
				}
				return nil
			}
			_, err = build(BackendSQLite, "x.db", db,
				func(*sql.DB) (repository.ReadModelStore, error) { return nil, failAt(0) },
				func(*sql.DB) (repository.EventStore, error) { return nil, failAt(1) },
				func(*sql.DB) (repository.SnapshotStore, error) { return nil, failAt(2) },
				func(*sql.DB) (repository.BranchStore, error) { return nil, failAt(3) },
			)
			if !errors.Is(err, boom) {
				t.Fatalf("build: err = %v, want boom", err)
			}
			if !strings.Contains(err.Error(), stage+" store") {
				t.Errorf("error %q does not name the %s store", err, stage)
			}
			if pingErr := db.Ping(); pingErr == nil {
				t.Error("database still open after a failed build")
			}
		})
	}
}

func TestClose_Idempotent(t *testing.T) {
	stores, err := OpenSQLite(filepath.Join(t.TempDir(), "myfamily.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	if err := stores.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := stores.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// ============================================================================
// Shared scenario
// ============================================================================

// assertSurvivesRestart writes through every store via the HTTP API, closes
// everything, reopens from the same config and reads it all back — a person
// (event log + read model + search), a snapshot and a branch.
func assertSurvivesRestart(t *testing.T, cfg *config.Config) {
	t.Helper()

	first, err := Open(cfg)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	server := newServer(first)
	personID := createPerson(t, server, "Persistent", "Ancestor")
	mustDo(t, server, http.MethodPost, "/api/v1/snapshots", `{"name":"Before restart"}`, http.StatusCreated)
	branch := mustDo(t, server, http.MethodPost, "/api/v1/branches", `{"name":"Restart research"}`, http.StatusCreated)
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second, err := Open(cfg)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })
	server = newServer(second)

	person := mustDo(t, server, http.MethodGet, "/api/v1/persons/"+personID, "", http.StatusOK)
	if person["given_name"] != "Persistent" || person["surname"] != "Ancestor" {
		t.Errorf("person after restart = %v", person)
	}
	if n := countPersons(t, server); n != 1 {
		t.Errorf("person count after restart = %d, want 1", n)
	}

	search := mustDo(t, server, http.MethodGet, "/api/v1/search?q=Ancestor", "", http.StatusOK)
	if items, _ := search["items"].([]any); len(items) != 1 {
		t.Errorf("search after restart = %v, want one hit", search)
	}

	history := mustDo(t, server, http.MethodGet, "/api/v1/persons/"+personID+"/history", "", http.StatusOK)
	if items, _ := history["items"].([]any); len(items) == 0 {
		t.Errorf("person history after restart is empty: %v", history)
	}

	snapshots := mustDo(t, server, http.MethodGet, "/api/v1/snapshots", "", http.StatusOK)
	if !strings.Contains(fmt.Sprint(snapshots), "Before restart") {
		t.Errorf("snapshot missing after restart: %v", snapshots)
	}

	branches := mustDo(t, server, http.MethodGet, "/api/v1/branches", "", http.StatusOK)
	if !strings.Contains(fmt.Sprint(branches), fmt.Sprint(branch["id"])) {
		t.Errorf("branch %v missing after restart: %v", branch["id"], branches)
	}

	// The event log continues where it left off: an update quoting the
	// persisted version succeeds.
	mustDo(t, server, http.MethodPut, "/api/v1/persons/"+personID,
		fmt.Sprintf(`{"given_name":"Persisted","version":%v}`, person["version"]), http.StatusOK)
}

func assertComplete(t *testing.T, s *Stores) {
	t.Helper()
	if s.Events == nil || s.ReadModel == nil || s.Snapshots == nil || s.Branches == nil {
		t.Fatalf("incomplete stores: %+v", s)
	}
}

func newServer(s *Stores) *api.Server {
	cfg := &config.Config{Port: 8080, LogFormat: "text"}
	return api.NewServer(cfg, s.Events, s.ReadModel, s.Snapshots, nil, api.WithBranchStore(s.Branches))
}

func mustDo(t *testing.T, server *api.Server, method, path, body string, want int) map[string]any {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, http.NoBody)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("%s %s: status = %d, want %d. Body: %s", method, path, rec.Code, want, rec.Body.String())
	}
	out := map[string]any{}
	if strings.TrimSpace(rec.Body.String()) != "" {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s %s: decode %q: %v", method, path, rec.Body.String(), err)
		}
	}
	return out
}

func createPerson(t *testing.T, server *api.Server, given, surname string) string {
	t.Helper()
	resp := mustDo(t, server, http.MethodPost, "/api/v1/persons",
		fmt.Sprintf(`{"given_name":%q,"surname":%q,"gender":"unknown"}`, given, surname), http.StatusCreated)
	id, _ := resp["id"].(string)
	if id == "" {
		t.Fatalf("create person returned no id: %v", resp)
	}
	return id
}

func countPersons(t *testing.T, server *api.Server) int {
	t.Helper()
	resp := mustDo(t, server, http.MethodGet, "/api/v1/persons", "", http.StatusOK)
	total, ok := resp["total"].(float64)
	if !ok {
		t.Fatalf("person list has no total: %v", resp)
	}
	return int(total)
}

// ============================================================================
// PostgreSQL provisioning
// ============================================================================

// localPostgresEnv names an optional PostgreSQL server URL; when set, each
// test gets a fresh database on it. Otherwise a testcontainer is used, and the
// test skips when Docker is unavailable.
const localPostgresEnv = "MYFAMILY_TEST_POSTGRES_URL"

// postgresDatabase returns the URL of a fresh, empty PostgreSQL database that
// is dropped when the test ends.
func postgresDatabase(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping PostgreSQL test in short mode")
	}
	serverURL := os.Getenv(localPostgresEnv)
	if serverURL == "" {
		if exec.Command("docker", "info").Run() != nil {
			t.Skip("Docker is not available and " + localPostgresEnv + " is unset, skipping PostgreSQL test")
		}
		// Not t.Context(): it is cancelled before t.Cleanup runs.
		ctx := context.Background()
		container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
			tcpostgres.WithDatabase("testdb"),
			tcpostgres.WithUsername("test"),
			tcpostgres.WithPassword("test"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(60*time.Second)),
		)
		if err != nil {
			t.Fatalf("start postgres container: %v", err)
		}
		t.Cleanup(func() { _ = container.Terminate(ctx) })
		serverURL, err = container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			t.Fatalf("postgres connection string: %v", err)
		}
	}

	admin, err := sql.Open("postgres", serverURL)
	if err != nil {
		t.Fatalf("connect to postgres: %v", err)
	}
	name := "t_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	// #nosec G202 -- name is generated above from a UUID, never external input.
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil { // nosemgrep
		admin.Close()
		t.Fatalf("create database %s: %v", name, err)
	}
	t.Cleanup(func() {
		// #nosec G202 -- name is generated above from a UUID, never external input.
		_, _ = admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)") // nosemgrep
		admin.Close()
	})

	dsn, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse postgres URL: %v", err)
	}
	dsn.Path = "/" + name
	return dsn.String()
}
