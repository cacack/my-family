package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	pgstore "github.com/cacack/my-family/internal/repository/postgres"
)

// TestAccountStore_CleanupUsesIndexes pins that an index can serve the
// expired-session and audit-retention deletes (#1433). Sequential scans are
// disabled for the plan, since on a near-empty table one is cheaper; what is
// pinned is that the planner has an index to use, one per OR-ed session time.
func TestAccountStore_CleanupUsesIndexes(t *testing.T) {
	db, cleanup := setupPostgres(t)
	defer cleanup()
	if _, err := pgstore.NewAccountStore(db); err != nil {
		t.Fatalf("NewAccountStore: %v", err)
	}
	wantIndexes := map[string][]string{
		"sessions":   {"idx_sessions_last_seen_at", "idx_sessions_created_at"},
		"auth_audit": {"idx_auth_audit_at"},
	}
	ctx := context.Background()
	for table, query := range pgstore.CleanupDeletes {
		args := make([]any, strings.Count(query, "$"))
		for i := range args {
			args[i] = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		if _, err := tx.ExecContext(ctx, "SET LOCAL enable_seqscan = off"); err != nil {
			t.Fatalf("disable seqscan: %v", err)
		}
		rows, err := tx.QueryContext(ctx, "EXPLAIN "+query, args...)
		if err != nil {
			t.Fatalf("%s: explain: %v", table, err)
		}
		var steps []string
		for rows.Next() {
			var step string
			if err := rows.Scan(&step); err != nil {
				t.Fatalf("%s: scan plan: %v", table, err)
			}
			steps = append(steps, strings.TrimSpace(step))
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("%s: plan: %v", table, err)
		}
		_ = tx.Rollback()
		plan := strings.Join(steps, " | ")
		t.Logf("%s: %s", table, plan)
		if strings.Contains(plan, "Seq Scan") {
			t.Errorf("%s: plan %q scans the table", table, plan)
		}
		for _, index := range wantIndexes[table] {
			if !strings.Contains(plan, " on "+index) && !strings.Contains(plan, " using "+index) {
				t.Errorf("%s: plan %q, want a search of %s", table, plan, index)
			}
		}
	}
}
