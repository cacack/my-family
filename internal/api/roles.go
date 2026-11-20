package api

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"

	"github.com/cacack/my-family/internal/repository"
)

// rolePublic is the x-min-role of an operation served without a session.
// It is not a membership role: no user holds it.
const rolePublic repository.Role = "public"

// apiBase is where the API routes are mounted.
const apiBase = "/api/v1"

// handRouteRoles declares the minimum role of the API routes registered by
// hand rather than generated from openapi.yaml, keyed like routeRoles.
var handRouteRoles = map[string]repository.Role{
	routeKey(http.MethodGet, apiBase+"/health"):                rolePublic,
	routeKey(http.MethodGet, apiBase+"/config"):                repository.RoleViewer,
	routeKey(http.MethodGet, apiBase+"/openapi.yaml"):          repository.RoleViewer,
	routeKey(http.MethodGet, apiBase+"/docs"):                  repository.RoleViewer,
	routeKey(http.MethodPost, apiBase+"/demo/reset"):           repository.RoleOwner,
	routeKey(http.MethodPost, apiBase+"/gedcom/import/stream"): repository.RoleOwner,
}

// signedInRoutes need a viewer who is signed in, so serveVisitor refuses
// them to a visitor: the operations on one's own account (including its home
// person), which a visitor does not have, and the change log (history,
// restore points, snapshots and comparisons), whose change records name who
// made each change, which ADR-008 §4 hides from anonymous principals, and a
// research branch's view (health and evidence coverage), since a visitor
// reads the mainline only. A `?branch=` scope on any route is refused to a
// visitor too (visitorMayCall).
var signedInRoutes = map[string]bool{
	routeKey(http.MethodGet, apiBase+"/auth/me"):                        true,
	routeKey(http.MethodPatch, apiBase+"/auth/me"):                      true,
	routeKey(http.MethodPut, apiBase+"/settings/home-person"):           true,
	routeKey(http.MethodDelete, apiBase+"/settings/home-person"):        true,
	routeKey(http.MethodGet, apiBase+"/history"):                        true,
	routeKey(http.MethodGet, apiBase+"/persons/:id/history"):            true,
	routeKey(http.MethodGet, apiBase+"/families/:id/history"):           true,
	routeKey(http.MethodGet, apiBase+"/sources/:id/history"):            true,
	routeKey(http.MethodGet, apiBase+"/persons/:id/restore-points"):     true,
	routeKey(http.MethodGet, apiBase+"/families/:id/restore-points"):    true,
	routeKey(http.MethodGet, apiBase+"/sources/:id/restore-points"):     true,
	routeKey(http.MethodGet, apiBase+"/citations/:id/restore-points"):   true,
	routeKey(http.MethodGet, apiBase+"/snapshots"):                      true,
	routeKey(http.MethodGet, apiBase+"/snapshots/:id"):                  true,
	routeKey(http.MethodGet, apiBase+"/snapshots/:id/compare-current"):  true,
	routeKey(http.MethodGet, apiBase+"/snapshots/:id1/compare/:id2"):    true,
	routeKey(http.MethodGet, apiBase+"/branches/:id/compare"):           true,
	routeKey(http.MethodGet, apiBase+"/branches/:id/evidence-coverage"): true,
	routeKey(http.MethodGet, apiBase+"/branches/:id/health"):            true,
}

// routeRoles is the minimum role of every API route (ADR-007): each
// operation's x-min-role in openapi.yaml, plus handRouteRoles. A route absent
// from it is refused for everyone (AC-004).
var routeRoles = mustLoadRouteRoles(openapiSpec)

// routeKey identifies a route as the router matched it: the method and the
// registered path, with Echo's :param placeholders (echo.GetPath).
func routeKey(method, path string) string {
	return method + " " + path
}

// specParam matches an OpenAPI path parameter, `{id}`.
var specParam = regexp.MustCompile(`\{([^}/]+)\}`)

