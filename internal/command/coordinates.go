package command

import (
	"fmt"

	"github.com/cacack/gedcom-go/v3/gedcom"
)

// InvalidCoordinateError is a newly entered place coordinate that was
// rejected, naming the field (#1604). It matches ErrInvalidInput with
// errors.Is.
type InvalidCoordinateError struct {
	Field  string
	Reason string
}

func (e *InvalidCoordinateError) Error() string {
	return fmt.Sprintf("%v: %s: %s", ErrInvalidInput, e.Field, e.Reason)
}

func (e *InvalidCoordinateError) Unwrap() error { return ErrInvalidInput }

// CoordinatesPatch is the update of one place's coordinate pair, in GEDCOM
// form ("N42.3601", "W71.0589"). The two halves are set or cleared together.
// A create passes SetPtr of each half.
type CoordinatesPatch struct {
	Latitude  Patch[string]
	Longitude Patch[string]
}

// validate checks the pair for the place whose API fields start with prefix
// ("birth_place" for birth_place_latitude and birth_place_longitude): both
// halves left out, both cleared, or both set to a value on the right axis and
// in range, as the map reads them (gedcom.ParseLatitude, gedcom.ParseLongitude).
func (p CoordinatesPatch) validate(prefix string) error {
	latField, longField := prefix+"_latitude", prefix+"_longitude"
	if p.Latitude.Omitted() != p.Longitude.Omitted() || p.Latitude.Cleared() != p.Longitude.Cleared() {
		// Name the half left out, or else the one cleared beside a value.
		missing := latField
		if p.Longitude.Omitted() || (!p.Latitude.Omitted() && p.Longitude.Cleared()) {
			missing = longField
		}
		return &InvalidCoordinateError{Field: missing, Reason: fmt.Sprintf("%s and %s must be given together", latField, longField)}
	}
	if lat, ok := p.Latitude.Value(); ok {
		if _, err := gedcom.ParseLatitude(lat); err != nil {
			return &InvalidCoordinateError{Field: latField, Reason: err.Error()}
		}
	}
	if long, ok := p.Longitude.Value(); ok {
		if _, err := gedcom.ParseLongitude(long); err != nil {
			return &InvalidCoordinateError{Field: longField, Reason: err.Error()}
		}
	}
	return nil
}

// recordCoordinates validates the pair (see validate) and records each half
// that changes under prefix+"_lat" and prefix+"_long", the change keys that
// match the created events' JSON names ("birth_place_lat"); nil records a
// clear.
func recordCoordinates(changes map[string]any, prefix string, input CoordinatesPatch, currentLat, currentLong *string) error {
	if err := input.validate(prefix); err != nil {
		return err
	}
	recordOptionalChange(changes, prefix+"_lat", input.Latitude, currentLat)
	recordOptionalChange(changes, prefix+"_long", input.Longitude, currentLong)
	return nil
}

// createdCoordinates validates a create's pair (see validate) and returns it
// as the created event's optional values.
func createdCoordinates(prefix string, input CoordinatesPatch) (lat, long *string, err error) {
	if err := input.validate(prefix); err != nil {
		return nil, nil, err
	}
	if v, ok := input.Latitude.Value(); ok {
		lat = &v
	}
	if v, ok := input.Longitude.Value(); ok {
		long = &v
	}
	return lat, long, nil
}
