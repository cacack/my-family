package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository/memory"
)

func setupTestServer() *api.Server {
	cfg := &config.Config{
		Port:      8080,
		LogFormat: "text",
	}
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	return api.NewServer(cfg, eventStore, readStore, nil)
}

func TestHealthCheck(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", http.NoBody)
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("Status = %s, want ok", resp["status"])
	}
}

func TestCreatePerson(t *testing.T) {
	server := setupTestServer()
	body := `{"given_name":"John","surname":"Doe","gender":"male","birth_date":"1 JAN 1990"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
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
	if resp["given_name"] != "John" {
		t.Errorf("given_name = %v, want John", resp["given_name"])
	}
	if resp["surname"] != "Doe" {
		t.Errorf("surname = %v, want Doe", resp["surname"])
	}
	if resp["id"] == nil || resp["id"] == "" {
		t.Error("Expected non-empty id")
	}
}

func TestCreatePerson_ValidationError(t *testing.T) {
	server := setupTestServer()
	// Missing required surname
	body := `{"given_name":"John"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListPersons(t *testing.T) {
	server := setupTestServer()

	// Create a person first
	body := `{"given_name":"John","surname":"Doe"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	// List persons
	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["total"].(float64) != 1 {
		t.Errorf("total = %v, want 1", resp["total"])
	}
	items := resp["items"].([]any)
	if len(items) != 1 {
		t.Errorf("items count = %d, want 1", len(items))
	}
}

func TestGetPerson(t *testing.T) {
	server := setupTestServer()

	// Create a person first
	body := `{"given_name":"Jane","surname":"Smith"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	personID := createResp["id"].(string)

	// Get the person
	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+personID, http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp["given_name"] != "Jane" {
		t.Errorf("given_name = %v, want Jane", resp["given_name"])
	}
}

func TestGetPerson_NotFound(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/00000000-0000-0000-0000-000000000001", http.NoBody)
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUpdatePerson(t *testing.T) {
	server := setupTestServer()

	// Create a person first
	body := `{"given_name":"John","surname":"Doe"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	personID := createResp["id"].(string)

	// Update the person
	updateBody := `{"given_name":"Jane","version":1}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/persons/"+personID, strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(updateRec, updateReq)

	if updateRec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}

	var updateResp map[string]any
	if err := json.Unmarshal(updateRec.Body.Bytes(), &updateResp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if updateResp["given_name"] != "Jane" {
		t.Errorf("given_name = %v, want Jane", updateResp["given_name"])
	}
}

func TestUpdatePerson_VersionConflict(t *testing.T) {
	server := setupTestServer()

	// Create a person
	body := `{"given_name":"John","surname":"Doe"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	personID := createResp["id"].(string)

	// Update with wrong version
	updateBody := `{"given_name":"Jane","version":999}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/persons/"+personID, strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(updateRec, updateReq)

	if updateRec.Code != http.StatusConflict {
		t.Errorf("Status = %d, want %d", updateRec.Code, http.StatusConflict)
	}
}

func TestDeletePerson(t *testing.T) {
	server := setupTestServer()

	// Create a person
	body := `{"given_name":"John","surname":"Doe"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	personID := createResp["id"].(string)

	// Delete the person
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/persons/"+personID+"?version=1", http.NoBody)
	deleteRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusNoContent {
		t.Errorf("Status = %d, want %d", deleteRec.Code, http.StatusNoContent)
	}

	// Verify person is gone
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+personID, http.NoBody)
	getRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Errorf("Status = %d after delete, want %d", getRec.Code, http.StatusNotFound)
	}
}

func TestSearchPersons(t *testing.T) {
	server := setupTestServer()

	// Create some persons
	for _, name := range []string{"John Smith", "Jane Smith", "Bob Johnson"} {
		parts := strings.Split(name, " ")
		body := `{"given_name":"` + parts[0] + `","surname":"` + parts[1] + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.Echo().ServeHTTP(rec, req)
	}

	// Search for Smith
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=Smith", http.NoBody)
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
}

func TestSearchPersons_QueryTooShort(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=a", http.NoBody)
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestOpenAPISpec(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml", http.NoBody)
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/x-yaml" {
		t.Errorf("Content-Type = %s, want application/x-yaml", contentType)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "openapi: 3.0.3") {
		t.Error("Expected OpenAPI 3.0.3 specification")
	}
	if !strings.Contains(body, "My Family Genealogy API") {
		t.Error("Expected API title in spec")
	}
}

func TestSwaggerUI(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs", http.NoBody)
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("Content-Type = %s, want text/html", contentType)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "swagger-ui") {
		t.Error("Expected Swagger UI HTML")
	}
	// html/template escapes slashes in URLs, so check for escaped version
	if !strings.Contains(body, "openapi.yaml") {
		t.Error("Expected OpenAPI spec URL in Swagger UI")
	}
}

func TestCreatePerson_InvalidJSON(t *testing.T) {
	server := setupTestServer()
	body := `{"given_name":"John",invalid json`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetPerson_InvalidUUID(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/not-a-uuid", http.NoBody)
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdatePerson_InvalidUUID(t *testing.T) {
	server := setupTestServer()
	body := `{"given_name":"Jane","version":1}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/persons/not-a-uuid", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdatePerson_InvalidJSON(t *testing.T) {
	server := setupTestServer()

	// Create a person first
	body := `{"given_name":"John","surname":"Doe"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	personID := createResp["id"].(string)

	// Update with invalid JSON
	updateBody := `{"given_name":"Jane",invalid`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/persons/"+personID, strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(updateRec, updateReq)

	if updateRec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", updateRec.Code, http.StatusBadRequest)
	}
}

func TestUpdatePerson_NotFound(t *testing.T) {
	server := setupTestServer()
	body := `{"given_name":"Jane","version":1}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/persons/00000000-0000-0000-0000-000000000001", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeletePerson_InvalidUUID(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/persons/not-a-uuid?version=1", http.NoBody)
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeletePerson_NotFound(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/persons/00000000-0000-0000-0000-000000000001?version=1", http.NoBody)
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestSearchPersons_EmptyQuery(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=", http.NoBody)
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetPerson_WithFamilies(t *testing.T) {
	server := setupTestServer()

	// Create three persons
	person1Body := `{"given_name":"Parent1","surname":"Smith"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(person1Body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	var person1 map[string]any
	json.Unmarshal(rec.Body.Bytes(), &person1)

	person2Body := `{"given_name":"Parent2","surname":"Doe"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(person2Body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	var person2 map[string]any
	json.Unmarshal(rec.Body.Bytes(), &person2)

	childBody := `{"given_name":"Child","surname":"Smith"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(childBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	var child map[string]any
	json.Unmarshal(rec.Body.Bytes(), &child)

	// Create family with both parents
	familyBody := `{"partner1_id":"` + person1["id"].(string) + `","partner2_id":"` + person2["id"].(string) + `","relationship_type":"marriage"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/families", strings.NewReader(familyBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	var family map[string]any
	json.Unmarshal(rec.Body.Bytes(), &family)

	// Add child to family
	addChildBody := `{"child_id":"` + child["id"].(string) + `"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/families/"+family["id"].(string)+"/children", strings.NewReader(addChildBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	// Get child person - should show family_as_child
	req = httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+child["id"].(string), http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var childResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &childResp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Verify family_as_child is present
	if childResp["family_as_child"] == nil {
		t.Error("Expected family_as_child field")
	}

	// Get parent person - should show families_as_partner
	req = httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+person1["id"].(string), http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var parentResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &parentResp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Verify families_as_partner is present
	if parentResp["families_as_partner"] == nil {
		t.Error("Expected families_as_partner field")
	}

	families := parentResp["families_as_partner"].([]any)
	if len(families) != 1 {
		t.Errorf("Expected 1 family, got %d", len(families))
	}
}

func TestGetFamilyGroupSheet(t *testing.T) {
	server := setupTestServer()

	// Create husband
	husbandBody := `{"given_name":"John","surname":"Smith","gender":"male","birth_date":"1 JAN 1960","birth_place":"New York"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(husbandBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	var husband map[string]any
	json.Unmarshal(rec.Body.Bytes(), &husband)

	// Create wife
	wifeBody := `{"given_name":"Jane","surname":"Doe","gender":"female","birth_date":"15 MAR 1965","birth_place":"Boston"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(wifeBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	var wife map[string]any
	json.Unmarshal(rec.Body.Bytes(), &wife)

	// Create child
	childBody := `{"given_name":"Jimmy","surname":"Smith","gender":"male","birth_date":"20 JUN 1990"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(childBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	var child map[string]any
	json.Unmarshal(rec.Body.Bytes(), &child)

	// Create family
	familyBody := `{"partner1_id":"` + husband["id"].(string) + `","partner2_id":"` + wife["id"].(string) + `","relationship_type":"marriage","marriage_date":"25 DEC 1985","marriage_place":"Chicago"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/families", strings.NewReader(familyBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	var family map[string]any
	json.Unmarshal(rec.Body.Bytes(), &family)

	// Add child to family
	addChildBody := `{"child_id":"` + child["id"].(string) + `"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/families/"+family["id"].(string)+"/children", strings.NewReader(addChildBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	// Get group sheet
	req = httptest.NewRequest(http.MethodGet, "/api/v1/families/"+family["id"].(string)+"/group-sheet", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var gs map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &gs); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Verify husband
	if gs["husband"] == nil {
		t.Error("Expected husband in group sheet")
	} else {
		h := gs["husband"].(map[string]any)
		if h["given_name"] != "John" {
			t.Errorf("Husband given_name = %v, want John", h["given_name"])
		}
	}

	// Verify wife
	if gs["wife"] == nil {
		t.Error("Expected wife in group sheet")
	} else {
		w := gs["wife"].(map[string]any)
		if w["given_name"] != "Jane" {
			t.Errorf("Wife given_name = %v, want Jane", w["given_name"])
		}
	}

	// Verify marriage
	if gs["marriage"] == nil {
		t.Error("Expected marriage event in group sheet")
	}

	// Verify children
	if gs["children"] == nil {
		t.Error("Expected children in group sheet")
	} else {
		children := gs["children"].([]any)
		if len(children) != 1 {
			t.Errorf("Children count = %d, want 1", len(children))
		} else {
			c := children[0].(map[string]any)
			if c["given_name"] != "Jimmy" {
				t.Errorf("Child given_name = %v, want Jimmy", c["given_name"])
			}
		}
	}
}

func TestGetFamilyGroupSheet_InvalidUUID(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/families/not-a-uuid/group-sheet", http.NoBody)
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetFamilyGroupSheet_NotFound(t *testing.T) {
	server := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/families/00000000-0000-0000-0000-000000000001/group-sheet", http.NoBody)
	rec := httptest.NewRecorder()

	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
