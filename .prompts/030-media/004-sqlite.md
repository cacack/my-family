# Stage 4: SQLite Implementation

<objective>
Implement the media ReadModelStore methods for SQLite using BLOB columns for binary data storage.
</objective>

<context>
Reference existing patterns in:
- @file:internal/repository/sqlite/readmodel.go - Table creation, scan helpers, CRUD operations
- @file:internal/repository/readmodel.go - MediaReadModel struct, interface methods

Dependencies:
- Stage 2 must be complete (MediaReadModel and interface methods defined)

Can run in parallel with:
- Stage 3 (PostgreSQL implementation)
- Stage 7 (Thumbnail generation)
</context>

<requirements>

## 1. Media Table Schema

Add to `createTables()` in `internal/repository/sqlite/readmodel.go`:

```sql
-- Media table
CREATE TABLE IF NOT EXISTS media (
    id TEXT PRIMARY KEY,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT,
    mime_type TEXT NOT NULL,
    media_type TEXT NOT NULL,
    filename TEXT NOT NULL,
    file_size INTEGER NOT NULL,
    file_data BLOB NOT NULL,
    thumbnail_data BLOB,
    crop_left INTEGER,
    crop_top INTEGER,
    crop_width INTEGER,
    crop_height INTEGER,
    gedcom_xref TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Index for entity lookups (most common query pattern)
CREATE INDEX IF NOT EXISTS idx_media_entity ON media(entity_type, entity_id);

-- Index for listing by type
CREATE INDEX IF NOT EXISTS idx_media_type ON media(media_type);
```

Note SQLite differences from PostgreSQL:
- UUID stored as TEXT
- BIGINT becomes INTEGER
- BYTEA becomes BLOB
- TIMESTAMPTZ becomes TEXT (ISO 8601 format)
- No DEFAULT NOW() - must provide timestamp in Go

## 2. Implement Interface Methods

Follow the PostgreSQL implementation pattern but with SQLite-specific SQL syntax:

### GetMedia (metadata only)

```go
func (s *ReadModelStore) GetMedia(ctx context.Context, id uuid.UUID) (*repository.MediaReadModel, error) {
    row := s.db.QueryRowContext(ctx, `
        SELECT id, entity_type, entity_id, title, description, mime_type, media_type,
               filename, file_size, crop_left, crop_top, crop_width, crop_height,
               gedcom_xref, version, created_at, updated_at
        FROM media WHERE id = ?
    `, id.String())

    return scanMediaMetadata(row)
}
```

### GetMediaWithData (includes binary)

```go
func (s *ReadModelStore) GetMediaWithData(ctx context.Context, id uuid.UUID) (*repository.MediaReadModel, error) {
    row := s.db.QueryRowContext(ctx, `
        SELECT id, entity_type, entity_id, title, description, mime_type, media_type,
               filename, file_size, file_data, thumbnail_data,
               crop_left, crop_top, crop_width, crop_height,
               gedcom_xref, version, created_at, updated_at
        FROM media WHERE id = ?
    `, id.String())

    return scanMediaFull(row)
}
```

### GetMediaThumbnail

```go
func (s *ReadModelStore) GetMediaThumbnail(ctx context.Context, id uuid.UUID) ([]byte, error) {
    var thumbnail []byte
    err := s.db.QueryRowContext(ctx, `
        SELECT thumbnail_data FROM media WHERE id = ?
    `, id.String()).Scan(&thumbnail)

    if err == sql.ErrNoRows {
        return nil, nil
    }
    return thumbnail, err
}
```

### ListMediaForEntity

