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
	echo              *echo.Echo
	config            *config.Config
	readStore         repository.ReadModelStore
	commandHandler    *command.Handler
	personService     *query.PersonService
	familyService     *query.FamilyService
	pedigreeService   *query.PedigreeService
	ahnentafelService *query.AhnentafelService
	sourceService     *query.SourceService
	historyService    *query.HistoryService
	rollbackService   *query.RollbackService
	browseService     *query.BrowseService
	qualityService    *query.QualityService
	snapshotService   *query.SnapshotService
	frontendFS        fs.FS
}

// NewServer creates a new API server with all dependencies.
func NewServer(
	cfg *config.Config,
	eventStore repository.EventStore,
	readStore repository.ReadModelStore,
	snapshotStore repository.SnapshotStore,
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
	ahnentafelSvc := query.NewAhnentafelService(pedigreeSvc)
	sourceSvc := query.NewSourceService(readStore)
	historySvc := query.NewHistoryService(eventStore, readStore)
	rollbackSvc := query.NewRollbackService(eventStore, readStore)
	browseSvc := query.NewBrowseService(readStore)
	qualitySvc := query.NewQualityService(readStore)
	snapshotSvc := query.NewSnapshotService(snapshotStore, eventStore, historySvc)

	server := &Server{
		echo:              e,
		config:            cfg,
		readStore:         readStore,
		commandHandler:    cmdHandler,
		personService:     personSvc,
		familyService:     familySvc,
		pedigreeService:   pedigreeSvc,
		ahnentafelService: ahnentafelSvc,
		sourceService:     sourceSvc,
		historyService:    historySvc,
		rollbackService:   rollbackSvc,
		browseService:     browseSvc,
		qualityService:    qualitySvc,
		snapshotService:   snapshotSvc,
		frontendFS:        frontendFS,
	}

	// Register routes
	server.registerRoutes()

	return server
}

// registerRoutes sets up all API routes.
func (s *Server) registerRoutes() {
	api := s.echo.Group("/api/v1")

	// Health check (outside generated routes)
	api.GET("/health", s.healthCheck)

	// API documentation (outside generated routes)
	s.registerDocsRoutes(api)

	// Use generated strict handler registration for all API routes
	// This provides compile-time type safety for all endpoints
	strictServer := NewStrictServer(s)
	strictHandler := NewStrictHandler(strictServer, nil)
	RegisterHandlersWithBaseURL(s.echo, strictHandler, "/api/v1")

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
