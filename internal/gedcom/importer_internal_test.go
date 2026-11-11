package gedcom

import (
	"slices"
	"testing"

	"github.com/cacack/gedcom-go/v3/gedcom"
	"github.com/google/uuid"
)

// Every imported attribute's fact type exports under the tag it came from,
// and is a valid fact type (#1287).
func TestPersonAttributeFactTypes_ExportUnderTheirOwnTag(t *testing.T) {
	for tag, factType := range personAttributeFactTypes {
		if !factType.IsValid() {
			t.Errorf("%s: fact type %q is not a valid FactType", tag, factType)
		}
		if got := mapFactTypeToGedcomTag(factType); got != string(tag) {
			t.Errorf("%s: fact type %q exports as %q", tag, factType, got)
		}
	}
}

// An attribute that is not imported, for want of a value or of a person fact
// type, is warned about instead of dropped silently (#1287).
func TestExtractAttributes_WarnsAboutAttributesNotImported(t *testing.T) {
	indi := &gedcom.Individual{
		XRef: "@I1@",
		Attributes: []*gedcom.Attribute{
			{Type: gedcom.AttributeOccupation},
			{Type: "_MILT", Value: "Navy"},
			nil,
			{Type: gedcom.AttributeNationality, Value: "American"},
		},
	}
	result := &ImportResult{}
	attrs, _ := extractAttributesFromIndividual(indi, uuid.New(), result)

	if len(attrs) != 1 || attrs[0].Value != "American" {
		t.Errorf("attributes = %+v, want only NATI", attrs)
	}
	want := []string{
		"Individual @I1@: attribute OCCU has no value, not imported",
		"Individual @I1@: attribute _MILT has no person fact type, not imported",
	}
	if !slices.Equal(result.Warnings, want) {
		t.Errorf("warnings = %q, want %q", result.Warnings, want)
	}
}
