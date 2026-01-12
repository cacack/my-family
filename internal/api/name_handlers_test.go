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

func setupNameTestServer() *api.Server {
	cfg := &config.Config{
		Port:      8080,
		LogFormat: "text",
	}
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	return api.NewServer(cfg, eventStore, readStore, nil)
}

func createPersonForNameTest(t *testing.T, server *api.Server) string {
	t.Helper()
	body := `{"given_name":"John","surname":"Doe"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("Failed to create person: %s", rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	return resp["id"].(string)
}

func TestGetPersonNames(t *testing.T) {
	server := setupNameTestServer()
	personID := createPersonForNameTest(t, server)

	// Get names for the person (should have one primary name created with person)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+personID+"/names", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	total := int(resp["total"].(float64))
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}

	items := resp["items"].([]any)
	if len(items) != 1 {
		t.Errorf("items count = %d, want 1", len(items))
	}

	// Verify the name is primary
	name := items[0].(map[string]any)
	if name["is_primary"] != true {
		t.Error("Expected first name to be primary")
	}
	if name["given_name"] != "John" {
		t.Errorf("given_name = %v, want John", name["given_name"])
	}
}

func TestGetPersonNames_InvalidID(t *testing.T) {
	server := setupNameTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/invalid-uuid/names", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAddPersonName(t *testing.T) {
	server := setupNameTestServer()
	personID := createPersonForNameTest(t, server)

	// Add a maiden name
	body := `{"given_name":"Jane","surname":"Smith","name_type":"birth"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/"+personID+"/names", strings.NewReader(body))
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

	if resp["given_name"] != "Jane" {
		t.Errorf("given_name = %v, want Jane", resp["given_name"])
	}
	if resp["surname"] != "Smith" {
		t.Errorf("surname = %v, want Smith", resp["surname"])
	}
	if resp["name_type"] != "birth" {
		t.Errorf("name_type = %v, want birth", resp["name_type"])
	}
	// Second name should not be primary
	if resp["is_primary"] != false {
		t.Error("Expected second name to not be primary")
	}
}

func TestAddPersonName_AsPrimary(t *testing.T) {
	server := setupNameTestServer()
	personID := createPersonForNameTest(t, server)

	// Add a name as primary (should demote existing primary)
	body := `{"given_name":"Johnny","surname":"Doe","is_primary":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/"+personID+"/names", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp["is_primary"] != true {
		t.Error("Expected new name to be primary")
	}

	// Verify we now have 2 names
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+personID+"/names", http.NoBody)
	listRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(listRec, listReq)

	var listResp map[string]any
	json.Unmarshal(listRec.Body.Bytes(), &listResp)

	total := int(listResp["total"].(float64))
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}

	// Count primary names (should be exactly 1)
	items := listResp["items"].([]any)
	primaryCount := 0
	for _, item := range items {
		name := item.(map[string]any)
		if name["is_primary"] == true {
			primaryCount++
		}
	}
	if primaryCount != 1 {
		t.Errorf("primary count = %d, want 1", primaryCount)
	}
}

func TestAddPersonName_InvalidPersonID(t *testing.T) {
	server := setupNameTestServer()

	body := `{"given_name":"Test","surname":"Name"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/invalid-uuid/names", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAddPersonName_MissingGivenName(t *testing.T) {
	server := setupNameTestServer()
	personID := createPersonForNameTest(t, server)

	body := `{"surname":"Smith"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/"+personID+"/names", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdatePersonName(t *testing.T) {
	server := setupNameTestServer()
	personID := createPersonForNameTest(t, server)

	// Get the existing name ID
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+personID+"/names", http.NoBody)
	listRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(listRec, listReq)

	var listResp map[string]any
	json.Unmarshal(listRec.Body.Bytes(), &listResp)
	items := listResp["items"].([]any)
	nameID := items[0].(map[string]any)["id"].(string)

	// Update the name
	body := `{"given_name":"Jonathan","nickname":"Jon"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/persons/"+personID+"/names/"+nameID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp["given_name"] != "Jonathan" {
		t.Errorf("given_name = %v, want Jonathan", resp["given_name"])
	}
	if resp["nickname"] != "Jon" {
		t.Errorf("nickname = %v, want Jon", resp["nickname"])
	}
}

func TestUpdatePersonName_InvalidPersonID(t *testing.T) {
	server := setupNameTestServer()

	body := `{"given_name":"Test"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/persons/invalid-uuid/names/00000000-0000-0000-0000-000000000001", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdatePersonName_InvalidNameID(t *testing.T) {
	server := setupNameTestServer()
	personID := createPersonForNameTest(t, server)

	body := `{"given_name":"Test"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/persons/"+personID+"/names/invalid-uuid", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeletePersonName(t *testing.T) {
	server := setupNameTestServer()
	personID := createPersonForNameTest(t, server)

	// Add a second name (non-primary)
	addBody := `{"given_name":"Jane","surname":"Doe"}`
	addReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons/"+personID+"/names", strings.NewReader(addBody))
	addReq.Header.Set("Content-Type", "application/json")
	addRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(addRec, addReq)

	var addResp map[string]any
	json.Unmarshal(addRec.Body.Bytes(), &addResp)
	nameID := addResp["id"].(string)

	// Delete the second name
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/persons/"+personID+"/names/"+nameID, http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	// Verify we're back to 1 name
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+personID+"/names", http.NoBody)
	listRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(listRec, listReq)

	var listResp map[string]any
	json.Unmarshal(listRec.Body.Bytes(), &listResp)

	total := int(listResp["total"].(float64))
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
}

func TestDeletePersonName_InvalidPersonID(t *testing.T) {
	server := setupNameTestServer()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/persons/invalid-uuid/names/00000000-0000-0000-0000-000000000001", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeletePersonName_InvalidNameID(t *testing.T) {
	server := setupNameTestServer()
	personID := createPersonForNameTest(t, server)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/persons/"+personID+"/names/invalid-uuid", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeletePersonName_CannotDeletePrimary(t *testing.T) {
	server := setupNameTestServer()
	personID := createPersonForNameTest(t, server)

	// Add a second name so we have 2 names
	addBody := `{"given_name":"Jane","surname":"Doe"}`
	addReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons/"+personID+"/names", strings.NewReader(addBody))
	addReq.Header.Set("Content-Type", "application/json")
	addRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(addRec, addReq)

	// Get the primary name ID
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+personID+"/names", http.NoBody)
	listRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(listRec, listReq)

	var listResp map[string]any
	json.Unmarshal(listRec.Body.Bytes(), &listResp)
	items := listResp["items"].([]any)

	var primaryNameID string
	for _, item := range items {
		name := item.(map[string]any)
		if name["is_primary"] == true {
			primaryNameID = name["id"].(string)
			break
		}
	}

	// Try to delete the primary name
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/persons/"+personID+"/names/"+primaryNameID, http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	// Should fail
	if rec.Code == http.StatusNoContent {
		t.Error("Should not be able to delete primary name")
	}
}

func TestDeletePersonName_CannotDeleteLast(t *testing.T) {
	server := setupNameTestServer()
	personID := createPersonForNameTest(t, server)

	// Get the only name ID
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+personID+"/names", http.NoBody)
	listRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(listRec, listReq)

	var listResp map[string]any
	json.Unmarshal(listRec.Body.Bytes(), &listResp)
	items := listResp["items"].([]any)
	nameID := items[0].(map[string]any)["id"].(string)

	// Try to delete the only name
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/persons/"+personID+"/names/"+nameID, http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	// Should fail
	if rec.Code == http.StatusNoContent {
		t.Error("Should not be able to delete the last name")
	}
}

func TestAddPersonName_WithAllFields(t *testing.T) {
	server := setupNameTestServer()
	personID := createPersonForNameTest(t, server)

	// Add a name with all optional fields
	body := `{
		"given_name":"Dr",
		"surname":"Smith",
		"name_prefix":"Dr.",
		"name_suffix":"Jr.",
		"surname_prefix":"von",
		"nickname":"Smitty",
		"name_type":"professional"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons/"+personID+"/names", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp["name_prefix"] != "Dr." {
		t.Errorf("name_prefix = %v, want Dr.", resp["name_prefix"])
	}
	if resp["name_suffix"] != "Jr." {
		t.Errorf("name_suffix = %v, want Jr.", resp["name_suffix"])
	}
	if resp["surname_prefix"] != "von" {
		t.Errorf("surname_prefix = %v, want von", resp["surname_prefix"])
	}
	if resp["nickname"] != "Smitty" {
		t.Errorf("nickname = %v, want Smitty", resp["nickname"])
	}
	if resp["name_type"] != "professional" {
		t.Errorf("name_type = %v, want professional", resp["name_type"])
	}
}
