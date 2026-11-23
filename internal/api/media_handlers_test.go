package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cacack/my-family/internal/repository"
)

// createTestJPEGImage creates a small test JPEG image.
func createTestJPEGImage() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{100, 150, 200, 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
	return buf.Bytes()
}

// createMultipartRequest creates a multipart form request for file upload.
func createMultipartRequest(url, fieldName, filename string, fileData []byte, fields map[string]string) (*http.Request, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add file
	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		return nil, err
	}
	_, err = io.Copy(part, bytes.NewReader(fileData))
	if err != nil {
		return nil, err
	}

	// Add other fields
	for key, val := range fields {
		_ = writer.WriteField(key, val)
	}

	err = writer.Close()
	if err != nil {
		return nil, err
	}

	req := httptest.NewRequest(http.MethodPost, url, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, nil
}

func TestUploadPersonMedia(t *testing.T) {
	server := setupTestServer()

	// First create a person
	personBody := `{"given_name":"John","surname":"Doe","gender":"male"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", bytes.NewReader([]byte(personBody)))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	if personRec.Code != http.StatusCreated {
		t.Fatalf("Failed to create person: %d - %s", personRec.Code, personRec.Body.String())
	}

	var personResp map[string]any
	_ = json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	// Upload media
	jpegData := createTestJPEGImage()
	fields := map[string]string{
		"title":       "Family Portrait",
		"description": "A family portrait from 1920",
		"media_type":  "photo",
	}

	req, err := createMultipartRequest(
		fmt.Sprintf("/api/v1/persons/%s/media", personID),
		"file",
		"portrait.jpg",
		jpegData,
		fields,
	)
	if err != nil {
		t.Fatalf("Failed to create multipart request: %v", err)
	}

	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var mediaResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &mediaResp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// The form's title, description and media type are stored as sent (#1000).
	assertUploadedFields(t, "upload", mediaResp)
	if mediaResp["id"] == nil || mediaResp["id"] == "" {
		t.Error("Expected non-empty id")
	}

	// An upload stores content, so it reports has_content (#1583).
	if mediaResp["has_content"] != true {
		t.Errorf("upload has_content = %v, want true", mediaResp["has_content"])
	}

	// An uploaded image reports its thumbnail on upload, get and list (#903).
	if mediaResp["has_thumbnail"] != true {
		t.Errorf("upload has_thumbnail = %v, want true", mediaResp["has_thumbnail"])
	}
	getRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/media/%s", mediaResp["id"]), http.NoBody))
	var getResp map[string]any
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil || getResp["has_thumbnail"] != true {
		t.Errorf("get has_thumbnail = %v (status %d, err %v), want true", getResp["has_thumbnail"], getRec.Code, err)
	}
	if getResp["has_content"] != true {
		t.Errorf("get has_content = %v, want true", getResp["has_content"])
	}
	// has_content is derived from the stored size while the download gate
	// reads the stored bytes; the two must agree. The reference-only half is
	// TestReferenceOnlyMedia_ContentAndThumbnailNotFound.
	contentRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(contentRec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/media/%s/content", mediaResp["id"]), http.NoBody))
	if contentRec.Code != http.StatusOK || contentRec.Body.Len() == 0 {
		t.Errorf("content status = %d with %d bytes, want %d and the stored bytes for an item with has_content true",
			contentRec.Code, contentRec.Body.Len(), http.StatusOK)
	}
	assertUploadedFields(t, "get", getResp)
	listRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/persons/%s/media", personID), http.NoBody))
	var listResp struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil || len(listResp.Items) != 1 ||
		listResp.Items[0]["has_thumbnail"] != true || listResp.Items[0]["has_content"] != true {
		t.Errorf("list = %s (err %v), want one item with has_thumbnail and has_content true", listRec.Body.String(), err)
	}
}

func TestUploadPersonMedia_MissingTitle(t *testing.T) {
	server := setupTestServer()

	// Create a person
	personBody := `{"given_name":"Jane","surname":"Doe","gender":"female"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", bytes.NewReader([]byte(personBody)))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	var personResp map[string]any
	_ = json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	// Upload media without title field: the spec requires one, so it is refused.
	jpegData := createTestJPEGImage()
	fields := map[string]string{
		"description": "Missing title",
	}

	req, _ := createMultipartRequest(
		fmt.Sprintf("/api/v1/persons/%s/media", personID),
		"file",
		"test.jpg",
		jpegData,
		fields,
	)

	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d. Body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("title is required")) {
		t.Errorf("Body = %s, want it to say the title is required", rec.Body.String())
	}

	// Nothing was stored.
	listRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/persons/%s/media", personID), http.NoBody))
	if !bytes.Contains(listRec.Body.Bytes(), []byte(`"total":0`)) {
		t.Errorf("list after refused upload = %s, want total 0", listRec.Body.String())
	}
}

