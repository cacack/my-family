package api

import (
	"context"
	"errors"
	"fmt"
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

// previewCookieName is the cookie an owner's browser carries while the owner
// previews the tree as someone else (#1413): previewViewer or previewVisitor.
// The web app sets and clears it; the server only reads it (servePreview).
const previewCookieName = "myfamily_preview"

// The principals an owner may preview the tree as.
const (
	previewViewer  = "viewer"
	previewVisitor = "visitor"
)

// Error codes of the sign-in layer.
const (
	CodeUnauthenticated    = "UNAUTHENTICATED"
	CodeSetupRequired      = "SETUP_REQUIRED"
	CodeForbidden          = "FORBIDDEN"
	CodeInvalidCredentials = "INVALID_CREDENTIALS" // #nosec G101 -- an error code, not a credential.
	CodeInvalidSetupToken  = "INVALID_SETUP_TOKEN"
	CodeRateLimited        = "RATE_LIMITED"
	CodeAccountDisabled    = "ACCOUNT_DISABLED"
)

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

// authenticate resolves who is making each request and refuses it unless their
// role includes the route's (ADR-007). A route with no declared role in
// routeRoles is refused for everyone, open mode included (AC-004). In open
// mode the requester is the owner, with no actor recorded. Otherwise the
// user comes from an API token (`Authorization: Bearer`, #1416) or else the
// session cookie: a valid one puts the user and their role on the request
// context, and the user as the actor their changes are attributed to
// (domain.ContextWithActor). A token that is not valid is refused with 401
// at once, never served as a visitor; a token never previews (servePreview)
// and is refused sessionOnlyRoutes with 403. A route that is not
// public is then refused with 503 while the tree has no owner, 401 without a
// session (unless the owner lets visitors in, see serveVisitor) and 403 below
// its role; an API path that matches no route is
// treated as a viewer's, so only a signed-in user learns it is a 404 or 405.
// Non-API paths (the web UI's files) are always served: the UI shows the
// setup or sign-in page itself.
func (s *Server) authenticate(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		req := c.Request()
		info := requestInfo{clientIP: c.RealIP(), secure: isSecure(req)}
		ctx := context.WithValue(req.Context(), requestInfoKey{}, info)
		if !strings.HasPrefix(echo.GetPath(req), "/api/") {
			c.SetRequest(req.WithContext(ctx))
			return next(c)
		}
		key := routeKey(req.Method, c.Path())
		required, declared := routeRoles[key]
		if !declared {
			if s.routes[key] {
				return refuse(c, http.StatusForbidden, CodeForbidden, "This operation declares no role, so no one may call it")
			}
			// Not a route: the router answers 404 or 405, to signed-in users.
			required = repository.RoleViewer
		}
		if s.config.OpenMode() {
			ctx = withRole(ctx, repository.RoleOwner, false)
			if cookie, err := req.Cookie(previewCookieName); err == nil && required != rolePublic {
				return s.servePreview(ctx, c, next, required, cookie.Value)
			}
			return s.serveTree(ctx, c, next)
		}

		ctx, viaToken, err := s.resolveUser(ctx, req)
		if errors.Is(err, auth.ErrNoAPIToken) {
			return refuse(c, http.StatusUnauthorized, CodeUnauthenticated,
				"This API token is unknown or revoked, or its account is disabled")
		}
		if err != nil {
			return err
		}
		c.SetRequest(req.WithContext(ctx))
		if required == rolePublic {
			return next(c)
		}

		setupRequired, err := s.auth.SetupRequired(ctx)
		if err != nil {
			return err
		}
		if setupRequired {
			return refuse(c, http.StatusServiceUnavailable, CodeSetupRequired,
				"This server has no owner yet. Create one on the setup page or with `myfamily user create`.")
		}
		if userFrom(ctx) == nil {
			return s.serveVisitor(ctx, c, next, required)
		}
		if viaToken && sessionOnlyRoutes[key] {
			return refuse(c, http.StatusForbidden, CodeForbidden, "An API token cannot do this: sign in to do it")
		}
		role := roleFrom(ctx)
		if cookie, err := req.Cookie(previewCookieName); err == nil && role == repository.RoleOwner && !viaToken {
			return s.servePreview(ctx, c, next, required, cookie.Value)
		}
		if !role.Includes(required) {
			return refuse(c, http.StatusForbidden, CodeForbidden, forbiddenMessage(role, required))
		}
		return s.serveTree(ctx, c, next)
	}
}

