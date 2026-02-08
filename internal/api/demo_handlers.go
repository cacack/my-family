package api

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/cacack/my-family/internal/demo"
)

// getAppConfig returns application configuration visible to the frontend.
func (s *Server) getAppConfig(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{
		"demo_mode": s.config.DemoMode,
	})
}

// resetDemo resets all data and re-seeds the demo family tree.
func (s *Server) resetDemo(c echo.Context) error {
	if s.demo == nil {
		return c.JSON(http.StatusForbidden, map[string]string{
			"code":    "not_demo_mode",
			"message": "Reset is only available in demo mode",
		})
	}

	// Reset all stores
	s.demo.eventStore.Reset()
	s.demo.readStore.Reset()
	s.demo.snapshotStore.Reset()

	// Re-seed demo data
	if err := demo.SeedDemoData(context.Background(), s.commandHandler); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"code":    "seed_failed",
			"message": "Failed to re-seed demo data: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "reset"})
}
