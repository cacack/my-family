package auth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

const testPassword = "correct horse battery"

// clock is a settable time source for expiry tests.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newTestService(t *testing.T) (*Service, *memory.AccountStore, *clock) {
	t.Helper()
	store := memory.NewAccountStore()
	svc := NewService(store)
	c := &clock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	svc.now = c.Now
	return svc, store, c
}

func TestHashPassword_RoundTrip(t *testing.T) {
	hash, err := HashPassword(testPassword)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Errorf("hash %q is not an argon2id PHC string with the expected parameters", hash)
	}
	if strings.Contains(hash, testPassword) {
		t.Error("hash contains the password")
	}
	if ok, err := VerifyPassword(testPassword, hash); err != nil || !ok {
		t.Errorf("VerifyPassword(right) = %v, %v; want true", ok, err)
	}
	if ok, err := VerifyPassword("wrong password", hash); err != nil || ok {
		t.Errorf("VerifyPassword(wrong) = %v, %v; want false", ok, err)
	}
	other, _ := HashPassword(testPassword)
	if other == hash {
		t.Error("two hashes of one password are equal: the salt is not random")
	}
}

func TestVerifyPassword_MalformedHash(t *testing.T) {
	for _, hash := range []string{
		"", "plain", "$argon2i$v=19$m=1,t=1,p=1$c2FsdA$a2V5",
		"$argon2id$v=18$m=1,t=1,p=1$c2FsdA$a2V5",
		"$argon2id$v=19$m=x$c2FsdA$a2V5",
		"$argon2id$v=19$m=1,t=0,p=1$c2FsdA$a2V5",
		"$argon2id$v=19$m=1,t=1,p=1$!!$a2V5",
		"$argon2id$v=19$m=1,t=1,p=1$c2FsdA$",
	} {
		if ok, err := VerifyPassword("x", hash); err == nil || ok {
			t.Errorf("VerifyPassword(%q) = %v, %v; want an error", hash, ok, err)
		}
	}
}

func TestCreateOwner_Validation(t *testing.T) {
	tests := []struct {
		name, username, displayName, password string
		wantErr                               bool
	}{
		{"valid", "ada", "Ada Lovelace", testPassword, false},
		{"username is normalized", "  Grace.H@Example ", "Grace", testPassword, false},
		{"empty username", "  ", "Ada", testPassword, true},
		{"username with a space", "ada l", "Ada", testPassword, true},
		{"username too long", strings.Repeat("a", 65), "Ada", testPassword, true},
		{"empty display name", "bob", " ", testPassword, true},
		{"display name too long", "bob", strings.Repeat("a", 101), testPassword, true},
		{"short password", "bob", "Bob", "short", true},
		{"long password", "bob", "Bob", strings.Repeat("p", 257), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store, _ := newTestService(t)
			user, err := svc.CreateOwner(context.Background(), Actor{}, tt.username, tt.displayName, tt.password)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("err = %v, want ErrInvalidInput", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateOwner: %v", err)
			}
			if user.Username != NormalizeUsername(tt.username) {
				t.Errorf("Username = %q, want %q", user.Username, NormalizeUsername(tt.username))
			}
			if user.PasswordHash == tt.password || !strings.HasPrefix(user.PasswordHash, "$argon2id$") {
				t.Errorf("PasswordHash = %q, want an argon2id hash", user.PasswordHash)
			}
			if n, _ := store.CountMembers(context.Background(), repository.DefaultTreeID, repository.RoleOwner); n != 1 {
				t.Errorf("owner memberships = %d, want 1", n)
			}
		})
	}
}

func TestCreateOwner_AuditsActor(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	user, err := svc.CreateOwner(ctx, CommandLine, "ada", "Ada", testPassword)
	if err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
	entries, _, _ := svc.AuthAudit(ctx, 10, 0)
	if len(entries) != 1 || entries[0].Action != repository.AuthActionUserCreated ||
		entries[0].ActorID != uuid.Nil || entries[0].ClientIP != CommandLine.ClientIP ||
		entries[0].SubjectID != user.ID || entries[0].Detail != string(repository.RoleOwner) {
		t.Errorf("audit = %+v, want the command line creating owner ada", entries)
	}
}

