package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository/memory"
)

func setupImportTestServer(t *testing.T) *api.Server {
	t.Helper()
	cfg := &config.Config{
		Port:      8080,
		LogFormat: "text",
	}
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	return api.NewServer(cfg, eventStore, readStore, nil)
}

const testGedcom = `0 HEAD
1 SOUR Test
1 GEDC
2 VERS 5.5
2 FORM LINEAGE-LINKED
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
1 BIRT
2 DATE 15 JAN 1850
2 PLAC Springfield, IL
1 DEAT
2 DATE 20 MAR 1920
2 PLAC Chicago, IL
0 @I2@ INDI
1 NAME Jane /Smith/
1 SEX F
1 BIRT
2 DATE ABT 1855
2 PLAC Boston, MA
0 @I3@ INDI
1 NAME Junior /Doe/
1 SEX M
1 BIRT
2 DATE 1880
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 CHIL @I3@
1 MARR
2 DATE 10 JUN 1875
2 PLAC Springfield, IL
0 TRLR
`

func TestImportGedcom_Success(t *testing.T) {
	server := setupImportTestServer(t)

	// Create multipart form with GEDCOM file
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "test.ged")
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(part, testGedcom)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Verify counts
	personsImported := int(result["persons_imported"].(float64))
	if personsImported != 3 {
		t.Errorf("Expected 3 persons imported, got %d", personsImported)
	}

	familiesImported := int(result["families_imported"].(float64))
	if familiesImported != 1 {
		t.Errorf("Expected 1 family imported, got %d", familiesImported)
	}

	// Verify success flag
	if result["success"] != true {
		t.Error("Response should include success: true")
	}
}

func TestImportGedcom_NoFile(t *testing.T) {
	server := setupImportTestServer(t)

	// Create empty multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestImportGedcom_InvalidExtension(t *testing.T) {
	server := setupImportTestServer(t)

	// Create multipart form with wrong file extension but valid GEDCOM content.
	// The server validates content, not file extension, so this should succeed.
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "test.txt")
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(part, testGedcom)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	// Server accepts valid GEDCOM content regardless of file extension
	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if result["success"] != true {
		t.Error("Import should succeed with valid GEDCOM content")
	}
}

func TestImportGedcom_VerifyPersonsCreated(t *testing.T) {
	server := setupImportTestServer(t)

	// Import GEDCOM
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.ged")
	io.WriteString(part, testGedcom)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Import failed: %d: %s", rec.Code, rec.Body.String())
	}

	// Verify persons are accessible via search
	req = httptest.NewRequest(http.MethodGet, "/api/v1/search?q=Doe", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Search failed: %d: %s", rec.Code, rec.Body.String())
	}

	var searchResult map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &searchResult)

	total := int(searchResult["total"].(float64))
	if total < 2 {
		t.Errorf("Expected at least 2 persons with surname 'Doe', got %d", total)
	}
}

func TestImportGedcom_VerifyFamiliesCreated(t *testing.T) {
	server := setupImportTestServer(t)

	// Import GEDCOM
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.ged")
	io.WriteString(part, testGedcom)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Import failed: %d: %s", rec.Code, rec.Body.String())
	}

	// Verify families are accessible
	req = httptest.NewRequest(http.MethodGet, "/api/v1/families", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("List families failed: %d: %s", rec.Code, rec.Body.String())
	}

	var listResult map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &listResult)

	families := listResult["items"].([]interface{})
	if len(families) != 1 {
		t.Errorf("Expected 1 family, got %d", len(families))
	}
}

func TestImportGedcom_EmptyFile(t *testing.T) {
	server := setupImportTestServer(t)

	// Create multipart form with empty GEDCOM (just header and trailer)
	emptyGedcom := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 TRLR
`

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "empty.ged")
	io.WriteString(part, emptyGedcom)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	// Empty GEDCOM should fail validation
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusInternalServerError {
		t.Fatalf("Expected status 400 or 500 for empty GEDCOM, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestImportGedcom_MissingNames(t *testing.T) {
	server := setupImportTestServer(t)

	// GEDCOM with individual missing name
	gedcom := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 SEX M
0 TRLR
`

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.ged")
	io.WriteString(part, gedcom)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Import should succeed with warnings, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)

	// Should have warnings about missing name
	warnings := result["warnings"].([]interface{})
	if len(warnings) == 0 {
		t.Error("Expected warnings for missing name")
	}

	// Person should still be imported (with Unknown names)
	personsImported := int(result["persons_imported"].(float64))
	if personsImported != 1 {
		t.Errorf("Expected 1 person imported, got %d", personsImported)
	}
}

func TestImportGedcom_GedExtension(t *testing.T) {
	server := setupImportTestServer(t)

	// Create multipart form with .GED extension (uppercase)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "TEST.GED")
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(part, testGedcom)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200 for .GED extension, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestImportGedcom_NoContentType(t *testing.T) {
	server := setupImportTestServer(t)

	// Request without content-type header
	// Note: The generated strict server returns 500 when multipart parsing fails
	// due to missing Content-Type. Ideally this would be 400, but the generated
	// code doesn't handle this case gracefully.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import", bytes.NewReader([]byte{}))
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	// Server returns 500 when content-type is missing (multipart parsing failure)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("Expected status 500, got %d: %s", rec.Code, rec.Body.String())
	}
}
