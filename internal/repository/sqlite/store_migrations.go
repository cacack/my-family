package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// StoreMigrations returns the numbered migrations of the tables a rebuild does
// not re-create (ADR-011): the event log, accounts and settings. Its names and
// order match postgres.StoreMigrations. Released migrations are never edited;
// a change to one of those tables is a new migration at the end of both lists
// (DB-010).
func StoreMigrations() []repository.StoreMigration {
	return []repository.StoreMigration{
		// Every database before the runner carries some older shape of these
		// tables, so the baseline alone is idempotent; it needs foreign keys
		// off for the composite-key copy of a pre-ADR-005 events table.
		{Name: "baseline", Apply: migrateBaseline, ForeignKeysOff: true},
	}
}

// NewStoreMigrator returns a migrator of db with StoreMigrations.
func NewStoreMigrator(db *sql.DB) *repository.StoreMigrator {
	return &repository.StoreMigrator{DB: db, Backend: storeSchemaBackend{}, Migrations: StoreMigrations()}
}

// migrateStoreSchema brings db's store schema up to date, for the stores whose
// tables the migrations own.
func migrateStoreSchema(db *sql.DB) error {
	return NewStoreMigrator(db).Run(context.Background())
}

// storeSchemaBackend is SQLite's repository.StoreSchemaBackend.
type storeSchemaBackend struct{}

// BeginLocked creates the stamp table and its one row outside the transaction,
// each statement atomic on its own, then begins with a write so the
// transaction holds SQLite's write lock before it reads the stamp. A
// transaction that read first could not take the lock after another process
// committed.
func (storeSchemaBackend) BeginLocked(ctx context.Context, conn *sql.Conn) (*sql.Tx, error) {
	if _, err := conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS store_schema (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			version INTEGER NOT NULL,
			name TEXT NOT NULL
		);
		INSERT OR IGNORE INTO store_schema (id, version, name) VALUES (1, 0, '');
	`); err != nil {
		return nil, fmt.Errorf("create store_schema: %w", err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE store_schema SET version = version WHERE id = 1`); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("lock store_schema: %w", err)
	}
	return tx, nil
}

func (storeSchemaBackend) ReadStamp(ctx context.Context, q repository.RowQuerier) (repository.StoreSchemaStamp, error) {
	var tables int
	if err := q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'store_schema'`).Scan(&tables); err != nil {
		return repository.StoreSchemaStamp{}, err
	}
	if tables == 0 {
		return repository.StoreSchemaStamp{}, nil
	}
	var stamp repository.StoreSchemaStamp
	err := q.QueryRowContext(ctx, `SELECT version, name FROM store_schema WHERE id = 1`).Scan(&stamp.Version, &stamp.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return repository.StoreSchemaStamp{}, nil
	}
	return stamp, err
}

func (storeSchemaBackend) WriteStamp(ctx context.Context, tx *sql.Tx, stamp repository.StoreSchemaStamp) error {
	_, err := tx.ExecContext(ctx, `UPDATE store_schema SET version = ?, name = ? WHERE id = 1`, stamp.Version, stamp.Name)
	return err
}

// SetForeignKeys sets the pragma, which is a no-op inside a transaction, on
// the connection the migration's transaction runs on.
func (storeSchemaBackend) SetForeignKeys(ctx context.Context, conn *sql.Conn, on bool) error {
	pragma := "PRAGMA foreign_keys = OFF"
	if on {
		pragma = "PRAGMA foreign_keys = ON"
	}
	_, err := conn.ExecContext(ctx, pragma)
	return err
}

// migrateBaseline is migration 1: the DDL the stores ran for these tables at
// start-up before the runner, including the two one-off upgrades of the
// `events` table. It meets unstamped databases of every earlier shape, so each
// step is idempotent, and on a database already at this shape it changes
// nothing.
func migrateBaseline(ctx context.Context, tx *sql.Tx) error {
	mainBranch := domain.MainBranchID.String()

	// #733 guard: refuse a database whose `events` table is the read model's,
	// BEFORE either upgrade touches it. The branch_id step would ALTER a column
	// onto the read model's table, and the composite-key step would read its DDL,
	// conclude the event log is legacy, and abort mid-copy with the opaque
	// "copy events: no such column: stream_id". This is NOT dead code: pre-#733
	// databases in the wild still carry the old name, and the read model only
	// renames itself when it is opened (DB-007). Do not remove it.
	ownedByReadModel, err := eventsTableBelongsToReadModel(ctx, tx)
	if err != nil {
		return err
	}
	if ownedByReadModel {
		return ErrReadModelEventsTable
	}

	// Both upgrades run before the CREATE batch, which indexes branch_id.
	if err := migrateEventsBranchID(ctx, tx, mainBranch); err != nil {
		return err
	}
	if err := migrateEventsCompositeVersionKey(ctx, tx, mainBranch); err != nil {
		return err
	}

	// #nosec G201 -- mainBranch is a constant UUID literal, not user input.
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS streams (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			metadata TEXT
		);

		%[1]s;

		%[2]s
	`, baselineEventsTableDDL("IF NOT EXISTS events", mainBranch), baselineEventsIndexDDL)); err != nil {
		return fmt.Errorf("create event log tables: %w", err)
	}
	if _, err := tx.ExecContext(ctx, baselineAccountTablesDDL); err != nil {
		return fmt.Errorf("create account tables: %w", err)
	}
	if _, err := tx.ExecContext(ctx, baselineSettingsTablesDDL); err != nil {
		return fmt.Errorf("create settings tables: %w", err)
	}
	return nil
}

