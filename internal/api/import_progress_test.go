package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository/memory"
)

func setupStreamTestServer(t *testing.T) *api.Server {
	t.Helper()
	cfg := &config.Config{Port: 8080, LogFormat: "text"}
	eventStore := memory.NewEventStore()
	snapshotStore := memory.NewSnapshotStore(eventStore)
	readStore := memory.NewReadModelStore()
	return api.NewServer(cfg, eventStore, readStore, snapshotStore, nil)
}

// sseEvent is one parsed Server-Sent Event.
type sseEvent struct {
	name string
	data string
}

// parseSSE splits a Server-Sent Events response body into discrete events.
func parseSSE(body string) []sseEvent {
	var events []sseEvent
	for _, block := range strings.Split(body, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		var ev sseEvent
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event:"):
				ev.name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				ev.data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			}
		}
		events = append(events, ev)
	}
	return events
}

func TestImportGedcomStream_Success(t *testing.T) {
	server := setupStreamTestServer(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "test.ged")
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(part, testGedcom)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import/stream", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Expected text/event-stream content type, got %q", ct)
	}

	events := parseSSE(rec.Body.String())
	if len(events) == 0 {
		t.Fatal("No SSE events received")
	}

	// The terminal event must be a "result" carrying the import counts.
	last := events[len(events)-1]
	if last.name != "result" {
		t.Fatalf("Expected final event 'result', got %q (body: %s)", last.name, rec.Body.String())
	}

	var result struct {
		Success          bool `json:"success"`
		PersonsImported  int  `json:"persons_imported"`
		FamiliesImported int  `json:"families_imported"`
	}
	if err := json.Unmarshal([]byte(last.data), &result); err != nil {
		t.Fatalf("Failed to parse result event: %v", err)
	}
	if !result.Success {
		t.Error("result.success should be true")
	}
	if result.PersonsImported != 3 {
		t.Errorf("persons_imported = %d, want 3", result.PersonsImported)
	}
	if result.FamiliesImported != 1 {
		t.Errorf("families_imported = %d, want 1", result.FamiliesImported)
	}

	// Any progress events that were emitted must carry sane percentages.
	for _, ev := range events {
		if ev.name != "progress" {
			continue
		}
		var p struct {
			BytesRead  int64 `json:"bytes_read"`
			TotalBytes int64 `json:"total_bytes"`
			Percent    int   `json:"percent"`
		}
		if err := json.Unmarshal([]byte(ev.data), &p); err != nil {
			t.Fatalf("Failed to parse progress event: %v", err)
		}
		if p.Percent < 0 || p.Percent > 100 {
			t.Errorf("progress percent out of range: %d", p.Percent)
		}
		if p.TotalBytes <= 0 {
			t.Errorf("progress total_bytes should be known (>0), got %d", p.TotalBytes)
		}
	}
}

func TestImportGedcomStream_NoFile(t *testing.T) {
	server := setupStreamTestServer(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import/stream", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestImportGedcomStream_ImportError(t *testing.T) {
	server := setupStreamTestServer(t)

	// An empty GEDCOM (header + trailer, no records) fails import validation. Once
	// the SSE stream has begun (HTTP 200 + headers written), such failures are
	// surfaced as a terminal "error" event rather than an HTTP error status.
	emptyGedcom := "0 HEAD\n1 GEDC\n2 VERS 5.5\n1 CHAR UTF-8\n0 TRLR\n"

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "empty.ged")
	io.WriteString(part, emptyGedcom)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/gedcom/import/stream", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200 (errors reported in-stream), got %d: %s", rec.Code, rec.Body.String())
	}
	events := parseSSE(rec.Body.String())
	if len(events) == 0 {
		t.Fatal("Expected at least one SSE event")
	}
	last := events[len(events)-1]
	if last.name != "error" {
		t.Errorf("Expected terminal 'error' event for failed import, got %q (body: %s)", last.name, rec.Body.String())
	}
}
