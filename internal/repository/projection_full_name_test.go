package repository_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

// fullNameStores returns the read-model backends the full-name projection tests
// run against, so both stay in sync (DB-001).
func fullNameStores(t *testing.T) map[string]repository.ReadModelStore {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "myfamily-full-name-*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	db, err := sqlite.OpenDB(tmpFile.Name())
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	sqliteStore, err := sqlite.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}

	return map[string]repository.ReadModelStore{
		"memory": memory.NewReadModelStore(),
		"sqlite": sqliteStore,
	}
}

// TestProjector_FullNameHasNoStraySpace proves the projection never stores a
// full name with a leading or trailing space when a person has no surname or
// no given name, on create, on update and on merge.
func TestProjector_FullNameHasNoStraySpace(t *testing.T) {
	for backend, readStore := range fullNameStores(t) {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			projector := repository.NewProjector(readStore, nil)

			fullName := func(t *testing.T, id uuid.UUID) string {
				t.Helper()
				rm, err := readStore.GetPerson(ctx, domain.MainBranchID, id)
				if err != nil {
					t.Fatalf("GetPerson: %v", err)
				}
				if rm == nil {
					t.Fatal("person not found in read model")
				}
				return rm.FullName
			}
			project := func(t *testing.T, event domain.Event, version int64) {
				t.Helper()
				if err := projector.Project(ctx, event, version, domain.MainBranchID); err != nil {
					t.Fatalf("Project %s: %v", event.EventType(), err)
				}
			}

			t.Run("created", func(t *testing.T) {
				tests := []struct {
					name, givenName, surname, want string
				}{
					{"no surname", "Alpha", "", "Alpha"},
					{"no given name", "", "Placeholder", "Placeholder"},
					{"both names", "Alpha", "Placeholder", "Alpha Placeholder"},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						person := domain.NewPerson(tt.givenName, tt.surname)
						project(t, domain.NewPersonCreated(person), 1)
						if got := fullName(t, person.ID); got != tt.want {
							t.Errorf("FullName = %q, want %q", got, tt.want)
						}
					})
				}
			})

			t.Run("updated", func(t *testing.T) {
				tests := []struct {
					name    string
					changes map[string]any
					want    string
				}{
					{"surname cleared", map[string]any{"surname": ""}, "Beta"},
					{"given name cleared", map[string]any{"given_name": ""}, "Example"},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						person := domain.NewPerson("Beta", "Example")
						project(t, domain.NewPersonCreated(person), 1)
						project(t, domain.NewPersonUpdated(person.ID, tt.changes), 2)
						if got := fullName(t, person.ID); got != tt.want {
							t.Errorf("FullName = %q, want %q", got, tt.want)
						}
					})
				}
			})

			t.Run("merged", func(t *testing.T) {
				tests := []struct {
					name     string
					resolved map[string]any
					want     string
				}{
					{"surname resolved empty", map[string]any{"surname": ""}, "Gamma"},
					{"given name resolved empty", map[string]any{"given_name": ""}, "Sample"},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						survivor := domain.NewPerson("Gamma", "Sample")
						merged := domain.NewPerson("Delta", "Sample")
						project(t, domain.NewPersonCreated(survivor), 1)
						project(t, domain.NewPersonCreated(merged), 1)
						project(t, domain.NewPersonMerged(survivor.ID, merged.ID, map[string]any{}, tt.resolved,
							nil, nil, nil, nil, nil), 2)
						if got := fullName(t, survivor.ID); got != tt.want {
							t.Errorf("FullName = %q, want %q", got, tt.want)
						}
					})
				}
			})
		})
	}
}
