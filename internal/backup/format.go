// Package backup writes, verifies and loads the backup archive of ADR-010: one
// zip holding the event log and the tables the log cannot rebuild, with a
// manifest that records a SHA-256 for every other entry.
//
// Values are logical, not backend-native, so an archive written from SQLite
// reads the same as one written from PostgreSQL: times are RFC 3339 strings in
// UTC with nanosecond precision, booleans are JSON booleans, UUIDs are strings,
// and the log's JSON columns are embedded as JSON values.
package backup

import "time"

// FormatVersion is the archive format this package writes and the newest it
// reads (ADR-010 §5).
const FormatVersion = 1

// ManifestName is the archive's manifest entry, the one entry it does not list.
const ManifestName = "manifest.json"

// Entry names of the event log.
const (
	EventsEntry  = "events.ndjson"
	StreamsEntry = "streams.ndjson"
)

// timeLayout is how every time is written: UTC, fixed-width nanoseconds.
const timeLayout = "2006-01-02T15:04:05.000000000Z"

// Manifest is the archive's manifest.json.
type Manifest struct {
	FormatVersion int    `json:"format_version"`
	AppVersion    string `json:"app_version"`
	CreatedAt     string `json:"created_at"`
	// EventCount is how many events events.ndjson holds.
	EventCount int64 `json:"event_count"`
	// LastPosition is the highest log position in the archive, 0 for an
	// empty log.
	LastPosition int64 `json:"last_position"`
	// Entries lists every entry but the manifest, with its SHA-256.
	Entries []Entry `json:"entries"`
}

// Entry is one archive entry's name and the lowercase hex SHA-256 of its
// content.
type Entry struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

// kind is how a column's value is written.
type kind int

const (
	kindText kind = iota
	kindInt
	kindBool
	// kindTime is a time SQLite holds in the fixed-width timeLayout, as the
	// account tables do, so that it sorts as text.
	kindTime
	// kindLogTime is a time SQLite holds as the stores' formatTimestamp
	// writes it.
	kindLogTime
	// kindDatetime is a time SQLite holds in its own datetime('now') form.
	kindDatetime
	kindJSON
)

// sqliteTimeLayouts is how SQLite holds each kind of time, so a restore
// writes a time as the store that owns the column would (ADR-010 §5).
var sqliteTimeLayouts = map[kind]string{
	kindTime:     timeLayout,
	kindLogTime:  "2006-01-02T15:04:05.999999999Z07:00",
	kindDatetime: "2006-01-02 15:04:05.999999999",
}

type column struct {
	name string
	kind kind
}

// table is one database table the archive carries. Columns are listed by
// name, so the archive never depends on a backend's column order, and
// TestCarriedColumnsMatchSchema keeps the list in step with each backend.
type table struct {
	name    string
	entry   string
	format  rowFormat
	orderBy string
	columns []column
}

// rowFormat is how an entry lays out its rows.
type rowFormat int

const (
	// ndjson writes one JSON object per line: the log.
	ndjson rowFormat = iota
	// jsonArray writes one JSON array of objects: a carried table.
	jsonArray
)

// eventsTable and streamsTable are the event log (ADR-010 §5).
var (
	eventsTable = table{name: "events", entry: EventsEntry, format: ndjson, orderBy: "position", columns: []column{
		{"id", kindText}, {"stream_id", kindText}, {"stream_type", kindText}, {"branch_id", kindText},
		{"version", kindInt}, {"event_type", kindText}, {"data", kindJSON}, {"metadata", kindJSON},
		{"timestamp", kindLogTime}, {"position", kindInt},
	}}
	streamsTable = table{name: "streams", entry: StreamsEntry, format: ndjson, orderBy: "id", columns: []column{
		{"id", kindText}, {"type", kindText}, {"created_at", kindDatetime}, {"metadata", kindJSON},
	}}
)

// carriedTables are the tables outside the log that the archive carries, each
// as tables/<name>.json (ADR-010 §2), in an order a restore can insert them
// in: users before the tables that reference them. A new table outside the log must be
// added here or to leftOutTables in the test, as the ADR's table says.
var carriedTables = []table{
	carried("users", "id", []column{
		{"id", kindText}, {"username", kindText}, {"display_name", kindText}, {"password_hash", kindText},
		{"created_at", kindLogTime}, {"disabled", kindBool},
	}),
	carried("tree_memberships", "user_id, tree_id", []column{
		{"user_id", kindText}, {"tree_id", kindText}, {"role", kindText},
	}),
	carried("site_settings", "key", []column{
		{"key", kindText}, {"value", kindText},
	}),
	carried("tree_settings", "tree_id", []column{
		{"tree_id", kindText}, {"living_threshold_years", kindInt}, {"viewers_see_living", kindBool},
		{"anonymous_access", kindText},
	}),
	carried("account_settings", "user_id, key", []column{
		{"user_id", kindText}, {"key", kindText}, {"value", kindText},
	}),
	carried("auth_audit", "id", []column{
		{"id", kindInt}, {"at", kindTime}, {"action", kindText}, {"actor_id", kindText},
		{"subject_id", kindText}, {"username", kindText}, {"detail", kindText}, {"client_ip", kindText},
	}),
	carried("api_tokens", "id", []column{
		{"id", kindText}, {"token_hash", kindText}, {"user_id", kindText}, {"name", kindText},
		{"created_at", kindTime}, {"last_used_at", kindTime},
	}),
}

func carried(name, orderBy string, columns []column) table {
	return table{name: name, entry: TableEntry(name), format: jsonArray, orderBy: orderBy, columns: columns}
}

// TableEntry is the archive entry of a carried table.
func TableEntry(name string) string { return "tables/" + name + ".json" }

// EntryNames returns every entry a backup holds besides the manifest, in the
// order they are written.
func EntryNames() []string {
	names := []string{EventsEntry, StreamsEntry}
	for _, t := range carriedTables {
		names = append(names, t.entry)
	}
	return names
}

// formatTime writes t as the archive does.
func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }
