package sqlite_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

// openStoreSchemaDB opens a new database file in a temporary directory.
func openStoreSchemaDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "myfamily.db")
	db, err := sqlite.OpenDB(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, path
}

// migratorWith returns a migrator of db whose list is the real one plus extra,
// logging into the returned buffer.
func migratorWith(db *sql.DB, extra ...repository.StoreMigration) (*repository.StoreMigrator, *bytes.Buffer) {
	var logs bytes.Buffer
	m := sqlite.NewStoreMigrator(db)
	m.Migrations = append(m.Migrations, extra...)
	m.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	return m, &logs
}

func readStamp(t *testing.T, db *sql.DB) (int, string) {
	t.Helper()
	var version int
	var name string
	if err := db.QueryRow(`SELECT version, name FROM store_schema WHERE id = 1`).Scan(&version, &name); err != nil {
		t.Fatalf("read stamp: %v", err)
	}
	return version, name
}

// dumpDatabase renders every table's DDL and rows, except the stamp table's,
// so two dumps differ when any table outside the stamp changed.
func dumpDatabase(t *testing.T, db *sql.DB) string {
	t.Helper()
	rows, err := db.Query(`SELECT type, name, COALESCE(sql, '') FROM sqlite_master
		WHERE name NOT LIKE 'sqlite_%' AND tbl_name != 'store_schema' ORDER BY type, name`)
	if err != nil {
		t.Fatalf("list schema: %v", err)
	}
	var out strings.Builder
	var tables []string
	for rows.Next() {
		var typ, name, ddl string
		if err := rows.Scan(&typ, &name, &ddl); err != nil {
			t.Fatalf("scan schema: %v", err)
		}
		fmt.Fprintf(&out, "%s %s: %s\n", typ, name, ddl)
		if typ == "table" {
			tables = append(tables, name)
		}
	}
	rows.Close()
	for _, table := range tables {
		data, err := db.Query(`SELECT * FROM ` + table + ` ORDER BY 1`)
		if err != nil {
			t.Fatalf("read %s: %v", table, err)
		}
		cols, _ := data.Columns()
		for data.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := data.Scan(ptrs...); err != nil {
				t.Fatalf("scan %s: %v", table, err)
			}
			fmt.Fprintf(&out, "%s %v\n", table, vals)
		}
		data.Close()
	}
	return out.String()
}

// seedUnstampedDatabase builds a database as the release before the runner
// left it, with rows in the event log, accounts and settings: the same DDL, no
// store_schema table.
func seedUnstampedDatabase(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	events, err := sqlite.NewEventStore(db)
	if err != nil {
		t.Fatalf("event store: %v", err)
	}
	personID := uuid.New()
	if err := events.Append(ctx, personID, "Person", []domain.Event{domain.NewPersonCreated(domain.NewPerson("Ada", "Example"))}, -1, repository.MainScope); err != nil {
		t.Fatalf("append: %v", err)
	}
	accounts, err := sqlite.NewAccountStore(db)
	if err != nil {
		t.Fatalf("account store: %v", err)
	}
	user := &repository.User{ID: uuid.New(), Username: "ada", DisplayName: "Ada", PasswordHash: "x", CreatedAt: time.Now()}
	if err := accounts.CreateUser(ctx, user, repository.DefaultTreeID, repository.RoleOwner); err != nil {
		t.Fatalf("create user: %v", err)
	}
	settings, err := sqlite.NewSettingsStore(db)
	if err != nil {
		t.Fatalf("settings store: %v", err)
	}
	if err := settings.SetHomePerson(ctx, repository.SiteWide, personID); err != nil {
		t.Fatalf("set home person: %v", err)
	}
	if _, err := db.Exec(`DROP TABLE store_schema`); err != nil {
		t.Fatalf("drop stamp: %v", err)
	}
}

