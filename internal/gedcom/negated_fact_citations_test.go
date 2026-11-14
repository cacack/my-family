package gedcom_test

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// A SOUR under a birth, death or marriage is not written under its negated
// counterpart (NO BIRT, NO DEAT, NO MARR), nor the other way round (#1300).

const negatedFactsGedcom = `0 HEAD
1 GEDC
2 VERS 7.0
0 @S1@ SOUR
1 TITL Baptismal register
0 @S2@ SOUR
1 TITL Parish census
0 @S3@ SOUR
1 TITL Burial register
0 @S4@ SOUR
1 TITL Later letter
0 @S5@ SOUR
1 TITL Marriage bond
0 @S6@ SOUR
1 TITL Church minutes
0 @I1@ INDI
1 NAME A /B/
1 BIRT
2 DATE 1830
2 SOUR @S1@
1 NO BIRT
2 SOUR @S2@
1 DEAT
2 DATE 1890
2 SOUR @S3@
1 NO DEAT
2 SOUR @S4@
0 @I2@ INDI
1 NAME C /D/
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 MARR
2 DATE 1855
2 SOUR @S5@
1 NO MARR
2 SOUR @S6@
0 TRLR
`

func TestRoundTrip_NegatedEventsKeepTheirOwnSources(t *testing.T) {
	out := importAndExport(t, negatedFactsGedcom)
	for head, sours := range map[string][]string{
		"1 BIRT": {"2 SOUR @S1@"}, "1 NO BIRT": {"2 SOUR @S2@"},
		"1 DEAT": {"2 SOUR @S3@"}, "1 NO DEAT": {"2 SOUR @S4@"},
		"1 MARR": {"2 SOUR @S5@"}, "1 NO MARR": {"2 SOUR @S6@"},
	} {
		if got := factSourLines(out, head, ""); !slices.Equal(got, sours) {
			t.Errorf("%s SOURs = %q, want %q; export:\n%s", head, got, sours, out)
		}
	}
}

