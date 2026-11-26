// Package config provides configuration loading and management.
package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultAuthAuditRetentionDays is how many days the auth audit log keeps an
// entry when AUTH_AUDIT_RETENTION_DAYS is unset.
const DefaultAuthAuditRetentionDays = 365

// MaxAuthAuditRetentionDays is the longest retention Validate accepts, well
// inside what a time.Duration can hold.
const MaxAuthAuditRetentionDays = 36500

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

	// TrustedProxies is TRUSTED_PROXIES as given: comma-separated addresses
	// or CIDR ranges of the reverse proxies whose X-Forwarded-For is
	// believed. Read it through TrustedProxyRanges; Validate rejects an entry
	// that is neither. Empty trusts no one.
	TrustedProxies string

	// AuthAuditRetentionDays is AUTH_AUDIT_RETENTION_DAYS as given: how many
	// days an auth audit entry is kept before sign-in prunes it. Empty means
	// DefaultAuthAuditRetentionDays. Read it through AuthAuditRetention;
	// Validate rejects anything but a whole number from 1 to
	// MaxAuthAuditRetentionDays.
	AuthAuditRetentionDays string

	// GeocoderEnabled is GEOCODER_ENABLED: whether the server may send place
	// names to the geocoding provider at GeocoderURL (#1562). Off by
	// default, since it sends the tree's place names to an outside service.
	GeocoderEnabled bool
	// GeocoderURL is GEOCODER_URL: the base URL of the Nominatim-compatible
	// provider. Validate rejects a value that is not an absolute http(s) URL.
	GeocoderURL string
	// GeocoderContact is GEOCODER_CONTACT: an email address or URL that
	// identifies this install in the User-Agent, as the Nominatim usage
	// policy requires. Validate requires it when GeocoderEnabled.
	GeocoderContact string
}

// DefaultGeocoderURL is the public Nominatim endpoint, used when GEOCODER_URL
// is unset.
const DefaultGeocoderURL = "https://nominatim.openstreetmap.org"

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

// TrustedProxyRanges parses TrustedProxies: each entry is a CIDR range or a
// single address (a range of one). It returns nil when no proxy is trusted.
func (c *Config) TrustedProxyRanges() ([]*net.IPNet, error) {
	var ranges []*net.IPNet
	for _, entry := range strings.Split(c.TrustedProxies, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, ipNet, err := net.ParseCIDR(entry); err == nil {
			ranges = append(ranges, ipNet)
			continue
		}
		ip := net.ParseIP(entry)
		if ip == nil {
			return nil, fmt.Errorf("TRUSTED_PROXIES: %q is not an address or CIDR range", entry)
		}
		bits := 8 * net.IPv6len
		if ip4 := ip.To4(); ip4 != nil {
			ip, bits = ip4, 8*net.IPv4len
		}
		ranges = append(ranges, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
	}
	return ranges, nil
}

// AuthAuditRetention is how long the auth audit log keeps an entry. It is
// zero, which keeps every entry, for a value Validate rejects.
func (c *Config) AuthAuditRetention() time.Duration {
	days, err := c.authAuditRetentionDays()
	if err != nil {
		return 0
	}
	return time.Duration(days) * 24 * time.Hour
}

// authAuditRetentionDays parses AuthAuditRetentionDays.
func (c *Config) authAuditRetentionDays() (int, error) {
	value := strings.TrimSpace(c.AuthAuditRetentionDays)
	if value == "" {
		return DefaultAuthAuditRetentionDays, nil
	}
	days, err := strconv.Atoi(value)
	if err != nil || days < 1 || days > MaxAuthAuditRetentionDays {
		return 0, fmt.Errorf("AUTH_AUDIT_RETENTION_DAYS must be a whole number of days from 1 to %d, got %q",
			MaxAuthAuditRetentionDays, c.AuthAuditRetentionDays)
	}
	return days, nil
}

// Validate reports a setting the server must not start with.
func (c *Config) Validate() error {
	switch c.AuthMode {
	case "", AuthModeAccounts, AuthModeOpen:
	default:
		return fmt.Errorf("AUTH_MODE must be %q or %q, got %q", AuthModeAccounts, AuthModeOpen, c.AuthMode)
	}
	if _, err := c.authAuditRetentionDays(); err != nil {
		return err
	}
	if err := c.validateGeocoder(); err != nil {
		return err
	}
	_, err := c.TrustedProxyRanges()
	return err
}

// validateGeocoder checks the geocoder settings, which matter only when it is
// enabled.
func (c *Config) validateGeocoder() error {
	if !c.GeocoderEnabled {
		return nil
	}
	if strings.TrimSpace(c.GeocoderContact) == "" {
		return fmt.Errorf("GEOCODER_CONTACT must name an email address or URL when GEOCODER_ENABLED is set")
	}
	u, err := url.Parse(c.GeocoderURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("GEOCODER_URL must be an absolute http or https URL, got %q", c.GeocoderURL)
	}
	return nil
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

		TrustedProxies:         os.Getenv("TRUSTED_PROXIES"),
		AuthAuditRetentionDays: os.Getenv("AUTH_AUDIT_RETENTION_DAYS"),

		GeocoderEnabled: getEnvBoolOrDefault("GEOCODER_ENABLED", false),
		GeocoderURL:     getEnvOrDefault("GEOCODER_URL", DefaultGeocoderURL),
		GeocoderContact: strings.TrimSpace(os.Getenv("GEOCODER_CONTACT")),
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
