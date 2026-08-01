package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	pgstore "github.com/cacack/my-family/internal/repository/postgres"
)

func TestPostgresBranchStore_Create(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	store, err := pgstore.NewBranchStore(db)
	if err != nil {
		t.Fatalf("NewBranchStore() error = %v", err)
	}

	ctx := context.Background()
	branch := &domain.Branch{
		ID:           uuid.New(),
		Name:         "Test Branch",
		Description:  "Test description",
		BasePosition: 42,
		Status:       domain.BranchStatusActive,
		CreatedAt:    time.Now().UTC(),
	}

	if err := store.Create(ctx, branch); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	retrieved, err := store.Get(ctx, branch.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if retrieved.Name != branch.Name {
		t.Errorf("Name = %v, want %v", retrieved.Name, branch.Name)
	}
	if retrieved.Description != branch.Description {
		t.Errorf("Description = %v, want %v", retrieved.Description, branch.Description)
	}
	if retrieved.BasePosition != branch.BasePosition {
		t.Errorf("BasePosition = %v, want %v", retrieved.BasePosition, branch.BasePosition)
	}
	if retrieved.Status != branch.Status {
		t.Errorf("Status = %v, want %v", retrieved.Status, branch.Status)
	}
}

func TestPostgresBranchStore_Get_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	store, err := pgstore.NewBranchStore(db)
	if err != nil {
		t.Fatalf("NewBranchStore() error = %v", err)
	}

	ctx := context.Background()
	if _, err := store.Get(ctx, uuid.New()); err != repository.ErrBranchNotFound {
		t.Errorf("Get() error = %v, want %v", err, repository.ErrBranchNotFound)
	}
}

func TestPostgresBranchStore_Upsert(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	store, err := pgstore.NewBranchStore(db)
	if err != nil {
		t.Fatalf("NewBranchStore() error = %v", err)
	}

	ctx := context.Background()
	branch := &domain.Branch{
		ID:           uuid.New(),
		Name:         "Original",
		BasePosition: 1,
		Status:       domain.BranchStatusActive,
		CreatedAt:    time.Now().UTC(),
	}

	if err := store.Upsert(ctx, branch); err != nil {
		t.Fatalf("Upsert() insert error = %v", err)
	}

	branch.Name = "Updated"
	branch.Status = domain.BranchStatusMerged
	if err := store.Upsert(ctx, branch); err != nil {
		t.Fatalf("Upsert() update error = %v", err)
	}

	retrieved, err := store.Get(ctx, branch.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if retrieved.Name != "Updated" {
		t.Errorf("Name = %v, want Updated", retrieved.Name)
	}
	if retrieved.Status != domain.BranchStatusMerged {
		t.Errorf("Status = %v, want %v", retrieved.Status, domain.BranchStatusMerged)
	}

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List() returned %d items, want 1", len(list))
	}
}

