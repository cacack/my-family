package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/query"
)

// Rollback request/response types

// RestorePointResponse represents a single restore point in the API response.
type RestorePointResponse struct {
	Version   int64  `json:"version"`
	Timestamp string `json:"timestamp"`
	Action    string `json:"action"`
	Summary   string `json:"summary"`
}

// RestorePointsResponse represents the paginated restore points response.
type RestorePointsResponse struct {
	Items   []RestorePointResponse `json:"items"`
	Total   int                    `json:"total"`
	HasMore bool                   `json:"has_more"`
}

// RollbackRequest represents a rollback request.
type RollbackRequest struct {
	TargetVersion int64 `json:"target_version"`
}

// RollbackResponse represents the result of a rollback operation.
type RollbackResponse struct {
	EntityID   string         `json:"entity_id"`
	EntityType string         `json:"entity_type"`
	NewVersion int64          `json:"new_version"`
	Changes    map[string]any `json:"changes"`
	Message    string         `json:"message"`
}

// Person rollback handlers

// getPersonRestorePoints handles GET /persons/{id}/restore-points
func (s *Server) getPersonRestorePoints(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid person ID")
	}

	// Verify person exists
	_, err = s.personService.GetPerson(c.Request().Context(), id)
	if err != nil {
		return err
	}

	return s.getRestorePoints(c, "Person", id)
}

// rollbackPerson handles POST /persons/{id}/rollback
func (s *Server) rollbackPerson(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid person ID")
	}

	// Verify person exists
	_, err = s.personService.GetPerson(c.Request().Context(), id)
	if err != nil {
		return err
	}

	var req RollbackRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	if req.TargetVersion < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "target_version must be a positive integer")
	}

	result, err := s.commandHandler.RollbackPerson(c.Request().Context(), id, req.TargetVersion)
	if err != nil {
		return s.handleRollbackError(err)
	}

	return c.JSON(http.StatusOK, convertRollbackResultToResponse(result, "Person rolled back successfully"))
}

// Family rollback handlers

// getFamilyRestorePoints handles GET /families/{id}/restore-points
func (s *Server) getFamilyRestorePoints(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid family ID")
	}

	// Verify family exists
	_, err = s.familyService.GetFamily(c.Request().Context(), id)
	if err != nil {
		return err
	}

	return s.getRestorePoints(c, "Family", id)
}

// rollbackFamily handles POST /families/{id}/rollback
func (s *Server) rollbackFamily(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid family ID")
	}

	// Verify family exists
	_, err = s.familyService.GetFamily(c.Request().Context(), id)
	if err != nil {
		return err
	}

	var req RollbackRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	if req.TargetVersion < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "target_version must be a positive integer")
	}

	result, err := s.commandHandler.RollbackFamily(c.Request().Context(), id, req.TargetVersion)
	if err != nil {
		return s.handleRollbackError(err)
	}

	return c.JSON(http.StatusOK, convertRollbackResultToResponse(result, "Family rolled back successfully"))
}

// Source rollback handlers

// getSourceRestorePoints handles GET /sources/{id}/restore-points
func (s *Server) getSourceRestorePoints(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid source ID")
	}

	// Verify source exists
	_, err = s.sourceService.GetSource(c.Request().Context(), id)
	if err != nil {
		return err
	}

	return s.getRestorePoints(c, "Source", id)
}

// rollbackSource handles POST /sources/{id}/rollback
func (s *Server) rollbackSource(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid source ID")
	}

	// Verify source exists
	_, err = s.sourceService.GetSource(c.Request().Context(), id)
	if err != nil {
		return err
	}

	var req RollbackRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	if req.TargetVersion < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "target_version must be a positive integer")
	}

	result, err := s.commandHandler.RollbackSource(c.Request().Context(), id, req.TargetVersion)
	if err != nil {
		return s.handleRollbackError(err)
	}

	return c.JSON(http.StatusOK, convertRollbackResultToResponse(result, "Source rolled back successfully"))
}

// Citation rollback handlers

// getCitationRestorePoints handles GET /citations/{id}/restore-points
func (s *Server) getCitationRestorePoints(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid citation ID")
	}

	// Verify citation exists
	_, err = s.sourceService.GetCitation(c.Request().Context(), id)
	if err != nil {
		return err
	}

	return s.getRestorePoints(c, "Citation", id)
}

// rollbackCitation handles POST /citations/{id}/rollback
func (s *Server) rollbackCitation(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid citation ID")
	}

	// Verify citation exists
	_, err = s.sourceService.GetCitation(c.Request().Context(), id)
	if err != nil {
		return err
	}

	var req RollbackRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	if req.TargetVersion < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "target_version must be a positive integer")
	}

	result, err := s.commandHandler.RollbackCitation(c.Request().Context(), id, req.TargetVersion)
	if err != nil {
		return s.handleRollbackError(err)
	}

	return c.JSON(http.StatusOK, convertRollbackResultToResponse(result, "Citation rolled back successfully"))
}

// Helper functions

// getRestorePoints is a generic helper for fetching restore points.
func (s *Server) getRestorePoints(c echo.Context, entityType string, entityID uuid.UUID) error {
	// Parse pagination parameters
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))

	// Apply defaults
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	result, err := s.rollbackService.GetRestorePoints(c.Request().Context(), entityType, entityID, limit, offset)
	if err != nil {
		if errors.Is(err, query.ErrNoEvents) {
			return echo.NewHTTPError(http.StatusNotFound, "No history found for this entity")
		}
		return err
	}

	response := RestorePointsResponse{
		Items:   make([]RestorePointResponse, len(result.RestorePoints)),
		Total:   result.TotalCount,
		HasMore: result.HasMore,
	}

	for i, rp := range result.RestorePoints {
		response.Items[i] = RestorePointResponse{
			Version:   rp.Version,
			Timestamp: rp.Timestamp.Format(time.RFC3339),
			Action:    rp.Action,
			Summary:   rp.Summary,
		}
	}

	return c.JSON(http.StatusOK, response)
}

// handleRollbackError converts rollback errors to appropriate HTTP responses.
func (s *Server) handleRollbackError(err error) error {
	switch {
	case errors.Is(err, command.ErrRollbackInvalidVersion):
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid target version: must be positive and less than current version")
	case errors.Is(err, command.ErrRollbackDeletedEntity):
		return echo.NewHTTPError(http.StatusConflict, "Cannot rollback a deleted entity")
	case errors.Is(err, command.ErrRollbackNoChanges):
		return echo.NewHTTPError(http.StatusBadRequest, "Target version matches current version, no rollback needed")
	case errors.Is(err, query.ErrNoEvents):
		return echo.NewHTTPError(http.StatusNotFound, "No history found for this entity")
	case errors.Is(err, query.ErrInvalidVersion):
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid version specified")
	default:
		return err
	}
}

// convertRollbackResultToResponse converts a command.RollbackResult to an API response.
func convertRollbackResultToResponse(result *command.RollbackResult, message string) RollbackResponse {
	return RollbackResponse{
		EntityID:   result.EntityID.String(),
		EntityType: result.EntityType,
		NewVersion: result.NewVersion,
		Changes:    result.Changes,
		Message:    message,
	}
}
