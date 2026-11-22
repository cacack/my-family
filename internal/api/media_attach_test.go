package api_test

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
)

// importReference imports referenceMediaGedcom and returns the one
// reference-only media item it makes, marked private.
func importReference(t *testing.T, server *api.Server) map[string]any {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "reference.ged")
	_, _ = io.WriteString(part, referenceMediaGedcom)
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("import status = %d: %s", rec.Code, rec.Body.String())
	}

	people := decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/search?q=Reference", ""))["items"].([]any)
	if len(people) != 1 {
		t.Fatalf("search found %d people, want 1", len(people))
	}
	personID := people[0].(map[string]any)["id"].(string)
	items := decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/persons/"+personID+"/media", ""))["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("person has %d media items, want 1", len(items))
	}
	item := items[0].(map[string]any)
	id := item["id"].(string)
	rec = do(t, server, http.MethodPut, "/api/v1/media/"+id, fmt.Sprintf(`{"private":true,"version":%v}`, item["version"]))
	if rec.Code != http.StatusOK {
		t.Fatalf("mark private status = %d: %s", rec.Code, rec.Body.String())
	}
	return decodeJSON(t, rec)
}

// attachFile sends file as the content of media id.
func attachFile(t *testing.T, server *api.Server, id, filename string, file []byte) *httptest.ResponseRecorder {
	t.Helper()
	req, err := createMultipartRequest("/api/v1/media/"+id+"/content", "file", filename, file, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Method = http.MethodPut
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	return rec
}

// TestAttachMediaContent fills an imported reference with an uploaded file in
// place (#1504): the same item then serves the file and a thumbnail, and
// keeps its title, owner, file references and private mark, which a GEDCOM
// export still writes.
func TestAttachMediaContent(t *testing.T) {
	server := setupTestServer()
	ref := importReference(t, server)
	id := ref["id"].(string)
	jpegData := createTestJPEGImage()

	rec := attachFile(t, server, id, "wedding.jpg", jpegData)
	if rec.Code != http.StatusOK {
		t.Fatalf("attach status = %d: %s", rec.Code, rec.Body.String())
	}
	attached := decodeJSON(t, rec)

	got := decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/media/"+id, ""))
	for _, m := range []map[string]any{attached, got} {
		if m["id"] != id || m["file_size"] != float64(len(jpegData)) || m["mime_type"] != "image/jpeg" ||
			m["filename"] != "wedding.jpg" || m["has_content"] != true || m["has_thumbnail"] != true {
			t.Errorf("item = %v; want id %s, file_size %d, image/jpeg, wedding.jpg, has_content and has_thumbnail", m, id, len(jpegData))
		}
		if m["version"] != ref["version"].(float64)+1 {
			t.Errorf("version = %v, want %v", m["version"], ref["version"].(float64)+1)
		}
		files, _ := m["files"].([]any)
		if m["title"] != "Wedding" || m["entity_id"] != ref["entity_id"] || m["private"] != true ||
			len(files) != 1 || files[0].(map[string]any)["path"] != "photos/wedding.jpg" {
			t.Errorf("item = %v; want the title, owner, files and private mark kept", m)
		}
	}

	content := do(t, server, http.MethodGet, "/api/v1/media/"+id+"/content", "")
	if content.Code != http.StatusOK || !bytes.Equal(content.Body.Bytes(), jpegData) {
		t.Errorf("content = %d with %d bytes, want 200 with the %d uploaded bytes", content.Code, content.Body.Len(), len(jpegData))
	}
	if thumb := do(t, server, http.MethodGet, "/api/v1/media/"+id+"/thumbnail", ""); thumb.Code != http.StatusOK || thumb.Body.Len() == 0 {
		t.Errorf("thumbnail = %d with %d bytes, want 200 with a thumbnail", thumb.Code, thumb.Body.Len())
	}

	export := do(t, server, http.MethodGet, "/api/v1/gedcom/export", "")
	if export.Code != http.StatusOK {
		t.Fatalf("export status = %d", export.Code)
	}
	for _, line := range []string{"1 FILE photos/wedding.jpg", "2 TRAN photos/wedding-small.jpg"} {
		if !strings.Contains(export.Body.String(), line) {
			t.Errorf("export lacks %q:\n%s", line, export.Body.String())
		}
	}
	if strings.Contains(export.Body.String(), "FILE wedding.jpg") {
		t.Errorf("export names the uploaded filename instead of the imported path:\n%s", export.Body.String())
	}

	// A second attach is refused: replacing content is not supported.
	again := attachFile(t, server, id, "other.jpg", jpegData)
	if again.Code != http.StatusConflict || decodeJSON(t, again)["code"] != "has_content" {
		t.Errorf("second attach = %d %s, want 409 has_content", again.Code, again.Body.String())
	}
}

// TestAttachMediaContent_Refusals refuses a file uploadPersonMedia would
// refuse, and an unknown item, leaving the reference unchanged.
func TestAttachMediaContent_Refusals(t *testing.T) {
	server := setupTestServer()
	ref := importReference(t, server)
	id := ref["id"].(string)

	tests := []struct {
		name     string
		id       string
		file     []byte
		wantCode int
	}{
		{"empty file", id, nil, http.StatusBadRequest},
		{"oversized file", id, bytes.Repeat([]byte{0xFF}, 10<<20+1), http.StatusBadRequest},
		{"unsupported file", id, []byte("just some text"), http.StatusBadRequest},
		{"unknown item", "00000000-0000-0000-0000-000000000001", createTestJPEGImage(), http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := attachFile(t, server, tt.id, "f.bin", tt.file); rec.Code != tt.wantCode {
				t.Errorf("status = %d (%s), want %d", rec.Code, rec.Body.String(), tt.wantCode)
			}
			got := decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/media/"+id, ""))
			if got["has_content"] != false || got["version"] != ref["version"] {
				t.Errorf("after the refusal: has_content %v, version %v; want false and %v", got["has_content"], got["version"], ref["version"])
			}
		})
	}

	// A body with no file part is refused before any lookup.
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	_ = writer.WriteField("title", "No file")
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/media/"+id+"/content", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("no file part: status = %d, want 400", rec.Code)
	}
}
