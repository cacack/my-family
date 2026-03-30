package citation

import "github.com/cacack/my-family/internal/domain"

// registry holds all templates indexed by ID.
var registry map[string]*Template

func init() {
	registry = make(map[string]*Template, len(allTemplates))
	for i := range allTemplates {
		registry[allTemplates[i].ID] = &allTemplates[i]
	}
}

// GetTemplate returns a template by ID, or nil if not found.
func GetTemplate(id string) *Template {
	return registry[id]
}

// ListTemplates returns all registered templates.
func ListTemplates() []Template {
	return allTemplates
}

// TemplatesForSourceType returns templates applicable to the given source type.
func TemplatesForSourceType(st domain.SourceType) []Template {
	var result []Template
	for _, t := range allTemplates {
		for _, s := range t.SourceTypes {
			if s == st {
				result = append(result, t)
				break
			}
		}
	}
	return result
}

// Categories returns the distinct category names in display order.
func Categories() []string {
	seen := make(map[string]bool)
	var cats []string
	for _, t := range allTemplates {
		if !seen[t.Category] {
			seen[t.Category] = true
			cats = append(cats, t.Category)
		}
	}
	return cats
}
