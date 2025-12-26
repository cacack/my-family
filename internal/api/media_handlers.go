package api

import (
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Media request/response types

type MediaResponse struct {
	ID           string    `json:"id"`
	EntityType   string    `json:"entity_type"`
	EntityID     string    `json:"entity_id"`
	Title        string    `json:"title"`
	Description  string    `json:"description,omitempty"`
	MimeType     string    `json:"mime_type"`
	MediaType    string    `json:"media_type,omitempty"`
	Filename     string    `json:"filename"`
	FileSize     int64     `json:"file_size"`
	HasThumbnail bool      `json:"has_thumbnail"`
	CropLeft     *int      `json:"crop_left,omitempty"`
	CropTop      *int      `json:"crop_top,omitempty"`
	CropWidth    *int      `json:"crop_width,omitempty"`
	CropHeight   *int      `json:"crop_height,omitempty"`
	Version      int64     `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type MediaListResponse struct {
	Items []MediaResponse `json:"items"`
	Total int             `json:"total"`
}

type UpdateMediaRequest struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	MediaType   *string `json:"media_type,omitempty"`
	CropLeft    *int    `json:"crop_left,omitempty"`
	CropTop     *int    `json:"crop_top,omitempty"`
	CropWidth   *int    `json:"crop_width,omitempty"`
	CropHeight  *int    `json:"crop_height,omitempty"`
	Version     int64   `json:"version" validate:"required"`
}

// listPersonMedia handles GET /persons/:id/media
func (s *Server) listPersonMedia(c echo.Context) error {
	personID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid person ID")
	}

	// Verify person exists
	person, err := s.readStore.GetPerson(c.Request().Context(), personID)
	if err != nil {
		return err
	}
	if person == nil {
		return echo.NewHTTPError(http.StatusNotFound, "person not found")
	}

	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	items, total, err := s.readStore.ListMediaForEntity(c.Request().Context(), "person", personID, repository.ListOptions{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return err
	}

	response := MediaListResponse{
		Items: make([]MediaResponse, len(items)),
		Total: total,
	}

	for i, m := range items {
		response.Items[i] = convertMediaToResponse(m)
	}

	return c.JSON(http.StatusOK, response)
}

// uploadPersonMedia handles POST /persons/:id/media
func (s *Server) uploadPersonMedia(c echo.Context) error {
	personID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid person ID")
	}

	// Verify person exists
	person, err := s.readStore.GetPerson(c.Request().Context(), personID)
	if err != nil {
		return err
	}
	if person == nil {
		return echo.NewHTTPError(http.StatusNotFound, "person not found")
	}

	// Get form values
	title := c.FormValue("title")
	if title == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "title is required")
	}

	description := c.FormValue("description")
	mediaType := c.FormValue("media_type")

	// Get file
	file, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "file is required")
	}

	// Check file size (10MB max)
	if file.Size > domain.MaxMediaFileSize {
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "file too large (max 10MB)")
	}

	// Read file data
	src, err := file.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to open file")
	}
	defer src.Close()

	fileData, err := io.ReadAll(src)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read file")
	}

	// Upload media
	result, err := s.commandHandler.UploadMedia(c.Request().Context(), command.UploadMediaInput{
		EntityType:  "person",
		EntityID:    personID,
		Title:       title,
		Description: description,
		MediaType:   mediaType,
		Filename:    file.Filename,
		FileData:    fileData,
	})
	if err != nil {
		return err
	}

	// Get the created media for response
	media, err := s.readStore.GetMedia(c.Request().Context(), result.ID)
	if err != nil {
		return err
	}
	if media == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to retrieve created media")
	}

	return c.JSON(http.StatusCreated, convertMediaToResponse(*media))
}

// getMedia handles GET /media/:id
func (s *Server) getMedia(c echo.Context) error {
	mediaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid media ID")
	}

	media, err := s.readStore.GetMedia(c.Request().Context(), mediaID)
	if err != nil {
		return err
	}
	if media == nil {
		return echo.NewHTTPError(http.StatusNotFound, "media not found")
	}

	return c.JSON(http.StatusOK, convertMediaToResponse(*media))
}

// updateMedia handles PUT /media/:id
func (s *Server) updateMedia(c echo.Context) error {
	mediaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid media ID")
	}

	var req UpdateMediaRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	result, err := s.commandHandler.UpdateMedia(c.Request().Context(), command.UpdateMediaInput{
		ID:          mediaID,
		Title:       req.Title,
		Description: req.Description,
		MediaType:   req.MediaType,
		CropLeft:    req.CropLeft,
		CropTop:     req.CropTop,
		CropWidth:   req.CropWidth,
		CropHeight:  req.CropHeight,
		Version:     req.Version,
	})
	if err != nil {
		return err
	}

	// Get updated media for response
	media, err := s.readStore.GetMedia(c.Request().Context(), mediaID)
	if err != nil {
		return err
	}
	if media == nil {
		return echo.NewHTTPError(http.StatusNotFound, "media not found")
	}

	media.Version = result.Version
	return c.JSON(http.StatusOK, convertMediaToResponse(*media))
}

// deleteMedia handles DELETE /media/:id
func (s *Server) deleteMedia(c echo.Context) error {
	mediaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid media ID")
	}

	version, err := strconv.ParseInt(c.QueryParam("version"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "version parameter is required")
	}

	if err := s.commandHandler.DeleteMedia(c.Request().Context(), mediaID, version, "user request"); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

// downloadMedia handles GET /media/:id/content
func (s *Server) downloadMedia(c echo.Context) error {
	mediaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid media ID")
	}

	media, err := s.readStore.GetMediaWithData(c.Request().Context(), mediaID)
	if err != nil {
		return err
	}
	if media == nil {
		return echo.NewHTTPError(http.StatusNotFound, "media not found")
	}

	// Set appropriate headers
	c.Response().Header().Set("Content-Type", media.MimeType)
	c.Response().Header().Set("Content-Disposition", "inline; filename=\""+media.Filename+"\"")
	c.Response().Header().Set("Content-Length", strconv.FormatInt(media.FileSize, 10))

	return c.Blob(http.StatusOK, media.MimeType, media.FileData)
}

// getMediaThumbnail handles GET /media/:id/thumbnail
func (s *Server) getMediaThumbnail(c echo.Context) error {
	mediaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid media ID")
	}

	thumbnail, err := s.readStore.GetMediaThumbnail(c.Request().Context(), mediaID)
	if err != nil {
		return err
	}
	if thumbnail == nil || len(thumbnail) == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "thumbnail not found")
	}

	// Thumbnails are stored as JPEG
	return c.Blob(http.StatusOK, "image/jpeg", thumbnail)
}

// convertMediaToResponse converts a MediaReadModel to a MediaResponse.
func convertMediaToResponse(m repository.MediaReadModel) MediaResponse {
	return MediaResponse{
		ID:           m.ID.String(),
		EntityType:   m.EntityType,
		EntityID:     m.EntityID.String(),
		Title:        m.Title,
		Description:  m.Description,
		MimeType:     m.MimeType,
		MediaType:    string(m.MediaType),
		Filename:     m.Filename,
		FileSize:     m.FileSize,
		HasThumbnail: len(m.ThumbnailData) > 0,
		CropLeft:     m.CropLeft,
		CropTop:      m.CropTop,
		CropWidth:    m.CropWidth,
		CropHeight:   m.CropHeight,
		Version:      m.Version,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}
