package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository/memory"
)

func setupMiddlewareTestServer() *api.Server {
	cfg := &config.Config{
		Port:      8080,
		LogFormat: "text",
	}
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	return api.NewServer(cfg, eventStore, readStore, nil)
}

func TestErrorHandler_NotFound(t *testing.T) {
	server := setupMiddlewareTestServer()

	// Request non-existent route
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nonexistent", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse error response: %v", err)
	}

	if resp["code"] == nil {
		t.Error("Error response should have code field")
	}
	if resp["message"] == nil {
		t.Error("Error response should have message field")
	}
}

func TestErrorHandler_MethodNotAllowed(t *testing.T) {
	server := setupMiddlewareTestServer()

	// PATCH is not allowed on /persons
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/persons", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestNewAPIError(t *testing.T) {
	err := api.NewAPIError("TEST_CODE", "test message")
	if err.Code != "TEST_CODE" {
		t.Errorf("Code = %s, want TEST_CODE", err.Code)
	}
	if err.Message != "test message" {
		t.Errorf("Message = %s, want test message", err.Message)
	}
}

func TestWithDetails(t *testing.T) {
	err := api.NewAPIError("TEST_CODE", "test message")
	details := map[string]any{"key": "value"}
	err = err.WithDetails(details)

	if err.Details == nil {
		t.Fatal("Details should not be nil")
	}
	if err.Details["key"] != "value" {
		t.Errorf("Details[key] = %v, want value", err.Details["key"])
	}
}

func TestServerWithJSONLogging(t *testing.T) {
	cfg := &config.Config{
		Port:      8080,
		LogFormat: "json",
	}
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	server := api.NewServer(cfg, eventStore, readStore, nil)

	// Make a request to ensure JSON logging is configured
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestCORSHeaders(t *testing.T) {
	server := setupMiddlewareTestServer()

	// OPTIONS request to check CORS
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/persons", http.NoBody)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	// Check CORS headers are present
	if rec.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Error("Expected Access-Control-Allow-Origin header")
	}
}
