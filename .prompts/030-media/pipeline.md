# Media Management Foundation Pipeline

Issue: #30 - Media Management Foundation
Goal: Add photo/document upload, thumbnails, gallery view, person attachments

## Pipeline Overview

This multi-stage pipeline implements media management for the genealogy platform following existing event-sourcing patterns.

## Execution Order

```
Stage 1: Foundation (Domain + Events)
    |
    v
Stage 2: Repository Interface
    |
    +---> Stage 3: PostgreSQL Implementation ----+
    |                                             |
    +---> Stage 4: SQLite Implementation ---------+
    |                                             |
    +---> Stage 7: Thumbnail Generation ----------+
          (can run in parallel)                   |
                                                  v
                                           Stage 5: Projection Handler
                                                  |
                                                  v
                                           Stage 6: Command Handlers
                                                  |
                                                  v
                                           Stage 8: OpenAPI + HTTP Handlers
                                                  |
                                                  v
                                           Stage 9: GEDCOM Integration
                                                  |
                                                  v
                                           Stage 10: Tests
```

## Stage Files

| Stage | File | Description | Dependencies |
|-------|------|-------------|--------------|
| 1 | `001-foundation.md` | Domain model, events, MediaType enum | None |
| 2 | `002-repository.md` | ReadModel interface additions | Stage 1 |
| 3 | `003-postgres.md` | PostgreSQL implementation with BYTEA | Stage 2 |
| 4 | `004-sqlite.md` | SQLite implementation with BLOB | Stage 2 |
| 5 | `005-projection.md` | Event projection handlers | Stages 2, 3, 4 |
| 6 | `006-commands.md` | Command handlers for media CRUD | Stages 1-5 |
| 7 | `007-thumbnail.md` | Image thumbnail generation | Stage 1 (parallel) |
| 8 | `008-api.md` | OpenAPI spec + HTTP handlers | Stages 1-7 |
| 9 | `009-gedcom.md` | GEDCOM OBJE record import | Stages 1-8 |
| 10 | `010-tests.md` | Test coverage for all components | All stages |

## Key Design Decisions

1. **Binary Storage**: Store file data as `[]byte` in domain, BYTEA/BLOB in DB
   - Simpler deployment (no external storage)
   - 10MB file limit keeps DB reasonable
   - Future: can migrate to object storage if needed

2. **Thumbnail Generation**: Inline with upload, not background job
   - Keeps architecture simple
   - 300x300 max, preserves aspect ratio
   - Non-images get nil thumbnail

3. **Entity Attachment**: Polymorphic via EntityType + EntityID
   - Supports Person, Family, Source attachments
   - Extensible for future entity types

4. **Event Sourcing**: Full CRUD via MediaCreated/Updated/Deleted events
   - Consistent with existing Source/Citation pattern
   - Supports rollback capability

## Execution Instructions

Run stages in order, waiting for dependencies:

```bash
# Stage 1 - Foundation
claude code /run-prompt .prompts/030-media/001-foundation.md

# Stage 2 - Repository Interface
claude code /run-prompt .prompts/030-media/002-repository.md

# Stages 3, 4, 7 can run in parallel
claude code /run-prompt .prompts/030-media/003-postgres.md &
claude code /run-prompt .prompts/030-media/004-sqlite.md &
claude code /run-prompt .prompts/030-media/007-thumbnail.md &
wait

# Continue sequential stages
claude code /run-prompt .prompts/030-media/005-projection.md
claude code /run-prompt .prompts/030-media/006-commands.md
claude code /run-prompt .prompts/030-media/008-api.md
claude code /run-prompt .prompts/030-media/009-gedcom.md
claude code /run-prompt .prompts/030-media/010-tests.md
```

## Validation Checklist

After all stages complete:

- [ ] `go build ./...` succeeds
- [ ] `go test ./...` passes
- [ ] `make check-coverage` meets 85% threshold
- [ ] OpenAPI spec validates
- [ ] Manual test: upload image, verify thumbnail
- [ ] Manual test: attach media to person
- [ ] Manual test: GEDCOM import with OBJE records
