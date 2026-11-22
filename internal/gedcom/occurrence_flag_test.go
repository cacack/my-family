package gedcom_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
)

// A "1 BIRT Y" that asserts only that the birth happened is kept as a birth
// life event when the person record has no birth (#1454).
func TestImport_BareBirth(t *testing.T) {
	tests := []struct {
		name          string
		birt          string // the individual's BIRT lines
		wantEvent     bool   // imported as a birth life event
		wantBirthDate string // the person record's birth date
		wantNotes     string // the person record's birth notes
		wantCitation  bool   // the life event carries the BIRT's citation
	}{
		{name: "BIRT Y", birt: "1 BIRT Y\n", wantEvent: true},
		{name: "BIRT with no payload", birt: "1 BIRT\n"},
		{name: "BIRT Y with a citation", birt: "1 BIRT Y\n2 SOUR @S1@\n", wantEvent: true, wantCitation: true},
		{name: "BIRT Y with a note", birt: "1 BIRT Y\n2 NOTE At home\n", wantNotes: "At home"},
		{name: "dated BIRT", birt: "1 BIRT\n2 DATE 1850\n", wantBirthDate: "1850"},
		{name: "BIRT Y after a dated BIRT", birt: "1 BIRT\n2 DATE 1850\n1 BIRT Y\n", wantBirthDate: "1850"},
		{name: "BIRT Y before a dated BIRT", birt: "1 BIRT Y\n1 BIRT\n2 DATE 1850\n", wantBirthDate: "1850"},
		{name: "NO BIRT", birt: "1 NO BIRT\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := "0 HEAD\n1 GEDC\n2 VERS 7.0\n0 @S1@ SOUR\n1 TITL Register\n0 @I1@ INDI\n1 NAME A /B/\n" + tt.birt + "0 TRLR\n"
			_, persons, _, _, citations, _, events, _, _, _, _, _, _, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(data))
			if err != nil {
				t.Fatalf("Import: %v", err)
			}
			if len(persons) != 1 {
				t.Fatalf("persons = %d, want 1", len(persons))
			}
			if got := persons[0].BirthDate; got != tt.wantBirthDate {
				t.Errorf("BirthDate = %q, want %q", got, tt.wantBirthDate)
			}
			if got := persons[0].BirthNotes; got != tt.wantNotes {
				t.Errorf("BirthNotes = %q, want %q", got, tt.wantNotes)
			}
			var births []gedcom.EventData
			for _, e := range events {
				if e.FactType == domain.FactPersonBirth && !e.IsNegated {
					births = append(births, e)
				}
			}
			if got := len(births) == 1; got != tt.wantEvent || len(births) > 1 {
				t.Fatalf("birth life events = %+v, want one: %v", births, tt.wantEvent)
			}
			if tt.wantCitation {
				if len(citations) != 1 || citations[0].FactID == nil || *citations[0].FactID != births[0].ID {
					t.Errorf("citations = %+v, want one on the birth life event %s", citations, births[0].ID)
				}
			}
		})
	}
}

// level1Lines returns, sorted, the level-1 lines of out that start with one of
// tags followed by a space or the end of the line.
func level1Lines(out string, tags ...string) []string {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		for _, tag := range tags {
			if line == "1 "+tag || strings.HasPrefix(line, "1 "+tag+" ") {
				lines = append(lines, line)
			}
		}
	}
	slices.Sort(lines)
	return lines
}

// Named events asserted with "Y" are exported with it and import back as the
// same events, none duplicated (#1454).
func TestRoundTrip_OccurrenceFlag(t *testing.T) {
	data := "0 HEAD\n1 GEDC\n2 VERS 7.0\n0 @I1@ INDI\n1 NAME A /B/\n" +
		"1 BIRT Y\n1 DEAT Y\n1 BURI Y\n1 CREM Y\n1 FAMS @F1@\n" +
		"0 @I2@ INDI\n1 NAME C /D/\n1 FAMS @F1@\n" +
		"0 @F1@ FAM\n1 HUSB @I1@\n1 WIFE @I2@\n1 DIV Y\n0 TRLR\n"
	tags := []string{"BIRT", "DEAT", "BURI", "CREM", "DIV"}
	first := importAndExport(t, data)
	want := []string{"1 BIRT Y", "1 BURI Y", "1 CREM Y", "1 DEAT Y", "1 DIV Y"}
	if got := level1Lines(first, tags...); !slices.Equal(got, want) {
		t.Errorf("event lines = %q, want %q; export:\n%s", got, want, first)
	}

	_, _, _, _, _, _, events, _, _, _, _, _, _, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(first))
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	counts := make(map[domain.FactType]int)
	for _, e := range events {
		if !e.IsNegated && e.Date == "" && e.Place == "" {
			counts[e.FactType]++
		}
	}
	for _, ft := range []domain.FactType{domain.FactPersonBirth, domain.FactPersonDeath, domain.FactPersonBurial, domain.FactPersonCremation, domain.FactFamilyDivorce} {
		if counts[ft] != 1 {
			t.Errorf("re-imported %s events = %d, want 1 (all: %v)", ft, counts[ft], counts)
		}
	}
	// Record order is not stable across imports, so the event lines are
	// compared, not the whole export.
	if second := importAndExport(t, first); !slices.Equal(level1Lines(second, tags...), want) {
		t.Errorf("second export event lines = %q, want %q", level1Lines(second, tags...), want)
	}
}

// A generic EVEN, a residence and a negated event are never written with "Y",
// whose line value is not the occurrence flag (#1454).
func TestExport_OccurrenceFlagNotWritten(t *testing.T) {
	data := "0 HEAD\n1 GEDC\n2 VERS 7.0\n0 @I1@ INDI\n1 NAME A /B/\n" +
		"1 EVEN\n2 TYPE Moved\n1 RESI\n2 NOTE Lodger\n1 NO BURI\n1 FAMS @F1@\n" +
		"0 @I2@ INDI\n1 NAME C /D/\n1 FAMS @F1@\n" +
		"0 @F1@ FAM\n1 HUSB @I1@\n1 WIFE @I2@\n1 RESI\n2 NOTE Farm\n0 TRLR\n"
	out := importAndExport(t, data)
	want := []string{"1 EVEN", "1 NO BURI", "1 RESI", "1 RESI"}
	if got := level1Lines(out, "EVEN", "RESI", "NO"); !slices.Equal(got, want) {
		t.Errorf("event lines = %q, want %q; export:\n%s", got, want, out)
	}
}
