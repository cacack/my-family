package api

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/query"
)

// Source request/response types

type CreateSourceRequest struct {
	SourceType     string  `json:"source_type" validate:"required"`
	Title          string  `json:"title" validate:"required"`
	Author         *string `json:"author,omitempty"`
	Publisher      *string `json:"publisher,omitempty"`
	PublishDate    *string `json:"publish_date,omitempty"`
	URL            *string `json:"url,omitempty"`
	RepositoryName *string `json:"repository_name,omitempty"`
	CollectionName *string `json:"collection_name,omitempty"`
	CallNumber     *string `json:"call_number,omitempty"`
	Notes          *string `json:"notes,omitempty"`
}

type UpdateSourceRequest struct {
	SourceType     *string `json:"source_type,omitempty"`
	Title          *string `json:"title,omitempty"`
	Author         *string `json:"author,omitempty"`
	Publisher      *string `json:"publisher,omitempty"`
	PublishDate    *string `json:"publish_date,omitempty"`
	URL            *string `json:"url,omitempty"`
	RepositoryName *string `json:"repository_name,omitempty"`
	CollectionName *string `json:"collection_name,omitempty"`
	CallNumber     *string `json:"call_number,omitempty"`
	Notes          *string `json:"notes,omitempty"`
	Version        int64   `json:"version" validate:"required"`
}

type SourceResponse struct {
	ID             string  `json:"id"`
	SourceType     string  `json:"source_type"`
	Title          string  `json:"title"`
	Author         *string `json:"author,omitempty"`
	Publisher      *string `json:"publisher,omitempty"`
	PublishDate    *string `json:"publish_date,omitempty"`
	URL            *string `json:"url,omitempty"`
	RepositoryName *string `json:"repository_name,omitempty"`
	CollectionName *string `json:"collection_name,omitempty"`
	CallNumber     *string `json:"call_number,omitempty"`
	Notes          *string `json:"notes,omitempty"`
	CitationCount  int     `json:"citation_count"`
	Version        int64   `json:"version"`
}

type SourceDetailResponse struct {
	SourceResponse
	Citations []CitationResponse `json:"citations,omitempty"`
}

type SourceListResponse struct {
	Sources []SourceResponse `json:"sources"`
	Total   int              `json:"total"`
	Limit   int              `json:"limit"`
	Offset  int              `json:"offset"`
}

// Citation request/response types

type CreateCitationRequest struct {
	SourceID      string  `json:"source_id" validate:"required"`
	FactType      string  `json:"fact_type" validate:"required"`
	FactOwnerID   string  `json:"fact_owner_id" validate:"required"`
	Page          *string `json:"page,omitempty"`
	Volume        *string `json:"volume,omitempty"`
	SourceQuality *string `json:"source_quality,omitempty"`
	InformantType *string `json:"informant_type,omitempty"`
	EvidenceType  *string `json:"evidence_type,omitempty"`
	QuotedText    *string `json:"quoted_text,omitempty"`
	Analysis      *string `json:"analysis,omitempty"`
	TemplateID    *string `json:"template_id,omitempty"`
}

type UpdateCitationRequest struct {
	Page          *string `json:"page,omitempty"`
	Volume        *string `json:"volume,omitempty"`
	SourceQuality *string `json:"source_quality,omitempty"`
	InformantType *string `json:"informant_type,omitempty"`
	EvidenceType  *string `json:"evidence_type,omitempty"`
	QuotedText    *string `json:"quoted_text,omitempty"`
	Analysis      *string `json:"analysis,omitempty"`
	TemplateID    *string `json:"template_id,omitempty"`
	Version       int64   `json:"version" validate:"required"`
}

type CitationResponse struct {
	ID            string  `json:"id"`
	SourceID      string  `json:"source_id"`
	SourceTitle   string  `json:"source_title"`
	FactType      string  `json:"fact_type"`
	FactOwnerID   string  `json:"fact_owner_id"`
	Page          *string `json:"page,omitempty"`
	Volume        *string `json:"volume,omitempty"`
	SourceQuality *string `json:"source_quality,omitempty"`
	InformantType *string `json:"informant_type,omitempty"`
	EvidenceType  *string `json:"evidence_type,omitempty"`
	QuotedText    *string `json:"quoted_text,omitempty"`
	Analysis      *string `json:"analysis,omitempty"`
	TemplateID    *string `json:"template_id,omitempty"`
	Version       int64   `json:"version"`
}

type CitationListResponse struct {
	Citations []CitationResponse `json:"citations"`
	Total     int                `json:"total"`
}

// Source handlers

// listSources handles GET /sources
func (s *Server) listSources(c echo.Context) error {
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	sortBy := c.QueryParam("sort")
	sortOrder := c.QueryParam("order")
	searchQuery := c.QueryParam("q")

	result, err := s.sourceService.ListSources(c.Request().Context(), query.ListSourcesInput{
		Limit:     limit,
		Offset:    offset,
		SortBy:    sortBy,
		SortOrder: sortOrder,
		Query:     searchQuery,
	})
	if err != nil {
		return err
	}

	response := SourceListResponse{
		Sources: make([]SourceResponse, len(result.Sources)),
		Total:   result.Total,
		Limit:   result.Limit,
		Offset:  result.Offset,
	}

	for i, src := range result.Sources {
		response.Sources[i] = convertSourceToResponse(src)
	}

	return c.JSON(http.StatusOK, response)
}

// createSource handles POST /sources
func (s *Server) createSource(c echo.Context) error {
	var req CreateSourceRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	input := command.CreateSourceInput{
		SourceType: req.SourceType,
		Title:      req.Title,
	}

	if req.Author != nil {
		input.Author = *req.Author
	}
	if req.Publisher != nil {
		input.Publisher = *req.Publisher
	}
	if req.PublishDate != nil {
		input.PublishDate = *req.PublishDate
	}
	if req.URL != nil {
		input.URL = *req.URL
	}
	if req.RepositoryName != nil {
		input.RepositoryName = *req.RepositoryName
	}
	if req.CollectionName != nil {
		input.CollectionName = *req.CollectionName
	}
	if req.CallNumber != nil {
		input.CallNumber = *req.CallNumber
	}
	if req.Notes != nil {
		input.Notes = *req.Notes
	}

	result, err := s.commandHandler.CreateSource(c.Request().Context(), input)
	if err != nil {
		return err
	}

	// Fetch the created source to return full response
	source, err := s.sourceService.GetSource(c.Request().Context(), result.ID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, convertSourceDetailToResponse(*source))
}

// getSource handles GET /sources/:id
func (s *Server) getSource(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid source ID")
	}

	source, err := s.sourceService.GetSource(c.Request().Context(), id)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, convertSourceDetailToResponse(*source))
}

// updateSource handles PUT /sources/:id
func (s *Server) updateSource(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid source ID")
	}

	var req UpdateSourceRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	input := command.UpdateSourceInput{
		ID:             id,
		SourceType:     req.SourceType,
		Title:          req.Title,
		Author:         req.Author,
		Publisher:      req.Publisher,
		PublishDate:    req.PublishDate,
		URL:            req.URL,
		RepositoryName: req.RepositoryName,
		CollectionName: req.CollectionName,
		CallNumber:     req.CallNumber,
		Notes:          req.Notes,
		Version:        req.Version,
	}

	_, err = s.commandHandler.UpdateSource(c.Request().Context(), input)
	if err != nil {
		return err
	}

	// Fetch the updated source to return full response
	source, err := s.sourceService.GetSource(c.Request().Context(), id)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, convertSourceToResponse(source.Source))
}

// deleteSource handles DELETE /sources/:id
func (s *Server) deleteSource(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid source ID")
	}

	// Get version from query param
	version, _ := strconv.ParseInt(c.QueryParam("version"), 10, 64)
	if version == 0 {
		// Try to get current version
		source, err := s.sourceService.GetSource(c.Request().Context(), id)
		if err != nil {
			return err
		}
		version = source.Version
	}

	err = s.commandHandler.DeleteSource(c.Request().Context(), id, version, "")
	if err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

// searchSources handles GET /sources/search
func (s *Server) searchSources(c echo.Context) error {
	q := c.QueryParam("q")
	if len(q) < 2 {
		return echo.NewHTTPError(http.StatusBadRequest, "Search query must be at least 2 characters")
	}

	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit == 0 {
		limit = 20
	}

	sources, err := s.sourceService.SearchSources(c.Request().Context(), q, limit)
	if err != nil {
		return err
	}

	response := struct {
		Sources []SourceResponse `json:"sources"`
		Total   int              `json:"total"`
		Query   string           `json:"query"`
	}{
		Sources: make([]SourceResponse, len(sources)),
		Total:   len(sources),
		Query:   q,
	}

	for i, src := range sources {
		response.Sources[i] = convertSourceToResponse(src)
	}

	return c.JSON(http.StatusOK, response)
}

// Citation handlers

// getCitationsForSource handles GET /sources/:id/citations
func (s *Server) getCitationsForSource(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid source ID")
	}

	// Get source detail which includes citations
	source, err := s.sourceService.GetSource(c.Request().Context(), id)
	if err != nil {
		return err
	}

	response := CitationListResponse{
		Citations: make([]CitationResponse, len(source.Citations)),
		Total:     len(source.Citations),
	}

	for i, cit := range source.Citations {
		response.Citations[i] = convertCitationToResponse(cit)
	}

	return c.JSON(http.StatusOK, response)
}

