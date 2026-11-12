package pgtest

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestNewDatabase_FreshAndDropped(t *testing.T) {
	var dsn string
	t.Run("create", func(t *testing.T) {
		dsn = NewDatabase(t)
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer db.Close()
		var tables int
		if err := db.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'`).Scan(&tables); err != nil {
			t.Fatalf("query new database: %v", err)
		}
		if tables != 0 {
			t.Errorf("new database has %d tables, want 0", tables)
		}
	})
	if dsn == "" {
		return // skipped: no PostgreSQL available
	}

	// The subtest's cleanup has run, so its database must be gone.
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err == nil {
		t.Error("database still accepts connections after its test ended")
	}
}

func TestNewDatabase_Distinct(t *testing.T) {
	if a, b := NewDatabase(t), NewDatabase(t); a == b {
		t.Errorf("two calls returned the same database %q", a)
	}
}

func TestWithDatabase(t *testing.T) {
	got, err := withDatabase("postgres://u@host:5432/testdb?sslmode=disable", "t_x")
	if err != nil {
		t.Fatal(err)
	}
	if want := "postgres://u@host:5432/t_x?sslmode=disable"; got != want {
		t.Errorf("withDatabase = %q, want %q", got, want)
	}
	if _, err := withDatabase("postgres://u@host:bad/", "t_x"); err == nil {
		t.Error("malformed URL: want error")
	}
}

func TestRetry(t *testing.T) {
	calls := 0
	err := retry(context.Background(), func(context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("not yet")
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Errorf("retry = %v after %d calls, want nil after 3", err, calls)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	want := errors.New("never")
	if err := retry(ctx, func(context.Context) error { return want }); !errors.Is(err, want) {
		t.Errorf("retry on done context = %v, want last error %v", err, want)
	}
}

func TestPing_Unreachable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ping(ctx, "postgres://u@127.0.0.1:1/x?sslmode=disable"); err == nil {
		t.Error("ping unreachable server: want error")
	}
}
