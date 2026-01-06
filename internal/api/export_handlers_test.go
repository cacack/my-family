package api_test

import (
	"bytes"
	"encoding/json"
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/gedcom/export", http.NoBody)
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
	req = httptest.NewRequest(http.MethodGet, "/api/v1/gedcom/export", http.NoBody)
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
	req = httptest.NewRequest(http.MethodGet, "/api/v1/gedcom/export", http.NoBody)
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

// JSON/CSV Export API Tests

func TestExportTree_Empty(t *testing.T) {
	server := setupExportTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/tree", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Check Content-Type
	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", contentType)
	}

	// Check Content-Disposition
	contentDisposition := rec.Header().Get("Content-Disposition")
	if !strings.Contains(contentDisposition, "attachment") {
		t.Error("Content-Disposition should contain 'attachment'")
	}

	// Verify valid JSON structure
	var data map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	// Empty export should have empty arrays
	persons, ok := data["persons"].([]interface{})
	if !ok {
		t.Fatal("Expected 'persons' array in response")
	}
	if len(persons) != 0 {
		t.Errorf("Expected empty persons array, got %d items", len(persons))
	}

	families, ok := data["families"].([]interface{})
	if !ok {
		t.Fatal("Expected 'families' array in response")
	}
	if len(families) != 0 {
		t.Errorf("Expected empty families array, got %d items", len(families))
	}
}

func TestExportTree_WithData(t *testing.T) {
	server := setupExportTestServer(t)

	// Import data first
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

	// Export tree as JSON
	req = httptest.NewRequest(http.MethodGet, "/api/v1/export/tree", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify JSON structure with data
	var data map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	persons := data["persons"].([]interface{})
	if len(persons) != 2 {
		t.Errorf("Expected 2 persons, got %d", len(persons))
	}

	families := data["families"].([]interface{})
	if len(families) != 1 {
		t.Errorf("Expected 1 family, got %d", len(families))
	}
}

func TestExportPersons_JSON(t *testing.T) {
	server := setupExportTestServer(t)

	// Import data first
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.ged")
	io.WriteString(part, exportTestGedcom)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	// Export persons as JSON (default format)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/export/persons", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Check Content-Type
	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", contentType)
	}

	// Verify valid JSON array
	var persons []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &persons); err != nil {
		t.Fatalf("Invalid JSON array: %v", err)
	}

	if len(persons) != 2 {
		t.Errorf("Expected 2 persons, got %d", len(persons))
	}
}

func TestExportPersons_JSON_Explicit(t *testing.T) {
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

	// Export with explicit format=json
	req = httptest.NewRequest(http.MethodGet, "/api/v1/export/persons?format=json", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", contentType)
	}
}

func TestExportPersons_CSV(t *testing.T) {
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

	// Export as CSV
	req = httptest.NewRequest(http.MethodGet, "/api/v1/export/persons?format=csv", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Check Content-Type
	contentType := rec.Header().Get("Content-Type")
	if contentType != "text/csv" {
		t.Errorf("Content-Type = %s, want text/csv", contentType)
	}

	// Check Content-Disposition
	contentDisposition := rec.Header().Get("Content-Disposition")
	if !strings.Contains(contentDisposition, ".csv") {
		t.Error("Content-Disposition should contain .csv filename")
	}

	// Verify CSV structure
	output := rec.Body.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		t.Error("CSV should have header and at least one data row")
	}

	// Check header contains expected fields
	header := lines[0]
	if !strings.Contains(header, "id") {
		t.Error("CSV header should contain 'id'")
	}
	if !strings.Contains(header, "given_name") {
		t.Error("CSV header should contain 'given_name'")
	}
	if !strings.Contains(header, "surname") {
		t.Error("CSV header should contain 'surname'")
	}
}

func TestExportPersons_CSV_WithFields(t *testing.T) {
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

	// Export with specific fields
	req = httptest.NewRequest(http.MethodGet, "/api/v1/export/persons?format=csv&fields=id,surname,given_name", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify CSV has only requested fields
	output := rec.Body.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	header := lines[0]

	// Should have exactly 3 columns
	columns := strings.Split(header, ",")
	if len(columns) != 3 {
		t.Errorf("Expected 3 columns, got %d: %s", len(columns), header)
	}

	// Header should match requested order
	if columns[0] != "id" || columns[1] != "surname" || columns[2] != "given_name" {
		t.Errorf("Unexpected column order: %v", columns)
	}

	// Data should not contain fields we didn't request
	if strings.Contains(output, "birth_date") {
		t.Error("CSV should not contain unrequested field 'birth_date'")
	}
}

func TestExportPersons_InvalidFormat(t *testing.T) {
	server := setupExportTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/persons?format=xml", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rec.Code)
	}

	// Check error message
	body := rec.Body.String()
	if !strings.Contains(body, "Invalid format") {
		t.Error("Error should mention invalid format")
	}
}

