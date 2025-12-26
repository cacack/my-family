package domain

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// MaxMediaFileSize is the maximum allowed file size (10MB).
const MaxMediaFileSize = 10 * 1024 * 1024

// ValidEntityTypes for media attachment.
var ValidEntityTypes = []string{"person", "family", "source"}

// Media represents a media file attached to an entity.
type Media struct {
	ID            uuid.UUID `json:"id"`
	EntityType    string    `json:"entity_type"` // "person", "family", "source"
	EntityID      uuid.UUID `json:"entity_id"`   // ID of attached entity
	Title         string    `json:"title"`       // Display title (required)
	Description   string    `json:"description,omitempty"`
	MimeType      string    `json:"mime_type,omitempty"`
	MediaType     MediaType `json:"media_type,omitempty"`
	Filename      string    `json:"filename,omitempty"`
	FileSize      int64     `json:"file_size,omitempty"`
	FileData      []byte    `json:"file_data,omitempty"`
	ThumbnailData []byte    `json:"thumbnail_data,omitempty"`
	CropLeft      *int      `json:"crop_left,omitempty"`
	CropTop       *int      `json:"crop_top,omitempty"`
	CropWidth     *int      `json:"crop_width,omitempty"`
	CropHeight    *int      `json:"crop_height,omitempty"`
	GedcomXref    string    `json:"gedcom_xref,omitempty"` // Original GEDCOM @XREF@ for round-trip
	Version       int64     `json:"version"`               // Optimistic locking version
}

// MediaValidationError represents a validation error for a Media.
type MediaValidationError struct {
	Field   string
	Message string
}

func (e MediaValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// NewMedia creates a new Media with the given required fields.
func NewMedia(title string, entityType string, entityID uuid.UUID) *Media {
	return &Media{
		ID:         uuid.New(),
		Title:      title,
		EntityType: entityType,
		EntityID:   entityID,
		Version:    1,
	}
}

// Validate checks if the media has valid data.
func (m *Media) Validate() error {
	var errs []error

	// Title required and max 500 chars
	if m.Title == "" {
		errs = append(errs, MediaValidationError{Field: "title", Message: "cannot be empty"})
	} else if len(m.Title) > 500 {
		errs = append(errs, MediaValidationError{Field: "title", Message: "cannot exceed 500 characters"})
	}

	// EntityType must be valid
	if !isValidEntityType(m.EntityType) {
		errs = append(errs, MediaValidationError{Field: "entity_type", Message: fmt.Sprintf("must be one of: person, family, source; got: %s", m.EntityType)})
	}

	// EntityID required
	if m.EntityID == uuid.Nil {
		errs = append(errs, MediaValidationError{Field: "entity_id", Message: "cannot be empty"})
	}

	// MediaType validation
	if !m.MediaType.IsValid() {
		errs = append(errs, MediaValidationError{Field: "media_type", Message: fmt.Sprintf("invalid value: %s", m.MediaType)})
	}

	// If FileData present, require FileSize and MimeType
	if len(m.FileData) > 0 {
		if m.FileSize <= 0 {
			errs = append(errs, MediaValidationError{Field: "file_size", Message: "must be greater than 0 when file data is present"})
		}
		if m.MimeType == "" {
			errs = append(errs, MediaValidationError{Field: "mime_type", Message: "required when file data is present"})
		}
	}

	// FileSize max 10MB
	if m.FileSize > MaxMediaFileSize {
		errs = append(errs, MediaValidationError{Field: "file_size", Message: fmt.Sprintf("cannot exceed %d bytes (10MB)", MaxMediaFileSize)})
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// isValidEntityType checks if the entity type is valid.
func isValidEntityType(entityType string) bool {
	for _, valid := range ValidEntityTypes {
		if entityType == valid {
			return true
		}
	}
	return false
}
