package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Common source/citation command errors.
var (
	ErrSourceNotFound     = errors.New("source not found")
	ErrSourceHasCitations = errors.New("source has citations and cannot be deleted")
	ErrCitationNotFound   = errors.New("citation not found")
)

// CreateSourceInput contains the data for creating a new source.
type CreateSourceInput struct {
	SourceType     string
	Title          string
	Author         string
	Publisher      string
	PublishDate    string
	URL            string
	RepositoryName string
	CollectionName string
	CallNumber     string
	Notes          string
}

// CreateSourceResult contains the result of creating a source.
type CreateSourceResult struct {
	ID      uuid.UUID
	Version int64
}

// CreateSource creates a new source record.
func (h *Handler) CreateSource(ctx context.Context, input CreateSourceInput) (*CreateSourceResult, error) {
	// Validate required fields
	if input.Title == "" {
		return nil, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}

	// Create source entity
	source := domain.NewSource(input.Title, domain.SourceType(input.SourceType))

	if input.Author != "" {
		source.Author = input.Author
	}
	if input.Publisher != "" {
		source.Publisher = input.Publisher
	}
	if input.PublishDate != "" {
		pd := domain.ParseGenDate(input.PublishDate)
		source.PublishDate = &pd
	}
	if input.URL != "" {
		source.URL = input.URL
	}
	if input.RepositoryName != "" {
		source.RepositoryName = input.RepositoryName
	}
	if input.CollectionName != "" {
		source.CollectionName = input.CollectionName
	}
	if input.CallNumber != "" {
		source.CallNumber = input.CallNumber
	}
	if input.Notes != "" {
		source.Notes = input.Notes
	}

	// Validate source
	if err := source.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	// Create event
	event := domain.NewSourceCreated(source)

	// Execute command (append + project)
	version, err := h.execute(ctx, source.ID.String(), "Source", []domain.Event{event}, -1)
	if err != nil {
		return nil, err
	}

	return &CreateSourceResult{
		ID:      source.ID,
		Version: version,
	}, nil
}

// UpdateSourceInput contains the data for updating a source.
type UpdateSourceInput struct {
	ID             uuid.UUID
	SourceType     *string
	Title          *string
	Author         *string
	Publisher      *string
	PublishDate    *string
	URL            *string
	RepositoryName *string
	CollectionName *string
	CallNumber     *string
	Notes          *string
	Version        int64 // Required for optimistic locking
}

// UpdateSourceResult contains the result of updating a source.
type UpdateSourceResult struct {
	Version int64
}

// UpdateSource updates an existing source record.
func (h *Handler) UpdateSource(ctx context.Context, input UpdateSourceInput) (*UpdateSourceResult, error) {
	// Get current source from read model
	current, err := h.readStore.GetSource(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrSourceNotFound
	}

	// Check version for optimistic locking
	if current.Version != input.Version {
		return nil, repository.ErrConcurrencyConflict
	}

	// Build changes map
	changes := make(map[string]any)

	// Apply and validate changes
	testSource := &domain.Source{
		ID:             current.ID,
		SourceType:     current.SourceType,
		Title:          current.Title,
		Author:         current.Author,
		Publisher:      current.Publisher,
		URL:            current.URL,
		RepositoryName: current.RepositoryName,
		CollectionName: current.CollectionName,
		CallNumber:     current.CallNumber,
		Notes:          current.Notes,
	}

	if current.PublishDateRaw != "" {
		pd := domain.ParseGenDate(current.PublishDateRaw)
		testSource.PublishDate = &pd
	}

	if input.SourceType != nil {
		testSource.SourceType = domain.SourceType(*input.SourceType)
		changes["source_type"] = *input.SourceType
	}
	if input.Title != nil {
		testSource.Title = *input.Title
		changes["title"] = *input.Title
	}
	if input.Author != nil {
		testSource.Author = *input.Author
		changes["author"] = *input.Author
	}
	if input.Publisher != nil {
		testSource.Publisher = *input.Publisher
		changes["publisher"] = *input.Publisher
	}
	if input.PublishDate != nil {
		pd := domain.ParseGenDate(*input.PublishDate)
		testSource.PublishDate = &pd
		changes["publish_date"] = *input.PublishDate
	}
	if input.URL != nil {
		testSource.URL = *input.URL
		changes["url"] = *input.URL
	}
	if input.RepositoryName != nil {
		testSource.RepositoryName = *input.RepositoryName
		changes["repository_name"] = *input.RepositoryName
	}
	if input.CollectionName != nil {
		testSource.CollectionName = *input.CollectionName
		changes["collection_name"] = *input.CollectionName
	}
	if input.CallNumber != nil {
		testSource.CallNumber = *input.CallNumber
		changes["call_number"] = *input.CallNumber
	}
	if input.Notes != nil {
		testSource.Notes = *input.Notes
		changes["notes"] = *input.Notes
	}

	// No changes?
	if len(changes) == 0 {
		return &UpdateSourceResult{Version: current.Version}, nil
	}

	// Validate updated source
	if err := testSource.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	// Create event
	event := domain.NewSourceUpdated(input.ID, changes)

	// Execute command
	version, err := h.execute(ctx, input.ID.String(), "Source", []domain.Event{event}, input.Version)
	if err != nil {
		return nil, err
	}

	return &UpdateSourceResult{Version: version}, nil
}

