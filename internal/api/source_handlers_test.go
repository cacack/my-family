package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateSource(t *testing.T) {
	server := setupTestServer()
	body := `{"source_type":"book","title":"Census of 1900","author":"US Census Bureau","publisher":"NARA"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["title"] != "Census of 1900" {
		t.Errorf("title = %v, want Census of 1900", resp["title"])
	}
	if resp["source_type"] != "book" {
		t.Errorf("source_type = %v, want book", resp["source_type"])
	}
	if resp["id"] == nil || resp["id"] == "" {
		t.Error("Expected non-empty id")
	}
}

func TestCreateSource_MissingTitle(t *testing.T) {
	server := setupTestServer()
	body := `{"source_type":"book"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListSources(t *testing.T) {
	server := setupTestServer()

	// Create two sources
	body1 := `{"source_type":"book","title":"Source 1"}`
	createReq1 := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(body1))
	createReq1.Header.Set("Content-Type", "application/json")
	createRec1 := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec1, createReq1)

	body2 := `{"source_type":"census","title":"Source 2"}`
	createReq2 := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(body2))
	createReq2.Header.Set("Content-Type", "application/json")
	createRec2 := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec2, createReq2)

	// List sources
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sources", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp["total"].(float64) != 2 {
		t.Errorf("total = %v, want 2", resp["total"])
	}

	sources := resp["sources"].([]any)
	if len(sources) != 2 {
		t.Errorf("len(sources) = %d, want 2", len(sources))
	}
}

