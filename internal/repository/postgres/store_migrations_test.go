package postgres_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	pgstore "github.com/cacack/my-family/internal/repository/postgres"
	"github.com/cacack/my-family/internal/testutil/pgtest"
)

// migratorWith returns a migrator of db whose list is the real one plus extra,
// logging into the returned buffer.
func migratorWith(db *sql.DB, extra ...repository.StoreMigration) (*repository.StoreMigrator, *bytes.Buffer) {
	var logs bytes.Buffer
	m := pgstore.NewStoreMigrator(db)
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

// dumpDatabase renders every column, index and row of the current schema,
// except the stamp table's, so two dumps differ when anything outside the
// stamp changed.
func dumpDatabase(t *testing.T, db *sql.DB) string {
	t.Helper()
	var out strings.Builder
	cols, err := db.Query(`SELECT table_name, column_name, data_type, COALESCE(column_default, '')
		FROM information_schema.columns
		WHERE table_schema = CURRENT_SCHEMA() AND table_name != 'store_schema'
		ORDER BY table_name, column_name`)
	if err != nil {
		t.Fatalf("list columns: %v", err)
	}
	tables := map[string]bool{}
	var order []string
	for cols.Next() {
		var table, column, typ, def string
		if err := cols.Scan(&table, &column, &typ, &def); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		fmt.Fprintf(&out, "column %s.%s %s %s\n", table, column, typ, def)
		if !tables[table] {
			tables[table] = true
			order = append(order, table)
		}
	}
	cols.Close()
	idx, err := db.Query(`SELECT indexdef FROM pg_indexes
		WHERE schemaname = CURRENT_SCHEMA() AND tablename != 'store_schema' ORDER BY indexname`)
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	for idx.Next() {
		var def string
		if err := idx.Scan(&def); err != nil {
			t.Fatalf("scan index: %v", err)
		}
		fmt.Fprintf(&out, "index %s\n", def)
	}
	idx.Close()
	for _, table := range order {
		rows, err := db.Query(`SELECT row_to_json(t)::text FROM ` + table + ` t ORDER BY 1`)
		if err != nil {
			t.Fatalf("read %s: %v", table, err)
		}
		for rows.Next() {
			var row string
			if err := rows.Scan(&row); err != nil {
				t.Fatalf("scan %s: %v", table, err)
			}
			fmt.Fprintf(&out, "%s %s\n", table, row)
		}
		rows.Close()
	}
	return out.String()
}

// seedUnstampedDatabase builds a database as the release before the runner
// left it, with rows in the event log, accounts and settings: the same DDL, no
// store_schema table.
func seedUnstampedDatabase(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	events, err := pgstore.NewEventStore(db)
	if err != nil {
		t.Fatalf("event store: %v", err)
	}
	personID := uuid.New()
	if err := events.Append(ctx, personID, "Person", []domain.Event{domain.NewPersonCreated(domain.NewPerson("Ada", "Example"))}, -1, repository.MainScope); err != nil {
		t.Fatalf("append: %v", err)
	}
	accounts, err := pgstore.NewAccountStore(db)
	if err != nil {
		t.Fatalf("account store: %v", err)
	}
	user := &repository.User{ID: uuid.New(), Username: "ada", DisplayName: "Ada", PasswordHash: "x", CreatedAt: time.Now()}
	if err := accounts.CreateUser(ctx, user, repository.DefaultTreeID, repository.RoleOwner); err != nil {
		t.Fatalf("create user: %v", err)
	}
	settings, err := pgstore.NewSettingsStore(db)
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
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	db, cleanup := setupPostgres(t)
	defer cleanup()
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
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	db, cleanup := setupPostgres(t)
	defer cleanup()
	if err := pgstore.NewStoreMigrator(db).Run(ctx); err != nil {
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
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	db, cleanup := setupPostgres(t)
	defer cleanup()
	good := repository.StoreMigration{Name: "test_good", Apply: func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `ALTER TABLE site_settings ADD COLUMN note TEXT`)
		return err
	}}
	if m, _ := migratorWith(db, good); m.Run(ctx) != nil {
		t.Fatal("setup run failed")
	}
	before := dumpDatabase(t, db)

	boom := errors.New("boom")
	m, logs := migratorWith(db, good, repository.StoreMigration{Name: "test_bad", Apply: func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `CREATE TABLE half_done (x TEXT)`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `ALTER TABLE users ADD COLUMN half TEXT`); err != nil {
			return err
		}
		return boom
	}})
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
}

