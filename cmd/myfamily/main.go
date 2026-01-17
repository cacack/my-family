// Package main is the entry point for the My Family genealogy application.
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository/memory"
	"github.com/cacack/my-family/internal/web"
)

// Build-time variables injected by goreleaser.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		runServer()
	case "version":
		fmt.Printf("my-family %s (commit: %s, built: %s)\n", version, commit, date)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`My Family - Self-hosted genealogy software

Usage:
  myfamily <command>

Commands:
  serve     Start the HTTP server
  version   Show version information
  help      Show this help message

Environment Variables:
  DATABASE_URL   PostgreSQL connection string (optional, uses SQLite by default)
  SQLITE_PATH    SQLite database path (default: ./myfamily.db)
  PORT           HTTP server port (default: 8080)
  LOG_LEVEL      Log level: debug, info, warn, error (default: info)
  LOG_FORMAT     Log format: text, json (default: text)`)
}

func runServer() {
	// Load configuration
	cfg := config.Load()

	// Create repositories
	// For MVP, use in-memory stores. SQLite will be added later.
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	snapshotStore := memory.NewSnapshotStore(eventStore)

	// Get frontend filesystem (embedded in production, local in dev)
	frontendFS, err := web.GetFileSystem()
	if err != nil {
		log.Printf("Warning: Frontend not available: %v", err)
		frontendFS = nil
	}

	log.Printf("Starting My Family server on port %d", cfg.Port)
	if cfg.UsePostgreSQL() {
		log.Printf("Database: PostgreSQL")
	} else {
		log.Printf("Database: In-memory (SQLite path configured: %s)", cfg.SQLitePath)
	}
	if frontendFS != nil {
		log.Printf("Frontend: Embedded")
	} else {
		log.Printf("Frontend: Not available (API only)")
	}

	// Create and start server
	server := api.NewServer(cfg, eventStore, readStore, snapshotStore, frontendFS)

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		log.Println("Shutting down server...")
		if err := server.Shutdown(); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}
	}()

	// Start server
	if err := server.Start(); err != nil {
		log.Printf("Server stopped: %v", err)
	}
}
