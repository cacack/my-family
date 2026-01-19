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

func setupDescendancyTestServer(t *testing.T) *api.Server {
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

// GEDCOM with 3 generations of descendants from George
const descendancyTestGedcom = `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME George /Smith/
1 SEX M
1 BIRT
2 DATE 1 JAN 1940
0 @I2@ INDI
1 NAME Mary /Jones/
1 SEX F
1 BIRT
2 DATE 1 JAN 1945
0 @I3@ INDI
1 NAME John /Smith/
1 SEX M
1 BIRT
2 DATE 1 JAN 1970
0 @I4@ INDI
1 NAME Jane /Doe/
1 SEX F
1 BIRT
2 DATE 1 JAN 1975
0 @I5@ INDI
1 NAME Junior /Smith/
1 SEX M
1 BIRT
2 DATE 1 JAN 2000
0 @I6@ INDI
1 NAME Jenny /Smith/
1 SEX F
1 BIRT
2 DATE 1 JAN 2002
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 MARR
2 DATE 15 JUN 1965
1 CHIL @I3@
0 @F2@ FAM
1 HUSB @I3@
1 WIFE @I4@
1 MARR
2 DATE 15 JUN 1995
1 CHIL @I5@
1 CHIL @I6@
0 TRLR
`

func importDescendancyTestData(t *testing.T, server *api.Server) string {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.ged")
	io.WriteString(part, descendancyTestGedcom)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Import failed: %d: %s", rec.Code, rec.Body.String())
	}

	// Get George's ID from search (the root ancestor)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/search?q=George", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	var searchResult struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	json.Unmarshal(rec.Body.Bytes(), &searchResult)

	if len(searchResult.Items) == 0 {
		t.Fatal("Could not find George in search results")
	}

	return searchResult.Items[0].ID
}

func TestGetDescendancy_Success(t *testing.T) {
	server := setupDescendancyTestServer(t)
	georgeID := importDescendancyTestData(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/descendancy/"+georgeID, http.NoBody)
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
	if root["given_name"] != "George" {
		t.Errorf("Root given_name = %v, want George", root["given_name"])
	}
	if root["surname"] != "Smith" {
		t.Errorf("Root surname = %v, want Smith", root["surname"])
	}
	if int(root["generation"].(float64)) != 0 {
		t.Errorf("Root generation = %v, want 0", root["generation"])
	}

	// Verify spouses
	spouses := root["spouses"].([]interface{})
	if len(spouses) != 1 {
		t.Errorf("Expected 1 spouse, got %d", len(spouses))
	}
	spouse := spouses[0].(map[string]interface{})
	if spouse["name"] != "Mary Jones" {
		t.Errorf("Spouse name = %v, want Mary Jones", spouse["name"])
	}
	// Marriage date should be present
	if spouse["marriage_date"] == nil {
		t.Error("Marriage date should be present")
	}

	// Verify children (John is George's child)
	children := root["children"].([]interface{})
	if len(children) != 1 {
		t.Errorf("Expected 1 child, got %d", len(children))
	}
	john := children[0].(map[string]interface{})
	if john["given_name"] != "John" {
		t.Errorf("Child given_name = %v, want John", john["given_name"])
	}
	if int(john["generation"].(float64)) != 1 {
		t.Errorf("John generation = %v, want 1", john["generation"])
	}

	// Verify John's spouse
	johnSpouses := john["spouses"].([]interface{})
	if len(johnSpouses) != 1 {
		t.Errorf("Expected 1 spouse for John, got %d", len(johnSpouses))
	}
	johnSpouse := johnSpouses[0].(map[string]interface{})
	if johnSpouse["name"] != "Jane Doe" {
		t.Errorf("John's spouse name = %v, want Jane Doe", johnSpouse["name"])
	}

	// Verify grandchildren (Junior and Jenny are John's children)
	grandchildren := john["children"].([]interface{})
	if len(grandchildren) != 2 {
		t.Errorf("Expected 2 grandchildren, got %d", len(grandchildren))
	}

	// Verify counts
	totalDescendants := int(result["total_descendants"].(float64))
	if totalDescendants != 3 { // John, Junior, Jenny
		t.Errorf("total_descendants = %d, want 3", totalDescendants)
	}

	maxGeneration := int(result["max_generation"].(float64))
	if maxGeneration != 2 {
		t.Errorf("max_generation = %d, want 2", maxGeneration)
	}
}

func TestGetDescendancy_NotFound(t *testing.T) {
	server := setupDescendancyTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/descendancy/00000000-0000-0000-0000-000000000001", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("Expected status 404, got %d", rec.Code)
	}
}

