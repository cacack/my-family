package command

import (
	"fmt"
	"strings"

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
// in range, as the map reads them (gedcom.ParseLatitude, gedcom.ParseLongitude),
// for a place that has a name: export and the map both key coordinates by it.
// It returns the pair with each set value in canonical GEDCOM form: trimmed,
// with an upper-case direction letter.
func (p CoordinatesPatch) validate(prefix, place string) (CoordinatesPatch, error) {
	latField, longField := prefix+"_latitude", prefix+"_longitude"
	if p.Latitude.Omitted() != p.Longitude.Omitted() || p.Latitude.Cleared() != p.Longitude.Cleared() {
		// Name the half left out, or else the one cleared beside a value.
		missing := latField
		if p.Longitude.Omitted() || (!p.Latitude.Omitted() && p.Longitude.Cleared()) {
			missing = longField
		}
		return p, &InvalidCoordinateError{Field: missing, Reason: fmt.Sprintf("%s and %s must be given together", latField, longField)}
	}
	lat, ok := p.Latitude.Value()
	if !ok {
		return p, nil
	}
	long, _ := p.Longitude.Value()
	if _, err := gedcom.ParseLatitude(lat); err != nil {
		return p, &InvalidCoordinateError{Field: latField, Reason: err.Error()}
	}
	if _, err := gedcom.ParseLongitude(long); err != nil {
		return p, &InvalidCoordinateError{Field: longField, Reason: err.Error()}
	}
	if strings.TrimSpace(place) == "" {
		return p, &InvalidCoordinateError{Field: prefix, Reason: fmt.Sprintf("%s and %s need a %s", latField, longField, prefix)}
	}
	return CoordinatesPatch{Latitude: Set(canonicalCoordinate(lat)), Longitude: Set(canonicalCoordinate(long))}, nil
}

// canonicalCoordinate is a coordinate the gedcom parsers accepted, written as
// GEDCOM gives it: no surrounding space and an upper-case direction letter.
func canonicalCoordinate(v string) string {
	return strings.ToUpper(strings.TrimSpace(v))
}

// recordCoordinates validates the pair for a place now named place (see
// validate) and, when either half changes, records both under prefix+"_lat"
// and prefix+"_long", the change keys that match the created events' JSON
// names ("birth_place_lat"); nil records a clear. Recording the pair whole
// keeps a branch merge from joining one side's latitude to the other's
// longitude.
func recordCoordinates(changes map[string]any, prefix, place string, input CoordinatesPatch, currentLat, currentLong *string) error {
	input, err := input.validate(prefix, place)
	if err != nil {
		return err
	}
	pair := map[string]any{}
	recordOptionalChange(pair, prefix+"_lat", input.Latitude, currentLat)
	recordOptionalChange(pair, prefix+"_long", input.Longitude, currentLong)
	if len(pair) == 0 {
		return nil
	}
	changes[prefix+"_lat"], changes[prefix+"_long"] = nil, nil
	if lat, ok := input.Latitude.Value(); ok {
		long, _ := input.Longitude.Value()
		changes[prefix+"_lat"], changes[prefix+"_long"] = lat, long
	}
	return nil
}

// createdCoordinates validates a create's pair for a place named place (see
// validate) and returns it as the created event's optional values.
func createdCoordinates(prefix, place string, input CoordinatesPatch) (lat, long *string, err error) {
	input, err = input.validate(prefix, place)
	if err != nil {
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
