package backup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// ErrNotEmpty is returned by Load for a database that already holds data:
// restore never merges a backup into an instance (ADR-010 §4).
var ErrNotEmpty = errors.New("the database already holds data; restore only into an empty database")

// Archive is a verified backup archive, ready to load into a database.
type Archive struct {
	Manifest *Manifest
	file     *os.File
	entries  map[string]*zip.File
	sums     map[string]string
}

// OpenArchive opens and verifies the archive at path (see Verify), so a
// damaged or too-new archive is refused before any database is touched. The
// caller must Close it.
func OpenArchive(path string) (*Archive, error) {
	// #nosec G304 -- path is the archive the operator named.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	a, err := newArchive(f)
	if err != nil {
		return nil, errors.Join(err, f.Close())
	}
	return a, nil
}

func newArchive(f *os.File) (*Archive, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	m, err := Verify(f, info.Size())
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(f, info.Size())
	if err != nil {
		return nil, err
	}
	a := &Archive{Manifest: m, file: f, entries: map[string]*zip.File{}, sums: map[string]string{}}
	for _, zf := range zr.File {
		a.entries[zf.Name] = zf
	}
	for _, e := range m.Entries {
		a.sums[e.Name] = e.SHA256
	}
	return a, nil
}

// Close closes the archive's file.
func (a *Archive) Close() error { return a.file.Close() }

// Load inserts the archive's event log and carried tables into db, which must
// be empty (ADR-010 §4) and already hold this release's tables, in one
// transaction: restore steps 3 and 4. Every column is written as archived,
// log positions included, through the target's own encoding. A failure rolls
// back and leaves db as it was. The caller replays the log afterwards.
//
// Each row loads by column name: a column the archive lacks takes the
// column's default, and a column this release does not know fails the load.
func (a *Archive) Load(ctx context.Context, db *sql.DB, dialect Dialect) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin restore: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	tables := append([]table{streamsTable, eventsTable}, carriedTables...)
	if err := checkEmpty(ctx, tx, tables); err != nil {
		return err
	}
	for _, t := range tables {
		if err := a.loadTable(ctx, tx, dialect, t); err != nil {
			return err
		}
	}
	if err := checkLoaded(ctx, tx, a.Manifest); err != nil {
		return err
	}
	if dialect == Postgres {
		// The serial columns the load wrote explicit values into would
		// otherwise hand out values already taken (ADR-010 restore step 4).
		for _, t := range tables {
			if t.serial == "" {
				continue
			}
			// #nosec G202 -- table and column names are package constants, not input.
			// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
			query := "SELECT setval(pg_get_serial_sequence('" + t.name + "', '" + t.serial + "'), " +
				"COALESCE(MAX(" + t.serial + "), 0) + 1, false) FROM " + t.name
			if _, err := tx.ExecContext(ctx, query); err != nil {
				return fmt.Errorf("reset the %s.%s sequence: %w", t.name, t.serial, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit restore: %w", err)
	}
	return nil
}

// checkEmpty refuses a database with a row in the log or any carried table.
func checkEmpty(ctx context.Context, tx *sql.Tx, tables []table) error {
	for _, t := range tables {
		var rows int
		// #nosec G202 -- table names are package constants, not input.
		// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM (SELECT 1 FROM "+t.name+" LIMIT 1) head").Scan(&rows); err != nil {
			return fmt.Errorf("check %s is empty: %w", t.name, err)
		}
		if rows > 0 {
			return fmt.Errorf("%w: its %s table holds rows", ErrNotEmpty, t.name)
		}
	}
	return nil
}

// checkLoaded confirms the loaded log is the one the manifest describes.
func checkLoaded(ctx context.Context, tx *sql.Tx, m *Manifest) error {
	var count, last int64
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*), COALESCE(MAX(position), 0) FROM events").Scan(&count, &last); err != nil {
		return fmt.Errorf("count restored events: %w", err)
	}
	if count != m.EventCount || last != m.LastPosition {
		return fmt.Errorf("restored %d events ending at position %d, the manifest says %d ending at %d",
			count, last, m.EventCount, m.LastPosition)
	}
	return nil
}

