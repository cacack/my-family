# Stage 5: Projection Handler

<objective>
Add event handlers for MediaCreated, MediaUpdated, and MediaDeleted events to project domain events into the read model.
</objective>

<context>
Reference existing patterns in:
- @file:internal/repository/projection.go - Projector struct, Project() switch statement, projection methods

Dependencies:
- Stage 1 complete (MediaCreated, MediaUpdated, MediaDeleted events)
- Stage 2 complete (MediaReadModel, interface methods)
- Stages 3 and 4 complete (PostgreSQL and SQLite implementations)
</context>

<requirements>

## 1. Update Project() Method

Add cases to the switch statement in `Project()` method:

```go
func (p *Projector) Project(ctx context.Context, event domain.Event, version int64) error {
    switch e := event.(type) {
    // ... existing cases ...
    case domain.MediaCreated:
        return p.projectMediaCreated(ctx, e, version)
    case domain.MediaUpdated:
        return p.projectMediaUpdated(ctx, e, version)
    case domain.MediaDeleted:
        return p.projectMediaDeleted(ctx, e)
    default:
        return nil
    }
}
```

## 2. Implement Projection Methods

### projectMediaCreated

```go
func (p *Projector) projectMediaCreated(ctx context.Context, e domain.MediaCreated, version int64) error {
    media := &MediaReadModel{
        ID:            e.MediaID,
        EntityType:    e.EntityType,
        EntityID:      e.EntityID,
        Title:         e.Title,
        Description:   e.Description,
        MimeType:      e.MimeType,
        MediaType:     e.MediaType,
        Filename:      e.Filename,
        FileSize:      e.FileSize,
        FileData:      e.FileData,
        ThumbnailData: e.ThumbnailData,
        GedcomXref:    e.GedcomXref,
        Version:       version,
        CreatedAt:     e.OccurredAt(),
        UpdatedAt:     e.OccurredAt(),
    }

    return p.readStore.SaveMedia(ctx, media)
}
```

### projectMediaUpdated

Handle field updates from the Changes map:

```go
func (p *Projector) projectMediaUpdated(ctx context.Context, e domain.MediaUpdated, version int64) error {
    media, err := p.readStore.GetMediaWithData(ctx, e.MediaID)
    if err != nil {
        return err
    }
    if media == nil {
        return nil // Media doesn't exist in read model, skip
    }

    // Apply changes
    for key, value := range e.Changes {
        switch key {
        case "title":
            if v, ok := value.(string); ok {
                media.Title = v
            }
        case "description":
            if v, ok := value.(string); ok {
                media.Description = v
            }
        case "media_type":
            if v, ok := value.(string); ok {
                media.MediaType = domain.MediaType(v)
            }
        case "file_data":
            if v, ok := value.([]byte); ok {
                media.FileData = v
            }
        case "thumbnail_data":
            if v, ok := value.([]byte); ok {
                media.ThumbnailData = v
            }
        case "file_size":
            if v, ok := value.(int64); ok {
                media.FileSize = v
            }
        case "mime_type":
            if v, ok := value.(string); ok {
                media.MimeType = v
            }
        case "filename":
            if v, ok := value.(string); ok {
                media.Filename = v
            }
        case "crop_left":
            if v, ok := value.(int); ok {
                media.CropLeft = &v
            } else if value == nil {
                media.CropLeft = nil
            }
        case "crop_top":
            if v, ok := value.(int); ok {
                media.CropTop = &v
            } else if value == nil {
                media.CropTop = nil
            }
        case "crop_width":
            if v, ok := value.(int); ok {
                media.CropWidth = &v
            } else if value == nil {
                media.CropWidth = nil
            }
        case "crop_height":
            if v, ok := value.(int); ok {
                media.CropHeight = &v
            } else if value == nil {
                media.CropHeight = nil
            }
        }
    }

    media.Version = version
    media.UpdatedAt = e.OccurredAt()

    return p.readStore.SaveMedia(ctx, media)
}
```

### projectMediaDeleted

```go
func (p *Projector) projectMediaDeleted(ctx context.Context, e domain.MediaDeleted) error {
    return p.readStore.DeleteMedia(ctx, e.MediaID)
}
```

</requirements>

<implementation>

1. Open `internal/repository/projection.go`
2. Add three cases to `Project()` switch statement
3. Add three projection methods at the end of the file
4. Import `domain` package if not already imported
5. Run `go build ./internal/repository/...`

</implementation>

<verification>
```bash
# Build repository package
go build ./internal/repository/...

# Run projection tests
go test ./internal/repository/... -v -run Projection

# Verify no interface mismatches
go vet ./internal/repository/...
```
</verification>

<output>
After completing this stage, provide:
1. Events handled
2. Fields projected in updates
3. Any edge cases handled

Example output:
```
Stage 5 Complete: Projection Handler

Added to Project() switch:
- domain.MediaCreated -> projectMediaCreated
- domain.MediaUpdated -> projectMediaUpdated
- domain.MediaDeleted -> projectMediaDeleted

projectMediaCreated:
- Maps all 13 fields from event to MediaReadModel
- Sets Version, CreatedAt, UpdatedAt from event

projectMediaUpdated:
- Handles 12 changeable fields
- Preserves binary data unless explicitly changed
- Supports null crop values

projectMediaDeleted:
- Delegates to DeleteMedia (hard delete)

Build verification: go build passed

Ready for Stage 6 (Command Handlers)
```
</output>
