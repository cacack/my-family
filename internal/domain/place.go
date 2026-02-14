package domain

import (
	"fmt"
	"strconv"
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

// ParseGEDCOMCoordinate converts a GEDCOM coordinate string to decimal degrees.
// GEDCOM format uses a direction prefix: "N42.3601", "S33.8688", "W71.0589", "E151.2093".
// N/E are positive, S/W are negative. Returns an error for invalid formats.
func ParseGEDCOMCoordinate(coord string) (float64, error) {
	coord = strings.TrimSpace(coord)
	if coord == "" {
		return 0, fmt.Errorf("empty coordinate")
	}

	direction := coord[0]
	value, err := strconv.ParseFloat(coord[1:], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid coordinate %q: %w", coord, err)
	}

	switch direction {
	case 'N', 'n', 'E', 'e':
		return value, nil
	case 'S', 's', 'W', 'w':
		return -value, nil
	default:
		return 0, fmt.Errorf("invalid direction %q in coordinate %q", string(direction), coord)
	}
}
