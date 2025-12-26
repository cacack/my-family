# Stage 3: PostgreSQL Implementation

<objective>
Implement the media ReadModelStore methods for PostgreSQL using BYTEA columns for binary data storage.
</objective>

<context>
Reference existing patterns in:
- @file:internal/repository/postgres/readmodel.go - Table creation, scan helpers, CRUD operations
- @file:internal/repository/readmodel.go - MediaReadModel struct, interface methods

Dependencies:
- Stage 2 must be complete (MediaReadModel and interface methods defined)

Can run in parallel with:
- Stage 4 (SQLite implementation)
- Stage 7 (Thumbnail generation)
</context>

<requirements>

## 1. Media Table Schema

Add to `createTables()` in `internal/repository/postgres/readmodel.go`:

```sql
-- Media table
CREATE TABLE IF NOT EXISTS media (
    id UUID PRIMARY KEY,
    entity_type VARCHAR(20) NOT NULL,
    entity_id UUID NOT NULL,
    title VARCHAR(500) NOT NULL,
    description TEXT,
    mime_type VARCHAR(100) NOT NULL,
    media_type VARCHAR(20) NOT NULL,
    filename VARCHAR(255) NOT NULL,
    file_size BIGINT NOT NULL,
    file_data BYTEA NOT NULL,
    thumbnail_data BYTEA,
    crop_left INTEGER,
    crop_top INTEGER,
    crop_width INTEGER,
    crop_height INTEGER,
    gedcom_xref VARCHAR(50),
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for entity lookups (most common query pattern)
CREATE INDEX IF NOT EXISTS idx_media_entity ON media(entity_type, entity_id);

-- Index for listing by type
CREATE INDEX IF NOT EXISTS idx_media_type ON media(media_type);
```

## 2. Implement Interface Methods

### GetMedia (metadata only, no binary data)

```go
func (s *ReadModelStore) GetMedia(ctx context.Context, id uuid.UUID) (*repository.MediaReadModel, error) {
    row := s.db.QueryRowContext(ctx, `
        SELECT id, entity_type, entity_id, title, description, mime_type, media_type,
               filename, file_size, crop_left, crop_top, crop_width, crop_height,
               gedcom_xref, version, created_at, updated_at
        FROM media WHERE id = $1
    `, id)

    return scanMediaMetadata(row)
}
```

### GetMediaWithData (includes FileData, ThumbnailData)

```go
func (s *ReadModelStore) GetMediaWithData(ctx context.Context, id uuid.UUID) (*repository.MediaReadModel, error) {
    row := s.db.QueryRowContext(ctx, `
        SELECT id, entity_type, entity_id, title, description, mime_type, media_type,
               filename, file_size, file_data, thumbnail_data,
               crop_left, crop_top, crop_width, crop_height,
               gedcom_xref, version, created_at, updated_at
        FROM media WHERE id = $1
    `, id)

    return scanMediaFull(row)
}
```

### GetMediaThumbnail (just thumbnail bytes)

```go
func (s *ReadModelStore) GetMediaThumbnail(ctx context.Context, id uuid.UUID) ([]byte, error) {
    var thumbnail []byte
    err := s.db.QueryRowContext(ctx, `
        SELECT thumbnail_data FROM media WHERE id = $1
    `, id).Scan(&thumbnail)

    if err == sql.ErrNoRows {
        return nil, nil
    }
    return thumbnail, err
}
```

### ListMediaForEntity (paginated)

```go
func (s *ReadModelStore) ListMediaForEntity(ctx context.Context, entityType string, entityID uuid.UUID, opts repository.ListOptions) ([]repository.MediaReadModel, int, error) {
    // Count total
    var total int
    err := s.db.QueryRowContext(ctx,
        "SELECT COUNT(*) FROM media WHERE entity_type = $1 AND entity_id = $2",
        entityType, entityID).Scan(&total)
    if err != nil {
        return nil, 0, fmt.Errorf("count media: %w", err)
    }

    // Query with pagination (metadata only, ordered by created_at DESC)
    rows, err := s.db.QueryContext(ctx, `
        SELECT id, entity_type, entity_id, title, description, mime_type, media_type,
               filename, file_size, crop_left, crop_top, crop_width, crop_height,
               gedcom_xref, version, created_at, updated_at
        FROM media
        WHERE entity_type = $1 AND entity_id = $2
        ORDER BY created_at DESC
        LIMIT $3 OFFSET $4
    `, entityType, entityID, opts.Limit, opts.Offset)
    if err != nil {
        return nil, 0, fmt.Errorf("query media: %w", err)
    }
    defer rows.Close()

    var items []repository.MediaReadModel
    for rows.Next() {
        m, err := scanMediaMetadataRow(rows)
        if err != nil {
            return nil, 0, err
        }
        items = append(items, *m)
    }

    return items, total, rows.Err()
}
```

### SaveMedia (upsert)

