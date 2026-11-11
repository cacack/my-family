package gedcom_test

import (
	"context"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
)

// Family EVEN, DIVF, CENS and RESI (issue #1285).
const familyEventTagsGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @S1@ SOUR
1 TITL County History
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
0 @I2@ INDI
1 NAME Mary /Roe/
1 SEX F
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 EVEN Moved to Ohio
2 TYPE Relocation
2 DATE 1850
2 PLAC Columbus, Ohio
2 NOTE Went by wagon
2 SOUR @S1@
3 PAGE p. 12
1 DIVF
2 DATE 1860
1 CENS
2 DATE 1860
1 RESI
2 DATE 1855
0 TRLR
`

func TestImportFamilyEventTags(t *testing.T) {
	_, _, _, _, citations, _, events, _, _, _, _, _, _, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(familyEventTagsGedcom))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	byType := make(map[domain.FactType]gedcom.EventData)
	for _, e := range events {
		byType[e.FactType] = e
	}

	even, ok := byType[domain.FactFamilyGenericEvent]
	if !ok {
		t.Fatalf("no %s event; events = %+v", domain.FactFamilyGenericEvent, events)
	}
	if even.OwnerType != "family" || even.Description != "Moved to Ohio" || even.TypeDetail != "Relocation" ||
		even.Date != "1850" || even.Place != "Columbus, Ohio" || even.Notes != "Went by wagon" {
		t.Errorf("family EVEN = %+v", even)
	}
	for _, ft := range []domain.FactType{domain.FactFamilyDivorceFiling, domain.FactFamilyCensus, domain.FactFamilyResidence} {
		if _, ok := byType[ft]; !ok {
			t.Errorf("no %s event", ft)
		}
	}

	var cited bool
	for _, c := range citations {
		if c.FactType == string(domain.FactFamilyGenericEvent) && c.FactOwnerID == even.OwnerID && c.Page == "p. 12" {
			cited = true
		}
	}
	if !cited {
		t.Errorf("no citation for the family EVEN; citations = %+v", citations)
	}
}

func TestFamilyEventTags_RoundTrip(t *testing.T) {
	out := strings.ReplaceAll(importAndExport(t, familyEventTagsGedcom), "\r\n", "\n")

	famStart := strings.Index(out, "\n0 @F")
	if famStart < 0 {
		t.Fatalf("no FAM record in export:\n%s", out)
	}
	fam := out[famStart:]
	if end := strings.Index(fam[1:], "\n0 "); end >= 0 {
		fam = fam[:end+2]
	}

	for _, want := range []string{
		"1 EVEN Moved to Ohio\n2 DATE 1850\n",
		"2 TYPE Relocation\n",
		"2 PLAC Columbus, Ohio\n",
		"3 PAGE p. 12\n",
		"1 DIVF\n2 DATE 1860\n",
		"1 CENS\n2 DATE 1860\n",
		"1 RESI\n2 DATE 1855\n",
	} {
		if !strings.Contains(fam, want) {
			t.Errorf("FAM record lacks %q:\n%s", want, fam)
		}
	}
}
