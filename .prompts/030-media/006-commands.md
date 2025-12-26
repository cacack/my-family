# Stage 6: Command Handlers

<objective>
Create command handlers for uploading, updating, and deleting media files, including input validation and thumbnail generation integration.
</objective>

<context>
Reference existing patterns in:
- @file:internal/command/source_commands.go - CreateSource, UpdateSource, DeleteSource patterns
- @file:internal/command/handler.go - Handler struct, execute() method

Dependencies:
- Stages 1-5 complete (domain, repository, projections)
- Stage 7 complete (thumbnail generation) - or stub if running in parallel
</context>

<requirements>

## 1. Create internal/command/media_commands.go

### Errors

```go
package command

import "errors"

// Media command errors
var (
    ErrMediaNotFound     = errors.New("media not found")
    ErrFileTooLarge      = errors.New("file exceeds maximum size of 10MB")
    ErrUnsupportedFormat = errors.New("unsupported file format")
    ErrEntityNotFound    = errors.New("target entity not found")
)

// MaxFileSize is the maximum allowed file size (10MB)
const MaxFileSize = 10 * 1024 * 1024

// AllowedMimeTypes lists supported file types
var AllowedMimeTypes = map[string]bool{
    // Images
    "image/jpeg": true,
    "image/png":  true,
    "image/gif":  true,
    "image/webp": true,
    // Documents
    "application/pdf":    true,
    "text/plain":         true,
    // Add more as needed
}
```

### UploadMedia Command

```go
// UploadMediaInput contains data for uploading a new media file.
type UploadMediaInput struct {
    EntityType  string // "person", "family", "source"
    EntityID    uuid.UUID
    Title       string
    Description string
    Filename    string
    MimeType    string
    FileData    []byte
}

// UploadMediaResult contains the result of uploading media.
type UploadMediaResult struct {
    ID      uuid.UUID
    Version int64
}

// UploadMedia uploads a new media file and attaches it to an entity.
func (h *Handler) UploadMedia(ctx context.Context, input UploadMediaInput) (*UploadMediaResult, error) {
    // 1. Validate input
    if input.Title == "" {
        return nil, fmt.Errorf("%w: title is required", ErrInvalidInput)
    }
    if len(input.FileData) == 0 {
        return nil, fmt.Errorf("%w: file data is required", ErrInvalidInput)
    }
    if len(input.FileData) > MaxFileSize {
        return nil, ErrFileTooLarge
    }
    if !AllowedMimeTypes[input.MimeType] {
        return nil, fmt.Errorf("%w: %s", ErrUnsupportedFormat, input.MimeType)
    }

    // 2. Validate entity exists
    if err := h.validateEntity(ctx, input.EntityType, input.EntityID); err != nil {
        return nil, err
    }

    // 3. Determine media type from MIME type
    mediaType := inferMediaType(input.MimeType)

    // 4. Generate thumbnail for images
    var thumbnailData []byte
    if isImageMime(input.MimeType) {
        thumbnailData, _ = media.GenerateThumbnail(input.FileData, 300)
        // Ignore thumbnail errors - non-critical
    }

    // 5. Create domain entity
    m := domain.NewMedia(input.Title, input.EntityType, input.EntityID)
    m.Description = input.Description
    m.Filename = input.Filename
    m.MimeType = input.MimeType
    m.MediaType = mediaType
    m.FileSize = int64(len(input.FileData))
    m.FileData = input.FileData
    m.ThumbnailData = thumbnailData

    // 6. Validate
    if err := m.Validate(); err != nil {
        return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
    }

    // 7. Create and emit event
    event := domain.NewMediaCreated(m)
    version, err := h.execute(ctx, m.ID.String(), "Media", []domain.Event{event}, -1)
    if err != nil {
        return nil, err
    }

    return &UploadMediaResult{ID: m.ID, Version: version}, nil
}

// Helper to validate entity exists
func (h *Handler) validateEntity(ctx context.Context, entityType string, entityID uuid.UUID) error {
    switch entityType {
    case "person":
        p, err := h.readStore.GetPerson(ctx, entityID)
        if err != nil {
            return err
        }
        if p == nil {
            return fmt.Errorf("%w: person %s", ErrEntityNotFound, entityID)
        }
    case "family":
        f, err := h.readStore.GetFamily(ctx, entityID)
        if err != nil {
            return err
        }
        if f == nil {
            return fmt.Errorf("%w: family %s", ErrEntityNotFound, entityID)
        }
    case "source":
        s, err := h.readStore.GetSource(ctx, entityID)
        if err != nil {
            return err
        }
        if s == nil {
            return fmt.Errorf("%w: source %s", ErrEntityNotFound, entityID)
        }
    default:
        return fmt.Errorf("%w: invalid entity type: %s", ErrInvalidInput, entityType)
    }
    return nil
}

// inferMediaType determines MediaType from MIME type
func inferMediaType(mimeType string) domain.MediaType {
    switch {
    case strings.HasPrefix(mimeType, "image/"):
        return domain.MediaPhoto
    case strings.HasPrefix(mimeType, "audio/"):
        return domain.MediaAudio
    case strings.HasPrefix(mimeType, "video/"):
        return domain.MediaVideo
    case mimeType == "application/pdf":
        return domain.MediaDocument
    default:
        return domain.MediaDocument
    }
}

func isImageMime(mimeType string) bool {
    return strings.HasPrefix(mimeType, "image/")
}
```

