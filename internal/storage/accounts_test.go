package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/testutil/pgtest"
)

// accountBackends opens a fresh set of stores on each backend, so one account
// contract proves memory, SQLite and PostgreSQL alike (DB-001).
func accountBackends() map[Backend]func(t *testing.T) *Stores {
	return map[Backend]func(t *testing.T) *Stores{
		BackendMemory: func(*testing.T) *Stores { return OpenMemory() },
		BackendSQLite: func(t *testing.T) *Stores {
			stores, err := OpenSQLite(filepath.Join(t.TempDir(), "myfamily.db"), AccessServe)
			if err != nil {
				t.Fatalf("OpenSQLite: %v", err)
			}
			return stores
		},
		BackendPostgres: func(t *testing.T) *Stores {
			stores, err := OpenPostgres(pgtest.NewDatabase(t), AccessServe)
			if err != nil {
				t.Fatalf("OpenPostgres: %v", err)
			}
			return stores
		},
	}
}

func TestAccountStore_Contract(t *testing.T) {
	for backend, open := range accountBackends() {
		t.Run(string(backend), func(t *testing.T) {
			stores := open(t)
			t.Cleanup(func() { _ = stores.Close() })
			runAccountStoreContract(t, stores.Accounts)
		})
	}
}

// at returns a UTC time truncated to microseconds, the finest resolution every
// backend stores.
func at(t time.Time) time.Time { return t.UTC().Truncate(time.Microsecond) }

func runAccountStoreContract(t *testing.T, store repository.AccountStore) {
	ctx := context.Background()
	created := at(time.Now())
	owner := &repository.User{
		ID: uuid.New(), Username: "ada", DisplayName: "Ada Lovelace",
		PasswordHash: "$argon2id$hash", CreatedAt: created,
	}

	if n, err := store.CountMembers(ctx, repository.DefaultTreeID, repository.RoleOwner); err != nil || n != 0 {
		t.Fatalf("CountMembers on an empty store = %d, %v; want 0", n, err)
	}
	if err := store.CreateUser(ctx, owner, repository.DefaultTreeID, repository.RoleOwner); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if n, err := store.CountMembers(ctx, repository.DefaultTreeID, repository.RoleOwner); err != nil || n != 1 {
		t.Errorf("CountMembers = %d, %v; want 1", n, err)
	}
	if n, err := store.CountMembers(ctx, uuid.New(), repository.RoleOwner); err != nil || n != 0 {
		t.Errorf("CountMembers of another tree = %d, %v; want 0", n, err)
	}

	duplicate := *owner
	duplicate.ID = uuid.New()
	if err := store.CreateUser(ctx, &duplicate, repository.DefaultTreeID, repository.RoleOwner); !errors.Is(err, repository.ErrUsernameTaken) {
		t.Errorf("CreateUser with a taken username: err = %v, want ErrUsernameTaken", err)
	}
	if n, _ := store.CountMembers(ctx, repository.DefaultTreeID, repository.RoleOwner); n != 1 {
		t.Errorf("a refused CreateUser left a membership: CountMembers = %d, want 1", n)
	}

	for name, get := range map[string]func() (*repository.User, error){
		"GetUser":           func() (*repository.User, error) { return store.GetUser(ctx, owner.ID) },
		"GetUserByUsername": func() (*repository.User, error) { return store.GetUserByUsername(ctx, "ada") },
	} {
		got, err := get()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got.ID != owner.ID || got.Username != owner.Username || got.DisplayName != owner.DisplayName ||
			got.PasswordHash != owner.PasswordHash || !got.CreatedAt.Equal(created) {
			t.Errorf("%s = %+v, want %+v", name, got, owner)
		}
	}
	if _, err := store.GetUser(ctx, uuid.New()); !errors.Is(err, repository.ErrUserNotFound) {
		t.Errorf("GetUser of an unknown id: err = %v, want ErrUserNotFound", err)
	}
	if _, err := store.GetUserByUsername(ctx, "nobody"); !errors.Is(err, repository.ErrUserNotFound) {
		t.Errorf("GetUserByUsername of an unknown name: err = %v, want ErrUserNotFound", err)
	}

	unknown := uuid.New()
	names, err := store.DisplayNames(ctx, []uuid.UUID{owner.ID, unknown})
	if err != nil {
		t.Fatalf("DisplayNames: %v", err)
	}
	if len(names) != 1 || names[owner.ID] != "Ada Lovelace" {
		t.Errorf("DisplayNames = %v, want only %s: Ada Lovelace", names, owner.ID)
	}
	if names, err := store.DisplayNames(ctx, nil); err != nil || len(names) != 0 {
		t.Errorf("DisplayNames(nil) = %v, %v; want empty", names, err)
	}

	// Sessions.
	session := &repository.Session{TokenHash: "hash-1", UserID: owner.ID, CreatedAt: created, LastSeenAt: created}
	if err := store.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	got, err := store.GetSession(ctx, "hash-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.UserID != owner.ID || !got.CreatedAt.Equal(created) || !got.LastSeenAt.Equal(created) {
		t.Errorf("GetSession = %+v, want %+v", got, session)
	}
	later := created.Add(time.Hour)
	if err := store.TouchSession(ctx, "hash-1", later); err != nil {
		t.Fatalf("TouchSession: %v", err)
	}
	if got, _ := store.GetSession(ctx, "hash-1"); got == nil || !got.LastSeenAt.Equal(later) {
		t.Errorf("after TouchSession, LastSeenAt = %v, want %v", got, later)
	}
	if err := store.TouchSession(ctx, "missing", later); !errors.Is(err, repository.ErrSessionNotFound) {
		t.Errorf("TouchSession of a missing session: err = %v, want ErrSessionNotFound", err)
	}
	if _, err := store.GetSession(ctx, "missing"); !errors.Is(err, repository.ErrSessionNotFound) {
		t.Errorf("GetSession of a missing session: err = %v, want ErrSessionNotFound", err)
	}

	// Expiry: one idle too long, one too old, one current.
	idle := &repository.Session{TokenHash: "idle", UserID: owner.ID, CreatedAt: later, LastSeenAt: created}
	old := &repository.Session{TokenHash: "old", UserID: owner.ID, CreatedAt: created.Add(-time.Hour), LastSeenAt: later}
	for _, s := range []*repository.Session{idle, old} {
		if err := store.CreateSession(ctx, s); err != nil {
			t.Fatalf("CreateSession(%s): %v", s.TokenHash, err)
		}
	}
	removed, err := store.DeleteExpiredSessions(ctx, created.Add(time.Minute), created)
	if err != nil {
		t.Fatalf("DeleteExpiredSessions: %v", err)
	}
	if removed != 2 {
		t.Errorf("DeleteExpiredSessions removed %d, want 2", removed)
	}
	for hash, want := range map[string]bool{"hash-1": true, "idle": false, "old": false} {
		_, err := store.GetSession(ctx, hash)
		if exists := err == nil; exists != want {
			t.Errorf("after expiry, session %s exists = %v, want %v (err %v)", hash, exists, want, err)
		}
	}

	if err := store.DeleteSession(ctx, "hash-1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := store.GetSession(ctx, "hash-1"); !errors.Is(err, repository.ErrSessionNotFound) {
		t.Errorf("GetSession after DeleteSession: err = %v, want ErrSessionNotFound", err)
	}
	if err := store.DeleteSession(ctx, "hash-1"); err != nil {
		t.Errorf("DeleteSession twice: %v", err)
	}
}

