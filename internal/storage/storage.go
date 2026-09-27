// Package storage selects and constructs the repository stores the server runs
// on (ADR-002). It is the single place that turns configuration into a concrete
// backend, so `serve` and the tests that exercise it build stores the same way.
//
// Selection, in order:
//
//  1. DEMO_MODE        -> in-memory stores (sample data, reset on demand, no persistence)
//  2. DATABASE_URL set -> PostgreSQL at that URL
//  3. otherwise        -> SQLite at SQLITE_PATH (default ./myfamily.db)
//
// Every SQL backend keeps all four stores (event log, read model, snapshots,
// branch registry) in ONE database, which is the only topology the config
// surface can express (DB-006). Each store runs its own DDL/migrations when it
// is constructed, so opening the stores also brings the schema up to date.
//
// There is deliberately no silent fallback: if the selected backend cannot be
// opened (unreachable PostgreSQL, a SQLite file in a missing directory, or a
// binary built without cgo so the SQLite driver is a stub) Open returns an
// error and the server refuses to start rather than quietly running in memory
// and losing every write on restart.
package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
	"github.com/cacack/my-family/internal/repository/postgres"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

// Backend names a storage implementation.
type Backend string

// The storage backends Open can construct.
const (
	BackendMemory   Backend = "memory"
	BackendSQLite   Backend = "sqlite"
	BackendPostgres Backend = "postgres"
)

// ErrSQLiteUnavailable is returned when SQLite is selected but this binary was
// built without cgo, so the SQLite driver cannot open any database.
var ErrSQLiteUnavailable = errors.New(
	"SQLite storage is unavailable: this binary was built without cgo (CGO_ENABLED=0) " +
		"and the SQLite driver requires it. Set DATABASE_URL to use PostgreSQL, " +
		"rebuild with CGO_ENABLED=1, or use the Docker image")

// sqliteAvailable is cgoEnabled, held in a variable so tests can exercise the
// cgo-less refusal path from a cgo build.
var sqliteAvailable = cgoEnabled

// MemoryStores holds the concrete in-memory stores, which demo mode needs for
// its reset endpoint (the repository interfaces carry no Reset).
type MemoryStores struct {
	Events    *memory.EventStore
	ReadModel *memory.ReadModelStore
	Snapshots *memory.SnapshotStore
	Branches  *memory.BranchStore
}

// Stores is an opened set of repositories plus what is needed to describe and
// release them.
type Stores struct {
	// Backend is the implementation actually in use.
	Backend Backend
	// Location describes where the data lives, safe to log: the SQLite path,
	// or the PostgreSQL URL with any password redacted.
	Location string

	Events    repository.EventStore
	ReadModel repository.ReadModelStore
	Snapshots repository.SnapshotStore
	Branches  repository.BranchStore

	// Memory is non-nil only for BackendMemory.
	Memory *MemoryStores

	db *sql.DB
}

// Describe returns a one-line, log-safe description of the store in use.
func (s *Stores) Describe() string {
	switch s.Backend {
	case BackendSQLite:
		return "SQLite (" + s.Location + ")"
	case BackendPostgres:
		return "PostgreSQL (" + s.Location + ")"
	default:
		return "In-memory (no persistence)"
	}
}

// Close releases the underlying database connection. It is a no-op for the
// in-memory backend and safe to call more than once.
func (s *Stores) Close() error {
	if s.db == nil {
		return nil
	}
	db := s.db
	s.db = nil
	if err := db.Close(); err != nil {
		return fmt.Errorf("close %s database: %w", s.Backend, err)
	}
	return nil
}

// Select reports which backend Open would construct for cfg.
func Select(cfg *config.Config) Backend {
	switch {
	case cfg.DemoMode:
		return BackendMemory
	case cfg.UsePostgreSQL():
		return BackendPostgres
	default:
		return BackendSQLite
	}
}

