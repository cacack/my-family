package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// StoreMigrations returns the numbered migrations of the tables a rebuild does
// not re-create (ADR-011): the event log, accounts and settings. Its names and
// order match sqlite.StoreMigrations. Released migrations are never edited; a
// change to one of those tables is a new migration at the end of both lists
// (DB-010).
func StoreMigrations() []repository.StoreMigration {
	return []repository.StoreMigration{
		// Every database before the runner carries some older shape of these
		// tables, so the baseline alone is idempotent.
		{Name: "baseline", Apply: migrateBaseline},
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

// storeSchemaLockKey is the transaction-scoped advisory lock
// (pg_advisory_xact_lock) every migration takes before it re-reads the stamp,
// so two servers starting together apply each migration once. The value is
// arbitrary ("schema" in ASCII).
const storeSchemaLockKey int64 = 0x736368656d61

// storeSchemaBackend is PostgreSQL's repository.StoreSchemaBackend.
type storeSchemaBackend struct{}

// BeginLocked takes the advisory lock first, so creating the stamp table
// cannot race another process doing the same.
func (storeSchemaBackend) BeginLocked(ctx context.Context, conn *sql.Conn) (*sql.Tx, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, storeSchemaLockKey); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("lock store schema: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS store_schema (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			version INTEGER NOT NULL,
			name TEXT NOT NULL
		)`); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("create store_schema: %w", err)
	}
	return tx, nil
}

func (storeSchemaBackend) ReadStamp(ctx context.Context, q repository.RowQuerier) (repository.StoreSchemaStamp, error) {
	var exists bool
	if err := q.QueryRowContext(ctx, `SELECT to_regclass('store_schema') IS NOT NULL`).Scan(&exists); err != nil {
		return repository.StoreSchemaStamp{}, err
	}
	if !exists {
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
	_, err := tx.ExecContext(ctx, `
		INSERT INTO store_schema (id, version, name) VALUES (1, $1, $2)
		ON CONFLICT (id) DO UPDATE SET version = excluded.version, name = excluded.name`,
		stamp.Version, stamp.Name)
	return err
}

// SetForeignKeys does nothing: PostgreSQL checks foreign keys per statement and
// can change a key inside a transaction.
func (storeSchemaBackend) SetForeignKeys(context.Context, *sql.Conn, bool) error {
	return nil
}

// migrateBaseline is migration 1: the DDL the stores ran for these tables at
// start-up before the runner, including the two upgrades of the `events` table
// (the branch_id column and the composite version key). It meets unstamped
// databases of every earlier shape, so each statement is idempotent, and on a
// database already at this shape it changes nothing.
func migrateBaseline(ctx context.Context, tx *sql.Tx) error {
	// DO NOT REMOVE: this guard looks like dead code on a current database, but
	// it is what keeps a pre-#733 database from being handled blind. Before #733
	// the read model also owned a table named events, and ADR-002 puts both
	// stores in one database. Against that table every statement below aims at
	// the wrong relation: CREATE TABLE IF NOT EXISTS no-ops, ALTER TABLE ... ADD
	// COLUMN branch_id would land on the read model's table, and the composite
	// index finally fails on the missing stream_id. The event log must not
	// migrate a table it does not own, so refuse with a diagnosable error
	// instead; ReadModelStore.renameLegacyEventsTable is the mirror of this
	// check and owns the actual rename; both sides share one discriminator probe
	// (eventsTableBelongsToReadModel) so they can never disagree about who owns
	// the name (DB-007).
	ownedByReadModel, err := eventsTableBelongsToReadModel(ctx, tx)
	if err != nil {
		return err
	}
	if ownedByReadModel {
		return ErrReadModelEventsTable
	}

	// #nosec G201 -- mainBranch is a constant UUID literal, not user input.
	mainBranch := domain.MainBranchID.String()
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS streams (
			id UUID PRIMARY KEY,
			type VARCHAR(50) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			metadata JSONB
		);

		CREATE TABLE IF NOT EXISTS events (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			stream_id UUID NOT NULL REFERENCES streams(id),
			stream_type VARCHAR(50) NOT NULL,
			branch_id UUID NOT NULL DEFAULT '%[1]s',
			version BIGINT NOT NULL,
			event_type VARCHAR(100) NOT NULL,
			data JSONB NOT NULL,
			metadata JSONB,
			timestamp TIMESTAMPTZ NOT NULL,
			position BIGSERIAL UNIQUE
		);

		ALTER TABLE events ADD COLUMN IF NOT EXISTS branch_id UUID NOT NULL DEFAULT '%[1]s';

		-- Per-branch versioning (ADR-005): drop the pre-branch per-stream uniqueness.
		-- events_stream_id_version_key is the name Postgres auto-generated for the
		-- table-level UNIQUE(stream_id, version); idx_events_stream_version was its
		-- companion lookup index. Both are superseded by the composite unique index
		-- below. CREATE INDEX IF NOT EXISTS cannot redefine an existing index, hence
		-- the explicit DROP of the old one plus a new name; both are no-ops once done.
		ALTER TABLE events DROP CONSTRAINT IF EXISTS events_stream_id_version_key;
		DROP INDEX IF EXISTS idx_events_stream_version;
		CREATE UNIQUE INDEX IF NOT EXISTS idx_events_stream_branch_version ON events(stream_id, branch_id, version);

		CREATE INDEX IF NOT EXISTS idx_events_position ON events(position);
		CREATE INDEX IF NOT EXISTS idx_events_event_type ON events(event_type, timestamp);
		CREATE INDEX IF NOT EXISTS idx_events_timestamp_position ON events(timestamp, position);
		CREATE INDEX IF NOT EXISTS idx_events_branch ON events(branch_id);
		CREATE INDEX IF NOT EXISTS idx_events_stream_branch ON events(stream_id, branch_id);
	`, mainBranch)); err != nil {
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

// baselineAccountTablesDDL creates the account tables (ADR-007) and
// tree_settings.
const baselineAccountTablesDDL = `
	CREATE TABLE IF NOT EXISTS users (
		id UUID PRIMARY KEY,
		username TEXT NOT NULL UNIQUE,
		display_name TEXT NOT NULL,
		password_hash TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL,
		disabled BOOLEAN NOT NULL DEFAULT FALSE
	);

	CREATE TABLE IF NOT EXISTS tree_memberships (
		user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		tree_id UUID NOT NULL,
		role TEXT NOT NULL,
		PRIMARY KEY (user_id, tree_id)
	);

	CREATE TABLE IF NOT EXISTS sessions (
		token_hash TEXT PRIMARY KEY,
		user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		created_at TIMESTAMPTZ NOT NULL,
		last_seen_at TIMESTAMPTZ NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
	-- DeleteExpiredSessions filters on either time.
	CREATE INDEX IF NOT EXISTS idx_sessions_last_seen_at ON sessions(last_seen_at);
	CREATE INDEX IF NOT EXISTS idx_sessions_created_at ON sessions(created_at);

	CREATE TABLE IF NOT EXISTS api_tokens (
		id UUID PRIMARY KEY,
		token_hash TEXT NOT NULL UNIQUE,
		user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL,
		last_used_at TIMESTAMPTZ
	);

	CREATE INDEX IF NOT EXISTS idx_api_tokens_user_id ON api_tokens(user_id);

	CREATE TABLE IF NOT EXISTS invites (
		token_hash TEXT PRIMARY KEY,
		tree_id UUID NOT NULL,
		role TEXT NOT NULL,
		created_by UUID NOT NULL,
		created_at TIMESTAMPTZ NOT NULL,
		expires_at TIMESTAMPTZ NOT NULL
	);

	CREATE TABLE IF NOT EXISTS password_resets (
		token_hash TEXT PRIMARY KEY,
		user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		created_by UUID,
		created_at TIMESTAMPTZ NOT NULL,
		expires_at TIMESTAMPTZ NOT NULL
	);

	CREATE TABLE IF NOT EXISTS auth_audit (
		id BIGSERIAL PRIMARY KEY,
		at TIMESTAMPTZ NOT NULL,
		action TEXT NOT NULL,
		actor_id UUID,
		subject_id UUID,
		username TEXT NOT NULL,
		detail TEXT NOT NULL,
		client_ip TEXT NOT NULL
	);

	-- DeleteAuthAuditBefore filters on it.
	CREATE INDEX IF NOT EXISTS idx_auth_audit_at ON auth_audit(at);

	CREATE TABLE IF NOT EXISTS tree_settings (
		tree_id UUID PRIMARY KEY,
		living_threshold_years INTEGER NOT NULL,
		viewers_see_living BOOLEAN NOT NULL,
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
