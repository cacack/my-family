package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// importRoutePaths are the route templates of the two GEDCOM import endpoints:
// the generated strict route and the hand-registered SSE stream.
var importRoutePaths = map[string]struct{}{
	"/api/v1/gedcom/import":        {},
	"/api/v1/gedcom/import/stream": {},
}

// refuseBranchScopedImport is the server-side backstop for issue #825.
//
// GEDCOM import always writes the mainline: command.Handler.ImportGedcom
// appends and projects with a hardcoded main scope, and importing onto a
// research branch is a stated non-goal (ADR-005). Neither import route
// declares the `branchScope` parameter, so an unknown `?branch=` would
// otherwise be silently ignored — a client that believed it was importing into
// its branch would rewrite the mainline instead. The UI withdraws import while
// a branch is active; this middleware makes the API refuse the same request
// loudly rather than trusting every caller to have done so.
//
// Branch scope is transmitted only as the `?branch=` query parameter (see the
// web client's withBranch), so that is the only carrier checked. Any value —
// even an empty one — is refused: the parameter's presence alone says the
// caller thinks it is scoped.
func refuseBranchScopedImport(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if _, isImport := importRoutePaths[c.Path()]; isImport && c.Request().Method == http.MethodPost {
			if _, scoped := c.QueryParams()["branch"]; scoped {
				return echo.NewHTTPError(http.StatusBadRequest,
					"GEDCOM import always writes the mainline and cannot be scoped to a research branch; switch to the mainline to import")
			}
		}
		return next(c)
	}
}
