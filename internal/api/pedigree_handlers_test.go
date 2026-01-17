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

func setupPedigreeTestServer(t *testing.T) *api.Server {
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

// GEDCOM with 3 generations of ancestors
const pedigreeTestGedcom = `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Junior /Smith/
1 SEX M
1 BIRT
2 DATE 1 JAN 2000
0 @I2@ INDI
1 NAME John /Smith/
1 SEX M
1 BIRT
2 DATE 1 JAN 1970
0 @I3@ INDI
1 NAME Jane /Doe/
1 SEX F
1 BIRT
2 DATE 1 JAN 1975
0 @I4@ INDI
1 NAME George /Smith/
1 SEX M
1 BIRT
2 DATE 1 JAN 1940
0 @I5@ INDI
1 NAME Mary /Jones/
1 SEX F
1 BIRT
2 DATE 1 JAN 1945
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

func importPedigreeTestData(t *testing.T, server *api.Server) string {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.ged")
	io.WriteString(part, pedigreeTestGedcom)
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

func TestGetPedigree_Success(t *testing.T) {
	server := setupPedigreeTestServer(t)
	juniorID := importPedigreeTestData(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pedigree/"+juniorID, http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Verify root
	root := result["root"].(map[string]interface{})
	if root["given_name"] != "Junior" {
		t.Errorf("Root given_name = %v, want Junior", root["given_name"])
	}
	if root["surname"] != "Smith" {
		t.Errorf("Root surname = %v, want Smith", root["surname"])
	}
	if int(root["generation"].(float64)) != 0 {
		t.Errorf("Root generation = %v, want 0", root["generation"])
	}

	// Verify father
	father := root["father"].(map[string]interface{})
	if father["given_name"] != "John" {
		t.Errorf("Father given_name = %v, want John", father["given_name"])
	}
	if int(father["generation"].(float64)) != 1 {
		t.Errorf("Father generation = %v, want 1", father["generation"])
	}

	// Verify mother
	mother := root["mother"].(map[string]interface{})
	if mother["given_name"] != "Jane" {
		t.Errorf("Mother given_name = %v, want Jane", mother["given_name"])
	}

	// Verify grandfather (father's father)
	grandfather := father["father"].(map[string]interface{})
	if grandfather["given_name"] != "George" {
		t.Errorf("Grandfather given_name = %v, want George", grandfather["given_name"])
	}
	if int(grandfather["generation"].(float64)) != 2 {
		t.Errorf("Grandfather generation = %v, want 2", grandfather["generation"])
	}

	// Verify grandmother (father's mother)
	grandmother := father["mother"].(map[string]interface{})
	if grandmother["given_name"] != "Mary" {
		t.Errorf("Grandmother given_name = %v, want Mary", grandmother["given_name"])
	}

	// Verify counts
	totalAncestors := int(result["total_ancestors"].(float64))
	if totalAncestors != 4 {
		t.Errorf("total_ancestors = %d, want 4", totalAncestors)
	}

	maxGeneration := int(result["max_generation"].(float64))
	if maxGeneration != 2 {
		t.Errorf("max_generation = %d, want 2", maxGeneration)
	}
}

func TestGetPedigree_NotFound(t *testing.T) {
	server := setupPedigreeTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pedigree/00000000-0000-0000-0000-000000000001", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("Expected status 404, got %d", rec.Code)
	}
}

func TestGetPedigree_InvalidID(t *testing.T) {
	server := setupPedigreeTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pedigree/not-a-uuid", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", rec.Code)
	}
}

func TestGetPedigree_MaxGenerations(t *testing.T) {
	server := setupPedigreeTestServer(t)
	juniorID := importPedigreeTestData(t, server)

	// Request only 1 generation
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pedigree/"+juniorID+"?generations=1", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)

	root := result["root"].(map[string]interface{})
	father := root["father"].(map[string]interface{})

	// Father should exist (generation 1)
	if father["given_name"] != "John" {
		t.Error("Father should be present at generation 1")
	}

	// Grandfather should be nil (generation 2 exceeds limit)
	if father["father"] != nil {
		t.Error("Grandfather should be nil when max generation is 1")
	}

	// Max generation should be 1
	maxGeneration := int(result["max_generation"].(float64))
	if maxGeneration != 1 {
		t.Errorf("max_generation = %d, want 1", maxGeneration)
	}
}

func TestGetPedigree_NoAncestors(t *testing.T) {
	server := setupPedigreeTestServer(t)

	// Create a person with no ancestors
	body := `{"given_name":"Orphan","surname":"Child"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	var created map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &created)
	personID := created["id"].(string)

	// Get pedigree for person with no ancestors
	req = httptest.NewRequest(http.MethodGet, "/api/v1/pedigree/"+personID, http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)

	root := result["root"].(map[string]interface{})
	if root["given_name"] != "Orphan" {
		t.Errorf("Root given_name = %v, want Orphan", root["given_name"])
	}

	// Should have no ancestors
	if root["father"] != nil {
		t.Error("Father should be nil")
	}
	if root["mother"] != nil {
		t.Error("Mother should be nil")
	}

	totalAncestors := int(result["total_ancestors"].(float64))
	if totalAncestors != 0 {
		t.Errorf("total_ancestors = %d, want 0", totalAncestors)
	}
}
