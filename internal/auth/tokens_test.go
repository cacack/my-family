package auth

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestAPIToken_Lifecycle(t *testing.T) {
	m := newManaged(t)
	ctx := context.Background()

	for _, name := range []string{"", "  ", strings.Repeat("é", maxAPITokenNameLen+1)} {
		if _, err := m.svc.CreateAPIToken(ctx, m.by, name); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("CreateAPIToken(%q): err = %v, want ErrInvalidInput", name, err)
		}
	}
	created, err := m.svc.CreateAPIToken(ctx, m.by, " backup ")
	if err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}
	if created.Name != "backup" || created.UserID != m.owner.ID || !strings.HasPrefix(created.Token, APITokenPrefix) {
		t.Errorf("created = %+v, want the trimmed name, the owner and a prefixed token", created.APIToken)
	}
	if created.TokenHash != hashToken(created.Token) || strings.Contains(created.TokenHash, created.Token) {
		t.Error("the stored record is not the token's hash")
	}

	user, err := m.svc.AuthenticateAPIToken(ctx, created.Token)
	if err != nil || user.ID != m.owner.ID {
		t.Fatalf("AuthenticateAPIToken = %v, %v; want the owner", user, err)
	}
	tokens, err := m.svc.APITokens(ctx, m.owner.ID)
	if err != nil || len(tokens) != 1 || tokens[0].LastUsedAt == nil || !tokens[0].LastUsedAt.Equal(m.clock.Now()) {
		t.Fatalf("APITokens after use = %+v, %v; want one, last used now", tokens, err)
	}

	// Use within touchInterval writes nothing; use after it does.
	first := *tokens[0].LastUsedAt
	m.clock.advance(touchInterval / 2)
	if _, err := m.svc.AuthenticateAPIToken(ctx, created.Token); err != nil {
		t.Fatal(err)
	}
	if tokens, _ := m.svc.APITokens(ctx, m.owner.ID); !tokens[0].LastUsedAt.Equal(first) {
		t.Errorf("a use within %v moved last used to %v", touchInterval, tokens[0].LastUsedAt)
	}
	m.clock.advance(touchInterval)
	if _, err := m.svc.AuthenticateAPIToken(ctx, created.Token); err != nil {
		t.Fatal(err)
	}
	if tokens, _ := m.svc.APITokens(ctx, m.owner.ID); !tokens[0].LastUsedAt.Equal(m.clock.Now()) {
		t.Errorf("last used = %v, want %v", tokens[0].LastUsedAt, m.clock.Now())
	}

	for name, token := range map[string]string{
		"unknown": APITokenPrefix + "nope", "unprefixed": strings.TrimPrefix(created.Token, APITokenPrefix), "empty": "",
	} {
		if _, err := m.svc.AuthenticateAPIToken(ctx, token); !errors.Is(err, ErrNoAPIToken) {
			t.Errorf("AuthenticateAPIToken(%s): err = %v, want ErrNoAPIToken", name, err)
		}
	}

	other := Actor{UserID: uuid.New()}
	if err := m.svc.RevokeAPIToken(ctx, other, created.ID); !errors.Is(err, ErrAPITokenNotFound) {
		t.Errorf("RevokeAPIToken by another user: err = %v, want ErrAPITokenNotFound", err)
	}
	if err := m.svc.RevokeAPIToken(ctx, m.by, created.ID); err != nil {
		t.Fatalf("RevokeAPIToken: %v", err)
	}
	if _, err := m.svc.AuthenticateAPIToken(ctx, created.Token); !errors.Is(err, ErrNoAPIToken) {
		t.Errorf("AuthenticateAPIToken after revoke: err = %v, want ErrNoAPIToken", err)
	}
	actions := m.actions(t)
	if !slices.Contains(actions, repository.AuthActionAPITokenCreated) || !slices.Contains(actions, repository.AuthActionAPITokenRevoked) {
		t.Errorf("audit actions = %v, want token created and revoked", actions)
	}
}

func TestAPIToken_RefusedForADisabledOrMissingUser(t *testing.T) {
	m := newManaged(t)
	ctx := context.Background()
	grace, _ := m.invite(t, "grace", repository.RoleEditor)
	created, err := m.svc.CreateAPIToken(ctx, Actor{UserID: grace.ID}, "script")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.svc.SetDisabled(ctx, m.by, grace.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.svc.AuthenticateAPIToken(ctx, created.Token); !errors.Is(err, ErrNoAPIToken) {
		t.Errorf("token of a disabled user: err = %v, want ErrNoAPIToken", err)
	}

	// A token whose user row is gone authenticates no one.
	store := memory.NewAccountStore()
	svc := NewService(store)
	if err := store.CreateAPIToken(ctx, &repository.APIToken{
		ID: uuid.New(), TokenHash: hashToken(APITokenPrefix + "orphan"), UserID: uuid.New(), Name: "orphan", CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AuthenticateAPIToken(ctx, APITokenPrefix+"orphan"); !errors.Is(err, ErrNoAPIToken) {
		t.Errorf("token of a missing user: err = %v, want ErrNoAPIToken", err)
	}
}

// failingTokens is a store whose API token reads and writes fail.
type failingTokens struct {
	repository.AccountStore
}

var errDiskFull = errors.New("disk full")

func (failingTokens) CreateAPIToken(context.Context, *repository.APIToken) error { return errDiskFull }
func (failingTokens) GetAPIToken(context.Context, string) (*repository.APIToken, error) {
	return nil, errDiskFull
}
func (failingTokens) DeleteAPIToken(context.Context, uuid.UUID, uuid.UUID) (*repository.APIToken, error) {
	return nil, errDiskFull
}

func TestAPIToken_StoreFailures(t *testing.T) {
	svc := NewService(failingTokens{memory.NewAccountStore()})
	ctx := context.Background()
	if _, err := svc.CreateAPIToken(ctx, Actor{UserID: uuid.New()}, "x"); !errors.Is(err, errDiskFull) {
		t.Errorf("CreateAPIToken: err = %v, want the store's error", err)
	}
	if _, err := svc.AuthenticateAPIToken(ctx, APITokenPrefix+"x"); !errors.Is(err, errDiskFull) || errors.Is(err, ErrNoAPIToken) {
		t.Errorf("AuthenticateAPIToken: err = %v, want the store's error", err)
	}
	if err := svc.RevokeAPIToken(ctx, Actor{UserID: uuid.New()}, uuid.New()); !errors.Is(err, errDiskFull) || errors.Is(err, ErrAPITokenNotFound) {
		t.Errorf("RevokeAPIToken: err = %v, want the store's error", err)
	}
}
