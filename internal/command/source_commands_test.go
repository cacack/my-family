package command_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// TestCreateSource tests creating a new source.
func TestCreateSource(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	tests := []struct {
		name    string
		input   command.CreateSourceInput
		wantErr bool
	}{
		{
			name: "valid book source",
			input: command.CreateSourceInput{
				SourceType:  "book",
				Title:       "The History of Springfield",
				Author:      "John Smith",
				Publisher:   "Historical Press",
				PublishDate: "1995",
			},
			wantErr: false,
		},
		{
			name: "valid archive source",
			input: command.CreateSourceInput{
				SourceType:     "archive",
				Title:          "County Records Collection",
				RepositoryName: "State Archive",
				CollectionName: "Birth Records",
				CallNumber:     "BR-1850-1900",
			},
			wantErr: false,
		},
		{
			name: "missing title",
			input: command.CreateSourceInput{
				SourceType: "book",
				Title:      "",
			},
			wantErr: true,
		},
		{
			name: "invalid source type",
			input: command.CreateSourceInput{
				SourceType: "invalid_type",
				Title:      "Test Source",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler.CreateSource(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateSource() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if result.ID == uuid.Nil {
					t.Error("Expected non-nil ID")
				}
				if result.Version != 1 {
					t.Errorf("Version = %d, want 1", result.Version)
				}

				// Verify source in read model
				source, _ := readStore.GetSource(ctx, result.ID)
				if source == nil {
					t.Fatal("Source not found in read model")
				}
				if source.Title != tt.input.Title {
					t.Errorf("Title = %s, want %s", source.Title, tt.input.Title)
				}
			}
		})
	}
}

// TestUpdateSource tests updating a source.
func TestUpdateSource(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create source first
	createResult, err := handler.CreateSource(ctx, command.CreateSourceInput{
		SourceType: "book",
		Title:      "Original Title",
		Author:     "Original Author",
	})
	if err != nil {
		t.Fatalf("CreateSource failed: %v", err)
	}

	tests := []struct {
		name    string
		input   command.UpdateSourceInput
		wantErr bool
	}{
		{
			name: "update title",
			input: command.UpdateSourceInput{
				ID:      createResult.ID,
				Title:   strPtr("Updated Title"),
				Version: createResult.Version,
			},
			wantErr: false,
		},
		{
			name: "update multiple fields",
			input: command.UpdateSourceInput{
				ID:        createResult.ID,
				Author:    strPtr("New Author"),
				Publisher: strPtr("New Publisher"),
				Version:   2, // version from previous update
			},
			wantErr: false,
		},
		{
			name: "wrong version (optimistic locking)",
			input: command.UpdateSourceInput{
				ID:      createResult.ID,
				Title:   strPtr("Should Fail"),
				Version: 999,
			},
			wantErr: true,
		},
		{
			name: "invalid source type",
			input: command.UpdateSourceInput{
				ID:         createResult.ID,
				SourceType: strPtr("invalid_type"),
				Version:    3,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler.UpdateSource(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateSource() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && result.Version <= tt.input.Version {
				t.Errorf("Version not incremented: got %d, want > %d", result.Version, tt.input.Version)
			}
		})
	}
}

// TestUpdateSource_NoChanges tests that updating without changes returns current version.
func TestUpdateSource_NoChanges(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create source
	createResult, _ := handler.CreateSource(ctx, command.CreateSourceInput{
		SourceType: "book",
		Title:      "Test Source",
	})

	// Update with no changes
	result, err := handler.UpdateSource(ctx, command.UpdateSourceInput{
		ID:      createResult.ID,
		Version: createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateSource failed: %v", err)
	}

	if result.Version != createResult.Version {
		t.Errorf("Version changed without updates: got %d, want %d", result.Version, createResult.Version)
	}
}

// TestDeleteSource tests deleting a source.
func TestDeleteSource(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create source
	createResult, _ := handler.CreateSource(ctx, command.CreateSourceInput{
		SourceType: "book",
		Title:      "Test Source",
	})

	// Delete source
	err := handler.DeleteSource(ctx, createResult.ID, createResult.Version, "Test deletion")
	if err != nil {
		t.Errorf("DeleteSource failed: %v", err)
	}

	// Verify deleted from read model
	source, _ := readStore.GetSource(ctx, createResult.ID)
	if source != nil {
		t.Error("Source should be deleted from read model")
	}
}

// TestDeleteSource_WithCitations tests that deleting a source with citations fails.
func TestDeleteSource_WithCitations(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create source
	sourceResult, _ := handler.CreateSource(ctx, command.CreateSourceInput{
		SourceType: "book",
		Title:      "Test Source",
	})

	// Create person for citation
	personResult, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	// Create citation
	_, _ = handler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    sourceResult.ID,
		FactType:    "person_birth",
		FactOwnerID: personResult.ID,
	})

	// Try to delete source
	err := handler.DeleteSource(ctx, sourceResult.ID, sourceResult.Version, "Should fail")
	if err != command.ErrSourceHasCitations {
		t.Errorf("DeleteSource should fail with ErrSourceHasCitations, got: %v", err)
	}
}

