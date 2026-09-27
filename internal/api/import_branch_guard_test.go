package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestImport_RefusesBranchScope pins the #825 backstop: GEDCOM import always
// writes the mainline, so a request that carries a branch scope is refused
// before anything is written, on both the plain and the streaming route.
func TestImport_RefusesBranchScope(t *testing.T) {
	cases := []struct {
		name  string
		route string
	}{
		{"import with branch id", "/api/v1/gedcom/import?branch=11111111-1111-1111-1111-111111111111"},
		{"import with empty branch", "/api/v1/gedcom/import?branch="},
		{"stream with branch id", "/api/v1/gedcom/import/stream?branch=11111111-1111-1111-1111-111111111111"},
		{"stream with empty branch", "/api/v1/gedcom/import/stream?branch="},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := setupImportTestServer(t)

			body := &bytes.Buffer{}
			writer := multipart.NewWriter(body)
			part, err := writer.CreateFormFile("file", "test.ged")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(part, testGedcom); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}

			req := httptest.NewRequest(http.MethodPost, tc.route, body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			rec := httptest.NewRecorder()
			server.Echo().ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			var apiErr struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &apiErr); err != nil {
				t.Fatalf("response is not a JSON error: %v (%s)", err, rec.Body.String())
			}
			if apiErr.Code != "BAD_REQUEST" {
				t.Errorf("code = %q, want BAD_REQUEST", apiErr.Code)
			}
			if !strings.Contains(apiErr.Message, "mainline") {
				t.Errorf("message should explain the mainline-only import, got %q", apiErr.Message)
			}

			// Nothing reached the mainline.
			listRec := httptest.NewRecorder()
			server.Echo().ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/api/v1/persons", http.NoBody))
			if listRec.Code != http.StatusOK {
				t.Fatalf("list persons: %d %s", listRec.Code, listRec.Body.String())
			}
			var list struct {
				Total int `json:"total"`
			}
			if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil {
				t.Fatal(err)
			}
			if list.Total != 0 {
				t.Errorf("refused import still wrote %d persons", list.Total)
			}
		})
	}
}

// TestImport_UnscopedStillWorks confirms the guard leaves an unscoped import,
// and unrelated query parameters, alone.
func TestImport_UnscopedStillWorks(t *testing.T) {
	server := setupImportTestServer(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "test.ged")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, testGedcom); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import?unrelated=1", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}
