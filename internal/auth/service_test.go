package auth

import (
	"context"
	"errors"
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
			user, err := svc.CreateOwner(context.Background(), tt.username, tt.displayName, tt.password)
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

func TestCreateOwner_TakenUsername(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.CreateOwner(ctx, "ada", "Ada", testPassword); err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
	if _, err := svc.CreateOwner(ctx, "ADA", "Another Ada", testPassword); !errors.Is(err, ErrInvalidInput) {
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
	if _, err := NewService(store).CreateOwner(ctx, "ada", "Ada", testPassword); err != nil {
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
	owner, err := svc.CreateOwner(ctx, "ada", "Ada", testPassword)
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
	if _, err := svc.CreateOwner(ctx, "ada", "Ada", testPassword); err != nil {
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
	if _, err := svc.CreateOwner(ctx, "ada", "Ada", testPassword); err != nil {
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
			if _, err := svc.CreateOwner(ctx, "ada", "Ada", testPassword); err != nil {
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
	if _, err := svc.CreateOwner(ctx, "ada", "Ada", testPassword); err != nil {
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
	owner, err := svc.CreateOwner(ctx, "ada", "Ada Lovelace", testPassword)
	if err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
	names, err := svc.DisplayNames(ctx, []uuid.UUID{owner.ID})
	if err != nil || names[owner.ID] != "Ada Lovelace" {
		t.Errorf("DisplayNames = %v, %v", names, err)
	}
}