func TestGetSource(t *testing.T) {
	server := setupTestServer()

	// Create a source
	body := `{"source_type":"book","title":"My Source"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	sourceID := createResp["id"].(string)

	// Get the source
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sources/"+sourceID, nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["title"] != "My Source" {
		t.Errorf("title = %v, want My Source", resp["title"])
	}
}

func TestGetSource_NotFound(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sources/00000000-0000-0000-0000-000000000001", nil)
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUpdateSource(t *testing.T) {
	server := setupTestServer()

	// Create a source
	body := `{"source_type":"book","title":"Original Title"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	sourceID := createResp["id"].(string)
	version := int64(createResp["version"].(float64))

	// Update the source
	updateBody := fmt.Sprintf(`{"title":"Updated Title","version":%d}`, version)
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/sources/"+sourceID, strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(updateRec, updateReq)

	if updateRec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(updateRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["title"] != "Updated Title" {
		t.Errorf("title = %v, want Updated Title", resp["title"])
	}
}

func TestUpdateSource_VersionConflict(t *testing.T) {
	server := setupTestServer()

	// Create a source
	body := `{"source_type":"book","title":"Original Title"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	sourceID := createResp["id"].(string)

	// Update with wrong version
	updateBody := `{"title":"Updated Title","version":999}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/sources/"+sourceID, strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(updateRec, updateReq)

	if updateRec.Code != http.StatusConflict {
		t.Errorf("Status = %d, want %d", updateRec.Code, http.StatusConflict)
	}
}

func TestDeleteSource(t *testing.T) {
	server := setupTestServer()

	// Create a source
	body := `{"source_type":"book","title":"To Delete"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	sourceID := createResp["id"].(string)

	// Delete the source
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/sources/"+sourceID, nil)
	deleteRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusNoContent {
		t.Errorf("Status = %d, want %d", deleteRec.Code, http.StatusNoContent)
	}

	// Verify deletion
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/sources/"+sourceID, nil)
	getRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Errorf("Status after delete = %d, want %d", getRec.Code, http.StatusNotFound)
	}
}

func TestSearchSources(t *testing.T) {
	server := setupTestServer()

	// Create sources
	body1 := `{"source_type":"book","title":"Census of 1900"}`
	createReq1 := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(body1))
	createReq1.Header.Set("Content-Type", "application/json")
	createRec1 := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec1, createReq1)

	body2 := `{"source_type":"book","title":"Census of 1910"}`
	createReq2 := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(body2))
	createReq2.Header.Set("Content-Type", "application/json")
	createRec2 := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec2, createReq2)

	// Search for "Census"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sources/search?q=Census", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	sources := resp["sources"].([]any)
	if len(sources) < 2 {
		t.Errorf("len(sources) = %d, want at least 2", len(sources))
	}
}

func TestCreateCitation(t *testing.T) {
	server := setupTestServer()

	// Create a source first
	sourceBody := `{"source_type":"book","title":"Test Source"}`
	sourceReq := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(sourceBody))
	sourceReq.Header.Set("Content-Type", "application/json")
	sourceRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(sourceRec, sourceReq)

	var sourceResp map[string]any
	json.Unmarshal(sourceRec.Body.Bytes(), &sourceResp)
	sourceID := sourceResp["id"].(string)

	// Create a person for fact_owner_id
	personBody := `{"given_name":"John","surname":"Doe"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(personBody))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	var personResp map[string]any
	json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	// Create citation
	citationBody := `{"source_id":"` + sourceID + `","fact_type":"person_birth","fact_owner_id":"` + personID + `","page":"123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/citations", strings.NewReader(citationBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["source_id"] != sourceID {
		t.Errorf("source_id = %v, want %s", resp["source_id"], sourceID)
	}
	if resp["fact_type"] != "person_birth" {
		t.Errorf("fact_type = %v, want person_birth", resp["fact_type"])
	}
}

func TestGetCitation(t *testing.T) {
	server := setupTestServer()

	// Create source and person
	sourceBody := `{"source_type":"book","title":"Test Source"}`
	sourceReq := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(sourceBody))
	sourceReq.Header.Set("Content-Type", "application/json")
	sourceRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(sourceRec, sourceReq)

	var sourceResp map[string]any
	json.Unmarshal(sourceRec.Body.Bytes(), &sourceResp)
	sourceID := sourceResp["id"].(string)

	personBody := `{"given_name":"John","surname":"Doe"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(personBody))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	var personResp map[string]any
	json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	// Create citation
	citationBody := `{"source_id":"` + sourceID + `","fact_type":"person_birth","fact_owner_id":"` + personID + `"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/citations", strings.NewReader(citationBody))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	citationID := createResp["id"].(string)

	// Get the citation
	req := httptest.NewRequest(http.MethodGet, "/api/v1/citations/"+citationID, nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["fact_type"] != "person_birth" {
		t.Errorf("fact_type = %v, want person_birth", resp["fact_type"])
	}
}

func TestGetCitationsForPerson(t *testing.T) {
	server := setupTestServer()

	// Create source
	sourceBody := `{"source_type":"book","title":"Test Source"}`
	sourceReq := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(sourceBody))
	sourceReq.Header.Set("Content-Type", "application/json")
	sourceRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(sourceRec, sourceReq)

	var sourceResp map[string]any
	json.Unmarshal(sourceRec.Body.Bytes(), &sourceResp)
	sourceID := sourceResp["id"].(string)

	// Create person
	personBody := `{"given_name":"John","surname":"Doe"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(personBody))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	var personResp map[string]any
	json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	// Create two citations for the person
	citation1Body := `{"source_id":"` + sourceID + `","fact_type":"person_birth","fact_owner_id":"` + personID + `"}`
	citation1Req := httptest.NewRequest(http.MethodPost, "/api/v1/citations", strings.NewReader(citation1Body))
	citation1Req.Header.Set("Content-Type", "application/json")
	citation1Rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(citation1Rec, citation1Req)

	citation2Body := `{"source_id":"` + sourceID + `","fact_type":"person_death","fact_owner_id":"` + personID + `"}`
	citation2Req := httptest.NewRequest(http.MethodPost, "/api/v1/citations", strings.NewReader(citation2Body))
	citation2Req.Header.Set("Content-Type", "application/json")
	citation2Rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(citation2Rec, citation2Req)

	// Get citations for person
	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+personID+"/citations", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	citations := resp["citations"].([]any)
	if len(citations) != 2 {
		t.Errorf("len(citations) = %d, want 2", len(citations))
	}
}

func TestDeleteCitation(t *testing.T) {
	server := setupTestServer()

	// Create source and person
	sourceBody := `{"source_type":"book","title":"Test Source"}`
	sourceReq := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(sourceBody))
	sourceReq.Header.Set("Content-Type", "application/json")
	sourceRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(sourceRec, sourceReq)

	var sourceResp map[string]any
	json.Unmarshal(sourceRec.Body.Bytes(), &sourceResp)
	sourceID := sourceResp["id"].(string)

	personBody := `{"given_name":"John","surname":"Doe"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(personBody))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	var personResp map[string]any
	json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	// Create citation
	citationBody := `{"source_id":"` + sourceID + `","fact_type":"person_birth","fact_owner_id":"` + personID + `"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/citations", strings.NewReader(citationBody))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	citationID := createResp["id"].(string)

	// Delete the citation
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/citations/"+citationID, nil)
	deleteRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusNoContent {
		t.Errorf("Status = %d, want %d", deleteRec.Code, http.StatusNoContent)
	}

	// Verify deletion
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/citations/"+citationID, nil)
	getRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Errorf("Status after delete = %d, want %d", getRec.Code, http.StatusNotFound)
	}
}

func TestDeleteSource_WithCitations(t *testing.T) {
	server := setupTestServer()

	// Create source
	sourceBody := `{"source_type":"book","title":"Test Source"}`
	sourceReq := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(sourceBody))
	sourceReq.Header.Set("Content-Type", "application/json")
	sourceRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(sourceRec, sourceReq)

	var sourceResp map[string]any
	json.Unmarshal(sourceRec.Body.Bytes(), &sourceResp)
	sourceID := sourceResp["id"].(string)

	// Create person
	personBody := `{"given_name":"John","surname":"Doe"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(personBody))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	var personResp map[string]any
	json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	// Create citation
	citationBody := `{"source_id":"` + sourceID + `","fact_type":"person_birth","fact_owner_id":"` + personID + `"}`
	citationReq := httptest.NewRequest(http.MethodPost, "/api/v1/citations", strings.NewReader(citationBody))
	citationReq.Header.Set("Content-Type", "application/json")
	citationRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(citationRec, citationReq)

	// Try to delete source (should fail)
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/sources/"+sourceID, nil)
	deleteRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusConflict {
		t.Errorf("Status = %d, want %d", deleteRec.Code, http.StatusConflict)
	}
}
