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

// strPtr is a helper to create string pointers for tests
func strPtr(s string) *string {
	return &s
}

func TestCoordinateOrigin_Validate(t *testing.T) {
	tests := []struct {
		name    string
		origin  CoordinateOrigin
		wantErr bool
	}{
		{"imported", CoordinateOrigin{Source: CoordinateImported}, false},
		{"manual", CoordinateOrigin{Source: CoordinateManual}, false},
		{"copied", CoordinateOrigin{Source: CoordinateCopied}, false},
		{"geocoded full", CoordinateOrigin{Source: CoordinateGeocoded, Provider: "Nominatim", Precision: MatchFull}, false},
		{"geocoded partial", CoordinateOrigin{Source: CoordinateGeocoded, Provider: "Nominatim", Precision: MatchPartial}, false},
		{"unknown source", CoordinateOrigin{Source: "guessed"}, true},
		{"empty source", CoordinateOrigin{}, true},
		{"geocoded without a provider", CoordinateOrigin{Source: CoordinateGeocoded, Precision: MatchFull}, true},
		{"geocoded without a precision", CoordinateOrigin{Source: CoordinateGeocoded, Provider: "Nominatim"}, true},
		{"geocoded with an unknown precision", CoordinateOrigin{Source: CoordinateGeocoded, Provider: "Nominatim", Precision: "close"}, true},
		{"manual with a provider", CoordinateOrigin{Source: CoordinateManual, Provider: "Nominatim"}, true},
		{"copied with a precision", CoordinateOrigin{Source: CoordinateCopied, Precision: MatchFull}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.origin.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEffectiveCoordinateOrigin(t *testing.T) {
	lat, long, empty := "N1", "E1", ""
	manual := &CoordinateOrigin{Source: CoordinateManual}
	if got := EffectiveCoordinateOrigin(nil, &long, manual); got != nil {
		t.Errorf("half a pair = %v, want nil", got)
	}
	if got := EffectiveCoordinateOrigin(&empty, &long, nil); got != nil {
		t.Errorf("an empty half = %v, want nil", got)
	}
	if got := EffectiveCoordinateOrigin(&lat, &long, nil); got == nil || got.Source != CoordinateImported {
		t.Errorf("no stored origin = %v, want imported", got)
	}
	if got := EffectiveCoordinateOrigin(&lat, &long, manual); got != manual {
		t.Errorf("stored origin = %v, want it", got)
	}
}

func TestCoordinateOrigin_ChangeValueRoundTrip(t *testing.T) {
	for _, o := range []CoordinateOrigin{
		{Source: CoordinateManual},
		{Source: CoordinateGeocoded, Provider: "Nominatim", Precision: MatchPartial},
	} {
		got, ok := CoordinateOriginFromChange(o.ChangeValue())
		if !ok || got == nil || *got != o {
			t.Errorf("round trip of %v = %v, %v", o, got, ok)
		}
	}
	if got, ok := CoordinateOriginFromChange(nil); !ok || got != nil {
		t.Errorf("nil change = %v, %v; want a clear", got, ok)
	}
	for _, bad := range []any{"manual", map[string]any{"source": "guessed"}, 3} {
		if _, ok := CoordinateOriginFromChange(bad); ok {
			t.Errorf("CoordinateOriginFromChange(%v) accepted it", bad)
		}
	}
}

func TestCoordinateOrigin_String(t *testing.T) {
	if got := (CoordinateOrigin{Source: CoordinateManual}).String(); got != "manual" {
		t.Errorf("manual = %q", got)
	}
	got := (CoordinateOrigin{Source: CoordinateGeocoded, Provider: "Nominatim", Precision: MatchPartial}).String()
	if got != "geocoded by Nominatim (partial match)" {
		t.Errorf("geocoded = %q", got)
	}
}
