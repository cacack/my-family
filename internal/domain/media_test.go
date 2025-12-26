package domain

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNewMedia(t *testing.T) {
	entityID := uuid.New()
	m := NewMedia("Test Photo", "person", entityID)

	if m.ID == uuid.Nil {
		t.Error("Expected non-nil UUID")
	}
	if m.Title != "Test Photo" {
		t.Errorf("Title = %v, want Test Photo", m.Title)
	}
	if m.EntityType != "person" {
		t.Errorf("EntityType = %v, want person", m.EntityType)
	}
	if m.EntityID != entityID {
		t.Errorf("EntityID = %v, want %v", m.EntityID, entityID)
	}
	if m.Version != 1 {
		t.Errorf("Version = %v, want 1", m.Version)
	}
}

func TestMedia_Validate(t *testing.T) {
	entityID := uuid.New()

	tests := []struct {
		name    string
		media   *Media
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid media",
			media:   NewMedia("Test Photo", "person", entityID),
			wantErr: false,
		},
		{
			name:    "valid media with family entity type",
			media:   NewMedia("Family Photo", "family", entityID),
			wantErr: false,
		},
		{
			name:    "valid media with source entity type",
			media:   NewMedia("Document", "source", entityID),
			wantErr: false,
		},
		{
			name:    "empty title",
			media:   &Media{ID: uuid.New(), EntityType: "person", EntityID: entityID},
			wantErr: true,
			errMsg:  "title",
		},
		{
			name: "title too long",
			media: &Media{
				ID:         uuid.New(),
				Title:      strings.Repeat("a", 501),
				EntityType: "person",
				EntityID:   entityID,
			},
			wantErr: true,
			errMsg:  "title",
		},
		{
			name: "invalid entity type",
			media: &Media{
				ID:         uuid.New(),
				Title:      "Test",
				EntityType: "invalid",
				EntityID:   entityID,
			},
			wantErr: true,
			errMsg:  "entity_type",
		},
		{
			name: "empty entity ID",
			media: &Media{
				ID:         uuid.New(),
				Title:      "Test",
				EntityType: "person",
				EntityID:   uuid.Nil,
			},
			wantErr: true,
			errMsg:  "entity_id",
		},
		{
			name: "invalid media type",
			media: &Media{
				ID:         uuid.New(),
				Title:      "Test",
				EntityType: "person",
				EntityID:   entityID,
				MediaType:  MediaType("invalid"),
			},
			wantErr: true,
			errMsg:  "media_type",
		},
		{
			name: "file data without file size",
			media: &Media{
				ID:         uuid.New(),
				Title:      "Test",
				EntityType: "person",
				EntityID:   entityID,
				FileData:   []byte{1, 2, 3},
				MimeType:   "image/jpeg",
			},
			wantErr: true,
			errMsg:  "file_size",
		},
		{
			name: "file data without mime type",
			media: &Media{
				ID:         uuid.New(),
				Title:      "Test",
				EntityType: "person",
				EntityID:   entityID,
				FileData:   []byte{1, 2, 3},
				FileSize:   3,
			},
			wantErr: true,
			errMsg:  "mime_type",
		},
		{
			name: "file size too large",
			media: &Media{
				ID:         uuid.New(),
				Title:      "Test",
				EntityType: "person",
				EntityID:   entityID,
				FileSize:   MaxMediaFileSize + 1,
			},
			wantErr: true,
			errMsg:  "file_size",
		},
		{
			name: "valid with all fields",
			media: func() *Media {
				m := NewMedia("Family Portrait", "person", entityID)
				m.Description = "A family portrait from 1920"
				m.MimeType = "image/jpeg"
				m.MediaType = MediaPhoto
				m.Filename = "portrait.jpg"
				m.FileSize = 1024
				m.FileData = make([]byte, 1024)
				m.GedcomXref = "@M1@"
				return m
			}(),
			wantErr: false,
		},
		{
			name: "valid with crop region",
			media: func() *Media {
				m := NewMedia("Cropped Photo", "person", entityID)
				m.MimeType = "image/jpeg"
				m.FileSize = 100
				m.FileData = make([]byte, 100)
				cropLeft := 10
				cropTop := 20
				cropWidth := 100
				cropHeight := 150
				m.CropLeft = &cropLeft
				m.CropTop = &cropTop
				m.CropWidth = &cropWidth
				m.CropHeight = &cropHeight
				return m
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.media.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("Validate() error = %v, want error containing %q", err, tt.errMsg)
			}
		})
	}
}

func TestMediaValidationError_Error(t *testing.T) {
	err := MediaValidationError{Field: "title", Message: "cannot be empty"}
	want := "title: cannot be empty"
	if err.Error() != want {
		t.Errorf("Error() = %v, want %v", err.Error(), want)
	}
}

func TestIsValidEntityType(t *testing.T) {
	tests := []struct {
		entityType string
		want       bool
	}{
		{"person", true},
		{"family", true},
		{"source", true},
		{"invalid", false},
		{"", false},
		{"Person", false}, // Case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.entityType, func(t *testing.T) {
			got := isValidEntityType(tt.entityType)
			if got != tt.want {
				t.Errorf("isValidEntityType(%q) = %v, want %v", tt.entityType, got, tt.want)
			}
		})
	}
}

func TestMediaTypeValidation(t *testing.T) {
	entityID := uuid.New()

	// Test all valid media types
	validTypes := []MediaType{MediaPhoto, MediaDocument, MediaAudio, MediaVideo, MediaCertificate}
	for _, mt := range validTypes {
		m := NewMedia("Test", "person", entityID)
		m.MediaType = mt
		if err := m.Validate(); err != nil {
			t.Errorf("MediaType %q should be valid, got error: %v", mt, err)
		}
	}

	// Test empty media type (should be valid as default)
	m := NewMedia("Test", "person", entityID)
	m.MediaType = ""
	if err := m.Validate(); err != nil {
		t.Errorf("Empty MediaType should be valid, got error: %v", err)
	}
}