// TestRebuild_KeepsAccounts proves `myfamily rebuild` leaves the account
// tables alone on each SQL backend: they are neither event log nor read model
// (ADR-007).
func TestRebuild_KeepsAccounts(t *testing.T) {
	backends := map[Backend]func(t *testing.T) func(Access) (*Stores, error){
		BackendSQLite: func(t *testing.T) func(Access) (*Stores, error) {
			path := filepath.Join(t.TempDir(), "myfamily.db")
			return func(access Access) (*Stores, error) { return OpenSQLite(path, access) }
		},
		BackendPostgres: func(t *testing.T) func(Access) (*Stores, error) {
			url := pgtest.NewDatabase(t)
			return func(access Access) (*Stores, error) { return OpenPostgres(url, access) }
		},
	}
	for backend, opener := range backends {
		t.Run(string(backend), func(t *testing.T) {
			open := opener(t)
			ctx := context.Background()

			stores, err := open(AccessServe)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			now := at(time.Now())
			user := &repository.User{ID: uuid.New(), Username: "ada", DisplayName: "Ada", PasswordHash: "h", CreatedAt: now}
			if err := stores.Accounts.CreateUser(ctx, user, repository.DefaultTreeID, repository.RoleOwner); err != nil {
				t.Fatalf("CreateUser: %v", err)
			}
			if err := stores.Accounts.CreateSession(ctx, &repository.Session{TokenHash: "s", UserID: user.ID, CreatedAt: now, LastSeenAt: now}); err != nil {
				t.Fatalf("CreateSession: %v", err)
			}
			if err := stores.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			rebuild, err := open(AccessRebuild)
			if err != nil {
				t.Fatalf("open for rebuild: %v", err)
			}
			defer func() { _ = rebuild.Close() }()
			if _, err := rebuild.RebuildReadModel(ctx); err != nil {
				t.Fatalf("RebuildReadModel: %v", err)
			}
			if _, err := rebuild.Accounts.GetUser(ctx, user.ID); err != nil {
				t.Errorf("user gone after rebuild: %v", err)
			}
			if n, err := rebuild.Accounts.CountMembers(ctx, repository.DefaultTreeID, repository.RoleOwner); err != nil || n != 1 {
				t.Errorf("owner membership after rebuild: CountMembers = %d, %v; want 1", n, err)
			}
			if _, err := rebuild.Accounts.GetSession(ctx, "s"); err != nil {
				t.Errorf("session gone after rebuild: %v", err)
			}
		})
	}
}