// loadTable inserts every row of t's entry. It hashes the entry as it reads
// it and fails unless the sum is still the manifest's, so a file changed
// since OpenArchive verified it is not loaded.
func (a *Archive) loadTable(ctx context.Context, tx *sql.Tx, dialect Dialect, t table) error {
	zf := a.entries[t.entry]
	if zf == nil {
		return fmt.Errorf("entry %s is missing", t.entry)
	}
	rc, err := zf.Open()
	if err != nil {
		return fmt.Errorf("read %s: %w", t.entry, err)
	}
	defer rc.Close()
	h := sha256.New()
	content := io.TeeReader(rc, h)
	dec := json.NewDecoder(content)

	if t.format == jsonArray {
		if tok, err := dec.Token(); err != nil || tok != json.Delim('[') {
			return fmt.Errorf("read %s: not a JSON array", t.entry)
		}
	}
	ins := inserter{tx: tx, table: t}
	defer ins.close()
	for n := 1; dec.More(); n++ {
		var row map[string]json.RawMessage
		if err := dec.Decode(&row); err != nil {
			return fmt.Errorf("read %s row %d: %w", t.entry, n, err)
		}
		if err := ins.insert(ctx, dialect, row); err != nil {
			return fmt.Errorf("restore %s row %d: %w", t.entry, n, err)
		}
	}
	if t.format == jsonArray {
		if _, err := dec.Token(); err != nil {
			return fmt.Errorf("read %s: %w", t.entry, err)
		}
	}
	// #nosec G110 -- the rest of an entry already verified; it is hashed, not held.
	if _, err := io.Copy(io.Discard, content); err != nil {
		return fmt.Errorf("read %s: %w", t.entry, err)
	}
	if hexSum(h) != a.sums[t.entry] {
		return fmt.Errorf("entry %s changed after it was verified", t.entry)
	}
	return nil
}

// inserter inserts rows into one table, preparing one statement per set of
// columns the rows hold (in practice one per table).
type inserter struct {
	tx      *sql.Tx
	table   table
	columns string
	stmt    *sql.Stmt
}

func (ins *inserter) insert(ctx context.Context, dialect Dialect, row map[string]json.RawMessage) error {
	var names, marks []string
	var args []any
	for _, c := range ins.table.columns {
		raw, ok := row[c.name]
		if !ok {
			continue // the column's default
		}
		v, err := storedValue(dialect, c.kind, raw)
		if err != nil {
			return fmt.Errorf("%s: %w", c.name, err)
		}
		names = append(names, c.name)
		args = append(args, v)
		marks = append(marks, "$"+strconv.Itoa(len(args)))
	}
	if len(names) != len(row) {
		for name := range row {
			if !ins.table.has(name) {
				return fmt.Errorf("column %s is not one this release knows; use a newer my-family", name)
			}
		}
	}
	if joined := strings.Join(names, ", "); ins.stmt == nil || joined != ins.columns {
		ins.close()
		// #nosec G202 -- table and column names are package constants, not input.
		// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
		stmt, err := ins.tx.PrepareContext(ctx, "INSERT INTO "+ins.table.name+" ("+joined+") VALUES ("+strings.Join(marks, ", ")+")")
		if err != nil {
			return err
		}
		ins.stmt, ins.columns = stmt, joined
	}
	_, err := ins.stmt.ExecContext(ctx, args...)
	return err
}

func (ins *inserter) close() {
	if ins.stmt != nil {
		_ = ins.stmt.Close()
		ins.stmt = nil
	}
}

func (t table) has(name string) bool {
	for _, c := range t.columns {
		if c.name == name {
			return true
		}
	}
	return false
}

// storedValue turns an archived JSON value into what the target backend
// stores for a column of kind k: SQLite keeps booleans as 0/1 and times as
// text in the column's own layout, PostgreSQL takes bool and time.Time.
func storedValue(dialect Dialect, k kind, raw json.RawMessage) (any, error) {
	if string(raw) == "null" {
		return nil, nil
	}
	switch k {
	case kindText:
		var s string
		err := json.Unmarshal(raw, &s)
		return s, err
	case kindInt:
		var n int64
		err := json.Unmarshal(raw, &n)
		return n, err
	case kindBool:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, err
		}
		if dialect == SQLite {
			if b {
				return int64(1), nil
			}
			return int64(0), nil
		}
		return b, nil
	case kindTime, kindLogTime, kindDatetime:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return nil, err
		}
		if dialect == SQLite {
			return t.UTC().Format(sqliteTimeLayouts[k]), nil
		}
		return t, nil
	case kindJSON:
		if !json.Valid(raw) {
			return nil, errors.New("not valid JSON")
		}
		return string(raw), nil
	}
	return nil, fmt.Errorf("unknown column kind %d", k)
}
