package postgres_test

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	pgstore "github.com/cacack/my-family/internal/repository/postgres"
)

// TestResetReadModel_CoversEveryReadModelTable fails when a read-model table is
// added without being added to the reset list, which would let a rebuild
// replay the log on top of stale rows (#845).
func TestResetReadModel_CoversEveryReadModelTable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, cleanup := setupPostgres(t)
	defer cleanup()
	if _, err := pgstore.NewReadModelStore(db); err != nil {
		t.Fatalf("read model store: %v", err)
	}
	if _, err := pgstore.NewEventStore(db); err != nil {
		t.Fatalf("event store: %v", err)
	}
	if _, err := pgstore.NewBranchStore(db); err != nil {
		t.Fatalf("branch store: %v", err)
	}
	if _, err := pgstore.NewSnapshotStore(db); err != nil {
		t.Fatalf("snapshot store: %v", err)
	}

	rows, err := db.Query(`SELECT table_name FROM information_schema.tables
		WHERE table_schema = CURRENT_SCHEMA() AND table_type = 'BASE TABLE'`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()
	// Tables ResetReadModel keeps; rebuild_state must outlive the reset.
	logOwned := map[string]bool{"events": true, "streams": true, "branches": true, "snapshots": true, "rebuild_state": true}
	var schema []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if !logOwned[name] {
			schema = append(schema, name)
		}
	}
	reset := pgstore.ReadModelTables()
	sort.Strings(schema)
	sort.Strings(reset)
	if strings.Join(schema, ",") != strings.Join(reset, ",") {
		t.Errorf("read-model tables in the schema:\n %v\nreset list:\n %v", schema, reset)
	}
}

func TestResetReadModel_EmptiesRowsAndMarkers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	store, cleanup := setupReadModelStore(t)
	defer cleanup()
	ctx := context.Background()

	personID := uuid.New()
	if err := store.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{ID: personID, GivenName: "Ada", Surname: "Example", Version: 1, UpdatedAt: time.Now()}); err != nil {
		t.Fatalf("save person: %v", err)
	}
	repoID := uuid.New()
	if err := store.SaveRepository(ctx, &repository.RepositoryReadModel{ID: repoID, Name: "Archive", Version: 1, UpdatedAt: time.Now()}); err != nil {
		t.Fatalf("save repository: %v", err)
	}
	if err := store.ReplaceRepositoryExternalIDs(ctx, repoID, []repository.RepositoryExternalIDReadModel{{RepositoryID: repoID, Value: "R-1"}}); err != nil {
		t.Fatalf("save repository exid: %v", err)
	}
	branch := domain.BranchID(uuid.New())
	failedAt := time.Date(2026, 1, 2, 3, 4, 5, 6000, time.UTC) // PostgreSQL microsecond precision
	for i, f := range []repository.ProjectionFailure{
		{StreamID: personID, BranchID: domain.MainBranchID, Version: 2, EventType: "PersonUpdated", Error: "boom", FailedAt: failedAt},
		{StreamID: repoID, BranchID: branch, Version: 1, EventType: "RepositoryCreated", Error: "bang", FailedAt: failedAt},
	} {
		if err := store.RecordProjectionFailure(ctx, f); err != nil {
			t.Fatalf("record failure %d: %v", i, err)
		}
	}

	got, err := store.ListProjectionFailures(ctx, 1)
	if err != nil {
		t.Fatalf("list failures: %v", err)
	}
	if len(got) != 1 || got[0].StreamID != personID || !got[0].BranchID.IsMain() || got[0].Version != 2 ||
		got[0].EventType != "PersonUpdated" || got[0].Error != "boom" || !got[0].FailedAt.Equal(failedAt) {
		t.Fatalf("ListProjectionFailures(1) = %+v, want the person's marker", got)
	}
	if all, err := store.ListProjectionFailures(ctx, 10); err != nil || len(all) != 2 || all[1].BranchID != branch {
		t.Fatalf("ListProjectionFailures(10) = %+v, %v; want both, oldest first", all, err)
	}

	if err := store.ResetReadModel(ctx); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if p, err := store.GetPerson(ctx, domain.MainBranchID, personID); err != nil || p != nil {
		t.Errorf("person after reset = %v, %v; want gone", p, err)
	}
	if r, err := store.GetRepository(ctx, repoID); err != nil || r != nil {
		t.Errorf("repository after reset = %v, %v; want gone", r, err)
	}
	if left, err := store.ListProjectionFailures(ctx, 10); err != nil || len(left) != 0 {
		t.Errorf("failures after reset = %+v, %v; want none", left, err)
	}
}

// TestRebuildState_RoundTripAndOutlivesReset covers the state an unfinished
// rebuild leaves behind (#845): saved, replaced, kept by the reset, cleared.
func TestRebuildState_RoundTripAndOutlivesReset(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	db, cleanup := setupPostgres(t)
	defer cleanup()
	store, err := pgstore.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("read model store: %v", err)
	}
	if got, err := store.GetRebuildState(ctx); err != nil || got != nil {
		t.Fatalf("state before save = %q, %v; want none", got, err)
	}
	for _, state := range []string{`{"a":1}`, `{"b":2}`} {
		if err := store.SaveRebuildState(ctx, []byte(state)); err != nil {
			t.Fatalf("save %s: %v", state, err)
		}
	}
	if err := store.ResetReadModel(ctx); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if got, err := store.GetRebuildState(ctx); err != nil || string(got) != `{"b":2}` {
		t.Fatalf("state after reset = %q, %v; want the latest save", got, err)
	}
	if err := store.ClearRebuildState(ctx); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got, err := store.GetRebuildState(ctx); err != nil || got != nil {
		t.Fatalf("state after clear = %q, %v; want none", got, err)
	}
}
