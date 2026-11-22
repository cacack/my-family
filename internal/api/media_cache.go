package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// mediaBytesRoutes are the routes that serve a media item's bytes.
var mediaBytesRoutes = map[string]bool{
	routeKey(http.MethodGet, apiBase+"/media/:id/content"):   true,
	routeKey(http.MethodGet, apiBase+"/media/:id/thumbnail"): true,
}

// noStoreMedia tells the browser never to keep media bytes (#1578). Whether
// an item may be shown depends on who asks, and the owner's preview (#1413)
// changes who that is within one page, with the same URLs, so a kept copy
// could show the previewed principal the owner's image, or the reverse. It
// also keeps an item marked private from being shown from a cache after.
func noStoreMedia(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if mediaBytesRoutes[routeKey(c.Request().Method, c.Path())] {
			c.Response().Header().Set(echo.HeaderCacheControl, "no-store")
		}
		return next(c)
	}
}