// baselineEventsTableDDL returns the CREATE TABLE statement for the events
// table. target is the name clause, "IF NOT EXISTS events" for the fresh create
// and "events_new" for the legacy copy, so both share one definition.
func baselineEventsTableDDL(target, mainBranch string) string {
	// #nosec G201 -- target is a package-internal literal and mainBranch is a
	// constant UUID literal; neither is user input.
	return fmt.Sprintf(`
		CREATE TABLE %[1]s (
			id TEXT PRIMARY KEY,
			stream_id TEXT NOT NULL,
			stream_type TEXT NOT NULL,
			branch_id TEXT NOT NULL DEFAULT '%[2]s',
			version INTEGER NOT NULL,
			event_type TEXT NOT NULL,
			data TEXT NOT NULL,
			metadata TEXT,
			timestamp TEXT NOT NULL,
			position INTEGER NOT NULL,
			FOREIGN KEY (stream_id) REFERENCES streams(id),
			UNIQUE(stream_id, branch_id, version)
		)`, target, mainBranch)
}

// baselineEventsIndexDDL is the full index set for the events table. The
// legacy copy recreates it verbatim because DROP TABLE takes the original
// indexes with it.
const baselineEventsIndexDDL = `
	CREATE INDEX IF NOT EXISTS idx_events_stream_branch_version ON events(stream_id, branch_id, version);
	CREATE INDEX IF NOT EXISTS idx_events_position ON events(position);
	CREATE INDEX IF NOT EXISTS idx_events_event_type ON events(event_type, timestamp);
	CREATE INDEX IF NOT EXISTS idx_events_timestamp_position ON events(timestamp, position);
	CREATE INDEX IF NOT EXISTS idx_events_branch_julian_position ON events(branch_id, julianday(timestamp), position, event_type);
	CREATE INDEX IF NOT EXISTS idx_events_julian_position_branch ON events(julianday(timestamp), position, branch_id, event_type);
	CREATE INDEX IF NOT EXISTS idx_events_branch ON events(branch_id);
	CREATE INDEX IF NOT EXISTS idx_events_stream_branch ON events(stream_id, branch_id);
`

// migrateEventsBranchID adds the branch_id column (ADR-005) to an events table
// created before it. SQLite has no ADD COLUMN IF NOT EXISTS, so the column is
// looked up first; a fresh database has no events table yet and is skipped.
func migrateEventsBranchID(ctx context.Context, tx *sql.Tx, mainBranch string) error {
	var tables, present int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'events'`).Scan(&tables); err != nil {
		return fmt.Errorf("inspect events table: %w", err)
	}
	if tables == 0 {
		return nil
	}
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('events') WHERE name = 'branch_id'`).Scan(&present); err != nil {
		return fmt.Errorf("inspect events table: %w", err)
	}
	if present > 0 {
		return nil
	}
	// #nosec G201 -- mainBranch is a constant UUID literal, not user input.
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(
		"ALTER TABLE events ADD COLUMN branch_id TEXT NOT NULL DEFAULT '%s'", mainBranch)); err != nil {
		return fmt.Errorf("add events.branch_id: %w", err)
	}
	return nil
}