func TestCreateOwner_TakenUsername(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.CreateOwner(ctx, Actor{}, "ada", "Ada", testPassword); err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
	if _, err := svc.CreateOwner(ctx, Actor{}, "ADA", "Another Ada", testPassword); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("second CreateOwner of the same name: err = %v, want ErrInvalidInput", err)
	}
}

func TestSetup(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	required, err := svc.SetupRequired(ctx)
	if err != nil || !required {
		t.Fatalf("SetupRequired on a fresh store = %v, %v; want true", required, err)
	}
	if _, _, err := svc.CompleteSetup(ctx, "anything", "ada", "Ada", testPassword, "ip"); !errors.Is(err, ErrInvalidSetupToken) {
		t.Errorf("setup before a token was issued: err = %v, want ErrInvalidSetupToken", err)
	}
	token, err := svc.SetupToken(ctx)
	if err != nil || len(token) != 32 {
		t.Fatalf("SetupToken = %q, %v; want 32 hex characters", token, err)
	}
	if again, _ := svc.SetupToken(ctx); again != token {
		t.Error("SetupToken changed between calls")
	}
	if _, _, err := svc.CompleteSetup(ctx, "wrong", "ada", "Ada", testPassword, "ip"); !errors.Is(err, ErrInvalidSetupToken) {
		t.Errorf("setup with a wrong token: err = %v, want ErrInvalidSetupToken", err)
	}
	if _, _, err := svc.CompleteSetup(ctx, token, "ada", "Ada", "short", "ip"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("setup with a short password: err = %v, want ErrInvalidInput", err)
	}

	user, session, err := svc.CompleteSetup(ctx, " "+token+" ", "ada", "Ada", testPassword, "ip")
	if err != nil {
		t.Fatalf("CompleteSetup: %v", err)
	}
	if got, err := svc.Authenticate(ctx, session); err != nil || got.ID != user.ID {
		t.Errorf("Authenticate(setup session) = %v, %v; want the owner", got, err)
	}
	entries, _, _ := svc.AuthAudit(ctx, 10, 0)
	if len(entries) != 1 || entries[0].Action != repository.AuthActionUserCreated ||
		entries[0].ActorID != user.ID || entries[0].SubjectID != user.ID || entries[0].ClientIP != "ip" {
		t.Errorf("setup audit = %+v, want the new owner creating themselves from ip", entries)
	}
	if required, _ := svc.SetupRequired(ctx); required {
		t.Error("SetupRequired after setup = true")
	}
	if token, _ := svc.SetupToken(ctx); token != "" {
		t.Errorf("SetupToken after setup = %q, want empty", token)
	}
	if _, _, err := svc.CompleteSetup(ctx, token, "bob", "Bob", testPassword, "ip"); !errors.Is(err, ErrSetupDone) {
		t.Errorf("second setup: err = %v, want ErrSetupDone", err)
	}
}

// An owner created from the CLI against a running server ends its setup.
func TestSetupRequired_SeesOwnerCreatedElsewhere(t *testing.T) {
	svc, store, _ := newTestService(t)
	ctx := context.Background()
	if required, _ := svc.SetupRequired(ctx); !required {
		t.Fatal("SetupRequired on a fresh store = false")
	}
	if _, err := NewService(store).CreateOwner(ctx, Actor{}, "ada", "Ada", testPassword); err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
	if required, _ := svc.SetupRequired(ctx); required {
		t.Error("SetupRequired = true after another process created an owner")
	}
}

func TestSetup_RateLimited(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	token, _ := svc.SetupToken(ctx)
	for range maxFailuresPerClient {
		_, _, _ = svc.CompleteSetup(ctx, "wrong", "ada", "Ada", testPassword, "ip")
	}
	if _, _, err := svc.CompleteSetup(ctx, token, "ada", "Ada", testPassword, "ip"); !errors.Is(err, ErrRateLimited) {
		t.Errorf("setup after repeated wrong tokens: err = %v, want ErrRateLimited", err)
	}
}

