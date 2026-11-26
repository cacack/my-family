package gedcom_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/gedcom"
)

// A MAP under a PLAC that names no place is kept as the record's own pair,
// reported as an import warning naming the record and event, and written back
// under a valueless PLAC rather than dropped on export (#1807).
func TestRoundTrip_NamelessPlaceCoordinates(t *testing.T) {
	const pair = "3 MAP\n4 LATI N51.5074\n4 LONG W0.1278\n"
	const fam = "0 @I1@ INDI\n1 NAME Ada /L/\n1 FAMS @F1@\n0 @F1@ FAM\n1 WIFE @I1@"
	tests := []struct {
		name        string
		record      string // the record's lines, after its level-0 line
		level0      string // the record's level-0 line, after any records it needs
		wantWarning string // "" when none is expected
		wantBlock   string // the exported event lines that carry the pair
	}{
		{name: "record birth with no date", level0: "0 @I1@ INDI", record: "1 NAME Ada /L/\n1 BIRT\n2 PLAC\n" + pair,
			wantWarning: "Individual @I1@: BIRT coordinates", wantBlock: "1 BIRT\n2 PLAC\n" + pair},
		{name: "record death", level0: "0 @I1@ INDI", record: "1 NAME Ada /L/\n1 DEAT\n2 DATE 1852\n2 PLAC\n" + pair,
			wantWarning: "Individual @I1@: DEAT coordinates", wantBlock: "2 PLAC\n" + pair},
		{name: "life event", level0: "0 @I1@ INDI", record: "1 NAME Ada /L/\n1 CHR\n2 DATE 1815\n2 PLAC\n" + pair,
			wantWarning: "Individual @I1@: CHR coordinates", wantBlock: "2 PLAC\n" + pair},
		{name: "attribute", level0: "0 @I1@ INDI", record: "1 NAME Ada /L/\n1 OCCU Mathematician\n2 PLAC\n" + pair,
			wantWarning: "Individual @I1@: OCCU coordinates", wantBlock: "1 OCCU Mathematician\n2 PLAC\n" + pair},
		{name: "place text with only empty parts", level0: "0 @I1@ INDI", record: "1 NAME Ada /L/\n1 BIRT\n2 PLAC , ,\n" + pair,
			wantWarning: "Individual @I1@: BIRT coordinates", wantBlock: "1 BIRT\n2 PLAC , ,\n" + pair},
		{name: "record marriage with no date", level0: fam, record: "1 MARR\n2 PLAC\n" + pair,
			wantWarning: "Family @F1@: MARR coordinates", wantBlock: "1 MARR\n2 PLAC\n" + pair},
		{name: "family event", level0: fam, record: "1 DIV\n2 DATE 1860\n2 PLAC\n" + pair,
			wantWarning: "Family @F1@: DIV coordinates", wantBlock: "2 PLAC\n" + pair},
		{name: "named place", level0: "0 @I1@ INDI", record: "1 NAME Ada /L/\n1 BIRT\n2 PLAC London\n" + pair,
			wantBlock: "1 BIRT\n2 PLAC London\n" + pair},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := "0 HEAD\n1 GEDC\n2 VERS 5.5.1\n1 CHAR UTF-8\n" + tt.level0 + "\n" + tt.record + "0 TRLR\n"
			result, _, _, _, _, _, _, _, _, _, _, _, _, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(data))
			if err != nil {
				t.Fatalf("Import: %v", err)
			}
			nameless := slices.DeleteFunc(slices.Clone(result.Warnings), func(w string) bool { return !strings.Contains(w, "no place name") })
			if tt.wantWarning == "" {
				if len(nameless) != 0 {
					t.Errorf("warnings = %q, want none about a missing place name", nameless)
				}
			} else if want := tt.wantWarning + " have no place name; kept on the record"; !slices.Equal(nameless, []string{want}) {
				t.Errorf("warnings = %q, want [%q]", nameless, want)
			}

			first := importAndExport(t, data)
			if !strings.Contains(first, "\n"+tt.wantBlock) {
				t.Errorf("export lacks\n%s\ngot:\n%s", tt.wantBlock, first)
			}
			if second := importAndExport(t, first); second != first {
				t.Errorf("re-export differs:\n%s\nfirst:\n%s", second, first)
			}
		})
	}
}