func TestStoreMigrations_StampsUnstampedDatabaseWithoutChangingIt(t *testing.T) {
	ctx := context.Background()
	db, _ := openStoreSchemaDB(t)
	seedUnstampedDatabase(t, db)
	before := dumpDatabase(t, db)

	m, logs := migratorWith(db)
	if err := m.Run(ctx); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if v, name := readStamp(t, db); v != 1 || name != "baseline" {
		t.Errorf("stamp = %d %q, want 1 baseline", v, name)
	}
	if after := dumpDatabase(t, db); after != before {
		t.Errorf("the baseline changed the database:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if !strings.Contains(logs.String(), "name=baseline") {
		t.Errorf("logs %q do not name the baseline", logs.String())
	}

	logs.Reset()
	if err := m.Run(ctx); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if logs.Len() != 0 {
		t.Errorf("second run applied something: %q", logs.String())
	}
	if v, _ := readStamp(t, db); v != 1 {
		t.Errorf("stamp after second run = %d, want 1", v)
	}
}

func TestStoreMigrations_AppliesLaterMigrationsOnceInOrder(t *testing.T) {
	ctx := context.Background()
	db, _ := openStoreSchemaDB(t)
	if err := sqlite.NewStoreMigrator(db).Run(ctx); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO site_settings (key, value) VALUES ('k', 'v')`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	m, logs := migratorWith(db,
		repository.StoreMigration{Name: "test_add_note", Apply: func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `ALTER TABLE site_settings ADD COLUMN note TEXT`)
			return err
		}},
		// Fails unless test_add_note ran first.
		repository.StoreMigration{Name: "test_fill_note", Apply: func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `UPDATE site_settings SET note = 'filled'`)
			return err
		}},
	)
	if err := m.Run(ctx); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := logs.String()
	first, second := strings.Index(out, "name=test_add_note"), strings.Index(out, "name=test_fill_note")
	if first < 0 || second < first || strings.Contains(out, "name=baseline") {
		t.Errorf("logs = %q, want test_add_note then test_fill_note, and not the baseline", out)
	}
	if v, name := readStamp(t, db); v != 3 || name != "test_fill_note" {
		t.Errorf("stamp = %d %q, want 3 test_fill_note", v, name)
	}
	var note string
	if err := db.QueryRow(`SELECT note FROM site_settings WHERE key = 'k'`).Scan(&note); err != nil || note != "filled" {
		t.Errorf("note = %q, %v; want filled", note, err)
	}

	logs.Reset()
	if err := m.Run(ctx); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if logs.Len() != 0 {
		t.Errorf("second run applied something: %q", logs.String())
	}
}

func TestStoreMigrations_FailedMigrationChangesNothing(t *testing.T) {
	ctx := context.Background()
	db, _ := openStoreSchemaDB(t)
	good := repository.StoreMigration{Name: "test_good", Apply: func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `ALTER TABLE site_settings ADD COLUMN note TEXT`)
		return err
	}}
	if m, _ := migratorWith(db, good); m.Run(ctx) != nil {
		t.Fatal("setup run failed")
	}
	before := dumpDatabase(t, db)

	boom := errors.New("boom")
	m, logs := migratorWith(db, good, repository.StoreMigration{
		Name:           "test_bad",
		ForeignKeysOff: true,
		Apply: func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `CREATE TABLE half_done (x TEXT)`); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `ALTER TABLE users ADD COLUMN half TEXT`); err != nil {
				return err
			}
			return boom
		},
	})
	err := m.Run(ctx)
	if !errors.Is(err, boom) {
		t.Fatalf("run: err = %v, want boom", err)
	}
	if !strings.Contains(err.Error(), "test_bad") || !strings.Contains(err.Error(), "3") {
		t.Errorf("error %q does not name the migration", err)
	}
	if !strings.Contains(logs.String(), "name=test_bad") {
		t.Errorf("logs %q: the failed migration was not logged before it ran", logs.String())
	}
	if v, name := readStamp(t, db); v != 2 || name != "test_good" {
		t.Errorf("stamp = %d %q, want 2 test_good", v, name)
	}
	if after := dumpDatabase(t, db); after != before {
		t.Errorf("the failed migration left changes:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	var fk int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil || fk != 1 {
		t.Errorf("foreign_keys = %d, %v after the migration; want back on", fk, err)
	}
}

func TestStoreMigrations_RefusesNewerDatabase(t *testing.T) {
	ctx := context.Background()
	db, _ := openStoreSchemaDB(t)
	if err := sqlite.NewStoreMigrator(db).Run(ctx); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if _, err := db.Exec(`UPDATE store_schema SET version = 7, name = 'from_the_future'`); err != nil {
		t.Fatalf("stamp: %v", err)
	}
	m := sqlite.NewStoreMigrator(db)
	for name, run := range map[string]func(context.Context) error{"Check": m.Check, "Run": m.Run} {
		err := run(ctx)
		if !errors.Is(err, repository.ErrStoreSchemaNewer) {
			t.Fatalf("%s: err = %v, want ErrStoreSchemaNewer", name, err)
		}
		for _, want := range []string{"version 7 (from_the_future)", "up to 1 (baseline)"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: error %q does not say %q", name, err, want)
			}
		}
	}
	if _, err := sqlite.NewEventStore(db); !errors.Is(err, repository.ErrStoreSchemaNewer) {
		t.Errorf("NewEventStore on a newer database: err = %v, want ErrStoreSchemaNewer", err)
	}
}

// TestStoreMigrations_CheckCurrent: the backup accepts only this binary's
// last migration, and an unstamped database only while the list holds just
// the baseline (ADR-011 §4). It only reads.
func TestStoreMigrations_CheckCurrent(t *testing.T) {
	ctx := context.Background()
	extra := repository.StoreMigration{Name: "extra", Apply: func(context.Context, *sql.Tx) error { return nil }}
	for _, tt := range []struct {
		name    string
		stamp   string // run on a database at the baseline; "" keeps it there
		extra   bool   // the binary knows one migration after the baseline
		refused []string
		newer   bool
	}{
		{name: "current"},
		{name: "unstamped, baseline only", stamp: `DROP TABLE store_schema`},
		{name: "unstamped row, baseline only", stamp: `UPDATE store_schema SET version = 0, name = ''`},
		{name: "older", extra: true, refused: []string{"version 1 (baseline)", "this binary's is 2 (extra)", "myfamily serve"}},
		{name: "unstamped, later migrations", stamp: `DROP TABLE store_schema`, extra: true,
			refused: []string{"version 0 (none)", "this binary's is 2 (extra)"}},
		{name: "newer", stamp: `UPDATE store_schema SET version = 7, name = 'from_the_future'`,
			refused: []string{"version 7 (from_the_future)", "up to 1 (baseline)"}, newer: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := openStoreSchemaDB(t)
			if err := sqlite.NewStoreMigrator(db).Run(ctx); err != nil {
				t.Fatalf("baseline: %v", err)
			}
			if tt.stamp != "" {
				if _, err := db.Exec(tt.stamp); err != nil {
					t.Fatalf("stamp: %v", err)
				}
			}
			before := dumpDatabase(t, db)
			m := sqlite.NewStoreMigrator(db)
			if tt.extra {
				m, _ = migratorWith(db, extra)
			}
			err := m.CheckCurrent(ctx)
			if tt.refused == nil {
				if err != nil {
					t.Fatalf("CheckCurrent: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("CheckCurrent: want a refusal")
				}
				for _, want := range tt.refused {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not say %q", err, want)
					}
				}
				if errors.Is(err, repository.ErrStoreSchemaNewer) != tt.newer {
					t.Errorf("errors.Is(%v, ErrStoreSchemaNewer) = %v, want %v", err, !tt.newer, tt.newer)
				}
				if errors.Is(err, repository.ErrStoreSchemaOlder) == tt.newer {
					t.Errorf("errors.Is(%v, ErrStoreSchemaOlder) = %v, want %v", err, tt.newer, !tt.newer)
				}
			}
			if after := dumpDatabase(t, db); after != before {
				t.Errorf("CheckCurrent changed the database:\n before: %s\n after:  %s", before, after)
			}
		})
	}
}

func TestStoreMigrations_NoStampTableIsVersionZero(t *testing.T) {
	db, _ := openStoreSchemaDB(t)
	if err := sqlite.NewStoreMigrator(db).Check(context.Background()); err != nil {
		t.Fatalf("Check on an empty database: %v", err)
	}
	var tables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master`).Scan(&tables); err != nil || tables != 0 {
		t.Errorf("Check created %d schema objects (%v); it must only read", tables, err)
	}
}