// TestDeleteSource_WrongVersion tests optimistic locking on delete.
func TestDeleteSource_WrongVersion(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create source
	createResult, _ := handler.CreateSource(ctx, command.CreateSourceInput{
		SourceType: "book",
		Title:      "Test Source",
	})

	// Try to delete with wrong version
	err := handler.DeleteSource(ctx, createResult.ID, 999, "Should fail")
	if err != repository.ErrConcurrencyConflict {
		t.Errorf("DeleteSource should fail with ErrConcurrencyConflict, got: %v", err)
	}
}

// TestCreateCitation tests creating a new citation.
func TestCreateCitation(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create source and person for citations
	sourceResult, _ := handler.CreateSource(ctx, command.CreateSourceInput{
		SourceType: "book",
		Title:      "Test Source",
	})

	personResult, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	tests := []struct {
		name    string
		input   command.CreateCitationInput
		wantErr bool
	}{
		{
			name: "valid citation",
			input: command.CreateCitationInput{
				SourceID:      sourceResult.ID,
				FactType:      "person_birth",
				FactOwnerID:   personResult.ID,
				Page:          "123",
				SourceQuality: "original",
				InformantType: "primary",
				EvidenceType:  "direct",
			},
			wantErr: false,
		},
		{
			name: "minimal citation",
			input: command.CreateCitationInput{
				SourceID:    sourceResult.ID,
				FactType:    "person_death",
				FactOwnerID: personResult.ID,
			},
			wantErr: false,
		},
		{
			name: "missing source_id",
			input: command.CreateCitationInput{
				SourceID:    uuid.Nil,
				FactType:    "person_birth",
				FactOwnerID: personResult.ID,
			},
			wantErr: true,
		},
		{
			name: "missing fact_owner_id",
			input: command.CreateCitationInput{
				SourceID:    sourceResult.ID,
				FactType:    "person_birth",
				FactOwnerID: uuid.Nil,
			},
			wantErr: true,
		},
		{
			name: "non-existent source",
			input: command.CreateCitationInput{
				SourceID:    uuid.New(),
				FactType:    "person_birth",
				FactOwnerID: personResult.ID,
			},
			wantErr: true,
		},
		{
			name: "invalid fact type",
			input: command.CreateCitationInput{
				SourceID:    sourceResult.ID,
				FactType:    "invalid_type",
				FactOwnerID: personResult.ID,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler.CreateCitation(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateCitation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if result.ID == uuid.Nil {
					t.Error("Expected non-nil ID")
				}
				if result.Version != 1 {
					t.Errorf("Version = %d, want 1", result.Version)
				}

				// Verify citation in read model
				citation, _ := readStore.GetCitation(ctx, result.ID)
				if citation == nil {
					t.Fatal("Citation not found in read model")
				}
				if citation.SourceID != tt.input.SourceID {
					t.Errorf("SourceID = %v, want %v", citation.SourceID, tt.input.SourceID)
				}
			}
		})
	}
}

