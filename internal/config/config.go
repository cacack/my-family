// Package config provides configuration loading and management.
package config

import (
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
