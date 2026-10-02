package sqlite_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

func setupSnapshotTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Create events table for GetMaxPosition
	_, err = db.Exec(`
		CREATE TABLE events (
			id TEXT PRIMARY KEY,
			stream_id TEXT NOT NULL,
			stream_type TEXT NOT NULL,
			event_type TEXT NOT NULL,
			data TEXT NOT NULL,
			version INTEGER NOT NULL,
			position INTEGER NOT NULL,
			timestamp TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create events table: %v", err)
	}

	return db
}

func TestSQLiteSnapshotStore_Create(t *testing.T) {
	db := setupSnapshotTestDB(t)
	defer db.Close()

	store, err := sqlite.NewSnapshotStore(db)
	if err != nil {
		t.Fatalf("NewSnapshotStore() error = %v", err)
	}

	ctx := context.Background()
	snapshot := &domain.Snapshot{
		ID:          uuid.New(),
		Name:        "Test Snapshot",
		Description: "Test description",
		Position:    42,
		CreatedAt:   time.Now().UTC(),
	}

	err = store.Create(ctx, snapshot)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Verify we can retrieve it
	retrieved, err := store.Get(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if retrieved.ID != snapshot.ID {
		t.Errorf("ID = %v, want %v", retrieved.ID, snapshot.ID)
	}
	if retrieved.Name != snapshot.Name {
		t.Errorf("Name = %v, want %v", retrieved.Name, snapshot.Name)
	}
	if retrieved.Description != snapshot.Description {
		t.Errorf("Description = %v, want %v", retrieved.Description, snapshot.Description)
	}
	if retrieved.Position != snapshot.Position {
		t.Errorf("Position = %v, want %v", retrieved.Position, snapshot.Position)
	}
}

func TestSQLiteSnapshotStore_Create_NoDescription(t *testing.T) {
	db := setupSnapshotTestDB(t)
	defer db.Close()

	store, err := sqlite.NewSnapshotStore(db)
	if err != nil {
		t.Fatalf("NewSnapshotStore() error = %v", err)
	}

	ctx := context.Background()
	snapshot := &domain.Snapshot{
		ID:        uuid.New(),
		Name:      "Test Snapshot",
		Position:  42,
		CreatedAt: time.Now().UTC(),
	}

	err = store.Create(ctx, snapshot)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Verify we can retrieve it with empty description
	retrieved, err := store.Get(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if retrieved.Description != "" {
		t.Errorf("Description = %v, want empty string", retrieved.Description)
	}
}

func TestSQLiteSnapshotStore_Get_NotFound(t *testing.T) {
	db := setupSnapshotTestDB(t)
	defer db.Close()

	store, err := sqlite.NewSnapshotStore(db)
	if err != nil {
		t.Fatalf("NewSnapshotStore() error = %v", err)
	}

	ctx := context.Background()
	_, err = store.Get(ctx, uuid.New())
	if err != repository.ErrSnapshotNotFound {
		t.Errorf("Get() error = %v, want %v", err, repository.ErrSnapshotNotFound)
	}
}

func TestSQLiteSnapshotStore_List(t *testing.T) {
	db := setupSnapshotTestDB(t)
	defer db.Close()

	store, err := sqlite.NewSnapshotStore(db)
	if err != nil {
		t.Fatalf("NewSnapshotStore() error = %v", err)
	}

	ctx := context.Background()

	// Create multiple snapshots with different times
	now := time.Now().UTC()
	snapshots := []*domain.Snapshot{
		{
			ID:        uuid.New(),
			Name:      "First",
			Position:  1,
			CreatedAt: now.Add(-2 * time.Hour),
		},
		{
			ID:        uuid.New(),
			Name:      "Second",
			Position:  2,
			CreatedAt: now.Add(-1 * time.Hour),
		},
		{
			ID:        uuid.New(),
			Name:      "Third",
			Position:  3,
			CreatedAt: now,
		},
	}

	for _, s := range snapshots {
		if err := store.Create(ctx, s); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	// List should return snapshots ordered by created_at DESC
	list, err := store.List(ctx, domain.MainBranchID)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(list) != 3 {
		t.Fatalf("List() returned %d items, want 3", len(list))
	}

	// Verify order (newest first)
	if list[0].Name != "Third" {
		t.Errorf("First item Name = %v, want Third", list[0].Name)
	}
	if list[1].Name != "Second" {
		t.Errorf("Second item Name = %v, want Second", list[1].Name)
	}
	if list[2].Name != "First" {
		t.Errorf("Third item Name = %v, want First", list[2].Name)
	}
}

func TestSQLiteSnapshotStore_List_Empty(t *testing.T) {
	db := setupSnapshotTestDB(t)
	defer db.Close()

	store, err := sqlite.NewSnapshotStore(db)
	if err != nil {
		t.Fatalf("NewSnapshotStore() error = %v", err)
	}

	ctx := context.Background()
	list, err := store.List(ctx, domain.MainBranchID)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if list == nil {
		t.Error("List() returned nil, want empty slice")
	}
	if len(list) != 0 {
		t.Errorf("List() returned %d items, want 0", len(list))
	}
}

func TestSQLiteSnapshotStore_Delete(t *testing.T) {
	db := setupSnapshotTestDB(t)
	defer db.Close()

	store, err := sqlite.NewSnapshotStore(db)
	if err != nil {
		t.Fatalf("NewSnapshotStore() error = %v", err)
	}

	ctx := context.Background()
	snapshot := &domain.Snapshot{
		ID:        uuid.New(),
		Name:      "To Delete",
		Position:  1,
		CreatedAt: time.Now().UTC(),
	}

	if err := store.Create(ctx, snapshot); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Delete the snapshot
	err = store.Delete(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify it's gone
	_, err = store.Get(ctx, snapshot.ID)
	if err != repository.ErrSnapshotNotFound {
		t.Errorf("Get() after delete error = %v, want %v", err, repository.ErrSnapshotNotFound)
	}
}

func TestSQLiteSnapshotStore_Delete_NotFound(t *testing.T) {
	db := setupSnapshotTestDB(t)
	defer db.Close()

	store, err := sqlite.NewSnapshotStore(db)
	if err != nil {
		t.Fatalf("NewSnapshotStore() error = %v", err)
	}

	ctx := context.Background()
	err = store.Delete(ctx, uuid.New())
	if err != repository.ErrSnapshotNotFound {
		t.Errorf("Delete() error = %v, want %v", err, repository.ErrSnapshotNotFound)
	}
}

func TestSQLiteSnapshotStore_GetMaxPosition(t *testing.T) {
	db := setupSnapshotTestDB(t)
	defer db.Close()

	store, err := sqlite.NewSnapshotStore(db)
	if err != nil {
		t.Fatalf("NewSnapshotStore() error = %v", err)
	}

	ctx := context.Background()

	// Initially should be 0
	pos, err := store.GetMaxPosition(ctx)
	if err != nil {
		t.Fatalf("GetMaxPosition() error = %v", err)
	}
	if pos != 0 {
		t.Errorf("GetMaxPosition() = %d, want 0", pos)
	}

	// Insert an event
	_, err = db.Exec(`
		INSERT INTO events (id, stream_id, stream_type, event_type, data, version, position, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, uuid.New().String(), uuid.New().String(), "test", "TestEvent", "{}", 1, 5, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("Insert event error = %v", err)
	}

	// Now max position should be 5
	pos, err = store.GetMaxPosition(ctx)
	if err != nil {
		t.Fatalf("GetMaxPosition() error = %v", err)
	}
	if pos != 5 {
		t.Errorf("GetMaxPosition() = %d, want 5", pos)
	}

	// Insert another event with higher position
	_, err = db.Exec(`
		INSERT INTO events (id, stream_id, stream_type, event_type, data, version, position, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, uuid.New().String(), uuid.New().String(), "test", "TestEvent", "{}", 2, 10, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("Insert event error = %v", err)
	}

	// Now max position should be 10
	pos, err = store.GetMaxPosition(ctx)
	if err != nil {
		t.Fatalf("GetMaxPosition() error = %v", err)
	}
	if pos != 10 {
		t.Errorf("GetMaxPosition() = %d, want 10", pos)
	}
}

// TestSQLiteSnapshotStore_Upsert covers the projection's write path (issue
// #624): inserting when absent, overwriting when present, so replaying
// SnapshotCreated is idempotent.
func TestSQLiteSnapshotStore_Upsert(t *testing.T) {
	db := setupSnapshotTestDB(t)
	defer db.Close()

	store, err := sqlite.NewSnapshotStore(db)
	if err != nil {
		t.Fatalf("NewSnapshotStore() error = %v", err)
	}
	ctx := context.Background()

	snapshot := &domain.Snapshot{
		ID:          uuid.New(),
		Name:        "Pre-DNA results",
		Description: "before",
		Position:    42,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
	}

	if err := store.Upsert(ctx, snapshot); err != nil {
		t.Fatalf("Upsert() insert error = %v", err)
	}

	retrieved, err := store.Get(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if retrieved.Name != snapshot.Name || retrieved.Position != snapshot.Position {
		t.Errorf("row = %+v, want %+v", retrieved, snapshot)
	}

	updated := *snapshot
	updated.Name = "After courthouse trip"
	updated.Description = ""
	updated.Position = 99
	if err := store.Upsert(ctx, &updated); err != nil {
		t.Fatalf("Upsert() overwrite error = %v", err)
	}

	retrieved, err = store.Get(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("Get() after overwrite error = %v", err)
	}
	if retrieved.Name != "After courthouse trip" || retrieved.Position != 99 {
		t.Errorf("row after overwrite = %+v, want the updated values", retrieved)
	}
	// A cleared description must overwrite as NULL, not linger from the insert.
	if retrieved.Description != "" {
		t.Errorf("description = %q, want it cleared by the overwrite", retrieved.Description)
	}

	all, err := store.List(ctx, domain.MainBranchID)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(all) != 1 {
		t.Errorf("List() returned %d snapshots, want 1", len(all))
	}
}

// TestSQLiteSnapshotStore_BranchScoped: a snapshot keeps its branch, and List
// answers one scope at a time (issue #839).
func TestSQLiteSnapshotStore_BranchScoped(t *testing.T) {
	db := setupSnapshotTestDB(t)
	defer db.Close()
	store, err := sqlite.NewSnapshotStore(db)
	if err != nil {
		t.Fatalf("NewSnapshotStore() error = %v", err)
	}
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	now := time.Now().UTC().Truncate(time.Microsecond)
	mainSnap := &domain.Snapshot{ID: uuid.New(), Name: "Mainline", Position: 5, CreatedAt: now}
	branchSnap := &domain.Snapshot{ID: uuid.New(), BranchID: branch, Name: "On branch", Position: 7, CreatedAt: now.Add(time.Second)}
	if err := store.Create(ctx, mainSnap); err != nil {
		t.Fatalf("Create(main) error = %v", err)
	}
	if err := store.Upsert(ctx, branchSnap); err != nil {
		t.Fatalf("Upsert(branch) error = %v", err)
	}

	got, err := store.Get(ctx, branchSnap.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.BranchID != branch || got.Position != 7 {
		t.Errorf("Get() = (%v, %d), want (%v, 7)", got.BranchID, got.Position, branch)
	}
	got, err = store.Get(ctx, mainSnap.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !got.BranchID.IsMain() {
		t.Errorf("mainline snapshot BranchID = %v, want main", got.BranchID)
	}

	for _, tc := range []struct {
		scope domain.BranchID
		want  uuid.UUID
	}{{domain.MainBranchID, mainSnap.ID}, {branch, branchSnap.ID}} {
		list, err := store.List(ctx, tc.scope)
		if err != nil {
			t.Fatalf("List(%v) error = %v", tc.scope, err)
		}
		if len(list) != 1 || list[0].ID != tc.want {
			t.Errorf("List(%v) = %v, want only %v", tc.scope, list, tc.want)
		}
	}
	if list, err := store.List(ctx, domain.BranchID(uuid.New())); err != nil || len(list) != 0 {
		t.Errorf("List(unknown branch) = %v, %v; want empty", list, err)
	}

	// An upsert replay keeps the branch.
	branchSnap.Name = "Renamed"
	if err := store.Upsert(ctx, branchSnap); err != nil {
		t.Fatalf("Upsert(replay) error = %v", err)
	}
	if got, err := store.Get(ctx, branchSnap.ID); err != nil || got.BranchID != branch || got.Name != "Renamed" {
		t.Errorf("after replay Get() = %+v, %v", got, err)
	}
}

// TestSQLiteSnapshotStore_MigratesBranchID: a snapshots table from before #839
// gains branch_id, and its existing rows become mainline snapshots. Opening the
// store again is a no-op.
func TestSQLiteSnapshotStore_MigratesBranchID(t *testing.T) {
	db := setupSnapshotTestDB(t)
	defer db.Close()
	ctx := context.Background()

	if _, err := db.Exec(`
		CREATE TABLE snapshots (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			position INTEGER NOT NULL,
			created_at TEXT NOT NULL
		)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	legacyID := uuid.New()
	if _, err := db.Exec(`INSERT INTO snapshots (id, name, position, created_at) VALUES (?, ?, ?, ?)`,
		legacyID.String(), "Legacy", 3, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}

	for range 2 {
		store, err := sqlite.NewSnapshotStore(db)
		if err != nil {
			t.Fatalf("NewSnapshotStore() error = %v", err)
		}
		list, err := store.List(ctx, domain.MainBranchID)
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(list) != 1 || list[0].ID != legacyID || !list[0].BranchID.IsMain() {
			t.Fatalf("List(main) = %+v, want the legacy row as a mainline snapshot", list)
		}
	}
}