// Open selects a backend from cfg (see Select) and constructs all four stores
// over it, running each store's schema setup. The caller must Close the
// result.
func Open(cfg *config.Config) (*Stores, error) {
	switch Select(cfg) {
	case BackendMemory:
		return OpenMemory(), nil
	case BackendPostgres:
		return OpenPostgres(cfg.DatabaseURL)
	default:
		return OpenSQLite(cfg.SQLitePath)
	}
}

// OpenMemory constructs the in-memory stores.
func OpenMemory() *Stores {
	events := memory.NewEventStore()
	mem := &MemoryStores{
		Events:    events,
		ReadModel: memory.NewReadModelStore(),
		Snapshots: memory.NewSnapshotStore(events),
		Branches:  memory.NewBranchStore(),
	}
	return &Stores{
		Backend:   BackendMemory,
		Location:  "memory",
		Events:    mem.Events,
		ReadModel: mem.ReadModel,
		Snapshots: mem.Snapshots,
		Branches:  mem.Branches,
		Memory:    mem,
	}
}

// OpenSQLite opens (creating if needed) the SQLite database at path and
// constructs all four stores over it.
func OpenSQLite(path string) (*Stores, error) {
	if !sqliteAvailable {
		return nil, ErrSQLiteUnavailable
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("SQLite storage selected but SQLITE_PATH is empty")
	}
	db, err := sqlite.OpenDB(path)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database %q: %w", path, err)
	}
	// The read model is constructed FIRST: on a pre-#733 database it renames
	// its old `events` table to `life_events`, which the event store otherwise
	// refuses to touch (sqlite.ErrReadModelEventsTable).
	return build(BackendSQLite, path, db,
		func(db *sql.DB) (repository.ReadModelStore, error) { return sqlite.NewReadModelStore(db) },
		func(db *sql.DB) (repository.EventStore, error) { return sqlite.NewEventStore(db) },
		func(db *sql.DB) (repository.SnapshotStore, error) { return sqlite.NewSnapshotStore(db) },
		func(db *sql.DB) (repository.BranchStore, error) { return sqlite.NewBranchStore(db) },
	)
}

// OpenPostgres connects to the PostgreSQL database at connStr and constructs
// all four stores over it.
//
// No error it returns carries the connection string's password: lib/pq repeats
// the raw DSN in some of its errors (notably URL parse failures), so driver
// errors are scrubbed before they are wrapped, and a URL-shaped string that
// does not parse is rejected before it reaches the driver at all.
func OpenPostgres(connStr string) (*Stores, error) {
	if strings.Contains(connStr, "://") {
		if !validURL(connStr) {
			// Deliberately not wrapping err: url.Error quotes the whole input,
			// and even its fragment (e.g. an invalid escape) is part of the
			// password.
			return nil, errors.New("DATABASE_URL is not a valid URL; " +
				"check that special characters in the password (such as %, @, / or spaces) are percent-encoded")
		}
	}
	location := redactPostgres(connStr)
	db, err := postgres.OpenDB(connStr)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL database %s: %w", location, scrubError(err, connStr))
	}
	// Read model first, for the same pre-#733 reason as SQLite.
	return build(BackendPostgres, location, db,
		func(db *sql.DB) (repository.ReadModelStore, error) { return postgres.NewReadModelStore(db) },
		func(db *sql.DB) (repository.EventStore, error) { return postgres.NewEventStore(db) },
		func(db *sql.DB) (repository.SnapshotStore, error) { return postgres.NewSnapshotStore(db) },
		func(db *sql.DB) (repository.BranchStore, error) { return postgres.NewBranchStore(db) },
	)
}

// build constructs the four stores over db in a fixed order (read model first)
// and closes db if any construction fails, so a failed Open leaks nothing.
func build(
	backend Backend,
	location string,
	db *sql.DB,
	newReadModel func(*sql.DB) (repository.ReadModelStore, error),
	newEvents func(*sql.DB) (repository.EventStore, error),
	newSnapshots func(*sql.DB) (repository.SnapshotStore, error),
	newBranches func(*sql.DB) (repository.BranchStore, error),
) (*Stores, error) {
	s := &Stores{Backend: backend, Location: location, db: db}
	fail := func(what string, err error) (*Stores, error) {
		return nil, errors.Join(
			fmt.Errorf("initialize %s %s store: %w", backend, what, err),
			s.Close(),
		)
	}
	var err error
	if s.ReadModel, err = newReadModel(db); err != nil {
		return fail("read model", err)
	}
	if s.Events, err = newEvents(db); err != nil {
		return fail("event", err)
	}
	if s.Snapshots, err = newSnapshots(db); err != nil {
		return fail("snapshot", err)
	}
	if s.Branches, err = newBranches(db); err != nil {
		return fail("branch", err)
	}
	return s, nil
}

// validURL reports whether a URL-form connection string parses cleanly,
// including its query (whose malformed pairs url.URL.Query would drop).
func validURL(connStr string) bool {
	u, err := url.Parse(connStr)
	if err != nil {
		return false
	}
	_, err = url.ParseQuery(u.RawQuery)
	return err == nil
}

// redactPostgres returns a log-safe rendering of a PostgreSQL connection
// string: a URL with its password masked, or a placeholder for the key=value
// form (which cannot be redacted reliably and may carry a password).
func redactPostgres(connStr string) string {
	u, err := url.Parse(connStr)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return "connection string not shown"
	}
	// lib/pq also accepts the password as a query parameter. u.Query() would
	// silently drop a malformed pair (and leave it in RawQuery), so a query
	// that does not parse cleanly is not shown at all.
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "connection string not shown"
	}
	if q.Has("password") {
		q.Set("password", "xxxxx")
		u.RawQuery = q.Encode()
	}
	return u.Redacted()
}

// passwordKV matches a password in lib/pq's key=value connection-string form,
// quoted or bare.
var passwordKV = regexp.MustCompile(`(?i)\bpassword\s*=\s*('(?:[^'\\]|\\.)*'|\S+)`)

// postgresSecrets returns the substrings of connStr that must never appear in
// output: the whole string and every form of its password (URL userinfo, the
// password query parameter, or the key=value password), escaped and unescaped.
func postgresSecrets(connStr string) []string {
	secrets := []string{connStr}
	add := func(v string) {
		if v == "" {
			return
		}
		secrets = append(secrets, v)
		if unq, err := url.PathUnescape(v); err == nil && unq != v {
			secrets = append(secrets, unq)
		}
		if unq, err := url.QueryUnescape(v); err == nil && unq != v {
			secrets = append(secrets, unq)
		}
		if t := strings.Trim(v, "'"); t != v {
			secrets = append(secrets, t)
		}
	}
	if u, err := url.Parse(connStr); err == nil {
		if pw, ok := u.User.Password(); ok {
			add(pw)
			add(url.PathEscape(pw))
			add(url.QueryEscape(pw))
		}
		for _, pw := range u.Query()["password"] {
			add(pw)
			add(url.QueryEscape(pw))
		}
	}
	for _, m := range passwordKV.FindAllStringSubmatch(connStr, -1) {
		add(m[1])
	}
	return secrets
}

// scrubbedError is a driver error whose text has had the connection string's
// secrets removed. It still unwraps to the original for errors.Is/As.
type scrubbedError struct {
	msg string
	err error
}

func (e *scrubbedError) Error() string { return e.msg }
func (e *scrubbedError) Unwrap() error { return e.err }

// scrubError removes the connection string and its password from err's text.
func scrubError(err error, connStr string) error {
	msg := err.Error()
	secrets := postgresSecrets(connStr)
	// Longest first, so the whole DSN is replaced before its password is.
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, s := range secrets {
		if s == connStr {
			msg = strings.ReplaceAll(msg, s, "<connection string>")
		} else {
			msg = strings.ReplaceAll(msg, s, "xxxxx")
		}
	}
	return &scrubbedError{msg: msg, err: err}
}
