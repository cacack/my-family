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

func setupAhnentafelTestServer(t *testing.T) *api.Server {
	t.Helper()
	cfg := &config.Config{
		Port:      8080,
		LogFormat: "text",
	}
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	return api.NewServer(cfg, eventStore, readStore, nil)
}

// GEDCOM with 3 generations of ancestors
const ahnentafelTestGedcom = `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Junior /Smith/
1 SEX M
1 BIRT
2 DATE 1 JAN 2000
2 PLAC New York
0 @I2@ INDI
1 NAME John /Smith/
1 SEX M
1 BIRT
2 DATE 1 JAN 1970
2 PLAC Boston
1 DEAT
2 DATE 1 JAN 2020
2 PLAC New York
0 @I3@ INDI
1 NAME Jane /Doe/
1 SEX F
1 BIRT
2 DATE 1 JAN 1975
2 PLAC Chicago
0 @I4@ INDI
1 NAME George /Smith/
1 SEX M
1 BIRT
2 DATE 1 JAN 1940
2 PLAC Philadelphia
0 @I5@ INDI
1 NAME Mary /Jones/
1 SEX F
1 BIRT
2 DATE 1 JAN 1945
2 PLAC Los Angeles
0 @F1@ FAM
1 HUSB @I2@
1 WIFE @I3@
1 CHIL @I1@
0 @F2@ FAM
1 HUSB @I4@
1 WIFE @I5@
1 CHIL @I2@
0 TRLR
`

func importAhnentafelTestData(t *testing.T, server *api.Server) string {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.ged")
	io.WriteString(part, ahnentafelTestGedcom)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Import failed: %d: %s", rec.Code, rec.Body.String())
	}

	// Get Junior's ID from search
	req = httptest.NewRequest(http.MethodGet, "/api/v1/search?q=Junior", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	var searchResult struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	json.Unmarshal(rec.Body.Bytes(), &searchResult)

	if len(searchResult.Items) == 0 {
		t.Fatal("Could not find Junior in search results")
	}

	return searchResult.Items[0].ID
}

