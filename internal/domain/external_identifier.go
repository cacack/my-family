// Package domain provides the core domain types for the genealogy application.
package domain

import "strings"

// ExternalIdentifier links a record to an external system using a typed URI
// identifier. It is the domain representation of the GEDCOM 7.0 EXID structure:
//
//	1 EXID 9876543210
//	  2 TYPE http://www.familysearch.org/ark
//
// The Type URI names the external system (FamilySearch, FindAGrave, etc.) and
// Value is the identifier within that system.
type ExternalIdentifier struct {
	// Value is the external identifier string (the EXID payload).
	Value string `json:"value"`

	// Type is the URI identifying the external system (the TYPE subordinate).
	// May be empty when a source file omits it.
	Type string `json:"type,omitempty"`
}

// knownExternalIDLinks maps well-known EXID type URIs to a human-readable label
// and a URL template. The "%s" placeholder is replaced with the identifier value.
// Templates are intentionally conservative: only systems with a stable, public
// record-URL scheme are included so the UI can render a working "View on ..." link.
var knownExternalIDLinks = []struct {
	// typeMatch is matched case-insensitively against the type URI. A URI matches
	// when it contains this substring, tolerating http/https and trailing paths.
	typeMatch   string
	label       string
	urlTemplate string
}{
	{"familysearch.org/ark", "FamilySearch", "https://www.familysearch.org/tree/person/details/%s"},
	{"findagrave.com", "Find a Grave", "https://www.findagrave.com/memorial/%s"},
	{"ancestry.com", "Ancestry", "https://www.ancestry.com/search/?pid=%s"},
	{"wikitree.com", "WikiTree", "https://www.wikitree.com/wiki/%s"},
	{"geni.com", "Geni", "https://www.geni.com/people/id/%s"},
}

// Label returns a human-readable name for the external system identified by the
// Type URI, or the raw Type when the system is not recognized.
func (e ExternalIdentifier) Label() string {
	t := strings.ToLower(e.Type)
	for _, k := range knownExternalIDLinks {
		if strings.Contains(t, k.typeMatch) {
			return k.label
		}
	}
	return e.Type
}

// URL returns a browsable URL for this identifier when its Type URI maps to a
// known external system, along with ok=true. For unrecognized systems it returns
// ("", false) so callers can decide how to present the raw value.
func (e ExternalIdentifier) URL() (string, bool) {
	if e.Value == "" {
		return "", false
	}
	t := strings.ToLower(e.Type)
	for _, k := range knownExternalIDLinks {
		if strings.Contains(t, k.typeMatch) {
			return strings.Replace(k.urlTemplate, "%s", e.Value, 1), true
		}
	}
	return "", false
}
