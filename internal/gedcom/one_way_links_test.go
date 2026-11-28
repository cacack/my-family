package gedcom_test

import (
	"context"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
)

// oneWayLinksGedcom records every family link on the family side only
// (HUSB/WIFE/CHIL with no FAMS/FAMC), except @I4@, whose FAMC carries a PEDI.
const oneWayLinksGedcom = `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
0 @I2@ INDI
1 NAME Jane /Doe/
1 SEX F
0 @I3@ INDI
1 NAME Kid /Doe/
0 @I4@ INDI
1 NAME Adopted /Doe/
1 FAMC @F1@
2 PEDI adopted
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 CHIL @I3@
1 CHIL @I4@
0 TRLR
`

// countContaining returns how many warnings contain substr.
func countContaining(warnings []string, substr string) int {
	n := 0
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			n++
		}
	}
	return n
}

// TestImportOneWayFamilyLinks checks that the family record is the authority
// for its members: links recorded only on the family side still import, and
// each one is reported once by the validator.
func TestImportOneWayFamilyLinks(t *testing.T) {
	result, persons, families, _, _, _, _, _, _, _, _, _, _, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(oneWayLinksGedcom))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if len(families) != 1 {
		t.Fatalf("len(families) = %d, want 1", len(families))
	}
	idOf := make(map[string]string, len(persons))
	for _, p := range persons {
		idOf[p.GedcomXref] = p.ID.String()
	}

	fam := families[0]
	if fam.Partner1ID == nil || fam.Partner1ID.String() != idOf["@I1@"] {
		t.Errorf("Partner1ID = %v, want @I1@ (%s)", fam.Partner1ID, idOf["@I1@"])
	}
	if fam.Partner2ID == nil || fam.Partner2ID.String() != idOf["@I2@"] {
		t.Errorf("Partner2ID = %v, want @I2@ (%s)", fam.Partner2ID, idOf["@I2@"])
	}
	wantChildren := []struct {
		xref string
		rel  domain.ChildRelationType
	}{
		{"@I3@", domain.ChildBiological},
		{"@I4@", domain.ChildAdopted},
	}
	if len(fam.ChildIDs) != len(wantChildren) || len(fam.ChildRelTypes) != len(wantChildren) {
		t.Fatalf("children = %v / %v, want %d", fam.ChildIDs, fam.ChildRelTypes, len(wantChildren))
	}
	for i, want := range wantChildren {
		if fam.ChildIDs[i].String() != idOf[want.xref] {
			t.Errorf("ChildIDs[%d] = %s, want %s (%s)", i, fam.ChildIDs[i], want.xref, idOf[want.xref])
		}
		if fam.ChildRelTypes[i] != want.rel {
			t.Errorf("ChildRelTypes[%d] = %q, want %q", i, fam.ChildRelTypes[i], want.rel)
		}
	}

	for _, tc := range []struct{ code, xref string }{
		{"UNRECIPROCATED_HUSB", "@I1@"},
		{"UNRECIPROCATED_WIFE", "@I2@"},
		{"UNRECIPROCATED_CHIL", "@I3@"},
	} {
		if n := countContaining(result.Warnings, tc.code+": "); n != 1 {
			t.Errorf("%s warnings = %d, want 1; warnings: %v", tc.code, n, result.Warnings)
		}
		if n := countContaining(result.Warnings, tc.xref+" in family @F1@"); n != 1 {
			t.Errorf("one-way link warnings for %s = %d, want 1; warnings: %v", tc.xref, n, result.Warnings)
		}
	}
	if n := countContaining(result.Warnings, "@I4@ in family @F1@"); n != 0 {
		t.Errorf("two-way link @I4@ reported %d times, want 0; warnings: %v", n, result.Warnings)
	}
	if n := countContaining(result.Warnings, "not found"); n != 0 {
		t.Errorf("unexpected \"not found\" warnings: %v", result.Warnings)
	}
}

// TestImportFamilyMemberNotFound checks that a HUSB/WIFE/CHIL pointing to no
// record keeps its "not found" warning and leaves the member off.
func TestImportFamilyMemberNotFound(t *testing.T) {
	data := `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Kid /Doe/
0 @F1@ FAM
1 HUSB @I8@
1 WIFE @I9@
1 CHIL @I1@
1 CHIL @I7@
0 TRLR
`
	result, _, families, _, _, _, _, _, _, _, _, _, _, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(data))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if len(families) != 1 {
		t.Fatalf("len(families) = %d, want 1", len(families))
	}
	fam := families[0]
	if fam.Partner1ID != nil || fam.Partner2ID != nil {
		t.Errorf("partners = %v, %v, want both nil", fam.Partner1ID, fam.Partner2ID)
	}
	if len(fam.ChildIDs) != 1 {
		t.Errorf("len(ChildIDs) = %d, want 1 (the dangling CHIL left off)", len(fam.ChildIDs))
	}
	for _, want := range []string{
		"Family @F1@: husband @I8@ not found",
		"Family @F1@: wife @I9@ not found",
		"Family @F1@: child @I7@ not found",
	} {
		if n := countContaining(result.Warnings, want); n != 1 {
			t.Errorf("%q warnings = %d, want 1; warnings: %v", want, n, result.Warnings)
		}
	}
}
