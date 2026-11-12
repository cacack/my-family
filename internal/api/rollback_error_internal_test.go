package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/query"
)

// TestHandleRollbackErrorStrict_NoEventsIsNotFound pins the status a rollback
// against an entity with no events maps to: 404, not 400 (issue #683).
func TestHandleRollbackErrorStrict_NoEventsIsNotFound(t *testing.T) {
	got, err := handleRollbackErrorStrict[string](query.ErrNoEvents,
		func(Error) string { return "400" },
		func(Error) string { return "404" },
		func(Error) string { return "409" },
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "404" {
		t.Errorf("query.ErrNoEvents mapped to %s, want 404", got)
	}
}

// TestHandleRollbackErrorStrict_DeletedEntity pins the deleted-entity refusal
// to 409 CONFLICT (#989). The handlers 404 a deleted entity before the command
// runs, so this refusal is reached only through a race and is tested here.
func TestHandleRollbackErrorStrict_DeletedEntity(t *testing.T) {
	resp, err := handleRollbackErrorStrict[RollbackPersonResponseObject](command.ErrRollbackDeletedEntity,
		func(e Error) RollbackPersonResponseObject { return RollbackPerson400JSONResponse{BadRequestJSONResponse(e)} },
		func(e Error) RollbackPersonResponseObject { return RollbackPerson404JSONResponse{NotFoundJSONResponse(e)} },
		func(e Error) RollbackPersonResponseObject { return RollbackPerson409JSONResponse(e) },
	)
	if err != nil {
		t.Fatalf("handleRollbackErrorStrict: %v", err)
	}

	rec := httptest.NewRecorder()
	if err := resp.VisitRollbackPersonResponse(rec); err != nil {
		t.Fatalf("VisitRollbackPersonResponse: %v", err)
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	var body Error
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding body %q: %v", rec.Body.String(), err)
	}
	if body.Code != CodeConflict {
		t.Errorf("code = %q, want %q", body.Code, CodeConflict)
	}
}