// migrateEventsCompositeVersionKey upgrades a legacy events table whose UNIQUE
// constraint is (stream_id, version) to (stream_id, branch_id, version), so
// optimistic versioning is per-branch (ADR-005). SQLite cannot alter a table
// constraint, so this copies the table, with foreign keys off (the migration's
// ForeignKeysOff). It checks the row count and keeps every position and id.
// Fresh databases and already-composite ones are left untouched.
func migrateEventsCompositeVersionKey(ctx context.Context, tx *sql.Tx, mainBranch string) error {
	legacy, err := hasLegacyVersionKey(ctx, tx)
	if err != nil {
		return err
	}
	if !legacy {
		return nil
	}

	slog.Info("rebuilding sqlite events table for per-branch versioning (ADR-005): UNIQUE(stream_id, version) -> UNIQUE(stream_id, branch_id, version)")

	var before int64
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM events").Scan(&before); err != nil {
		return fmt.Errorf("count events: %w", err)
	}
	if _, err := tx.ExecContext(ctx, baselineEventsTableDDL("events_new", mainBranch)); err != nil {
		return fmt.Errorf("create events_new: %w", err)
	}
	// Explicit column lists: every position and id value is carried over as-is.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO events_new (id, stream_id, stream_type, branch_id, version, event_type, data, metadata, timestamp, position)
		SELECT id, stream_id, stream_type, branch_id, version, event_type, data, metadata, timestamp, position FROM events
	`); err != nil {
		return fmt.Errorf("copy events: %w", err)
	}
	var after int64
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM events_new").Scan(&after); err != nil {
		return fmt.Errorf("count copied events: %w", err)
	}
	if after != before {
		return fmt.Errorf("copied %d of %d events", after, before)
	}
	if _, err := tx.ExecContext(ctx, "DROP TABLE events"); err != nil {
		return fmt.Errorf("drop legacy events: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "ALTER TABLE events_new RENAME TO events"); err != nil {
		return fmt.Errorf("rename events_new: %w", err)
	}
	if _, err := tx.ExecContext(ctx, baselineEventsIndexDDL); err != nil {
		return fmt.Errorf("recreate indexes: %w", err)
	}
	return nil
}

// hasLegacyVersionKey reports whether the stored events DDL predates the
// composite UNIQUE(stream_id, branch_id, version) constraint. sqlite_master
// holds the CREATE statement verbatim, so the match is made against a
// whitespace- and case-normalized copy. A missing events table is not legacy.
func hasLegacyVersionKey(ctx context.Context, tx *sql.Tx) (bool, error) {
	var ddl string
	err := tx.QueryRowContext(ctx,
		"SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'events'").Scan(&ddl)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect events schema: %w", err)
	}
	normalized := strings.ToLower(strings.Join(strings.Fields(ddl), ""))
	return !strings.Contains(normalized, "unique(stream_id,branch_id,version)"), nil
}

// baselineAccountTablesDDL creates the account tables (ADR-007) and
// tree_settings.
const baselineAccountTablesDDL = `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		username TEXT NOT NULL UNIQUE,
		display_name TEXT NOT NULL,
		password_hash TEXT NOT NULL,
		created_at TEXT NOT NULL,
		disabled INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS tree_memberships (
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		tree_id TEXT NOT NULL,
		role TEXT NOT NULL,
		PRIMARY KEY (user_id, tree_id)
	);

	CREATE TABLE IF NOT EXISTS sessions (
		token_hash TEXT PRIMARY KEY,
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		created_at TEXT NOT NULL,
		last_seen_at TEXT NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
	-- DeleteExpiredSessions filters on either time.
	CREATE INDEX IF NOT EXISTS idx_sessions_last_seen_at ON sessions(last_seen_at);
	CREATE INDEX IF NOT EXISTS idx_sessions_created_at ON sessions(created_at);

	CREATE TABLE IF NOT EXISTS api_tokens (
		id TEXT PRIMARY KEY,
		token_hash TEXT NOT NULL UNIQUE,
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		created_at TEXT NOT NULL,
		last_used_at TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_api_tokens_user_id ON api_tokens(user_id);

	CREATE TABLE IF NOT EXISTS invites (
		token_hash TEXT PRIMARY KEY,
		tree_id TEXT NOT NULL,
		role TEXT NOT NULL,
		created_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		expires_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS password_resets (
		token_hash TEXT PRIMARY KEY,
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		created_by TEXT,
		created_at TEXT NOT NULL,
		expires_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS auth_audit (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		at TEXT NOT NULL,
		action TEXT NOT NULL,
		actor_id TEXT,
		subject_id TEXT,
		username TEXT NOT NULL,
		detail TEXT NOT NULL,
		client_ip TEXT NOT NULL
	);

	-- DeleteAuthAuditBefore filters on it.
	CREATE INDEX IF NOT EXISTS idx_auth_audit_at ON auth_audit(at);

	CREATE TABLE IF NOT EXISTS tree_settings (
		tree_id TEXT PRIMARY KEY,
		living_threshold_years INTEGER NOT NULL,
		viewers_see_living INTEGER NOT NULL,
		anonymous_access TEXT NOT NULL
	);
`

// baselineSettingsTablesDDL creates the site and account settings tables.
const baselineSettingsTablesDDL = `
	CREATE TABLE IF NOT EXISTS site_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS account_settings (
		user_id TEXT NOT NULL,
		key TEXT NOT NULL,
		value TEXT NOT NULL,
		PRIMARY KEY (user_id, key)
	);
`
