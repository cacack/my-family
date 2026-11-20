package sqlite_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/sqlite"
	"github.com/cacack/my-family/internal/testutil/settingstest"
)

func TestSQLiteSettingsStore(t *testing.T) {
	db := setupBranchTestDB(t)
	defer db.Close()

	store, err := sqlite.NewSettingsStore(db)
	if err != nil {
		t.Fatalf("NewSettingsStore() error = %v", err)
	}
	settingstest.Run(t, store)

	// Constructing again over the same database keeps what was saved.
	if _, err := sqlite.NewSettingsStore(db); err != nil {
		t.Fatalf("NewSettingsStore() again error = %v", err)
	}
}

func TestSQLiteSettingsStore_CorruptValue(t *testing.T) {
	db := setupBranchTestDB(t)
	defer db.Close()

	store, err := sqlite.NewSettingsStore(db)
	if err != nil {
		t.Fatalf("NewSettingsStore() error = %v", err)
	}
	if _, err := db.Exec(`INSERT INTO site_settings (key, value) VALUES ('home_person_id', 'not-a-uuid')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.HomePerson(t.Context(), repository.SiteWide); err == nil {
		t.Error("HomePerson() with a corrupt value: want error")
	}
	// A move skips the corrupt value and still moves the accounts it names.
	merged, survivor, account := uuid.New(), uuid.New(), uuid.New()
	if err := store.SetHomePerson(t.Context(), account, merged); err != nil {
		t.Fatal(err)
	}
	if err := store.MoveHomePersons(t.Context(), map[uuid.UUID]uuid.UUID{merged: survivor}); err != nil {
		t.Fatalf("MoveHomePersons() past a corrupt value: %v", err)
	}
	if got, _ := store.HomePerson(t.Context(), account); got != survivor {
		t.Errorf("HomePerson(account) after move = %v, want %v", got, survivor)
	}
}

func TestSQLiteSettingsStore_ClosedDB(t *testing.T) {
	db := setupBranchTestDB(t)
	store, err := sqlite.NewSettingsStore(db)
	if err != nil {
		t.Fatalf("NewSettingsStore() error = %v", err)
	}
	db.Close()
	for _, account := range []uuid.UUID{repository.SiteWide, uuid.New()} {
		if _, err := store.HomePerson(t.Context(), account); err == nil {
			t.Errorf("HomePerson(%v) on a closed database: want error", account)
		}
		if err := store.SetHomePerson(t.Context(), account, uuid.New()); err == nil {
			t.Errorf("SetHomePerson(%v) on a closed database: want error", account)
		}
		if err := store.SetHomePerson(t.Context(), account, uuid.Nil); err == nil {
			t.Errorf("SetHomePerson(%v, nil) on a closed database: want error", account)
		}
	}
	if err := store.MoveHomePersons(t.Context(), map[uuid.UUID]uuid.UUID{uuid.New(): uuid.New()}); err == nil {
		t.Error("MoveHomePersons() on a closed database: want error")
	}
	if _, err := sqlite.NewSettingsStore(db); err == nil {
		t.Error("NewSettingsStore() on a closed database: want error")
	}
}