// TestUpdateCitation tests updating a citation.
func TestUpdateCitation(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create source and person
	sourceResult, _ := handler.CreateSource(ctx, command.CreateSourceInput{
		SourceType: "book",
		Title:      "Test Source",
	})

	personResult, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	// Create citation
	createResult, _ := handler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    sourceResult.ID,
		FactType:    "person_birth",
		FactOwnerID: personResult.ID,
		Page:        "100",
	})

	tests := []struct {
		name    string
		input   command.UpdateCitationInput
		wantErr bool
	}{
		{
			name: "update page",
			input: command.UpdateCitationInput{
				ID:      createResult.ID,
				Page:    strPtr("200"),
				Version: createResult.Version,
			},
			wantErr: false,
		},
		{
			name: "update GPS fields",
			input: command.UpdateCitationInput{
				ID:            createResult.ID,
				SourceQuality: strPtr("original"),
				InformantType: strPtr("primary"),
				EvidenceType:  strPtr("direct"),
				Version:       2,
			},
			wantErr: false,
		},
		{
			name: "wrong version",
			input: command.UpdateCitationInput{
				ID:      createResult.ID,
				Page:    strPtr("300"),
				Version: 999,
			},
			wantErr: true,
		},
		{
			name: "invalid evidence type",
			input: command.UpdateCitationInput{
				ID:           createResult.ID,
				EvidenceType: strPtr("invalid"),
				Version:      3,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler.UpdateCitation(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateCitation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && result.Version <= tt.input.Version {
				t.Errorf("Version not incremented: got %d, want > %d", result.Version, tt.input.Version)
			}
		})
	}
}

// TestDeleteCitation tests deleting a citation.
func TestDeleteCitation(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create source and person
	sourceResult, _ := handler.CreateSource(ctx, command.CreateSourceInput{
		SourceType: "book",
		Title:      "Test Source",
	})

	personResult, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	// Create citation
	createResult, _ := handler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    sourceResult.ID,
		FactType:    "person_birth",
		FactOwnerID: personResult.ID,
	})

	// Delete citation
	err := handler.DeleteCitation(ctx, createResult.ID, createResult.Version, "Test deletion")
	if err != nil {
		t.Errorf("DeleteCitation failed: %v", err)
	}

	// Verify deleted from read model
	citation, _ := readStore.GetCitation(ctx, createResult.ID)
	if citation != nil {
		t.Error("Citation should be deleted from read model")
	}
}

// TestDeleteCitation_NotFound tests deleting a non-existent citation.
func TestDeleteCitation_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Try to delete non-existent citation
	err := handler.DeleteCitation(ctx, uuid.New(), 1, "Should fail")
	if err != command.ErrCitationNotFound {
		t.Errorf("DeleteCitation should fail with ErrCitationNotFound, got: %v", err)
	}
}

// TestDeleteCitation_WrongVersion tests optimistic locking on citation delete.
func TestDeleteCitation_WrongVersion(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create source and person
	sourceResult, _ := handler.CreateSource(ctx, command.CreateSourceInput{
		SourceType: "book",
		Title:      "Test Source",
	})

	personResult, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	// Create citation
	createResult, _ := handler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    sourceResult.ID,
		FactType:    "person_birth",
		FactOwnerID: personResult.ID,
	})

	// Try to delete with wrong version
	err := handler.DeleteCitation(ctx, createResult.ID, 999, "Should fail")
	if err != repository.ErrConcurrencyConflict {
		t.Errorf("DeleteCitation should fail with ErrConcurrencyConflict, got: %v", err)
	}
}

