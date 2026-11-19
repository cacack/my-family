package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/cacack/my-family/internal/auth"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// sessionCookieName is the cookie a signed-in browser carries.
const sessionCookieName = "myfamily_session"

// Error codes of the sign-in layer.
const (
	CodeUnauthenticated    = "UNAUTHENTICATED"
	CodeSetupRequired      = "SETUP_REQUIRED"
	CodeForbidden          = "FORBIDDEN"
	CodeInvalidCredentials = "INVALID_CREDENTIALS" // #nosec G101 -- an error code, not a credential.
	CodeInvalidSetupToken  = "INVALID_SETUP_TOKEN"
	CodeRateLimited        = "RATE_LIMITED"
)

// publicAPIPaths are the API routes served without a session, compared with
// the path the router matches (echo.GetPath). Everything else under /api/
// needs one, so a new route is closed until someone opens it here.
var publicAPIPaths = map[string]bool{
	"/api/v1/health":        true,
	"/api/v1/auth/status":   true,
	"/api/v1/auth/setup":    true,
	"/api/v1/auth/sign-in":  true,
	"/api/v1/auth/sign-out": true,
}

// WithAuth supplies the sign-in service. The server requires it unless the
// config is in open mode (ADR-007).
func WithAuth(svc *auth.Service) ServerOption {
	return func(s *Server) {
		s.auth = svc
	}
}

// requestInfo is what the auth handlers need from the HTTP request besides
// its body. The strict handlers see only a context, so authenticate puts it
// there.
type requestInfo struct {
	clientIP     string
	secure       bool
	sessionToken string
}

type requestInfoKey struct{}

type userKey struct{}

func requestInfoFrom(ctx context.Context) requestInfo {
	info, _ := ctx.Value(requestInfoKey{}).(requestInfo)
	return info
}

// userFrom returns the signed-in user, or nil.
func userFrom(ctx context.Context) *repository.User {
	user, _ := ctx.Value(userKey{}).(*repository.User)
	return user
}

// isSecure reports whether the client reached the server over TLS, directly
// or through a proxy that says so. Trusting X-Forwarded-Proto here only ever
// adds Secure to a cookie, which a client lying about it denies itself.
func isSecure(req *http.Request) bool {
	return req.TLS != nil || strings.EqualFold(req.Header.Get("X-Forwarded-Proto"), "https")
}

func refuse(c echo.Context, status int, code, message string) error {
	return c.JSON(status, APIError{Code: code, Message: message})
}

// authenticate resolves who is making each request (ADR-007). In open mode
// that is the owner, with no actor recorded. Otherwise the session cookie is
// the only source of the user: a valid one puts the user on the request
// context, and as the actor its changes are attributed to
// (domain.ContextWithActor). An API request outside publicAPIPaths is then
// refused with 503 while the tree has no owner and 401 without a session.
// Non-API paths (the web UI's files) are always served: the UI shows the
// setup or sign-in page itself.
func (s *Server) authenticate(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		req := c.Request()
		info := requestInfo{clientIP: c.RealIP(), secure: isSecure(req)}
		path := echo.GetPath(req)
		ctx := req.Context()
		if s.config.OpenMode() || !strings.HasPrefix(path, "/api/") {
			c.SetRequest(req.WithContext(context.WithValue(ctx, requestInfoKey{}, info)))
			return next(c)
		}

		if cookie, err := req.Cookie(sessionCookieName); err == nil {
			info.sessionToken = cookie.Value
		}
		ctx = context.WithValue(ctx, requestInfoKey{}, info)
		if info.sessionToken != "" {
			user, err := s.auth.Authenticate(ctx, info.sessionToken)
			switch {
			case err == nil:
				ctx = context.WithValue(ctx, userKey{}, user)
				ctx = domain.ContextWithActor(ctx, user.ID.String())
			case !errors.Is(err, auth.ErrNoSession):
				return err
			}
		}
		c.SetRequest(req.WithContext(ctx))
		if publicAPIPaths[path] {
			return next(c)
		}

		required, err := s.auth.SetupRequired(ctx)
		if err != nil {
			return err
		}
		if required {
			return refuse(c, http.StatusServiceUnavailable, CodeSetupRequired,
				"This server has no owner yet. Create one on the setup page or with `myfamily user create`.")
		}
		if userFrom(ctx) == nil {
			return refuse(c, http.StatusUnauthorized, CodeUnauthenticated, "Sign in to continue")
		}
		return next(c)
	}
}

// refuseCrossOrigin refuses a state-changing request a browser sent from
// another site: its Origin header must name this server's host (or the host a
// proxy forwarded, which a cross-site page cannot set). A request without an
// Origin is not from a cross-site page and passes. Together with
// SameSite=Lax cookies this is the CSRF defense; there is no CORS, so no
// other origin can read a response either.
func refuseCrossOrigin(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		req := c.Request()
		switch req.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			return next(c)
		}
		origin := req.Header.Get(echo.HeaderOrigin)
		if origin == "" {
			return next(c)
		}
		u, err := url.Parse(origin)
		if err == nil && u.Host != "" {
			if strings.EqualFold(u.Host, req.Host) {
				return next(c)
			}
			if forwarded := req.Header.Get("X-Forwarded-Host"); forwarded != "" && strings.EqualFold(u.Host, forwarded) {
				return next(c)
			}
		}
		return refuse(c, http.StatusForbidden, CodeForbidden, "Cross-origin request refused")
	}
}

