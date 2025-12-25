package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/cacack/my-family/internal/query"
)

// ChangeEntryResponse represents a single change entry in the API response.
type ChangeEntryResponse struct {
	ID         string                 `json:"id"`
	Timestamp  string                 `json:"timestamp"`
	EntityType string                 `json:"entity_type"`
	EntityID   string                 `json:"entity_id"`
	EntityName string                 `json:"entity_name"`
	Action     string                 `json:"action"`
	Changes    map[string]FieldChange `json:"changes,omitempty"`
	UserID     *string                `json:"user_id,omitempty"`
}

// FieldChange represents a field-level change.
type FieldChange struct {
	OldValue any `json:"old_value,omitempty"`
	NewValue any `json:"new_value,omitempty"`
}

// ChangeHistoryResponse represents the paginated history response.
type ChangeHistoryResponse struct {
	Items   []ChangeEntryResponse `json:"items"`
	Total   int                   `json:"total"`
	Limit   int                   `json:"limit"`
	Offset  int                   `json:"offset"`
	HasMore bool                  `json:"has_more"`
}

// getGlobalHistory handles GET /history - Global changelog
func (s *Server) getGlobalHistory(c echo.Context) error {
	// Parse query parameters
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	entityType := c.QueryParam("entity_type")
	fromStr := c.QueryParam("from")
	toStr := c.QueryParam("to")

	// Parse time filters
	// Default to a wide time range if not specified
	fromTime := time.Time{}                  // Zero value (year 1)
	toTime := time.Now().Add(24 * time.Hour) // Tomorrow
	var err error

	if fromStr != "" {
		fromTime, err = time.Parse(time.RFC3339, fromStr)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid 'from' timestamp. Use ISO 8601 format.")
		}
	}

	if toStr != "" {
		toTime, err = time.Parse(time.RFC3339, toStr)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid 'to' timestamp. Use ISO 8601 format.")
		}
	}

	// Validate entity_type if provided
	if entityType != "" {
		validTypes := map[string]bool{
			"person":   true,
			"family":   true,
			"source":   true,
			"citation": true,
		}
		if !validTypes[entityType] {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid entity_type. Must be one of: person, family, source, citation")
		}
	}

	// Build event types filter from entity_type
	var eventTypes []string
	if entityType != "" {
		eventTypes = mapEntityTypeToEventTypes(entityType)
	}

	// Query history service
	result, err := s.historyService.GetGlobalHistory(c.Request().Context(), query.GetGlobalHistoryInput{
		FromTime:   fromTime,
		ToTime:     toTime,
		EventTypes: eventTypes,
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		return err
	}

	// Convert to API response
	response := ChangeHistoryResponse{
		Items:   make([]ChangeEntryResponse, len(result.Entries)),
		Total:   result.TotalCount,
		Limit:   result.Limit,
		Offset:  result.Offset,
		HasMore: result.HasMore,
	}

	for i, entry := range result.Entries {
		response.Items[i] = convertChangeEntryToResponse(entry)
	}

	return c.JSON(http.StatusOK, response)
}

// getPersonHistory handles GET /persons/{id}/history
func (s *Server) getPersonHistory(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid person ID")
	}

	// Verify person exists
	_, err = s.personService.GetPerson(c.Request().Context(), id)
	if err != nil {
		return err
	}

	// Parse pagination parameters
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))

	// Query history
	result, err := s.historyService.GetEntityHistory(c.Request().Context(), "person", id, limit, offset)
	if err != nil {
		return err
	}

	// Convert to API response
	response := ChangeHistoryResponse{
		Items:   make([]ChangeEntryResponse, len(result.Entries)),
		Total:   result.TotalCount,
		Limit:   result.Limit,
		Offset:  result.Offset,
		HasMore: result.HasMore,
	}

	for i, entry := range result.Entries {
		response.Items[i] = convertChangeEntryToResponse(entry)
	}

	return c.JSON(http.StatusOK, response)
}

// getFamilyHistory handles GET /families/{id}/history
func (s *Server) getFamilyHistory(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid family ID")
	}

	// Verify family exists
	_, err = s.familyService.GetFamily(c.Request().Context(), id)
	if err != nil {
		return err
	}

	// Parse pagination parameters
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))

	// Query history
	result, err := s.historyService.GetEntityHistory(c.Request().Context(), "family", id, limit, offset)
	if err != nil {
		return err
	}

	// Convert to API response
	response := ChangeHistoryResponse{
		Items:   make([]ChangeEntryResponse, len(result.Entries)),
		Total:   result.TotalCount,
		Limit:   result.Limit,
		Offset:  result.Offset,
		HasMore: result.HasMore,
	}

	for i, entry := range result.Entries {
		response.Items[i] = convertChangeEntryToResponse(entry)
	}

	return c.JSON(http.StatusOK, response)
}

// getSourceHistory handles GET /sources/{id}/history
func (s *Server) getSourceHistory(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid source ID")
	}

	// Verify source exists
	_, err = s.sourceService.GetSource(c.Request().Context(), id)
	if err != nil {
		return err
	}

	// Parse pagination parameters
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))

	// Query history
	result, err := s.historyService.GetEntityHistory(c.Request().Context(), "source", id, limit, offset)
	if err != nil {
		return err
	}

	// Convert to API response
	response := ChangeHistoryResponse{
		Items:   make([]ChangeEntryResponse, len(result.Entries)),
		Total:   result.TotalCount,
		Limit:   result.Limit,
		Offset:  result.Offset,
		HasMore: result.HasMore,
	}

	for i, entry := range result.Entries {
		response.Items[i] = convertChangeEntryToResponse(entry)
	}

	return c.JSON(http.StatusOK, response)
}

// convertChangeEntryToResponse converts a query.ChangeEntry to API response format.
func convertChangeEntryToResponse(entry query.ChangeEntry) ChangeEntryResponse {
	resp := ChangeEntryResponse{
		ID:         entry.ID.String(),
		Timestamp:  entry.Timestamp.Format(time.RFC3339),
		EntityType: entry.EntityType,
		EntityID:   entry.EntityID.String(),
		EntityName: entry.EntityName,
		Action:     entry.Action,
		UserID:     entry.UserID,
	}

	if len(entry.Changes) > 0 {
		resp.Changes = make(map[string]FieldChange)
		for field, change := range entry.Changes {
			resp.Changes[field] = FieldChange{
				OldValue: change.OldValue,
				NewValue: change.NewValue,
			}
		}
	}

	return resp
}

// mapEntityTypeToEventTypes maps an entity type to its corresponding event types.
func mapEntityTypeToEventTypes(entityType string) []string {
	switch entityType {
	case "person":
		return []string{"PersonCreated", "PersonUpdated", "PersonDeleted"}
	case "family":
		return []string{"FamilyCreated", "FamilyUpdated", "FamilyDeleted", "ChildLinkedToFamily", "ChildUnlinkedFromFamily"}
	case "source":
		return []string{"SourceCreated", "SourceUpdated", "SourceDeleted"}
	case "citation":
		return []string{"CitationCreated", "CitationUpdated", "CitationDeleted"}
	default:
		return nil
	}
}
