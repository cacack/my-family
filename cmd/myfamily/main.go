// Package main is the entry point for the My Family genealogy application.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/demo"
	"github.com/cacack/my-family/internal/storage"
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
		if err := runServer(); err != nil {
			log.Fatal(err)
		}
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
  DATABASE_URL   PostgreSQL connection string (if set, data is stored in PostgreSQL)
  SQLITE_PATH    SQLite database path, used when DATABASE_URL is unset (default: ./myfamily.db)
  PORT           HTTP server port (default: 8080)
  LOG_LEVEL      Log level: debug, info, warn, error (default: info)
  LOG_FORMAT     Log format: text, json (default: text)
  DEMO_MODE      Run in memory with sample data, no persistence; overrides
                 DATABASE_URL and SQLITE_PATH (default: false)`)
}

// runServer opens the configured store, serves until SIGINT/SIGTERM, then
// closes the store. It returns an error only for failures that must stop the
// process with a non-zero exit.
func runServer() error {
	// Load configuration
	cfg := config.Load()

	// Open the configured store: DEMO_MODE -> memory, DATABASE_URL -> PostgreSQL,
	// otherwise SQLite at SQLITE_PATH. A backend that cannot be opened stops the
	// server here; there is no silent fallback to memory.
	stores, err := storage.Open(cfg)
	if err != nil {
		return fmt.Errorf("failed to open %s storage: %w", storage.Select(cfg), err)
	}
	defer func() {
		if err := stores.Close(); err != nil {
			log.Printf("Error closing storage: %v", err)
		}
	}()

	// Get frontend filesystem (embedded in production, local in dev)
	frontendFS, err := web.GetFileSystem()
	if err != nil {
		log.Printf("Warning: Frontend not available: %v", err)
		frontendFS = nil
	}

	log.Printf("Starting My Family server on port %d", cfg.Port)
	log.Printf("Database: %s", stores.Describe())
	if cfg.DemoMode {
		log.Printf("Mode: DEMO (sample data, no persistence)")
		if cfg.UsePostgreSQL() {
			log.Printf("Note: DATABASE_URL is ignored in demo mode")
		}
	}
	if frontendFS != nil {
		log.Printf("Frontend: Embedded")
	} else {
		log.Printf("Frontend: Not available (API only)")
	}

	// Build server options
	serverOpts := []api.ServerOption{api.WithBranchStore(stores.Branches)}
	if cfg.DemoMode {
		mem := stores.Memory
		serverOpts = append(serverOpts, api.WithDemoReset(mem.Events, mem.ReadModel, mem.Snapshots))
	}

	// Create server
	server := api.NewServer(cfg, stores.Events, stores.ReadModel, stores.Snapshots, frontendFS, serverOpts...)

	// Seed demo data if demo mode is enabled
	if cfg.DemoMode {
		if err := demo.SeedDemoData(context.Background(), server.CommandHandler()); err != nil {
			return fmt.Errorf("failed to seed demo data: %w", err)
		}
		log.Printf("Demo data loaded: sample family tree ready")
	}

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

	// Start server; it returns once Shutdown closes it, and the deferred
	// Close then releases the database.
	if err := server.Start(); err != nil {
		log.Printf("Server stopped: %v", err)
	}
	return nil
}
