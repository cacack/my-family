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

// A person's place facts follow the line of each event's and attribute's
// tag; with no lines, as in a document built in code, its events come
// before its attributes (#1879).
func TestPlaceFacts_OrderWithinAPerson(t *testing.T) {
	place := func(form string) *gedcom.PlaceDetail { return &gedcom.PlaceDetail{Name: "Kiel", Form: form} }
	for _, tc := range []struct {
		name               string
		birtLine, occuLine int
		want               []string
	}{
		{"built in code", 0, 0, []string{"BIRT", "OCCU"}},
		{"attribute first in file", 4, 2, []string{"OCCU", "BIRT"}},
		{"event first in file", 2, 4, []string{"BIRT", "OCCU"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := &gedcom.Document{Records: []*gedcom.Record{{
				Type: gedcom.RecordTypeIndividual,
				Entity: &gedcom.Individual{
					XRef:       "@I1@",
					Events:     []*gedcom.Event{{Type: gedcom.EventBirth, PlaceDetail: place("City"), LineNumber: tc.birtLine}},
					Attributes: []*gedcom.Attribute{{Type: gedcom.AttributeOccupation, PlaceDetail: place("Town"), LineNumber: tc.occuLine}},
				},
			}}}
			var got []string
			for _, p := range placeFacts(doc) {
				got = append(got, p.Tag)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("tags = %q, want %q", got, tc.want)
			}
		})
	}
}
