package sqlite_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/repository/sqlite"
)

// TestAccountStore_CleanupUsesIndexes pins that the expired-session and
// audit-retention deletes search an index instead of scanning their table
// (#1433). The session delete ORs two times, so it needs one index per time.
func TestAccountStore_CleanupUsesIndexes(t *testing.T) {
	db, err := sqlite.OpenDB(filepath.Join(t.TempDir(), "myfamily.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := sqlite.NewAccountStore(db); err != nil {
		t.Fatalf("NewAccountStore: %v", err)
	}
	wantIndexes := map[string][]string{
		"sessions":   {"idx_sessions_last_seen_at", "idx_sessions_created_at"},
		"auth_audit": {"idx_auth_audit_at"},
	}
	for table, query := range sqlite.CleanupDeletes {
		args := make([]any, strings.Count(query, "?"))
		for i := range args {
			args[i] = "2026-01-01T00:00:00.000000Z"
		}
		rows, err := db.Query("EXPLAIN QUERY PLAN "+query, args...)
		if err != nil {
			t.Fatalf("%s: explain: %v", table, err)
		}
		var steps []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatalf("%s: scan plan: %v", table, err)
			}
			steps = append(steps, detail)
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("%s: plan: %v", table, err)
		}
		plan := strings.Join(steps, " | ")
		t.Logf("%s: %s", table, plan)
		if strings.Contains(plan, "SCAN "+table) {
			t.Errorf("%s: plan %q scans the table", table, plan)
		}
		for _, index := range wantIndexes[table] {
			if !strings.Contains(plan, "USING INDEX "+index) {
				t.Errorf("%s: plan %q, want a search of %s", table, plan, index)
			}
		}
	}
}
