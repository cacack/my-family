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
// is constructed, so opening the stores also brings the schema up to date. A
// server's Open then refuses a read model stamped with another schema version
// than this binary's, and asks for `myfamily rebuild` (#680, ADR-006).
//
// There is deliberately no silent fallback: if the selected backend cannot be
// opened (unreachable PostgreSQL, or a SQLite file in a missing directory) Open
// returns an error and the server refuses to start rather than quietly running
// in memory and losing every write on restart.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
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

	db     *sql.DB
	lock   dbLock
	access Access
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

// Close releases the database lock and the underlying database connection.
// It is a no-op for the in-memory backend and safe to call more than once.
func (s *Stores) Close() error {
	var errs []error
	if s.lock != nil {
		if err := s.lock.close(); err != nil {
			errs = append(errs, fmt.Errorf("release %s database lock: %w", s.Backend, err))
		}
		s.lock = nil
	}
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close %s database: %w", s.Backend, err))
		}
		s.db = nil
	}
	return errors.Join(errs...)
}

// ErrRebuildMemory is returned by RebuildReadModel on the in-memory backend,
// whose event log does not outlive the process, so there is nothing to repair.
var ErrRebuildMemory = errors.New("the in-memory backend has no persistent read model to rebuild; unset DEMO_MODE")

// RebuildReadModel empties the read model and replays the event log into it
// (#845); see repository.RebuildReadModel. The stores must have been opened
// for AccessRebuild, whose lock keeps every server out meanwhile (#1232).
func (s *Stores) RebuildReadModel(ctx context.Context) (repository.RebuildResult, error) {
	if s.Backend == BackendMemory {
		return repository.RebuildResult{}, ErrRebuildMemory
	}
	if s.access != AccessRebuild {
		return repository.RebuildResult{}, ErrRebuildNotExclusive
	}
	return repository.RebuildReadModel(ctx, s.Events, s.ReadModel, s.Branches, s.Snapshots)
}

// ProjectionFailureWarning returns a log line saying the read model is behind
// the event log, because a rebuild did not finish or naming the oldest failed
// projection, or "" when neither is recorded (#845).
func (s *Stores) ProjectionFailureWarning(ctx context.Context) string {
	state, err := s.ReadModel.GetRebuildState(ctx)
	if err != nil {
		return fmt.Sprintf("Warning: could not check for an unfinished rebuild: %v", err)
	}
	if state != nil {
		return "Warning: the read model is out of sync with the event log: a `myfamily rebuild` did not finish. " +
			"Stop the server and run `myfamily rebuild` again to repair it."
	}
	failures, err := s.ReadModel.ListProjectionFailures(ctx, 1)
	if err != nil {
		return fmt.Sprintf("Warning: could not check for failed projections: %v", err)
	}
	if len(failures) == 0 {
		return ""
	}
	f := failures[0]
	return fmt.Sprintf("Warning: the read model is out of sync with the event log: projecting %s (stream %s, version %d) failed at %s. "+
		"Stop the server and run `myfamily rebuild` to repair it.",
		f.EventType, f.StreamID, f.Version, f.FailedAt.Format("2006-01-02T15:04:05Z07:00"))
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
// over it, running each store's schema setup. A SQL backend is first locked
// for access (see Access), so Open fails with ErrServerRunning or
// ErrRebuildRunning, before touching the database, when the two would
// overlap. The caller must Close the result, which releases the lock.
func Open(cfg *config.Config, access Access) (*Stores, error) {
	switch Select(cfg) {
	case BackendMemory:
		return OpenMemory(), nil
	case BackendPostgres:
		return OpenPostgres(cfg.DatabaseURL, access)
	default:
		return OpenSQLite(cfg.SQLitePath, access)
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
// constructs all four stores over it, holding the lock for access on the file
// lockPath(path) until Close.
func OpenSQLite(path string, access Access) (*Stores, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("SQLite storage selected but SQLITE_PATH is empty")
	}
	lock, err := openFileLock(lockPath(path))
	if err != nil {
		return nil, fmt.Errorf("open SQLite database %q: lock file: %w", path, err)
	}
	if err := acquire(lock, access); err != nil {
		return nil, errors.Join(fmt.Errorf("open SQLite database %q: %w", path, err), lock.close())
	}
	db, err := sqlite.OpenDB(path)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("open SQLite database %q: %w", path, err), lock.close())
	}
	// The read model is constructed FIRST: on a pre-#733 database it renames
	// its old `events` table to `life_events`, which the event store otherwise
	// refuses to touch (sqlite.ErrReadModelEventsTable).
	return build(BackendSQLite, path, db, lock, access,
		func(db *sql.DB) (repository.ReadModelStore, error) { return sqlite.NewReadModelStore(db) },
		func(db *sql.DB) (repository.EventStore, error) { return sqlite.NewEventStore(db) },
		func(db *sql.DB) (repository.SnapshotStore, error) { return sqlite.NewSnapshotStore(db) },
		func(db *sql.DB) (repository.BranchStore, error) { return sqlite.NewBranchStore(db) },
	)
}

