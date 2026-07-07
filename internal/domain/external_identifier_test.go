package domain

import "testing"

func TestExternalIdentifierLabel(t *testing.T) {
	tests := []struct {
		name string
		typ  string
		want string
	}{
		{"familysearch ark", "http://www.familysearch.org/ark", "FamilySearch"},
		{"findagrave", "https://www.findagrave.com/", "Find a Grave"},
		{"ancestry", "https://www.ancestry.com/", "Ancestry"},
		{"wikitree", "https://www.wikitree.com/", "WikiTree"},
		{"geni", "https://www.geni.com/", "Geni"},
		{"case insensitive", "HTTPS://WWW.FINDAGRAVE.COM/", "Find a Grave"},
		{"unknown falls back to type", "https://example.com/custom", "https://example.com/custom"},
		{"empty type falls back to generic label", "", "External ID"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := ExternalIdentifier{Value: "X1", Type: tt.typ}
			if got := e.Label(); got != tt.want {
				t.Errorf("Label() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExternalIdentifierURL(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		typ     string
		wantURL string
		wantOK  bool
	}{
		{"familysearch", "KWCJ-QN7", "http://www.familysearch.org/ark", "https://www.familysearch.org/tree/person/details/KWCJ-QN7", true},
		{"findagrave", "12345", "https://www.findagrave.com/", "https://www.findagrave.com/memorial/12345", true},
		{"unknown type", "abc", "https://example.com/custom", "", false},
		{"empty value", "", "http://www.familysearch.org/ark", "", false},
		// The identifier value is attacker-controlled (verbatim from GEDCOM), so
		// structural URL characters must be escaped for the context they land in.
		{"path value escapes slash and fragment", "../../../evil#x", "http://www.familysearch.org/ark", "https://www.familysearch.org/tree/person/details/..%2F..%2F..%2Fevil%23x", true},
		{"query value escapes ampersand", "X1&redirect=evil.com", "https://www.ancestry.com/", "https://www.ancestry.com/search/?pid=X1%26redirect%3Devil.com", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := ExternalIdentifier{Value: tt.value, Type: tt.typ}
			gotURL, gotOK := e.URL()
			if gotURL != tt.wantURL || gotOK != tt.wantOK {
				t.Errorf("URL() = (%q, %v), want (%q, %v)", gotURL, gotOK, tt.wantURL, tt.wantOK)
			}
		})
	}
}
