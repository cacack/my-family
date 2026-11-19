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

func TestAccountStore_ManagementContract(t *testing.T) {
	for backend, open := range accountBackends() {
		t.Run(string(backend), func(t *testing.T) {
			stores := open(t)
			t.Cleanup(func() { _ = stores.Close() })
			runAccountManagementContract(t, stores.Accounts)
		})
	}
}

// runAccountManagementContract covers members, invites, password resets and
// the auth audit log (#1410).
func runAccountManagementContract(t *testing.T, store repository.AccountStore) {
	ctx := context.Background()
	now := at(time.Now())
	owner := &repository.User{ID: uuid.New(), Username: "ada", DisplayName: "Ada", PasswordHash: "h1", CreatedAt: now}
	if err := store.CreateUser(ctx, owner, repository.DefaultTreeID, repository.RoleOwner); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Invites: an expired one and an unknown one are refused; a valid one
	// creates the user with its role, once.
	invite := &repository.Invite{TokenHash: "inv", TreeID: repository.DefaultTreeID, Role: repository.RoleEditor,
		CreatedBy: owner.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	expired := &repository.Invite{TokenHash: "old", TreeID: repository.DefaultTreeID, Role: repository.RoleOwner,
		CreatedBy: owner.ID, CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour)}
	for _, inv := range []*repository.Invite{invite, expired} {
		if err := store.CreateInvite(ctx, inv); err != nil {
			t.Fatalf("CreateInvite(%s): %v", inv.TokenHash, err)
		}
	}
	newUser := func(name string) *repository.User {
		return &repository.User{ID: uuid.New(), Username: name, DisplayName: name, PasswordHash: "h", CreatedAt: now}
	}
	for _, hash := range []string{"old", "missing"} {
		if _, err := store.AcceptInvite(ctx, hash, now, newUser("x-"+hash)); !errors.Is(err, repository.ErrLinkNotFound) {
			t.Errorf("AcceptInvite(%s): err = %v, want ErrLinkNotFound", hash, err)
		}
	}
	if _, err := store.AcceptInvite(ctx, "inv", now, newUser("ada")); !errors.Is(err, repository.ErrUsernameTaken) {
		t.Errorf("AcceptInvite with a taken username: err = %v, want ErrUsernameTaken", err)
	}
	grace := newUser("grace")
	got, err := store.AcceptInvite(ctx, "inv", now, grace)
	if err != nil {
		t.Fatalf("AcceptInvite: %v (a refused username must leave the invite usable)", err)
	}
	if got.Role != repository.RoleEditor || got.CreatedBy != owner.ID || got.TreeID != repository.DefaultTreeID || !got.ExpiresAt.Equal(invite.ExpiresAt) {
		t.Errorf("AcceptInvite = %+v, want %+v", got, invite)
	}
	if role, err := store.MemberRole(ctx, repository.DefaultTreeID, grace.ID); err != nil || role != repository.RoleEditor {
		t.Errorf("invited member's role = %q, %v; want editor", role, err)
	}
	if _, err := store.AcceptInvite(ctx, "inv", now, newUser("again")); !errors.Is(err, repository.ErrLinkNotFound) {
		t.Errorf("AcceptInvite twice: err = %v, want ErrLinkNotFound", err)
	}

	// Members, roles and the disabled flag.
	members, err := store.ListMembers(ctx, repository.DefaultTreeID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 2 || members[0].User.ID != owner.ID || members[0].Role != repository.RoleOwner ||
		members[1].User.ID != grace.ID || members[1].Role != repository.RoleEditor || !members[0].User.CreatedAt.Equal(now) {
		t.Errorf("ListMembers = %+v, want ada (owner) then grace (editor)", members)
	}
	if members, err := store.ListMembers(ctx, uuid.New()); err != nil || len(members) != 0 {
		t.Errorf("ListMembers of another tree = %v, %v; want none", members, err)
	}
	if err := store.SetMemberRole(ctx, repository.DefaultTreeID, grace.ID, repository.RoleViewer); err != nil {
		t.Fatalf("SetMemberRole: %v", err)
	}
	if role, _ := store.MemberRole(ctx, repository.DefaultTreeID, grace.ID); role != repository.RoleViewer {
		t.Errorf("role after SetMemberRole = %q, want viewer", role)
	}
	if err := store.SetMemberRole(ctx, repository.DefaultTreeID, uuid.New(), repository.RoleViewer); !errors.Is(err, repository.ErrNotMember) {
		t.Errorf("SetMemberRole of a non-member: err = %v, want ErrNotMember", err)
	}
	changed := *grace
	changed.DisplayName, changed.PasswordHash, changed.Disabled = "Grace H", "h2", true
	if err := store.UpdateUser(ctx, &changed); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if u, err := store.GetUser(ctx, grace.ID); err != nil || u.DisplayName != "Grace H" || u.PasswordHash != "h2" || !u.Disabled || u.Username != "grace" {
		t.Errorf("after UpdateUser, GetUser = %+v, %v", u, err)
	}
	if members, _ := store.ListMembers(ctx, repository.DefaultTreeID); len(members) != 2 || !members[1].User.Disabled {
		t.Errorf("ListMembers after disabling = %+v, want grace disabled", members)
	}
	missing := newUser("nobody")
	if err := store.UpdateUser(ctx, missing); !errors.Is(err, repository.ErrUserNotFound) {
		t.Errorf("UpdateUser of an unknown user: err = %v, want ErrUserNotFound", err)
	}

	// Sessions of one user, keeping one.
	for _, hash := range []string{"g1", "g2", "g3"} {
		if err := store.CreateSession(ctx, &repository.Session{TokenHash: hash, UserID: grace.ID, CreatedAt: now, LastSeenAt: now}); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
	}
	if err := store.CreateSession(ctx, &repository.Session{TokenHash: "a1", UserID: owner.ID, CreatedAt: now, LastSeenAt: now}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := store.DeleteUserSessions(ctx, grace.ID, "g2"); err != nil {
		t.Fatalf("DeleteUserSessions: %v", err)
	}
	for hash, want := range map[string]bool{"g1": false, "g2": true, "g3": false, "a1": true} {
		if _, err := store.GetSession(ctx, hash); (err == nil) != want {
			t.Errorf("after DeleteUserSessions, session %s exists = %v, want %v", hash, err == nil, want)
		}
	}

	// Password resets: expired and unknown refused; a valid one sets the
	// password, ends every session and uses up every reset of the user.
	for _, r := range []*repository.PasswordReset{
		{TokenHash: "r1", UserID: grace.ID, CreatedBy: owner.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		{TokenHash: "r2", UserID: grace.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		{TokenHash: "r-old", UserID: grace.ID, CreatedAt: now, ExpiresAt: now.Add(-time.Second)},
	} {
		if err := store.CreatePasswordReset(ctx, r); err != nil {
			t.Fatalf("CreatePasswordReset(%s): %v", r.TokenHash, err)
		}
	}
	for _, hash := range []string{"r-old", "missing"} {
		if _, err := store.UsePasswordReset(ctx, hash, now, "bad"); !errors.Is(err, repository.ErrLinkNotFound) {
			t.Errorf("UsePasswordReset(%s): err = %v, want ErrLinkNotFound", hash, err)
		}
	}
	userID, err := store.UsePasswordReset(ctx, "r1", now, "h3")
	if err != nil || userID != grace.ID {
		t.Fatalf("UsePasswordReset = %v, %v; want %v", userID, err, grace.ID)
	}
	if u, _ := store.GetUser(ctx, grace.ID); u == nil || u.PasswordHash != "h3" {
		t.Errorf("password hash after reset = %+v, want h3", u)
	}
	if _, err := store.GetSession(ctx, "g2"); !errors.Is(err, repository.ErrSessionNotFound) {
		t.Errorf("a reset left a session: err = %v", err)
	}
	if _, err := store.GetSession(ctx, "a1"); err != nil {
		t.Errorf("a reset ended another user's session: %v", err)
	}
	if _, err := store.UsePasswordReset(ctx, "r2", now, "h4"); !errors.Is(err, repository.ErrLinkNotFound) {
		t.Errorf("a second reset of the same user: err = %v, want ErrLinkNotFound", err)
	}

	// The audit log: newest first, paged, with absent IDs kept absent.
	if entries, total, err := store.ListAuthAudit(ctx, 10, 0); err != nil || total != 0 || len(entries) != 0 {
		t.Errorf("ListAuthAudit on an empty log = %v, %d, %v", entries, total, err)
	}
	appended := []repository.AuthAuditEntry{
		{At: now, Action: repository.AuthActionSignInFailed, Username: "eve", ClientIP: "192.0.2.1"},
		{At: now.Add(time.Second), Action: repository.AuthActionSignIn, ActorID: owner.ID, SubjectID: owner.ID, Username: "ada", ClientIP: "192.0.2.2"},
		{At: now.Add(2 * time.Second), Action: repository.AuthActionRoleChanged, ActorID: owner.ID, SubjectID: grace.ID, Username: "grace", Detail: "editor -> viewer"},
	}
	for i := range appended {
		if err := store.AppendAuthAudit(ctx, &appended[i]); err != nil {
			t.Fatalf("AppendAuthAudit: %v", err)
		}
	}
	entries, total, err := store.ListAuthAudit(ctx, 2, 0)
	if err != nil || total != 3 || len(entries) != 2 {
		t.Fatalf("ListAuthAudit(2, 0) = %d entries, total %d, %v; want 2 of 3", len(entries), total, err)
	}
	if e := entries[0]; e.Action != repository.AuthActionRoleChanged || e.ActorID != owner.ID || e.SubjectID != grace.ID ||
		e.Username != "grace" || e.Detail != "editor -> viewer" || !e.At.Equal(appended[2].At) {
		t.Errorf("newest entry = %+v, want %+v", e, appended[2])
	}
	if entries[1].Action != repository.AuthActionSignIn || entries[1].ClientIP != "192.0.2.2" {
		t.Errorf("second entry = %+v, want the sign-in", entries[1])
	}
	entries, _, err = store.ListAuthAudit(ctx, 2, 2)
	if err != nil || len(entries) != 1 {
		t.Fatalf("ListAuthAudit(2, 2) = %v, %v; want the oldest entry", entries, err)
	}
	if e := entries[0]; e.Action != repository.AuthActionSignInFailed || e.ActorID != uuid.Nil || e.SubjectID != uuid.Nil || e.Username != "eve" {
		t.Errorf("oldest entry = %+v, want the failed sign-in with no actor or subject", e)
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
	viewer := &repository.User{
		ID: uuid.New(), Username: "grace", DisplayName: "Grace Hopper",
		PasswordHash: "$argon2id$hash", CreatedAt: created,
	}
	if err := store.CreateUser(ctx, viewer, repository.DefaultTreeID, repository.RoleViewer); err != nil {
		t.Fatalf("CreateUser of a viewer: %v", err)
	}
	for _, tc := range []struct {
		name   string
		treeID uuid.UUID
		userID uuid.UUID
		want   repository.Role
		err    error
	}{
		{"owner", repository.DefaultTreeID, owner.ID, repository.RoleOwner, nil},
		{"viewer", repository.DefaultTreeID, viewer.ID, repository.RoleViewer, nil},
		{"unknown user", repository.DefaultTreeID, uuid.New(), "", repository.ErrNotMember},
		{"another tree", uuid.New(), owner.ID, "", repository.ErrNotMember},
	} {
		if role, err := store.MemberRole(ctx, tc.treeID, tc.userID); role != tc.want || !errors.Is(err, tc.err) {
			t.Errorf("MemberRole of %s = %q, %v; want %q, %v", tc.name, role, err, tc.want, tc.err)
		}
	}
	if n, err := store.CountMembers(ctx, repository.DefaultTreeID, repository.RoleOwner); err != nil || n != 1 {
		t.Errorf("CountMembers of owners with a viewer present = %d, %v; want 1", n, err)
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
