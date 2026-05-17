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
	snapshotStore := memory.NewSnapshotStore(eventStore)
	readStore := memory.NewReadModelStore()
	return api.NewServer(cfg, eventStore, readStore, snapshotStore, nil)
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
	req = httptest.NewRequest(http.MethodGet, "/api/v1/families", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal list response: %v (body: %s)", err, rec.Body.String())
	}

	families := result["items"].([]interface{})
	if len(families) != 1 {
		t.Fatalf("Expected 1 family, got %d", len(families))
	}

	// Regression: list endpoint must populate partner1/partner2 so the
	// frontend can render names without an extra round trip (issue #252).
	// The read model stores the partner's full name as a single string and
	// places it in given_name (see partnerSummary in server_strict.go); until
	// that is split, asserting on the full string is the correct contract.
	family := families[0].(map[string]interface{})
	partner1, ok := family["partner1"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected partner1 object in list response, got %v", family["partner1"])
	}
	if got := partner1["given_name"]; got != "John Doe" {
		t.Errorf("partner1.given_name: want %q, got %q", "John Doe", got)
	}
	partner2, ok := family["partner2"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected partner2 object in list response, got %v", family["partner2"])
	}
	if got := partner2["given_name"]; got != "Jane Smith" {
		t.Errorf("partner2.given_name: want %q, got %q", "Jane Smith", got)
	}
}

func TestListFamilies_SinglePartner(t *testing.T) {
	server := setupFamilyTestServer(t)

	// Models a family where only one partner is recorded. Using partner2_id
	// (rather than partner1_id) exercises the asymmetric absence path: the
	// list response must omit BOTH partner1 and partner1_id, while partner2
	// and partner2_id must be present. This guards against partial-population
	// inconsistencies where one field appears without the other.
	person := createTestPerson(t, server, "Jane", "Doe")
	body := map[string]interface{}{
		"partner2_id": person["id"],
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/families", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Setup: create family failed: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/families", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal list response: %v (body: %s)", err, rec.Body.String())
	}
	families := result["items"].([]interface{})
	if len(families) != 1 {
		t.Fatalf("Expected 1 family, got %d", len(families))
	}

	family := families[0].(map[string]interface{})
	if _, present := family["partner1"]; present {
		t.Errorf("Expected partner1 to be omitted for single-partner family, got %v", family["partner1"])
	}
	if _, present := family["partner1_id"]; present {
		t.Errorf("Expected partner1_id to be omitted alongside partner1, got %v", family["partner1_id"])
	}
	partner2, ok := family["partner2"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected partner2 object in list response, got %v", family["partner2"])
	}
	if got := partner2["given_name"]; got != "Jane Doe" {
		t.Errorf("partner2.given_name: want %q, got %q", "Jane Doe", got)
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
	req = httptest.NewRequest(http.MethodGet, "/api/v1/families/"+created["id"].(string), http.NoBody)
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
			"person_id":         child["id"],
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
	req = httptest.NewRequest(http.MethodGet, "/api/v1/families/"+familyID, http.NoBody)
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

	// Verify each child has required fields per OpenAPI spec: person_id, relationship_type
	for i, c := range children {
		child := c.(map[string]interface{})

		if _, ok := child["person_id"]; !ok {
			t.Errorf("Child %d missing 'person_id' field", i)
		}
		if _, ok := child["relationship_type"]; !ok {
			t.Errorf("Child %d missing 'relationship_type' field", i)
		}
	}
}

func TestGetFamily_NotFound(t *testing.T) {
	server := setupFamilyTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/families/00000000-0000-0000-0000-000000000001", http.NoBody)
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
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/families/"+created["id"].(string)+"?version=1", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("Expected status 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify deleted
	req = httptest.NewRequest(http.MethodGet, "/api/v1/families/"+created["id"].(string), http.NoBody)
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
		"person_id":         child["id"],
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
		"person_id": child["id"],
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
		"person_id": child["id"],
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
	req = httptest.NewRequest(http.MethodGet, "/api/v1/families/"+family["id"].(string), http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	json.Unmarshal(rec.Body.Bytes(), &family)

	// Remove child
	version := int(family["version"].(float64))
	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/families/%s/children/%s?version=%d", family["id"].(string), child["id"].(string), version), http.NoBody)
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
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/families/"+family["id"].(string)+"/children/"+unrelated["id"].(string)+"?version=1", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateFamily_InvalidJSON(t *testing.T) {
	server := setupFamilyTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/families", bytes.NewReader([]byte(`{invalid json`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", rec.Code)
	}
}