func TestSignIn(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	owner, err := svc.CreateOwner(ctx, Actor{}, "ada", "Ada", testPassword)
	if err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}

	for _, tc := range []struct{ name, username, password string }{
		{"wrong password", "ada", "wrong password"},
		{"unknown user", "nobody", testPassword},
	} {
		if _, _, err := svc.SignIn(ctx, tc.username, tc.password, "ip"); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("%s: err = %v, want ErrInvalidCredentials", tc.name, err)
		}
	}

	user, token, err := svc.SignIn(ctx, " ADA ", testPassword, "ip")
	if err != nil {
		t.Fatalf("SignIn: %v", err)
	}
	if user.ID != owner.ID || token == "" {
		t.Fatalf("SignIn = %v, %q; want the owner and a token", user, token)
	}
	if got, err := svc.Authenticate(ctx, token); err != nil || got.ID != owner.ID {
		t.Errorf("Authenticate = %v, %v; want the owner", got, err)
	}
	if _, err := svc.Authenticate(ctx, token+"x"); !errors.Is(err, ErrNoSession) {
		t.Errorf("Authenticate(unknown token): err = %v, want ErrNoSession", err)
	}
	if _, err := svc.Authenticate(ctx, ""); !errors.Is(err, ErrNoSession) {
		t.Errorf("Authenticate(\"\"): err = %v, want ErrNoSession", err)
	}

	if err := svc.SignOut(ctx, token); err != nil {
		t.Fatalf("SignOut: %v", err)
	}
	if _, err := svc.Authenticate(ctx, token); !errors.Is(err, ErrNoSession) {
		t.Errorf("Authenticate after SignOut: err = %v, want ErrNoSession", err)
	}
	if err := svc.SignOut(ctx, token); err != nil {
		t.Errorf("second SignOut: %v", err)
	}
	if err := svc.SignOut(ctx, ""); err != nil {
		t.Errorf("SignOut(\"\"): %v", err)
	}
}

// The store keeps only a hash of the token.
func TestSignIn_StoresTokenHashed(t *testing.T) {
	svc, store, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.CreateOwner(ctx, Actor{}, "ada", "Ada", testPassword); err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
	_, token, err := svc.SignIn(ctx, "ada", testPassword, "ip")
	if err != nil {
		t.Fatalf("SignIn: %v", err)
	}
	if _, err := store.GetSession(ctx, token); !errors.Is(err, repository.ErrSessionNotFound) {
		t.Error("the session is stored under the raw token")
	}
	if _, err := store.GetSession(ctx, hashToken(token)); err != nil {
		t.Errorf("no session under the token's hash: %v", err)
	}
}

func TestSignIn_RateLimited(t *testing.T) {
	svc, _, c := newTestService(t)
	ctx := context.Background()
	if _, err := svc.CreateOwner(ctx, Actor{}, "ada", "Ada", testPassword); err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
	for range maxFailuresPerName {
		if _, _, err := svc.SignIn(ctx, "ada", "wrong password", "ip-1"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("failed sign-in: err = %v", err)
		}
	}
	if _, _, err := svc.SignIn(ctx, "ada", testPassword, "ip-2"); !errors.Is(err, ErrRateLimited) {
		t.Errorf("right password after %d failures: err = %v, want ErrRateLimited", maxFailuresPerName, err)
	}
	c.advance(limitWindow)
	if _, _, err := svc.SignIn(ctx, "ada", testPassword, "ip-2"); err != nil {
		t.Errorf("sign-in after the window: %v", err)
	}

	// One client guessing across many names is limited too.
	for i := range maxFailuresPerClient {
		_, _, _ = svc.SignIn(ctx, "user"+string(rune('a'+i)), "wrong password", "ip-3")
	}
	if _, _, err := svc.SignIn(ctx, "ada", testPassword, "ip-3"); !errors.Is(err, ErrRateLimited) {
		t.Errorf("sign-in from a client after %d failures: err = %v, want ErrRateLimited", maxFailuresPerClient, err)
	}
}

func TestSignIn_RateLimitHoldsUnderParallelAttempts(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.CreateOwner(ctx, Actor{}, "ada", "Ada", testPassword); err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
	const attempts = 4 * maxFailuresPerName
	var wg sync.WaitGroup
	errs := make(chan error, attempts)
	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := svc.SignIn(ctx, "ada", "wrong password", "ip-"+string(rune('a'+i)))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	checked := 0
	for err := range errs {
		if errors.Is(err, ErrInvalidCredentials) {
			checked++
		} else if !errors.Is(err, ErrRateLimited) {
			t.Errorf("parallel sign-in: err = %v", err)
		}
	}
	if checked != maxFailuresPerName {
		t.Errorf("%d parallel guesses were checked, want %d", checked, maxFailuresPerName)
	}
}

