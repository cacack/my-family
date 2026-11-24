package gedcom_test

import (
	"context"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
)

// importFamily imports data and returns its one family, the family's
// marriage life events, its citations and the import's warnings.
func importFamily(t *testing.T, data string) (gedcom.FamilyData, []gedcom.EventData, []gedcom.CitationData, []string) {
	t.Helper()
	result, _, families, _, citations, _, events, _, _, _, _, _, _, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(data))
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(families) != 1 {
		t.Fatalf("families = %+v, want one", families)
	}
	var marriages []gedcom.EventData
	for _, e := range events {
		if e.FactType == domain.FactFamilyMarriage {
			marriages = append(marriages, e)
		}
	}
	return families[0], marriages, citations, result.Warnings
}

const marriageFamilyHead = "0 HEAD\n1 GEDC\n2 VERS 5.5.1\n0 @S1@ SOUR\n1 TITL Register\n" +
	"0 @I1@ INDI\n1 NAME A /B/\n0 @I2@ INDI\n1 NAME C /D/\n0 @F1@ FAM\n1 HUSB @I1@\n1 WIFE @I2@\n"

// A later MARR does not replace the dated record marriage: the record keeps
// its date, place and notes, and the later one is a marriage life event with its
// note, through a round trip with a stable export (#1776).
func TestRoundTrip_RecordMarriageBesideLaterMarriage(t *testing.T) {
	data := marriageFamilyHead +
		"1 MARR\n2 DATE 1850\n2 PLAC Boston\n2 NOTE In church\n1 MARR\n2 NOTE Second ceremony\n0 TRLR\n"

	check := func(stage, data string) {
		t.Helper()
		family, marriages, _, warnings := importFamily(t, data)
		if family.MarriageDate != "1850" || family.MarriagePlace != "Boston" || family.MarriageNotes != "In church" {
			t.Errorf("%s: family marriage = %q, %q, notes %q; want 1850, Boston, notes \"In church\"",
				stage, family.MarriageDate, family.MarriagePlace, family.MarriageNotes)
		}
		if len(marriages) != 1 || marriages[0].Notes != "Second ceremony" || marriages[0].Date != "" || marriages[0].IsNegated {
			t.Errorf("%s: marriage events = %+v, want one undated with notes %q", stage, marriages, "Second ceremony")
		}
		for _, w := range warnings {
			if strings.Contains(w, "not imported") {
				t.Errorf("%s: dropped-note warning %q", stage, w)
			}
		}
	}

	check("import", data)
	first := importAndExport(t, data)
	check("re-import", first)
	if second := importAndExport(t, first); second != first {
		t.Errorf("second export differs from first:\n--- first\n%s\n--- second\n%s", first, second)
	}
}

// The family record holds the first dated MARR, else the first that is not
// bare, else the first; every other MARR that is not bare is a marriage life
// event with its citations, and a bare one adds nothing beside the record
// but its citations (#1776).
func TestImport_RecordMarriageIsFirstDated(t *testing.T) {
	tests := []struct {
		name       string
		marr       string
		date, note string   // the record's
		events     []string // each marriage life event's date and notes
		eventCited bool     // the citation is linked to the life event
	}{
		{"undated before dated", "1 MARR\n2 NOTE First\n2 SOUR @S1@\n1 MARR\n2 DATE 1850\n",
			"1850", "", []string{"|First"}, true},
		{"two dated", "1 MARR\n2 DATE 1850\n1 MARR\n2 DATE 1851\n2 SOUR @S1@\n",
			"1850", "", []string{"1851|"}, true},
		{"none dated", "1 MARR\n2 NOTE First\n1 MARR\n2 TYPE Civil\n2 SOUR @S1@\n",
			"", "First", []string{"|"}, true},
		{"bare beside dated", "1 MARR Y\n2 SOUR @S1@\n1 MARR\n2 DATE 1850\n",
			"1850", "", nil, false},
		{"bare after undated", "1 MARR\n2 NOTE First\n1 MARR Y\n2 SOUR @S1@\n",
			"", "First", nil, false},
		{"only bare", "1 MARR Y\n1 MARR Y\n2 SOUR @S1@\n",
			"", "", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			family, marriages, citations, _ := importFamily(t, marriageFamilyHead+tt.marr+"0 TRLR\n")
			if family.RelationshipType != domain.RelationMarriage || family.MarriageDate != tt.date || family.MarriageNotes != tt.note {
				t.Errorf("family = %s, %q, notes %q; want marriage, %q, notes %q",
					family.RelationshipType, family.MarriageDate, family.MarriageNotes, tt.date, tt.note)
			}
			var got []string
			for _, e := range marriages {
				got = append(got, e.Date+"|"+e.Notes)
			}
			if strings.Join(got, ",") != strings.Join(tt.events, ",") {
				t.Errorf("marriage events = %q, want %q", got, tt.events)
			}
			if len(citations) != 1 || citations[0].FactID == nil {
				t.Fatalf("citations = %+v, want one linked to a fact", citations)
			}
			want := family.ID
			if tt.eventCited {
				want = marriages[0].ID
			}
			if *citations[0].FactID != want {
				t.Errorf("citation fact = %s, want %s (life event: %v)", *citations[0].FactID, want, tt.eventCited)
			}
		})
	}
}
