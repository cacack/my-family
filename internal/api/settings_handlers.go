package api

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

// WithSettingsStore supplies the store of site and account settings backing
// the /settings/home-person endpoints. Without it those endpoints return 503.
func WithSettingsStore(store repository.SettingsStore) ServerOption {
	return func(s *Server) {
		s.settingsStore = store
	}
}

var errSettingsUnavailable = Error{
	Code:    "settings_unavailable",
	Message: "Settings store is not configured on this server",
}

// errNoAccount is a home-person change with no account to change: an
// anonymous visitor's, which authenticate refuses before it gets here.
var errNoAccount = errors.New("an anonymous visitor has no home person")

// homePersonAccount returns whose home person a request reads and changes:
// the site's in open mode, where no one signs in, otherwise the signed-in
// user's. ok is false for an anonymous visitor, who has none (#1443).
func (ss *StrictServer) homePersonAccount(ctx context.Context) (account uuid.UUID, ok bool) {
	if ss.server.config.OpenMode() {
		return repository.SiteWide, true
	}
	if user := userFrom(ctx); user != nil {
		return user.ID, true
	}
	return uuid.Nil, false
}

// homePerson returns the home person as the API shows it. It is not tied to
// a branch, so the person is read from the mainline, through the read path
// every person read takes; one that no longer exists there reads as unset.
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
	account, ok := ss.homePersonAccount(ctx)
	if !ok {
		return GetHomePerson200JSONResponse(HomePerson{}), nil
	}
	id, err := store.HomePerson(ctx, account)
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
	account, ok := ss.homePersonAccount(ctx)
	if !ok {
		return nil, errNoAccount
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
	if err := store.SetHomePerson(ctx, account, id); err != nil {
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
	account, ok := ss.homePersonAccount(ctx)
	if !ok {
		return nil, errNoAccount
	}
	if err := store.SetHomePerson(ctx, account, uuid.Nil); err != nil {
		return nil, err
	}
	return ClearHomePerson204Response{}, nil
}

// followHomePersonMerge moves the home person to the survivor when a mainline
// merge removes them, so merging away a duplicate of "me" keeps the choice. A
// branch-scoped merge leaves main, and so the setting, untouched.
func (ss *StrictServer) followHomePersonMerge(ctx context.Context, branch *domain.Branch, mergedID, survivorID uuid.UUID) {
	if branch != nil {
		return
	}
	ss.followHomePersonMerges(ctx, map[uuid.UUID]uuid.UUID{mergedID: survivorID})
}

// followHomePersonMerges moves the home person of the site and of every
// account to their survivor when merges that have landed on main removed
// them; survivors maps each merged-away person to the person now holding
// their data. The merges have already landed, so a failure here is logged
// rather than failing the request.
func (ss *StrictServer) followHomePersonMerges(ctx context.Context, survivors map[uuid.UUID]uuid.UUID) {
	store := ss.server.settingsStore
	if store == nil || len(survivors) == 0 {
		return
	}
	if err := store.MoveHomePersons(ctx, survivors); err != nil {
		ss.server.echo.Logger.Errorf("moving the home person to its merge survivor: %v", err)
	}
}
