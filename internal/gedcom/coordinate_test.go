package gedcom

import "testing"

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
