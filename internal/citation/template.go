// Package citation provides Evidence Explained citation templates,
// formatting, and validation for GPS-compliant genealogical citations.
package citation

import "github.com/cacack/my-family/internal/domain"

// FieldDef defines a single field within a citation template.
type FieldDef struct {
	Key      string `json:"key"`       // Machine key, e.g. "county"
	Label    string `json:"label"`     // Human label, e.g. "County"
	HelpText string `json:"help_text"` // Guidance for the user
	Required bool   `json:"required"`  // Whether the field must be populated
}

// Template defines an Evidence Explained citation template.
type Template struct {
	ID          string              `json:"id"`           // Stable hierarchical ID, e.g. "census.us.federal"
	Name        string              `json:"name"`         // Display name
	Category    string              `json:"category"`     // Grouping category
	Description string              `json:"description"`  // Brief help text
	SourceTypes []domain.SourceType `json:"source_types"` // Applicable source types
	Fields      []FieldDef          `json:"fields"`       // Ordered field definitions
	FullFormat  string              `json:"full_format"`  // Go text/template for full citation
	ShortFormat string              `json:"short_format"` // Go text/template for short citation
}

// RequiredFields returns only the required field definitions.
func (t *Template) RequiredFields() []FieldDef {
	var result []FieldDef
	for _, f := range t.Fields {
		if f.Required {
			result = append(result, f)
		}
	}
	return result
}
