package domain

import "testing"

func TestNewPlace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantName string
	}{
		{
			name:     "simple place name",
			input:    "London, England",
			wantName: "London, England",
		},
		{
			name:     "hierarchical place",
			input:    "Paris, Île-de-France, France",
			wantName: "Paris, Île-de-France, France",
		},
		{
			name:     "empty place",
			input:    "",
			wantName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPlace(tt.input)
			if p.Name != tt.wantName {
				t.Errorf("NewPlace(%q).Name = %q, want %q", tt.input, p.Name, tt.wantName)
			}
		})
	}
}

func TestPlace_String(t *testing.T) {
	tests := []struct {
		name  string
		place Place
		want  string
	}{
		{
			name:  "non-empty place",
			place: NewPlace("New York, NY, USA"),
			want:  "New York, NY, USA",
		},
		{
			name:  "empty place",
			place: NewPlace(""),
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.place.String(); got != tt.want {
				t.Errorf("Place.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPlace_IsEmpty(t *testing.T) {
	tests := []struct {
		name  string
		place Place
		want  bool
	}{
		{
			name:  "empty place",
			place: NewPlace(""),
			want:  true,
		},
		{
			name:  "non-empty place",
			place: NewPlace("Boston, MA"),
			want:  false,
		},
		{
			name:  "whitespace only (not considered empty)",
			place: NewPlace("   "),
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.place.IsEmpty(); got != tt.want {
				t.Errorf("Place.IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewPlaceWithCoordinates(t *testing.T) {
	tests := []struct {
		name      string
		placeName string
		lat       string
		long      string
		wantLat   *string
		wantLong  *string
	}{
		{
			name:      "place with both coordinates",
			placeName: "Boston, MA, USA",
			lat:       "N42.3601",
			long:      "W71.0589",
			wantLat:   strPtr("N42.3601"),
			wantLong:  strPtr("W71.0589"),
		},
		{
			name:      "place with only latitude",
			placeName: "Test Place",
			lat:       "N40.7128",
			long:      "",
			wantLat:   strPtr("N40.7128"),
			wantLong:  nil,
		},
		{
			name:      "place with only longitude",
			placeName: "Test Place",
			lat:       "",
			long:      "W74.0060",
			wantLat:   nil,
			wantLong:  strPtr("W74.0060"),
		},
		{
			name:      "place with no coordinates",
			placeName: "Unknown Place",
			lat:       "",
			long:      "",
			wantLat:   nil,
			wantLong:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPlaceWithCoordinates(tt.placeName, tt.lat, tt.long)

			if p.Name != tt.placeName {
				t.Errorf("NewPlaceWithCoordinates().Name = %q, want %q", p.Name, tt.placeName)
			}

			if tt.wantLat == nil {
				if p.Latitude != nil {
					t.Errorf("NewPlaceWithCoordinates().Latitude = %q, want nil", *p.Latitude)
				}
			} else {
				if p.Latitude == nil {
					t.Errorf("NewPlaceWithCoordinates().Latitude = nil, want %q", *tt.wantLat)
				} else if *p.Latitude != *tt.wantLat {
					t.Errorf("NewPlaceWithCoordinates().Latitude = %q, want %q", *p.Latitude, *tt.wantLat)
				}
			}

			if tt.wantLong == nil {
				if p.Longitude != nil {
					t.Errorf("NewPlaceWithCoordinates().Longitude = %q, want nil", *p.Longitude)
				}
			} else {
				if p.Longitude == nil {
					t.Errorf("NewPlaceWithCoordinates().Longitude = nil, want %q", *tt.wantLong)
				} else if *p.Longitude != *tt.wantLong {
					t.Errorf("NewPlaceWithCoordinates().Longitude = %q, want %q", *p.Longitude, *tt.wantLong)
				}
			}
		})
	}
}

func TestPlace_HasCoordinates(t *testing.T) {
	tests := []struct {
		name  string
		place Place
		want  bool
	}{
		{
			name:  "place with both coordinates",
			place: NewPlaceWithCoordinates("Boston, MA", "N42.3601", "W71.0589"),
			want:  true,
		},
		{
			name:  "place with only latitude",
			place: NewPlaceWithCoordinates("Test", "N42.3601", ""),
			want:  false,
		},
		{
			name:  "place with only longitude",
			place: NewPlaceWithCoordinates("Test", "", "W71.0589"),
			want:  false,
		},
		{
			name:  "place without coordinates",
			place: NewPlace("Unknown Place"),
			want:  false,
		},
		{
			name:  "place with empty string coordinates",
			place: NewPlaceWithCoordinates("Test", "", ""),
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.place.HasCoordinates(); got != tt.want {
				t.Errorf("Place.HasCoordinates() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseGEDCOMCoordinate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    float64
		wantErr bool
	}{
		{name: "north latitude", input: "N42.3601", want: 42.3601},
		{name: "south latitude", input: "S33.8688", want: -33.8688},
		{name: "west longitude", input: "W71.0589", want: -71.0589},
		{name: "east longitude", input: "E151.2093", want: 151.2093},
		{name: "lowercase north", input: "n42.3601", want: 42.3601},
		{name: "lowercase south", input: "s33.8688", want: -33.8688},
		{name: "lowercase west", input: "w71.0589", want: -71.0589},
		{name: "lowercase east", input: "e151.2093", want: 151.2093},
		{name: "zero north", input: "N0.0", want: 0.0},
		{name: "zero south", input: "S0.0", want: 0.0},
		{name: "empty string", input: "", wantErr: true},
		{name: "whitespace only", input: "   ", wantErr: true},
		{name: "invalid direction", input: "X42.3601", wantErr: true},
		{name: "no number", input: "N", wantErr: true},
		{name: "invalid number", input: "Nabc", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseGEDCOMCoordinate(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseGEDCOMCoordinate(%q) = %v, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseGEDCOMCoordinate(%q) unexpected error: %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("ParseGEDCOMCoordinate(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// strPtr is a helper to create string pointers for tests
func strPtr(s string) *string {
	return &s
}