// lockPath returns the lock file for the SQLite database at path: the
// database's real path, with symlinks resolved, plus ".lock". Without that, a
// server and a rebuild naming one database through a symlink to the file and
// through its real path would lock different files. A path that does not
// resolve (no database yet) is used as given.
func lockPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return path + ".lock"
}

// OpenPostgres connects to the PostgreSQL database at connStr and constructs
// all four stores over it, holding a session advisory lock for access on a
// connection of its own until Close.
//
// No error it returns carries the connection string's password: lib/pq repeats
// the raw DSN in some of its errors (notably URL parse failures), so driver
// errors are scrubbed before they are wrapped, and a URL-shaped string that
// does not parse is rejected before it reaches the driver at all.
func OpenPostgres(connStr string, access Access) (*Stores, error) {
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
	conn, err := db.Conn(context.Background())
	if err != nil {
		return nil, errors.Join(fmt.Errorf("open PostgreSQL database %s: lock connection: %w", location, scrubError(err, connStr)), db.Close())
	}
	lock := &advisoryLock{conn: conn}
	if err := acquire(lock, access); err != nil {
		return nil, errors.Join(fmt.Errorf("open PostgreSQL database %s: %w", location, err), lock.close(), db.Close())
	}
	// Read model first, for the same pre-#733 reason as SQLite.
	return build(BackendPostgres, location, db, lock, access,
		func(db *sql.DB) (repository.ReadModelStore, error) { return postgres.NewReadModelStore(db) },
		func(db *sql.DB) (repository.EventStore, error) { return postgres.NewEventStore(db) },
		func(db *sql.DB) (repository.SnapshotStore, error) { return postgres.NewSnapshotStore(db) },
		func(db *sql.DB) (repository.BranchStore, error) { return postgres.NewBranchStore(db) },
	)
}

// build constructs the four stores over db in a fixed order (read model first)
// and closes db and releases lock if any construction fails, so a failed Open
// leaks nothing. For AccessServe it then refuses a read model whose schema
// version is not this binary's (repository.CheckReadModelSchema).
func build(
	backend Backend,
	location string,
	db *sql.DB,
	lock dbLock,
	access Access,
	newReadModel func(*sql.DB) (repository.ReadModelStore, error),
	newEvents func(*sql.DB) (repository.EventStore, error),
	newSnapshots func(*sql.DB) (repository.SnapshotStore, error),
	newBranches func(*sql.DB) (repository.BranchStore, error),
) (*Stores, error) {
	s := &Stores{Backend: backend, Location: location, db: db, lock: lock, access: access}
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
	// A server never runs on a read model of another schema version, which it
	// might not be able to read or write; the rebuild is what fixes that, so
	// it is not checked (#680).
	if access == AccessServe {
		if err := repository.CheckReadModelSchema(context.Background(), s.Events, s.ReadModel); err != nil {
			return nil, errors.Join(err, s.Close())
		}
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
