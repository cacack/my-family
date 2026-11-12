package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

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
	publishDate, err := normalizeDateInput("publish_date", input.PublishDate)
	if err != nil {
		return nil, err
	}
	if publishDate != "" {
		pd := domain.ParseGenDate(publishDate)
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
		return nil, fmt.Errorf("executing create source command: %w", err)
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
	current, err := h.readStore.GetSource(ctx, h.branchID, input.ID)
	if err != nil {
		return nil, fmt.Errorf("getting source: %w", err)
	}
	if current == nil {
		return nil, ErrSourceNotFound
	}

	// Check version for optimistic locking
	if current.Version != input.Version {
		return nil, repository.ErrConcurrencyConflict
	}

	// Build changes map. A field equal to its current value is not a change
	// (the edit form sends every field), so only differing fields are recorded.
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

	recordChange(changes, "source_type", input.SourceType, &testSource.SourceType)
	recordChange(changes, "title", input.Title, &testSource.Title)
	recordChange(changes, "author", input.Author, &testSource.Author)
	recordChange(changes, "publisher", input.Publisher, &testSource.Publisher)
	publishDate, publishChanged, err := changedDate("publish_date", input.PublishDate, current.PublishDateRaw)
	if err != nil {
		return nil, err
	}
	if publishChanged {
		pd := domain.ParseGenDate(publishDate)
		testSource.PublishDate = &pd
		changes["publish_date"] = publishDate
	}
	recordChange(changes, "url", input.URL, &testSource.URL)
	recordChange(changes, "repository_name", input.RepositoryName, &testSource.RepositoryName)
	recordChange(changes, "collection_name", input.CollectionName, &testSource.CollectionName)
	recordChange(changes, "call_number", input.CallNumber, &testSource.CallNumber)
	recordChange(changes, "notes", input.Notes, &testSource.Notes)

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
		return nil, fmt.Errorf("executing update source command: %w", err)
	}

	return &UpdateSourceResult{Version: version}, nil
}

// DeleteSource deletes a source record.
func (h *Handler) DeleteSource(ctx context.Context, id uuid.UUID, version int64, reason string) error {
	// Get current source from read model
	current, err := h.readStore.GetSource(ctx, h.branchID, id)
	if err != nil {
		return fmt.Errorf("getting source: %w", err)
	}
	if current == nil {
		return ErrSourceNotFound
	}

	// Check version for optimistic locking
	if current.Version != version {
		return repository.ErrConcurrencyConflict
	}

	// Check if source has citations (referential integrity)
	citations, err := h.readStore.GetCitationsForSource(ctx, h.branchID, id)
	if err != nil {
		return fmt.Errorf("getting citations for source: %w", err)
	}
	if len(citations) > 0 {
		return ErrSourceHasCitations
	}

	// Create event
	event := domain.NewSourceDeleted(id, reason)

	// Execute command
	_, err = h.execute(ctx, id.String(), "Source", []domain.Event{event}, version)
	if err != nil {
		return fmt.Errorf("executing delete source command: %w", err)
	}
	return nil
}

// CreateCitationInput contains the data for creating a new citation.
type CreateCitationInput struct {
	SourceID      uuid.UUID
	FactType      string
	FactOwnerID   uuid.UUID
	FactID        *uuid.UUID // the cited life event or attribute, if any (#1294)
	Page          string
	Volume        string
	SourceQuality string
	InformantType string
	EvidenceType  string
	QuotedText    string
	Analysis      string
	TemplateID    string
	Fields        map[string]string
	GedcomXref    string
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
	source, err := h.readStore.GetSource(ctx, h.branchID, input.SourceID)
	if err != nil {
		return nil, fmt.Errorf("getting source: %w", err)
	}
	if source == nil {
		return nil, fmt.Errorf("%w: source does not exist", ErrInvalidInput)
	}

	if input.FactID != nil {
		if err := h.checkFactInstance(ctx, *input.FactID, domain.FactType(input.FactType), input.FactOwnerID); err != nil {
			return nil, err
		}
	}

	// Create citation entity
	citation := domain.NewCitation(
		input.SourceID,
		domain.FactType(input.FactType),
		input.FactOwnerID,
	)
	citation.FactID = input.FactID

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
	if len(input.Fields) > 0 {
		citation.Fields = input.Fields
	}
	if input.GedcomXref != "" {
		if err := validateGedcomXref(input.GedcomXref); err != nil {
			return nil, err
		}
		citation.GedcomXref = input.GedcomXref
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
		return nil, fmt.Errorf("executing create citation command: %w", err)
	}

	return &CreateCitationResult{
		ID:      citation.ID,
		Version: version,
	}, nil
}

// checkFactInstance verifies that factID is a life event or attribute of
// factType owned by factOwnerID on the handler's branch, so a citation (#1294)
// or an evidence analysis or proof summary (#1304) linked to it is listed with
// it and with no other fact.
func (h *Handler) checkFactInstance(ctx context.Context, factID uuid.UUID, factType domain.FactType, factOwnerID uuid.UUID) error {
	event, err := h.readStore.GetEvent(ctx, h.branchID, factID)
	if err != nil {
		return fmt.Errorf("getting life event: %w", err)
	}
	if event != nil {
		if event.FactType != factType || event.OwnerID != factOwnerID {
			return fmt.Errorf("%w: fact_id is a life event of another fact type or owner", ErrInvalidInput)
		}
		return nil
	}
	attr, err := h.readStore.GetAttribute(ctx, h.branchID, factID)
	if err != nil {
		return fmt.Errorf("getting attribute: %w", err)
	}
	if attr == nil {
		return fmt.Errorf("%w: fact_id is not a life event or attribute", ErrInvalidInput)
	}
	if attr.FactType != factType || attr.PersonID != factOwnerID {
		return fmt.Errorf("%w: fact_id is an attribute of another fact type or owner", ErrInvalidInput)
	}
	return nil
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
	Fields        map[string]string // nil means no change; empty map clears fields
	Version       int64             // Required for optimistic locking
}

