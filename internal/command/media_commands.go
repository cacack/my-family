package command

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/media"
	"github.com/cacack/my-family/internal/repository"
)

// Media command errors.
var (
	ErrMediaNotFound = errors.New("media not found")
)

// Allowed MIME types for upload.
var allowedMimeTypes = map[string]bool{
	"image/jpeg":      true,
	"image/png":       true,
	"image/gif":       true,
	"image/webp":      true,
	"application/pdf": true,
	"image/tiff":      true,
}

// UploadMediaInput contains the data for uploading new media.
type UploadMediaInput struct {
	EntityType  string // "person", "family", "source"
	EntityID    uuid.UUID
	Title       string
	Description string
	MediaType   string // "photo", "document", etc.
	Filename    string
	FileData    []byte
}

// UploadMediaResult contains the result of uploading media.
type UploadMediaResult struct {
	ID      uuid.UUID
	Version int64
}

// UploadMedia creates a new media record with optional thumbnail generation.
func (h *Handler) UploadMedia(ctx context.Context, input UploadMediaInput) (*UploadMediaResult, error) {
	// Validate required fields
	if input.Title == "" {
		return nil, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}
	if input.EntityID == uuid.Nil {
		return nil, fmt.Errorf("%w: entity_id is required", ErrInvalidInput)
	}
	// A stored size of 0 marks a reference-only (GEDCOM-imported) item with no
	// content (#1477); the API derives has_content from it (mediaHasContent in
	// internal/api), so an upload must never be empty.
	if len(input.FileData) == 0 {
		return nil, fmt.Errorf("%w: file data is required", ErrInvalidInput)
	}

	// Detect MIME type
	mimeType := http.DetectContentType(input.FileData)

	// Validate MIME type
	if !allowedMimeTypes[mimeType] {
		return nil, fmt.Errorf("%w: unsupported file type: %s", ErrInvalidInput, mimeType)
	}

	// Create media entity
	m := domain.NewMedia(input.Title, input.EntityType, input.EntityID)
	m.Description = input.Description
	m.MimeType = mimeType
	m.MediaType = domain.MediaType(input.MediaType)
	m.Filename = input.Filename
	m.FileSize = int64(len(input.FileData))
	m.FileData = input.FileData

	// Generate thumbnail for images
	if media.IsImageMimeType(mimeType) {
		opts := media.DefaultThumbnailOptions()
		thumbnail, err := media.GenerateThumbnail(input.FileData, opts)
		if err == nil && len(thumbnail) > 0 {
			m.ThumbnailData = thumbnail
		}
	}

	// Validate media
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	// Create event
	event := domain.NewMediaCreated(m)

	// Execute command (append + project)
	version, err := h.execute(ctx, m.ID.String(), "Media", []domain.Event{event}, -1)
	if err != nil {
		return nil, fmt.Errorf("executing upload media command: %w", err)
	}

	return &UploadMediaResult{
		ID:      m.ID,
		Version: version,
	}, nil
}

// UpdateMediaInput contains the data for updating media metadata.
type UpdateMediaInput struct {
	ID          uuid.UUID
	Title       *string // required: cannot be cleared
	Description Patch[string]
	MediaType   *string // required: cannot be cleared
	CropLeft    Patch[int]
	CropTop     Patch[int]
	CropWidth   Patch[int]
	CropHeight  Patch[int]
	Private     *bool // marks or unmarks the media private
	Version     int64 // Required for optimistic locking
}

// UpdateMediaResult contains the result of updating media.
type UpdateMediaResult struct {
	Version int64
}

