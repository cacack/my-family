package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/cacack/my-family/internal/auth"
	"github.com/cacack/my-family/internal/repository"
)

// signedInUser is the user making a request to one of their own account's
// operations. authenticate refuses such a request without one before the
// handler runs.
func signedInUser(ctx context.Context) (*repository.User, error) {
	user := userFrom(ctx)
	if user == nil {
		return nil, echo.NewHTTPError(http.StatusUnauthorized, "Sign in to continue")
	}
	return user, nil
}

func apiTokenOf(token *repository.APIToken) APIToken {
	return APIToken{Id: token.ID, Name: token.Name, CreatedAt: token.CreatedAt, LastUsedAt: token.LastUsedAt}
}

// ListAPITokens implements StrictServerInterface.
func (ss *StrictServer) ListAPITokens(ctx context.Context, _ ListAPITokensRequestObject) (ListAPITokensResponseObject, error) {
	if ss.server.config.OpenMode() {
		return ListAPITokens409JSONResponse{accountsOnly}, nil
	}
	user, err := signedInUser(ctx)
	if err != nil {
		return nil, err
	}
	tokens, err := ss.server.auth.APITokens(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	items := make([]APIToken, len(tokens))
	for i := range tokens {
		items[i] = apiTokenOf(&tokens[i])
	}
	return ListAPITokens200JSONResponse{Items: items}, nil
}

// CreateAPIToken implements StrictServerInterface.
func (ss *StrictServer) CreateAPIToken(ctx context.Context, request CreateAPITokenRequestObject) (CreateAPITokenResponseObject, error) {
	if ss.server.config.OpenMode() {
		return CreateAPIToken409JSONResponse{accountsOnly}, nil
	}
	if _, err := signedInUser(ctx); err != nil {
		return nil, err
	}
	created, err := ss.server.auth.CreateAPIToken(ctx, actorFrom(ctx), request.Body.Name)
	switch {
	case err == nil:
		return CreateAPIToken201JSONResponse{
			Id: created.ID, Name: created.Name, CreatedAt: created.CreatedAt, Token: created.Token,
		}, nil
	case errors.Is(err, auth.ErrInvalidInput):
		return CreateAPIToken400JSONResponse{BadRequestJSONResponse(authError(CodeValidation, err.Error()))}, nil
	default:
		return nil, err
	}
}

// RevokeAPIToken implements StrictServerInterface.
func (ss *StrictServer) RevokeAPIToken(ctx context.Context, request RevokeAPITokenRequestObject) (RevokeAPITokenResponseObject, error) {
	if ss.server.config.OpenMode() {
		return RevokeAPIToken409JSONResponse{accountsOnly}, nil
	}
	if _, err := signedInUser(ctx); err != nil {
		return nil, err
	}
	err := ss.server.auth.RevokeAPIToken(ctx, actorFrom(ctx), request.TokenId)
	switch {
	case err == nil:
		return RevokeAPIToken204Response{}, nil
	case errors.Is(err, auth.ErrAPITokenNotFound):
		return RevokeAPIToken404JSONResponse{NotFoundJSONResponse(authError(CodeNotFound, "You have no such API token"))}, nil
	default:
		return nil, err
	}
}
