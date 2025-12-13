// Package api provides the HTTP API server for the genealogy application.
package api

import (
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

// Server wraps the Echo server with application dependencies.
type Server struct {
	echo            *echo.Echo
	config          *config.Config
	readStore       repository.ReadModelStore
	commandHandler  *command.Handler
	personService   *query.PersonService
	familyService   *query.FamilyService
	pedigreeService *query.PedigreeService
	frontendFS      fs.FS
}

// NewServer creates a new API server with all dependencies.
func NewServer(
	cfg *config.Config,
	eventStore repository.EventStore,
	readStore repository.ReadModelStore,
	frontendFS fs.FS,
) *Server {
	e := echo.New()
	e.HideBanner = true

	// Setup middleware stack (order matters)
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())

	// Configure logger based on config
	if cfg.LogFormat == "json" {
		e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
			Format: `{"time":"${time_rfc3339}","id":"${id}","method":"${method}","uri":"${uri}","status":${status},"latency":"${latency_human}"}` + "\n",
		}))
	} else {
		e.Use(middleware.Logger())
	}

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
	}))

	// Custom error handler
	e.HTTPErrorHandler = customErrorHandler

	// Create services
	cmdHandler := command.NewHandler(eventStore, readStore)
	personSvc := query.NewPersonService(readStore)
	familySvc := query.NewFamilyService(readStore)
	pedigreeSvc := query.NewPedigreeService(readStore)

	server := &Server{
		echo:            e,
		config:          cfg,
		readStore:       readStore,
		commandHandler:  cmdHandler,
		personService:   personSvc,
		familyService:   familySvc,
		pedigreeService: pedigreeSvc,
		frontendFS:      frontendFS,
	}

	// Register routes
	server.registerRoutes()

	return server
}

// registerRoutes sets up all API routes.
func (s *Server) registerRoutes() {
	api := s.echo.Group("/api/v1")

	// Health check
	api.GET("/health", s.healthCheck)

	// API documentation
	s.registerDocsRoutes(api)

	// Person routes
	api.GET("/persons", s.listPersons)
	api.POST("/persons", s.createPerson)
	api.GET("/persons/:id", s.getPerson)
	api.PUT("/persons/:id", s.updatePerson)
	api.DELETE("/persons/:id", s.deletePerson)

	// Search
	api.GET("/search", s.searchPersons)

	// Families (placeholder - will be implemented in Phase 4)
	api.GET("/families", s.listFamilies)
	api.POST("/families", s.createFamily)
	api.GET("/families/:id", s.getFamily)
	api.PUT("/families/:id", s.updateFamily)
	api.DELETE("/families/:id", s.deleteFamily)
	api.POST("/families/:id/children", s.addChildToFamily)
	api.DELETE("/families/:id/children/:personId", s.removeChildFromFamily)

	// Pedigree (placeholder - will be implemented in Phase 6)
	api.GET("/pedigree/:id", s.getPedigree)

	// GEDCOM (placeholder - will be implemented in Phase 5)
	api.POST("/gedcom/import", s.importGedcom)
	api.GET("/gedcom/export", s.exportGedcom)

	// Serve frontend if available
	if s.frontendFS != nil {
		// Serve static files
		fileServer := http.FileServer(http.FS(s.frontendFS))
		s.echo.GET("/*", echo.WrapHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Don't serve frontend for API routes
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}

			// Try to serve the requested file
			path := r.URL.Path
			if path == "/" {
				path = "/index.html"
			}

			// Check if file exists
			if _, err := fs.Stat(s.frontendFS, strings.TrimPrefix(path, "/")); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}

			// Fall back to index.html for SPA routing
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
		})))
	}
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.config.Port)
	return s.echo.Start(addr)
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown() error {
	return s.echo.Close()
}

// Echo returns the underlying Echo instance (for testing).
func (s *Server) Echo() *echo.Echo {
	return s.echo
}

// Health check handler.
func (s *Server) healthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