func TestExportPersons_InvalidField(t *testing.T) {
	server := setupExportTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/persons?format=csv&fields=id,invalid_field_name", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rec.Code)
	}

	// Check error message
	body := rec.Body.String()
	if !strings.Contains(body, "Invalid field") {
		t.Error("Error should mention invalid field")
	}
	if !strings.Contains(body, "invalid_field_name") {
		t.Error("Error should mention the invalid field name")
	}
}

func TestExportFamilies_JSON(t *testing.T) {
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

	// Export families as JSON
	req = httptest.NewRequest(http.MethodGet, "/api/v1/export/families?format=json", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Check Content-Type
	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", contentType)
	}

	// Verify valid JSON array
	var families []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &families); err != nil {
		t.Fatalf("Invalid JSON array: %v", err)
	}

	if len(families) != 1 {
		t.Errorf("Expected 1 family, got %d", len(families))
	}
}

func TestExportFamilies_CSV(t *testing.T) {
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

	// Export as CSV
	req = httptest.NewRequest(http.MethodGet, "/api/v1/export/families?format=csv", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Check Content-Type
	contentType := rec.Header().Get("Content-Type")
	if contentType != "text/csv" {
		t.Errorf("Content-Type = %s, want text/csv", contentType)
	}

	// Verify CSV structure
	output := rec.Body.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		t.Error("CSV should have header and at least one data row")
	}

	// Check header contains expected fields
	header := lines[0]
	if !strings.Contains(header, "id") {
		t.Error("CSV header should contain 'id'")
	}
	if !strings.Contains(header, "partner1_name") {
		t.Error("CSV header should contain 'partner1_name'")
	}
}

func TestExportFamilies_CSV_WithFields(t *testing.T) {
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

	// Export with specific fields
	req = httptest.NewRequest(http.MethodGet, "/api/v1/export/families?format=csv&fields=id,partner1_name,partner2_name", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify CSV has only requested fields
	output := rec.Body.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	header := lines[0]

	// Should have exactly 3 columns
	columns := strings.Split(header, ",")
	if len(columns) != 3 {
		t.Errorf("Expected 3 columns, got %d: %s", len(columns), header)
	}
}

func TestExportFamilies_InvalidFormat(t *testing.T) {
	server := setupExportTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/families?format=yaml", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rec.Code)
	}
}

func TestExportFamilies_InvalidField(t *testing.T) {
	server := setupExportTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/families?format=csv&fields=id,nonexistent_field", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rec.Code)
	}

	// Check error message
	body := rec.Body.String()
	if !strings.Contains(body, "Invalid field") {
		t.Error("Error should mention invalid field")
	}
}

func TestExportPersons_Empty(t *testing.T) {
	server := setupExportTestServer(t)

	// Export persons without importing data (empty database)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/persons?format=json", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	// Verify empty JSON array
	var persons []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &persons); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	if len(persons) != 0 {
		t.Errorf("Expected empty array, got %d items", len(persons))
	}
}

func TestExportPersons_CSV_Empty(t *testing.T) {
	server := setupExportTestServer(t)

	// Export as CSV without data
	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/persons?format=csv", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	// Verify CSV has header only
	output := rec.Body.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		t.Errorf("Expected 1 line (header only), got %d", len(lines))
	}

	// Header should still contain expected fields
	if !strings.Contains(lines[0], "id") {
		t.Error("CSV header should contain 'id' even when empty")
	}
}

func TestExportFamilies_Empty(t *testing.T) {
	server := setupExportTestServer(t)

	// Export families without importing data
	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/families?format=json", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	// Verify empty JSON array
	var families []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &families); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	if len(families) != 0 {
		t.Errorf("Expected empty array, got %d items", len(families))
	}
}

func TestExportFamilies_CSV_Empty(t *testing.T) {
	server := setupExportTestServer(t)

	// Export as CSV without data
	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/families?format=csv", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	// Verify CSV has header only
	output := rec.Body.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		t.Errorf("Expected 1 line (header only), got %d", len(lines))
	}
}

func TestExportPersons_FieldsOnlyApplyToCSV(t *testing.T) {
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

	// Fields parameter should be ignored for JSON format
	req = httptest.NewRequest(http.MethodGet, "/api/v1/export/persons?format=json&fields=id,surname", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	// JSON should contain all fields regardless of fields parameter
	var persons []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &persons); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	if len(persons) > 0 {
		// Should have more fields than just id and surname
		if _, ok := persons[0]["given_name"]; !ok {
			t.Error("JSON export should contain all fields, including given_name")
		}
	}
}
