package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/repository"
)

// APITokenPrefix starts every API token, so one pasted somewhere it should
// not be is recognizable as a My Family credential.
const APITokenPrefix = "mf_"

// maxAPITokenNameLen bounds an API token's name.
const maxAPITokenNameLen = 100

// Errors returned by the API token methods.
var (
	// ErrNoAPIToken is an unknown or revoked API token, or one whose user is
	// disabled.
	ErrNoAPIToken = errors.New("API token is unknown or revoked")
	// ErrAPITokenNotFound is a revoke of a token the user does not have.
	ErrAPITokenNotFound = errors.New("API token not found")
)

// NewAPIToken is an API token as created: its stored record and the token
// itself, which is shown this once. Only its hash is stored.
type NewAPIToken struct {
	repository.APIToken
	Token string
}

// CreateAPIToken issues a new API token for by.UserID, named name. A token
// acts as its user, with the role they hold at each request.
func (s *Service) CreateAPIToken(ctx context.Context, by Actor, name string) (*NewAPIToken, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxAPITokenNameLen {
		return nil, invalid("token name must be 1 to %d characters", maxAPITokenNameLen)
	}
	random, err := newToken()
	if err != nil {
		return nil, err
	}
	token := APITokenPrefix + random
	created := &NewAPIToken{
		APIToken: repository.APIToken{
			ID: uuid.New(), TokenHash: hashToken(token), UserID: by.UserID, Name: name, CreatedAt: s.now().UTC(),
		},
		Token: token,
	}
	if err := s.store.CreateAPIToken(ctx, &created.APIToken); err != nil {
		return nil, fmt.Errorf("create API token: %w", err)
	}
	s.audit(ctx, by, repository.AuthActionAPITokenCreated, s.userOrID(ctx, by.UserID), name)
	return created, nil
}

// APITokens returns userID's API tokens, newest first. None carries the
// token itself.
func (s *Service) APITokens(ctx context.Context, userID uuid.UUID) ([]repository.APIToken, error) {
	return s.store.ListAPITokens(ctx, userID)
}

// RevokeAPIToken removes by.UserID's API token with id, so its next request
// is refused. It returns ErrAPITokenNotFound when they have no such token.
func (s *Service) RevokeAPIToken(ctx context.Context, by Actor, id uuid.UUID) error {
	token, err := s.store.DeleteAPIToken(ctx, by.UserID, id)
	if errors.Is(err, repository.ErrAPITokenNotFound) {
		return ErrAPITokenNotFound
	}
	if err != nil {
		return fmt.Errorf("revoke API token: %w", err)
	}
	s.audit(ctx, by, repository.AuthActionAPITokenRevoked, s.userOrID(ctx, by.UserID), token.Name)
	return nil
}

// revokeAllAPITokens removes every API token of user, as by, auditing each.
func (s *Service) revokeAllAPITokens(ctx context.Context, by Actor, user *repository.User) error {
	tokens, err := s.store.ListAPITokens(ctx, user.ID)
	if err != nil {
		return fmt.Errorf("list API tokens: %w", err)
	}
	for _, token := range tokens {
		_, err := s.store.DeleteAPIToken(ctx, user.ID, token.ID)
		if errors.Is(err, repository.ErrAPITokenNotFound) {
			continue
		}
		if err != nil {
			return fmt.Errorf("revoke API token: %w", err)
		}
		s.audit(ctx, by, repository.AuthActionAPITokenRevoked, user, token.Name)
	}
	return nil
}

// AuthenticateAPIToken returns the user an API token belongs to, or
// ErrNoAPIToken when it is unknown, revoked or its user is disabled.
func (s *Service) AuthenticateAPIToken(ctx context.Context, token string) (*repository.User, error) {
	if !strings.HasPrefix(token, APITokenPrefix) {
		return nil, ErrNoAPIToken
	}
	tokenHash := hashToken(token)
	stored, err := s.store.GetAPIToken(ctx, tokenHash)
	if errors.Is(err, repository.ErrAPITokenNotFound) {
		return nil, ErrNoAPIToken
	}
	if err != nil {
		return nil, fmt.Errorf("authenticate API token: %w", err)
	}
	user, err := s.store.GetUser(ctx, stored.UserID)
	if errors.Is(err, repository.ErrUserNotFound) {
		return nil, ErrNoAPIToken
	}
	if err != nil {
		return nil, fmt.Errorf("authenticate API token: %w", err)
	}
	if user.Disabled {
		return nil, ErrNoAPIToken
	}
	now := s.now().UTC()
	if stored.LastUsedAt == nil || now.Sub(*stored.LastUsedAt) >= touchInterval {
		if err := s.store.TouchAPIToken(ctx, tokenHash, now); err != nil && !errors.Is(err, repository.ErrAPITokenNotFound) {
			slog.Warn("auth: could not record API token use", "error", err)
		}
	}
	return user, nil
}

// userOrID returns the user with id for an audit entry's subject, or a user
// with only the ID when it cannot be read: the action has already happened.
func (s *Service) userOrID(ctx context.Context, id uuid.UUID) *repository.User {
	user, err := s.store.GetUser(ctx, id)
	if err != nil {
		return &repository.User{ID: id}
	}
	return user
}
