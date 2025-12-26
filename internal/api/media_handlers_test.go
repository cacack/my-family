package api_test

import (
	"bytes"
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

	if mediaResp["title"] != "Family Portrait" {
		t.Errorf("title = %v, want Family Portrait", mediaResp["title"])
	}
	if mediaResp["id"] == nil || mediaResp["id"] == "" {
		t.Error("Expected non-empty id")
	}
	// Note: has_thumbnail requires checking via GetMediaWithData since GetMedia
	// excludes binary data for efficiency. The thumbnail test verifies this separately.
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

	// Upload media without title
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
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
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
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/persons/%s/media", personID), nil)
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
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/media/%s", mediaID), nil)
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/00000000-0000-0000-0000-000000000001", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetMedia_InvalidID(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/invalid-uuid", nil)
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
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/media/%s/content", mediaID), nil)
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
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/media/%s/thumbnail", mediaID), nil)
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
	deleteReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/media/%s?version=%d", mediaID, version), nil)
	deleteRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusNoContent {
		t.Errorf("Status = %d, want %d. Body: %s", deleteRec.Code, http.StatusNoContent, deleteRec.Body.String())
	}

	// Verify deleted
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/media/%s", mediaID), nil)
	getRec := httptest.NewRecorder()
	server.Echo().ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Errorf("Status after delete = %d, want %d", getRec.Code, http.StatusNotFound)
	}
}

func TestDeleteMedia_InvalidID(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/media/invalid-uuid?version=1", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteMedia_MissingVersion(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/media/00000000-0000-0000-0000-000000000001", nil)
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/invalid-uuid/content", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDownloadMedia_NotFound(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/00000000-0000-0000-0000-000000000001/content", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetMediaThumbnail_InvalidID(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/invalid-uuid/thumbnail", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetMediaThumbnail_NotFound(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/00000000-0000-0000-0000-000000000001/thumbnail", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestListPersonMedia_InvalidID(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/invalid-uuid/media", nil)
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListPersonMedia_PersonNotFound(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/00000000-0000-0000-0000-000000000001/media", nil)
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
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/persons/%s/media?limit=500", personID), nil)
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