// TestUpdateSource_AllFields tests updating all source fields.
func TestUpdateSource_AllFields(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create source
	createResult, _ := handler.CreateSource(ctx, command.CreateSourceInput{
		SourceType: "book",
		Title:      "Original Title",
	})

	// Update all fields
	result, err := handler.UpdateSource(ctx, command.UpdateSourceInput{
		ID:             createResult.ID,
		SourceType:     strPtr("archive"),
		Title:          strPtr("Updated Title"),
		Author:         strPtr("New Author"),
		Publisher:      strPtr("New Publisher"),
		PublishDate:    strPtr("2024"),
		URL:            strPtr("http://example.com"),
		RepositoryName: strPtr("National Archives"),
		CollectionName: strPtr("Census Records"),
		CallNumber:     strPtr("ABC-123"),
		Notes:          strPtr("Updated notes"),
		Version:        createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateSource failed: %v", err)
	}

	if result.Version <= createResult.Version {
		t.Errorf("Version not incremented: got %d, want > %d", result.Version, createResult.Version)
	}

	// Verify changes in read model
	source, _ := readStore.GetSource(ctx, createResult.ID)
	if source.Title != "Updated Title" {
		t.Errorf("Title = %s, want Updated Title", source.Title)
	}
	if source.Author != "New Author" {
		t.Errorf("Author = %s, want New Author", source.Author)
	}
	if source.SourceType != "archive" {
		t.Errorf("SourceType = %s, want archive", source.SourceType)
	}
}

// TestUpdateSource_NotFound tests updating a non-existent source.
func TestUpdateSource_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Try to update non-existent source
	_, err := handler.UpdateSource(ctx, command.UpdateSourceInput{
		ID:      uuid.New(),
		Title:   strPtr("Should Fail"),
		Version: 1,
	})
	if err != command.ErrSourceNotFound {
		t.Errorf("UpdateSource should fail with ErrSourceNotFound, got: %v", err)
	}
}

// TestUpdateCitation_SourceIDChange tests changing the source of a citation.
func TestUpdateCitation_SourceIDChange(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create two sources
	source1, _ := handler.CreateSource(ctx, command.CreateSourceInput{
		SourceType: "book",
		Title:      "Source 1",
	})
	source2, _ := handler.CreateSource(ctx, command.CreateSourceInput{
		SourceType: "book",
		Title:      "Source 2",
	})

	personResult, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	// Create citation with source1
	createResult, _ := handler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    source1.ID,
		FactType:    "person_birth",
		FactOwnerID: personResult.ID,
	})

	// Update to source2
	result, err := handler.UpdateCitation(ctx, command.UpdateCitationInput{
		ID:       createResult.ID,
		SourceID: &source2.ID,
		Version:  createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateCitation failed: %v", err)
	}

	if result.Version <= createResult.Version {
		t.Errorf("Version not incremented")
	}

	// Verify change in read model
	citation, _ := readStore.GetCitation(ctx, createResult.ID)
	if citation.SourceID != source2.ID {
		t.Errorf("SourceID = %v, want %v", citation.SourceID, source2.ID)
	}
}

// TestUpdateCitation_InvalidSourceID tests changing to a non-existent source.
func TestUpdateCitation_InvalidSourceID(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create source and person
	sourceResult, _ := handler.CreateSource(ctx, command.CreateSourceInput{
		SourceType: "book",
		Title:      "Test Source",
	})

	personResult, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	// Create citation
	createResult, _ := handler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    sourceResult.ID,
		FactType:    "person_birth",
		FactOwnerID: personResult.ID,
	})

	// Try to update to non-existent source
	nonExistentSourceID := uuid.New()
	_, err := handler.UpdateCitation(ctx, command.UpdateCitationInput{
		ID:       createResult.ID,
		SourceID: &nonExistentSourceID,
		Version:  createResult.Version,
	})
	if err == nil {
		t.Error("UpdateCitation should fail when changing to non-existent source")
	}
}

// TestUpdateCitation_NotFound tests updating a non-existent citation.
func TestUpdateCitation_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Try to update non-existent citation
	_, err := handler.UpdateCitation(ctx, command.UpdateCitationInput{
		ID:      uuid.New(),
		Page:    strPtr("Should Fail"),
		Version: 1,
	})
	if err != command.ErrCitationNotFound {
		t.Errorf("UpdateCitation should fail with ErrCitationNotFound, got: %v", err)
	}
}