// getCitationsForPerson handles GET /persons/:id/citations
func (s *Server) getCitationsForPerson(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid person ID")
	}

	citations, err := s.sourceService.GetCitationsForPerson(c.Request().Context(), id)
	if err != nil {
		return err
	}

	response := CitationListResponse{
		Citations: make([]CitationResponse, len(citations)),
		Total:     len(citations),
	}

	for i, cit := range citations {
		response.Citations[i] = convertCitationToResponse(cit)
	}

	return c.JSON(http.StatusOK, response)
}

// createCitation handles POST /citations
func (s *Server) createCitation(c echo.Context) error {
	var req CreateCitationRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	sourceID, err := uuid.Parse(req.SourceID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid source_id")
	}

	factOwnerID, err := uuid.Parse(req.FactOwnerID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid fact_owner_id")
	}

	input := command.CreateCitationInput{
		SourceID:    sourceID,
		FactType:    req.FactType,
		FactOwnerID: factOwnerID,
	}

	if req.Page != nil {
		input.Page = *req.Page
	}
	if req.Volume != nil {
		input.Volume = *req.Volume
	}
	if req.SourceQuality != nil {
		input.SourceQuality = *req.SourceQuality
	}
	if req.InformantType != nil {
		input.InformantType = *req.InformantType
	}
	if req.EvidenceType != nil {
		input.EvidenceType = *req.EvidenceType
	}
	if req.QuotedText != nil {
		input.QuotedText = *req.QuotedText
	}
	if req.Analysis != nil {
		input.Analysis = *req.Analysis
	}
	if req.TemplateID != nil {
		input.TemplateID = *req.TemplateID
	}

	result, err := s.commandHandler.CreateCitation(c.Request().Context(), input)
	if err != nil {
		return err
	}

	// Fetch the created citation to return full response
	citation, err := s.sourceService.GetCitation(c.Request().Context(), result.ID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, convertCitationToResponse(*citation))
}

// getCitation handles GET /citations/:id
func (s *Server) getCitation(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid citation ID")
	}

	citation, err := s.sourceService.GetCitation(c.Request().Context(), id)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, convertCitationToResponse(*citation))
}

// updateCitation handles PUT /citations/:id
func (s *Server) updateCitation(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid citation ID")
	}

	var req UpdateCitationRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	input := command.UpdateCitationInput{
		ID:            id,
		Page:          req.Page,
		Volume:        req.Volume,
		SourceQuality: req.SourceQuality,
		InformantType: req.InformantType,
		EvidenceType:  req.EvidenceType,
		QuotedText:    req.QuotedText,
		Analysis:      req.Analysis,
		TemplateID:    req.TemplateID,
		Version:       req.Version,
	}

	_, err = s.commandHandler.UpdateCitation(c.Request().Context(), input)
	if err != nil {
		return err
	}

	// Fetch the updated citation to return full response
	citation, err := s.sourceService.GetCitation(c.Request().Context(), id)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, convertCitationToResponse(*citation))
}

// deleteCitation handles DELETE /citations/:id
func (s *Server) deleteCitation(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid citation ID")
	}

	// Get version from query param
	version, _ := strconv.ParseInt(c.QueryParam("version"), 10, 64)
	if version == 0 {
		// Try to get current version
		citation, err := s.sourceService.GetCitation(c.Request().Context(), id)
		if err != nil {
			return err
		}
		version = citation.Version
	}

	err = s.commandHandler.DeleteCitation(c.Request().Context(), id, version, "")
	if err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

// Helper functions

func convertSourceToResponse(s query.Source) SourceResponse {
	return SourceResponse{
		ID:             s.ID.String(),
		SourceType:     s.SourceType,
		Title:          s.Title,
		Author:         s.Author,
		Publisher:      s.Publisher,
		PublishDate:    s.PublishDate,
		URL:            s.URL,
		RepositoryName: s.RepositoryName,
		CollectionName: s.CollectionName,
		CallNumber:     s.CallNumber,
		Notes:          s.Notes,
		CitationCount:  s.CitationCount,
		Version:        s.Version,
	}
}

func convertSourceDetailToResponse(sd query.SourceDetail) SourceDetailResponse {
	resp := SourceDetailResponse{
		SourceResponse: convertSourceToResponse(sd.Source),
		Citations:      make([]CitationResponse, len(sd.Citations)),
	}
	for i, cit := range sd.Citations {
		resp.Citations[i] = convertCitationToResponse(cit)
	}
	return resp
}

func convertCitationToResponse(c query.Citation) CitationResponse {
	return CitationResponse{
		ID:            c.ID.String(),
		SourceID:      c.SourceID.String(),
		SourceTitle:   c.SourceTitle,
		FactType:      c.FactType,
		FactOwnerID:   c.FactOwnerID.String(),
		Page:          c.Page,
		Volume:        c.Volume,
		SourceQuality: c.SourceQuality,
		InformantType: c.InformantType,
		EvidenceType:  c.EvidenceType,
		QuotedText:    c.QuotedText,
		Analysis:      c.Analysis,
		TemplateID:    c.TemplateID,
		Version:       c.Version,
	}
}
