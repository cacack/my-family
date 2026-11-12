// Package integration_test drives the real HTTP API against every storage
// backend, so cross-feature flows are proven against the SQL backends rather
// than only in memory (DB-001).
//
// The bundles are built by `internal/storage` — the same constructor `serve`
// uses — so each SQL backend runs on exactly the topology a deployment gets:
// one database holding all four stores, schema set up by the stores themselves.
//
// The package holds no non-test code: it is a harness plus scenarios. Each
// scenario body is written ONCE and executed against every entry in `backends`,
// which is what makes it a parity test rather than three similar tests.
package integration_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/storage"
	"github.com/cacack/my-family/internal/testutil/pgtest"
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

// setupMemory builds the in-memory bundle, as demo mode does.
func setupMemory(t *testing.T) stores {
	t.Helper()
	return fromStorage(t, storage.OpenMemory(), nil)
}

// All four stores share ONE database per SQL backend — the topology ADR-002
// describes and the only one config can express (a single `SQLITE_PATH` /
// `DATABASE_URL`). The schemas coexist because their table names are disjoint
// (DB-006): the log owns `events`, the read model's life facts live in
// `life_events`. The file lives in t.TempDir(), which removes it.
func setupSQLite(t *testing.T) stores {
	t.Helper()
	opened, err := storage.OpenSQLite(filepath.Join(t.TempDir(), "myfamily.db"), storage.AccessServe)
	return fromStorage(t, opened, err)
}

// fromStorage adapts a storage bundle to the harness's interface-only view and
// closes it when the test ends.
func fromStorage(t *testing.T, opened *storage.Stores, err error) stores {
	t.Helper()
	if err != nil {
		t.Fatalf("open stores: %v", err)
	}
	t.Cleanup(func() {
		if err := opened.Close(); err != nil {
			t.Errorf("close stores: %v", err)
		}
	})
	return stores{
		events:    opened.Events,
		read:      opened.ReadModel,
		snapshots: opened.Snapshots,
		branches:  opened.Branches,
	}
}

// setupPostgres builds the bundle over a fresh, empty PostgreSQL database.
//
// It skips rather than fails when the environment cannot supply one (see
// pgtest), so `go test ./...` still passes on a machine without Docker. Each
// call gets its own database, which keeps the backends independent.
func setupPostgres(t *testing.T) stores {
	t.Helper()
	// One database for all four stores, as on SQLite (DB-006).
	opened, err := storage.OpenPostgres(pgtest.NewDatabase(t), storage.AccessServe)
	return fromStorage(t, opened, err)
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
