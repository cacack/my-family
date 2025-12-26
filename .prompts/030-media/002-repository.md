# Stage 2: Repository Interface

<objective>
Extend the ReadModelStore interface with media-specific operations and add the MediaReadModel struct for read-side queries.
</objective>

<context>
Reference existing patterns in:
- @file:internal/repository/readmodel.go - ReadModelStore interface, SourceReadModel struct

Dependencies:
- Stage 1 must be complete (domain.MediaType exists)
</context>

<requirements>

## 1. MediaReadModel Struct

Add to `internal/repository/readmodel.go`:

```go
// MediaReadModel represents a media file in the read model.
type MediaReadModel struct {
    ID            uuid.UUID        `json:"id"`
    EntityType    string           `json:"entity_type"`
    EntityID      uuid.UUID        `json:"entity_id"`
    Title         string           `json:"title"`
    Description   string           `json:"description,omitempty"`
    MimeType      string           `json:"mime_type"`
    MediaType     domain.MediaType `json:"media_type"`
    Filename      string           `json:"filename"`
    FileSize      int64            `json:"file_size"`
    FileData      []byte           `json:"-"` // Excluded from JSON by default
    ThumbnailData []byte           `json:"-"` // Excluded from JSON by default
    CropLeft      *int             `json:"crop_left,omitempty"`
    CropTop       *int             `json:"crop_top,omitempty"`
    CropWidth     *int             `json:"crop_width,omitempty"`
    CropHeight    *int             `json:"crop_height,omitempty"`
    GedcomXref    string           `json:"gedcom_xref,omitempty"`
    Version       int64            `json:"version"`
    CreatedAt     time.Time        `json:"created_at"`
    UpdatedAt     time.Time        `json:"updated_at"`
}
```

Note: FileData and ThumbnailData use `json:"-"` to prevent accidental inclusion in API responses. These are fetched separately via dedicated endpoints.

## 2. ReadModelStore Interface Extensions

Add these methods to the `ReadModelStore` interface:

```go
// Media operations
GetMedia(ctx context.Context, id uuid.UUID) (*MediaReadModel, error)
GetMediaWithData(ctx context.Context, id uuid.UUID) (*MediaReadModel, error) // Includes FileData
GetMediaThumbnail(ctx context.Context, id uuid.UUID) ([]byte, error)
ListMediaForEntity(ctx context.Context, entityType string, entityID uuid.UUID, opts ListOptions) ([]MediaReadModel, int, error)
SaveMedia(ctx context.Context, media *MediaReadModel) error
DeleteMedia(ctx context.Context, id uuid.UUID) error
```

### Method Specifications

| Method | Returns | Notes |
|--------|---------|-------|
| GetMedia | Metadata only | FileData and ThumbnailData are nil |
| GetMediaWithData | Full record | Includes FileData, ThumbnailData |
| GetMediaThumbnail | []byte | Just thumbnail bytes for efficient serving |
| ListMediaForEntity | List + count | Paginated, ordered by created_at DESC |
| SaveMedia | error | Upsert (INSERT ON CONFLICT UPDATE) |
| DeleteMedia | error | Hard delete |

</requirements>

<implementation>

1. Open `internal/repository/readmodel.go`
2. Add MediaReadModel struct after CitationReadModel (around line 102)
3. Add media methods to ReadModelStore interface (in the interface block)
4. Group with comment `// Media operations`
5. Run `go build ./internal/repository/...` to verify

</implementation>

<verification>
```bash
# Build repository package (will fail until postgres/sqlite implement interface)
go build ./internal/repository/...

# Verify interface is syntactically correct
go vet ./internal/repository/...
```

Note: Build may show errors for postgres/sqlite packages not implementing the new interface methods. This is expected - they will be implemented in Stages 3 and 4.
</verification>

<output>
After completing this stage, provide:
1. MediaReadModel fields summary
2. Interface methods added
3. Any notes on design decisions

Example output:
```
Stage 2 Complete: Repository Interface

Added MediaReadModel struct with 17 fields:
- Metadata: ID, EntityType, EntityID, Title, Description, MimeType, MediaType, Filename, FileSize
- Binary: FileData, ThumbnailData (json:"-")
- Crop: CropLeft, CropTop, CropWidth, CropHeight
- Tracking: GedcomXref, Version, CreatedAt, UpdatedAt

Extended ReadModelStore interface with 6 methods:
- GetMedia (metadata only)
- GetMediaWithData (includes binary)
- GetMediaThumbnail (thumbnail bytes only)
- ListMediaForEntity (paginated list)
- SaveMedia (upsert)
- DeleteMedia (hard delete)

Ready for Stages 3, 4, 7 (can run in parallel):
- Stage 3: PostgreSQL implementation
- Stage 4: SQLite implementation
- Stage 7: Thumbnail generation
```
</output>
