package repository

import "testing"

func TestNormalizePlace(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"England", "England"},
		{"London, England", "London,England"},
		{"London,England", "London,England"},
		{"Darmstadt,,,Germany", "Darmstadt,Germany"},
		{". James,,England", ". James,England"},
		{" St. James Park , London , England ", "St. James Park,London,England"},
		{"London, England,", "London,England"},
		{", ,London", "London"},
		{" , , ", ""},
		{"New  York, NY", "New  York,NY"},
	}
	for _, tt := range tests {
		if got := NormalizePlace(tt.in); got != tt.want {
			t.Errorf("NormalizePlace(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
