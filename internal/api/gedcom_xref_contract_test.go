package api_test

import (
	"strings"
	"testing"
)

// TestSpecGedcomXrefInputsConstrained checks that every request schema taking
// a gedcom_xref states the shape the command layer enforces
// (command.validateGedcomXref), so a new create/update schema can't add the
// field unconstrained (#527 item 6). Response schemas stay unconstrained:
// they echo stored xrefs, which GEDCOM import keeps verbatim.
func TestSpecGedcomXrefInputsConstrained(t *testing.T) {
	const wantPattern = `^(@[^@\s]+@)?$`
	const wantMaxLength = 64

	checked := 0
	for name, ref := range apiSpec.Components.Schemas {
		if !strings.HasSuffix(name, "Create") && !strings.HasSuffix(name, "Update") {
			continue
		}
		prop := ref.Value.Properties["gedcom_xref"]
		if prop == nil {
			continue
		}
		checked++
		if prop.Value.Pattern != wantPattern {
			t.Errorf("%s.gedcom_xref pattern = %q, want %q", name, prop.Value.Pattern, wantPattern)
		}
		if prop.Value.MaxLength == nil || *prop.Value.MaxLength != wantMaxLength {
			t.Errorf("%s.gedcom_xref maxLength = %v, want %d", name, prop.Value.MaxLength, wantMaxLength)
		}
	}
	if checked == 0 {
		t.Fatal("no create/update schema has a gedcom_xref property")
	}
}
