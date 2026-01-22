package domain

import "strings"

// Address represents a structured GEDCOM address.
// This is an embedded struct used by LifeEvent (RESI events), Repository,
// and Submitter entities. It does not have its own ID or events.
type Address struct {
	Line1      string `json:"line1,omitempty"`       // ADR1
	Line2      string `json:"line2,omitempty"`       // ADR2
	Line3      string `json:"line3,omitempty"`       // ADR3
	City       string `json:"city,omitempty"`        // CITY
	State      string `json:"state,omitempty"`       // STAE
	PostalCode string `json:"postal_code,omitempty"` // POST
	Country    string `json:"country,omitempty"`     // CTRY
	Phone      string `json:"phone,omitempty"`       // PHON
	Email      string `json:"email,omitempty"`       // EMAIL
	Fax        string `json:"fax,omitempty"`         // FAX
	Website    string `json:"website,omitempty"`     // WWW
}

// String returns a single-line representation of the address.
// Components are joined with ", " in the order: lines, city, state, postal code, country.
func (a *Address) String() string {
	if a == nil {
		return ""
	}

	var parts []string

	// Address lines
	if a.Line1 != "" {
		parts = append(parts, a.Line1)
	}
	if a.Line2 != "" {
		parts = append(parts, a.Line2)
	}
	if a.Line3 != "" {
		parts = append(parts, a.Line3)
	}

	// City, State PostalCode format (common US format)
	cityStatePart := ""
	if a.City != "" {
		cityStatePart = a.City
	}
	if a.State != "" {
		if cityStatePart != "" {
			cityStatePart += ", " + a.State
		} else {
			cityStatePart = a.State
		}
	}
	if a.PostalCode != "" {
		if cityStatePart != "" {
			cityStatePart += " " + a.PostalCode
		} else {
			cityStatePart = a.PostalCode
		}
	}
	if cityStatePart != "" {
		parts = append(parts, cityStatePart)
	}

	// Country
	if a.Country != "" {
		parts = append(parts, a.Country)
	}

	return strings.Join(parts, ", ")
}

// IsEmpty returns true if all fields are empty.
func (a *Address) IsEmpty() bool {
	if a == nil {
		return true
	}
	return a.Line1 == "" &&
		a.Line2 == "" &&
		a.Line3 == "" &&
		a.City == "" &&
		a.State == "" &&
		a.PostalCode == "" &&
		a.Country == "" &&
		a.Phone == "" &&
		a.Email == "" &&
		a.Fax == "" &&
		a.Website == ""
}

// StreetAddress returns a combined string of the address lines.
func (a *Address) StreetAddress() string {
	if a == nil {
		return ""
	}

	var lines []string
	if a.Line1 != "" {
		lines = append(lines, a.Line1)
	}
	if a.Line2 != "" {
		lines = append(lines, a.Line2)
	}
	if a.Line3 != "" {
		lines = append(lines, a.Line3)
	}

	return strings.Join(lines, ", ")
}
