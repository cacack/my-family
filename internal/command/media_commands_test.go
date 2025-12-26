package command_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// createTestJPEG creates a small test JPEG image.
func createTestJPEG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{100, 150, 200, 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
	return buf.Bytes()
}

// TestUploadMedia tests uploading new media.
func TestUploadMedia(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a person to attach media to
	personResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Smith",
		Gender:    "male",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	jpegData := createTestJPEG()

	tests := []struct {
		name    string
		input   command.UploadMediaInput
		wantErr bool
	}{
		{
			name: "valid image upload",
			input: command.UploadMediaInput{
				EntityType:  "person",
				EntityID:    personResult.ID,
				Title:       "Family Portrait",
				Description: "A portrait from 1920",
				MediaType:   "photo",
				Filename:    "portrait.jpg",
				FileData:    jpegData,
			},
			wantErr: false,
		},
		{
			name: "missing title",
			input: command.UploadMediaInput{
				EntityType: "person",
				EntityID:   personResult.ID,
				Title:      "",
				FileData:   jpegData,
			},
			wantErr: true,
		},
		{
			name: "missing entity ID",
			input: command.UploadMediaInput{
				EntityType: "person",
				EntityID:   uuid.Nil,
				Title:      "Test",
				FileData:   jpegData,
			},
			wantErr: true,
		},
		{
			name: "missing file data",
			input: command.UploadMediaInput{
				EntityType: "person",
				EntityID:   personResult.ID,
				Title:      "Test",
				FileData:   []byte{},
			},
			wantErr: true,
		},
		{
			name: "unsupported file type",
			input: command.UploadMediaInput{
				EntityType: "person",
				EntityID:   personResult.ID,
				Title:      "Test",
				FileData:   []byte("not an image"), // Will be detected as text/plain
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler.UploadMedia(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("UploadMedia() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if result.ID == uuid.Nil {
					t.Error("Expected non-nil ID")
				}
				if result.Version != 1 {
					t.Errorf("Version = %d, want 1", result.Version)
				}

				// Verify media in read model (use GetMediaWithData to include binary data)
				media, _ := readStore.GetMediaWithData(ctx, result.ID)
				if media == nil {
					t.Fatal("Media not found in read model")
				}
				if media.Title != tt.input.Title {
					t.Errorf("Title = %s, want %s", media.Title, tt.input.Title)
				}
				if media.EntityID != tt.input.EntityID {
					t.Errorf("EntityID = %v, want %v", media.EntityID, tt.input.EntityID)
				}
				// Should have generated thumbnail for image
				if len(media.ThumbnailData) == 0 {
					t.Error("Expected thumbnail to be generated for image")
				}
			}
		})
	}
}

