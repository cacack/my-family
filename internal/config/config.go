// Package config provides configuration loading and management.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds the application configuration.
type Config struct {
	// Database configuration
	DatabaseURL string // PostgreSQL connection string (if set, uses PostgreSQL)
	SQLitePath  string // SQLite database path (default: ./myfamily.db)

	// Server configuration
	Port      int    // HTTP server port (default: 8080)
	LogLevel  string // Logging level: debug, info, warn, error (default: info)
	LogFormat string // Log format: text, json (default: text)

	// Demo mode
	DemoMode bool // Run with pre-loaded sample data (ephemeral)

	// AuthMode is AUTH_MODE as given: AuthModeAccounts (also the empty
	// default) or AuthModeOpen. Read it through OpenMode; Validate rejects
	// anything else.
	AuthMode AuthMode
}

// AuthMode says whether requests must sign in (ADR-007).
type AuthMode string

// The AUTH_MODE values.
const (
	// AuthModeAccounts requires a signed-in user for every request. It is the
	// default, and the empty value means it too, so a Config built without
	// one fails closed.
	AuthModeAccounts AuthMode = "accounts"
	// AuthModeOpen treats every request as the owner, with no sign-in: for a
	// trusted network, demo mode and end-to-end tests. Only the environment
	// can set it, never the UI.
	AuthModeOpen AuthMode = "open"
)

// OpenMode reports whether every request is treated as the owner without
// signing in: AUTH_MODE=open, or DEMO_MODE, which implies it.
func (c *Config) OpenMode() bool {
	return c.AuthMode == AuthModeOpen || c.DemoMode
}

// Validate reports a setting the server must not start with.
func (c *Config) Validate() error {
	switch c.AuthMode {
	case "", AuthModeAccounts, AuthModeOpen:
		return nil
	default:
		return fmt.Errorf("AUTH_MODE must be %q or %q, got %q", AuthModeAccounts, AuthModeOpen, c.AuthMode)
	}
}

// Load reads configuration from environment variables.
func Load() *Config {
	cfg := &Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		SQLitePath:  getEnvOrDefault("SQLITE_PATH", "./myfamily.db"),
		Port:        getEnvIntOrDefault("PORT", 8080),
		LogLevel:    getEnvOrDefault("LOG_LEVEL", "info"),
		LogFormat:   getEnvOrDefault("LOG_FORMAT", "text"),
		DemoMode:    getEnvBoolOrDefault("DEMO_MODE", false),
		AuthMode:    AuthMode(strings.ToLower(strings.TrimSpace(os.Getenv("AUTH_MODE")))),
	}
	return cfg
}

// UsePostgreSQL returns true if PostgreSQL should be used.
func (c *Config) UsePostgreSQL() bool {
	return c.DatabaseURL != ""
}

// getEnvOrDefault returns the environment variable value or a default.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvBoolOrDefault returns the environment variable as bool or a default.
func getEnvBoolOrDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		switch strings.ToLower(value) {
		case "true", "1", "yes":
			return true
		case "false", "0", "no":
			return false
		}
	}
	return defaultValue
}

// getEnvIntOrDefault returns the environment variable as int or a default.
func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
