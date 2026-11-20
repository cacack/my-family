package api

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/cacack/my-family/internal/repository"
)

// WithTreeSettings supplies the store of the tree's privacy settings
// (ADR-008 §6). It is optional: without it the defaults apply and cannot be
// changed.
func WithTreeSettings(store repository.TreeSettingsStore) ServerOption {
	return func(s *Server) {
		s.settings = store
	}
}

// treeSettings returns the tree's saved settings, or the defaults when the
// server has no settings store.
func (s *Server) treeSettings(ctx context.Context) (repository.TreeSettings, error) {
	if s.settings == nil {
		return repository.DefaultTreeSettings(), nil
	}
	return s.settings.GetTreeSettings(ctx, repository.DefaultTreeID)
}

func privacySettingsOf(settings repository.TreeSettings) PrivacySettings {
	return PrivacySettings{
		LivingThresholdYears: settings.LivingThresholdYears,
		ViewersSeeLiving:     settings.ViewersSeeLiving,
		AnonymousAccess:      AnonymousAccess(settings.AnonymousAccess),
	}
}

// GetPrivacySettings implements StrictServerInterface.
func (ss *StrictServer) GetPrivacySettings(ctx context.Context, _ GetPrivacySettingsRequestObject) (GetPrivacySettingsResponseObject, error) {
	settings, err := ss.server.treeSettings(ctx)
	if err != nil {
		return nil, err
	}
	return GetPrivacySettings200JSONResponse(privacySettingsOf(settings)), nil
}

// UpdatePrivacySettings implements StrictServerInterface.
func (ss *StrictServer) UpdatePrivacySettings(ctx context.Context, request UpdatePrivacySettingsRequestObject) (UpdatePrivacySettingsResponseObject, error) {
	if ss.server.settings == nil {
		return nil, echo.NewHTTPError(http.StatusServiceUnavailable, "This server keeps no settings")
	}
	settings := repository.TreeSettings{
		LivingThresholdYears: request.Body.LivingThresholdYears,
		ViewersSeeLiving:     request.Body.ViewersSeeLiving,
		AnonymousAccess:      repository.AnonymousAccess(request.Body.AnonymousAccess),
	}
	if err := settings.Validate(); err != nil {
		return UpdatePrivacySettings400JSONResponse{BadRequestJSONResponse(Error{Code: CodeValidation, Message: err.Error()})}, nil
	}
	previous, err := ss.server.settings.GetTreeSettings(ctx, repository.DefaultTreeID)
	if err != nil {
		return nil, err
	}
	if err := ss.server.settings.SaveTreeSettings(ctx, repository.DefaultTreeID, settings); err != nil {
		return nil, err
	}
	// Who may see the tree without signing in is an access change, so it is
	// audited with the other access changes (ADR-007). Like theirs, a failed
	// audit write is logged and leaves the saved change in place, so closing
	// the tree to visitors never waits on the log.
	if settings.AnonymousAccess != previous.AnonymousAccess && ss.server.auth != nil {
		ss.server.auth.AuditAnonymousAccess(ctx, actorFrom(ctx), settings.AnonymousAccess)
	}
	return UpdatePrivacySettings200JSONResponse(privacySettingsOf(settings)), nil
}
