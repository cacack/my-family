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
		name string
		place Place
		want string
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