// specRoles reads each operation's x-min-role from an OpenAPI document. An
// operation without one is left out, so it is refused at runtime; a value that
// is not a role is an error.
func specRoles(spec []byte) (map[string]repository.Role, error) {
	doc, err := openapi3.NewLoader().LoadFromData(spec)
	if err != nil {
		return nil, fmt.Errorf("load openapi spec: %w", err)
	}
	roles := make(map[string]repository.Role)
	for path, item := range doc.Paths.Map() {
		echoPath := apiBase + specParam.ReplaceAllString(path, ":$1")
		for method, op := range item.Operations() {
			value, ok := op.Extensions["x-min-role"]
			if !ok {
				continue
			}
			role, _ := value.(string)
			if r := repository.Role(role); r != rolePublic && !r.Valid() {
				return nil, fmt.Errorf("%s %s: x-min-role %v is not public, viewer, editor or owner", method, path, value)
			}
			roles[routeKey(method, echoPath)] = repository.Role(role)
		}
	}
	return roles, nil
}

func mustLoadRouteRoles(spec []byte) map[string]repository.Role {
	roles, err := specRoles(spec)
	if err != nil {
		panic(err)
	}
	for key, role := range handRouteRoles {
		roles[key] = role
	}
	return roles
}

type roleKey struct{}

// withRole returns ctx carrying the requester's role, and the principal the
// read side shows the tree to (ADR-008 §4): anonymous for a visitor, who holds
// the viewer role without an account.
func withRole(ctx context.Context, role repository.Role, anonymous bool) context.Context {
	ctx = context.WithValue(ctx, roleKey{}, role)
	return repository.ContextWithPrincipal(ctx, repository.Principal{Role: role, Anonymous: anonymous})
}

// roleFrom returns the role of whoever is making the request: the signed-in
// user's role, the owner in open mode, or "" when no one is signed in.
func roleFrom(ctx context.Context) repository.Role {
	role, _ := ctx.Value(roleKey{}).(repository.Role)
	return role
}

// GEDCOM export limits (#387): each export builds the whole document, so one
// user may start exportBurst at once and one more every exportInterval.
const (
	exportInterval = 10 * time.Second
	exportBurst    = 5
)

// exportRoutes are the GEDCOM exports limitExports throttles.
var exportRoutes = map[string]bool{
	routeKey(http.MethodGet, apiBase+"/gedcom/export"):         true,
	routeKey(http.MethodGet, apiBase+"/gedcom/export/preview"): true,
}

// limitExports rate-limits GEDCOM exports and previews per signed-in user. In
// open mode there are no users and nothing is limited: the operator chose a
// trusted network.
func limitExports() echo.MiddlewareFunc {
	return middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Skipper: func(c echo.Context) bool {
			return userFrom(c.Request().Context()) == nil || !exportRoutes[routeKey(c.Request().Method, c.Path())]
		},
		Store: middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{
			Rate:      rate.Every(exportInterval),
			Burst:     exportBurst,
			ExpiresIn: time.Duration(exportBurst) * exportInterval,
		}),
		IdentifierExtractor: func(c echo.Context) (string, error) {
			return userFrom(c.Request().Context()).ID.String(), nil
		},
		DenyHandler: func(c echo.Context, _ string, _ error) error {
			return refuse(c, http.StatusTooManyRequests, CodeRateLimited,
				"Too many GEDCOM exports in a short time; try again in a few seconds")
		},
	})
}

// Anonymous read limits (#1414): a visitor needs no account, so each client
// address may call the costly reads anonymousBurst times at once and one
// more every anonymousInterval. Signed-in users are not limited here.
const (
	anonymousInterval = time.Second
	anonymousBurst    = 30
)

