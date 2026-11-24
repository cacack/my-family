package domain

import (
	"fmt"
	"slices"
	"strings"
)

// Place represents a location associated with an event (birth, death, marriage).
// It stores the place name as provided, supporting hierarchical location strings.
// Coordinates are optional and store latitude/longitude in GEDCOM format (e.g., "N42.3601", "W71.0589").
type Place struct {
	Name      string  `json:"name"`
	Latitude  *string `json:"latitude,omitempty"`
	Longitude *string `json:"longitude,omitempty"`
}

// NewPlace creates a new Place value object without coordinates.
func NewPlace(name string) Place {
	return Place{Name: name}
}

// NewPlaceWithCoordinates creates a new Place value object with coordinates.
// Coordinates should be in GEDCOM format (e.g., "N42.3601", "W71.0589").
func NewPlaceWithCoordinates(name, lat, long string) Place {
	p := Place{Name: name}
	if lat != "" {
		p.Latitude = &lat
	}
	if long != "" {
		p.Longitude = &long
	}
	return p
}

// String returns the place name.
func (p Place) String() string {
	return p.Name
}

// IsEmpty returns true if the place has no name.
func (p Place) IsEmpty() bool {
	return p.Name == ""
}

// HasCoordinates returns true if both latitude and longitude are set.
func (p Place) HasCoordinates() bool {
	return p.Latitude != nil && p.Longitude != nil && *p.Latitude != "" && *p.Longitude != ""
}

// CoordinateSource is how a coordinate pair was obtained (#1563).
type CoordinateSource string

const (
	// CoordinateImported is a pair read from a GEDCOM file. A pair stored with
	// no origin reads as imported: every pair stored before #1563 came from
	// an import.
	CoordinateImported CoordinateSource = "imported"
	// CoordinateManual is a pair the researcher typed in.
	CoordinateManual CoordinateSource = "manual"
	// CoordinateCopied is a pair copied from another event's place.
	CoordinateCopied CoordinateSource = "copied"
	// CoordinateGeocoded is a geocoder's guess from the place name.
	CoordinateGeocoded CoordinateSource = "geocoded"
)

// AllCoordinateSources returns every valid CoordinateSource value.
func AllCoordinateSources() []CoordinateSource {
	return []CoordinateSource{CoordinateImported, CoordinateManual, CoordinateCopied, CoordinateGeocoded}
}

// IsValid reports whether the value is one of AllCoordinateSources.
func (s CoordinateSource) IsValid() bool {
	return slices.Contains(AllCoordinateSources(), s)
}

// MatchPrecision is how much of a place name a geocoder matched (#1563).
type MatchPrecision string

const (
	// MatchFull is a match of the whole place name.
	MatchFull MatchPrecision = "full"
	// MatchPartial is a match of only a broader part of it, such as the
	// county of a parish the geocoder did not find.
	MatchPartial MatchPrecision = "partial"
)

// IsValid reports whether the value is MatchFull or MatchPartial.
func (m MatchPrecision) IsValid() bool {
	return m == MatchFull || m == MatchPartial
}

// CoordinateOrigin records where a coordinate pair came from (#1563): its
// source and, for a geocoded pair, the geocoder and how precise its match was.
type CoordinateOrigin struct {
	Source    CoordinateSource `json:"source"`
	Provider  string           `json:"provider,omitempty"`
	Precision MatchPrecision   `json:"precision,omitempty"`
}

// Validate checks that a geocoded origin names its provider and precision
// and that no other origin has either.
func (o CoordinateOrigin) Validate() error {
	if !o.Source.IsValid() {
		return fmt.Errorf("unknown coordinate origin %q", o.Source)
	}
	if o.Source != CoordinateGeocoded {
		if o.Provider != "" || o.Precision != "" {
			return fmt.Errorf("only a geocoded origin has a provider and precision")
		}
		return nil
	}
	if strings.TrimSpace(o.Provider) == "" {
		return fmt.Errorf("a geocoded origin needs a provider")
	}
	if !o.Precision.IsValid() {
		return fmt.Errorf("a geocoded origin needs a precision of full or partial, not %q", o.Precision)
	}
	return nil
}

// ChangeValue is the origin as a PersonUpdated or FamilyUpdated change value,
// in the JSON shape every reader of a stored event decodes.
func (o CoordinateOrigin) ChangeValue() map[string]any {
	change := map[string]any{"source": string(o.Source)}
	if o.Provider != "" {
		change["provider"] = o.Provider
	}
	if o.Precision != "" {
		change["precision"] = string(o.Precision)
	}
	return change
}

// CoordinateOriginFromChange reads a change value written by ChangeValue:
// the origin, or nil for a clear, which reads as imported. ok is false for a
// value of any other shape.
func CoordinateOriginFromChange(value any) (origin *CoordinateOrigin, ok bool) {
	switch v := value.(type) {
	case nil:
		return nil, true
	case map[string]any:
		source, _ := v["source"].(string)
		provider, _ := v["provider"].(string)
		precision, _ := v["precision"].(string)
		o := CoordinateOrigin{Source: CoordinateSource(source), Provider: provider, Precision: MatchPrecision(precision)}
		if !o.Source.IsValid() {
			return nil, false
		}
		return &o, true
	}
	return nil, false
}

// String describes the origin for a reader: "geocoded by Nominatim (partial
// match)", or the source alone.
func (o CoordinateOrigin) String() string {
	if o.Source != CoordinateGeocoded {
		return string(o.Source)
	}
	return fmt.Sprintf("geocoded by %s (%s match)", o.Provider, o.Precision)
}

// EffectiveCoordinateOrigin is the origin of the pair lat, long as read: nil
// when the pair is not set, stored when it is recorded, and imported
// otherwise (see CoordinateImported).
func EffectiveCoordinateOrigin(lat, long *string, stored *CoordinateOrigin) *CoordinateOrigin {
	if lat == nil || long == nil || *lat == "" || *long == "" {
		return nil
	}
	if stored != nil {
		return stored
	}
	return &CoordinateOrigin{Source: CoordinateImported}
}
