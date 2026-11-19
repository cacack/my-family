// Package settingstest is the shared behaviour suite every
// repository.SettingsStore implementation runs, so the backends cannot drift.
package settingstest

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/repository"
)

// Run exercises store, which must start empty.
func Run(t *testing.T, store repository.SettingsStore) {
	t.Helper()
	ctx := context.Background()

	get := func() uuid.UUID {
		t.Helper()
		id, err := store.HomePerson(ctx)
		if err != nil {
			t.Fatalf("HomePerson() error = %v", err)
		}
		return id
	}
	set := func(id uuid.UUID) {
		t.Helper()
		if err := store.SetHomePerson(ctx, id); err != nil {
			t.Fatalf("SetHomePerson(%v) error = %v", id, err)
		}
	}

	if got := get(); got != uuid.Nil {
		t.Errorf("new store HomePerson() = %v, want unset", got)
	}
	set(uuid.Nil) // clearing when unset succeeds
	first, second := uuid.New(), uuid.New()
	set(first)
	if got := get(); got != first {
		t.Errorf("HomePerson() = %v, want %v", got, first)
	}
	set(second)
	if got := get(); got != second {
		t.Errorf("HomePerson() after replace = %v, want %v", got, second)
	}
	set(uuid.Nil)
	if got := get(); got != uuid.Nil {
		t.Errorf("HomePerson() after clear = %v, want unset", got)
	}
}