// anonymousCostlyRoutes are the reads a visitor may call whose cost grows
// with the tree: lists, browse indexes and their drill-downs, searches,
// charts, aggregates and reports, plus file bytes. limitAnonymousReads
// throttles them. Reads of one record, or of one record's own sub-lists, are
// cheap and not listed; TestAnonymousRouteClassification lists those, so a
// new viewer route must be put in one class or the other.
var anonymousCostlyRoutes = map[string]bool{
	routeKey(http.MethodGet, apiBase+"/persons"):                            true,
	routeKey(http.MethodGet, apiBase+"/families"):                           true,
	routeKey(http.MethodGet, apiBase+"/sources"):                            true,
	routeKey(http.MethodGet, apiBase+"/sources/:id/citations"):              true,
	routeKey(http.MethodGet, apiBase+"/notes"):                              true,
	routeKey(http.MethodGet, apiBase+"/repositories"):                       true,
	routeKey(http.MethodGet, apiBase+"/submitters"):                         true,
	routeKey(http.MethodGet, apiBase+"/associations"):                       true,
	routeKey(http.MethodGet, apiBase+"/lds-ordinances"):                     true,
	routeKey(http.MethodGet, apiBase+"/evidence-analyses"):                  true,
	routeKey(http.MethodGet, apiBase+"/evidence-conflicts"):                 true,
	routeKey(http.MethodGet, apiBase+"/proof-summaries"):                    true,
	routeKey(http.MethodGet, apiBase+"/research-logs"):                      true,
	routeKey(http.MethodGet, apiBase+"/branches"):                           true,
	routeKey(http.MethodGet, apiBase+"/persons/duplicates/dismissed"):       true,
	routeKey(http.MethodGet, apiBase+"/browse/surnames/:surname/persons"):   true,
	routeKey(http.MethodGet, apiBase+"/browse/places/:place/persons"):       true,
	routeKey(http.MethodGet, apiBase+"/browse/cemeteries/:place/persons"):   true,
	routeKey(http.MethodGet, apiBase+"/search"):                             true,
	routeKey(http.MethodGet, apiBase+"/sources/search"):                     true,
	routeKey(http.MethodGet, apiBase+"/pedigree/:id"):                       true,
	routeKey(http.MethodGet, apiBase+"/descendancy/:id"):                    true,
	routeKey(http.MethodGet, apiBase+"/ahnentafel/:id"):                     true,
	routeKey(http.MethodGet, apiBase+"/relationship/:personId1/:personId2"): true,
	routeKey(http.MethodGet, apiBase+"/families/:id/group-sheet"):           true,
	routeKey(http.MethodGet, apiBase+"/browse/surnames"):                    true,
	routeKey(http.MethodGet, apiBase+"/browse/places"):                      true,
	routeKey(http.MethodGet, apiBase+"/browse/cemeteries"):                  true,
	routeKey(http.MethodGet, apiBase+"/browse/brick-walls"):                 true,
	routeKey(http.MethodGet, apiBase+"/map/locations"):                      true,
	routeKey(http.MethodGet, apiBase+"/statistics"):                         true,
	routeKey(http.MethodGet, apiBase+"/analytics/discovery"):                true,
	routeKey(http.MethodGet, apiBase+"/quality/overview"):                   true,
	routeKey(http.MethodGet, apiBase+"/quality/report"):                     true,
	routeKey(http.MethodGet, apiBase+"/quality/validation"):                 true,
	routeKey(http.MethodGet, apiBase+"/persons/duplicates"):                 true,
	routeKey(http.MethodGet, apiBase+"/branches/:id/drift"):                 true,
	routeKey(http.MethodGet, apiBase+"/media/:id/content"):                  true,
	routeKey(http.MethodGet, apiBase+"/media/:id/thumbnail"):                true,
}

// limitAnonymousReads rate-limits anonymous visitors' costly reads per client
// address (the connection's, not a header's: see the server's IPExtractor).
func limitAnonymousReads() echo.MiddlewareFunc {
	return middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Skipper: func(c echo.Context) bool {
			return !isAnonymous(c.Request().Context()) || !anonymousCostlyRoutes[routeKey(c.Request().Method, c.Path())]
		},
		Store: middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{
			Rate:      rate.Every(anonymousInterval),
			Burst:     anonymousBurst,
			ExpiresIn: time.Duration(anonymousBurst) * anonymousInterval,
		}),
		IdentifierExtractor: func(c echo.Context) (string, error) {
			return c.RealIP(), nil
		},
		DenyHandler: func(c echo.Context, _ string, _ error) error {
			return refuse(c, http.StatusTooManyRequests, CodeRateLimited,
				"Too many requests in a short time; wait a few seconds, or sign in")
		},
	})
}