// resolveUser puts the requester's user on ctx (signedIn) from an API token,
// or else from the session cookie, and reports whether it came from a token.
// An API token that is not valid is auth.ErrNoAPIToken; a session that is not
// leaves ctx without a user.
func (s *Server) resolveUser(ctx context.Context, req *http.Request) (context.Context, bool, error) {
	if token, ok := bearerToken(req); ok {
		user, err := s.auth.AuthenticateAPIToken(ctx, token)
		if err != nil {
			return ctx, true, err
		}
		ctx, err = s.signedIn(ctx, user)
		return ctx, true, err
	}
	cookie, err := req.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return ctx, false, nil
	}
	info := requestInfoFrom(ctx)
	info.sessionToken = cookie.Value
	ctx = context.WithValue(ctx, requestInfoKey{}, info)
	user, err := s.auth.Authenticate(ctx, cookie.Value)
	if errors.Is(err, auth.ErrNoSession) {
		return ctx, false, nil
	}
	if err != nil {
		return ctx, false, err
	}
	ctx, err = s.signedIn(ctx, user)
	return ctx, false, err
}

// bearerToken returns the API token an `Authorization: Bearer` header
// carries, and whether the request has one.
func bearerToken(req *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(req.Header.Get(echo.HeaderAuthorization), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	return strings.TrimSpace(token), true
}

// signedIn returns ctx carrying user, signed in by a session or an API token:
// the user, the role they hold now, and the user as the actor their changes
// are attributed to.
func (s *Server) signedIn(ctx context.Context, user *repository.User) (context.Context, error) {
	role, err := s.auth.Role(ctx, repository.DefaultTreeID, user.ID)
	if err != nil && !errors.Is(err, repository.ErrNotMember) {
		return nil, err
	}
	ctx = context.WithValue(ctx, userKey{}, user)
	ctx = withRole(ctx, role, false)
	return domain.ContextWithActor(ctx, user.ID.String()), nil
}

// servePreview serves an owner's request as the principal they preview the
// tree as (#1413), so the preview shows exactly what the API returns to that
// principal: a viewer's role, or a visitor's (the anonymous principal, with
// no user on the context, refused the account and change-log routes as
// serveVisitor refuses them). A visitor preview works whether or not visitors
// are let in, so the owner can look before letting them in. What the
// principal may not call is refused with 403, not 401: the owner is still
// signed in. The cookie is ignored for everyone but the owner, since it can
// only narrow what they see; a value that is no principal is refused, rather
// than showing the owner everything while they think they are previewing.
func (s *Server) servePreview(ctx context.Context, c echo.Context, next echo.HandlerFunc, required repository.Role, as string) error {
	const stop = " Stop the preview to do it as yourself."
	switch as {
	case previewViewer:
		if !repository.RoleViewer.Includes(required) {
			return refuse(c, http.StatusForbidden, CodeForbidden, "A viewer cannot do this."+stop)
		}
		ctx = withRole(ctx, repository.RoleViewer, false)
	case previewVisitor:
		if !visitorMayCall(c, required) {
			return refuse(c, http.StatusForbidden, CodeForbidden, "A visitor cannot do this."+stop)
		}
		ctx = asVisitor(ctx)
	default:
		return refuse(c, http.StatusBadRequest, CodeBadRequest,
			fmt.Sprintf("Cannot preview the tree as %q: only as a viewer or a visitor. Stop the preview and start it again.", as))
	}
	return s.serveTree(ctx, c, next)
}

// serveVisitor serves a request with no session as the anonymous principal
// (ADR-007): when visitorsLetIn, it may call what a viewer may, except
// signedInRoutes; otherwise, or beyond that, it is refused with 401, since
// signing in may allow it. The anonymous principal holds the viewer role with
// no user on the context (isAnonymous).
func (s *Server) serveVisitor(ctx context.Context, c echo.Context, next echo.HandlerFunc, required repository.Role) error {
	settings, err := s.treeSettings(ctx)
	if err != nil {
		return err
	}
	if !s.visitorsLetIn(settings) || !visitorMayCall(c, required) {
		return refuse(c, http.StatusUnauthorized, CodeUnauthenticated, "Sign in to continue")
	}
	ctx = asVisitor(ctx)
	c.SetRequest(c.Request().WithContext(repository.ContextWithTreeSettings(ctx, settings)))
	return next(c)
}

// visitorsLetIn reports whether visitors may browse the tree: its anonymous
// access is viewer and the read side protects them from living people and
// private records (repository.AnonymousViewingProtected). The settings API
// refuses viewer while they are unprotected; this also refuses a viewer
// written to the store some other way.
func (s *Server) visitorsLetIn(settings repository.TreeSettings) bool {
	return s.visitorsProtected && settings.AnonymousAccess == repository.AnonymousAccessViewer
}

// visitorMayCall reports whether the anonymous principal may make the
// request, once let in: what a viewer may, except signedInRoutes, and only on
// the mainline, so never with a `?branch=` scope (ADR-008 §4). serveVisitor
// and a visitor preview both ask it, so the preview refuses what a visitor is
// refused.
func visitorMayCall(c echo.Context, required repository.Role) bool {
	return required == repository.RoleViewer &&
		!signedInRoutes[routeKey(c.Request().Method, c.Path())] &&
		!c.QueryParams().Has("branch")
}

// asVisitor returns ctx carrying the anonymous principal: the viewer role
// with no user. serveVisitor grants it, and so does a visitor preview.
func asVisitor(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, userKey{}, (*repository.User)(nil))
	return withRole(ctx, repository.RoleViewer, true)
}