func TestCreateFamily_InvalidPartner1UUID(t *testing.T) {
	server := setupFamilyTestServer(t)

	body := map[string]interface{}{
		"partner1_id": "not-a-uuid",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/families", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", rec.Code)
	}
}

func TestCreateFamily_InvalidPartner2UUID(t *testing.T) {
	server := setupFamilyTestServer(t)

	person1 := createTestPerson(t, server, "John", "Doe")

	body := map[string]interface{}{
		"partner1_id": person1["id"],
		"partner2_id": "not-a-uuid",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/families", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", rec.Code)
	}
}

func TestGetFamily_InvalidUUID(t *testing.T) {
	server := setupFamilyTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/families/not-a-uuid", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", rec.Code)
	}
}

func TestUpdateFamily_InvalidUUID(t *testing.T) {
	server := setupFamilyTestServer(t)

	body := map[string]interface{}{
		"version": 1,
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/families/not-a-uuid", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", rec.Code)
	}
}

func TestUpdateFamily_InvalidJSON(t *testing.T) {
	server := setupFamilyTestServer(t)

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

	// Update with invalid JSON
	req = httptest.NewRequest(http.MethodPut, "/api/v1/families/"+created["id"].(string), bytes.NewReader([]byte(`{invalid`)))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", rec.Code)
	}
}

func TestUpdateFamily_NotFound(t *testing.T) {
	server := setupFamilyTestServer(t)

	body := map[string]interface{}{
		"version": 1,
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/families/00000000-0000-0000-0000-000000000001", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("Expected status 404, got %d", rec.Code)
	}
}

func TestDeleteFamily_InvalidUUID(t *testing.T) {
	server := setupFamilyTestServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/families/not-a-uuid?version=1", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", rec.Code)
	}
}

func TestDeleteFamily_NotFound(t *testing.T) {
	server := setupFamilyTestServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/families/00000000-0000-0000-0000-000000000001?version=1", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("Expected status 404, got %d", rec.Code)
	}
}

func TestAddChildToFamily_InvalidFamilyUUID(t *testing.T) {
	server := setupFamilyTestServer(t)

	child := createTestPerson(t, server, "Junior", "Doe")

	childBody := map[string]interface{}{
		"person_id": child["id"],
	}
	jsonBody, _ := json.Marshal(childBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/families/not-a-uuid/children", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", rec.Code)
	}
}

func TestAddChildToFamily_InvalidJSON(t *testing.T) {
	server := setupFamilyTestServer(t)

	parent1 := createTestPerson(t, server, "John", "Doe")
	parent2 := createTestPerson(t, server, "Jane", "Smith")

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

	// Add child with invalid JSON
	req = httptest.NewRequest(http.MethodPost, "/api/v1/families/"+family["id"].(string)+"/children", bytes.NewReader([]byte(`{invalid`)))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", rec.Code)
	}
}

func TestAddChildToFamily_InvalidChildUUID(t *testing.T) {
	server := setupFamilyTestServer(t)

	parent1 := createTestPerson(t, server, "John", "Doe")
	parent2 := createTestPerson(t, server, "Jane", "Smith")

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

	// Add child with invalid UUID
	childBody := map[string]interface{}{
		"person_id": "not-a-uuid",
	}
	jsonBody, _ = json.Marshal(childBody)

	req = httptest.NewRequest(http.MethodPost, "/api/v1/families/"+family["id"].(string)+"/children", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", rec.Code)
	}
}

func TestRemoveChildFromFamily_InvalidFamilyUUID(t *testing.T) {
	server := setupFamilyTestServer(t)

	child := createTestPerson(t, server, "Junior", "Doe")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/families/not-a-uuid/children/"+child["id"].(string)+"?version=1", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", rec.Code)
	}
}

func TestRemoveChildFromFamily_InvalidChildUUID(t *testing.T) {
	server := setupFamilyTestServer(t)

	parent1 := createTestPerson(t, server, "John", "Doe")
	parent2 := createTestPerson(t, server, "Jane", "Smith")

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

	// Remove child with invalid UUID
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/families/"+family["id"].(string)+"/children/not-a-uuid?version=1", http.NoBody)
	rec = httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", rec.Code)
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
