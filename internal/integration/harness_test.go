// Package integration_test drives the real HTTP API against every storage
// backend, so cross-feature flows are proven against the SQL backends rather
// than only in memory (DB-001).
//
// One caveat worth stating plainly: `cmd/myfamily` wires the in-memory stores
// unconditionally today, and the SQL backends here each run on TWO databases to
// dodge the table collision in #733. So this proves backend parity for the
// branch/merge flows, not that any deployed topology works end to end.
//
// The package holds no non-test code: it is a harness plus scenarios. Each
// scenario body is written ONCE and executed against every entry in `backends`,
// which is what makes it a parity test rather than three similar tests.
package integration_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
	pgstore "github.com/cacack/my-family/internal/repository/postgres"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

// ============================================================================
// Backend table
// ============================================================================

// stores is the set of repositories a server needs. The fields are the
// repository interfaces rather than the concrete types so a scenario body is
// backend-agnostic by construction — it cannot reach for a memory-only method.
type stores struct {
	events    repository.EventStore
	read      repository.ReadModelStore
	snapshots repository.SnapshotStore
	branches  repository.BranchStore
}

// backend is one storage implementation the scenarios run against. setup
// returns a ready bundle and registers its own teardown with t.Cleanup, so a
// scenario never has to know which backend it is on.
type backend struct {
	name  string
	setup func(t *testing.T) stores
}

// backends is the table every scenario in this package iterates. Adding a
// backend here adds it to every scenario at once.
var backends = []backend{
	{"Memory", setupMemory},
	{"SQLite", setupSQLite},
	{"Postgres", setupPostgres},
}

// setupMemory builds the in-memory bundle. The memory snapshot store needs the
// concrete event store, which is why it is constructed before the bundle.
func setupMemory(t *testing.T) stores {
	t.Helper()
	eventStore := memory.NewEventStore()
	return stores{
		events:    eventStore,
		read:      memory.NewReadModelStore(),
		snapshots: memory.NewSnapshotStore(eventStore),
		branches:  memory.NewBranchStore(),
	}
}

// The SQL backends need TWO databases, not one, because the event log and the
// read model both define a table called `events` — the log's append-only stream
// in eventstore.go, the read model's life-event facts in readmodel.go. Their
// columns are unrelated, and `CREATE TABLE IF NOT EXISTS` makes the second
// definition a silent no-op whose indexes then fail ("no such column:
// owner_type"), whichever order the stores are built in.
//
// Splitting them is sound: no code path joins the two schemas or spans them in a
// transaction — each store reaches its own tables through its own *sql.DB. The
// grouping below is the one the schemas require: the snapshot store's
// GetMaxPosition reads `MAX(position) FROM events`, i.e. the LOG's table, so it
// belongs with the event store, while the branch registry belongs with the read
// model it purges overlay rows from.
//
// This is a real defect in the SQL backends, not a property of this harness —
// tracked as #733. Nothing has caught it before because cmd/myfamily wires the
// in-memory stores only, and every existing SQL test builds one store at a time.
// When #733 renames one of the two tables, collapse this back to a single
// database per backend so the tested topology matches the deployed one.
func setupSQLite(t *testing.T) stores {
	t.Helper()

	dir := t.TempDir()
	logDB := openSQLite(t, filepath.Join(dir, "eventlog.db"))
	readDB := openSQLite(t, filepath.Join(dir, "readmodel.db"))

	eventStore, err := sqlite.NewEventStore(logDB)
	if err != nil {
		t.Fatalf("create sqlite event store: %v", err)
	}
	snapshotStore, err := sqlite.NewSnapshotStore(logDB)
	if err != nil {
		t.Fatalf("create sqlite snapshot store: %v", err)
	}
	readStore, err := sqlite.NewReadModelStore(readDB)
	if err != nil {
		t.Fatalf("create sqlite read model store: %v", err)
	}
	branchStore, err := sqlite.NewBranchStore(readDB)
	if err != nil {
		t.Fatalf("create sqlite branch store: %v", err)
	}

	return stores{events: eventStore, read: readStore, snapshots: snapshotStore, branches: branchStore}
}