func TestGetDescendancy_InvalidID(t *testing.T) {
	server := setupDescendancyTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/descendancy/not-a-uuid", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", rec.Code)
	}
}

func TestGetDescendancy_MaxGenerations(t *testing.T) {
	server := setupDescendancyTestServer(t)
	georgeID := importDescendancyTestData(t, server)

	// Request only 1 generation
	req := httptest.NewRequest(http.MethodGet, "/api/v1/descendancy/"+georgeID+"?generations=1", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)

	root := result["root"].(map[string]interface{})
	children := root["children"].([]interface{})

	// Children should exist (generation 1)
	if len(children) == 0 {
		t.Error("Children should be present at generation 1")
	}

	// John should not have children (generation 2 exceeds limit)
	john := children[0].(map[string]interface{})
	if john["children"] != nil {
		t.Error("Grandchildren should be nil when max generation is 1")
	}

	// Max generation should be 1
	maxGeneration := int(result["max_generation"].(float64))
	if maxGeneration != 1 {
		t.Errorf("max_generation = %d, want 1", maxGeneration)
	}
}

func TestGetDescendancy_NoDescendants(t *testing.T) {
	server := setupDescendancyTestServer(t)

	// Create a person with no descendants
	body := `{"given_name":"Leaf","surname":"Person"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	var created map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &created)
	personID := created["id"].(string)

	// Get descendancy for person with no descendants
	req = httptest.NewRequest(http.MethodGet, "/api/v1/descendancy/"+personID, http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)

	root := result["root"].(map[string]interface{})
	if root["given_name"] != "Leaf" {
		t.Errorf("Root given_name = %v, want Leaf", root["given_name"])
	}

	// Should have no children or spouses
	if root["children"] != nil {
		t.Error("Children should be nil")
	}
	if root["spouses"] != nil {
		t.Error("Spouses should be nil")
	}

	totalDescendants := int(result["total_descendants"].(float64))
	if totalDescendants != 0 {
		t.Errorf("total_descendants = %d, want 0", totalDescendants)
	}
}

func TestGetDescendancy_FromMiddleOfTree(t *testing.T) {
	server := setupDescendancyTestServer(t)
	importDescendancyTestData(t, server)

	// Get John's ID (he's in the middle of the tree)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=John", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	var searchResult struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	json.Unmarshal(rec.Body.Bytes(), &searchResult)

	if len(searchResult.Items) == 0 {
		t.Fatal("Could not find John in search results")
	}
	johnID := searchResult.Items[0].ID

	// Get descendancy starting from John
	req = httptest.NewRequest(http.MethodGet, "/api/v1/descendancy/"+johnID, http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)

	// John should be the root
	root := result["root"].(map[string]interface{})
	if root["given_name"] != "John" {
		t.Errorf("Root given_name = %v, want John", root["given_name"])
	}
	if int(root["generation"].(float64)) != 0 {
		t.Errorf("John should be generation 0 as root, got %v", root["generation"])
	}

	// John should have 2 children (Junior and Jenny)
	children := root["children"].([]interface{})
	if len(children) != 2 {
		t.Errorf("Expected 2 children, got %d", len(children))
	}

	// Total descendants should be 2 (Junior and Jenny only, not George who is an ancestor)
	totalDescendants := int(result["total_descendants"].(float64))
	if totalDescendants != 2 {
		t.Errorf("total_descendants = %d, want 2", totalDescendants)
	}
}