// assertUploadedFields checks a media payload carries the form fields
// TestUploadPersonMedia sends.
func assertUploadedFields(t *testing.T, label string, media map[string]any) {
	t.Helper()
	for field, want := range map[string]string{
		"title":       "Family Portrait",
		"description": "A family portrait from 1920",
		"media_type":  "photo",
	} {
		if media[field] != want {
			t.Errorf("%s %s = %v, want %q", label, field, media[field], want)
		}
	}
}

// Parts are read by name, not position: a title sent before the file is stored
// as the title, and the file bytes as the content (#1000).
func TestUploadPersonMedia_FieldsBeforeFile(t *testing.T) {
	server := setupTestServer()
	personRec := httptest.NewRecorder()
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", bytes.NewReader([]byte(`{"given_name":"Order","surname":"Test"}`)))
	personReq.Header.Set("Content-Type", "application/json")
	server.Echo().ServeHTTP(personRec, personReq)
	var personResp map[string]any
	_ = json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	jpegData := createTestJPEGImage()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	_ = writer.WriteField("title", "Wedding Day")
	_ = writer.WriteField("media_type", "photo")
	_ = writer.WriteField("unrelated", "ignored")
	part, _ := writer.CreateFormFile("file", "wedding.jpg")
	_, _ = part.Write(jpegData)
	_ = writer.WriteField("description", "Sent after the file")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/persons/%s/media", personID), &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Status = %d, want 201. Body: %s", rec.Code, rec.Body.String())
	}
	var media map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &media)
	if media["title"] != "Wedding Day" || media["description"] != "Sent after the file" || media["media_type"] != "photo" {
		t.Errorf("media = %v, want the title, description and media_type sent", media)
	}
	if media["filename"] != "wedding.jpg" {
		t.Errorf("filename = %v, want wedding.jpg", media["filename"])
	}

	contentRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(contentRec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/media/%s/content", media["id"]), http.NoBody))
	if !bytes.Equal(contentRec.Body.Bytes(), jpegData) {
		t.Errorf("content = %d bytes (status %d), want the %d uploaded bytes", contentRec.Body.Len(), contentRec.Code, len(jpegData))
	}
}

// An oversized text part is refused rather than buffered without bound.
func TestUploadPersonMedia_OversizedField(t *testing.T) {
	server := setupTestServer()
	personRec := httptest.NewRecorder()
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", bytes.NewReader([]byte(`{"given_name":"Big","surname":"Field"}`)))
	personReq.Header.Set("Content-Type", "application/json")
	server.Echo().ServeHTTP(personRec, personReq)
	var personResp map[string]any
	_ = json.Unmarshal(personRec.Body.Bytes(), &personResp)

	req, _ := createMultipartRequest(fmt.Sprintf("/api/v1/persons/%s/media", personResp["id"]), "file", "big.jpg", createTestJPEGImage(),
		map[string]string{"title": "Big", "description": string(bytes.Repeat([]byte("x"), 64<<10+1))})
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !bytes.Contains(rec.Body.Bytes(), []byte("description")) {
		t.Errorf("Status = %d, body %s; want 400 naming the description", rec.Code, rec.Body.String())
	}
}

