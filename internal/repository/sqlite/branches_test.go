package sqlite_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

func setupBranchTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	return db
}

func TestSQLiteBranchStore_Create(t *testing.T) {
	db := setupBranchTestDB(t)
	defer db.Close()

	store, err := sqlite.NewBranchStore(db)
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

	if retrieved.ID != branch.ID {
		t.Errorf("ID = %v, want %v", retrieved.ID, branch.ID)
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

func TestSQLiteBranchStore_Create_NoDescription(t *testing.T) {
	db := setupBranchTestDB(t)
	defer db.Close()

	store, err := sqlite.NewBranchStore(db)
	if err != nil {
		t.Fatalf("NewBranchStore() error = %v", err)
	}

	ctx := context.Background()
	branch := &domain.Branch{
		ID:        uuid.New(),
		Name:      "Test Branch",
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
	if retrieved.Description != "" {
		t.Errorf("Description = %v, want empty string", retrieved.Description)
	}
}

func TestSQLiteBranchStore_Get_NotFound(t *testing.T) {
	db := setupBranchTestDB(t)
	defer db.Close()

	store, err := sqlite.NewBranchStore(db)
	if err != nil {
		t.Fatalf("NewBranchStore() error = %v", err)
	}

	ctx := context.Background()
	_, err = store.Get(ctx, uuid.New())
	if err != repository.ErrBranchNotFound {
		t.Errorf("Get() error = %v, want %v", err, repository.ErrBranchNotFound)
	}
}

func TestSQLiteBranchStore_Upsert(t *testing.T) {
	db := setupBranchTestDB(t)
	defer db.Close()

	store, err := sqlite.NewBranchStore(db)
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

	// Insert.
	if err := store.Upsert(ctx, branch); err != nil {
		t.Fatalf("Upsert() insert error = %v", err)
	}

	// Update (same ID) — idempotent replay.
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

func TestSQLiteBranchStore_List(t *testing.T) {
	db := setupBranchTestDB(t)
	defer db.Close()

	store, err := sqlite.NewBranchStore(db)
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

func TestSQLiteBranchStore_List_Empty(t *testing.T) {
	db := setupBranchTestDB(t)
	defer db.Close()

	store, err := sqlite.NewBranchStore(db)
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

func TestSQLiteBranchStore_Delete(t *testing.T) {
	db := setupBranchTestDB(t)
	defer db.Close()

	store, err := sqlite.NewBranchStore(db)
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

	_, err = store.Get(ctx, branch.ID)
	if err != repository.ErrBranchNotFound {
		t.Errorf("Get() after delete error = %v, want %v", err, repository.ErrBranchNotFound)
	}
}

func TestSQLiteBranchStore_Delete_NotFound(t *testing.T) {
	db := setupBranchTestDB(t)
	defer db.Close()

	store, err := sqlite.NewBranchStore(db)
	if err != nil {
		t.Fatalf("NewBranchStore() error = %v", err)
	}

	ctx := context.Background()
	err = store.Delete(ctx, uuid.New())
	if err != repository.ErrBranchNotFound {
		t.Errorf("Delete() error = %v, want %v", err, repository.ErrBranchNotFound)
	}
}

func TestSQLiteBranchStore_UpdateStatus(t *testing.T) {
	db := setupBranchTestDB(t)
	defer db.Close()

	store, err := sqlite.NewBranchStore(db)
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

func TestSQLiteBranchStore_UpdateStatus_NotFound(t *testing.T) {
	db := setupBranchTestDB(t)
	defer db.Close()

	store, err := sqlite.NewBranchStore(db)
	if err != nil {
		t.Fatalf("NewBranchStore() error = %v", err)
	}

	ctx := context.Background()
	err = store.UpdateStatus(ctx, uuid.New(), domain.BranchStatusMerged)
	if err != repository.ErrBranchNotFound {
		t.Errorf("UpdateStatus() error = %v, want %v", err, repository.ErrBranchNotFound)
	}
}
