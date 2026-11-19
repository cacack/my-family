package api

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

// WithSettingsStore supplies the store of site settings backing the /settings
// endpoints. Without it those endpoints return 503.
func WithSettingsStore(store repository.SettingsStore) ServerOption {
	return func(s *Server) {
		s.settingsStore = store
	}
}

var errSettingsUnavailable = Error{
	Code:    "settings_unavailable",
	Message: "Settings store is not configured on this server",
}

// homePerson returns the home person as the API shows it. The setting is
// site-wide, so the person is read from the mainline; one that no longer exists
// there reads as unset.
func (ss *StrictServer) homePerson(ctx context.Context, id uuid.UUID) (HomePerson, error) {
	if id == uuid.Nil {
		return HomePerson{}, nil
	}
	person, err := ss.server.personService.GetPersonCore(ctx, domain.MainBranchID, id)
	if errors.Is(err, query.ErrNotFound) {
		return HomePerson{}, nil
	}
	if err != nil {
		return HomePerson{}, err
	}
	generated := convertQueryPersonToGenerated(*person)
	return HomePerson{Person: &generated}, nil
}

// GetHomePerson implements StrictServerInterface.
func (ss *StrictServer) GetHomePerson(ctx context.Context, _ GetHomePersonRequestObject) (GetHomePersonResponseObject, error) {
	store := ss.server.settingsStore
	if store == nil {
		return GetHomePerson503JSONResponse{SettingsUnavailableJSONResponse(errSettingsUnavailable)}, nil
	}
	id, err := store.HomePerson(ctx)
	if err != nil {
		return nil, err
	}
	home, err := ss.homePerson(ctx, id)
	if err != nil {
		return nil, err
	}
	return GetHomePerson200JSONResponse(home), nil
}

// SetHomePerson implements StrictServerInterface.
func (ss *StrictServer) SetHomePerson(ctx context.Context, request SetHomePersonRequestObject) (SetHomePersonResponseObject, error) {
	store := ss.server.settingsStore
	if store == nil {
		return SetHomePerson503JSONResponse{SettingsUnavailableJSONResponse(errSettingsUnavailable)}, nil
	}
	id := request.Body.PersonId
	if id == uuid.Nil {
		return SetHomePerson400JSONResponse{BadRequestJSONResponse{
			Code:    "bad_request",
			Message: "person_id is required",
		}}, nil
	}
	home, err := ss.homePerson(ctx, id)
	if err != nil {
		return nil, err
	}
	if home.Person == nil {
		return SetHomePerson404JSONResponse{NotFoundJSONResponse{
			Code:    "not_found",
			Message: "Person not found",
		}}, nil
	}
	if err := store.SetHomePerson(ctx, id); err != nil {
		return nil, err
	}
	return SetHomePerson200JSONResponse(home), nil
}

// ClearHomePerson implements StrictServerInterface.
func (ss *StrictServer) ClearHomePerson(ctx context.Context, _ ClearHomePersonRequestObject) (ClearHomePersonResponseObject, error) {
	store := ss.server.settingsStore
	if store == nil {
		return ClearHomePerson503JSONResponse{SettingsUnavailableJSONResponse(errSettingsUnavailable)}, nil
	}
	if err := store.SetHomePerson(ctx, uuid.Nil); err != nil {
		return nil, err
	}
	return ClearHomePerson204Response{}, nil
}
