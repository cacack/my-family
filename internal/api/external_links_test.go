package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
	"github.com/google/uuid"
)

// TestGetPerson_ExternalLinks verifies that a person's stored GEDCOM 7.0
// external identifiers are exposed on the PersonDetail response with a
// server-resolved display label, and a browsable URL only when the type URI
// maps to a known system.
func TestGetPerson_ExternalLinks(t *testing.T) {
	cfg := &config.Config{Port: 8080, LogFormat: "text"}
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	snapshotStore := memory.NewSnapshotStore(eventStore)
	server := api.NewServer(cfg, eventStore, readStore, snapshotStore, nil)

	// Create a person via the API.
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", strings.NewReader(`{"given_name":"Ada","surname":"Lovelace"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(createRec, createReq)
	var created map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("create person: %d: %s", createRec.Code, createRec.Body.String())
	}
	personID, err := uuid.Parse(created["id"].(string))
	if err != nil {
		t.Fatalf("parse person id: %v", err)
	}

	// Seed external IDs directly in the read model (they are import-only; no create API).
	if err := readStore.ReplacePersonExternalIDs(context.Background(), personID, []repository.PersonExternalIDReadModel{
		{PersonID: personID, Sequence: 0, Value: "KWCJ-QN7", Type: "http://www.familysearch.org/ark"},
		{PersonID: personID, Sequence: 1, Value: "X99", Type: "http://example.com/unknown-system"},
	}); err != nil {
		t.Fatalf("seed external ids: %v", err)
	}

	// GET the person detail.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/"+personID.String(), http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get person: %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		ExternalIDs []struct {
			Value string  `json:"value"`
			Type  string  `json:"type"`
			Label string  `json:"label"`
			URL   *string `json:"url"`
		} `json:"external_ids"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.ExternalIDs) != 2 {
		t.Fatalf("external_ids len = %d, want 2: %s", len(resp.ExternalIDs), rec.Body.String())
	}

	byValue := map[string]struct {
		Label string
		URL   *string
	}{}
	for _, e := range resp.ExternalIDs {
		byValue[e.Value] = struct {
			Label string
			URL   *string
		}{e.Label, e.URL}
	}

	// Known system: resolved label + browsable URL.
	fs := byValue["KWCJ-QN7"]
	if fs.Label != "FamilySearch" {
		t.Errorf("label = %q, want FamilySearch", fs.Label)
	}
	if fs.URL == nil || *fs.URL != "https://www.familysearch.org/tree/person/details/KWCJ-QN7" {
		t.Errorf("url = %v, want the FamilySearch record URL", fs.URL)
	}

	// Unknown system: label falls back to the raw type URI, url omitted.
	unk := byValue["X99"]
	if unk.Label != "http://example.com/unknown-system" {
		t.Errorf("label = %q, want the raw type URI", unk.Label)
	}
	if unk.URL != nil {
		t.Errorf("url = %v, want nil for an unrecognized system", *unk.URL)
	}
}

// TestGetEntity_ExternalLinks verifies that family, source, and repository
// details expose stored GEDCOM 7.0 external identifiers with server-resolved
// labels and URLs, mirroring the person behavior above.
func TestGetEntity_ExternalLinks(t *testing.T) {
	cfg := &config.Config{Port: 8080, LogFormat: "text"}
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	snapshotStore := memory.NewSnapshotStore(eventStore)
	server := api.NewServer(cfg, eventStore, readStore, snapshotStore, nil)
	ctx := context.Background()

	familyID, sourceID, repoID := uuid.New(), uuid.New(), uuid.New()

	if err := readStore.SaveFamily(ctx, &repository.FamilyReadModel{ID: familyID}); err != nil {
		t.Fatalf("seed family: %v", err)
	}
	if err := readStore.ReplaceFamilyExternalIDs(ctx, familyID, []repository.FamilyExternalIDReadModel{
		{FamilyID: familyID, Sequence: 0, Value: "F100", Type: "http://www.familysearch.org/ark"},
	}); err != nil {
		t.Fatalf("seed family external ids: %v", err)
	}

	if err := readStore.SaveSource(ctx, &repository.SourceReadModel{ID: sourceID, SourceType: "book", Title: "Census"}); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	if err := readStore.ReplaceSourceExternalIDs(ctx, sourceID, []repository.SourceExternalIDReadModel{
		{SourceID: sourceID, Sequence: 0, Value: "S100", Type: "http://example.com/unknown-system"},
	}); err != nil {
		t.Fatalf("seed source external ids: %v", err)
	}

	if err := readStore.SaveRepository(ctx, &repository.RepositoryReadModel{ID: repoID, Name: "Archives"}); err != nil {
		t.Fatalf("seed repository: %v", err)
	}
	if err := readStore.ReplaceRepositoryExternalIDs(ctx, repoID, []repository.RepositoryExternalIDReadModel{
		{RepositoryID: repoID, Sequence: 0, Value: "R100", Type: "http://www.familysearch.org/ark"},
	}); err != nil {
		t.Fatalf("seed repository external ids: %v", err)
	}

	tests := []struct {
		name      string
		path      string
		wantValue string
		wantLabel string
		wantURL   bool
	}{
		// Known system: resolved label + browsable URL.
		{"family", "/api/v1/families/" + familyID.String(), "F100", "FamilySearch", true},
		// Unknown system: label falls back to the raw type URI, url omitted.
		{"source", "/api/v1/sources/" + sourceID.String(), "S100", "http://example.com/unknown-system", false},
		{"repository", "/api/v1/repositories/" + repoID.String(), "R100", "FamilySearch", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, http.NoBody)
			rec := httptest.NewRecorder()
			server.Echo().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s: %d: %s", tt.path, rec.Code, rec.Body.String())
			}

			var resp struct {
				ExternalIDs []struct {
					Value string  `json:"value"`
					Type  string  `json:"type"`
					Label string  `json:"label"`
					URL   *string `json:"url"`
				} `json:"external_ids"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(resp.ExternalIDs) != 1 {
				t.Fatalf("external_ids len = %d, want 1: %s", len(resp.ExternalIDs), rec.Body.String())
			}
			got := resp.ExternalIDs[0]
			if got.Value != tt.wantValue {
				t.Errorf("value = %q, want %q", got.Value, tt.wantValue)
			}
			if got.Label != tt.wantLabel {
				t.Errorf("label = %q, want %q", got.Label, tt.wantLabel)
			}
			if tt.wantURL && got.URL == nil {
				t.Error("url = nil, want a browsable URL")
			}
			if !tt.wantURL && got.URL != nil {
				t.Errorf("url = %q, want omitted for unrecognized system", *got.URL)
			}
		})
	}
}
