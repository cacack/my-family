package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/gedcom"
)

// importProgressEvent is the payload for a Server-Sent Events "progress" event
// emitted while a GEDCOM file is being parsed.
type importProgressEvent struct {
	BytesRead  int64 `json:"bytes_read"`
	TotalBytes int64 `json:"total_bytes"` // -1 when unknown
	// Percent is the completion percentage (0-100), or -1 when the total size is
	// unknown. Provided for convenience so clients need not compute it.
	Percent int `json:"percent"`
}

// importStreamResult is the payload for the terminal SSE "result" event,
// mirroring the JSON shape returned by the standard /gedcom/import endpoint.
type importStreamResult struct {
	Success          bool            `json:"success"`
	PersonsImported  int             `json:"persons_imported"`
	FamiliesImported int             `json:"families_imported"`
	Warnings         []importMessage `json:"warnings,omitempty"`
	Errors           []importMessage `json:"errors,omitempty"`
}

type importMessage struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// registerImportProgressRoutes wires the streaming GEDCOM import endpoint. It is
// registered outside the generated handler because Server-Sent Events do not map
// cleanly onto the OpenAPI-generated strict server.
func (s *Server) registerImportProgressRoutes(api *echo.Group) {
	api.POST("/gedcom/import/stream", s.importGedcomStream)
}

// importGedcomStream imports a GEDCOM file while streaming parse progress to the
// client via Server-Sent Events (SSE). It emits zero or more "progress" events
// followed by exactly one terminal event: "result" on success or "error" on
// failure.
//
// The upload is buffered first so the total size is known up front, allowing the
// progress events to report a meaningful percentage. This matches the existing
// import behaviour, which already materialises the full document in memory.
func (s *Server) importGedcomStream(c echo.Context) error {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return s.sseError(c, http.StatusBadRequest, "No file uploaded")
	}

	src, err := fileHeader.Open()
	if err != nil {
		return s.sseError(c, http.StatusBadRequest, "Failed to read uploaded file")
	}
	defer func() { _ = src.Close() }()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, src); err != nil {
		return s.sseError(c, http.StatusBadRequest, "Failed to read uploaded file")
	}
	totalSize := int64(buf.Len())

	// Set up the SSE response stream.
	resp := c.Response()
	resp.Header().Set(echo.HeaderContentType, "text/event-stream")
	resp.Header().Set("Cache-Control", "no-cache")
	resp.Header().Set("Connection", "keep-alive")
	resp.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering (e.g. nginx)
	resp.WriteHeader(http.StatusOK)

	flusher, canFlush := resp.Writer.(http.Flusher)

	// Throttle progress events so we do not flood the client: emit at most once
	// per whole-percent change (and always the first read).
	lastPercent := -1
	emitProgress := func(bytesRead, totalBytes int64) {
		percent := -1
		if totalBytes > 0 {
			percent = int(bytesRead * 100 / totalBytes)
		}
		if percent == lastPercent {
			return
		}
		lastPercent = percent
		_ = s.writeSSE(c, "progress", importProgressEvent{
			BytesRead:  bytesRead,
			TotalBytes: totalBytes,
			Percent:    percent,
		})
		if canFlush {
			flusher.Flush()
		}
	}

	result, err := s.commandHandler.ImportGedcom(c.Request().Context(), command.ImportGedcomInput{
		Filename:   fileHeader.Filename,
		FileSize:   totalSize,
		Reader:     &buf,
		OnProgress: gedcom.ImportProgressCallback(emitProgress),
	})
	if err != nil {
		_ = s.writeSSE(c, "error", map[string]string{"message": err.Error()})
		if canFlush {
			flusher.Flush()
		}
		return nil
	}

	payload := importStreamResult{
		Success:          true,
		PersonsImported:  result.PersonsImported,
		FamiliesImported: result.FamiliesImported,
	}
	for _, w := range result.Warnings {
		payload.Warnings = append(payload.Warnings, importMessage{Message: w})
	}
	for _, e := range result.Errors {
		payload.Errors = append(payload.Errors, importMessage{Message: e})
	}

	_ = s.writeSSE(c, "result", payload)
	if canFlush {
		flusher.Flush()
	}
	return nil
}

// writeSSE writes a single named Server-Sent Event with a JSON-encoded payload.
func (s *Server) writeSSE(c echo.Context, event string, data any) error {
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(c.Response().Writer, "event: %s\ndata: %s\n\n", event, encoded)
	return err
}

// sseError writes a JSON error response for failures that occur before the SSE
// stream has started (e.g. a missing upload).
func (s *Server) sseError(c echo.Context, status int, message string) error {
	return c.JSON(status, map[string]string{"code": "bad_request", "message": message})
}
