package domain

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
