package api

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/cacack/my-family/internal/auth"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository/memory"
)

// TestHomePerson_NoAccountIsUnauthenticated pins the handlers' own refusal of
// a change with no signed-in account to 401, should one get past the route
// gate that refuses it first.
func TestHomePerson_NoAccountIsUnauthenticated(t *testing.T) {
	events := memory.NewEventStore()
	server := NewServer(&config.Config{AuthMode: config.AuthModeAccounts}, events, memory.NewReadModelStore(),
		memory.NewSnapshotStore(events), nil,
		WithAuth(auth.NewService(memory.NewAccountStore())), WithSettingsStore(memory.NewSettingsStore()))
	ss := NewStrictServer(server)
	ctx := context.Background()
	personID := uuid.New()

	_, setErr := ss.SetHomePerson(ctx, SetHomePersonRequestObject{Body: &SetHomePersonJSONRequestBody{PersonId: personID}})
	_, clearErr := ss.ClearHomePerson(ctx, ClearHomePersonRequestObject{})
	for name, err := range map[string]error{"set": setErr, "clear": clearErr} {
		var httpErr *echo.HTTPError
		if !errors.As(err, &httpErr) || httpErr.Code != http.StatusUnauthorized {
			t.Errorf("%s with no account: err = %v, want a 401", name, err)
		}
	}
}
