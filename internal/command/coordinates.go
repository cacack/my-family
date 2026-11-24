package command

import (
	"fmt"
	"strings"

	"github.com/cacack/gedcom-go/v3/gedcom"

	"github.com/cacack/my-family/internal/domain"
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
// A create passes SetPtr of each half. Origin is how the pair was obtained
// (#1563), stated only beside a set pair; a pair set without one is manual.
type CoordinatesPatch struct {
	Latitude  Patch[string]
	Longitude Patch[string]
	Origin    *domain.CoordinateOrigin
}

// coordinateNames name one pair's API fields, which errors report, and its
// change keys.
type coordinateNames struct {
	place, lat, long, origin   string // API fields: the place's text and the pair's
	latKey, longKey, originKey string // change keys
}

// eventPlaceNames are the names of the pair beside an event's place text
// whose API fields start with prefix: "birth_place" has the fields
// birth_place_latitude, birth_place_longitude and
// birth_place_coordinates_origin and the change keys birth_place_lat,
// birth_place_long and birth_place_origin, the created events' JSON names.
func eventPlaceNames(prefix string) coordinateNames {
	return coordinateNames{
		place: prefix, lat: prefix + "_latitude", long: prefix + "_longitude", origin: prefix + "_coordinates_origin",
		latKey: prefix + "_lat", longKey: prefix + "_long", originKey: prefix + "_origin",
	}
}

// validate checks the pair named by n: both halves left out, both cleared,
// or both set to a value on the right axis and in range, as the map reads
// them (gedcom.ParseLatitude, gedcom.ParseLongitude), for a place that has a
// name: export and the map both key coordinates by it. A stated origin needs
// a set pair and must be valid (CoordinateOrigin.Validate). It returns the
// pair with each set value in canonical GEDCOM form: trimmed, with an
// upper-case direction letter, and the origin's provider trimmed.
func (p CoordinatesPatch) validate(n coordinateNames, place string) (CoordinatesPatch, error) {
	latField, longField, originField := n.lat, n.long, n.origin
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
		if p.Origin != nil {
			return p, &InvalidCoordinateError{Field: originField, Reason: fmt.Sprintf("%s needs %s and %s", originField, latField, longField)}
		}
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
		return p, &InvalidCoordinateError{Field: n.place, Reason: fmt.Sprintf("%s and %s need a %s", latField, longField, n.place)}
	}
	valid := CoordinatesPatch{Latitude: Set(canonicalCoordinate(lat)), Longitude: Set(canonicalCoordinate(long))}
	if p.Origin != nil {
		origin := *p.Origin
		origin.Provider = strings.TrimSpace(origin.Provider)
		if err := origin.Validate(); err != nil {
			return p, &InvalidCoordinateError{Field: originField, Reason: err.Error()}
		}
		valid.Origin = &origin
	}
	return valid, nil
}

// statedOrManual is the origin of a pair being set: the one stated, or
// manual, as a pair given with none was typed in.
func statedOrManual(stated *domain.CoordinateOrigin) domain.CoordinateOrigin {
	if stated != nil {
		return *stated
	}
	return domain.CoordinateOrigin{Source: domain.CoordinateManual}
}

// canonicalCoordinate is a coordinate the gedcom parsers accepted, written as
// GEDCOM gives it: no surrounding space and an upper-case direction letter.
func canonicalCoordinate(v string) string {
	return strings.ToUpper(strings.TrimSpace(v))
}

// placeCoordinates is a place's stored coordinate pair and its origin.
type placeCoordinates struct {
	Lat, Long *string
	Origin    *domain.CoordinateOrigin
}

// recordCoordinates records the pair beside an event's place text whose API
// fields start with prefix (eventPlaceNames), for a place now named place
// (see recordPair).
func recordCoordinates(changes map[string]any, prefix, place string, input CoordinatesPatch, current placeCoordinates) error {
	return recordPair(changes, eventPlaceNames(prefix), place, input, current)
}

// recordPair validates the pair named by n for a place now named place (see
// validate) and, when either half changes, records both under n's change
// keys with its origin; nil records a clear. Recording the pair whole keeps a
// branch merge from joining one side's latitude to the other's longitude, or
// a pair to another's origin. A pair left as it is records a stated origin
// that differs, with the pair it describes, and otherwise nothing.
func recordPair(changes map[string]any, n coordinateNames, place string, input CoordinatesPatch, current placeCoordinates) error {
	input, err := input.validate(n, place)
	if err != nil {
		return err
	}
	pair := map[string]any{}
	recordOptionalChange(pair, n.latKey, input.Latitude, current.Lat)
	recordOptionalChange(pair, n.longKey, input.Longitude, current.Long)
	if len(pair) == 0 {
		if input.Origin != nil && *input.Origin != *domain.EffectiveCoordinateOrigin(current.Lat, current.Long, current.Origin) {
			changes[n.latKey], changes[n.longKey] = *current.Lat, *current.Long
			changes[n.originKey] = input.Origin.ChangeValue()
		}
		return nil
	}
	changes[n.latKey], changes[n.longKey], changes[n.originKey] = nil, nil, nil
	if lat, ok := input.Latitude.Value(); ok {
		long, _ := input.Longitude.Value()
		changes[n.latKey], changes[n.longKey] = lat, long
		changes[n.originKey] = statedOrManual(input.Origin).ChangeValue()
	}
	return nil
}

// createdCoordinates validates a create's pair for a place named place (see
// validate) and returns it as the created event's optional values.
func createdCoordinates(prefix, place string, input CoordinatesPatch) (placeCoordinates, error) {
	input, err := input.validate(eventPlaceNames(prefix), place)
	if err != nil {
		return placeCoordinates{}, err
	}
	lat, ok := input.Latitude.Value()
	if !ok {
		return placeCoordinates{}, nil
	}
	long, _ := input.Longitude.Value()
	origin := statedOrManual(input.Origin)
	return placeCoordinates{Lat: &lat, Long: &long, Origin: &origin}, nil
}