// UpdateMedia updates media metadata (not the file itself).
func (h *Handler) UpdateMedia(ctx context.Context, input UpdateMediaInput) (*UpdateMediaResult, error) {
	// Get current media from read model
	current, err := h.readStore.GetMedia(ctx, h.branchID, input.ID)
	if err != nil {
		return nil, fmt.Errorf("getting media: %w", err)
	}
	if current == nil {
		return nil, ErrMediaNotFound
	}

	// Check version for optimistic locking
	if current.Version != input.Version {
		return nil, repository.ErrConcurrencyConflict
	}

	// Build changes map. A field equal to its current value is not a change
	// (the edit form sends every field), so only differing fields are recorded.
	changes := make(map[string]any)
	testMedia := domain.Media{
		ID:         current.ID,
		EntityType: current.EntityType,
		EntityID:   current.EntityID,
		Title:      current.Title,
		MediaType:  current.MediaType,
		FileSize:   current.FileSize,
	}

	if input.Title != nil && *input.Title != current.Title {
		changes["title"] = *input.Title
		testMedia.Title = *input.Title
	}
	if description, ok := input.Description.Change(); ok && description != current.Description {
		changes["description"] = description
	}
	if input.MediaType != nil && string(current.MediaType) != *input.MediaType {
		changes["media_type"] = *input.MediaType
		testMedia.MediaType = domain.MediaType(*input.MediaType)
	}
	addCropChanges(changes, input, current)
	recordPrivate(changes, input.Private, current.Restriction)

	version := current.Version
	if len(changes) > 0 {
		// Validate updated media
		if err := testMedia.Validate(); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}

		// Create event
		event := domain.NewMediaUpdated(input.ID, changes)

		// Execute command
		version, err = h.execute(ctx, input.ID.String(), "Media", []domain.Event{event}, input.Version)
		if err != nil {
			return nil, fmt.Errorf("executing update media command: %w", err)
		}
	}

	// Run even when this item is unchanged, so that resending the mark
	// repairs a group a failed earlier update left out of step.
	if input.Private != nil && current.GroupID != nil {
		if err := h.markMediaGroupPrivate(ctx, *current.GroupID, *input.Private); err != nil {
			return nil, err
		}
	}

	return &UpdateMediaResult{Version: version}, nil
}

// markMediaGroupPrivate marks or unmarks private every item of a media group:
// the copies an import made of one OBJE record linked from several owners,
// which show the same file (#1478). An item already marked so is left as is.
func (h *Handler) markMediaGroupPrivate(ctx context.Context, groupID uuid.UUID, private bool) error {
	items, err := h.readStore.ListMediaInGroup(ctx, h.branchID, groupID)
	if err != nil {
		return fmt.Errorf("listing media group: %w", err)
	}
	for _, item := range items {
		changes := make(map[string]any)
		recordPrivate(changes, &private, item.Restriction)
		if len(changes) == 0 {
			continue
		}
		event := domain.NewMediaUpdated(item.ID, changes)
		if _, err := h.execute(ctx, item.ID.String(), "Media", []domain.Event{event}, item.Version); err != nil {
			return fmt.Errorf("marking media %s of the group: %w", item.ID, err)
		}
	}
	return nil
}

// addCropChanges records each crop field in input that differs from current.
func addCropChanges(changes map[string]any, input UpdateMediaInput, current *repository.MediaReadModel) {
	recordOptionalChange(changes, "crop_left", input.CropLeft, current.CropLeft)
	recordOptionalChange(changes, "crop_top", input.CropTop, current.CropTop)
	recordOptionalChange(changes, "crop_width", input.CropWidth, current.CropWidth)
	recordOptionalChange(changes, "crop_height", input.CropHeight, current.CropHeight)
}

// DeleteMedia deletes a media record.
func (h *Handler) DeleteMedia(ctx context.Context, id uuid.UUID, version int64, reason string) error {
	// Get current media from read model
	current, err := h.readStore.GetMedia(ctx, h.branchID, id)
	if err != nil {
		return fmt.Errorf("getting media: %w", err)
	}
	if current == nil {
		return ErrMediaNotFound
	}

	// Check version for optimistic locking
	if current.Version != version {
		return repository.ErrConcurrencyConflict
	}

	// Create event
	event := domain.NewMediaDeleted(id, reason)

	// Execute command
	_, err = h.execute(ctx, id.String(), "Media", []domain.Event{event}, version)
	if err != nil {
		return fmt.Errorf("executing delete media command: %w", err)
	}
	return nil
}

// RollbackMedia rolls back media to a specific version.
// A rollback that changes the private mark of a group item gives the mark to
// the whole group, as UpdateMedia does (#1478).
func (h *Handler) RollbackMedia(ctx context.Context, mediaID uuid.UUID, targetVersion int64) (*RollbackResult, error) {
	result, err := h.rollbackEntity(ctx, "Media", mediaID, targetVersion, func(id uuid.UUID) (bool, error) {
		m, err := h.readStore.GetMedia(ctx, domain.MainBranchID, id)
		if err != nil {
			return false, err
		}
		return m == nil, nil
	})
	if err != nil {
		return nil, err
	}
	if _, ok := result.Changes["restriction"]; !ok {
		return result, nil
	}
	m, err := h.readStore.GetMedia(ctx, domain.MainBranchID, mediaID)
	if err != nil {
		return nil, fmt.Errorf("getting rolled back media: %w", err)
	}
	if m != nil && m.GroupID != nil {
		if err := h.markMediaGroupPrivate(ctx, *m.GroupID, m.Restriction.IsPrivate()); err != nil {
			return nil, err
		}
	}
	return result, nil
}