// A citation naming the birth, death or marriage held on the record (fact_id
// = the owner's ID, #1319) is written under the record's event only, which
// is exported for it even when the record holds nothing else of it. One with
// no fact_id is mostly a citation of that record event created before #1319,
// so it too goes under the record's event when there is one, and otherwise
// under the NO event of its fact type (#1300).
func TestExport_UnlinkedCitationsOfNegatedFacts(t *testing.T) {
	ctx := context.Background()
	rs := memory.NewReadModelStore()
	main := domain.MainBranchID
	sourceID, markedSourceID := uuid.New(), uuid.New()
	mustSave(t, rs.SaveSource(ctx, main, &repository.SourceReadModel{ID: sourceID, Title: "Old source", Version: 1}))
	mustSave(t, rs.SaveSource(ctx, main, &repository.SourceReadModel{ID: markedSourceID, Title: "Record source", Version: 1}))

	both, negatedOnly, bare := uuid.New(), uuid.New(), uuid.New()
	mustSave(t, rs.SavePerson(ctx, main, &repository.PersonReadModel{ID: both, GivenName: "Both", BirthDateRaw: "1830", DeathDateRaw: "1890", Version: 1}))
	mustSave(t, rs.SavePerson(ctx, main, &repository.PersonReadModel{ID: negatedOnly, GivenName: "Negated", Version: 1}))
	mustSave(t, rs.SavePerson(ctx, main, &repository.PersonReadModel{ID: bare, GivenName: "Bare", Version: 1}))
	married, unmarried := uuid.New(), uuid.New()
	mustSave(t, rs.SaveFamily(ctx, main, &repository.FamilyReadModel{ID: married, Partner1ID: &both, RelationshipType: domain.RelationMarriage, MarriageDateRaw: "1855", Version: 1}))
	mustSave(t, rs.SaveFamily(ctx, main, &repository.FamilyReadModel{ID: unmarried, Partner1ID: &negatedOnly, RelationshipType: domain.RelationUnknown, Version: 1}))

	for _, owner := range []struct {
		ownerType string
		id        uuid.UUID
		facts     []domain.FactType
		marked    bool
	}{
		{"person", both, []domain.FactType{domain.FactPersonBirth, domain.FactPersonDeath}, true},
		{"person", negatedOnly, []domain.FactType{domain.FactPersonBirth, domain.FactPersonDeath}, false},
		{"person", bare, []domain.FactType{domain.FactPersonBirth}, true},
		{"family", married, []domain.FactType{domain.FactFamilyMarriage}, true},
		{"family", unmarried, []domain.FactType{domain.FactFamilyMarriage}, false},
	} {
		for _, ft := range owner.facts {
			mustSave(t, rs.SaveEvent(ctx, main, &repository.EventReadModel{ID: uuid.New(), OwnerType: owner.ownerType, OwnerID: owner.id, FactType: ft, IsNegated: true, Version: 1}))
			mustSave(t, rs.SaveCitation(ctx, main, &repository.CitationReadModel{ID: uuid.New(), SourceID: sourceID, SourceTitle: "Old source", FactType: ft, FactOwnerID: owner.id, Version: 1}))
			if owner.marked {
				ownerID := owner.id
				mustSave(t, rs.SaveCitation(ctx, main, &repository.CitationReadModel{ID: uuid.New(), SourceID: markedSourceID, SourceTitle: "Record source", FactType: ft, FactOwnerID: owner.id, FactID: &ownerID, Version: 1}))
			}
		}
	}

	buf := &bytes.Buffer{}
	if _, err := gedcom.NewExporter(rs).Export(ctx, buf); err != nil {
		t.Fatalf("export: %v", err)
	}
	out := buf.String()
	old, marked := "2 SOUR "+sourceXref(t, out, "Old source"), "2 SOUR "+sourceXref(t, out, "Record source")
	for _, c := range []struct {
		marker, without string // the record with a line starting with marker and none equal to without
		want            map[string][]string
	}{
		{"1 NAME Both", "", map[string][]string{"1 BIRT": {old, marked}, "1 NO BIRT": nil, "1 DEAT": {old, marked}, "1 NO DEAT": nil}},
		{"1 NAME Negated", "", map[string][]string{"1 BIRT": nil, "1 NO BIRT": {old}, "1 DEAT": nil, "1 NO DEAT": {old}}},
		{"1 NAME Bare", "", map[string][]string{"1 BIRT": {old, marked}, "1 NO BIRT": nil}},
		{"1 MARR", "", map[string][]string{"1 MARR": {old, marked}, "1 NO MARR": nil}},
		{"1 HUSB ", "1 MARR", map[string][]string{"1 NO MARR": {old}}},
	} {
		record := recordWith(out, c.marker, c.without)
		for head, want := range c.want {
			got := factSourLines(record, head, "")
			slices.Sort(got)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("record with %q: %s SOURs = %q, want %q; record:\n%s", c.marker, head, got, want, record)
			}
		}
	}
}

// sourceXref returns the xref of the SOUR record titled title in out.
func sourceXref(t *testing.T, out, title string) string {
	t.Helper()
	xref := strings.Fields(recordWith(out, "1 TITL "+title, ""))
	if len(xref) < 2 {
		t.Fatalf("no source titled %q in:\n%s", title, out)
	}
	return xref[1]
}

// recordWith returns the first record of out with a line starting with
// marker and no line equal to without.
func recordWith(out, marker, without string) string {
	for _, record := range strings.Split(out, "\n0 ") {
		lines := strings.Split(record, "\n")
		if slices.ContainsFunc(lines, func(l string) bool { return strings.HasPrefix(l, marker) }) && !slices.Contains(lines, without) {
			return "0 " + record
		}
	}
	return ""
}

func mustSave(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("save: %v", err)
	}
}