```go
func (s *ReadModelStore) ListMediaForEntity(ctx context.Context, entityType string, entityID uuid.UUID, opts repository.ListOptions) ([]repository.MediaReadModel, int, error) {
    // Count total
    var total int
    err := s.db.QueryRowContext(ctx,
        "SELECT COUNT(*) FROM media WHERE entity_type = ? AND entity_id = ?",
        entityType, entityID.String()).Scan(&total)
    if err != nil {
        return nil, 0, fmt.Errorf("count media: %w", err)
    }

    // Query with pagination
    rows, err := s.db.QueryContext(ctx, `
        SELECT id, entity_type, entity_id, title, description, mime_type, media_type,
               filename, file_size, crop_left, crop_top, crop_width, crop_height,
               gedcom_xref, version, created_at, updated_at
        FROM media
        WHERE entity_type = ? AND entity_id = ?
        ORDER BY created_at DESC
        LIMIT ? OFFSET ?
    `, entityType, entityID.String(), opts.Limit, opts.Offset)
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

### SaveMedia (upsert with INSERT OR REPLACE)

```go
func (s *ReadModelStore) SaveMedia(ctx context.Context, media *repository.MediaReadModel) error {
    _, err := s.db.ExecContext(ctx, `
        INSERT OR REPLACE INTO media (
            id, entity_type, entity_id, title, description, mime_type, media_type,
            filename, file_size, file_data, thumbnail_data,
            crop_left, crop_top, crop_width, crop_height,
            gedcom_xref, version, created_at, updated_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `, media.ID.String(), media.EntityType, media.EntityID.String(),
       media.Title, nullableString(media.Description), media.MimeType, string(media.MediaType),
       media.Filename, media.FileSize, media.FileData, media.ThumbnailData,
       nullableInt(media.CropLeft), nullableInt(media.CropTop),
       nullableInt(media.CropWidth), nullableInt(media.CropHeight),
       nullableString(media.GedcomXref), media.Version,
       media.CreatedAt.Format(time.RFC3339), media.UpdatedAt.Format(time.RFC3339))

    return err
}
```

### DeleteMedia

```go
func (s *ReadModelStore) DeleteMedia(ctx context.Context, id uuid.UUID) error {
    _, err := s.db.ExecContext(ctx, "DELETE FROM media WHERE id = ?", id.String())
    return err
}
```

## 3. Scanner Helper Functions

Add scan helpers that handle SQLite's TEXT-based UUIDs and timestamps:

```go
func scanMediaMetadata(row rowScanner) (*repository.MediaReadModel, error) {
    var (
        idStr, entityType, entityIDStr    string
        title, mimeType, mediaType        string
        filename                          string
        description, gedcomXref           sql.NullString
        fileSize, version                 int64
        cropLeft, cropTop                 sql.NullInt64
        cropWidth, cropHeight             sql.NullInt64
        createdAtStr, updatedAtStr        string
    )

    err := row.Scan(&idStr, &entityType, &entityIDStr, &title, &description,
        &mimeType, &mediaType, &filename, &fileSize,
        &cropLeft, &cropTop, &cropWidth, &cropHeight,
        &gedcomXref, &version, &createdAtStr, &updatedAtStr)

    if err == sql.ErrNoRows {
        return nil, nil
    }
    if err != nil {
        return nil, fmt.Errorf("scan media metadata: %w", err)
    }

    id, _ := uuid.Parse(idStr)
    entityID, _ := uuid.Parse(entityIDStr)
    createdAt, _ := time.Parse(time.RFC3339, createdAtStr)
    updatedAt, _ := time.Parse(time.RFC3339, updatedAtStr)

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

    // Handle nullable crop fields
    if cropLeft.Valid {
        v := int(cropLeft.Int64)
        m.CropLeft = &v
    }
    // ... similar for cropTop, cropWidth, cropHeight

    return m, nil
}

func scanMediaMetadataRow(rows *sql.Rows) (*repository.MediaReadModel, error) {
    return scanMediaMetadata(rows)
}

func scanMediaFull(row rowScanner) (*repository.MediaReadModel, error) {
    // Similar but includes file_data and thumbnail_data scanning
}
```

</requirements>

<implementation>

1. Add media table to `createTables()` in `internal/repository/sqlite/readmodel.go`
2. Add scanner helper functions
3. Implement all 6 interface methods
4. Run `go build ./internal/repository/sqlite/...`

</implementation>

<verification>
```bash
# Build sqlite package
go build ./internal/repository/sqlite/...

# Verify interface implementation
go vet ./internal/repository/sqlite/...

# Run existing sqlite tests
go test ./internal/repository/sqlite/... -v
```
</verification>

<output>
After completing this stage, provide:
1. SQLite-specific schema differences
2. Methods implemented
3. Any SQLite-specific considerations

Example output:
```
Stage 4 Complete: SQLite Implementation

Created media table:
- 19 columns using TEXT for UUIDs, BLOB for binary data
- Indexes: idx_media_entity, idx_media_type
- Timestamps stored as RFC3339 TEXT

Implemented 6 ReadModelStore methods:
- GetMedia, GetMediaWithData, GetMediaThumbnail
- ListMediaForEntity, SaveMedia, DeleteMedia

SQLite-specific differences:
- UUID as TEXT, not UUID type
- INSERT OR REPLACE instead of ON CONFLICT
- Timestamps as TEXT with RFC3339 parsing
- Placeholder ? instead of $1

Build verification: go build passed

Ready for Stage 5 (Projection Handler)
```
</output>
