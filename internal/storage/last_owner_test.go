package storage

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/auth"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/testutil/pgtest"
)

func TestAccountStore_LastOwnerContract(t *testing.T) {
	for backend, open := range accountBackends() {
		t.Run(string(backend), func(t *testing.T) {
			stores := open(t)
			t.Cleanup(func() { _ = stores.Close() })
			runLastOwnerContract(t, stores.Accounts)
		})
	}
}

// runLastOwnerContract checks that the store refuses to demote or disable a
// tree's last active owner (#1432), changing nothing, and allows every other
// change of an owner.
func runLastOwnerContract(t *testing.T, store repository.AccountStore) {
	ctx := context.Background()
	now := at(time.Now())
	create := func(name string, role repository.Role) *repository.User {
		t.Helper()
		u := &repository.User{ID: uuid.New(), Username: name, DisplayName: name, PasswordHash: "h", CreatedAt: now}
		if err := store.CreateUser(ctx, u, repository.DefaultTreeID, role); err != nil {
			t.Fatalf("CreateUser(%s): %v", name, err)
		}
		return u
	}
	ada := create("ada", repository.RoleOwner)
	bob := create("bob", repository.RoleOwner)
	eve := create("eve", repository.RoleEditor)
	update := func(u *repository.User, name string, disabled bool) error {
		changed := *u
		changed.DisplayName, changed.Disabled = name, disabled
		return store.UpdateUser(ctx, &changed)
	}
	setRole := func(u *repository.User, role repository.Role) error {
		return store.SetMemberRole(ctx, repository.DefaultTreeID, u.ID, role)
	}
	mustOK := func(what string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	}
	wantLastOwner := func(what string, err error) {
		t.Helper()
		if !errors.Is(err, repository.ErrLastOwner) {
			t.Errorf("%s: err = %v, want ErrLastOwner", what, err)
		}
	}

	// A disabled owner does not count: with bob disabled, ada is the last.
	mustOK("disabling one of two owners", update(bob, "bob", true))
	wantLastOwner("demoting the last active owner", setRole(ada, repository.RoleEditor))
	wantLastOwner("disabling the last active owner", update(ada, "Ada changed", true))
	if u, err := store.GetUser(ctx, ada.ID); err != nil || u.Disabled || u.DisplayName != "ada" {
		t.Errorf("a refused UpdateUser changed the user: %+v, %v", u, err)
	}
	if role, err := store.MemberRole(ctx, repository.DefaultTreeID, ada.ID); err != nil || role != repository.RoleOwner {
		t.Errorf("a refused SetMemberRole changed the role: %q, %v", role, err)
	}

	// The last active owner may still be renamed, keep their role, and be
	// written as already enabled; a disabled owner may change in any way.
	mustOK("renaming the last active owner", update(ada, "Ada", false))
	mustOK("setting the last owner's role to owner", setRole(ada, repository.RoleOwner))
	mustOK("renaming a disabled owner", update(bob, "Bob", true))
	mustOK("demoting a disabled owner", setRole(bob, repository.RoleViewer))
	mustOK("promoting a disabled user", setRole(bob, repository.RoleOwner))
	mustOK("demoting a non-owner", setRole(eve, repository.RoleViewer))
	mustOK("disabling a non-owner", update(eve, "eve", true))

	// With bob enabled again either owner may step down, then the other is last.
	mustOK("re-enabling an owner", update(bob, "Bob", false))
	mustOK("demoting one of two active owners", setRole(ada, repository.RoleEditor))
	wantLastOwner("demoting the remaining owner", setRole(bob, repository.RoleViewer))
	wantLastOwner("disabling the remaining owner", update(bob, "Bob", true))

	// Missing rows still report as missing, not as the last owner.
	if err := store.SetMemberRole(ctx, repository.DefaultTreeID, uuid.New(), repository.RoleViewer); !errors.Is(err, repository.ErrNotMember) {
		t.Errorf("SetMemberRole of a non-member: err = %v, want ErrNotMember", err)
	}
	if err := store.SetMemberRole(ctx, uuid.New(), bob.ID, repository.RoleViewer); !errors.Is(err, repository.ErrNotMember) {
		t.Errorf("SetMemberRole in another tree: err = %v, want ErrNotMember", err)
	}
	if err := update(&repository.User{ID: uuid.New()}, "nobody", true); !errors.Is(err, repository.ErrUserNotFound) {
		t.Errorf("UpdateUser of an unknown user: err = %v, want ErrUserNotFound", err)
	}
}

