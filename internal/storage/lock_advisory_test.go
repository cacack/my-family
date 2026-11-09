package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

// advisoryServer stands in for PostgreSQL's advisory-lock functions: it
// records each query and answers the try-lock ones with grant.
type advisoryServer struct {
	mu      sync.Mutex
	grant   bool
	fail    error
	queries []string
}

func (s *advisoryServer) Open(string) (driver.Conn, error) { return &advisoryConn{s}, nil }

type advisoryConn struct{ s *advisoryServer }

func (c *advisoryConn) Prepare(query string) (driver.Stmt, error) {
	return &advisoryStmt{c.s, query}, nil
}
func (c *advisoryConn) Close() error              { return nil }
func (c *advisoryConn) Begin() (driver.Tx, error) { return nil, errors.New("no transactions") }

type advisoryStmt struct {
	s     *advisoryServer
	query string
}

func (st *advisoryStmt) Close() error  { return nil }
func (st *advisoryStmt) NumInput() int { return 1 }
func (st *advisoryStmt) record() error {
	st.s.mu.Lock()
	defer st.s.mu.Unlock()
	st.s.queries = append(st.s.queries, st.query)
	return st.s.fail
}
func (st *advisoryStmt) Exec([]driver.Value) (driver.Result, error) {
	return driver.RowsAffected(0), st.record()
}
func (st *advisoryStmt) Query([]driver.Value) (driver.Rows, error) {
	if err := st.record(); err != nil {
		return nil, err
	}
	return &advisoryRows{value: st.s.grant}, nil
}

type advisoryRows struct {
	value bool
	done  bool
}

func (r *advisoryRows) Columns() []string { return []string{"ok"} }
func (r *advisoryRows) Close() error      { return nil }
func (r *advisoryRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = r.value
	return nil
}

func newAdvisoryLock(t *testing.T, server *advisoryServer) *advisoryLock {
	t.Helper()
	db := sql.OpenDB(connector{server})
	t.Cleanup(func() { _ = db.Close() })
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("conn: %v", err)
	}
	return &advisoryLock{conn: conn}
}

type connector struct{ s *advisoryServer }

func (c connector) Connect(context.Context) (driver.Conn, error) { return c.s.Open("") }
func (c connector) Driver() driver.Driver                        { return c.s }

func TestAdvisoryLock_ModesAndRelease(t *testing.T) {
	for _, exclusive := range []bool{false, true} {
		server := &advisoryServer{grant: true}
		lock := newAdvisoryLock(t, server)
		ok, err := lock.tryLock(exclusive)
		if err != nil || !ok {
			t.Fatalf("tryLock(%v) = %v, %v; want granted", exclusive, ok, err)
		}
		if err := lock.close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		take, release := "pg_try_advisory_lock_shared(", "pg_advisory_unlock_shared("
		if exclusive {
			take, release = "pg_try_advisory_lock(", "pg_advisory_unlock("
		}
		if len(server.queries) != 2 || !strings.Contains(server.queries[0], take) || !strings.Contains(server.queries[1], release) {
			t.Errorf("exclusive=%v queries = %q, want %s then %s", exclusive, server.queries, take, release)
		}
	}
}

func TestAdvisoryLock_RefusedAndFailing(t *testing.T) {
	server := &advisoryServer{grant: false}
	lock := newAdvisoryLock(t, server)
	if ok, err := lock.tryLock(true); err != nil || ok {
		t.Fatalf("tryLock = %v, %v; want refused", ok, err)
	}
	// Nothing is held, so close only returns the connection.
	if err := lock.close(); err != nil || len(server.queries) != 1 {
		t.Fatalf("close = %v after queries %q; want no unlock", err, server.queries)
	}

	boom := errors.New("boom")
	failing := newAdvisoryLock(t, &advisoryServer{fail: boom})
	if _, err := failing.tryLock(false); !errors.Is(err, boom) {
		t.Fatalf("tryLock error = %v, want boom", err)
	}
	failing.held = true
	if err := failing.close(); !errors.Is(err, boom) {
		t.Fatalf("close error = %v, want the unlock failure", err)
	}
}
