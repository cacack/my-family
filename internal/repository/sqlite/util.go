package sqlite

import (
	"database/sql"
	"fmt"
	"time"
)

// OpenDB opens a SQLite database connection with recommended settings.
// The mattn/go-sqlite3 driver should be built with CGO_ENABLED=1.
// FTS5 is enabled via the "fts5" build tag or when the SQLite library supports it.
func OpenDB(path string) (*sql.DB, error) {
	// Note: go-sqlite3 includes FTS5 by default when compiled with CGO
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_foreign_keys=on")
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

// nullableUUID converts a *uuid.UUID to sql.NullString.
func nullableUUID(id *[16]byte) sql.NullString {
	if id == nil {
		return sql.NullString{}
	}
	return sql.NullString{
		String: fmt.Sprintf("%x-%x-%x-%x-%x", id[0:4], id[4:6], id[6:8], id[8:10], id[10:16]),
		Valid:  true,
	}
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
