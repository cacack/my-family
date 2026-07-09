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
	snapshotStore := memory.NewSnapshotStore(eventStore)
	readStore := memory.NewReadModelStore()
	return api.NewServer(cfg, eventStore, readStore, snapshotStore, nil)
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
	if !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("Content-Type = %s, want application/json", contentType)
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
	if !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("Content-Type = %s, want application/json", contentType)
	}

	// Verify valid JSON object with persons array and total
	var result struct {
		Persons []map[string]interface{} `json:"persons"`
		Total   int                      `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	if len(result.Persons) != 2 {
		t.Errorf("Expected 2 persons, got %d", len(result.Persons))
	}
	if result.Total != 2 {
		t.Errorf("Expected total 2, got %d", result.Total)
	}
}

func TestExportPersons_WithAllFields(t *testing.T) {
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

	// Export persons
	req = httptest.NewRequest(http.MethodGet, "/api/v1/export/persons", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	// Verify JSON contains all expected fields
	var result struct {
		Persons []map[string]interface{} `json:"persons"`
		Total   int                      `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	if len(result.Persons) > 0 {
		person := result.Persons[0]
		if _, ok := person["id"]; !ok {
			t.Error("Person should have id field")
		}
		if _, ok := person["given_name"]; !ok {
			t.Error("Person should have given_name field")
		}
		if _, ok := person["surname"]; !ok {
			t.Error("Person should have surname field")
		}
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
	req = httptest.NewRequest(http.MethodGet, "/api/v1/export/families", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Check Content-Type
	contentType := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("Content-Type = %s, want application/json", contentType)
	}

	// Verify valid JSON object with families array and total
	var result struct {
		Families []map[string]interface{} `json:"families"`
		Total    int                      `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	if len(result.Families) != 1 {
		t.Errorf("Expected 1 family, got %d", len(result.Families))
	}
	if result.Total != 1 {
		t.Errorf("Expected total 1, got %d", result.Total)
	}
}

func TestExportPersons_Empty(t *testing.T) {
	server := setupExportTestServer(t)

	// Export persons without importing data (empty database)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/persons", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	// Verify JSON object with empty persons array
	var result struct {
		Persons []map[string]interface{} `json:"persons"`
		Total   int                      `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	if len(result.Persons) != 0 {
		t.Errorf("Expected empty array, got %d items", len(result.Persons))
	}
	if result.Total != 0 {
		t.Errorf("Expected total 0, got %d", result.Total)
	}
}

func TestExportFamilies_Empty(t *testing.T) {
	server := setupExportTestServer(t)

	// Export families without importing data
	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/families", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	// Verify JSON object with empty families array
	var result struct {
		Families []map[string]interface{} `json:"families"`
		Total    int                      `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	if len(result.Families) != 0 {
		t.Errorf("Expected empty array, got %d items", len(result.Families))
	}
	if result.Total != 0 {
		t.Errorf("Expected total 0, got %d", result.Total)
	}
}

// Export estimate endpoint tests

func TestGetExportEstimate_Empty(t *testing.T) {
	server := setupExportTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/estimate", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Check Content-Type
	contentType := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("Content-Type = %s, want application/json", contentType)
	}

	// Verify valid JSON structure
	var result struct {
		PersonCount    int   `json:"person_count"`
		FamilyCount    int   `json:"family_count"`
		SourceCount    int   `json:"source_count"`
		CitationCount  int   `json:"citation_count"`
		EventCount     int   `json:"event_count"`
		NoteCount      int   `json:"note_count"`
		TotalRecords   int   `json:"total_records"`
		EstimatedBytes int64 `json:"estimated_bytes"`
		IsLargeExport  bool  `json:"is_large_export"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	// Empty export should have zero counts
	if result.PersonCount != 0 {
		t.Errorf("PersonCount = %d, want 0", result.PersonCount)
	}
	if result.FamilyCount != 0 {
		t.Errorf("FamilyCount = %d, want 0", result.FamilyCount)
	}
	if result.TotalRecords != 0 {
		t.Errorf("TotalRecords = %d, want 0", result.TotalRecords)
	}
	if result.IsLargeExport {
		t.Error("IsLargeExport = true, want false for empty database")
	}
}

func TestGetExportEstimate_WithData(t *testing.T) {
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

	// Now get estimate
	req = httptest.NewRequest(http.MethodGet, "/api/v1/export/estimate", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		PersonCount    int   `json:"person_count"`
		FamilyCount    int   `json:"family_count"`
		SourceCount    int   `json:"source_count"`
		TotalRecords   int   `json:"total_records"`
		EstimatedBytes int64 `json:"estimated_bytes"`
		IsLargeExport  bool  `json:"is_large_export"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	// Should have persons and families from imported data
	if result.PersonCount != 2 {
		t.Errorf("PersonCount = %d, want 2", result.PersonCount)
	}
	if result.FamilyCount != 1 {
		t.Errorf("FamilyCount = %d, want 1", result.FamilyCount)
	}
	if result.TotalRecords < 3 {
		t.Errorf("TotalRecords = %d, want >= 3", result.TotalRecords)
	}
	if result.EstimatedBytes <= 0 {
		t.Errorf("EstimatedBytes = %d, want > 0", result.EstimatedBytes)
	}
	// Small test data should not be large export
	if result.IsLargeExport {
		t.Error("IsLargeExport = true, want false for small data")
	}
}

func TestExportGedcom_VersionParam(t *testing.T) {
	server := setupExportTestServer(t)

	// Import some data first.
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

	tests := []struct {
		name    string
		query   string
		wantTag string
	}{
		{name: "explicit 5.5.1", query: "?version=5.5.1", wantTag: "2 VERS 5.5.1\n"},
		{name: "explicit 7.0", query: "?version=7.0", wantTag: "2 VERS 7.0\n"},
		{name: "default", query: "", wantTag: "2 VERS 5.5\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/gedcom/export"+tt.query, http.NoBody)
			rec := httptest.NewRecorder()
			server.Echo().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("Export failed: %d: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.wantTag) {
				t.Errorf("output should contain %q; got:\n%s", tt.wantTag, rec.Body.String())
			}
		})
	}
}

// Uses a non-FamilySearch EXID so a 5.5.x downgrade is genuinely lossy: a
// FamilySearch ARK would be mapped to _FSFTID and report no loss (issue #599).
const exportPreviewGedcom = `0 HEAD
1 GEDC
2 VERS 7.0
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Test /Person/
1 EXID 12345678
2 TYPE https://www.findagrave.com
0 TRLR
`

func TestPreviewGedcomExport(t *testing.T) {
	server := setupExportTestServer(t)

	// Import data carrying a 7.0-only feature (EXID).
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.ged")
	io.WriteString(part, exportPreviewGedcom)
	writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Import failed: %d: %s", rec.Code, rec.Body.String())
	}

	// Downgrade preview to 5.5.1 reports EXID data loss without producing a file.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/gedcom/export/preview?version=5.5.1", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Preview failed: %d: %s", rec.Code, rec.Body.String())
	}
	var preview api.ExportPreview
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	if preview.SourceVersion != "7.0" || preview.TargetVersion != "5.5.1" {
		t.Errorf("source=%q target=%q, want 7.0/5.5.1", preview.SourceVersion, preview.TargetVersion)
	}
	if !preview.HasDataLoss || len(preview.DataLoss) == 0 {
		t.Fatalf("expected data loss downgrading EXID; got %+v", preview)
	}
	foundEXID := false
	for _, d := range preview.DataLoss {
		if strings.Contains(d.Feature, "EXID") {
			foundEXID = true
			// affectedRecords must carry the XREF of the affected individual.
			if d.AffectedRecords == nil || len(*d.AffectedRecords) == 0 {
				t.Errorf("EXID data loss should list affectedRecords; got %+v", d)
			} else if (*d.AffectedRecords)[0] != "@I1@" {
				t.Errorf("affectedRecords[0] = %q, want @I1@", (*d.AffectedRecords)[0])
			}
		}
	}
	if !foundEXID {
		t.Errorf("data loss should mention EXID; got %+v", preview.DataLoss)
	}

	// Omitting the version defaults to 5.5 but auto-upgrades to 7.0 because the
	// data requires it, so there is no downgrade and no data loss.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/gedcom/export/preview", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Preview default failed: %d: %s", rec.Code, rec.Body.String())
	}
	var previewDefault api.ExportPreview
	if err := json.Unmarshal(rec.Body.Bytes(), &previewDefault); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if previewDefault.TargetVersion != "7.0" || previewDefault.HasDataLoss {
		t.Errorf("default preview: target=%q hasDataLoss=%v, want 7.0 / false", previewDefault.TargetVersion, previewDefault.HasDataLoss)
	}

	// Preview at 7.0 keeps everything; no loss.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/gedcom/export/preview?version=7.0", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Preview 7.0 failed: %d: %s", rec.Code, rec.Body.String())
	}
	var preview70 api.ExportPreview
	if err := json.Unmarshal(rec.Body.Bytes(), &preview70); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if preview70.HasDataLoss {
		t.Errorf("7.0 preview should have no data loss; got %+v", preview70.DataLoss)
	}
}

func TestPreviewGedcomExport_InvalidVersion(t *testing.T) {
	server := setupExportTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gedcom/export/preview?version=9.9", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400; got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestExportGedcom_InvalidVersion(t *testing.T) {
	server := setupExportTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/gedcom/export?version=9.9", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}

	var errResp struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}
	if errResp.Code != "invalid_version" {
		t.Errorf("Code = %q, want invalid_version", errResp.Code)
	}
}