func TestGetAhnentafel_JSONFormat_Success(t *testing.T) {
	server := setupAhnentafelTestServer(t)
	juniorID := importAhnentafelTestData(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ahnentafel/"+juniorID, http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify content type
	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Content-Type = %v, want application/json", contentType)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Verify subject
	subject := result["subject"].(map[string]interface{})
	if subject["given_name"] != "Junior" {
		t.Errorf("subject.given_name = %v, want Junior", subject["given_name"])
	}
	if subject["surname"] != "Smith" {
		t.Errorf("subject.surname = %v, want Smith", subject["surname"])
	}

	// Verify entries
	entries := result["entries"].([]interface{})
	if len(entries) != 5 {
		t.Errorf("len(entries) = %d, want 5", len(entries))
	}

	// Verify first entry (subject, number 1)
	entry1 := entries[0].(map[string]interface{})
	if int(entry1["number"].(float64)) != 1 {
		t.Errorf("entry[0].number = %v, want 1", entry1["number"])
	}
	if entry1["given_name"] != "Junior" {
		t.Errorf("entry[0].given_name = %v, want Junior", entry1["given_name"])
	}
	if int(entry1["generation"].(float64)) != 0 {
		t.Errorf("entry[0].generation = %v, want 0", entry1["generation"])
	}

	// Verify father entry (number 2)
	entry2 := entries[1].(map[string]interface{})
	if int(entry2["number"].(float64)) != 2 {
		t.Errorf("entry[1].number = %v, want 2", entry2["number"])
	}
	if entry2["given_name"] != "John" {
		t.Errorf("entry[1].given_name = %v, want John", entry2["given_name"])
	}
	if int(entry2["generation"].(float64)) != 1 {
		t.Errorf("entry[1].generation = %v, want 1", entry2["generation"])
	}

	// Verify mother entry (number 3)
	entry3 := entries[2].(map[string]interface{})
	if int(entry3["number"].(float64)) != 3 {
		t.Errorf("entry[2].number = %v, want 3", entry3["number"])
	}
	if entry3["given_name"] != "Jane" {
		t.Errorf("entry[2].given_name = %v, want Jane", entry3["given_name"])
	}

	// Verify paternal grandfather (number 4)
	entry4 := entries[3].(map[string]interface{})
	if int(entry4["number"].(float64)) != 4 {
		t.Errorf("entry[3].number = %v, want 4", entry4["number"])
	}
	if entry4["given_name"] != "George" {
		t.Errorf("entry[3].given_name = %v, want George", entry4["given_name"])
	}
	if int(entry4["generation"].(float64)) != 2 {
		t.Errorf("entry[3].generation = %v, want 2", entry4["generation"])
	}

	// Verify paternal grandmother (number 5)
	entry5 := entries[4].(map[string]interface{})
	if int(entry5["number"].(float64)) != 5 {
		t.Errorf("entry[4].number = %v, want 5", entry5["number"])
	}
	if entry5["given_name"] != "Mary" {
		t.Errorf("entry[4].given_name = %v, want Mary", entry5["given_name"])
	}

	// Verify counts
	totalCount := int(result["total_count"].(float64))
	if totalCount != 5 {
		t.Errorf("total_count = %d, want 5", totalCount)
	}

	generations := int(result["generations"].(float64))
	if generations != 2 {
		t.Errorf("generations = %d, want 2", generations)
	}

	knownCount := int(result["known_count"].(float64))
	if knownCount != 5 {
		t.Errorf("known_count = %d, want 5", knownCount)
	}
}

func TestGetAhnentafel_TextFormat_Success(t *testing.T) {
	server := setupAhnentafelTestServer(t)
	juniorID := importAhnentafelTestData(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ahnentafel/"+juniorID+"?format=text", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify content type
	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") {
		t.Errorf("Content-Type = %v, want text/plain", contentType)
	}

	body := rec.Body.String()

	// Verify header
	if !strings.Contains(body, "AHNENTAFEL REPORT") {
		t.Error("Text output should contain 'AHNENTAFEL REPORT' header")
	}
	if !strings.Contains(body, "Subject: Junior Smith") {
		t.Error("Text output should contain 'Subject: Junior Smith'")
	}

	// Verify entries are present
	if !strings.Contains(body, "1. Junior Smith") {
		t.Error("Text output should contain '1. Junior Smith'")
	}
	if !strings.Contains(body, "2. John Smith (Father)") {
		t.Error("Text output should contain '2. John Smith (Father)'")
	}
	if !strings.Contains(body, "3. Jane Doe (Mother)") {
		t.Error("Text output should contain '3. Jane Doe (Mother)'")
	}
	if !strings.Contains(body, "4. George Smith (Father's Father)") {
		t.Error("Text output should contain '4. George Smith (Father's Father)'")
	}
	if !strings.Contains(body, "5. Mary Jones (Father's Mother)") {
		t.Error("Text output should contain '5. Mary Jones (Father's Mother)'")
	}

	// Verify birth/death info is present
	if !strings.Contains(body, "b.") {
		t.Error("Text output should contain birth info")
	}
	if !strings.Contains(body, "d.") {
		t.Error("Text output should contain death info")
	}

	// Verify footer
	if !strings.Contains(body, "Generated:") {
		t.Error("Text output should contain 'Generated:' footer")
	}
	if !strings.Contains(body, "Total ancestors: 5") {
		t.Error("Text output should contain 'Total ancestors: 5'")
	}
	if !strings.Contains(body, "Generations: 2") {
		t.Error("Text output should contain 'Generations: 2'")
	}
}

func TestGetAhnentafel_NotFound(t *testing.T) {
	server := setupAhnentafelTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ahnentafel/00000000-0000-0000-0000-000000000001", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("Expected status 404, got %d", rec.Code)
	}
}

func TestGetAhnentafel_InvalidID(t *testing.T) {
	server := setupAhnentafelTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ahnentafel/not-a-uuid", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", rec.Code)
	}
}

func TestGetAhnentafel_InvalidFormat(t *testing.T) {
	server := setupAhnentafelTestServer(t)
	juniorID := importAhnentafelTestData(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ahnentafel/"+juniorID+"?format=xml", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify error message
	if !strings.Contains(rec.Body.String(), "Invalid format") {
		t.Error("Error response should mention invalid format")
	}
}

func TestGetAhnentafel_MaxGenerations(t *testing.T) {
	server := setupAhnentafelTestServer(t)
	juniorID := importAhnentafelTestData(t, server)

	// Request only 1 generation
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ahnentafel/"+juniorID+"?generations=1", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)

	entries := result["entries"].([]interface{})
	// Should have 3 entries: subject (1), father (2), mother (3)
	if len(entries) != 3 {
		t.Errorf("len(entries) = %d, want 3 (subject + parents only)", len(entries))
	}

	// Generations should be 1
	generations := int(result["generations"].(float64))
	if generations != 1 {
		t.Errorf("generations = %d, want 1", generations)
	}
}

func TestGetAhnentafel_GenerationsLimit(t *testing.T) {
	server := setupAhnentafelTestServer(t)
	juniorID := importAhnentafelTestData(t, server)

	// Request 15 generations (should be capped at 10)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ahnentafel/"+juniorID+"?generations=15", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Should still return data (just limited by actual ancestors available)
	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)

	entries := result["entries"].([]interface{})
	if len(entries) == 0 {
		t.Error("Should return at least some entries")
	}
}

