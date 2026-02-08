package config

import (
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// Test Load() with no environment variables set
	cfg := Load()

	if cfg.DatabaseURL != "" {
		t.Errorf("expected DatabaseURL to be empty, got %q", cfg.DatabaseURL)
	}

	if cfg.SQLitePath != "./myfamily.db" {
		t.Errorf("expected SQLitePath to be './myfamily.db', got %q", cfg.SQLitePath)
	}

	if cfg.Port != 8080 {
		t.Errorf("expected Port to be 8080, got %d", cfg.Port)
	}

	if cfg.LogLevel != "info" {
		t.Errorf("expected LogLevel to be 'info', got %q", cfg.LogLevel)
	}

	if cfg.LogFormat != "text" {
		t.Errorf("expected LogFormat to be 'text', got %q", cfg.LogFormat)
	}

	if cfg.DemoMode {
		t.Error("expected DemoMode to be false by default")
	}
}

func TestLoad_AllEnvVarsSet(t *testing.T) {
	// Test Load() with all environment variables set
	t.Setenv("DATABASE_URL", "postgresql://user:pass@localhost:5432/mydb")
	t.Setenv("SQLITE_PATH", "/custom/path/db.sqlite")
	t.Setenv("PORT", "3000")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "json")

	cfg := Load()

	if cfg.DatabaseURL != "postgresql://user:pass@localhost:5432/mydb" {
		t.Errorf("expected DatabaseURL to be 'postgresql://user:pass@localhost:5432/mydb', got %q", cfg.DatabaseURL)
	}

	if cfg.SQLitePath != "/custom/path/db.sqlite" {
		t.Errorf("expected SQLitePath to be '/custom/path/db.sqlite', got %q", cfg.SQLitePath)
	}

	if cfg.Port != 3000 {
		t.Errorf("expected Port to be 3000, got %d", cfg.Port)
	}

	if cfg.LogLevel != "debug" {
		t.Errorf("expected LogLevel to be 'debug', got %q", cfg.LogLevel)
	}

	if cfg.LogFormat != "json" {
		t.Errorf("expected LogFormat to be 'json', got %q", cfg.LogFormat)
	}
}

func TestUsePostgreSQL_WithDatabaseURL(t *testing.T) {
	// Test UsePostgreSQL() returns true when DATABASE_URL is set
	cfg := &Config{
		DatabaseURL: "postgresql://localhost/test",
	}

	if !cfg.UsePostgreSQL() {
		t.Error("expected UsePostgreSQL() to return true when DatabaseURL is set")
	}
}

func TestUsePostgreSQL_WithoutDatabaseURL(t *testing.T) {
	// Test UsePostgreSQL() returns false when DATABASE_URL is empty
	cfg := &Config{
		DatabaseURL: "",
	}

	if cfg.UsePostgreSQL() {
		t.Error("expected UsePostgreSQL() to return false when DatabaseURL is empty")
	}
}

func TestGetEnvOrDefault_EnvVarSet(t *testing.T) {
	// Test getEnvOrDefault() with environment variable set
	t.Setenv("TEST_VAR", "custom_value")

	result := getEnvOrDefault("TEST_VAR", "default_value")

	if result != "custom_value" {
		t.Errorf("expected 'custom_value', got %q", result)
	}
}

func TestGetEnvOrDefault_EnvVarUnset(t *testing.T) {
	// Test getEnvOrDefault() with environment variable unset
	result := getEnvOrDefault("NONEXISTENT_VAR", "default_value")

	if result != "default_value" {
		t.Errorf("expected 'default_value', got %q", result)
	}
}

func TestGetEnvOrDefault_EnvVarEmpty(t *testing.T) {
	// Test getEnvOrDefault() with environment variable set to empty string
	t.Setenv("EMPTY_VAR", "")

	result := getEnvOrDefault("EMPTY_VAR", "default_value")

	if result != "default_value" {
		t.Errorf("expected 'default_value', got %q", result)
	}
}

func TestGetEnvIntOrDefault_ValidInt(t *testing.T) {
	// Test getEnvIntOrDefault() with valid integer
	t.Setenv("TEST_INT", "9000")

	result := getEnvIntOrDefault("TEST_INT", 1234)

	if result != 9000 {
		t.Errorf("expected 9000, got %d", result)
	}
}

func TestGetEnvIntOrDefault_InvalidInt(t *testing.T) {
	// Test getEnvIntOrDefault() with invalid integer (should return default)
	t.Setenv("TEST_INVALID_INT", "not_a_number")

	result := getEnvIntOrDefault("TEST_INVALID_INT", 1234)

	if result != 1234 {
		t.Errorf("expected default value 1234, got %d", result)
	}
}

func TestGetEnvIntOrDefault_EnvVarUnset(t *testing.T) {
	// Test getEnvIntOrDefault() with environment variable unset
	result := getEnvIntOrDefault("NONEXISTENT_INT_VAR", 5678)

	if result != 5678 {
		t.Errorf("expected default value 5678, got %d", result)
	}
}

func TestGetEnvIntOrDefault_EmptyString(t *testing.T) {
	// Test getEnvIntOrDefault() with environment variable set to empty string
	t.Setenv("EMPTY_INT_VAR", "")

	result := getEnvIntOrDefault("EMPTY_INT_VAR", 4321)

	if result != 4321 {
		t.Errorf("expected default value 4321, got %d", result)
	}
}

func TestGetEnvBoolOrDefault_TrueValues(t *testing.T) {
	for _, val := range []string{"true", "1", "yes", "TRUE", "Yes"} {
		t.Setenv("TEST_BOOL", val)
		if !getEnvBoolOrDefault("TEST_BOOL", false) {
			t.Errorf("expected true for %q", val)
		}
	}
}

func TestGetEnvBoolOrDefault_FalseValues(t *testing.T) {
	for _, val := range []string{"false", "0", "no", "FALSE", "No"} {
		t.Setenv("TEST_BOOL", val)
		if getEnvBoolOrDefault("TEST_BOOL", true) {
			t.Errorf("expected false for %q", val)
		}
	}
}

func TestGetEnvBoolOrDefault_Default(t *testing.T) {
	if getEnvBoolOrDefault("NONEXISTENT_BOOL", true) != true {
		t.Error("expected default true")
	}
	if getEnvBoolOrDefault("NONEXISTENT_BOOL", false) != false {
		t.Error("expected default false")
	}
}

func TestLoad_DemoMode(t *testing.T) {
	t.Setenv("DEMO_MODE", "true")
	cfg := Load()
	if !cfg.DemoMode {
		t.Error("expected DemoMode to be true when DEMO_MODE=true")
	}
}
