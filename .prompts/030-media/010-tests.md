# Stage 10: Test Coverage

<objective>
Create comprehensive test coverage for all media management components, ensuring 85% per-package coverage as required by the project.
</objective>

<context>
Reference existing test patterns in:
- @file:internal/domain/*_test.go - Domain validation tests
- @file:internal/command/*_test.go - Command handler tests
- @file:internal/repository/postgres/readmodel_test.go - Database integration tests

Dependencies:
- All previous stages complete (1-9)

Coverage requirement: 85% per package (enforced by CI)
</context>

<requirements>

## 1. Domain Tests (internal/domain/media_test.go)

```go
package domain

import (
    "testing"

    "github.com/google/uuid"
)

func TestMediaType_IsValid(t *testing.T) {
    tests := []struct {
        mt   MediaType
        want bool
    }{
        {MediaPhoto, true},
        {MediaDocument, true},
        {MediaAudio, true},
        {MediaVideo, true},
        {MediaCertificate, true},
        {"", true},           // Empty is valid (optional field)
        {"invalid", false},
    }

    for _, tt := range tests {
        if got := tt.mt.IsValid(); got != tt.want {
            t.Errorf("MediaType(%q).IsValid() = %v, want %v", tt.mt, got, tt.want)
        }
    }
}

func TestNewMedia(t *testing.T) {
    entityID := uuid.New()
    m := NewMedia("Test Photo", "person", entityID)

    if m.ID == uuid.Nil {
        t.Error("expected non-nil ID")
    }
    if m.Title != "Test Photo" {
        t.Errorf("expected title 'Test Photo', got %q", m.Title)
    }
    if m.EntityType != "person" {
        t.Errorf("expected entity type 'person', got %q", m.EntityType)
    }
    if m.EntityID != entityID {
        t.Errorf("expected entity ID %s, got %s", entityID, m.EntityID)
    }
}

func TestMedia_Validate(t *testing.T) {
    entityID := uuid.New()

    tests := []struct {
        name    string
        modify  func(*Media)
        wantErr bool
    }{
        {
            name:    "valid media",
            modify:  func(m *Media) { m.FileData = []byte("data"); m.FileSize = 4; m.MimeType = "image/jpeg" },
            wantErr: false,
        },
        {
            name:    "missing title",
            modify:  func(m *Media) { m.Title = "" },
            wantErr: true,
        },
        {
            name:    "title too long",
            modify:  func(m *Media) { m.Title = string(make([]byte, 501)) },
            wantErr: true,
        },
        {
            name:    "invalid entity type",
            modify:  func(m *Media) { m.EntityType = "invalid" },
            wantErr: true,
        },
        {
            name:    "missing entity ID",
            modify:  func(m *Media) { m.EntityID = uuid.Nil },
            wantErr: true,
        },
        {
            name:    "file data without size",
            modify:  func(m *Media) { m.FileData = []byte("data"); m.FileSize = 0; m.MimeType = "image/jpeg" },
            wantErr: true,
        },
        {
            name:    "file data without mime type",
            modify:  func(m *Media) { m.FileData = []byte("data"); m.FileSize = 4; m.MimeType = "" },
            wantErr: true,
        },
        {
            name:    "file size exceeds max",
            modify:  func(m *Media) { m.FileSize = 11 * 1024 * 1024 },
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            m := NewMedia("Test", "person", entityID)
            tt.modify(m)
            err := m.Validate()
            if (err != nil) != tt.wantErr {
                t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}

func TestMediaCreated(t *testing.T) {
    m := NewMedia("Test", "person", uuid.New())
    m.FileData = []byte("test")
    m.FileSize = 4
    m.MimeType = "image/jpeg"

    event := NewMediaCreated(m)

    if event.EventType() != "MediaCreated" {
        t.Errorf("expected event type 'MediaCreated', got %q", event.EventType())
    }
    if event.AggregateID() != m.ID {
        t.Error("aggregate ID should match media ID")
    }
    if event.Title != m.Title {
        t.Error("event title should match media title")
    }
}

func TestMediaUpdated(t *testing.T) {
    mediaID := uuid.New()
    changes := map[string]any{"title": "New Title"}

    event := NewMediaUpdated(mediaID, changes)

    if event.EventType() != "MediaUpdated" {
        t.Errorf("expected event type 'MediaUpdated', got %q", event.EventType())
    }
    if event.AggregateID() != mediaID {
        t.Error("aggregate ID should match media ID")
    }
    if event.Changes["title"] != "New Title" {
        t.Error("changes should contain title update")
    }
}

func TestMediaDeleted(t *testing.T) {
    mediaID := uuid.New()
    event := NewMediaDeleted(mediaID, "test reason")

    if event.EventType() != "MediaDeleted" {
        t.Errorf("expected event type 'MediaDeleted', got %q", event.EventType())
    }
    if event.Reason != "test reason" {
        t.Error("reason should be set")
    }
}
```

## 2. Command Handler Tests (internal/command/media_commands_test.go)

```go
package command

import (
    "context"
    "testing"

    "github.com/google/uuid"
)

func TestUploadMedia(t *testing.T) {
    h := setupTestHandler(t) // Use existing test setup pattern

    // Create a person to attach media to
    personResult, err := h.CreatePerson(context.Background(), CreatePersonInput{
        GivenName: "Test",
        Surname:   "Person",
    })
    if err != nil {
        t.Fatalf("failed to create person: %v", err)
    }

    t.Run("valid upload", func(t *testing.T) {
        input := UploadMediaInput{
            EntityType:  "person",
            EntityID:    personResult.ID,
            Title:       "Test Photo",
            Description: "A test photo",
            Filename:    "test.jpg",
            MimeType:    "image/jpeg",
            FileData:    createTestJPEG(),
        }

        result, err := h.UploadMedia(context.Background(), input)
        if err != nil {
            t.Fatalf("unexpected error: %v", err)
        }
        if result.ID == uuid.Nil {
            t.Error("expected non-nil media ID")
        }
    })

    t.Run("missing title uses filename", func(t *testing.T) {
        input := UploadMediaInput{
            EntityType: "person",
            EntityID:   personResult.ID,
            Title:      "",
            Filename:   "photo.jpg",
            MimeType:   "image/jpeg",
            FileData:   createTestJPEG(),
        }

        result, err := h.UploadMedia(context.Background(), input)
        // Title validation should fail
        if err == nil {
            t.Error("expected error for missing title")
        }
    })

    t.Run("file too large", func(t *testing.T) {
        largeData := make([]byte, MaxFileSize+1)
        input := UploadMediaInput{
            EntityType: "person",
            EntityID:   personResult.ID,
            Title:      "Large File",
            Filename:   "large.jpg",
            MimeType:   "image/jpeg",
            FileData:   largeData,
        }

        _, err := h.UploadMedia(context.Background(), input)
        if err != ErrFileTooLarge {
            t.Errorf("expected ErrFileTooLarge, got %v", err)
        }
    })

    t.Run("unsupported format", func(t *testing.T) {
        input := UploadMediaInput{
            EntityType: "person",
            EntityID:   personResult.ID,
            Title:      "Executable",
            Filename:   "test.exe",
            MimeType:   "application/x-msdownload",
            FileData:   []byte("MZ..."),
        }

        _, err := h.UploadMedia(context.Background(), input)
        if err == nil || !errors.Is(err, ErrUnsupportedFormat) {
            t.Errorf("expected ErrUnsupportedFormat, got %v", err)
        }
    })

    t.Run("entity not found", func(t *testing.T) {
        input := UploadMediaInput{
            EntityType: "person",
            EntityID:   uuid.New(), // Non-existent person
            Title:      "Test",
            Filename:   "test.jpg",
            MimeType:   "image/jpeg",
            FileData:   createTestJPEG(),
        }

        _, err := h.UploadMedia(context.Background(), input)
        if err == nil || !errors.Is(err, ErrEntityNotFound) {
            t.Errorf("expected ErrEntityNotFound, got %v", err)
        }
    })
}

func TestUpdateMedia(t *testing.T) {
    h := setupTestHandler(t)
    ctx := context.Background()

    // Setup: create person and upload media
    personResult, _ := h.CreatePerson(ctx, CreatePersonInput{GivenName: "Test", Surname: "Person"})
    uploadResult, _ := h.UploadMedia(ctx, UploadMediaInput{
        EntityType: "person",
        EntityID:   personResult.ID,
        Title:      "Original Title",
        Filename:   "test.jpg",
        MimeType:   "image/jpeg",
        FileData:   createTestJPEG(),
    })

    t.Run("update title", func(t *testing.T) {
        newTitle := "Updated Title"
        input := UpdateMediaInput{
            ID:      uploadResult.ID,
            Title:   &newTitle,
            Version: uploadResult.Version,
        }

        result, err := h.UpdateMedia(ctx, input)
        if err != nil {
            t.Fatalf("unexpected error: %v", err)
        }
        if result.Version <= uploadResult.Version {
            t.Error("version should increment")
        }
    })

    t.Run("version conflict", func(t *testing.T) {
        newTitle := "Another Title"
        input := UpdateMediaInput{
            ID:      uploadResult.ID,
            Title:   &newTitle,
            Version: 0, // Stale version
        }

        _, err := h.UpdateMedia(ctx, input)
        if err != repository.ErrConcurrencyConflict {
            t.Errorf("expected ErrConcurrencyConflict, got %v", err)
        }
    })
}

func TestDeleteMedia(t *testing.T) {
    h := setupTestHandler(t)
    ctx := context.Background()

    // Setup
    personResult, _ := h.CreatePerson(ctx, CreatePersonInput{GivenName: "Test", Surname: "Person"})
    uploadResult, _ := h.UploadMedia(ctx, UploadMediaInput{
        EntityType: "person",
        EntityID:   personResult.ID,
        Title:      "To Delete",
        Filename:   "test.jpg",
        MimeType:   "image/jpeg",
        FileData:   createTestJPEG(),
    })

    t.Run("successful delete", func(t *testing.T) {
        err := h.DeleteMedia(ctx, uploadResult.ID, uploadResult.Version, "test deletion")
        if err != nil {
            t.Fatalf("unexpected error: %v", err)
        }

        // Verify deleted
        media, _ := h.readStore.GetMedia(ctx, uploadResult.ID)
        if media != nil {
            t.Error("media should be deleted")
        }
    })

    t.Run("not found", func(t *testing.T) {
        err := h.DeleteMedia(ctx, uuid.New(), 1, "test")
        if err != ErrMediaNotFound {
            t.Errorf("expected ErrMediaNotFound, got %v", err)
        }
    })
}

// Test helper
func createTestJPEG() []byte {
    // Create minimal valid JPEG bytes
    // Use thumbnail package helper or embed a tiny test image
    return []byte{
        0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46,
        0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01,
        0x00, 0x01, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
        // ... minimal JPEG data ...
        0xFF, 0xD9,
    }
}
```

## 3. Repository Tests (add to existing test files)

### PostgreSQL (internal/repository/postgres/readmodel_test.go)

```go
func TestMediaOperations(t *testing.T) {
    store := setupTestPostgresStore(t)
    ctx := context.Background()

    t.Run("SaveAndGetMedia", func(t *testing.T) {
        media := &repository.MediaReadModel{
            ID:          uuid.New(),
            EntityType:  "person",
            EntityID:    uuid.New(),
            Title:       "Test Photo",
            MimeType:    "image/jpeg",
            MediaType:   domain.MediaPhoto,
            Filename:    "test.jpg",
            FileSize:    1024,
            FileData:    []byte("fake jpeg data"),
            ThumbnailData: []byte("fake thumbnail"),
            Version:     1,
            CreatedAt:   time.Now(),
            UpdatedAt:   time.Now(),
        }

        err := store.SaveMedia(ctx, media)
        if err != nil {
            t.Fatalf("SaveMedia failed: %v", err)
        }

        // Get metadata only
        got, err := store.GetMedia(ctx, media.ID)
        if err != nil {
            t.Fatalf("GetMedia failed: %v", err)
        }
        if got.Title != media.Title {
            t.Error("title mismatch")
        }
        if got.FileData != nil {
            t.Error("GetMedia should not return FileData")
        }

        // Get with data
        gotFull, err := store.GetMediaWithData(ctx, media.ID)
        if err != nil {
            t.Fatalf("GetMediaWithData failed: %v", err)
        }
        if !bytes.Equal(gotFull.FileData, media.FileData) {
            t.Error("FileData mismatch")
        }
    })

    t.Run("ListMediaForEntity", func(t *testing.T) {
        entityID := uuid.New()

        // Create 3 media items
        for i := 0; i < 3; i++ {
            media := &repository.MediaReadModel{
                ID:         uuid.New(),
                EntityType: "person",
                EntityID:   entityID,
                Title:      fmt.Sprintf("Photo %d", i),
                MimeType:   "image/jpeg",
                MediaType:  domain.MediaPhoto,
                Filename:   fmt.Sprintf("photo%d.jpg", i),
                FileSize:   1024,
                FileData:   []byte("data"),
                Version:    1,
                CreatedAt:  time.Now().Add(time.Duration(i) * time.Hour),
                UpdatedAt:  time.Now(),
            }
            store.SaveMedia(ctx, media)
        }

        items, total, err := store.ListMediaForEntity(ctx, "person", entityID, repository.ListOptions{Limit: 2})
        if err != nil {
            t.Fatalf("ListMediaForEntity failed: %v", err)
        }
        if total != 3 {
            t.Errorf("expected total 3, got %d", total)
        }
        if len(items) != 2 {
            t.Errorf("expected 2 items, got %d", len(items))
        }
    })

    t.Run("GetMediaThumbnail", func(t *testing.T) {
        media := &repository.MediaReadModel{
            ID:            uuid.New(),
            EntityType:    "person",
            EntityID:      uuid.New(),
            Title:         "With Thumbnail",
            MimeType:      "image/jpeg",
            MediaType:     domain.MediaPhoto,
            Filename:      "thumb.jpg",
            FileSize:      1024,
            FileData:      []byte("data"),
            ThumbnailData: []byte("thumbnail bytes"),
            Version:       1,
            CreatedAt:     time.Now(),
            UpdatedAt:     time.Now(),
        }
        store.SaveMedia(ctx, media)

        thumb, err := store.GetMediaThumbnail(ctx, media.ID)
        if err != nil {
            t.Fatalf("GetMediaThumbnail failed: %v", err)
        }
        if !bytes.Equal(thumb, media.ThumbnailData) {
            t.Error("thumbnail mismatch")
        }
    })

    t.Run("DeleteMedia", func(t *testing.T) {
        media := &repository.MediaReadModel{
            ID:         uuid.New(),
            EntityType: "person",
            EntityID:   uuid.New(),
            Title:      "To Delete",
            MimeType:   "image/jpeg",
            MediaType:  domain.MediaPhoto,
            Filename:   "delete.jpg",
            FileSize:   1024,
            FileData:   []byte("data"),
            Version:    1,
            CreatedAt:  time.Now(),
            UpdatedAt:  time.Now(),
        }
        store.SaveMedia(ctx, media)

        err := store.DeleteMedia(ctx, media.ID)
        if err != nil {
            t.Fatalf("DeleteMedia failed: %v", err)
        }

        got, _ := store.GetMedia(ctx, media.ID)
        if got != nil {
            t.Error("media should be deleted")
        }
    })
}
```

## 4. API Handler Tests (internal/api/handlers_test.go)

```go
func TestMediaHandlers(t *testing.T) {
    h := setupTestAPIHandler(t)
    e := echo.New()

    t.Run("UploadPersonMedia", func(t *testing.T) {
        // Create test person first
        personID := createTestPerson(t, h)

        // Create multipart request
        body := &bytes.Buffer{}
        writer := multipart.NewWriter(body)

        // Add file
        part, _ := writer.CreateFormFile("file", "test.jpg")
        part.Write(createTestJPEG())

        // Add title
        writer.WriteField("title", "Test Upload")
        writer.Close()

        req := httptest.NewRequest(http.MethodPost, "/persons/"+personID.String()+"/media", body)
        req.Header.Set("Content-Type", writer.FormDataContentType())
        rec := httptest.NewRecorder()
        c := e.NewContext(req, rec)
        c.SetParamNames("id")
        c.SetParamValues(personID.String())

        err := h.UploadPersonMedia(c)
        if err != nil {
            t.Fatalf("handler error: %v", err)
        }
        if rec.Code != http.StatusCreated {
            t.Errorf("expected status 201, got %d", rec.Code)
        }
    })

    t.Run("DownloadMedia", func(t *testing.T) {
        // Setup: create person and upload media
        personID := createTestPerson(t, h)
        mediaID := uploadTestMedia(t, h, personID)

        req := httptest.NewRequest(http.MethodGet, "/media/"+mediaID.String()+"/download", nil)
        rec := httptest.NewRecorder()
        c := e.NewContext(req, rec)
        c.SetParamNames("id")
        c.SetParamValues(mediaID.String())

        err := h.DownloadMedia(c)
        if err != nil {
            t.Fatalf("handler error: %v", err)
        }
        if rec.Code != http.StatusOK {
            t.Errorf("expected status 200, got %d", rec.Code)
        }
        if rec.Header().Get("Content-Disposition") == "" {
            t.Error("expected Content-Disposition header")
        }
    })

    t.Run("GetMediaThumbnail", func(t *testing.T) {
        personID := createTestPerson(t, h)
        mediaID := uploadTestMedia(t, h, personID)

        req := httptest.NewRequest(http.MethodGet, "/media/"+mediaID.String()+"/thumbnail", nil)
        rec := httptest.NewRecorder()
        c := e.NewContext(req, rec)
        c.SetParamNames("id")
        c.SetParamValues(mediaID.String())

        err := h.GetMediaThumbnail(c)
        if err != nil {
            t.Fatalf("handler error: %v", err)
        }
        if rec.Code != http.StatusOK {
            t.Errorf("expected status 200, got %d", rec.Code)
        }
        if rec.Header().Get("Content-Type") != "image/jpeg" {
            t.Error("expected image/jpeg content type")
        }
    })
}
```

</requirements>

<implementation>

1. Create `internal/domain/media_test.go`
2. Create `internal/command/media_commands_test.go`
3. Add media tests to `internal/repository/postgres/readmodel_test.go`
4. Add media tests to `internal/repository/sqlite/readmodel_test.go`
5. Create or update `internal/api/handlers_test.go`
6. Run `make check-coverage` to verify 85% threshold

</implementation>

<verification>
```bash
# Run all tests
go test ./... -v

# Check coverage per package
go test ./internal/domain/... -cover
go test ./internal/command/... -cover
go test ./internal/repository/postgres/... -cover
go test ./internal/repository/sqlite/... -cover
go test ./internal/api/... -cover
go test ./internal/media/... -cover
go test ./internal/gedcom/... -cover

# Run coverage check (must pass 85%)
make check-coverage

# Generate coverage report
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```
</verification>

<output>
After completing this stage, provide:
1. Test files created
2. Coverage percentages per package
3. Any gaps identified and addressed

Example output:
```
Stage 10 Complete: Test Coverage

Test files created/updated:
- internal/domain/media_test.go (new)
- internal/command/media_commands_test.go (new)
- internal/repository/postgres/readmodel_test.go (extended)
- internal/repository/sqlite/readmodel_test.go (extended)
- internal/api/handlers_test.go (extended)

Coverage results:
- internal/domain: 92%
- internal/command: 87%
- internal/repository/postgres: 86%
- internal/repository/sqlite: 85%
- internal/api: 88%
- internal/media: 95%
- internal/gedcom: 83% (slight gap in error paths)

make check-coverage: PASSED (all packages >= 85%)

Pipeline Complete!
```
</output>
