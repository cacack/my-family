package query

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
	"github.com/cacack/my-family/internal/repository/postgres"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

// The countingReadStore tests in entity_names_test.go show the service layer
// makes one Get*ByIDs call per entity type. The tests here check the other
// half of the #697 guarantee on the real SQL backends: each of those calls is
// ONE SQL statement, so naming N entries costs a fixed number of statements
// whatever N is. A wrapping database/sql driver counts every statement that
// reaches the underlying driver.

// statementCounter counts SQL statements that reach a wrapped driver.
type statementCounter struct{ n atomic.Int64 }

func (c *statementCounter) count() int64 { return c.n.Load() }
func (c *statementCounter) reset()       { c.n.Store(0) }

// countingConnector opens connections of drv to dsn, each wrapped so that
// every statement it runs is counted.
type countingConnector struct {
	drv     driver.Driver
	dsn     string
	counter *statementCounter
}

func (c *countingConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := c.drv.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return &countingConn{Conn: conn, counter: c.counter}, nil
}

func (c *countingConnector) Driver() driver.Driver { return c.drv }

// countingConn forwards to the wrapped connection, counting each prepared,
// queried or executed statement once. A driver.ErrSkip answer is not counted:
// database/sql then falls back to PrepareContext, which is.
type countingConn struct {
	driver.Conn
	counter *statementCounter
}

func (c *countingConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	c.counter.n.Add(1)
	if pc, ok := c.Conn.(driver.ConnPrepareContext); ok {
		return pc.PrepareContext(ctx, query)
	}
	return c.Prepare(query)
}

func (c *countingConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	qc, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	rows, err := qc.QueryContext(ctx, query, args)
	if !errors.Is(err, driver.ErrSkip) {
		c.counter.n.Add(1)
	}
	return rows, err
}

func (c *countingConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	ec, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	res, err := ec.ExecContext(ctx, query, args)
	if !errors.Is(err, driver.ErrSkip) {
		c.counter.n.Add(1)
	}
	return res, err
}

func (c *countingConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	bt, ok := c.Conn.(driver.ConnBeginTx)
	if !ok {
		return nil, errors.New("wrapped driver connection does not support BeginTx")
	}
	return bt.BeginTx(ctx, opts)
}

func (c *countingConn) CheckNamedValue(nv *driver.NamedValue) error {
	if nc, ok := c.Conn.(driver.NamedValueChecker); ok {
		return nc.CheckNamedValue(nv)
	}
	return driver.ErrSkip
}

func (c *countingConn) ResetSession(ctx context.Context) error {
	if sr, ok := c.Conn.(driver.SessionResetter); ok {
		return sr.ResetSession(ctx)
	}
	return nil
}

func (c *countingConn) IsValid() bool {
	if v, ok := c.Conn.(driver.Validator); ok {
		return v.IsValid()
	}
	return true
}

// sqlBackend opens a fresh read model on one SQL backend behind a statement
// counter.
type sqlBackend struct {
	name string
	open func(t *testing.T) (repository.ReadModelStore, *statementCounter)
}

func countedSQLBackends() []sqlBackend {
	return []sqlBackend{
		{name: "sqlite", open: openCountedSQLite},
		{name: "postgres", open: openCountedPostgres},
	}
}

func openCountedSQLite(t *testing.T) (repository.ReadModelStore, *statementCounter) {
	t.Helper()
	counter := &statementCounter{}
	dsn := filepath.Join(t.TempDir(), "names.db") + "?_foreign_keys=on&_busy_timeout=5000"
	db := sql.OpenDB(&countingConnector{drv: &sqlite3.SQLiteDriver{}, dsn: dsn, counter: counter})
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	store, err := sqlite.NewReadModelStore(db)
	require.NoError(t, err)
	return store, counter
}

func openCountedPostgres(t *testing.T) (repository.ReadModelStore, *statementCounter) {
	t.Helper()
	serverURL := os.Getenv("MYFAMILY_TEST_POSTGRES_URL")
	if serverURL == "" {
		t.Skip("MYFAMILY_TEST_POSTGRES_URL not set, skipping PostgreSQL statement-count test")
	}
	admin, err := sql.Open("postgres", serverURL)
	require.NoError(t, err)
	name := "t_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	// #nosec G202 -- name is generated above from a UUID, never external input.
	_, err = admin.Exec("CREATE DATABASE " + name)
	require.NoError(t, err)

	dsn, err := url.Parse(serverURL)
	require.NoError(t, err)
	dsn.Path = "/" + name
	counter := &statementCounter{}
	db := sql.OpenDB(&countingConnector{drv: &pq.Driver{}, dsn: dsn.String(), counter: counter})
	t.Cleanup(func() {
		_ = db.Close()
		// #nosec G202 -- name is generated above from a UUID, never external input.
		_, _ = admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)")
		_ = admin.Close()
	})
	store, err := postgres.NewReadModelStore(db)
	require.NoError(t, err)
	return store, counter
}

