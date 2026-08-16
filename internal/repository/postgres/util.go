package postgres

import (
	"database/sql"
	"fmt"
	"time"
)

// OpenDB opens a PostgreSQL database connection.
func OpenDB(connStr string) (*sql.DB, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	return db, nil
}

// eventsTableBelongsToReadModel reports whether an existing `events` table is the
// read model's pre-#733 life-fact table rather than the event log's. Only the read
// model's table carries an owner_type column, so that column is the discriminator —
// and this function is its single home, shared by EventStore.createTables (which
// refuses such a database) and ReadModelStore.renameLegacyEventsTable (which renames
// it). Both stores must agree on who owns the name, so the probe must not be
// duplicated.
//
// It is scoped to CURRENT_SCHEMA(), which is where both stores' unqualified CREATE
// TABLE statements land; a table of the same name elsewhere on the search_path is
// not the one either store is about to touch. information_schema.columns yields no
// rows for a table that does not exist, so a database with no `events` table at all
// reports (false, nil). A query error is returned, never swallowed: a failed probe
// must not be read as "not the read model's table".
func eventsTableBelongsToReadModel(db *sql.DB) (bool, error) {
	var ownedByReadModel bool
	if err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = CURRENT_SCHEMA()
			  AND table_name = 'events' AND column_name = 'owner_type'
		)`).Scan(&ownedByReadModel); err != nil {
		return false, fmt.Errorf("inspect events table ownership: %w", err)
	}
	return ownedByReadModel, nil
}