// openSQLite opens one sqlite file and closes it when the test ends. The file
// lives in t.TempDir(), which removes it.
func openSQLite(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sqlite.OpenDB(path)
	if err != nil {
		t.Fatalf("open sqlite database %s: %v", path, err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// setupPostgres builds the bundle over a throwaway PostgreSQL container.
//
// It skips rather than fails when the environment cannot supply one — the same
// graceful-skip contract internal/repository/postgres/eventstore_test.go
// established, so `go test ./...` still passes on a machine without Docker.
// One container per call keeps the backends independent; it costs a few
// seconds per subtest and buys a guaranteed-empty database.
func setupPostgres(t *testing.T) stores {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping PostgreSQL integration test in short mode")
	}
	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping PostgreSQL integration test")
	}

	// Not t.Context(): it is cancelled before t.Cleanup runs, and terminating
	// the container needs a live context.
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { container.Terminate(ctx) })

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres connection string: %v", err)
	}

	admin := openPostgres(t, connStr)
	// The container is up but the server may still be finishing startup.
	var pingErr error
	for i := 0; i < 30; i++ {
		if pingErr = admin.Ping(); pingErr == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if pingErr != nil {
		t.Fatalf("ping postgres: %v", pingErr)
	}

	// Two databases, for the `events` table collision documented on setupSQLite.
	logDB := createPostgresDatabase(t, admin, connStr, "eventlog")
	readDB := createPostgresDatabase(t, admin, connStr, "readmodel")

	eventStore, err := pgstore.NewEventStore(logDB)
	if err != nil {
		t.Fatalf("create postgres event store: %v", err)
	}
	snapshotStore, err := pgstore.NewSnapshotStore(logDB)
	if err != nil {
		t.Fatalf("create postgres snapshot store: %v", err)
	}
	readStore, err := pgstore.NewReadModelStore(readDB)
	if err != nil {
		t.Fatalf("create postgres read model store: %v", err)
	}
	branchStore, err := pgstore.NewBranchStore(readDB)
	if err != nil {
		t.Fatalf("create postgres branch store: %v", err)
	}

	return stores{events: eventStore, read: readStore, snapshots: snapshotStore, branches: branchStore}
}

// createPostgresDatabase creates a database in the running container and
// returns a connection to it. CREATE DATABASE cannot run inside a transaction
// and needs a connection to some other database, which is what admin is.
func createPostgresDatabase(t *testing.T, admin *sql.DB, connStr, name string) *sql.DB {
	t.Helper()
	// #nosec G202 -- name is a literal from this file, never external input.
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatalf("create database %s: %v", name, err)
	}

	dsn, err := url.Parse(connStr)
	if err != nil {
		t.Fatalf("parse postgres connection string: %v", err)
	}
	dsn.Path = "/" + name
	return openPostgres(t, dsn.String())
}

// openPostgres opens a connection and closes it when the test ends.
func openPostgres(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("connect to postgres: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// isDockerAvailable reports whether a Docker daemon is reachable.
func isDockerAvailable() bool {
	return exec.Command("docker", "info").Run() == nil
}

// newServer wires a bundle into the real HTTP server, branch registry included
// — the same construction cmd/myfamily/main.go performs.
func newServer(t *testing.T, st stores) *api.Server {
	t.Helper()
	cfg := &config.Config{Port: 8080, LogFormat: "text"}
	return api.NewServer(cfg, st.events, st.read, st.snapshots, nil,
		api.WithBranchStore(st.branches))
}

// ============================================================================
// HTTP driver
// ============================================================================

// do issues an in-process request against the server and returns the recorder.
func do(t *testing.T, server *api.Server, method, path, body string) *httptest.ResponseRecorder {
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
	return rec
}

// mustDo is do plus the status assertion every step of a scenario needs, and
// returns the decoded body (nil for an empty one, e.g. 204). It fails the test
// rather than returning an error so a scenario reads as a sequence of steps.
func mustDo(t *testing.T, server *api.Server, method, path, body string, wantStatus int) map[string]any {
	t.Helper()
	rec := do(t, server, method, path, body)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s: status = %d, want %d. Body: %s", method, path, rec.Code, wantStatus, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) == "" {
		return nil
	}
	return decodeJSON(t, rec)
}

// decodeJSON parses a recorder body into a map, failing the test on error.
func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("parse response %q: %v", rec.Body.String(), err)
	}
	return out
}