// sharedDatabaseBackends opens two independent sets of stores on one fresh
// database of each SQL backend, as two processes sharing it would.
func sharedDatabaseBackends() map[Backend]func(t *testing.T) (*Stores, *Stores) {
	twice := func(t *testing.T, open func() (*Stores, error)) (*Stores, *Stores) {
		t.Helper()
		var pair [2]*Stores
		for i := range pair {
			stores, err := open()
			if err != nil {
				t.Fatalf("open stores %d: %v", i, err)
			}
			t.Cleanup(func() { _ = stores.Close() })
			pair[i] = stores
		}
		return pair[0], pair[1]
	}
	return map[Backend]func(t *testing.T) (*Stores, *Stores){
		BackendSQLite: func(t *testing.T) (*Stores, *Stores) {
			path := filepath.Join(t.TempDir(), "myfamily.db")
			return twice(t, func() (*Stores, error) { return OpenSQLite(path, AccessServe) })
		},
		BackendPostgres: func(t *testing.T) (*Stores, *Stores) {
			dsn := pgtest.NewDatabase(t)
			return twice(t, func() (*Stores, error) { return OpenPostgres(dsn, AccessServe) })
		},
	}
}

// TestLastOwner_TwoServicesRace has two auth services, each with its own
// connection to one database, remove each other's owner at the same time.
// Their in-process locks cannot see each other, so only the store keeps one
// active owner (#1432).
func TestLastOwner_TwoServicesRace(t *testing.T) {
	type change func(svc *auth.Service, id uuid.UUID) error
	demote := func(svc *auth.Service, id uuid.UUID) error {
		_, err := svc.SetRole(context.Background(), auth.CommandLine, id, repository.RoleEditor)
		return err
	}
	disable := func(svc *auth.Service, id uuid.UUID) error {
		_, err := svc.SetDisabled(context.Background(), auth.CommandLine, id, true)
		return err
	}
	modes := []struct {
		name string
		a, b change
	}{
		{"demote both", demote, demote},
		{"disable both", disable, disable},
		{"demote and disable", demote, disable},
	}
	const rounds = 10

	for backend, open := range sharedDatabaseBackends() {
		t.Run(string(backend), func(t *testing.T) {
			ctx := context.Background()
			storesA, storesB := open(t)
			svcA, svcB := auth.NewService(storesA.Accounts), auth.NewService(storesB.Accounts)
			now := at(time.Now())
			var owners [2]*repository.User
			for i, name := range []string{"ada", "bob"} {
				owners[i] = &repository.User{ID: uuid.New(), Username: name, DisplayName: name, PasswordHash: "h", CreatedAt: now}
				if err := storesA.Accounts.CreateUser(ctx, owners[i], repository.DefaultTreeID, repository.RoleOwner); err != nil {
					t.Fatalf("CreateUser(%s): %v", name, err)
				}
			}

			for _, mode := range modes {
				for round := range rounds {
					name := fmt.Sprintf("%s round %d", mode.name, round)
					// Both owners active again before each round.
					for _, u := range owners {
						if err := storesA.Accounts.SetMemberRole(ctx, repository.DefaultTreeID, u.ID, repository.RoleOwner); err != nil {
							t.Fatalf("%s: reset role: %v", name, err)
						}
						if err := storesA.Accounts.UpdateUser(ctx, u); err != nil {
							t.Fatalf("%s: re-enable: %v", name, err)
						}
					}

					start := make(chan struct{})
					var errA, errB error
					var wg sync.WaitGroup
					wg.Go(func() { <-start; errA = mode.a(svcA, owners[1].ID) })
					wg.Go(func() { <-start; errB = mode.b(svcB, owners[0].ID) })
					close(start)
					wg.Wait()

					refused := 0
					for _, err := range []error{errA, errB} {
						switch {
						case errors.Is(err, auth.ErrLastOwner):
							refused++
						case err != nil:
							t.Errorf("%s: unexpected error %v", name, err)
						}
					}
					if refused != 1 {
						t.Errorf("%s: %d changes refused (A: %v, B: %v), want exactly 1", name, refused, errA, errB)
					}
					members, err := storesB.Accounts.ListMembers(ctx, repository.DefaultTreeID)
					if err != nil {
						t.Fatalf("%s: ListMembers: %v", name, err)
					}
					active := 0
					for _, m := range members {
						if m.Role == repository.RoleOwner && !m.User.Disabled {
							active++
						}
					}
					if active != 1 {
						t.Fatalf("%s: %d active owners remain, want 1", name, active)
					}
				}
			}
		})
	}
}
