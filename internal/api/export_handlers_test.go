package api_test

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository/memory"
)

func setupExportTestServer(t *testing.T) *api.Server {
	t.Helper()
	cfg := &config.Config{
		Port:      8080,
		LogFormat: "text",
	}
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	return api.NewServer(cfg, eventStore, readStore, nil)
}

const exportTestGedcom = `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
1 BIRT
2 DATE 15 JAN 1850
2 PLAC Springfield, IL
0 @I2@ INDI
1 NAME Jane /Smith/
1 SEX F
1 BIRT
2 DATE ABT 1855
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 MARR
2 DATE 10 JUN 1875
0 TRLR
`

func TestExportGedcom_Empty(t *testing.T) {
	server := setupExportTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/gedcom/export", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Check content type
	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/x-gedcom" {
		t.Errorf("Content-Type = %s, want application/x-gedcom", contentType)
	}

	// Check content disposition
	contentDisposition := rec.Header().Get("Content-Disposition")
	if !strings.Contains(contentDisposition, "attachment") {
		t.Errorf("Content-Disposition should contain 'attachment'")
	}

	// Even empty should have valid GEDCOM structure
	body := rec.Body.String()
	if !strings.HasPrefix(body, "0 HEAD\n") {
		t.Error("Export should start with GEDCOM header")
	}
	if !strings.HasSuffix(body, "0 TRLR\n") {
		t.Error("Export should end with TRLR")
	}
}

func TestExportGedcom_WithData(t *testing.T) {
	server := setupExportTestServer(t)

	// Import some data first
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.ged")
	io.WriteString(part, exportTestGedcom)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Import failed: %d: %s", rec.Code, rec.Body.String())
	}

	// Now export
	req = httptest.NewRequest(http.MethodGet, "/api/v1/gedcom/export", nil)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Export failed: %d: %s", rec.Code, rec.Body.String())
	}

	output := rec.Body.String()

	// Should have header
	if !strings.HasPrefix(output, "0 HEAD\n") {
		t.Error("Export should start with header")
	}

	// Should have GEDC version
	if !strings.Contains(output, "2 VERS 5.5\n") {
		t.Error("Export should contain GEDCOM version")
	}

	// Should have individuals
	if !strings.Contains(output, "INDI\n") {
		t.Error("Export should contain individuals")
	}
	if !strings.Contains(output, "1 NAME John /Doe/\n") {
		t.Error("Export should contain John Doe")
	}
	if !strings.Contains(output, "1 NAME Jane /Smith/\n") {
		t.Error("Export should contain Jane Smith")
	}

	// Should have family
	if !strings.Contains(output, "FAM\n") {
		t.Error("Export should contain family")
	}
	if !strings.Contains(output, "1 HUSB @I") {
		t.Error("Export should contain HUSB reference")
	}
	if !strings.Contains(output, "1 WIFE @I") {
		t.Error("Export should contain WIFE reference")
	}

	// Should have marriage
	if !strings.Contains(output, "1 MARR\n") {
		t.Error("Export should contain marriage event")
	}
	if !strings.Contains(output, "2 DATE 10 JUN 1875\n") {
		t.Error("Export should contain marriage date")
	}

	// Should have trailer
	if !strings.HasSuffix(output, "0 TRLR\n") {
		t.Error("Export should end with trailer")
	}
}

func TestExportGedcom_RoundTrip(t *testing.T) {
	server := setupExportTestServer(t)

	// Import data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.ged")
	io.WriteString(part, exportTestGedcom)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	// Export data
	req = httptest.NewRequest(http.MethodGet, "/api/v1/gedcom/export", nil)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	output := rec.Body.String()

	// Key data should survive round-trip
	keyData := []string{
		"John /Doe/",
		"Jane /Smith/",
		"1 SEX M",
		"1 SEX F",
		"15 JAN 1850",
		"ABT 1855",
		"Springfield, IL",
		"10 JUN 1875",
	}

	for _, data := range keyData {
		if !strings.Contains(output, data) {
			t.Errorf("Round-trip should preserve: %s", data)
		}
	}
}
