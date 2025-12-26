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
		return nil, err
	}

	return &UploadMediaResult{
		ID:      m.ID,
		Version: version,
	}, nil
}

// UpdateMediaInput contains the data for updating media metadata.
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

// UpdateMedia updates media metadata (not the file itself).
func (h *Handler) UpdateMedia(ctx context.Context, input UpdateMediaInput) (*UpdateMediaResult, error) {
	// Get current media from read model
	current, err := h.readStore.GetMedia(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrMediaNotFound
	}

	// Check version for optimistic locking
	if current.Version != input.Version {
		return nil, repository.ErrConcurrencyConflict
	}

	// Build changes map
	changes := make(map[string]any)

	if input.Title != nil && *input.Title != current.Title {
		changes["title"] = *input.Title
	}
	if input.Description != nil && *input.Description != current.Description {
		changes["description"] = *input.Description
	}
	if input.MediaType != nil && string(current.MediaType) != *input.MediaType {
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

	// No changes?
	if len(changes) == 0 {
		return &UpdateMediaResult{Version: current.Version}, nil
	}

	// Create event
	event := domain.NewMediaUpdated(input.ID, changes)

	// Execute command
	version, err := h.execute(ctx, input.ID.String(), "Media", []domain.Event{event}, input.Version)
	if err != nil {
		return nil, err
	}

	return &UpdateMediaResult{Version: version}, nil
}

// DeleteMedia deletes a media record.
func (h *Handler) DeleteMedia(ctx context.Context, id uuid.UUID, version int64, reason string) error {
	// Get current media from read model
	current, err := h.readStore.GetMedia(ctx, id)
	if err != nil {
		return err
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
	return err
}

// RollbackMedia rolls back media to a specific version.
func (h *Handler) RollbackMedia(ctx context.Context, mediaID uuid.UUID, targetVersion int64) (*RollbackResult, error) {
	return h.rollbackEntity(ctx, "Media", mediaID, targetVersion, func(id uuid.UUID) (bool, error) {
		m, err := h.readStore.GetMedia(ctx, id)
		if err != nil {
			return false, err
		}
		return m == nil, nil
	})
}
