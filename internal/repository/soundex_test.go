package repository

import "testing"

func TestSoundex(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Robert", "R163"},
		{"Smith", "S530"},
		{"Smyth", "S530"},
		{"", ""},
		{"A", "A000"},
		{"Catherine", "C365"},
		{"Katherine", "K365"},
		{"Johnson", "J525"},
		{"Jonson", "J525"},
		{"Williams", "W452"},
		{"Ashcraft", "A261"},
		{"123", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Soundex(tt.input)
			if got != tt.want {
				t.Errorf("Soundex(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSoundexMatch(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"Smith/Smyth match", "Smith", "Smyth", true},
		{"Johnson/Jonson match", "Johnson", "Jonson", true},
		{"Catherine/Katherine differ", "Catherine", "Katherine", false},
		{"Robert/Rupert match", "Robert", "Rupert", true},
		{"empty a", "", "Smith", false},
		{"empty b", "Smith", "", false},
		{"both empty", "", "", false},
		{"same name", "Smith", "Smith", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SoundexMatch(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("SoundexMatch(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
