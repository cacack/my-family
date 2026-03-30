package citation

import (
	"testing"
)

func TestValidateFieldsAllPresent(t *testing.T) {
	tmpl := GetTemplate("census.us.federal")
	fields := map[string]string{
		"year":        "1850",
		"state":       "Virginia",
		"county":      "Augusta",
		"person_name": "John Smith",
	}

	issues := ValidateFields(tmpl, fields)
	if HasErrors(issues) {
		t.Errorf("expected no errors, got %v", issues)
	}
}

func TestValidateFieldsMissingRequired(t *testing.T) {
	tmpl := GetTemplate("census.us.federal")
	fields := map[string]string{
		"year":  "1850",
		"state": "Virginia",
		// missing county and person_name
	}

	issues := ValidateFields(tmpl, fields)
	if !HasErrors(issues) {
		t.Error("expected errors for missing required fields")
	}

	// Should have exactly 2 errors: county and person_name.
	errorCount := 0
	for _, i := range issues {
		if i.Level == ValidationError {
			errorCount++
		}
	}
	if errorCount != 2 {
		t.Errorf("expected 2 errors, got %d: %v", errorCount, issues)
	}
}

func TestValidateFieldsEmptyFields(t *testing.T) {
	tmpl := GetTemplate("vital.birth")
	issues := ValidateFields(tmpl, map[string]string{})
	if !HasErrors(issues) {
		t.Error("expected errors for empty fields on vital.birth template")
	}
}

func TestValidateFieldsNilFields(t *testing.T) {
	tmpl := GetTemplate("vital.birth")
	issues := ValidateFields(tmpl, nil)
	if !HasErrors(issues) {
		t.Error("expected errors for nil fields")
	}
}

func TestHasErrorsFalseForWarningsOnly(t *testing.T) {
	issues := []ValidationIssue{
		{Field: "town", Message: "Town is recommended", Level: ValidationWarning},
	}
	if HasErrors(issues) {
		t.Error("expected HasErrors to be false for warnings only")
	}
}

func TestHasErrorsFalseForEmpty(t *testing.T) {
	if HasErrors(nil) {
		t.Error("expected HasErrors to be false for nil")
	}
}