// UpdateCitationResult contains the result of updating a citation.
type UpdateCitationResult struct {
	Version int64
}

// UpdateCitation updates an existing citation record.
func (h *Handler) UpdateCitation(ctx context.Context, input UpdateCitationInput) (*UpdateCitationResult, error) { //nolint:gocyclo // field-by-field update is inherently branchy
	// Get current citation from read model
	current, err := h.readStore.GetCitation(ctx, h.branchID, input.ID)
	if err != nil {
		return nil, fmt.Errorf("getting citation: %w", err)
	}
	if current == nil {
		return nil, ErrCitationNotFound
	}

	// Check version for optimistic locking
	if current.Version != input.Version {
		return nil, repository.ErrConcurrencyConflict
	}

	// Build changes map. A field equal to its current value is not a change
	// (the edit form sends every field), so only differing fields are recorded.
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

	if input.SourceID != nil && *input.SourceID != current.SourceID {
		// Verify source exists
		source, err := h.readStore.GetSource(ctx, h.branchID, *input.SourceID)
		if err != nil {
			return nil, fmt.Errorf("getting source: %w", err)
		}
		if source == nil {
			return nil, fmt.Errorf("%w: source does not exist", ErrInvalidInput)
		}
		testCitation.SourceID = *input.SourceID
		changes["source_id"] = input.SourceID.String()
	}
	if input.FactType != nil && domain.FactType(*input.FactType) != current.FactType {
		testCitation.FactType = domain.FactType(*input.FactType)
		changes["fact_type"] = *input.FactType
	}
	if input.FactOwnerID != nil && *input.FactOwnerID != current.FactOwnerID {
		testCitation.FactOwnerID = *input.FactOwnerID
		changes["fact_owner_id"] = input.FactOwnerID.String()
	}
	// A citation moved to another fact type or owner no longer cites its life
	// event or attribute (#1294).
	_, typeChanged := changes["fact_type"]
	_, ownerChanged := changes["fact_owner_id"]
	if current.FactID != nil && (typeChanged || ownerChanged) {
		changes["fact_id"] = nil
	}
	if input.Page != nil && *input.Page != current.Page {
		testCitation.Page = *input.Page
		changes["page"] = *input.Page
	}
	if input.Volume != nil && *input.Volume != current.Volume {
		testCitation.Volume = *input.Volume
		changes["volume"] = *input.Volume
	}
	if input.SourceQuality != nil && domain.SourceQuality(*input.SourceQuality) != current.SourceQuality {
		testCitation.SourceQuality = domain.SourceQuality(*input.SourceQuality)
		changes["source_quality"] = *input.SourceQuality
	}
	if input.InformantType != nil && domain.InformantType(*input.InformantType) != current.InformantType {
		testCitation.InformantType = domain.InformantType(*input.InformantType)
		changes["informant_type"] = *input.InformantType
	}
	if input.EvidenceType != nil && domain.EvidenceType(*input.EvidenceType) != current.EvidenceType {
		testCitation.EvidenceType = domain.EvidenceType(*input.EvidenceType)
		changes["evidence_type"] = *input.EvidenceType
	}
	if input.QuotedText != nil && *input.QuotedText != current.QuotedText {
		testCitation.QuotedText = *input.QuotedText
		changes["quoted_text"] = *input.QuotedText
	}
	if input.Analysis != nil && *input.Analysis != current.Analysis {
		testCitation.Analysis = *input.Analysis
		changes["analysis"] = *input.Analysis
	}
	if input.TemplateID != nil && *input.TemplateID != current.TemplateID {
		testCitation.TemplateID = *input.TemplateID
		changes["template_id"] = *input.TemplateID
	}
	if input.Fields != nil {
		// No stored fields compares as an empty map, so a form that sends none
		// for a citation without any records no change.
		var currentFields map[string]string
		if current.FieldsJSON != "" {
			if err := json.Unmarshal([]byte(current.FieldsJSON), &currentFields); err != nil {
				return nil, fmt.Errorf("decoding citation fields: %w", err)
			}
		}
		if !maps.Equal(input.Fields, currentFields) {
			testCitation.Fields = input.Fields
			changes["fields"] = input.Fields
		}
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
		return nil, fmt.Errorf("executing update citation command: %w", err)
	}

	return &UpdateCitationResult{Version: version}, nil
}

// DeleteCitation deletes a citation record.
func (h *Handler) DeleteCitation(ctx context.Context, id uuid.UUID, version int64, reason string) error {
	// Get current citation from read model
	current, err := h.readStore.GetCitation(ctx, h.branchID, id)
	if err != nil {
		return fmt.Errorf("getting citation: %w", err)
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
	if err != nil {
		return fmt.Errorf("executing delete citation command: %w", err)
	}
	return nil
}