func TestPostgresBranchStore_List(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	store, err := pgstore.NewBranchStore(db)
	if err != nil {
		t.Fatalf("NewBranchStore() error = %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	branches := []*domain.Branch{
		{ID: uuid.New(), Name: "First", BasePosition: 1, Status: domain.BranchStatusActive, CreatedAt: now.Add(-2 * time.Hour)},
		{ID: uuid.New(), Name: "Second", BasePosition: 2, Status: domain.BranchStatusActive, CreatedAt: now.Add(-1 * time.Hour)},
		{ID: uuid.New(), Name: "Third", BasePosition: 3, Status: domain.BranchStatusActive, CreatedAt: now},
	}
	for _, b := range branches {
		if err := store.Create(ctx, b); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("List() returned %d items, want 3", len(list))
	}
	if list[0].Name != "Third" || list[1].Name != "Second" || list[2].Name != "First" {
		t.Errorf("List() order = [%v, %v, %v], want [Third, Second, First]", list[0].Name, list[1].Name, list[2].Name)
	}
}

func TestPostgresBranchStore_List_Empty(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	store, err := pgstore.NewBranchStore(db)
	if err != nil {
		t.Fatalf("NewBranchStore() error = %v", err)
	}

	ctx := context.Background()
	list, err := store.List(ctx)
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

func TestPostgresBranchStore_Delete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	store, err := pgstore.NewBranchStore(db)
	if err != nil {
		t.Fatalf("NewBranchStore() error = %v", err)
	}

	ctx := context.Background()
	branch := &domain.Branch{
		ID:        uuid.New(),
		Name:      "To Delete",
		Status:    domain.BranchStatusActive,
		CreatedAt: time.Now().UTC(),
	}
	if err := store.Create(ctx, branch); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := store.Delete(ctx, branch.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if _, err := store.Get(ctx, branch.ID); err != repository.ErrBranchNotFound {
		t.Errorf("Get() after delete error = %v, want %v", err, repository.ErrBranchNotFound)
	}
}

func TestPostgresBranchStore_Delete_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	store, err := pgstore.NewBranchStore(db)
	if err != nil {
		t.Fatalf("NewBranchStore() error = %v", err)
	}

	ctx := context.Background()
	if err := store.Delete(ctx, uuid.New()); err != repository.ErrBranchNotFound {
		t.Errorf("Delete() error = %v, want %v", err, repository.ErrBranchNotFound)
	}
}

func TestPostgresBranchStore_UpdateStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	store, err := pgstore.NewBranchStore(db)
	if err != nil {
		t.Fatalf("NewBranchStore() error = %v", err)
	}

	ctx := context.Background()
	branch := &domain.Branch{
		ID:        uuid.New(),
		Name:      "To Merge",
		Status:    domain.BranchStatusActive,
		CreatedAt: time.Now().UTC(),
	}
	if err := store.Create(ctx, branch); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := store.UpdateStatus(ctx, branch.ID, domain.BranchStatusMerged); err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}

	retrieved, err := store.Get(ctx, branch.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if retrieved.Status != domain.BranchStatusMerged {
		t.Errorf("Status = %v, want %v", retrieved.Status, domain.BranchStatusMerged)
	}
}

func TestPostgresBranchStore_UpdateStatus_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	store, err := pgstore.NewBranchStore(db)
	if err != nil {
		t.Fatalf("NewBranchStore() error = %v", err)
	}

	ctx := context.Background()
	if err := store.UpdateStatus(ctx, uuid.New(), domain.BranchStatusMerged); err != repository.ErrBranchNotFound {
		t.Errorf("UpdateStatus() error = %v, want %v", err, repository.ErrBranchNotFound)
	}
}

// TestPostgresBranchStore_MarkMerged mirrors the memory and SQLite assertions
// (DB-001 parity): the merge record is status, timestamp and note together.
func TestPostgresBranchStore_MarkMerged(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	store, err := pgstore.NewBranchStore(db)
	if err != nil {
		t.Fatalf("NewBranchStore() error = %v", err)
	}

	ctx := context.Background()
	branch := &domain.Branch{
		ID:        uuid.New(),
		Name:      "To Merge",
		Status:    domain.BranchStatusActive,
		CreatedAt: time.Now().UTC(),
	}
	if err := store.Create(ctx, branch); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	mergedAt := time.Now().UTC().Truncate(time.Millisecond)
	if err := store.MarkMerged(ctx, branch.ID, mergedAt, "verified against the 1880 census"); err != nil {
		t.Fatalf("MarkMerged() error = %v", err)
	}

	retrieved, err := store.Get(ctx, branch.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if retrieved.Status != domain.BranchStatusMerged {
		t.Errorf("Status = %v, want %v", retrieved.Status, domain.BranchStatusMerged)
	}
	if retrieved.MergedAt == nil {
		t.Fatal("MergedAt = nil, want the merge timestamp")
	}
	if !retrieved.MergedAt.Equal(mergedAt) {
		t.Errorf("MergedAt = %v, want %v", retrieved.MergedAt, mergedAt)
	}
	if retrieved.MergeNote != "verified against the 1880 census" {
		t.Errorf("MergeNote = %q, want the note", retrieved.MergeNote)
	}

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 || list[0].MergedAt == nil || !list[0].MergedAt.Equal(mergedAt) {
		t.Errorf("List() merge record = %+v, want MergedAt %v", list[0], mergedAt)
	}
}

func TestPostgresBranchStore_MarkMerged_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	store, err := pgstore.NewBranchStore(db)
	if err != nil {
		t.Fatalf("NewBranchStore() error = %v", err)
	}

	ctx := context.Background()
	if err := store.MarkMerged(ctx, uuid.New(), time.Now().UTC(), "note"); err != repository.ErrBranchNotFound {
		t.Errorf("MarkMerged() error = %v, want %v", err, repository.ErrBranchNotFound)
	}
}

