package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/cacack/my-family/internal/geocode"
)

// Geocoder looks up candidate coordinates for a place name (#1562), as
// geocode.Lookup does.
type Geocoder interface {
	Search(ctx context.Context, name string) ([]geocode.Candidate, error)
}

// WithGeocoder supplies the geocoder behind GET /geocode. It is optional and
// opt-in (GEOCODER_ENABLED): without it the lookup answers geocoder_disabled
// and nothing is sent anywhere.
func WithGeocoder(g Geocoder) ServerOption {
	return func(s *Server) {
		s.geocoder = g
	}
}

// GeocodePlace implements StrictServerInterface.
func (ss *StrictServer) GeocodePlace(ctx context.Context, request GeocodePlaceRequestObject) (GeocodePlaceResponseObject, error) {
	if ss.server.geocoder == nil {
		return GeocodePlace409JSONResponse{Code: "geocoder_disabled", Message: "No geocoder is enabled on this server (GEOCODER_ENABLED)"}, nil
	}
	candidates, err := ss.server.geocoder.Search(ctx, request.Params.Q)
	switch {
	case errors.Is(err, geocode.ErrQuery):
		return GeocodePlace400JSONResponse{BadRequestJSONResponse{Code: CodeValidation, Message: "q " + geocode.ErrQuery.Error()}}, nil
	case errors.Is(err, geocode.ErrTimeout):
		return GeocodePlace504JSONResponse{Code: "geocoder_timeout", Message: "The geocoder is busy or did not answer in time"}, nil
	case errors.Is(err, geocode.ErrProvider):
		return GeocodePlace502JSONResponse{Code: "geocoder_error", Message: "The geocoder failed"}, nil
	case err != nil:
		return nil, fmt.Errorf("geocode: %w", err)
	}
	items := make([]GeocodeCandidate, len(candidates))
	for i, c := range candidates {
		items[i] = GeocodeCandidate{
			DisplayName: c.DisplayName,
			Latitude:    c.Latitude,
			Longitude:   c.Longitude,
			Query:       c.Query,
			Provider:    c.Provider,
			Attribution: c.Attribution,
		}
	}
	return GeocodePlace200JSONResponse{Items: items, Total: len(items)}, nil
}
