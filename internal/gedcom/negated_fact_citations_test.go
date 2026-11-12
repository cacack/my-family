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

// A citation with no fact_id (imported before #1294) is written under the
// birth, death or marriage held on the record when there is one, and under the
// negated life event of its fact type when there is not.
func TestExport_UnlinkedCitationsOfNegatedFacts(t *testing.T) {
	ctx := context.Background()
	rs := memory.NewReadModelStore()
	main := domain.MainBranchID
	sourceID := uuid.New()
	mustSave(t, rs.SaveSource(ctx, main, &repository.SourceReadModel{ID: sourceID, Title: "Old source", Version: 1}))

	both, negatedOnly := uuid.New(), uuid.New()
	mustSave(t, rs.SavePerson(ctx, main, &repository.PersonReadModel{ID: both, GivenName: "Both", BirthDateRaw: "1830", DeathDateRaw: "1890", Version: 1}))
	mustSave(t, rs.SavePerson(ctx, main, &repository.PersonReadModel{ID: negatedOnly, GivenName: "Negated", Version: 1}))
	married, unmarried := uuid.New(), uuid.New()
	mustSave(t, rs.SaveFamily(ctx, main, &repository.FamilyReadModel{ID: married, Partner1ID: &both, RelationshipType: domain.RelationMarriage, MarriageDateRaw: "1855", Version: 1}))
	mustSave(t, rs.SaveFamily(ctx, main, &repository.FamilyReadModel{ID: unmarried, Partner1ID: &negatedOnly, RelationshipType: domain.RelationUnknown, Version: 1}))

	for _, owner := range []struct {
		ownerType string
		id        uuid.UUID
		facts     []domain.FactType
	}{
		{"person", both, []domain.FactType{domain.FactPersonBirth, domain.FactPersonDeath}},
		{"person", negatedOnly, []domain.FactType{domain.FactPersonBirth, domain.FactPersonDeath}},
		{"family", married, []domain.FactType{domain.FactFamilyMarriage}},
		{"family", unmarried, []domain.FactType{domain.FactFamilyMarriage}},
	} {
		for _, ft := range owner.facts {
			mustSave(t, rs.SaveEvent(ctx, main, &repository.EventReadModel{ID: uuid.New(), OwnerType: owner.ownerType, OwnerID: owner.id, FactType: ft, IsNegated: true, Version: 1}))
			mustSave(t, rs.SaveCitation(ctx, main, &repository.CitationReadModel{ID: uuid.New(), SourceID: sourceID, SourceTitle: "Old source", FactType: ft, FactOwnerID: owner.id, Version: 1}))
		}
	}

	buf := &bytes.Buffer{}
	if _, err := gedcom.NewExporter(rs).Export(ctx, buf); err != nil {
		t.Fatalf("export: %v", err)
	}
	out := buf.String()
	// Each record's own event (BIRT, DEAT, MARR) takes the citation when the
	// record has one; the NO event takes it only when it does not.
	for _, c := range []struct {
		marker, without string // the record with a line starting with marker and none equal to without
		want            map[string]int
	}{
		{"1 NAME Both", "", map[string]int{"1 BIRT": 1, "1 NO BIRT": 0, "1 DEAT": 1, "1 NO DEAT": 0}},
		{"1 NAME Negated", "", map[string]int{"1 BIRT": 0, "1 NO BIRT": 1, "1 DEAT": 0, "1 NO DEAT": 1}},
		{"1 MARR", "", map[string]int{"1 MARR": 1, "1 NO MARR": 0}},
		{"1 HUSB ", "1 MARR", map[string]int{"1 NO MARR": 1}},
	} {
		record := recordWith(out, c.marker, c.without)
		for head, n := range c.want {
			if got := factSourLines(record, head, ""); len(got) != n {
				t.Errorf("record with %q: %s SOURs = %q, want %d; record:\n%s", c.marker, head, got, n, record)
			}
		}
	}
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