// TestUpdateMedia tests updating media metadata.
func TestUpdateMedia(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a person to attach media to
	personResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Smith",
		Gender:    "male",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Upload media first
	uploadResult, err := handler.UploadMedia(ctx, command.UploadMediaInput{
		EntityType:  "person",
		EntityID:    personResult.ID,
		Title:       "Original Title",
		Description: "Original Description",
		MediaType:   "photo",
		Filename:    "test.jpg",
		FileData:    createTestJPEG(),
	})
	if err != nil {
		t.Fatalf("UploadMedia failed: %v", err)
	}

	tests := []struct {
		name    string
		input   command.UpdateMediaInput
		wantErr bool
	}{
		{
			name: "update title",
			input: command.UpdateMediaInput{
				ID:      uploadResult.ID,
				Title:   strPtr("Updated Title"),
				Version: uploadResult.Version,
			},
			wantErr: false,
		},
		{
			name: "update multiple fields",
			input: command.UpdateMediaInput{
				ID:          uploadResult.ID,
				Description: strPtr("Updated Description"),
				MediaType:   strPtr("document"),
				Version:     2,
			},
			wantErr: false,
		},
		{
			name: "update crop region",
			input: command.UpdateMediaInput{
				ID:         uploadResult.ID,
				CropLeft:   intPtr(10),
				CropTop:    intPtr(20),
				CropWidth:  intPtr(100),
				CropHeight: intPtr(150),
				Version:    3,
			},
			wantErr: false,
		},
		{
			name: "no changes",
			input: command.UpdateMediaInput{
				ID:      uploadResult.ID,
				Version: 4,
			},
			wantErr: false, // Should succeed with no changes
		},
		{
			name: "wrong version (optimistic locking)",
			input: command.UpdateMediaInput{
				ID:      uploadResult.ID,
				Title:   strPtr("Should Fail"),
				Version: 999,
			},
			wantErr: true,
		},
		{
			name: "non-existent media",
			input: command.UpdateMediaInput{
				ID:      uuid.New(),
				Title:   strPtr("Should Fail"),
				Version: 1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler.UpdateMedia(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateMedia() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.input.Title != nil {
				// Verify update in read model
				media, _ := readStore.GetMedia(ctx, tt.input.ID)
				if media == nil {
					t.Fatal("Media not found in read model")
				}
				if media.Title != *tt.input.Title {
					t.Errorf("Title = %s, want %s", media.Title, *tt.input.Title)
				}
			}

			if !tt.wantErr {
				if result == nil {
					t.Error("Expected non-nil result")
				}
			}
		})
	}
}

// TestDeleteMedia tests deleting media.
func TestDeleteMedia(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a person to attach media to
	personResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Smith",
		Gender:    "male",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Upload media first
	uploadResult, err := handler.UploadMedia(ctx, command.UploadMediaInput{
		EntityType: "person",
		EntityID:   personResult.ID,
		Title:      "To Be Deleted",
		Filename:   "test.jpg",
		FileData:   createTestJPEG(),
	})
	if err != nil {
		t.Fatalf("UploadMedia failed: %v", err)
	}

	// Test deleting non-existent media
	err = handler.DeleteMedia(ctx, uuid.New(), 1, "test")
	if err == nil {
		t.Error("Expected error when deleting non-existent media")
	}

	// Test deleting with wrong version
	err = handler.DeleteMedia(ctx, uploadResult.ID, 999, "test")
	if err == nil {
		t.Error("Expected error when deleting with wrong version")
	}

	// Test successful deletion
	err = handler.DeleteMedia(ctx, uploadResult.ID, uploadResult.Version, "user request")
	if err != nil {
		t.Fatalf("DeleteMedia() error = %v", err)
	}

	// Verify media is deleted
	media, _ := readStore.GetMedia(ctx, uploadResult.ID)
	if media != nil {
		t.Error("Media should be deleted from read model")
	}
}

// TestRollbackMedia tests rolling back media to a previous version.
func TestRollbackMedia(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a person to attach media to
	personResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Smith",
		Gender:    "male",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Upload media
	uploadResult, err := handler.UploadMedia(ctx, command.UploadMediaInput{
		EntityType: "person",
		EntityID:   personResult.ID,
		Title:      "Original Title",
		Filename:   "test.jpg",
		FileData:   createTestJPEG(),
	})
	if err != nil {
		t.Fatalf("UploadMedia failed: %v", err)
	}

	// Update media
	_, err = handler.UpdateMedia(ctx, command.UpdateMediaInput{
		ID:      uploadResult.ID,
		Title:   strPtr("Updated Title"),
		Version: uploadResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateMedia failed: %v", err)
	}

	// Verify title was updated
	media, _ := readStore.GetMedia(ctx, uploadResult.ID)
	if media.Title != "Updated Title" {
		t.Errorf("Title should be 'Updated Title', got %s", media.Title)
	}

	// Rollback to version 1
	result, err := handler.RollbackMedia(ctx, uploadResult.ID, 1)
	if err != nil {
		t.Fatalf("RollbackMedia() error = %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Verify title was rolled back
	media, _ = readStore.GetMedia(ctx, uploadResult.ID)
	if media.Title != "Original Title" {
		t.Errorf("Title should be 'Original Title' after rollback, got %s", media.Title)
	}
}

// Helper function to create *int pointers.
func intPtr(i int) *int {
	return &i
}

// TestUploadMedia_WithPDF tests uploading a PDF document.
func TestUploadMedia_WithPDF(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a person to attach media to
	personResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Smith",
		Gender:    "male",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Create a minimal PDF (magic bytes + enough to be detected)
	pdfData := []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\ntrailer\n<<>>\n%%EOF")

	result, err := handler.UploadMedia(ctx, command.UploadMediaInput{
		EntityType:  "person",
		EntityID:    personResult.ID,
		Title:       "Birth Certificate",
		Description: "Birth certificate scan",
		MediaType:   "document",
		Filename:    "birth_cert.pdf",
		FileData:    pdfData,
	})
	if err != nil {
		t.Fatalf("UploadMedia() error = %v", err)
	}

	if result.ID == uuid.Nil {
		t.Error("Expected non-nil ID")
	}

	// Verify in read model - no thumbnail for PDF
	media, _ := readStore.GetMedia(ctx, result.ID)
	if media == nil {
		t.Fatal("Media not found in read model")
	}
	if len(media.ThumbnailData) > 0 {
		t.Error("PDFs should not have thumbnails generated")
	}
}

// TestUploadMedia_WithInvalidEntityType tests upload with invalid entity type.
func TestUploadMedia_WithInvalidEntityType(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	_, err := handler.UploadMedia(ctx, command.UploadMediaInput{
		EntityType: "invalid",
		EntityID:   uuid.New(),
		Title:      "Test",
		FileData:   createTestJPEG(),
	})
	if err == nil {
		t.Error("Expected error for invalid entity type")
	}
}

// TestMediaVersionConflict tests optimistic locking behavior.
func TestMediaVersionConflict(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a person to attach media to
	personResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Smith",
		Gender:    "male",
	})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// Upload media
	uploadResult, err := handler.UploadMedia(ctx, command.UploadMediaInput{
		EntityType: "person",
		EntityID:   personResult.ID,
		Title:      "Test",
		FileData:   createTestJPEG(),
	})
	if err != nil {
		t.Fatalf("UploadMedia failed: %v", err)
	}

	// Update media
	_, err = handler.UpdateMedia(ctx, command.UpdateMediaInput{
		ID:      uploadResult.ID,
		Title:   strPtr("Updated"),
		Version: uploadResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateMedia failed: %v", err)
	}

	// Try to update with old version
	_, err = handler.UpdateMedia(ctx, command.UpdateMediaInput{
		ID:      uploadResult.ID,
		Title:   strPtr("Should Fail"),
		Version: uploadResult.Version, // Old version
	})
	if err == nil {
		t.Error("Expected version conflict error")
	}
	if err != repository.ErrConcurrencyConflict {
		t.Errorf("Expected ErrConcurrencyConflict, got %v", err)
	}
}
