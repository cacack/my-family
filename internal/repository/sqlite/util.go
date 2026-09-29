package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	// modernc.org/sqlite is a pure-Go SQLite (no cgo), so every build —
	// including the CGO_ENABLED=0 release binaries — can open a database
	// (ADR-002, #822). It registers itself as the "sqlite" driver.
	_ "modernc.org/sqlite"
)

// DriverName is the database/sql driver name the SQLite stores run on.
const DriverName = "sqlite"

// dsnParams are applied by the driver to every new connection. Pragmas such
// as foreign_keys and busy_timeout are per-connection, so they belong in the
// DSN rather than in a one-off Exec on whichever connection the pool hands out.
// _time_format=sqlite writes any bound time.Time in SQLite's own date format
// rather than Go's time.String layout.
const dsnParams = "_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_foreign_keys=on&_time_format=sqlite"

// DSN returns the connection string OpenDB uses for the database file at path.
// The path is a plain file name, not a "file:" URI, so the driver strips the
// query before opening it and a path is never percent-decoded.
func DSN(path string) string {
	return path + "?" + dsnParams
}

// OpenDB opens a SQLite database connection with recommended settings.
func OpenDB(path string) (*sql.DB, error) {
	db, err := sql.Open(DriverName, DSN(path))
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// Set connection pool for SQLite (max 1 writer, but multiple readers)
	db.SetMaxOpenConns(1) // SQLite doesn't handle concurrent writes well
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

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
// pragma_table_info yields no rows for a table that does not exist, so a database
// with no `events` table at all reports (false, nil). A query error is returned, never
// swallowed: a failed probe must not be read as "not the read model's table".
func eventsTableBelongsToReadModel(db *sql.DB) (bool, error) {
	var ownerTypeCols int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('events') WHERE name = 'owner_type'`,
	).Scan(&ownerTypeCols); err != nil {
		return false, fmt.Errorf("inspect events table ownership: %w", err)
	}
	return ownerTypeCols > 0, nil
}

// parseTimestamp parses an ISO 8601 timestamp string.
func parseTimestamp(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05.999999999Z",
		"2006-01-02T15:04:05Z",
		time.RFC3339Nano,
		time.RFC3339,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse timestamp: %s", s)
}

// formatTimestamp formats a time to ISO 8601 string.
func formatTimestamp(t time.Time) string {
	return t.Format("2006-01-02T15:04:05.999999999Z07:00")
}

// parseNullableTimestamp parses a nullable ISO 8601 timestamp column. A NULL
// column maps to a nil *time.Time, never to the zero time, so "not set" stays
// distinguishable from "set to the zero instant".
func parseNullableTimestamp(s sql.NullString) (*time.Time, error) {
	if !s.Valid || s.String == "" {
		return nil, nil
	}
	t, err := parseTimestamp(s.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// nullableTimestamp formats a *time.Time for storage, mapping nil to NULL.
func nullableTimestamp(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: formatTimestamp(*t), Valid: true}
}

// nullableString converts an empty string to sql.NullString.
func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// nullableInt converts a *int to sql.NullInt64.
func nullableInt(i *int) sql.NullInt64 {
	if i == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*i), Valid: true}
}

// nullableBytes converts empty bytes to nil (for NULL in SQLite).
func nullableBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return string(b) // SQLite stores JSON as TEXT
}

// nullableBlob converts empty bytes to nil (NULL) and otherwise binds the bytes
// as a BLOB. Use it for binary columns (file and thumbnail bytes): unlike
// nullableBytes, which is for JSON stored as TEXT, it never converts to string,
// so arbitrary binary data (NUL bytes, invalid UTF-8) keeps BLOB storage class.
func nullableBlob(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}