// scoped appends the ?branch= scope to a path. An empty branchID is the
// mainline and leaves the path untouched, which is what the API treats as
// "no scope" (resolveBranchScope).
func scoped(path, branchID string) string {
	if branchID == "" {
		return path
	}
	return path + "?branch=" + branchID
}

// ============================================================================
// Fixture helpers - the same calls a client would make
// ============================================================================

// createPerson creates a person on the mainline and returns its id.
func createPerson(t *testing.T, server *api.Server, givenName, surname string) string {
	t.Helper()
	resp := mustDo(t, server, http.MethodPost, "/api/v1/persons",
		fmt.Sprintf(`{"given_name":%q,"surname":%q,"gender":"unknown"}`, givenName, surname),
		http.StatusCreated)
	return mustString(t, resp, "id")
}

// createFamily creates a family on the mainline and returns its id.
func createFamily(t *testing.T, server *api.Server, partner1ID, partner2ID string) string {
	t.Helper()
	resp := mustDo(t, server, http.MethodPost, "/api/v1/families",
		fmt.Sprintf(`{"partner1_id":%q,"partner2_id":%q,"relationship_type":"marriage"}`, partner1ID, partner2ID),
		http.StatusCreated)
	return mustString(t, resp, "id")
}

// createBranch creates an active branch and returns its id.
func createBranch(t *testing.T, server *api.Server, name string) string {
	t.Helper()
	resp := mustDo(t, server, http.MethodPost, "/api/v1/branches",
		fmt.Sprintf(`{"name":%q}`, name), http.StatusCreated)
	if status := resp["status"]; status != "active" {
		t.Fatalf("new branch status = %v, want active", status)
	}
	return mustString(t, resp, "id")
}

// getEntity reads one entity on the given scope, asserting 200.
func getEntity(t *testing.T, server *api.Server, path, branchID string) map[string]any {
	t.Helper()
	return mustDo(t, server, http.MethodGet, scoped(path, branchID), "", http.StatusOK)
}

// entityVersion reads an entity's current version on the given scope. Writes
// carry the version of the row they are editing, and a branch row's version
// diverges from main's as soon as the branch edits it, so a branch-scoped write
// must read its expected version from the branch.
func entityVersion(t *testing.T, server *api.Server, path, branchID string) int64 {
	t.Helper()
	raw, ok := getEntity(t, server, path, branchID)["version"].(float64)
	if !ok {
		t.Fatalf("GET %s: no version field", scoped(path, branchID))
	}
	return int64(raw)
}

// mustString pulls a required string field out of a decoded response.
func mustString(t *testing.T, resp map[string]any, field string) string {
	t.Helper()
	value, _ := resp[field].(string)
	if value == "" {
		t.Fatalf("response has no %s: %v", field, resp)
	}
	return value
}

// ============================================================================
// Assertion helpers
// ============================================================================

// jsonArray pulls a required array field out of a decoded response. It fails on
// a missing or null field rather than treating it as empty: "the server omitted
// this" and "the server said none" are different answers, and several of these
// responses require [] and never null.
func jsonArray(t *testing.T, resp map[string]any, field string) []any {
	t.Helper()
	value, ok := resp[field].([]any)
	if !ok {
		t.Fatalf("%s missing or null in %v", field, resp)
	}
	return value
}

// optionalArray reads an array field that the response omits when it is empty
// (`children` on a childless family, for instance). A missing or null field
// reads as empty; a field of the wrong type still fails.
func optionalArray(t *testing.T, resp map[string]any, field string) []any {
	t.Helper()
	raw, present := resp[field]
	if !present || raw == nil {
		return nil
	}
	value, ok := raw.([]any)
	if !ok {
		t.Fatalf("%s = %v, want an array", field, raw)
	}
	return value
}

// entryField collects one field from every object in a decoded JSON array. It
// fails the test on a malformed entry rather than appending "", matching
// stringValues and jsonArray: a response that lost its shape should say so here,
// not turn into an empty string that some later assertion quietly tolerates.
func entryField(t *testing.T, entries []any, field string) []string {
	t.Helper()
	values := make([]string, 0, len(entries))
	for i, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("entry %d = %v, want an object", i, raw)
		}
		value, ok := entry[field].(string)
		if !ok {
			t.Fatalf("entry %d field %q = %v, want a string", i, field, entry[field])
		}
		values = append(values, value)
	}
	return values
}

// contains reports whether values includes want.
func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
