package citation

// ValidationLevel indicates the severity of a validation issue.
type ValidationLevel string

const (
	ValidationError   ValidationLevel = "error"   // Required field missing
	ValidationWarning ValidationLevel = "warning" // Recommended field missing
)

// ValidationIssue represents a problem with citation template fields.
type ValidationIssue struct {
	Field   string          `json:"field"`
	Message string          `json:"message"`
	Level   ValidationLevel `json:"level"`
}

// ValidateFields checks that the provided fields satisfy the template requirements.
// Returns errors for missing required fields and warnings for empty optional fields.
func ValidateFields(tmpl *Template, fields map[string]string) []ValidationIssue {
	var issues []ValidationIssue
	for _, fd := range tmpl.Fields {
		val := fields[fd.Key]
		if fd.Required && val == "" {
			issues = append(issues, ValidationIssue{
				Field:   fd.Key,
				Message: fd.Label + " is required",
				Level:   ValidationError,
			})
		}
	}
	return issues
}

// HasErrors returns true if any issue is an error.
func HasErrors(issues []ValidationIssue) bool {
	for _, i := range issues {
		if i.Level == ValidationError {
			return true
		}
	}
	return false
}
