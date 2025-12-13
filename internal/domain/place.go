package domain

// Place represents a location associated with an event (birth, death, marriage).
// It stores the place name as provided, supporting hierarchical location strings.
type Place struct {
	Name string `json:"name"`
}

// NewPlace creates a new Place value object.
func NewPlace(name string) Place {
	return Place{Name: name}
}

// String returns the place name.
func (p Place) String() string {
	return p.Name
}

// IsEmpty returns true if the place has no name.
func (p Place) IsEmpty() bool {
	return p.Name == ""
}