// DeleteSource deletes a source record.
func (h *Handler) DeleteSource(ctx context.Context, id uuid.UUID, version int64, reason string) error {
	// Get current source from read model
	current, err := h.readStore.GetSource(ctx, id)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrSourceNotFound
	}

	// Check version for optimistic locking
	if current.Version != version {
		return repository.ErrConcurrencyConflict
	}

	// Check if source has citations (referential integrity)
	citations, err := h.readStore.GetCitationsForSource(ctx, id)
	if err != nil {
		return err
	}
	if len(citations) > 0 {
		return ErrSourceHasCitations
	}

	// Create event
	event := domain.NewSourceDeleted(id, reason)

	// Execute command
	_, err = h.execute(ctx, id.String(), "Source", []domain.Event{event}, version)
	return err
}

// CreateCitationInput contains the data for creating a new citation.
type CreateCitationInput struct {
	SourceID      uuid.UUID
	FactType      string
	FactOwnerID   uuid.UUID
	Page          string
	Volume        string
	SourceQuality string
	InformantType string
	EvidenceType  string
	QuotedText    string
	Analysis      string
	TemplateID    string
}

// CreateCitationResult contains the result of creating a citation.
type CreateCitationResult struct {
	ID      uuid.UUID
	Version int64
}

// CreateCitation creates a new citation record.
func (h *Handler) CreateCitation(ctx context.Context, input CreateCitationInput) (*CreateCitationResult, error) {
	// Validate required fields
	if input.SourceID == uuid.Nil {
		return nil, fmt.Errorf("%w: source_id is required", ErrInvalidInput)
	}
	if input.FactOwnerID == uuid.Nil {
		return nil, fmt.Errorf("%w: fact_owner_id is required", ErrInvalidInput)
	}

	// Verify source exists
	source, err := h.readStore.GetSource(ctx, input.SourceID)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return nil, fmt.Errorf("%w: source does not exist", ErrInvalidInput)
	}

	// Create citation entity
	citation := domain.NewCitation(
		input.SourceID,
		domain.FactType(input.FactType),
		input.FactOwnerID,
	)

	if input.Page != "" {
		citation.Page = input.Page
	}
	if input.Volume != "" {
		citation.Volume = input.Volume
	}
	if input.SourceQuality != "" {
		citation.SourceQuality = domain.SourceQuality(input.SourceQuality)
	}
	if input.InformantType != "" {
		citation.InformantType = domain.InformantType(input.InformantType)
	}
	if input.EvidenceType != "" {
		citation.EvidenceType = domain.EvidenceType(input.EvidenceType)
	}
	if input.QuotedText != "" {
		citation.QuotedText = input.QuotedText
	}
	if input.Analysis != "" {
		citation.Analysis = input.Analysis
	}
	if input.TemplateID != "" {
		citation.TemplateID = input.TemplateID
	}

	// Validate citation
	if err := citation.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	// Create event
	event := domain.NewCitationCreated(citation)

	// Execute command (append + project)
	version, err := h.execute(ctx, citation.ID.String(), "Citation", []domain.Event{event}, -1)
	if err != nil {
		return nil, err
	}

	return &CreateCitationResult{
		ID:      citation.ID,
		Version: version,
	}, nil
}