// sessionCookie is the Set-Cookie value that stores token, or clears the
// cookie when token is empty.
func sessionCookie(token string, secure bool) string {
	// #nosec G124 -- Secure is set whenever the client connected over TLS;
	// requiring it on plain HTTP would make sign-in impossible there.
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(auth.AbsoluteTimeout.Seconds()),
	}
	if token == "" {
		cookie.MaxAge = -1
	}
	return cookie.String()
}

func accountOf(user *repository.User) Account {
	return Account{
		Id:          user.ID,
		Username:    user.Username,
		DisplayName: user.DisplayName,
	}
}

func authError(code, message string) Error {
	return Error{Code: code, Message: message}
}

const openModeMessage = "This server runs with AUTH_MODE=open: no one signs in"

// GetAuthStatus implements StrictServerInterface.
func (ss *StrictServer) GetAuthStatus(ctx context.Context, _ GetAuthStatusRequestObject) (GetAuthStatusResponseObject, error) {
	if ss.server.config.OpenMode() {
		return GetAuthStatus200JSONResponse{Mode: AuthStatusModeOpen}, nil
	}
	required, err := ss.server.auth.SetupRequired(ctx)
	if err != nil {
		return nil, err
	}
	status := GetAuthStatus200JSONResponse{Mode: AuthStatusModeAccounts, SetupRequired: required}
	if user := userFrom(ctx); user != nil {
		account := accountOf(user)
		status.Account = &account
	}
	return status, nil
}

// SetupOwner implements StrictServerInterface.
func (ss *StrictServer) SetupOwner(ctx context.Context, request SetupOwnerRequestObject) (SetupOwnerResponseObject, error) {
	if ss.server.config.OpenMode() {
		return SetupOwner409JSONResponse(authError(CodeConflict, openModeMessage)), nil
	}
	body := request.Body
	info := requestInfoFrom(ctx)
	user, token, err := ss.server.auth.CompleteSetup(ctx, body.Token, body.Username, body.DisplayName, body.Password, info.clientIP)
	switch {
	case err == nil:
		cookie := sessionCookie(token, info.secure)
		return SetupOwner201JSONResponse{Body: accountOf(user), Headers: SetupOwner201ResponseHeaders{SetCookie: &cookie}}, nil
	case errors.Is(err, auth.ErrInvalidInput):
		return SetupOwner400JSONResponse{BadRequestJSONResponse(authError(CodeValidation, err.Error()))}, nil
	case errors.Is(err, auth.ErrInvalidSetupToken):
		return SetupOwner403JSONResponse(authError(CodeInvalidSetupToken, "The setup token is wrong. Copy it from the server log.")), nil
	case errors.Is(err, auth.ErrSetupDone):
		return SetupOwner409JSONResponse(authError(CodeConflict, "Setup is already complete. Sign in instead.")), nil
	case errors.Is(err, auth.ErrRateLimited):
		return SetupOwner429JSONResponse{RateLimitedJSONResponse(authError(CodeRateLimited, err.Error()))}, nil
	default:
		return nil, err
	}
}

// SignIn implements StrictServerInterface.
func (ss *StrictServer) SignIn(ctx context.Context, request SignInRequestObject) (SignInResponseObject, error) {
	if ss.server.config.OpenMode() {
		return SignIn409JSONResponse(authError(CodeConflict, openModeMessage)), nil
	}
	info := requestInfoFrom(ctx)
	user, token, err := ss.server.auth.SignIn(ctx, request.Body.Username, request.Body.Password, info.clientIP)
	switch {
	case err == nil:
		cookie := sessionCookie(token, info.secure)
		return SignIn200JSONResponse{Body: accountOf(user), Headers: SignIn200ResponseHeaders{SetCookie: &cookie}}, nil
	case errors.Is(err, auth.ErrInvalidCredentials):
		return SignIn401JSONResponse(authError(CodeInvalidCredentials, "The username or password is wrong")), nil
	case errors.Is(err, auth.ErrRateLimited):
		return SignIn429JSONResponse{RateLimitedJSONResponse(authError(CodeRateLimited, err.Error()))}, nil
	default:
		return nil, err
	}
}

// SignOut implements StrictServerInterface. It revokes the session on the
// server, not just the cookie, so a copied cookie stops working too.
func (ss *StrictServer) SignOut(ctx context.Context, _ SignOutRequestObject) (SignOutResponseObject, error) {
	info := requestInfoFrom(ctx)
	if ss.server.auth != nil {
		if err := ss.server.auth.SignOut(ctx, info.sessionToken); err != nil {
			return nil, err
		}
	}
	cookie := sessionCookie("", info.secure)
	return SignOut204Response{Headers: SignOut204ResponseHeaders{SetCookie: &cookie}}, nil
}

// GetCurrentAccount implements StrictServerInterface.
func (ss *StrictServer) GetCurrentAccount(ctx context.Context, _ GetCurrentAccountRequestObject) (GetCurrentAccountResponseObject, error) {
	if ss.server.config.OpenMode() {
		return GetCurrentAccount404JSONResponse(authError(CodeNotFound, openModeMessage)), nil
	}
	user := userFrom(ctx)
	if user == nil {
		// authenticate refuses this before the handler runs.
		return nil, echo.NewHTTPError(http.StatusUnauthorized, "Sign in to continue")
	}
	return GetCurrentAccount200JSONResponse(accountOf(user)), nil
}