func TestSignIn_SuccessDoesNotCountAgainstClient(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.CreateOwner(ctx, Actor{}, "ada", "Ada", testPassword); err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
	for range maxFailuresPerClient + 1 {
		if _, _, err := svc.SignIn(ctx, "ada", testPassword, "ip-1"); err != nil {
			t.Fatalf("sign-in: %v", err)
		}
	}
}

func TestAuthenticate_Expiry(t *testing.T) {
	tests := []struct {
		name   string
		steps  []time.Duration // advance, then authenticate, for each
		expire bool            // whether the last authenticate fails
	}{
		{"active within the idle timeout", []time.Duration{IdleTimeout - time.Minute}, false},
		{"idle too long", []time.Duration{IdleTimeout}, true},
		{"activity keeps it alive", []time.Duration{IdleTimeout - time.Hour, IdleTimeout - time.Hour}, false},
		{"absolute timeout ends an active session", []time.Duration{6 * 24 * time.Hour, 6 * 24 * time.Hour, 6 * 24 * time.Hour, 6 * 24 * time.Hour, 6 * 24 * time.Hour}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store, c := newTestService(t)
			ctx := context.Background()
			if _, err := svc.CreateOwner(ctx, Actor{}, "ada", "Ada", testPassword); err != nil {
				t.Fatalf("CreateOwner: %v", err)
			}
			_, token, err := svc.SignIn(ctx, "ada", testPassword, "ip")
			if err != nil {
				t.Fatalf("SignIn: %v", err)
			}
			for i, step := range tt.steps {
				c.advance(step)
				_, err = svc.Authenticate(ctx, token)
				if i < len(tt.steps)-1 && err != nil {
					t.Fatalf("step %d: %v", i, err)
				}
			}
			if tt.expire {
				if !errors.Is(err, ErrNoSession) {
					t.Fatalf("err = %v, want ErrNoSession", err)
				}
				if _, err := store.GetSession(ctx, hashToken(token)); !errors.Is(err, repository.ErrSessionNotFound) {
					t.Error("an expired session was left in the store")
				}
			} else if err != nil {
				t.Fatalf("Authenticate: %v", err)
			}
		})
	}
}

func TestSignIn_DeletesExpiredSessions(t *testing.T) {
	svc, store, c := newTestService(t)
	ctx := context.Background()
	if _, err := svc.CreateOwner(ctx, Actor{}, "ada", "Ada", testPassword); err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
	_, stale, _ := svc.SignIn(ctx, "ada", testPassword, "ip")
	c.advance(IdleTimeout + time.Hour)
	if _, _, err := svc.SignIn(ctx, "ada", testPassword, "ip"); err != nil {
		t.Fatalf("SignIn: %v", err)
	}
	if _, err := store.GetSession(ctx, hashToken(stale)); !errors.Is(err, repository.ErrSessionNotFound) {
		t.Error("sign-in left an expired session in the store")
	}
}

