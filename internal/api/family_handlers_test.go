package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository/memory"
)

func setupFamilyTestServer(t *testing.T) *api.Server {
	t.Helper()
	cfg := &config.Config{
		Port:      8080,
		LogFormat: "text",
	}
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	return api.NewServer(cfg, eventStore, readStore, nil)
}

func TestCreateFamily(t *testing.T) {
	server := setupFamilyTestServer(t)

	// Create two persons first
	person1 := createTestPerson(t, server, "John", "Doe")
	person2 := createTestPerson(t, server, "Jane", "Smith")

	// Create family
	body := map[string]interface{}{
		"partner1_id":       person1["id"],
		"partner2_id":       person2["id"],
		"relationship_type": "marriage",
		"marriage_date":     "10 JUN 1975",
		"marriage_place":    "Springfield, IL",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/families", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("Expected status 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result["id"] == nil {
		t.Error("Response should include id")
	}
	if result["relationship_type"] != "marriage" {
		t.Errorf("Expected relationship_type 'marriage', got %v", result["relationship_type"])
	}
}

func TestCreateFamily_SingleParent(t *testing.T) {
	server := setupFamilyTestServer(t)

	// Create one person
	person := createTestPerson(t, server, "Jane", "Doe")

	// Create single-parent family
	body := map[string]interface{}{
		"partner2_id": person["id"],
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/families", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("Expected status 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateFamily_NoPartners(t *testing.T) {
	server := setupFamilyTestServer(t)

	// Create family with no partners - should fail validation
	body := map[string]interface{}{}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/families", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	// Server returns 500 for this validation error (caught by command layer, not API validation)
	if rec.Code != http.StatusInternalServerError && rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400 or 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListFamilies(t *testing.T) {
	server := setupFamilyTestServer(t)

	// Create two persons and a family
	person1 := createTestPerson(t, server, "John", "Doe")
	person2 := createTestPerson(t, server, "Jane", "Smith")

	body := map[string]interface{}{
		"partner1_id": person1["id"],
		"partner2_id": person2["id"],
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/families", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	// List families
	req = httptest.NewRequest(http.MethodGet, "/api/v1/families", nil)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)

	families := result["items"].([]interface{})
	if len(families) != 1 {
		t.Errorf("Expected 1 family, got %d", len(families))
	}
}

func TestGetFamily(t *testing.T) {
	server := setupFamilyTestServer(t)

	// Create two persons and a family
	person1 := createTestPerson(t, server, "John", "Doe")
	person2 := createTestPerson(t, server, "Jane", "Smith")

	body := map[string]interface{}{
		"partner1_id":    person1["id"],
		"partner2_id":    person2["id"],
		"marriage_date":  "15 MAR 1980",
		"marriage_place": "Chicago, IL",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/families", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	var created map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &created)

	// Get family
	req = httptest.NewRequest(http.MethodGet, "/api/v1/families/"+created["id"].(string), nil)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result["marriage_place"] != "Chicago, IL" {
		t.Errorf("Expected marriage_place 'Chicago, IL', got %v", result["marriage_place"])
	}
}

func TestGetFamily_WithChildren_ResponseFormat(t *testing.T) {
	server := setupFamilyTestServer(t)

	// Create parents and children
	parent1 := createTestPerson(t, server, "John", "Doe")
	parent2 := createTestPerson(t, server, "Jane", "Doe")
	child1 := createTestPerson(t, server, "Alice", "Doe")
	child2 := createTestPerson(t, server, "Bob", "Doe")

	// Create family
	body := map[string]interface{}{
		"partner1_id":       parent1["id"],
		"partner2_id":       parent2["id"],
		"relationship_type": "marriage",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/families", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	var family map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &family)
	familyID := family["id"].(string)

	// Add children
	for _, child := range []map[string]interface{}{child1, child2} {
		childBody := map[string]interface{}{
			"child_id":          child["id"],
			"relationship_type": "biological",
		}
		jsonBody, _ = json.Marshal(childBody)

		req = httptest.NewRequest(http.MethodPost, "/api/v1/families/"+familyID+"/children", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		rec = httptest.NewRecorder()
		server.Echo().ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("Failed to add child: %d: %s", rec.Code, rec.Body.String())
		}
	}

	// Get family with children
	req = httptest.NewRequest(http.MethodGet, "/api/v1/families/"+familyID, nil)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)

	// Verify children array exists and has correct structure
	children, ok := result["children"].([]interface{})
	if !ok {
		t.Fatal("Expected 'children' array in response")
	}
	if len(children) != 2 {
		t.Errorf("Expected 2 children, got %d", len(children))
	}

	// Verify each child has required fields: id, name, relationship_type
	for i, c := range children {
		child := c.(map[string]interface{})

		if _, ok := child["id"]; !ok {
			t.Errorf("Child %d missing 'id' field", i)
		}
		if _, ok := child["name"]; !ok {
			t.Errorf("Child %d missing 'name' field", i)
		}
		if _, ok := child["relationship_type"]; !ok {
			t.Errorf("Child %d missing 'relationship_type' field", i)
		}

		// Verify name is formatted correctly (not empty)
		name := child["name"].(string)
		if name == "" {
			t.Errorf("Child %d has empty name", i)
		}
	}

	// Verify child_count matches
	if result["child_count"].(float64) != 2 {
		t.Errorf("Expected child_count 2, got %v", result["child_count"])
	}
}

func TestGetFamily_NotFound(t *testing.T) {
	server := setupFamilyTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/families/00000000-0000-0000-0000-000000000001", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("Expected status 404, got %d", rec.Code)
	}
}

func TestUpdateFamily(t *testing.T) {
	server := setupFamilyTestServer(t)

	// Create persons and family
	person1 := createTestPerson(t, server, "John", "Doe")
	person2 := createTestPerson(t, server, "Jane", "Smith")

	body := map[string]interface{}{
		"partner1_id": person1["id"],
		"partner2_id": person2["id"],
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/families", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	var created map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &created)

	// Update family with marriage date
	updateBody := map[string]interface{}{
		"version":        1,
		"marriage_date":  "20 DEC 1985",
		"marriage_place": "New York, NY",
	}
	jsonBody, _ = json.Marshal(updateBody)

	req = httptest.NewRequest(http.MethodPut, "/api/v1/families/"+created["id"].(string), bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result["marriage_place"] != "New York, NY" {
		t.Errorf("Expected marriage_place 'New York, NY', got %v", result["marriage_place"])
	}
}

func TestDeleteFamily(t *testing.T) {
	server := setupFamilyTestServer(t)

	// Create persons and family
	person1 := createTestPerson(t, server, "John", "Doe")
	person2 := createTestPerson(t, server, "Jane", "Smith")

	body := map[string]interface{}{
		"partner1_id": person1["id"],
		"partner2_id": person2["id"],
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/families", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	var created map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &created)

	// Delete family
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/families/"+created["id"].(string)+"?version=1", nil)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("Expected status 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify deleted
	req = httptest.NewRequest(http.MethodGet, "/api/v1/families/"+created["id"].(string), nil)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("Expected status 404 after delete, got %d", rec.Code)
	}
}

func TestAddChildToFamily(t *testing.T) {
	server := setupFamilyTestServer(t)

	// Create parents and child
	parent1 := createTestPerson(t, server, "John", "Doe")
	parent2 := createTestPerson(t, server, "Jane", "Smith")
	child := createTestPerson(t, server, "Junior", "Doe")

	// Create family
	body := map[string]interface{}{
		"partner1_id": parent1["id"],
		"partner2_id": parent2["id"],
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/families", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	var family map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &family)

	// Add child
	childBody := map[string]interface{}{
		"child_id":          child["id"],
		"relationship_type": "biological",
	}
	jsonBody, _ = json.Marshal(childBody)

	req = httptest.NewRequest(http.MethodPost, "/api/v1/families/"+family["id"].(string)+"/children", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("Expected status 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAddChildToFamily_AlreadyLinked(t *testing.T) {
	server := setupFamilyTestServer(t)

	// Create parents and child
	parent1 := createTestPerson(t, server, "John", "Doe")
	parent2 := createTestPerson(t, server, "Jane", "Smith")
	child := createTestPerson(t, server, "Junior", "Doe")

	// Create family
	body := map[string]interface{}{
		"partner1_id": parent1["id"],
		"partner2_id": parent2["id"],
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/families", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	var family map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &family)

	// Add child first time
	childBody := map[string]interface{}{
		"child_id": child["id"],
	}
	jsonBody, _ = json.Marshal(childBody)

	req = httptest.NewRequest(http.MethodPost, "/api/v1/families/"+family["id"].(string)+"/children", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("First add should succeed with 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// Try to add same child again
	req = httptest.NewRequest(http.MethodPost, "/api/v1/families/"+family["id"].(string)+"/children", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("Expected status 409 (conflict), got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRemoveChildFromFamily(t *testing.T) {
	server := setupFamilyTestServer(t)

	// Create parents and child
	parent1 := createTestPerson(t, server, "John", "Doe")
	parent2 := createTestPerson(t, server, "Jane", "Smith")
	child := createTestPerson(t, server, "Junior", "Doe")

	// Create family
	body := map[string]interface{}{
		"partner1_id": parent1["id"],
		"partner2_id": parent2["id"],
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/families", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	var family map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &family)

	// Add child
	childBody := map[string]interface{}{
		"child_id": child["id"],
	}
	jsonBody, _ = json.Marshal(childBody)

	req = httptest.NewRequest(http.MethodPost, "/api/v1/families/"+family["id"].(string)+"/children", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("Failed to add child: %d: %s", rec.Code, rec.Body.String())
	}

	// Get updated version
	req = httptest.NewRequest(http.MethodGet, "/api/v1/families/"+family["id"].(string), nil)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	json.Unmarshal(rec.Body.Bytes(), &family)

	// Remove child
	version := int(family["version"].(float64))
	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/families/%s/children/%s?version=%d", family["id"].(string), child["id"].(string), version), nil)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("Expected status 204, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRemoveChildFromFamily_NotInFamily(t *testing.T) {
	server := setupFamilyTestServer(t)

	// Create parents and unrelated person
	parent1 := createTestPerson(t, server, "John", "Doe")
	parent2 := createTestPerson(t, server, "Jane", "Smith")
	unrelated := createTestPerson(t, server, "Random", "Person")

	// Create family
	body := map[string]interface{}{
		"partner1_id": parent1["id"],
		"partner2_id": parent2["id"],
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/families", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	var family map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &family)

	// Try to remove unrelated person as child - returns 400 (bad request) not 404
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/families/"+family["id"].(string)+"/children/"+unrelated["id"].(string)+"?version=1", nil)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// Helper function
func createTestPerson(t *testing.T, server *api.Server, givenName, surname string) map[string]interface{} {
	t.Helper()
	body := map[string]interface{}{
		"given_name": givenName,
		"surname":    surname,
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("Failed to create test person: %s", rec.Body.String())
	}

	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)
	return result
}
