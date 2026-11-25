package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"strings"
	"time"

	"github.com/lib/pq"
)

// Dialect names the database a backup reads, which decides how its read
// snapshot is opened.
type Dialect int

// The dialects Write reads.
const (
	SQLite Dialect = iota
	Postgres
)

// ErrReadModelOnlyValues refuses a backup of a database whose read model holds
// values no event records, which the archive would lose (ADR-010 §6). The
// caller finds them (storage's checkBackupSource); it lives here so the server
// can tell this refusal from a failed backup without importing storage.
var ErrReadModelOnlyValues = errors.New("the read model holds values that no event records, where a backup would lose them")

// ErrSourceShape refuses a backup of a database with a table that lacks a
// column this release reads, as when an older release last opened it and the
// stores have not been constructed on it since (ADR-010 §6).
var ErrSourceShape = errors.New("a table is not in this release's shape")

// ErrReadModelUnstamped refuses a backup of a database with events but no
// read-model version stamp: it predates #680, so its read model may hold
// values no event records, which the archive would lose (ADR-010 §6).
var ErrReadModelUnstamped = errors.New("the database predates the read model's version stamp (#680)")

// Write writes a backup archive of db to w and returns its manifest.
//
// Everything is read in one read transaction (ADR-010 §6): PostgreSQL's
// REPEATABLE READ, or SQLite's read transaction, which in WAL mode is a
// snapshot that does not block writers. A server may keep writing meanwhile;
// the archive holds the log as that snapshot saw it, and LastPosition is the
// highest position the snapshot holds. Write only reads: it runs no DDL.
//
// Write refuses a database it cannot copy whole (checkSource): one whose
// tables an older release last shaped, or one that may hold values only in
// its read model.
func Write(ctx context.Context, db *sql.DB, dialect Dialect, w io.Writer, appVersion string) (*Manifest, error) {
	opts := &sql.TxOptions{ReadOnly: true}
	if dialect == Postgres {
		opts.Isolation = sql.LevelRepeatableRead
	}
	tx, err := db.BeginTx(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("begin backup read: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := checkSource(ctx, tx, dialect); err != nil {
		return nil, err
	}

	manifest := &Manifest{
		FormatVersion: FormatVersion,
		AppVersion:    appVersion,
		CreatedAt:     formatTime(time.Now()),
		Entries:       []Entry{},
	}
	zw := zip.NewWriter(w)

	// The log first, then the tables beside it, all in the one snapshot.
	sum, err := writeEntry(zw, EventsEntry, func(out io.Writer) error {
		return copyRows(ctx, tx, eventsTable, out, func(row []any) {
			manifest.EventCount++
			// position is the last column, and NOT NULL.
			if position := row[len(row)-1].(int64); position > manifest.LastPosition {
				manifest.LastPosition = position
			}
		})
	})
	if err != nil {
		return nil, err
	}
	manifest.Entries = append(manifest.Entries, Entry{Name: EventsEntry, SHA256: sum})

	for _, t := range append([]table{streamsTable}, carriedTables...) {
		sum, err := writeEntry(zw, t.entry, func(out io.Writer) error {
			return copyRows(ctx, tx, t, out, nil)
		})
		if err != nil {
			return nil, err
		}
		manifest.Entries = append(manifest.Entries, Entry{Name: t.entry, SHA256: sum})
	}

	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode manifest: %w", err)
	}
	mw, err := zw.CreateHeader(&zip.FileHeader{Name: ManifestName, Method: zip.Deflate, Modified: time.Now().UTC()})
	if err != nil {
		return nil, fmt.Errorf("write %s: %w", ManifestName, err)
	}
	if _, err := mw.Write(append(encoded, '\n')); err != nil {
		return nil, fmt.Errorf("write %s: %w", ManifestName, err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("finish archive: %w", err)
	}
	return manifest, nil
}

// CheckSource refuses, in a read transaction of its own, a database Write
// would refuse (checkSource). It lets a caller run those checks before its
// own, which read tables checkSource has already found in shape.
func CheckSource(ctx context.Context, db *sql.DB, dialect Dialect) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("begin backup read: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	return checkSource(ctx, tx, dialect)
}

// checkSource refuses a database the archive could not copy whole.
//
// Every table must have the columns this release reads. A database an older
// release last opened may lack one, since the stores create and alter their
// tables only when constructed, which a backup does not do (ADR-010 §6).
//
// A database with events but no read-model version stamp predates #680, and
// may hold values only in its read model: coordinates and EXIDs imported
// before #1225, brick walls marked before #802, snapshots created before #624
// (ADR-010 §6). Every release that stamps the read model logs them, and its
// rebuild logs those left from before. A stamped database can still hold some,
// if a release before #680 wrote to it after a downgrade; the storage package
// checks for them directly (repository.PendingBackfills) before it calls
// Write.
func checkSource(ctx context.Context, tx *sql.Tx, dialect Dialect) error {
	for _, t := range append([]table{eventsTable, streamsTable}, carriedTables...) {
		names := make([]string, len(t.columns))
		for i, c := range t.columns {
			names[i] = c.name
		}
		// #nosec G202 -- table and column names are package constants, not input.
		// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
		rows, err := tx.QueryContext(ctx, "SELECT "+strings.Join(names, ", ")+" FROM "+t.name+" WHERE 1 = 0")
		if err != nil && !missingTableOrColumn(err) {
			return fmt.Errorf("read the database's %s table: %w", t.name, err)
		}
		if err != nil {
			return fmt.Errorf("%w: the database's %s table, as when an older release last opened it; "+
				"start `myfamily serve` with this release once, then back up: %w", ErrSourceShape, t.name, err)
		}
		_ = rows.Close()
	}

	exists := `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'read_model_schema'`
	if dialect == Postgres {
		exists = `SELECT CASE WHEN to_regclass('read_model_schema') IS NULL THEN 0 ELSE 1 END`
	}
	var stamped, version, events int
	if err := tx.QueryRowContext(ctx, exists).Scan(&stamped); err != nil {
		return fmt.Errorf("read the read model's schema version: %w", err)
	}
	if stamped > 0 {
		err := tx.QueryRowContext(ctx, `SELECT version FROM read_model_schema WHERE id = 1`).Scan(&version)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read the read model's schema version: %w", err)
		}
	}
	if version > 0 {
		return nil
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM (SELECT 1 FROM events LIMIT 1) head`).Scan(&events); err != nil {
		return fmt.Errorf("read events: %w", err)
	}
	if events > 0 {
		return fmt.Errorf("%w, so it may hold coordinates, EXIDs, brick walls or snapshots only in its read model, "+
			"where a backup would lose them; stop every server and run `myfamily rebuild`, which logs them, then back up",
			ErrReadModelUnstamped)
	}
	return nil
}

// missingTableOrColumn reports whether err names a table or column the
// database lacks, which a restart's store construction adds; any other error
// (a canceled read, a dropped connection, a busy database) is no refusal.
func missingTableOrColumn(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "42P01" || pqErr.Code == "42703" // undefined_table, undefined_column
	}
	msg := err.Error()
	return strings.Contains(msg, "no such table") || strings.Contains(msg, "no such column")
}

// writeEntry adds a deflated entry named name, filled by fill, and returns the
// SHA-256 of what fill wrote.
func writeEntry(zw *zip.Writer, name string, fill func(io.Writer) error) (string, error) {
	ew, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now().UTC()})
	if err != nil {
		return "", fmt.Errorf("write %s: %w", name, err)
	}
	h := sha256.New()
	if err := fill(io.MultiWriter(ew, h)); err != nil {
		return "", fmt.Errorf("write %s: %w", name, err)
	}
	return hexSum(h), nil
}

func hexSum(h hash.Hash) string { return hex.EncodeToString(h.Sum(nil)) }

// copyRows writes every row of t, in t.orderBy order, to out as JSON objects
// keyed by column name. seen, if set, is called with each row's logical
// values.
func copyRows(ctx context.Context, tx *sql.Tx, t table, out io.Writer, seen func([]any)) error {
	names := make([]string, len(t.columns))
	for i, c := range t.columns {
		names[i] = c.name
	}
	// #nosec G202 -- table and column names are package constants, not input.
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := tx.QueryContext(ctx, "SELECT "+strings.Join(names, ", ")+" FROM "+t.name+" ORDER BY "+t.orderBy)
	if err != nil {
		return fmt.Errorf("read %s: %w", t.name, err)
	}
	defer rows.Close()

	if t.format == jsonArray {
		if _, err := io.WriteString(out, "["); err != nil {
			return err
		}
	}
	raw := make([]any, len(t.columns))
	dest := make([]any, len(t.columns))
	for i := range raw {
		dest[i] = &raw[i]
	}
	values := make([]any, len(t.columns))
	var line bytes.Buffer
	first := true
	for rows.Next() {
		if err := rows.Scan(dest...); err != nil {
			return fmt.Errorf("read %s: %w", t.name, err)
		}
		for i, c := range t.columns {
			if values[i], err = logical(c.kind, raw[i]); err != nil {
				return fmt.Errorf("read %s.%s: %w", t.name, c.name, err)
			}
		}
		line.Reset()
		if t.format == jsonArray {
			if !first {
				line.WriteByte(',')
			}
			line.WriteByte('\n')
		}
		if err := encodeRow(&line, t.columns, values); err != nil {
			return fmt.Errorf("encode %s row: %w", t.name, err)
		}
		if t.format == ndjson {
			line.WriteByte('\n')
		}
		if _, err := out.Write(line.Bytes()); err != nil {
			return err
		}
		if seen != nil {
			seen(values)
		}
		first = false
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read %s: %w", t.name, err)
	}
	if t.format == jsonArray {
		end := "]\n"
		if !first {
			end = "\n]\n"
		}
		if _, err := io.WriteString(out, end); err != nil {
			return err
		}
	}
	return nil
}

// encodeRow writes one JSON object with the columns in their listed order.
func encodeRow(buf *bytes.Buffer, columns []column, values []any) error {
	buf.WriteByte('{')
	for i, c := range columns {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, _ := json.Marshal(c.name)
		buf.Write(key)
		buf.WriteByte(':')
		v, err := json.Marshal(values[i])
		if err != nil {
			return fmt.Errorf("%s: %w", c.name, err)
		}
		buf.Write(v)
	}
	buf.WriteByte('}')
	return nil
}

// logical turns a scanned column value into the archive's logical value: a
// string, int64, bool, RFC 3339 UTC time string, raw JSON, or nil for NULL.
// SQLite hands back text and integers for every column; PostgreSQL hands back
// time.Time, bool, and []byte for UUID and JSONB.
func logical(k kind, v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch k {
	case kindText:
		if s, ok := text(v); ok {
			return s, nil
		}
	case kindInt:
		if n, ok := v.(int64); ok {
			return n, nil
		}
	case kindBool:
		switch b := v.(type) {
		case bool:
			return b, nil
		case int64:
			if b == 0 || b == 1 {
				return b == 1, nil
			}
		}
	case kindTime, kindLogTime, kindDatetime:
		if t, ok := v.(time.Time); ok {
			return formatTime(t), nil
		}
		if s, ok := text(v); ok {
			t, err := parseTime(s)
			if err != nil {
				return nil, err
			}
			return formatTime(t), nil
		}
	case kindJSON:
		if s, ok := text(v); ok {
			if !json.Valid([]byte(s)) {
				return nil, errors.New("stored JSON is not valid")
			}
			return json.RawMessage(s), nil
		}
	}
	return nil, fmt.Errorf("unexpected stored value of type %T", v)
}

func text(v any) (string, bool) {
	switch s := v.(type) {
	case string:
		return s, true
	case []byte:
		return string(s), true
	}
	return "", false
}

// timeLayouts are the forms SQLite holds times in: RFC 3339 as the stores
// write them, and SQLite's own datetime('now') form, the streams default.
var timeLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05.999999999",
}

func parseTime(s string) (time.Time, error) {
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unreadable stored time %q", s)
}
