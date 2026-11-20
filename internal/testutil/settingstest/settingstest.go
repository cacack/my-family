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

	get := func(account uuid.UUID) uuid.UUID {
		t.Helper()
		id, err := store.HomePerson(ctx, account)
		if err != nil {
			t.Fatalf("HomePerson(%v) error = %v", account, err)
		}
		return id
	}
	set := func(account, id uuid.UUID) {
		t.Helper()
		if err := store.SetHomePerson(ctx, account, id); err != nil {
			t.Fatalf("SetHomePerson(%v, %v) error = %v", account, id, err)
		}
	}
	move := func(survivors map[uuid.UUID]uuid.UUID) {
		t.Helper()
		if err := store.MoveHomePersons(ctx, survivors); err != nil {
			t.Fatalf("MoveHomePersons(%v) error = %v", survivors, err)
		}
	}

	ada, bob := uuid.New(), uuid.New()
	accounts := []uuid.UUID{repository.SiteWide, ada, bob}

	t.Run("each account starts unset, and sets and clears its own", func(t *testing.T) {
		for _, account := range accounts {
			if got := get(account); got != uuid.Nil {
				t.Errorf("new store HomePerson(%v) = %v, want unset", account, got)
			}
			set(account, uuid.Nil) // clearing when unset succeeds
		}
		first, second := uuid.New(), uuid.New()
		set(ada, first)
		if got := get(ada); got != first {
			t.Errorf("HomePerson(ada) = %v, want %v", got, first)
		}
		if got := get(bob); got != uuid.Nil {
			t.Errorf("HomePerson(bob) = %v after ada set hers, want unset", got)
		}
		if got := get(repository.SiteWide); got != uuid.Nil {
			t.Errorf("HomePerson(site) = %v after ada set hers, want unset", got)
		}
		set(repository.SiteWide, second)
		set(ada, second)
		if got := get(ada); got != second {
			t.Errorf("HomePerson(ada) after replace = %v, want %v", got, second)
		}
		set(ada, uuid.Nil)
		if got := get(ada); got != uuid.Nil {
			t.Errorf("HomePerson(ada) after clear = %v, want unset", got)
		}
		if got := get(repository.SiteWide); got != second {
			t.Errorf("HomePerson(site) = %v after ada cleared hers, want %v", got, second)
		}
		set(repository.SiteWide, uuid.Nil)
		if got := get(repository.SiteWide); got != uuid.Nil {
			t.Errorf("HomePerson(site) after clear = %v, want unset", got)
		}
	})

	t.Run("a move follows every account on the merged person", func(t *testing.T) {
		merged, survivor, other := uuid.New(), uuid.New(), uuid.New()
		set(repository.SiteWide, merged)
		set(ada, merged)
		set(bob, other)
		move(map[uuid.UUID]uuid.UUID{merged: survivor})
		want := map[uuid.UUID]uuid.UUID{repository.SiteWide: survivor, ada: survivor, bob: other}
		for _, account := range accounts {
			if got := get(account); got != want[account] {
				t.Errorf("HomePerson(%v) after move = %v, want %v", account, got, want[account])
			}
		}
		move(map[uuid.UUID]uuid.UUID{uuid.New(): survivor}) // a person no one chose changes nothing
		for _, account := range accounts {
			if got := get(account); got != want[account] {
				t.Errorf("HomePerson(%v) after no-op move = %v, want %v", account, got, want[account])
			}
		}
	})
	t.Run("a move looks each home person up once, not along a chain", func(t *testing.T) {
		a, b, c := uuid.New(), uuid.New(), uuid.New()
		set(repository.SiteWide, a)
		set(ada, b)
		set(bob, uuid.Nil)
		move(map[uuid.UUID]uuid.UUID{a: b, b: c})
		want := map[uuid.UUID]uuid.UUID{repository.SiteWide: b, ada: c, bob: uuid.Nil}
		for _, account := range accounts {
			if got := get(account); got != want[account] {
				t.Errorf("HomePerson(%v) after chained move = %v, want %v", account, got, want[account])
			}
		}
	})
}