// TestPostgresBranchStore_UnmergedHasNoMergeRecord pins the nullable round-trip:
// a NULL merged_at maps to a nil *time.Time, not the zero time.
func TestPostgresBranchStore_UnmergedHasNoMergeRecord(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	store, err := pgstore.NewBranchStore(db)
	if err != nil {
		t.Fatalf("NewBranchStore() error = %v", err)
	}

	ctx := context.Background()
	branch := &domain.Branch{
		ID:        uuid.New(),
		Name:      "Never Merged",
		Status:    domain.BranchStatusActive,
		CreatedAt: time.Now().UTC(),
	}
	if err := store.Create(ctx, branch); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	retrieved, err := store.Get(ctx, branch.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if retrieved.MergedAt != nil {
		t.Errorf("MergedAt = %v, want nil", retrieved.MergedAt)
	}
	if retrieved.MergeNote != "" {
		t.Errorf("MergeNote = %q, want empty", retrieved.MergeNote)
	}

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 || list[0].MergedAt != nil || list[0].MergeNote != "" {
		t.Errorf("List() merge fields = %+v, want nil/empty", list[0])
	}
}

// TestPostgresBranchStore_MigrationIsIdempotent covers the pre-#55 upgrade path:
// a branches table without the merge columns gains them on open, and opening
// again is a no-op (ADD COLUMN IF NOT EXISTS).
func TestPostgresBranchStore_MigrationIsIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	// Pre-#55 shape: CREATE TABLE IF NOT EXISTS will leave this alone.
	if _, err := db.Exec(`
		CREATE TABLE branches (
			id UUID PRIMARY KEY,
			name VARCHAR(100) NOT NULL,
			description VARCHAR(500),
			base_position BIGINT NOT NULL,
			status VARCHAR(20) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
		t.Fatalf("create pre-#55 branches table: %v", err)
	}

	existing := uuid.New()
	if _, err := db.Exec(`
		INSERT INTO branches (id, name, base_position, status, created_at)
		VALUES ($1, 'Pre-existing', 7, 'active', NOW())`, existing); err != nil {
		t.Fatalf("seed pre-#55 branch: %v", err)
	}

	var store *pgstore.BranchStore
	for i := 0; i < 2; i++ {
		var err error
		store, err = pgstore.NewBranchStore(db)
		if err != nil {
			t.Fatalf("NewBranchStore pass %d on pre-#55 database: %v", i, err)
		}
	}

	ctx := context.Background()
	got, err := store.Get(ctx, existing)
	if err != nil {
		t.Fatalf("Get pre-existing branch after migration: %v", err)
	}
	if got.BasePosition != 7 || got.MergedAt != nil || got.MergeNote != "" {
		t.Errorf("migrated branch = %+v, want base=7 and no merge record", got)
	}

	mergedAt := time.Now().UTC().Truncate(time.Millisecond)
	if err := store.MarkMerged(ctx, existing, mergedAt, "migrated then merged"); err != nil {
		t.Fatalf("MarkMerged after migration: %v", err)
	}
	if got, err = store.Get(ctx, existing); err != nil {
		t.Fatalf("Get after MarkMerged: %v", err)
	}
	if got.MergedAt == nil || !got.MergedAt.Equal(mergedAt) {
		t.Errorf("MergedAt = %v, want %v", got.MergedAt, mergedAt)
	}
}