```go
func (s *ReadModelStore) SaveMedia(ctx context.Context, media *repository.MediaReadModel) error {
    _, err := s.db.ExecContext(ctx, `
        INSERT INTO media (id, entity_type, entity_id, title, description, mime_type, media_type,
                          filename, file_size, file_data, thumbnail_data,
                          crop_left, crop_top, crop_width, crop_height,
                          gedcom_xref, version, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
        ON CONFLICT(id) DO UPDATE SET
            entity_type = EXCLUDED.entity_type,
            entity_id = EXCLUDED.entity_id,
            title = EXCLUDED.title,
            description = EXCLUDED.description,
            mime_type = EXCLUDED.mime_type,
            media_type = EXCLUDED.media_type,
            filename = EXCLUDED.filename,
            file_size = EXCLUDED.file_size,
            file_data = EXCLUDED.file_data,
            thumbnail_data = EXCLUDED.thumbnail_data,
            crop_left = EXCLUDED.crop_left,
            crop_top = EXCLUDED.crop_top,
            crop_width = EXCLUDED.crop_width,
            crop_height = EXCLUDED.crop_height,
            gedcom_xref = EXCLUDED.gedcom_xref,
            version = EXCLUDED.version,
            updated_at = EXCLUDED.updated_at
    `, media.ID, media.EntityType, media.EntityID, media.Title,
       nullableString(media.Description), media.MimeType, string(media.MediaType),
       media.Filename, media.FileSize, media.FileData, media.ThumbnailData,
       nullableInt(media.CropLeft), nullableInt(media.CropTop),
       nullableInt(media.CropWidth), nullableInt(media.CropHeight),
       nullableString(media.GedcomXref), media.Version, media.CreatedAt, media.UpdatedAt)

    return err
}
```

### DeleteMedia

```go
func (s *ReadModelStore) DeleteMedia(ctx context.Context, id uuid.UUID) error {
    _, err := s.db.ExecContext(ctx, "DELETE FROM media WHERE id = $1", id)
    return err
}
```

## 3. Scanner Helper Functions

Add scan helpers following existing pattern:

```go
func scanMediaMetadata(row rowScanner) (*repository.MediaReadModel, error) {
    var (
        id, entityID                      uuid.UUID
        entityType, title, mimeType       string
        mediaType, filename               string
        description, gedcomXref           sql.NullString
        fileSize, version                 int64
        cropLeft, cropTop                 sql.NullInt64
        cropWidth, cropHeight             sql.NullInt64
        createdAt, updatedAt              time.Time
    )

    err := row.Scan(&id, &entityType, &entityID, &title, &description,
        &mimeType, &mediaType, &filename, &fileSize,
        &cropLeft, &cropTop, &cropWidth, &cropHeight,
        &gedcomXref, &version, &createdAt, &updatedAt)

    if err == sql.ErrNoRows {
        return nil, nil
    }
    if err != nil {
        return nil, fmt.Errorf("scan media metadata: %w", err)
    }

    m := &repository.MediaReadModel{
        ID:          id,
        EntityType:  entityType,
        EntityID:    entityID,
        Title:       title,
        Description: description.String,
        MimeType:    mimeType,
        MediaType:   domain.MediaType(mediaType),
        Filename:    filename,
        FileSize:    fileSize,
        GedcomXref:  gedcomXref.String,
        Version:     version,
        CreatedAt:   createdAt,
        UpdatedAt:   updatedAt,
    }

    if cropLeft.Valid {
        v := int(cropLeft.Int64)
        m.CropLeft = &v
    }
    if cropTop.Valid {
        v := int(cropTop.Int64)
        m.CropTop = &v
    }
    if cropWidth.Valid {
        v := int(cropWidth.Int64)
        m.CropWidth = &v
    }
    if cropHeight.Valid {
        v := int(cropHeight.Int64)
        m.CropHeight = &v
    }

    return m, nil
}

func scanMediaMetadataRow(rows *sql.Rows) (*repository.MediaReadModel, error) {
    return scanMediaMetadata(rows)
}

func scanMediaFull(row rowScanner) (*repository.MediaReadModel, error) {
    // Similar to scanMediaMetadata but also scans file_data and thumbnail_data
    // ... (include FileData and ThumbnailData in scan)
}
```

</requirements>

<implementation>

1. Add media table to `createTables()` in `internal/repository/postgres/readmodel.go`
2. Add scanner helper functions after existing scan helpers
3. Implement all 6 interface methods
4. Run `go build ./internal/repository/postgres/...`

</implementation>

<verification>
```bash
# Build postgres package
go build ./internal/repository/postgres/...

# Verify interface implementation
go vet ./internal/repository/postgres/...

# Run existing postgres tests (may need test DB)
go test ./internal/repository/postgres/... -v
```
</verification>

<output>
After completing this stage, provide:
1. Table schema created
2. Methods implemented
3. Index strategy
4. Any PostgreSQL-specific optimizations

Example output:
```
Stage 3 Complete: PostgreSQL Implementation

Created media table:
- 19 columns including BYTEA for file_data, thumbnail_data
- Indexes: idx_media_entity (entity_type, entity_id), idx_media_type

Implemented 6 ReadModelStore methods:
- GetMedia (metadata only, excludes binary)
- GetMediaWithData (full record with binary)
- GetMediaThumbnail (thumbnail bytes only)
- ListMediaForEntity (paginated, created_at DESC)
- SaveMedia (upsert)
- DeleteMedia (hard delete)

Added scanner helpers:
- scanMediaMetadata
- scanMediaMetadataRow
- scanMediaFull

Build verification: go build passed

Ready for Stage 5 (after Stage 4 completes)
```
</output>