// UpdateCitationInput contains the data for updating a citation.
type UpdateCitationInput struct {
	ID            uuid.UUID
	SourceID      *uuid.UUID
	FactType      *string
	FactOwnerID   *uuid.UUID
	Page          *string
	Volume        *string
	SourceQuality *string
	InformantType *string
	EvidenceType  *string
	QuotedText    *string
	Analysis      *string
	TemplateID    *string
	Version       int64 // Required for optimistic locking
}

// UpdateCitationResult contains the result of updating a citation.
type UpdateCitationResult struct {
	Version int64
}

// UpdateCitation updates an existing citation record.
func (h *Handler) UpdateCitation(ctx context.Context, input UpdateCitationInput) (*UpdateCitationResult, error) {
	// Get current citation from read model
	current, err := h.readStore.GetCitation(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrCitationNotFound
	}

	// Check version for optimistic locking
	if current.Version != input.Version {
		return nil, repository.ErrConcurrencyConflict
	}

	// Build changes map
	changes := make(map[string]any)

	// Apply and validate changes
	testCitation := &domain.Citation{
		ID:            current.ID,
		SourceID:      current.SourceID,
		FactType:      current.FactType,
		FactOwnerID:   current.FactOwnerID,
		Page:          current.Page,
		Volume:        current.Volume,
		SourceQuality: current.SourceQuality,
		InformantType: current.InformantType,
		EvidenceType:  current.EvidenceType,
		QuotedText:    current.QuotedText,
		Analysis:      current.Analysis,
		TemplateID:    current.TemplateID,
	}

	if input.SourceID != nil {
		// Verify source exists
		source, err := h.readStore.GetSource(ctx, *input.SourceID)
		if err != nil {
			return nil, err
		}
		if source == nil {
			return nil, fmt.Errorf("%w: source does not exist", ErrInvalidInput)
		}
		testCitation.SourceID = *input.SourceID
		changes["source_id"] = input.SourceID.String()
	}
	if input.FactType != nil {
		testCitation.FactType = domain.FactType(*input.FactType)
		changes["fact_type"] = *input.FactType
	}
	if input.FactOwnerID != nil {
		testCitation.FactOwnerID = *input.FactOwnerID
		changes["fact_owner_id"] = input.FactOwnerID.String()
	}
	if input.Page != nil {
		testCitation.Page = *input.Page
		changes["page"] = *input.Page
	}
	if input.Volume != nil {
		testCitation.Volume = *input.Volume
		changes["volume"] = *input.Volume
	}
	if input.SourceQuality != nil {
		testCitation.SourceQuality = domain.SourceQuality(*input.SourceQuality)
		changes["source_quality"] = *input.SourceQuality
	}
	if input.InformantType != nil {
		testCitation.InformantType = domain.InformantType(*input.InformantType)
		changes["informant_type"] = *input.InformantType
	}
	if input.EvidenceType != nil {
		testCitation.EvidenceType = domain.EvidenceType(*input.EvidenceType)
		changes["evidence_type"] = *input.EvidenceType
	}
	if input.QuotedText != nil {
		testCitation.QuotedText = *input.QuotedText
		changes["quoted_text"] = *input.QuotedText
	}
	if input.Analysis != nil {
		testCitation.Analysis = *input.Analysis
		changes["analysis"] = *input.Analysis
	}
	if input.TemplateID != nil {
		testCitation.TemplateID = *input.TemplateID
		changes["template_id"] = *input.TemplateID
	}

	// No changes?
	if len(changes) == 0 {
		return &UpdateCitationResult{Version: current.Version}, nil
	}

	// Validate updated citation
	if err := testCitation.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	// Create event
	event := domain.NewCitationUpdated(input.ID, changes)

	// Execute command
	version, err := h.execute(ctx, input.ID.String(), "Citation", []domain.Event{event}, input.Version)
	if err != nil {
		return nil, err
	}

	return &UpdateCitationResult{Version: version}, nil
}

// DeleteCitation deletes a citation record.
func (h *Handler) DeleteCitation(ctx context.Context, id uuid.UUID, version int64, reason string) error {
	// Get current citation from read model
	current, err := h.readStore.GetCitation(ctx, id)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrCitationNotFound
	}

	// Check version for optimistic locking
	if current.Version != version {
		return repository.ErrConcurrencyConflict
	}

	// Create event
	event := domain.NewCitationDeleted(id, reason)

	// Execute command
	_, err = h.execute(ctx, id.String(), "Citation", []domain.Event{event}, version)
	return err
}