// TestUpdateCitation_NoChanges tests updating citation with no changes.
func TestUpdateCitation_NoChanges(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create source and person
	sourceResult, _ := handler.CreateSource(ctx, command.CreateSourceInput{
		SourceType: "book",
		Title:      "Test Source",
	})

	personResult, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	// Create citation
	createResult, _ := handler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    sourceResult.ID,
		FactType:    "person_birth",
		FactOwnerID: personResult.ID,
	})

	// Update with no changes
	result, err := handler.UpdateCitation(ctx, command.UpdateCitationInput{
		ID:      createResult.ID,
		Version: createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateCitation failed: %v", err)
	}

	if result.Version != createResult.Version {
		t.Errorf("Version changed without updates: got %d, want %d", result.Version, createResult.Version)
	}
}

// TestUpdateCitation_AllFields tests updating all citation fields.
func TestUpdateCitation_AllFields(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create source and person
	sourceResult, _ := handler.CreateSource(ctx, command.CreateSourceInput{
		SourceType: "book",
		Title:      "Test Source",
	})

	personResult, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	// Create citation
	createResult, _ := handler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    sourceResult.ID,
		FactType:    "person_birth",
		FactOwnerID: personResult.ID,
	})

	// Update all fields
	result, err := handler.UpdateCitation(ctx, command.UpdateCitationInput{
		ID:            createResult.ID,
		Page:          strPtr("123"),
		Volume:        strPtr("Vol 1"),
		SourceQuality: strPtr("original"),
		InformantType: strPtr("primary"),
		EvidenceType:  strPtr("direct"),
		QuotedText:    strPtr("John Doe was born..."),
		Analysis:      strPtr("This is a strong source"),
		TemplateID:    strPtr("template-123"),
		Version:       createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateCitation failed: %v", err)
	}

	if result.Version <= createResult.Version {
		t.Errorf("Version not incremented")
	}

	// Verify changes in read model
	citation, _ := readStore.GetCitation(ctx, createResult.ID)
	if citation.Page != "123" {
		t.Errorf("Page = %s, want 123", citation.Page)
	}
	if citation.Volume != "Vol 1" {
		t.Errorf("Volume = %s, want Vol 1", citation.Volume)
	}
}

// TestUpdateCitation_InvalidInformantType tests updating with invalid informant type.
func TestUpdateCitation_InvalidInformantType(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create source and person
	sourceResult, _ := handler.CreateSource(ctx, command.CreateSourceInput{
		SourceType: "book",
		Title:      "Test Source",
	})

	personResult, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	// Create citation
	createResult, _ := handler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    sourceResult.ID,
		FactType:    "person_birth",
		FactOwnerID: personResult.ID,
	})

	// Try to update with invalid informant type
	_, err := handler.UpdateCitation(ctx, command.UpdateCitationInput{
		ID:            createResult.ID,
		InformantType: strPtr("invalid_type"),
		Version:       createResult.Version,
	})
	if err == nil {
		t.Error("UpdateCitation should fail with invalid informant type")
	}
}

// TestUpdateCitation_InvalidSourceQuality tests updating with invalid source quality.
func TestUpdateCitation_InvalidSourceQuality(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create source and person
	sourceResult, _ := handler.CreateSource(ctx, command.CreateSourceInput{
		SourceType: "book",
		Title:      "Test Source",
	})

	personResult, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	// Create citation
	createResult, _ := handler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    sourceResult.ID,
		FactType:    "person_birth",
		FactOwnerID: personResult.ID,
	})

	// Try to update with invalid source quality
	_, err := handler.UpdateCitation(ctx, command.UpdateCitationInput{
		ID:            createResult.ID,
		SourceQuality: strPtr("invalid_quality"),
		Version:       createResult.Version,
	})
	if err == nil {
		t.Error("UpdateCitation should fail with invalid source quality")
	}
}

// Helper function for string pointers
func strPtr(s string) *string {
	return &s
}
