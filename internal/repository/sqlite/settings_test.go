package sqlite_test

import (
	"testing"

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
	if _, err := store.HomePerson(t.Context()); err == nil {
		t.Error("HomePerson() with a corrupt value: want error")
	}
}

func TestSQLiteSettingsStore_ClosedDB(t *testing.T) {
	db := setupBranchTestDB(t)
	store, err := sqlite.NewSettingsStore(db)
	if err != nil {
		t.Fatalf("NewSettingsStore() error = %v", err)
	}
	db.Close()
	if _, err := store.HomePerson(t.Context()); err == nil {
		t.Error("HomePerson() on a closed database: want error")
	}
	if err := store.SetHomePerson(t.Context(), [16]byte{1}); err == nil {
		t.Error("SetHomePerson() on a closed database: want error")
	}
	if _, err := sqlite.NewSettingsStore(db); err == nil {
		t.Error("NewSettingsStore() on a closed database: want error")
	}
}
