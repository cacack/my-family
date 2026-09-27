package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
)

// putFamily sends a family update and returns the recorder.
func putFamily(t *testing.T, server *api.Server, id string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/families/"+id, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	return rec
}

func createFamilyOf(t *testing.T, server *api.Server, partner1, partner2 string) string {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"partner1_id": partner1, "partner2_id": partner2, "relationship_type": "marriage"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/families", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create family: %d %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	return created["id"].(string)
}

// TestUpdateFamily_PartnersTypeAndDate is the API face of issue #848: the PUT
// accepts new partners and the response (read back from the projection) shows
// them together with the relationship type and marriage date.
func TestUpdateFamily_PartnersTypeAndDate(t *testing.T) {
	server := setupFamilyTestServer(t)
	p1 := createTestPerson(t, server, "Avery", "Placeholder")["id"].(string)
	p2 := createTestPerson(t, server, "Blake", "Sample")["id"].(string)
	p3 := createTestPerson(t, server, "Cameron", "Example")["id"].(string)
	familyID := createFamilyOf(t, server, p1, p2)

	rec := putFamily(t, server, familyID, map[string]any{
		"version": 1, "partner1_id": p3, "partner2_id": p1,
		"relationship_type": "partnership", "marriage_date": "ABT 1880",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT family: %d %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["partner1_id"] != p3 || got["partner2_id"] != p1 {
		t.Errorf("partners = %v / %v, want %s / %s", got["partner1_id"], got["partner2_id"], p3, p1)
	}
	// The detail read carries the denormalized partner name.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/families/"+familyID, http.NoBody)
	detailRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(detailRec, req)
	var detail map[string]any
	_ = json.Unmarshal(detailRec.Body.Bytes(), &detail)
	if partner1, _ := detail["partner1"].(map[string]any); partner1 == nil || partner1["given_name"] != "Cameron" {
		t.Errorf("partner1 = %v, want Cameron", detail["partner1"])
	}
	if got["relationship_type"] != "partnership" {
		t.Errorf("relationship_type = %v, want partnership", got["relationship_type"])
	}
	if md, _ := got["marriage_date"].(map[string]any); md == nil || md["raw"] != "ABT 1880" {
		t.Errorf("marriage_date = %v, want raw ABT 1880", got["marriage_date"])
	}
}

func TestUpdateFamily_PartnerValidation(t *testing.T) {
	server := setupFamilyTestServer(t)
	p1 := createTestPerson(t, server, "Avery", "Placeholder")["id"].(string)
	p2 := createTestPerson(t, server, "Blake", "Sample")["id"].(string)
	child := createTestPerson(t, server, "Casey", "Placeholder")["id"].(string)
	familyID := createFamilyOf(t, server, p1, p2)

	link, _ := json.Marshal(map[string]any{"person_id": child})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/families/%s/children", familyID), bytes.NewReader(link))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("link child: %d %s", rec.Code, rec.Body.String())
	}

	tests := []struct {
		name string
		body map[string]any
		want int
	}{
		{"unknown partner", map[string]any{"partner1_id": uuid.NewString()}, http.StatusBadRequest},
		{"same person twice", map[string]any{"partner1_id": p2}, http.StatusBadRequest},
		{"child as partner", map[string]any{"partner2_id": child}, http.StatusConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.body["version"] = 2 // the create plus the child link
			if rec := putFamily(t, server, familyID, tt.body); rec.Code != tt.want {
				t.Errorf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

// TestUpdateFamily_ClearPartner (#826): the PUT removes a partner with
// clear_partner1/clear_partner2, and refuses setting and clearing the same one.
func TestUpdateFamily_ClearPartner(t *testing.T) {
	server := setupFamilyTestServer(t)
	p1 := createTestPerson(t, server, "Avery", "Placeholder")["id"].(string)
	p2 := createTestPerson(t, server, "Blake", "Sample")["id"].(string)
	familyID := createFamilyOf(t, server, p1, p2)

	rec := putFamily(t, server, familyID, map[string]any{"version": 1, "partner1_id": p2, "clear_partner1": true})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("set+clear partner1: %d %s, want 400", rec.Code, rec.Body.String())
	}

	rec = putFamily(t, server, familyID, map[string]any{"version": 1, "clear_partner2": true})
	if rec.Code != http.StatusOK {
		t.Fatalf("clear partner2: %d %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if _, ok := got["partner2_id"]; ok {
		t.Errorf("partner2_id = %v, want absent after clearing", got["partner2_id"])
	}
	if got["partner1_id"] != p1 {
		t.Errorf("partner1_id = %v, want %s", got["partner1_id"], p1)
	}
}
