package postgres_test

import (
	"testing"

	pgstore "github.com/cacack/my-family/internal/repository/postgres"
	"github.com/cacack/my-family/internal/testutil/settingstest"
)

func TestPostgresSettingsStore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	store, err := pgstore.NewSettingsStore(db)
	if err != nil {
		t.Fatalf("NewSettingsStore() error = %v", err)
	}
	settingstest.Run(t, store)
}
