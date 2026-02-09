package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
	"github.com/google/uuid"
)

func setupBrowseTestServer() *api.Server {
	cfg := &config.Config{
		Port:      8080,
		LogFormat: "text",
	}
	eventStore := memory.NewEventStore()
	snapshotStore := memory.NewSnapshotStore(eventStore)
	readStore := memory.NewReadModelStore()
	return api.NewServer(cfg, eventStore, readStore, snapshotStore, nil)
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

// setupBrowseTestServerWithStore returns both the server and the read model store
// so tests can directly insert life events for cemetery testing.
func setupBrowseTestServerWithStore() (*api.Server, *memory.ReadModelStore) {
	cfg := &config.Config{
		Port:      8080,
		LogFormat: "text",
	}
	eventStore := memory.NewEventStore()
	snapshotStore := memory.NewSnapshotStore(eventStore)
	readStore := memory.NewReadModelStore()
	return api.NewServer(cfg, eventStore, readStore, snapshotStore, nil), readStore
}

// createCemeteryTestData creates persons with burial/cremation events for cemetery tests.
func createCemeteryTestData(t *testing.T, readStore *memory.ReadModelStore) {
	t.Helper()
	ctx := context.Background()

	persons := []struct {
		id        uuid.UUID
		givenName string
		surname   string
	}{
		{uuid.New(), "John", "Smith"},
		{uuid.New(), "Jane", "Doe"},
		{uuid.New(), "Bob", "Jones"},
	}

	for _, p := range persons {
		err := readStore.SavePerson(ctx, &repository.PersonReadModel{
			ID:        p.id,
			GivenName: p.givenName,
			Surname:   p.surname,
			FullName:  p.givenName + " " + p.surname,
			Version:   1,
			UpdatedAt: time.Now(),
		})
		if err != nil {
			t.Fatalf("SavePerson() failed: %v", err)
		}
	}

	events := []*repository.EventReadModel{
		{
			ID:        uuid.New(),
			OwnerType: "person",
			OwnerID:   persons[0].id,
			FactType:  domain.FactPersonBurial,
			Place:     "Oakwood Cemetery, Springfield, IL",
			Version:   1,
			CreatedAt: time.Now(),
		},
		{
			ID:        uuid.New(),
			OwnerType: "person",
			OwnerID:   persons[1].id,
			FactType:  domain.FactPersonBurial,
			Place:     "Oakwood Cemetery, Springfield, IL",
			Version:   1,
			CreatedAt: time.Now(),
		},
		{
			ID:        uuid.New(),
			OwnerType: "person",
			OwnerID:   persons[2].id,
			FactType:  domain.FactPersonCremation,
			Place:     "Rose Hills Memorial Park",
			Version:   1,
			CreatedAt: time.Now(),
		},
	}

	for _, e := range events {
		if err := readStore.SaveEvent(ctx, e); err != nil {
			t.Fatalf("SaveEvent() failed: %v", err)
		}
	}
}

func TestBrowseCemeteries(t *testing.T) {
	server, readStore := setupBrowseTestServerWithStore()
	createCemeteryTestData(t, readStore)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/browse/cemeteries", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp struct {
		Items []struct {
			Place string `json:"place"`
			Count int    `json:"count"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Total != 2 {
		t.Errorf("Total = %d, want 2", resp.Total)
	}

	// Verify items are present
	found := map[string]int{}
	for _, item := range resp.Items {
		found[item.Place] = item.Count
	}
	if found["Oakwood Cemetery, Springfield, IL"] != 2 {
		t.Errorf("Oakwood count = %d, want 2", found["Oakwood Cemetery, Springfield, IL"])
	}
	if found["Rose Hills Memorial Park"] != 1 {
		t.Errorf("Rose Hills count = %d, want 1", found["Rose Hills Memorial Park"])
	}
}

func TestBrowseCemeteries_EmptyDatabase(t *testing.T) {
	server, _ := setupBrowseTestServerWithStore()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/browse/cemeteries", http.NoBody)
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

func TestGetPersonsByCemetery(t *testing.T) {
	server, readStore := setupBrowseTestServerWithStore()
	createCemeteryTestData(t, readStore)

	encoded := url.PathEscape("Oakwood Cemetery, Springfield, IL")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/browse/cemeteries/"+encoded+"/persons", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp struct {
		Items []struct {
			GivenName string `json:"given_name"`
			Surname   string `json:"surname"`
		} `json:"items"`
		Total int `json:"total"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp.Total != 2 {
		t.Errorf("Total = %d, want 2", resp.Total)
	}
}

func TestGetPersonsByCemetery_URLEncoded(t *testing.T) {
	server, readStore := setupBrowseTestServerWithStore()
	createCemeteryTestData(t, readStore)

	encoded := url.PathEscape("Rose Hills Memorial Park")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/browse/cemeteries/"+encoded+"/persons", http.NoBody)
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

func TestGetPersonsByCemetery_Pagination(t *testing.T) {
	server, readStore := setupBrowseTestServerWithStore()
	ctx := context.Background()

	// Create 5 persons with burial at the same place
	for i := 0; i < 5; i++ {
		personID := uuid.New()
		err := readStore.SavePerson(ctx, &repository.PersonReadModel{
			ID:        personID,
			GivenName: "Person",
			Surname:   "Test",
			FullName:  "Person Test",
			Version:   1,
			UpdatedAt: time.Now(),
		})
		if err != nil {
			t.Fatalf("SavePerson() failed: %v", err)
		}
		err = readStore.SaveEvent(ctx, &repository.EventReadModel{
			ID:        uuid.New(),
			OwnerType: "person",
			OwnerID:   personID,
			FactType:  domain.FactPersonBurial,
			Place:     "Test Cemetery",
			Version:   1,
			CreatedAt: time.Now(),
		})
		if err != nil {
			t.Fatalf("SaveEvent() failed: %v", err)
		}
	}

	encoded := url.PathEscape("Test Cemetery")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/browse/cemeteries/"+encoded+"/persons?limit=2&offset=0", http.NoBody)
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
