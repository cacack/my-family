package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/repository"
)

// TestCustomErrorHandler_ConflictCodes pins the 409 split (#940): a stale
// version carries VERSION_CONFLICT; every other refusal keeps CONFLICT.
func TestCustomErrorHandler_ConflictCodes(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"stale version", repository.ErrConcurrencyConflict, CodeVersionConflict},
		{"wrapped stale version", fmt.Errorf("saving: %w", repository.ErrConcurrencyConflict), CodeVersionConflict},
		{"circular ancestry", command.ErrCircularAncestry, CodeConflict},
		{"person has families", command.ErrPersonHasFamilies, CodeConflict},
		{"child already linked", command.ErrChildAlreadyLinked, CodeConflict},
		{"echo 409", echo.NewHTTPError(http.StatusConflict, "refused"), CodeConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			rec := httptest.NewRecorder()
			c := e.NewContext(httptest.NewRequest(http.MethodPut, "/", http.NoBody), rec)

			customErrorHandler(tc.err, c)

			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, want 409", rec.Code)
			}
			var got APIError
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode %q: %v", rec.Body.String(), err)
			}
			if got.Code != tc.want {
				t.Errorf("code = %q, want %q", got.Code, tc.want)
			}
		})
	}
}