func TestStoreMigrations_RefusesNewerDatabase(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	db, cleanup := setupPostgres(t)
	defer cleanup()
	if err := pgstore.NewStoreMigrator(db).Run(ctx); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if _, err := db.Exec(`UPDATE store_schema SET version = 7, name = 'from_the_future'`); err != nil {
		t.Fatalf("stamp: %v", err)
	}
	m := pgstore.NewStoreMigrator(db)
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
	if _, err := pgstore.NewEventStore(db); !errors.Is(err, repository.ErrStoreSchemaNewer) {
		t.Errorf("NewEventStore on a newer database: err = %v, want ErrStoreSchemaNewer", err)
	}
}

func TestStoreMigrations_NoStampTableIsVersionZero(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, cleanup := setupPostgres(t)
	defer cleanup()
	if err := pgstore.NewStoreMigrator(db).Check(context.Background()); err != nil {
		t.Fatalf("Check on an empty database: %v", err)
	}
	if tableExists(t, db, "store_schema") {
		t.Error("Check created the stamp table; it must only read")
	}
}

// TestStoreMigrations_ConcurrentStartsApplyOnce runs two processes' worth of
// migrators, each with its own pool, against one database: the migration fails
// if it runs twice, so both succeed only if the second sees the first's stamp
// under the advisory lock.
func TestStoreMigrations_ConcurrentStartsApplyOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	dsn := pgtest.NewDatabase(t)
	once := repository.StoreMigration{Name: "test_once", Apply: func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `CREATE TABLE applied_once (x TEXT)`)
		return err
	}}
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errs {
		db, err := sql.Open("postgres", dsn)
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

// TestStoreMigrations_ReadsStampOfCurrentSchemaOnly pins the stamp lookup to
// CURRENT_SCHEMA(). With search_path=app,other and another install stamped in
// other, a lookup over the whole search_path would skip migrating app (its
// stores would then use other's event log and accounts) or refuse app because
// other is newer.
func TestStoreMigrations_ReadsStampOfCurrentSchemaOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, cleanup := setupPostgres(t)
	defer cleanup()
	// SET search_path is session state, so the pool must hand out one connection.
	db.SetMaxOpenConns(1)
	ctx := context.Background()

	if _, err := db.Exec(`CREATE SCHEMA other; SET search_path = other`); err != nil {
		t.Fatalf("set up other: %v", err)
	}
	if err := pgstore.NewStoreMigrator(db).Run(ctx); err != nil {
		t.Fatalf("migrate other: %v", err)
	}

	if _, err := db.Exec(`CREATE SCHEMA app; SET search_path = app, other`); err != nil {
		t.Fatalf("set up app: %v", err)
	}
	if _, err := pgstore.NewEventStore(db); err != nil {
		t.Fatalf("NewEventStore in app: %v", err)
	}
	for _, table := range []string{"app.store_schema", "app.events", "app.streams", "app.users", "app.site_settings"} {
		if !tableExists(t, db, table) {
			t.Errorf("%s was not created; app's stamp was read from other", table)
		}
	}

	if _, err := db.Exec(`UPDATE other.store_schema SET version = 99; CREATE SCHEMA fresh; SET search_path = fresh, other`); err != nil {
		t.Fatalf("stamp other newer: %v", err)
	}
	if err := pgstore.NewStoreMigrator(db).Check(ctx); err != nil {
		t.Errorf("Check of unstamped fresh beside a newer other: %v", err)
	}
}