func TestGetAhnentafel_NoAncestors(t *testing.T) {
	server := setupAhnentafelTestServer(t)

	// Create a person with no ancestors
	body := `{"given_name":"Orphan","surname":"Child"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	var created map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &created)
	personID := created["id"].(string)

	// Get Ahnentafel for person with no ancestors
	req = httptest.NewRequest(http.MethodGet, "/api/v1/ahnentafel/"+personID, http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)

	// Verify subject
	subject := result["subject"].(map[string]interface{})
	if subject["given_name"] != "Orphan" {
		t.Errorf("subject.given_name = %v, want Orphan", subject["given_name"])
	}

	// Should have only 1 entry (the subject)
	entries := result["entries"].([]interface{})
	if len(entries) != 1 {
		t.Errorf("len(entries) = %d, want 1", len(entries))
	}

	totalCount := int(result["total_count"].(float64))
	if totalCount != 1 {
		t.Errorf("total_count = %d, want 1", totalCount)
	}

	generations := int(result["generations"].(float64))
	if generations != 0 {
		t.Errorf("generations = %d, want 0", generations)
	}
}

func TestGetAhnentafel_TextFormat_NoAncestors(t *testing.T) {
	server := setupAhnentafelTestServer(t)

	// Create a person with no ancestors
	body := `{"given_name":"Lonely","surname":"Person"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	var created map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &created)
	personID := created["id"].(string)

	// Get Ahnentafel in text format
	req = httptest.NewRequest(http.MethodGet, "/api/v1/ahnentafel/"+personID+"?format=text", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	body = rec.Body.String()

	// Verify header
	if !strings.Contains(body, "Subject: Lonely Person") {
		t.Error("Text output should contain 'Subject: Lonely Person'")
	}

	// Verify only subject entry
	if !strings.Contains(body, "1. Lonely Person") {
		t.Error("Text output should contain '1. Lonely Person'")
	}

	// Verify footer
	if !strings.Contains(body, "Total ancestors: 1") {
		t.Error("Text output should contain 'Total ancestors: 1'")
	}
	if !strings.Contains(body, "Generations: 0") {
		t.Error("Text output should contain 'Generations: 0'")
	}
}

func TestGetAhnentafel_EntryHasBirthAndDeath(t *testing.T) {
	server := setupAhnentafelTestServer(t)
	juniorID := importAhnentafelTestData(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ahnentafel/"+juniorID, http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)

	entries := result["entries"].([]interface{})

	// Check father entry (John Smith) has both birth and death dates
	entry2 := entries[1].(map[string]interface{})
	if entry2["birth_date"] == nil {
		t.Error("Father entry should have birth_date")
	}
	if entry2["death_date"] == nil {
		t.Error("Father entry should have death_date")
	}
	if entry2["birth_place"] == nil {
		t.Error("Father entry should have birth_place")
	}
	if entry2["death_place"] == nil {
		t.Error("Father entry should have death_place")
	}
}

func TestGetAhnentafel_DefaultGenerations(t *testing.T) {
	server := setupAhnentafelTestServer(t)
	juniorID := importAhnentafelTestData(t, server)

	// Request without specifying generations (should use default 5)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ahnentafel/"+juniorID, http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Test passes if we get a valid response with default generations
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Just verify we got a valid structure
	if result["entries"] == nil {
		t.Error("Response should have entries")
	}
	if result["total_count"] == nil {
		t.Error("Response should have total_count")
	}
}

func TestGetAhnentafel_ExplicitJSONFormat(t *testing.T) {
	server := setupAhnentafelTestServer(t)
	juniorID := importAhnentafelTestData(t, server)

	// Explicitly request JSON format
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ahnentafel/"+juniorID+"?format=json", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify content type
	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Content-Type = %v, want application/json", contentType)
	}

	// Verify it's valid JSON
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response as JSON: %v", err)
	}
}