func TestSignIn_DeletesExpiredLinks(t *testing.T) {
	svc, store, c := newTestService(t)
	ctx := context.Background()
	owner, err := svc.CreateOwner(ctx, Actor{}, "ada", "Ada", testPassword)
	if err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
	now := c.Now()
	for _, inv := range []*repository.Invite{
		{TokenHash: "inv-old", TreeID: repository.DefaultTreeID, Role: repository.RoleViewer, CreatedBy: owner.ID, CreatedAt: now, ExpiresAt: now.Add(-time.Minute)},
		{TokenHash: "inv-new", TreeID: repository.DefaultTreeID, Role: repository.RoleViewer, CreatedBy: owner.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
	} {
		if err := store.CreateInvite(ctx, inv); err != nil {
			t.Fatalf("CreateInvite: %v", err)
		}
	}
	for _, r := range []*repository.PasswordReset{
		{TokenHash: "r-old", UserID: owner.ID, CreatedAt: now, ExpiresAt: now.Add(-time.Minute)},
		{TokenHash: "r-new", UserID: owner.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
	} {
		if err := store.CreatePasswordReset(ctx, r); err != nil {
			t.Fatalf("CreatePasswordReset: %v", err)
		}
	}
	if _, _, err := svc.SignIn(ctx, "ada", testPassword, "ip"); err != nil {
		t.Fatalf("SignIn: %v", err)
	}
	// Listing from before either expiry shows every row still stored.
	before := now.Add(-time.Hour)
	invites, _ := store.ListInvites(ctx, repository.DefaultTreeID, before)
	if len(invites) != 1 || invites[0].TokenHash != "inv-new" {
		t.Errorf("invites after sign-in = %+v, want only inv-new", invites)
	}
	resets, _ := store.ListPasswordResets(ctx, before)
	if len(resets) != 1 || resets[0].TokenHash != "r-new" {
		t.Errorf("password resets after sign-in = %+v, want only r-new", resets)
	}
}

// failingPurge is a store whose expired links cannot be deleted.
type failingPurge struct {
	repository.AccountStore
}

func (failingPurge) DeleteExpiredLinks(context.Context, time.Time) (int, error) {
	return 0, errors.New("disk full")
}

func TestSignIn_PurgeFailureIsLoggedNotFatal(t *testing.T) {
	svc, store, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.CreateOwner(ctx, Actor{}, "ada", "Ada", testPassword); err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
	svc.store = failingPurge{store}
	var logged bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	if _, _, err := svc.SignIn(ctx, "ada", testPassword, "ip"); err != nil {
		t.Fatalf("SignIn with a failing purge: %v", err)
	}
	if got := logged.String(); !strings.Contains(got, "could not delete expired links") || !strings.Contains(got, "disk full") {
		t.Errorf("log = %q, want the purge failure", got)
	}
}

func TestSignIn_PrunesOldAuditEntries(t *testing.T) {
	const retention = 30 * 24 * time.Hour
	for _, tt := range []struct {
		name string
		opts []Option
		want []string
	}{
		{name: "with a retention", opts: []Option{WithAuditRetention(retention)}, want: []string{"recent"}},
		{name: "without one the log is kept", want: []string{"old", "recent"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc, store, c := newTestService(t)
			for _, opt := range tt.opts {
				opt(svc)
			}
			ctx := context.Background()
			if _, err := svc.CreateOwner(ctx, Actor{}, "ada", "Ada", testPassword); err != nil {
				t.Fatalf("CreateOwner: %v", err)
			}
			now := c.Now()
			for name, recorded := range map[string]time.Time{"old": now.Add(-retention - time.Hour), "recent": now.Add(-retention + time.Hour)} {
				if err := store.AppendAuthAudit(ctx, &repository.AuthAuditEntry{At: recorded,
					Action: repository.AuthActionSignInFailed, Username: name}); err != nil {
					t.Fatalf("AppendAuthAudit: %v", err)
				}
			}
			if _, _, err := svc.SignIn(ctx, "ada", testPassword, "ip"); err != nil {
				t.Fatalf("SignIn: %v", err)
			}
			entries, _, err := store.ListAuthAudit(ctx, 100, 0)
			if err != nil {
				t.Fatalf("ListAuthAudit: %v", err)
			}
			var failed []string
			for _, e := range entries {
				if e.Action == repository.AuthActionSignInFailed {
					failed = append(failed, e.Username)
				}
			}
			slices.Sort(failed)
			if !slices.Equal(failed, tt.want) {
				t.Errorf("failed sign-ins kept = %v, want %v", failed, tt.want)
			}
		})
	}
}

// failingAuditPrune is a store whose old audit entries cannot be deleted.
type failingAuditPrune struct {
	repository.AccountStore
}

func (failingAuditPrune) DeleteAuthAuditBefore(context.Context, time.Time) (int, error) {
	return 0, errors.New("disk full")
}

func TestSignIn_AuditPruneFailureIsLoggedNotFatal(t *testing.T) {
	svc, store, _ := newTestService(t)
	WithAuditRetention(time.Hour)(svc)
	ctx := context.Background()
	if _, err := svc.CreateOwner(ctx, Actor{}, "ada", "Ada", testPassword); err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
	svc.store = failingAuditPrune{store}
	var logged bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	if _, _, err := svc.SignIn(ctx, "ada", testPassword, "ip"); err != nil {
		t.Fatalf("SignIn with a failing audit prune: %v", err)
	}
	if got := logged.String(); !strings.Contains(got, "could not delete old audit entries") || !strings.Contains(got, "disk full") {
		t.Errorf("log = %q, want the prune failure", got)
	}
}

func TestAuthenticate_DeletedUser(t *testing.T) {
	svc, store, _ := newTestService(t)
	ctx := context.Background()
	if err := store.CreateSession(ctx, &repository.Session{
		TokenHash: hashToken("orphan"), UserID: uuid.New(), CreatedAt: svc.now(), LastSeenAt: svc.now(),
	}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, err := svc.Authenticate(ctx, "orphan"); !errors.Is(err, ErrNoSession) {
		t.Errorf("session of a missing user: err = %v, want ErrNoSession", err)
	}
}

func TestDisplayNames(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	owner, err := svc.CreateOwner(ctx, Actor{}, "ada", "Ada Lovelace", testPassword)
	if err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
	names, err := svc.DisplayNames(ctx, []uuid.UUID{owner.ID})
	if err != nil || names[owner.ID] != "Ada Lovelace" {
		t.Errorf("DisplayNames = %v, %v", names, err)
	}
}

// The first owner, whether created by setup or from the CLI, starts with the
// home person chosen for the site before accounts were enabled; later owners
// start with none (#1443).
func TestCreateOwner_FirstOwnerGetsSiteHomePerson(t *testing.T) {
	for _, viaSetup := range []bool{true, false} {
		t.Run(map[bool]string{true: "setup", false: "command line"}[viaSetup], func(t *testing.T) {
			ctx := context.Background()
			settings := memory.NewSettingsStore()
			site := uuid.New()
			if err := settings.SetHomePerson(ctx, repository.SiteWide, site); err != nil {
				t.Fatal(err)
			}
			svc := NewService(memory.NewAccountStore(), WithSettings(settings))

			var first *repository.User
			var err error
			if viaSetup {
				token, _ := svc.SetupToken(ctx)
				first, _, err = svc.CompleteSetup(ctx, token, "ada", "Ada", testPassword, "ip")
			} else {
				first, err = svc.CreateOwner(ctx, CommandLine, "ada", "Ada", testPassword)
			}
			if err != nil {
				t.Fatalf("creating the first owner: %v", err)
			}
			second, err := svc.CreateOwner(ctx, CommandLine, "bob", "Bob", testPassword)
			if err != nil {
				t.Fatalf("CreateOwner(bob): %v", err)
			}

			if got, _ := settings.HomePerson(ctx, first.ID); got != site {
				t.Errorf("first owner's home person = %v, want the site's %v", got, site)
			}
			if got, _ := settings.HomePerson(ctx, second.ID); got != uuid.Nil {
				t.Errorf("second owner's home person = %v, want none", got)
			}
			if got, _ := settings.HomePerson(ctx, repository.SiteWide); got != site {
				t.Errorf("site home person = %v, want it kept for open mode", got)
			}
		})
	}
}

// With no site home person, or no settings store, the first owner starts
// with none.
func TestCreateOwner_NoSiteHomePerson(t *testing.T) {
	ctx := context.Background()
	settings := memory.NewSettingsStore()
	user, err := NewService(memory.NewAccountStore(), WithSettings(settings)).CreateOwner(ctx, CommandLine, "ada", "Ada", testPassword)
	if err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
	if got, _ := settings.HomePerson(ctx, user.ID); got != uuid.Nil {
		t.Errorf("home person = %v, want none", got)
	}
	if _, err := NewService(memory.NewAccountStore()).CreateOwner(ctx, CommandLine, "ada", "Ada", testPassword); err != nil {
		t.Errorf("CreateOwner without a settings store: %v", err)
	}
}

// An owner set up before home persons were per account was never seeded; the
// carry-over gives the tree's only owner the site's home person unless they
// have chosen or cleared their own (#1474).
func TestCarryOverHomePerson(t *testing.T) {
	site, own := uuid.New(), uuid.New()
	tests := []struct {
		name     string
		noSite   bool
		owners   int
		editor   bool
		ownerSet func(ctx context.Context, s *memory.SettingsStore, owner uuid.UUID)
		want     uuid.UUID
	}{
		{name: "only owner with no choice gets the site's", owners: 1, want: site},
		{name: "other roles do not count as owners", owners: 1, editor: true, want: site},
		{name: "owner's own choice is kept", owners: 1, want: own,
			ownerSet: func(ctx context.Context, s *memory.SettingsStore, owner uuid.UUID) {
				_ = s.SetHomePerson(ctx, owner, own)
			}},
		{name: "owner's cleared choice stays cleared", owners: 1, want: uuid.Nil,
			ownerSet: func(ctx context.Context, s *memory.SettingsStore, owner uuid.UUID) {
				_ = s.SetHomePerson(ctx, owner, own)
				_ = s.SetHomePerson(ctx, owner, uuid.Nil)
			}},
		{name: "two owners get nothing", owners: 2, want: uuid.Nil},
		{name: "no site choice copies nothing", owners: 1, noSite: true, want: uuid.Nil},
		{name: "no owner is no error", owners: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			accounts, settings := memory.NewAccountStore(), memory.NewSettingsStore()
			// Owners are created as before #1443: without the settings store.
			bare := NewService(accounts)
			var owners []uuid.UUID
			for i := range tt.owners {
				u, err := bare.CreateOwner(ctx, CommandLine, fmt.Sprintf("owner%d", i), "Owner", testPassword)
				if err != nil {
					t.Fatal(err)
				}
				owners = append(owners, u.ID)
			}
			if tt.editor {
				u, err := bare.newUser("ed", "Ed", testPassword)
				if err != nil {
					t.Fatal(err)
				}
				if err := accounts.CreateUser(ctx, u, repository.DefaultTreeID, repository.RoleEditor); err != nil {
					t.Fatal(err)
				}
			}
			if !tt.noSite {
				_ = settings.SetHomePerson(ctx, repository.SiteWide, site)
			}
			if tt.ownerSet != nil {
				tt.ownerSet(ctx, settings, owners[0])
			}

			svc := NewService(accounts, WithSettings(settings))
			for range 2 { // a second run, as at the next startup, changes nothing
				if err := svc.CarryOverHomePerson(ctx); err != nil {
					t.Fatalf("CarryOverHomePerson: %v", err)
				}
			}
			for _, owner := range owners {
				if got, _ := settings.HomePerson(ctx, owner); got != tt.want {
					t.Errorf("owner's home person = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

// Without a settings store there is nothing to carry over; a failing one is
// reported.
func TestCarryOverHomePerson_Stores(t *testing.T) {
	ctx := context.Background()
	accounts := memory.NewAccountStore()
	if _, err := NewService(accounts).CreateOwner(ctx, CommandLine, "ada", "Ada", testPassword); err != nil {
		t.Fatal(err)
	}
	if err := NewService(accounts).CarryOverHomePerson(ctx); err != nil {
		t.Errorf("CarryOverHomePerson without a settings store: %v", err)
	}
	if err := NewService(accounts, WithSettings(brokenSettings{})).CarryOverHomePerson(ctx); err == nil {
		t.Error("CarryOverHomePerson with a failing settings store: want error")
	}
}

// brokenSettings fails every call.
type brokenSettings struct{}

func (brokenSettings) HomePerson(context.Context, uuid.UUID) (uuid.UUID, error) {
	return uuid.Nil, errors.New("settings table is locked")
}

func (brokenSettings) SetHomePerson(context.Context, uuid.UUID, uuid.UUID) error {
	return errors.New("settings table is locked")
}

func (brokenSettings) SeedHomePerson(context.Context, uuid.UUID) error {
	return errors.New("settings table is locked")
}

func (brokenSettings) MoveHomePersons(context.Context, map[uuid.UUID]uuid.UUID) error {
	return errors.New("settings table is locked")
}

// A settings failure does not undo or fail creating the first owner.
func TestCreateOwner_SettingsFailureStillCreatesOwner(t *testing.T) {
	ctx := context.Background()
	store := memory.NewAccountStore()
	if _, err := NewService(store, WithSettings(brokenSettings{})).CreateOwner(ctx, CommandLine, "ada", "Ada", testPassword); err != nil {
		t.Fatalf("CreateOwner with failing settings: %v", err)
	}
	if _, err := store.GetUserByUsername(ctx, "ada"); err != nil {
		t.Errorf("owner not created: %v", err)
	}
}
