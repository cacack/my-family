package query

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// SourceService provides query operations for sources and citations.
type SourceService struct {
	readStore repository.ReadModelStore
}

// NewSourceService creates a new source query service.
func NewSourceService(readStore repository.ReadModelStore) *SourceService {
	return &SourceService{readStore: readStore}
}

// Source represents a source in query results.
type Source struct {
	ID             uuid.UUID  `json:"id"`
	SourceType     string     `json:"source_type"`
	Title          string     `json:"title"`
	Author         *string    `json:"author,omitempty"`
	Publisher      *string    `json:"publisher,omitempty"`
	PublishDate    *string    `json:"publish_date,omitempty"`
	URL            *string    `json:"url,omitempty"`
	RepositoryName *string    `json:"repository_name,omitempty"`
	CollectionName *string    `json:"collection_name,omitempty"`
	CallNumber     *string    `json:"call_number,omitempty"`
	Notes          *string    `json:"notes,omitempty"`
	CitationCount  int        `json:"citation_count"`
	Version        int64      `json:"version"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Citation represents a citation in query results.
type Citation struct {
	ID            uuid.UUID  `json:"id"`
	SourceID      uuid.UUID  `json:"source_id"`
	SourceTitle   string     `json:"source_title"`
	FactType      string     `json:"fact_type"`
	FactOwnerID   uuid.UUID  `json:"fact_owner_id"`
	Page          *string    `json:"page,omitempty"`
	Volume        *string    `json:"volume,omitempty"`
	SourceQuality *string    `json:"source_quality,omitempty"`
	InformantType *string    `json:"informant_type,omitempty"`
	EvidenceType  *string    `json:"evidence_type,omitempty"`
	QuotedText    *string    `json:"quoted_text,omitempty"`
	Analysis      *string    `json:"analysis,omitempty"`
	TemplateID    *string    `json:"template_id,omitempty"`
	Version       int64      `json:"version"`
	CreatedAt     time.Time  `json:"created_at"`
}

// SourceDetail includes citations attached to this source.
type SourceDetail struct {
	Source
	Citations []Citation `json:"citations,omitempty"`
}

// ListSourcesInput contains options for listing sources.
type ListSourcesInput struct {
	Limit     int
	Offset    int
	SortBy    string // title, source_type, updated_at
	SortOrder string // asc, desc
	Query     string // optional search term
}

// SourceListResult contains paginated source results.
type SourceListResult struct {
	Sources []Source `json:"sources"`
	Total   int      `json:"total"`
	Limit   int      `json:"limit"`
	Offset  int      `json:"offset"`
}

// ListSources returns a paginated list of sources.
func (s *SourceService) ListSources(ctx context.Context, input ListSourcesInput) (*SourceListResult, error) {
	opts := repository.ListOptions{
		Limit:  input.Limit,
		Offset: input.Offset,
		Sort:   input.SortBy,
		Order:  input.SortOrder,
	}

	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > 100 {
		opts.Limit = 100
	}
	if opts.Sort == "" {
		opts.Sort = "title"
	}
	if opts.Order == "" {
		opts.Order = "asc"
	}

	readModels, total, err := s.readStore.ListSources(ctx, opts)
	if err != nil {
		return nil, err
	}

	sources := make([]Source, len(readModels))
	for i, rm := range readModels {
		sources[i] = convertReadModelToSource(rm)
	}

	return &SourceListResult{
		Sources: sources,
		Total:   total,
		Limit:   opts.Limit,
		Offset:  opts.Offset,
	}, nil
}

// GetSource returns a source by ID with its citations.
func (s *SourceService) GetSource(ctx context.Context, id uuid.UUID) (*SourceDetail, error) {
	rm, err := s.readStore.GetSource(ctx, id)
	if err != nil {
		return nil, err
	}
	if rm == nil {
		return nil, ErrNotFound
	}

	source := convertReadModelToSource(*rm)
	detail := &SourceDetail{
		Source: source,
	}

	// Get citations for this source
	citationRMs, err := s.readStore.GetCitationsForSource(ctx, id)
	if err != nil {
		return nil, err
	}

	detail.Citations = make([]Citation, len(citationRMs))
	for i, citationRM := range citationRMs {
		detail.Citations[i] = convertReadModelToCitation(citationRM)
	}

	return detail, nil
}

// SearchSources searches for sources by title, author, or other fields.
func (s *SourceService) SearchSources(ctx context.Context, query string, limit int) ([]Source, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	readModels, err := s.readStore.SearchSources(ctx, query, limit)
	if err != nil {
		return nil, err
	}

	sources := make([]Source, len(readModels))
	for i, rm := range readModels {
		sources[i] = convertReadModelToSource(rm)
	}

	return sources, nil
}

// GetCitationsForPerson returns all citations for a person.
func (s *SourceService) GetCitationsForPerson(ctx context.Context, personID uuid.UUID) ([]Citation, error) {
	readModels, err := s.readStore.GetCitationsForPerson(ctx, personID)
	if err != nil {
		return nil, err
	}

	citations := make([]Citation, len(readModels))
	for i, rm := range readModels {
		citations[i] = convertReadModelToCitation(rm)
	}

	return citations, nil
}

// GetCitation returns a single citation by ID.
func (s *SourceService) GetCitation(ctx context.Context, id uuid.UUID) (*Citation, error) {
	rm, err := s.readStore.GetCitation(ctx, id)
	if err != nil {
		return nil, err
	}
	if rm == nil {
		return nil, ErrNotFound
	}

	citation := convertReadModelToCitation(*rm)
	return &citation, nil
}

// GetCitationsForFact returns citations for a specific fact.
func (s *SourceService) GetCitationsForFact(ctx context.Context, factType string, factOwnerID uuid.UUID) ([]Citation, error) {
	readModels, err := s.readStore.GetCitationsForFact(ctx, domain.FactType(factType), factOwnerID)
	if err != nil {
		return nil, err
	}

	citations := make([]Citation, len(readModels))
	for i, rm := range readModels {
		citations[i] = convertReadModelToCitation(rm)
	}

	return citations, nil
}

// Helper function to convert read model to query result.
func convertReadModelToSource(rm repository.SourceReadModel) Source {
	s := Source{
		ID:            rm.ID,
		SourceType:    string(rm.SourceType),
		Title:         rm.Title,
		CitationCount: rm.CitationCount,
		Version:       rm.Version,
		UpdatedAt:     rm.UpdatedAt,
	}

	if rm.Author != "" {
		s.Author = &rm.Author
	}
	if rm.Publisher != "" {
		s.Publisher = &rm.Publisher
	}
	if rm.PublishDateRaw != "" {
		s.PublishDate = &rm.PublishDateRaw
	}
	if rm.URL != "" {
		s.URL = &rm.URL
	}
	if rm.RepositoryName != "" {
		s.RepositoryName = &rm.RepositoryName
	}
	if rm.CollectionName != "" {
		s.CollectionName = &rm.CollectionName
	}
	if rm.CallNumber != "" {
		s.CallNumber = &rm.CallNumber
	}
	if rm.Notes != "" {
		s.Notes = &rm.Notes
	}

	return s
}

// Helper function to convert citation read model to query result.
func convertReadModelToCitation(rm repository.CitationReadModel) Citation {
	c := Citation{
		ID:          rm.ID,
		SourceID:    rm.SourceID,
		SourceTitle: rm.SourceTitle,
		FactType:    string(rm.FactType),
		FactOwnerID: rm.FactOwnerID,
		Version:     rm.Version,
		CreatedAt:   rm.CreatedAt,
	}

	if rm.Page != "" {
		c.Page = &rm.Page
	}
	if rm.Volume != "" {
		c.Volume = &rm.Volume
	}
	if rm.SourceQuality != "" {
		sq := string(rm.SourceQuality)
		c.SourceQuality = &sq
	}
	if rm.InformantType != "" {
		it := string(rm.InformantType)
		c.InformantType = &it
	}
	if rm.EvidenceType != "" {
		et := string(rm.EvidenceType)
		c.EvidenceType = &et
	}
	if rm.QuotedText != "" {
		c.QuotedText = &rm.QuotedText
	}
	if rm.Analysis != "" {
		c.Analysis = &rm.Analysis
	}
	if rm.TemplateID != "" {
		c.TemplateID = &rm.TemplateID
	}

	return c
}