// TestStoreMigrations_ConcurrentStartsApplyOnce runs two processes' worth of
// migrators, each with its own connection, against one file: the migration
// fails if it runs twice, so both succeed only if the second sees the first's
// stamp under the lock.
func TestStoreMigrations_ConcurrentStartsApplyOnce(t *testing.T) {
	_, path := openStoreSchemaDB(t)
	once := repository.StoreMigration{Name: "test_once", Apply: func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `CREATE TABLE applied_once (x TEXT)`)
		return err
	}}
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errs {
		db, err := sqlite.OpenDB(path)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		t.Cleanup(func() { db.Close() })
		m, _ := migratorWith(db, once)
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = m.Run(context.Background())
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("migrator %d: %v", i, err)
		}
	}
}

// TestBranchStore_AddColumnFailureIsReturned is ADR-011 §5 for the branch
// registry: a failing ALTER at start-up is an error, not discarded as a
// duplicate column.
func TestBranchStore_AddColumnFailureIsReturned(t *testing.T) {
	db, _ := openStoreSchemaDB(t)
	// A pre-#55 table, then a read-only connection, so the first ALTER fails
	// as a locked or full database would.
	if _, err := db.Exec(`CREATE TABLE branches (id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT,
		base_position INTEGER NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL);
		CREATE INDEX idx_branches_created_at ON branches(created_at DESC);`); err != nil {
		t.Fatalf("create legacy branches: %v", err)
	}
	if _, err := db.Exec(`PRAGMA query_only = ON`); err != nil {
		t.Fatalf("query_only: %v", err)
	}
	_, err := sqlite.NewBranchStore(db)
	if err == nil || !strings.Contains(err.Error(), "add branches.merged_at") {
		t.Fatalf("NewBranchStore: err = %v, want the failed ALTER of merged_at", err)
	}
}