### UpdateMedia Command

```go
// UpdateMediaInput contains data for updating media metadata.
type UpdateMediaInput struct {
    ID          uuid.UUID
    Title       *string
    Description *string
    MediaType   *string
    CropLeft    *int
    CropTop     *int
    CropWidth   *int
    CropHeight  *int
    Version     int64 // Required for optimistic locking
}

// UpdateMediaResult contains the result of updating media.
type UpdateMediaResult struct {
    Version int64
}

// UpdateMedia updates media metadata (not file content).
func (h *Handler) UpdateMedia(ctx context.Context, input UpdateMediaInput) (*UpdateMediaResult, error) {
    // 1. Get current media
    current, err := h.readStore.GetMedia(ctx, input.ID)
    if err != nil {
        return nil, err
    }
    if current == nil {
        return nil, ErrMediaNotFound
    }

    // 2. Check version for optimistic locking
    if current.Version != input.Version {
        return nil, repository.ErrConcurrencyConflict
    }

    // 3. Build changes map
    changes := make(map[string]any)

    if input.Title != nil {
        changes["title"] = *input.Title
    }
    if input.Description != nil {
        changes["description"] = *input.Description
    }
    if input.MediaType != nil {
        changes["media_type"] = *input.MediaType
    }
    if input.CropLeft != nil {
        changes["crop_left"] = *input.CropLeft
    }
    if input.CropTop != nil {
        changes["crop_top"] = *input.CropTop
    }
    if input.CropWidth != nil {
        changes["crop_width"] = *input.CropWidth
    }
    if input.CropHeight != nil {
        changes["crop_height"] = *input.CropHeight
    }

    // 4. No changes?
    if len(changes) == 0 {
        return &UpdateMediaResult{Version: current.Version}, nil
    }

    // 5. Validate changes
    if input.Title != nil && *input.Title == "" {
        return nil, fmt.Errorf("%w: title cannot be empty", ErrInvalidInput)
    }

    // 6. Create and emit event
    event := domain.NewMediaUpdated(input.ID, changes)
    version, err := h.execute(ctx, input.ID.String(), "Media", []domain.Event{event}, input.Version)
    if err != nil {
        return nil, err
    }

    return &UpdateMediaResult{Version: version}, nil
}
```

### DeleteMedia Command

```go
// DeleteMedia deletes a media file.
func (h *Handler) DeleteMedia(ctx context.Context, id uuid.UUID, version int64, reason string) error {
    // 1. Get current media
    current, err := h.readStore.GetMedia(ctx, id)
    if err != nil {
        return err
    }
    if current == nil {
        return ErrMediaNotFound
    }

    // 2. Check version
    if current.Version != version {
        return repository.ErrConcurrencyConflict
    }

    // 3. Create and emit event
    event := domain.NewMediaDeleted(id, reason)
    _, err = h.execute(ctx, id.String(), "Media", []domain.Event{event}, version)
    return err
}
```

</requirements>

<implementation>

1. Create `internal/command/media_commands.go`
2. Add errors, constants, types, and command methods
3. Import required packages (context, fmt, strings, uuid, domain, repository, media)
4. Note: `media.GenerateThumbnail` comes from Stage 7 - stub it if not yet available
5. Run `go build ./internal/command/...`

</implementation>

<verification>
```bash
# Build command package
go build ./internal/command/...

# Run command tests (if any)
go test ./internal/command/... -v

# Verify no circular imports
go vet ./internal/command/...
```
</verification>

<output>
After completing this stage, provide:
1. Commands implemented
2. Validation rules
3. Error conditions handled

Example output:
```
Stage 6 Complete: Command Handlers

Created internal/command/media_commands.go

Commands:
- UploadMedia: validates input, checks entity exists, generates thumbnail, creates event
- UpdateMedia: optimistic locking, partial updates via changes map
- DeleteMedia: version check, soft delete via event

Validation rules:
- Title required, max 500 chars
- File size max 10MB (MaxFileSize constant)
- MIME type whitelist (AllowedMimeTypes)
- Entity must exist (person/family/source)
- Version required for update/delete

Error types:
- ErrMediaNotFound
- ErrFileTooLarge
- ErrUnsupportedFormat
- ErrEntityNotFound

Build verification: go build passed

Ready for Stage 8 (API)
```
</output>