// isAnonymous reports whether the request is served as a visitor: the
// anonymous principal asVisitor puts on the context, for a real visitor or
// an owner previewing as one.
func isAnonymous(ctx context.Context) bool {
	return roleFrom(ctx) == repository.RoleViewer && repository.PrincipalFrom(ctx).Anonymous
}

// serveTree calls a handler that may read the tree, with the owner's privacy
// settings on its context (ADR-008 §6). Public routes (health, sign-in, setup)
// read no tree data, so they skip this lookup and keep working when the
// settings store does not.
func (s *Server) serveTree(ctx context.Context, c echo.Context, next echo.HandlerFunc) error {
	settings, err := s.treeSettings(ctx)
	if err != nil {
		return err
	}
	c.SetRequest(c.Request().WithContext(repository.ContextWithTreeSettings(ctx, settings)))
	return next(c)
}

// forbiddenMessage tells a user why their role may not do something.
func forbiddenMessage(role, required repository.Role) string {
	if !role.Valid() {
		return "Your account has no role in this tree. Ask the owner for access."
	}
	return fmt.Sprintf("Your role (%s) cannot do this: it needs %s or above. Ask the tree's owner if you need it.", role, required)
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

func accountOf(user *repository.User, role repository.Role) Account {
	return Account{
		Id:          user.ID,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Role:        AccountRole(role),
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
	// A public route the web app needs to start: a settings failure reports
	// visitors as refused (the fail-closed value) rather than failing it.
	access := AnonymousAccessNone
	if settings, err := ss.server.treeSettings(ctx); err != nil {
		ss.server.echo.Logger.Errorf("reading the tree settings for the auth status: %v", err)
	} else if ss.server.visitorsLetIn(settings) {
		access = AnonymousAccessViewer
	}
	status := GetAuthStatus200JSONResponse{Mode: AuthStatusModeAccounts, SetupRequired: required, AnonymousAccess: &access}
	if user := userFrom(ctx); user != nil {
		account := accountOf(user, roleFrom(ctx))
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
		return SetupOwner201JSONResponse{Body: accountOf(user, repository.RoleOwner), Headers: SetupOwner201ResponseHeaders{SetCookie: &cookie}}, nil
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
		role, err := ss.server.auth.Role(ctx, repository.DefaultTreeID, user.ID)
		if err != nil && !errors.Is(err, repository.ErrNotMember) {
			return nil, err
		}
		cookie := sessionCookie(token, info.secure)
		return SignIn200JSONResponse{Body: accountOf(user, role), Headers: SignIn200ResponseHeaders{SetCookie: &cookie}}, nil
	case errors.Is(err, auth.ErrInvalidCredentials):
		return SignIn401JSONResponse(authError(CodeInvalidCredentials, "The username or password is wrong")), nil
	case errors.Is(err, auth.ErrAccountDisabled):
		return SignIn403JSONResponse(authError(CodeAccountDisabled, "This account is disabled. Ask the tree's owner to re-enable it.")), nil
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
		return GetCurrentAccount409JSONResponse{accountsOnly}, nil
	}
	user := userFrom(ctx)
	if user == nil {
		// authenticate refuses this before the handler runs.
		return nil, echo.NewHTTPError(http.StatusUnauthorized, "Sign in to continue")
	}
	return GetCurrentAccount200JSONResponse(accountOf(user, roleFrom(ctx))), nil
}
