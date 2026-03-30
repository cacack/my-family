package citation

import (
	"testing"

	"github.com/cacack/my-family/internal/domain"
)

func TestGetTemplate(t *testing.T) {
	tmpl := GetTemplate("census.us.federal")
	if tmpl == nil {
		t.Fatal("expected census.us.federal template, got nil")
	}
	if tmpl.Name != "U.S. Federal Census" {
		t.Errorf("expected name 'U.S. Federal Census', got %q", tmpl.Name)
	}
}

func TestGetTemplateNotFound(t *testing.T) {
	if tmpl := GetTemplate("nonexistent"); tmpl != nil {
		t.Errorf("expected nil for nonexistent template, got %v", tmpl)
	}
}

func TestListTemplates(t *testing.T) {
	templates := ListTemplates()
	if len(templates) < 20 {
		t.Errorf("expected at least 20 templates, got %d", len(templates))
	}
}

func TestTemplatesForSourceType(t *testing.T) {
	tests := []struct {
		sourceType domain.SourceType
		wantMin    int
	}{
		{domain.SourceCensus, 3},
		{domain.SourceVitalRecord, 4},
		{domain.SourceChurch, 3},
		{domain.SourceNewspaper, 3},
		{domain.SourceBook, 3},
		{domain.SourceArchive, 3},
		{domain.SourceWebpage, 4},
		{domain.SourceInterview, 1},
		{domain.SourceCorrespond, 1},
	}

	for _, tt := range tests {
		t.Run(string(tt.sourceType), func(t *testing.T) {
			result := TemplatesForSourceType(tt.sourceType)
			if len(result) < tt.wantMin {
				t.Errorf("expected at least %d templates for %s, got %d", tt.wantMin, tt.sourceType, len(result))
			}
		})
	}
}

func TestCategories(t *testing.T) {
	cats := Categories()
	if len(cats) < 7 {
		t.Errorf("expected at least 7 categories, got %d", len(cats))
	}

	// First category should be Census Records (matches template order)
	if cats[0] != "Census Records" {
		t.Errorf("expected first category to be 'Census Records', got %q", cats[0])
	}
}

func TestTemplateIDsUnique(t *testing.T) {
	seen := make(map[string]bool)
	for _, tmpl := range allTemplates {
		if seen[tmpl.ID] {
			t.Errorf("duplicate template ID: %s", tmpl.ID)
		}
		seen[tmpl.ID] = true
	}
}

func TestAllTemplatesHaveRequiredMetadata(t *testing.T) {
	for _, tmpl := range allTemplates {
		t.Run(tmpl.ID, func(t *testing.T) {
			if tmpl.ID == "" {
				t.Error("template has empty ID")
			}
			if tmpl.Name == "" {
				t.Error("template has empty Name")
			}
			if tmpl.Category == "" {
				t.Error("template has empty Category")
			}
			if len(tmpl.SourceTypes) == 0 {
				t.Error("template has no SourceTypes")
			}
			if len(tmpl.Fields) == 0 {
				t.Error("template has no Fields")
			}
			if tmpl.FullFormat == "" {
				t.Error("template has empty FullFormat")
			}
			if tmpl.ShortFormat == "" {
				t.Error("template has empty ShortFormat")
			}
		})
	}
}

func TestAllTemplatesHaveAtLeastOneRequiredField(t *testing.T) {
	for _, tmpl := range allTemplates {
		t.Run(tmpl.ID, func(t *testing.T) {
			required := tmpl.RequiredFields()
			if len(required) == 0 {
				t.Error("template has no required fields")
			}
		})
	}
}

func TestFieldKeysUniquePerTemplate(t *testing.T) {
	for _, tmpl := range allTemplates {
		t.Run(tmpl.ID, func(t *testing.T) {
			seen := make(map[string]bool)
			for _, f := range tmpl.Fields {
				if seen[f.Key] {
					t.Errorf("duplicate field key: %s", f.Key)
				}
				seen[f.Key] = true
			}
		})
	}
}