func TestUploadPersonMedia_InvalidPersonID(t *testing.T) {
	server := setupTestServer()

	jpegData := createTestJPEGImage()
	fields := map[string]string{"title": "Test"}

	req, _ := createMultipartRequest(
		"/api/v1/persons/invalid-uuid/media",
		"file",
		"test.jpg",
		jpegData,
		fields,
	)

	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUploadPersonMedia_PersonNotFound(t *testing.T) {
	server := setupTestServer()

	jpegData := createTestJPEGImage()
	fields := map[string]string{"title": "Test"}

	req, _ := createMultipartRequest(
		"/api/v1/persons/00000000-0000-0000-0000-000000000001/media",
		"file",
		"test.jpg",
		jpegData,
		fields,
	)

	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestListPersonMedia(t *testing.T) {
	server := setupTestServer()

	// Create a person
	personBody := `{"given_name":"Bob","surname":"Smith","gender":"male"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", bytes.NewReader([]byte(personBody)))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	var personResp map[string]any
	_ = json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	// Upload two media items
	jpegData := createTestJPEGImage()
	for i := 0; i < 2; i++ {
		fields := map[string]string{"title": fmt.Sprintf("Photo %d", i+1)}
		req, _ := createMultipartRequest(
			fmt.Sprintf("/api/v1/persons/%s/media", personID),
			"file",
			fmt.Sprintf("photo%d.jpg", i+1),
			jpegData,
			fields,
		)
		rec := httptest.NewRecorder()
		server.Echo().ServeHTTP(rec, req)
	}

	// List media
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/persons/%s/media", personID), http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	total := int(resp["total"].(float64))
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}

	items := resp["items"].([]any)
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(items))
	}
}

func TestGetMedia(t *testing.T) {
	server := setupTestServer()

	// Create person and upload media
	personBody := `{"given_name":"Test","surname":"User","gender":"male"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", bytes.NewReader([]byte(personBody)))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	var personResp map[string]any
	_ = json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	jpegData := createTestJPEGImage()
	fields := map[string]string{"title": "Get Test Photo"}
	uploadReq, _ := createMultipartRequest(
		fmt.Sprintf("/api/v1/persons/%s/media", personID),
		"file",
		"test.jpg",
		jpegData,
		fields,
	)
	uploadRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(uploadRec, uploadReq)

	var uploadResp map[string]any
	_ = json.Unmarshal(uploadRec.Body.Bytes(), &uploadResp)
	mediaID := uploadResp["id"].(string)

	// Get media
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/media/%s", mediaID), http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp["title"] != "Get Test Photo" {
		t.Errorf("title = %v, want Get Test Photo", resp["title"])
	}
}

func TestGetMedia_NotFound(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/00000000-0000-0000-0000-000000000001", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetMedia_InvalidID(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/invalid-uuid", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDownloadMedia(t *testing.T) {
	server := setupTestServer()

	// Create person and upload media
	personBody := `{"given_name":"Download","surname":"Test","gender":"male"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", bytes.NewReader([]byte(personBody)))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	var personResp map[string]any
	_ = json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	jpegData := createTestJPEGImage()
	fields := map[string]string{"title": "Download Test"}
	uploadReq, _ := createMultipartRequest(
		fmt.Sprintf("/api/v1/persons/%s/media", personID),
		"file",
		"download.jpg",
		jpegData,
		fields,
	)
	uploadRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(uploadRec, uploadReq)

	var uploadResp map[string]any
	_ = json.Unmarshal(uploadRec.Body.Bytes(), &uploadResp)
	mediaID := uploadResp["id"].(string)

	// Download content
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/media/%s/content", mediaID), http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "image/jpeg" {
		t.Errorf("Content-Type = %s, want image/jpeg", contentType)
	}

	if rec.Body.Len() == 0 {
		t.Error("Expected non-empty body")
	}
}

func TestGetMediaThumbnail(t *testing.T) {
	server := setupTestServer()

	// Create person and upload media
	personBody := `{"given_name":"Thumb","surname":"Test","gender":"male"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", bytes.NewReader([]byte(personBody)))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	var personResp map[string]any
	_ = json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	jpegData := createTestJPEGImage()
	fields := map[string]string{"title": "Thumbnail Test"}
	uploadReq, _ := createMultipartRequest(
		fmt.Sprintf("/api/v1/persons/%s/media", personID),
		"file",
		"thumb.jpg",
		jpegData,
		fields,
	)
	uploadRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(uploadRec, uploadReq)

	var uploadResp map[string]any
	_ = json.Unmarshal(uploadRec.Body.Bytes(), &uploadResp)
	mediaID := uploadResp["id"].(string)

	// Get thumbnail
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/media/%s/thumbnail", mediaID), http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "image/jpeg" {
		t.Errorf("Content-Type = %s, want image/jpeg", contentType)
	}
}

func TestUpdateMedia(t *testing.T) {
	server := setupTestServer()

	// Create person and upload media
	personBody := `{"given_name":"Update","surname":"Test","gender":"male"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", bytes.NewReader([]byte(personBody)))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	var personResp map[string]any
	_ = json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	jpegData := createTestJPEGImage()
	fields := map[string]string{"title": "Original Title"}
	uploadReq, _ := createMultipartRequest(
		fmt.Sprintf("/api/v1/persons/%s/media", personID),
		"file",
		"update.jpg",
		jpegData,
		fields,
	)
	uploadRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(uploadRec, uploadReq)

	var uploadResp map[string]any
	_ = json.Unmarshal(uploadRec.Body.Bytes(), &uploadResp)
	mediaID := uploadResp["id"].(string)
	version := int64(uploadResp["version"].(float64))

	// Update media
	updateBody := fmt.Sprintf(`{"title":"Updated Title","description":"New description","version":%d}`, version)
	updateReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/media/%s", mediaID), bytes.NewReader([]byte(updateBody)))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(updateRec, updateReq)

	if updateRec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d. Body: %s", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}

	var updateResp map[string]any
	_ = json.Unmarshal(updateRec.Body.Bytes(), &updateResp)

	if updateResp["title"] != "Updated Title" {
		t.Errorf("title = %v, want Updated Title", updateResp["title"])
	}
}

func TestDeleteMedia(t *testing.T) {
	server := setupTestServer()

	// Create person and upload media
	personBody := `{"given_name":"Delete","surname":"Test","gender":"male"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", bytes.NewReader([]byte(personBody)))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	var personResp map[string]any
	_ = json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	jpegData := createTestJPEGImage()
	fields := map[string]string{"title": "To Delete"}
	uploadReq, _ := createMultipartRequest(
		fmt.Sprintf("/api/v1/persons/%s/media", personID),
		"file",
		"delete.jpg",
		jpegData,
		fields,
	)
	uploadRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(uploadRec, uploadReq)

	var uploadResp map[string]any
	_ = json.Unmarshal(uploadRec.Body.Bytes(), &uploadResp)
	mediaID := uploadResp["id"].(string)
	version := int64(uploadResp["version"].(float64))

	// Delete media
	deleteReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/media/%s?version=%d", mediaID, version), http.NoBody)
	deleteRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusNoContent {
		t.Errorf("Status = %d, want %d. Body: %s", deleteRec.Code, http.StatusNoContent, deleteRec.Body.String())
	}

	// Verify deleted
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/media/%s", mediaID), http.NoBody)
	getRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Errorf("Status after delete = %d, want %d", getRec.Code, http.StatusNotFound)
	}
}

func TestDeleteMedia_InvalidID(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/media/invalid-uuid?version=1", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteMedia_MissingVersion(t *testing.T) {
	server := setupTestServer()

	// version is required for optimistic locking; omitting it is a 400.
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/media/00000000-0000-0000-0000-000000000001", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateMedia_InvalidID(t *testing.T) {
	server := setupTestServer()

	updateBody := `{"title":"Test","version":1}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/media/invalid-uuid", bytes.NewReader([]byte(updateBody)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateMedia_InvalidJSON(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodPut, "/api/v1/media/00000000-0000-0000-0000-000000000001", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDownloadMedia_InvalidID(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/invalid-uuid/content", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDownloadMedia_NotFound(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/00000000-0000-0000-0000-000000000001/content", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetMediaThumbnail_InvalidID(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/invalid-uuid/thumbnail", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetMediaThumbnail_NotFound(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/00000000-0000-0000-0000-000000000001/thumbnail", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestListPersonMedia_InvalidID(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/invalid-uuid/media", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListPersonMedia_PersonNotFound(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/00000000-0000-0000-0000-000000000001/media", http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestListPersonMedia_LimitCapping(t *testing.T) {
	server := setupTestServer()

	// Create a person
	personBody := `{"given_name":"Limit","surname":"Test","gender":"male"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", bytes.NewReader([]byte(personBody)))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	var personResp map[string]any
	_ = json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	// Request with limit > 100 (should be capped)
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/persons/%s/media?limit=500", personID), http.NoBody)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestUploadPersonMedia_MissingFile(t *testing.T) {
	server := setupTestServer()

	// Create a person
	personBody := `{"given_name":"NoFile","surname":"Test","gender":"female"}`
	personReq := httptest.NewRequest(http.MethodPost, "/api/v1/persons", bytes.NewReader([]byte(personBody)))
	personReq.Header.Set("Content-Type", "application/json")
	personRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(personRec, personReq)

	var personResp map[string]any
	_ = json.Unmarshal(personRec.Body.Bytes(), &personResp)
	personID := personResp["id"].(string)

	// Create request without file
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	_ = writer.WriteField("title", "Test Title")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/persons/%s/media", personID), &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// referenceMediaGedcom links an OBJE record, which carries file references
// but no file content, from a person (#1477).
const referenceMediaGedcom = `0 HEAD
1 GEDC
2 VERS 7.0
0 @M1@ OBJE
1 FILE photos/wedding.jpg
2 FORM image/jpeg
2 TITL Wedding
2 TRAN photos/wedding-small.jpg
3 FORM image/jpeg
0 @I1@ INDI
1 NAME John /Reference/
1 OBJE @M1@
0 TRLR
`

// TestReferenceOnlyMedia_ContentAndThumbnailNotFound checks that a media item
// imported from GEDCOM, which has no stored content, answers its content and
// thumbnail with documented 404s rather than an empty 200 (#1477).
func TestReferenceOnlyMedia_ContentAndThumbnailNotFound(t *testing.T) {
	server := setupTestServer()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "reference.ged")
	_, _ = io.WriteString(part, referenceMediaGedcom)
	_ = writer.Close()
	importReq := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import", body)
	importReq.Header.Set("Content-Type", writer.FormDataContentType())
	importRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(importRec, importReq)
	if importRec.Code != http.StatusOK {
		t.Fatalf("import status = %d: %s", importRec.Code, importRec.Body.String())
	}

	search := decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/search?q=Reference", ""))
	people := search["items"].([]any)
	if len(people) != 1 {
		t.Fatalf("search found %d people, want 1", len(people))
	}
	personID := people[0].(map[string]any)["id"].(string)

	list := decodeJSON(t, do(t, server, http.MethodGet, "/api/v1/persons/"+personID+"/media", ""))
	items := list["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("person has %d media items, want 1", len(items))
	}
	item := items[0].(map[string]any)
	if item["has_content"] != false || item["file_size"] != float64(0) || item["has_thumbnail"] != false {
		t.Errorf("has_content = %v, file_size = %v, has_thumbnail = %v; want false, 0 and false",
			item["has_content"], item["file_size"], item["has_thumbnail"])
	}
	files, _ := item["files"].([]any)
	if len(files) != 1 || files[0].(map[string]any)["path"] != "photos/wedding.jpg" {
		t.Fatalf("files = %v, want the one reference photos/wedding.jpg", item["files"])
	}
	if tran, _ := files[0].(map[string]any)["translations"].([]any); len(tran) != 1 ||
		tran[0].(map[string]any)["path"] != "photos/wedding-small.jpg" {
		t.Errorf("file translations = %v, want photos/wedding-small.jpg", files[0].(map[string]any)["translations"])
	}
	if item["format"] != "image/jpeg" {
		t.Errorf("format = %v, want image/jpeg", item["format"])
	}
	mediaID := item["id"].(string)

	tests := []struct {
		path     string
		wantCode string
	}{
		{"/content", "no_content"},
		{"/thumbnail", "not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := do(t, server, http.MethodGet, "/api/v1/media/"+mediaID+tt.path, "")
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
			}
			if got := decodeJSON(t, rec)["code"]; got != tt.wantCode {
				t.Errorf("code = %v, want %q", got, tt.wantCode)
			}
		})
	}
}

// ownerMediaGedcom gives a family and a source one media reference each, and
// a private family and a private source one more each (#1505). Everyone but
// Dee is dead; Dee is living, so her family has a protected partner.
const ownerMediaGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
0 @M1@ OBJE
1 FILE photos/wedding.jpg
2 FORM jpg
2 TITL Wedding
0 @M2@ OBJE
1 FILE scans/census-page.jpg
2 FORM jpg
2 TITL Census page
0 @M3@ OBJE
1 FILE photos/hidden-family.jpg
2 FORM jpg
2 TITL Hidden family
0 @M4@ OBJE
1 FILE scans/hidden-register.jpg
2 FORM jpg
2 TITL Hidden register
0 @M5@ OBJE
1 FILE photos/living-family.jpg
2 FORM jpg
2 TITL Living family
0 @I1@ INDI
1 NAME Ada /Owner/
1 DEAT Y
0 @I2@ INDI
1 NAME Bob /Owner/
1 DEAT Y
0 @I3@ INDI
1 NAME Cy /Owner/
1 DEAT Y
0 @I4@ INDI
1 NAME Dee /Owner/
1 BIRT
2 DATE 1990
0 @F1@ FAM
1 HUSB @I2@
1 WIFE @I1@
1 OBJE @M1@
0 @F2@ FAM
1 RESN privacy
1 HUSB @I3@
1 OBJE @M3@
0 @F3@ FAM
1 WIFE @I4@
1 OBJE @M5@
0 @S1@ SOUR
1 TITL 1851 Census
1 OBJE @M2@
0 @S2@ SOUR
1 TITL Hidden register
1 _RESN privacy
1 OBJE @M4@
0 TRLR
`

// importOwnerMedia imports ownerMediaGedcom as the owner and returns the
// media list path of each family and source, keyed by its one item's title.
func importOwnerMedia(t *testing.T, f *authFixture, owner *http.Cookie) map[string]string {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, _ := w.CreateFormFile("file", "owners.ged")
	_, _ = part.Write([]byte(ownerMediaGedcom))
	_ = w.Close()
	if rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/gedcom/import", body: body.String(), cookie: owner,
		header: map[string]string{"Content-Type": w.FormDataContentType()}}); rec.Code != http.StatusOK {
		t.Fatalf("import: %d %s", rec.Code, rec.Body)
	}

	paths := map[string]string{}
	// The family list's rows are under "items", the source list's under "sources".
	for list, key := range map[string]string{"families": "items", "sources": "sources"} {
		rows := decode[map[string]any](t, f.do(t, request{method: http.MethodGet, path: "/api/v1/" + list, cookie: owner}))
		for _, row := range rows[key].([]any) {
			path := "/api/v1/" + list + "/" + row.(map[string]any)["id"].(string) + "/media"
			rec := f.do(t, request{method: http.MethodGet, path: path, cookie: owner})
			if rec.Code != http.StatusOK {
				t.Fatalf("owner GET %s: %d %s", path, rec.Code, rec.Body)
			}
			items := decode[map[string]any](t, rec)["items"].([]any)
			if len(items) != 1 {
				t.Fatalf("owner GET %s = %v, want one item", path, items)
			}
			paths[items[0].(map[string]any)["title"].(string)] = path
		}
	}
	if len(paths) != 5 {
		t.Fatalf("media lists = %v, want 5", paths)
	}
	return paths
}

// TestListFamilyAndSourceMedia lists the media a family and a source own: each
// returns its own GEDCOM reference with its path, and an unknown owner is not
// found (#1505).
func TestListFamilyAndSourceMedia(t *testing.T) {
	f, cookies := rolesFixture(t)
	paths := importOwnerMedia(t, f, cookies[repository.RoleOwner])

	for title, wantPath := range map[string]string{"Wedding": "photos/wedding.jpg", "Census page": "scans/census-page.jpg"} {
		rec := f.do(t, request{method: http.MethodGet, path: paths[title], cookie: cookies[repository.RoleViewer]})
		if rec.Code != http.StatusOK {
			t.Fatalf("viewer GET %s: %d %s", paths[title], rec.Code, rec.Body)
		}
		resp := decode[map[string]any](t, rec)
		items := resp["items"].([]any)
		if len(items) != 1 || resp["total"] != float64(1) {
			t.Fatalf("%s = %v, want its one item", title, resp)
		}
		item := items[0].(map[string]any)
		files, _ := item["files"].([]any)
		if item["title"] != title || item["has_content"] != false || len(files) != 1 || files[0].(map[string]any)["path"] != wantPath {
			t.Errorf("%s item = %v, want the reference to %s", title, item, wantPath)
		}
	}

	for _, path := range []string{
		"/api/v1/families/00000000-0000-0000-0000-000000000001/media",
		"/api/v1/sources/00000000-0000-0000-0000-000000000001/media",
	} {
		if rec := f.do(t, request{method: http.MethodGet, path: path, cookie: cookies[repository.RoleOwner]}); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, rec.Code)
		}
	}
	for _, path := range []string{"/api/v1/families/not-a-uuid/media", "/api/v1/sources/not-a-uuid/media"} {
		if rec := f.do(t, request{method: http.MethodGet, path: path, cookie: cookies[repository.RoleOwner]}); rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want 400", path, rec.Code)
		}
	}
}

// TestListFamilyAndSourceMedia_ViewerPrivacy checks a viewer gets 404 for a
// private family or source, as for a hidden person, an empty list for a
// family with a protected partner, and no private item from a visible one,
// while the owner still sees it marked private (#1505).
func TestListFamilyAndSourceMedia_ViewerPrivacy(t *testing.T) {
	f, cookies := rolesFixture(t)
	owner, viewer := cookies[repository.RoleOwner], cookies[repository.RoleViewer]
	paths := importOwnerMedia(t, f, owner)

	for _, title := range []string{"Hidden family", "Hidden register"} {
		if rec := f.do(t, request{method: http.MethodGet, path: paths[title], cookie: viewer}); rec.Code != http.StatusNotFound {
			t.Errorf("viewer GET %s (%s) = %d %s, want 404", paths[title], title, rec.Code, rec.Body)
		}
	}

	// With viewers kept from the living, Dee's family resolves with a
	// placeholder partner, so it is found, but lists none of its media
	// (ADR-008 §3).
	settings := repository.DefaultTreeSettings()
	settings.ViewersSeeLiving = false
	if err := f.accounts.SaveTreeSettings(context.Background(), repository.DefaultTreeID, settings); err != nil {
		t.Fatalf("SaveTreeSettings: %v", err)
	}
	rec := f.do(t, request{method: http.MethodGet, path: paths["Living family"], cookie: viewer})
	if rec.Code != http.StatusOK {
		t.Fatalf("viewer GET %s: %d %s", paths["Living family"], rec.Code, rec.Body)
	}
	if resp := decode[map[string]any](t, rec); len(resp["items"].([]any)) != 0 || resp["total"] != float64(0) {
		t.Errorf("viewer Living family = %v, want nothing listed for a protected partner", resp)
	}
	if got := decode[map[string]any](t, f.do(t, request{method: http.MethodGet, path: paths["Living family"], cookie: owner}))["total"]; got != float64(1) {
		t.Errorf("owner Living family total = %v, want 1", got)
	}

	for _, title := range []string{"Wedding", "Census page"} {
		item := decode[map[string]any](t, f.do(t, request{method: http.MethodGet, path: paths[title], cookie: owner}))["items"].([]any)[0].(map[string]any)
		mark := fmt.Sprintf(`{"private":true,"version":%v}`, item["version"])
		if rec := f.do(t, request{method: http.MethodPut, path: "/api/v1/media/" + item["id"].(string), body: mark, cookie: owner}); rec.Code != http.StatusOK {
			t.Fatalf("mark %s private: %d %s", title, rec.Code, rec.Body)
		}

		rec := f.do(t, request{method: http.MethodGet, path: paths[title], cookie: viewer})
		if rec.Code != http.StatusOK {
			t.Fatalf("viewer GET %s: %d %s", paths[title], rec.Code, rec.Body)
		}
		if resp := decode[map[string]any](t, rec); len(resp["items"].([]any)) != 0 || resp["total"] != float64(0) {
			t.Errorf("viewer %s = %v, want no private item", title, resp)
		}
		got := decode[map[string]any](t, f.do(t, request{method: http.MethodGet, path: paths[title], cookie: owner}))["items"].([]any)
		if len(got) != 1 || got[0].(map[string]any)["private"] != true {
			t.Errorf("owner %s = %v, want the item marked private", title, got)
		}
	}
}
