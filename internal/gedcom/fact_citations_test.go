package gedcom_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/gedcom"
)

// A SOUR under one of several events or attributes of one fact type cites
// that one only (issue #1294).

const repeatedFactsGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @S1@ SOUR
1 TITL 1850 census
0 @S2@ SOUR
1 TITL 1860 census
0 @S3@ SOUR
1 TITL Farm ledger
0 @S4@ SOUR
1 TITL City directory
0 @S5@ SOUR
1 TITL Deed book
0 @S6@ SOUR
1 TITL Probate file
0 @S7@ SOUR
1 TITL Baptismal register
0 @S8@ SOUR
1 TITL Draft card
0 @S9@ SOUR
1 TITL Passport
0 @I1@ INDI
1 NAME A /B/
1 BIRT
2 DATE 1830
2 SOUR @S7@
1 CENS
2 DATE 1850
2 SOUR @S1@
1 CENS
2 DATE 1860
2 SOUR @S2@
1 OCCU Farmer
2 SOUR @S3@
1 OCCU Blacksmith
2 SOUR @S4@
1 IDNO 111
2 SOUR @S8@
1 IDNO 222
2 SOUR @S9@
0 @I2@ INDI
1 NAME C /D/
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 EVEN
2 TYPE Land purchase
2 DATE 1855
2 SOUR @S5@
1 EVEN
2 TYPE Bequest
2 DATE 1870
2 SOUR @S6@
0 TRLR
`

func TestImport_CitationLinksItsOwnFact(t *testing.T) {
	_, _, _, _, citations, _, events, attributes, _, _, _, _, _, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(repeatedFactsGedcom))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	factIDs := map[string]uuid.UUID{}
	for _, e := range events {
		factIDs[e.Date] = e.ID
	}
	for _, a := range attributes {
		factIDs[a.Value] = a.ID
	}
	want := map[string]string{ // source xref -> the fact it sits under
		"@S1@": "1850", "@S2@": "1860", "@S3@": "Farmer", "@S4@": "Blacksmith", "@S5@": "1855", "@S6@": "1870",
		"@S8@": "111", "@S9@": "222",
	}
	for _, c := range citations {
		fact, ok := want[c.SourceXref]
		if !ok {
			// The birth is stored on the person, so its SOUR names the
			// person's own record (#1319).
			if c.FactID == nil || *c.FactID != c.FactOwnerID {
				t.Errorf("citation of %s: fact_id = %v, want the owner %s", c.SourceXref, c.FactID, c.FactOwnerID)
			}
			continue
		}
		if c.FactID == nil || *c.FactID != factIDs[fact] {
			t.Errorf("citation of %s: fact_id = %v, want %s (%s)", c.SourceXref, c.FactID, factIDs[fact], fact)
		}
	}
}

func TestRoundTrip_RepeatedFactsKeepTheirOwnSources(t *testing.T) {
	out := importAndExport(t, repeatedFactsGedcom)
	want := map[string][]string{
		"1 BIRT":            {"2 SOUR @S7@"},
		"1 OCCU Farmer":     {"2 SOUR @S3@"},
		"1 OCCU Blacksmith": {"2 SOUR @S4@"},
		"1 IDNO 111":        {"2 SOUR @S8@"},
		"1 IDNO 222":        {"2 SOUR @S9@"},
	}
	for head, sours := range want {
		if got := factSourLines(out, head, ""); !slices.Equal(got, sours) {
			t.Errorf("%s SOURs = %q, want %q; export:\n%s", head, got, sours, out)
		}
	}
	for date, sours := range map[string][]string{
		"1850": {"2 SOUR @S1@"}, "1860": {"2 SOUR @S2@"}, "1855": {"2 SOUR @S5@"}, "1870": {"2 SOUR @S6@"},
	} {
		head := "1 CENS"
		if date == "1855" || date == "1870" {
			head = "1 EVEN"
		}
		if got := factSourLines(out, head, date); !slices.Equal(got, sours) {
			t.Errorf("%s %s SOURs = %q, want %q; export:\n%s", head, date, got, sours, out)
		}
	}
}

// factSourLines returns the level-2 SOUR lines under each level-1 line equal
// to head that has a "2 DATE <date>" line (any, if date is "").
func factSourLines(out, head, date string) []string {
	var got, sours []string
	in, dated := false, date == ""
	flush := func() {
		if in && dated {
			got = append(got, sours...)
		}
		sours, in, dated = nil, false, date == ""
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "0 ") || strings.HasPrefix(line, "1 "):
			flush()
			in = line == head
		case in && line == "2 DATE "+date:
			dated = true
		case in && strings.HasPrefix(line, "2 SOUR "):
			sours = append(sours, line)
		}
	}
	flush()
	return got
}
