# Stage 1: Domain Foundation

<objective>
Create the domain model, enums, and events for media management. This establishes the core data structures that all subsequent stages depend on.
</objective>

<context>
Reference existing patterns in:
- @file:internal/domain/enums.go - Enum pattern with IsValid() methods
- @file:internal/domain/events.go - Event pattern with BaseEvent, NewXxxCreated/Updated/Deleted
- @file:internal/domain/source.go - Entity pattern with NewSource(), Validate()
</context>

<requirements>

## 1. MediaType Enum (internal/domain/enums.go)

Add MediaType enum for categorizing uploaded files:

```go
// MediaType represents the type of media file.
type MediaType string

const (
    MediaPhoto       MediaType = "photo"
    MediaDocument    MediaType = "document"
    MediaAudio       MediaType = "audio"
    MediaVideo       MediaType = "video"
    MediaCertificate MediaType = "certificate"
)

// IsValid checks if the media type value is valid.
func (m MediaType) IsValid() bool {
    switch m {
    case MediaPhoto, MediaDocument, MediaAudio, MediaVideo, MediaCertificate, "":
        return true
    default:
        return false
    }
}
```

## 2. Media Entity (internal/domain/media.go - new file)

Create Media struct with these fields:

| Field | Type | Description |
|-------|------|-------------|
| ID | uuid.UUID | Primary key |
| EntityType | string | "person", "family", "source" |
| EntityID | uuid.UUID | ID of attached entity |
| Title | string | Display title (required) |
| Description | string | Optional description |
| MimeType | string | e.g., "image/jpeg" |
| MediaType | MediaType | Category enum |
| Filename | string | Original filename |
| FileSize | int64 | Size in bytes |
| FileData | []byte | Raw file content |
| ThumbnailData | []byte | Generated thumbnail (images only) |
| CropLeft | *int | Optional crop coordinates |
| CropTop | *int | |
| CropWidth | *int | |
| CropHeight | *int | |
| GedcomXref | string | GEDCOM OBJE xref if imported |
| Version | int64 | Optimistic locking |

Include:
- `NewMedia(title string, entityType string, entityID uuid.UUID) *Media`
- `Validate() error` checking:
  - Title required, max 500 chars
  - EntityType must be "person", "family", or "source"
  - EntityID required (non-nil UUID)
  - FileSize must be > 0 if FileData present
  - FileSize max 10MB (10 * 1024 * 1024)
  - MimeType required if FileData present

## 3. Media Events (internal/domain/events.go)

Add three events following existing patterns:

### MediaCreated
```go
type MediaCreated struct {
    BaseEvent
    MediaID       uuid.UUID `json:"media_id"`
    EntityType    string    `json:"entity_type"`
    EntityID      uuid.UUID `json:"entity_id"`
    Title         string    `json:"title"`
    Description   string    `json:"description,omitempty"`
    MimeType      string    `json:"mime_type"`
    MediaType     MediaType `json:"media_type"`
    Filename      string    `json:"filename"`
    FileSize      int64     `json:"file_size"`
    FileData      []byte    `json:"file_data"`
    ThumbnailData []byte    `json:"thumbnail_data,omitempty"`
    GedcomXref    string    `json:"gedcom_xref,omitempty"`
}

func (e MediaCreated) EventType() string      { return "MediaCreated" }
func (e MediaCreated) AggregateID() uuid.UUID { return e.MediaID }

func NewMediaCreated(m *Media) MediaCreated
```

### MediaUpdated
```go
type MediaUpdated struct {
    BaseEvent
    MediaID uuid.UUID      `json:"media_id"`
    Changes map[string]any `json:"changes"`
}

func (e MediaUpdated) EventType() string      { return "MediaUpdated" }
func (e MediaUpdated) AggregateID() uuid.UUID { return e.MediaID }

func NewMediaUpdated(mediaID uuid.UUID, changes map[string]any) MediaUpdated
```

### MediaDeleted
```go
type MediaDeleted struct {
    BaseEvent
    MediaID uuid.UUID `json:"media_id"`
    Reason  string    `json:"reason,omitempty"`
}

func (e MediaDeleted) EventType() string      { return "MediaDeleted" }
func (e MediaDeleted) AggregateID() uuid.UUID { return e.MediaID }

func NewMediaDeleted(mediaID uuid.UUID, reason string) MediaDeleted
```

</requirements>

<implementation>

1. Add MediaType enum to `internal/domain/enums.go`
2. Create `internal/domain/media.go` with Media struct and methods
3. Add events to `internal/domain/events.go` (append at end of file)
4. Run `go build ./internal/domain/...` to verify compilation

</implementation>

<verification>
```bash
# Build domain package
go build ./internal/domain/...

# Run existing domain tests
go test ./internal/domain/...

# Verify new types are exported
go doc github.com/cacack/my-family/internal/domain.MediaType
go doc github.com/cacack/my-family/internal/domain.Media
go doc github.com/cacack/my-family/internal/domain.MediaCreated
```
</verification>

<output>
After completing this stage, provide:
1. Summary of types added
2. File paths modified/created
3. Any validation rules implemented
4. Ready signal for Stage 2

Example output:
```
Stage 1 Complete: Domain Foundation

Added to internal/domain/enums.go:
- MediaType enum with 5 values + IsValid()

Created internal/domain/media.go:
- Media struct with 15 fields
- NewMedia() constructor
- Validate() with 6 validation rules

Added to internal/domain/events.go:
- MediaCreated event
- MediaUpdated event
- MediaDeleted event

Validation: go build ./internal/domain/... passed

Ready for Stage 2: Repository Interface
```
</output>
