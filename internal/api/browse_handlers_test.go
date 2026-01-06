package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository/memory"
)

func setupBrowseTestServer() *api.Server {
	cfg := &config.Config{
		Port:      8080,
		LogFormat: "text",
	}
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	return api.NewServer(cfg, eventStore, readStore, nil)
}

func createBrowseTestPerson(t *testing.T, server *api.Server, givenName, surname, birthPlace, deathPlace string) string {
	body := `{"given_name":"` + givenName + `","surname":"` + surname + `"`
	if birthPlace != "" {
		body += `,"birth_place":"` + birthPlace + `"`
	}
	if deathPlace != "" {
		body += `,"death_place":"` + deathPlace + `"`
	}
	body += `}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("CreatePerson failed: %d - %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	return resp["id"].(string)
}

func TestBrowseSurnames(t *testing.T) {
	server := setupBrowseTestServer()

	// Create test persons
	createBrowseTestPerson(t, server, "John", "Smith", "", "")
	createBrowseTestPerson(t, server, "Jane", "Smith", "", "")
	createBrowseTestPerson(t, server, "Bob", "Anderson", "", "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/browse/surnames", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp struct {
		Items []struct {
			Surname string
			Count   int
		} `json:"items"`
		Total        int `json:"total"`
		LetterCounts []struct {
			Letter string
			Count  int
		} `json:"letter_counts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Total != 2 { // Smith, Anderson
		t.Errorf("Total = %d, want 2", resp.Total)
	}
	if len(resp.LetterCounts) == 0 {
		t.Error("Expected letter_counts")
	}
}

func TestBrowseSurnames_WithLetter(t *testing.T) {
	server := setupBrowseTestServer()

	createBrowseTestPerson(t, server, "John", "Smith", "", "")
	createBrowseTestPerson(t, server, "Jane", "Simpson", "", "")
	createBrowseTestPerson(t, server, "Bob", "Anderson", "", "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/browse/surnames?letter=S", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp struct {
		Items []struct {
			Surname string
			Count   int
		} `json:"items"`
		Total int `json:"total"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp.Total != 2 { // Smith, Simpson
		t.Errorf("Total = %d, want 2", resp.Total)
	}

	for _, item := range resp.Items {
		if !strings.HasPrefix(item.Surname, "S") {
			t.Errorf("Surname %s should start with S", item.Surname)
		}
	}
}

func TestGetPersonsBySurname(t *testing.T) {
	server := setupBrowseTestServer()

	createBrowseTestPerson(t, server, "John", "Smith", "", "")
	createBrowseTestPerson(t, server, "Jane", "Smith", "", "")
	createBrowseTestPerson(t, server, "Bob", "Jones", "", "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/browse/surnames/Smith/persons", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp struct {
		Items []struct {
			GivenName string `json:"given_name"`
			Surname   string
		} `json:"items"`
		Total int `json:"total"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp.Total != 2 {
		t.Errorf("Total = %d, want 2", resp.Total)
	}

	for _, item := range resp.Items {
		if item.Surname != "Smith" {
			t.Errorf("Surname = %s, want Smith", item.Surname)
		}
	}
}

func TestGetPersonsBySurname_URLEncoded(t *testing.T) {
	server := setupBrowseTestServer()

	createBrowseTestPerson(t, server, "John", "O'Brien", "", "")

	// URL encode the surname
	encoded := url.PathEscape("O'Brien")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/browse/surnames/"+encoded+"/persons", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp struct{ Total int }
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp.Total != 1 {
		t.Errorf("Total = %d, want 1", resp.Total)
	}
}

func TestGetPersonsBySurname_Pagination(t *testing.T) {
	server := setupBrowseTestServer()

	for i := 0; i < 5; i++ {
		createBrowseTestPerson(t, server, "Person", "Smith", "", "")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/browse/surnames/Smith/persons?limit=2&offset=0", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	var resp struct {
		Items  []struct{} `json:"items"`
		Total  int        `json:"total"`
		Limit  int        `json:"limit"`
		Offset int        `json:"offset"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp.Total != 5 {
		t.Errorf("Total = %d, want 5", resp.Total)
	}
	if len(resp.Items) != 2 {
		t.Errorf("Items = %d, want 2", len(resp.Items))
	}
	if resp.Limit != 2 {
		t.Errorf("Limit = %d, want 2", resp.Limit)
	}
}

func TestBrowsePlaces(t *testing.T) {
	server := setupBrowseTestServer()

	createBrowseTestPerson(t, server, "John", "Doe", "New York, USA", "")
	createBrowseTestPerson(t, server, "Jane", "Doe", "Boston, USA", "")
	createBrowseTestPerson(t, server, "Bob", "Smith", "London, UK", "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/browse/places", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp struct {
		Items []struct {
			Name        string `json:"name"`
			FullName    string `json:"full_name"`
			Count       int    `json:"count"`
			HasChildren bool   `json:"has_children"`
		} `json:"items"`
		Total int `json:"total"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp.Total == 0 {
		t.Error("Expected some places")
	}
}

func TestBrowsePlaces_WithParent(t *testing.T) {
	server := setupBrowseTestServer()

	createBrowseTestPerson(t, server, "John", "Doe", "New York, USA", "")

	// URL encode the parent
	encoded := url.QueryEscape("USA")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/browse/places?parent="+encoded, http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGetPersonsByPlace(t *testing.T) {
	server := setupBrowseTestServer()

	createBrowseTestPerson(t, server, "John", "Doe", "New York, USA", "")
	createBrowseTestPerson(t, server, "Jane", "Doe", "Boston, USA", "")
	createBrowseTestPerson(t, server, "Bob", "Smith", "London, UK", "")

	// URL encode the place
	encoded := url.PathEscape("USA")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/browse/places/"+encoded+"/persons", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp struct {
		Items []struct{} `json:"items"`
		Total int        `json:"total"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp.Total != 2 { // John and Jane are from USA
		t.Errorf("Total = %d, want 2", resp.Total)
	}
}

func TestGetPersonsByPlace_DeathPlace(t *testing.T) {
	server := setupBrowseTestServer()

	createBrowseTestPerson(t, server, "John", "Doe", "", "Paris, France")

	encoded := url.PathEscape("France")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/browse/places/"+encoded+"/persons", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	var resp struct{ Total int }
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp.Total != 1 {
		t.Errorf("Total = %d, want 1", resp.Total)
	}
}

func TestGetPersonsByPlace_Pagination(t *testing.T) {
	server := setupBrowseTestServer()

	for i := 0; i < 5; i++ {
		createBrowseTestPerson(t, server, "Person", "Test", "New York, USA", "")
	}

	encoded := url.PathEscape("USA")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/browse/places/"+encoded+"/persons?limit=2&offset=0", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	var resp struct {
		Items  []struct{} `json:"items"`
		Total  int        `json:"total"`
		Limit  int        `json:"limit"`
		Offset int        `json:"offset"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp.Total != 5 {
		t.Errorf("Total = %d, want 5", resp.Total)
	}
	if len(resp.Items) != 2 {
		t.Errorf("Items = %d, want 2", len(resp.Items))
	}
}

func TestBrowseSurnames_EmptyDatabase(t *testing.T) {
	server := setupBrowseTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/browse/surnames", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp struct{ Total int }
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp.Total != 0 {
		t.Errorf("Total = %d, want 0", resp.Total)
	}
}

func TestBrowsePlaces_EmptyDatabase(t *testing.T) {
	server := setupBrowseTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/browse/places", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp struct{ Total int }
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp.Total != 0 {
		t.Errorf("Total = %d, want 0", resp.Total)
	}
}