// TestTransformStoredEvents_SQLStatementCountDoesNotScale checks, per SQL
// backend, that naming history entries is exactly one statement per entity
// type, for 2 entities per type and for 60.
func TestTransformStoredEvents_SQLStatementCountDoesNotScale(t *testing.T) {
	ctx := context.Background()
	for _, backend := range countedSQLBackends() {
		for _, n := range []int{2, 60} {
			t.Run(fmt.Sprintf("%s/%d per type", backend.name, n), func(t *testing.T) {
				store, counter := backend.open(t)
				set := seedNamedEvents(t, ctx, store, n)
				service := NewHistoryService(memory.NewEventStore(), store)
				counter.reset()

				entries, err := service.transformStoredEvents(ctx, set.events)
				require.NoError(t, err)
				require.Len(t, entries, 5*n)
				assert.Equal(t, int64(4), counter.count(), "one SQL statement per entity type, independent of entry count")

				for _, entry := range entries {
					assert.Equal(t, set.wantNames[entry.EntityID], entry.EntityName, "entry %s", entry.EntityID)
				}
			})
		}
	}
}

// TestEnrichConflictEntities_SQLStatementCountDoesNotScale is the same check
// for naming merge conflicts.
func TestEnrichConflictEntities_SQLStatementCountDoesNotScale(t *testing.T) {
	ctx := context.Background()
	for _, backend := range countedSQLBackends() {
		for _, n := range []int{2, 60} {
			t.Run(fmt.Sprintf("%s/%d per type", backend.name, n), func(t *testing.T) {
				store, counter := backend.open(t)
				set := seedNamedEvents(t, ctx, store, n)
				eventStore := memory.NewEventStore()
				service := NewBranchService(memory.NewBranchStore(), eventStore, NewHistoryService(eventStore, store))

				seen := make(map[uuid.UUID]bool)
				var conflicts []MergeConflict
				for _, evt := range set.events {
					if !seen[evt.StreamID] {
						seen[evt.StreamID] = true
						conflicts = append(conflicts, MergeConflict{StreamID: evt.StreamID, Kind: ConflictEditEdit})
					}
				}
				counter.reset()

				require.NoError(t, service.enrichConflictEntities(ctx, domain.MainBranchID, set.events, conflicts))
				assert.Equal(t, int64(4), counter.count(), "one SQL statement per entity type, independent of conflict count")

				for _, c := range conflicts {
					assert.Equal(t, set.wantNames[c.StreamID], c.EntityName)
				}
			})
		}
	}
}

// TestTransformStoredEventsOn_SQLStatementCountDoesNotScale is the branch-scope
// counterpart: through a branch overlay with a deleted entity of every type,
// naming costs one statement per type on the branch plus one main fallback per
// type, for 2 entities per type and for 60.
func TestTransformStoredEventsOn_SQLStatementCountDoesNotScale(t *testing.T) {
	ctx := context.Background()
	for _, backend := range countedSQLBackends() {
		for _, n := range []int{2, 60} {
			t.Run(fmt.Sprintf("%s/%d per type", backend.name, n), func(t *testing.T) {
				store, counter := backend.open(t)
				set := seedNamedEvents(t, ctx, store, n)
				branch, want := overlayOnBranch(t, ctx, store, set)
				service := NewHistoryService(memory.NewEventStore(), store)
				counter.reset()

				entries, err := service.transformStoredEventsOn(ctx, branch, set.events)
				require.NoError(t, err)
				require.Len(t, entries, 5*n)
				assert.Equal(t, int64(8), counter.count(), "one branch and one main-fallback statement per entity type")

				for _, entry := range entries {
					assert.Equal(t, want[entry.EntityID], entry.EntityName, "entry %s", entry.EntityID)
				}
				// The branch's view must not have leaked onto main.
				counter.reset()
				mainEntries, err := service.transformStoredEvents(ctx, set.events)
				require.NoError(t, err)
				assert.Equal(t, int64(4), counter.count())
				for _, entry := range mainEntries {
					assert.Equal(t, set.wantNames[entry.EntityID], entry.EntityName)
				}
			})
		}
	}
}
