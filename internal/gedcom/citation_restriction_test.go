package gedcom_test

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	gedcomgo "github.com/cacack/gedcom-go/v3/gedcom"
	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// citationRestrictionStore holds a private citation, by page, on a person's
// birth ("birth"), on the person's name ("name") and on a family's marriage
// ("marriage"), and a public one on the birth ("public").
func citationRestrictionStore(t *testing.T) *memory.ReadModelStore {
	t.Helper()
	ctx := context.Background()
	rs := memory.NewReadModelStore()
	main := domain.MainBranchID
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	sourceID := uuid.New()
	must(rs.SaveSource(ctx, main, &repository.SourceReadModel{ID: sourceID, SourceType: "book", Title: "Parish Register", Version: 1}))
	personID := uuid.New()
	must(rs.SavePerson(ctx, main, &repository.PersonReadModel{ID: personID, GivenName: "Ann", Surname: "Lee", FullName: "Ann Lee", BirthDateRaw: "1830", Version: 1}))
	name := repository.PersonNameReadModel{ID: uuid.New(), PersonID: personID, GivenName: "Ann", Surname: "Lee", IsPrimary: true}
	must(rs.SavePersonName(ctx, main, &name))
	familyID := uuid.New()
	must(rs.SaveFamily(ctx, main, &repository.FamilyReadModel{ID: familyID, Partner1ID: &personID, Partner1GivenName: "Ann", Partner1Surname: "Lee",
		RelationshipType: domain.RelationMarriage, MarriageDateRaw: "1850", Version: 1}))

	cite := func(page string, factType domain.FactType, ownerID uuid.UUID, factID *uuid.UUID, r domain.Restriction) {
		must(rs.SaveCitation(ctx, main, &repository.CitationReadModel{ID: uuid.New(), SourceID: sourceID, FactType: factType,
			FactOwnerID: ownerID, FactID: factID, Page: page, Restriction: r, Version: 1}))
	}
	cite("birth", domain.FactPersonBirth, personID, &personID, domain.RestrictionConfidential)
	cite("public", domain.FactPersonBirth, personID, &personID, "")
	cite("name", domain.FactPersonName, personID, &name.ID, domain.RestrictionConfidential)
	cite("marriage", domain.FactFamilyMarriage, familyID, &familyID, domain.RestrictionConfidential)
	return rs
}

// citationSubLines returns the lines directly under each SOUR pointer
// line, by the citation's PAGE.
func citationSubLines(t *testing.T, out string) map[string][]string {
	t.Helper()
	lines := strings.Split(out, "\n")
	byPage := map[string][]string{}
	for i, line := range lines {
		level, rest, ok := strings.Cut(line, " ")
		if !ok || !strings.HasPrefix(rest, "SOUR @") || level == "0" {
			continue
		}
		n, err := strconv.Atoi(level)
		if err != nil {
			t.Fatalf("bad level in %q", line)
		}
		child := fmt.Sprintf("%d ", n+1)
		var sub []string
		page := ""
		for _, l := range lines[i+1:] {
			lv, _, _ := strings.Cut(l, " ")
			if m, err := strconv.Atoi(lv); err != nil || m <= n {
				break
			}
			if strings.HasPrefix(l, child) {
				sub = append(sub, strings.TrimPrefix(l, child))
				if v, ok := strings.CutPrefix(l, child+"PAGE "); ok {
					page = v
				}
			}
		}
		byPage[page] = sub
	}
	return byPage
}

// A private citation exports with _RESN directly under its SOUR line, in
// 5.5.1 and 7.0, and re-imports with the same restriction (#1544).
func TestExport_CitationRestrictionRoundTrips(t *testing.T) {
	for _, version := range []gedcomgo.Version{gedcomgo.Version551, gedcomgo.Version70} {
		t.Run(string(version), func(t *testing.T) {
			ctx := context.Background()
			var buf bytes.Buffer
			if _, err := gedcom.NewExporter(citationRestrictionStore(t)).ExportWithOptions(ctx, &buf, gedcom.ExportOptions{TargetVersion: version}); err != nil {
				t.Fatalf("export: %v", err)
			}
			out := buf.String()
			sub := citationSubLines(t, out)
			for _, page := range []string{"birth", "name", "marriage", "public"} {
				lines, ok := sub[page]
				if !ok {
					t.Fatalf("no citation with PAGE %s in export:\n%s", page, out)
				}
				want := page != "public"
				var has bool
				for _, l := range lines {
					has = has || l == "_RESN confidential"
				}
				if has != want {
					t.Errorf("citation %s sublines %q: _RESN confidential present = %v, want %v", page, lines, has, want)
				}
			}

			_, _, _, _, citations, _, _, _, _, _, _, _, _, err := gedcom.NewImporter().Import(ctx, &buf)
			if err != nil {
				t.Fatalf("import: %v", err)
			}
			got := map[string]string{}
			for _, c := range citations {
				got[c.Page] = c.Restriction
			}
			want := map[string]string{"birth": "confidential", "name": "confidential", "marriage": "confidential", "public": ""}
			for page, r := range want {
				if g, ok := got[page]; !ok || g != r {
					t.Errorf("imported citation %s restriction = %q (found %v), want %q", page, g, ok, r)
				}
			}
		})
	}
}

// An imported citation's _RESN on a person's event, on a name and on a family
// event sets its restriction (#1544).
func TestImport_CitationRestriction(t *testing.T) {
	const input = `0 HEAD
1 GEDC
2 VERS 5.5.1
0 @S1@ SOUR
1 TITL Register
0 @I1@ INDI
1 NAME Ann /Lee/
2 SOUR @S1@
3 PAGE name
3 _RESN confidential
1 BURI
2 SOUR @S1@
3 PAGE burial
3 _RESN locked
1 OCCU Weaver
2 SOUR @S1@
3 PAGE occupation
0 @F1@ FAM
1 HUSB @I1@
1 MARR
2 SOUR @S1@
3 PAGE marriage
3 _RESN privacy
0 TRLR
`
	_, _, _, _, citations, _, _, _, _, _, _, _, _, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	got := map[string]string{}
	for _, c := range citations {
		got[c.Page] = c.Restriction
	}
	want := map[string]string{"name": "confidential", "burial": "locked", "occupation": "", "marriage": "privacy"}
	if len(got) != len(want) {
		t.Fatalf("citations = %+v, want pages %v", citations, want)
	}
	for page, r := range want {
		if got[page] != r {
			t.Errorf("citation %s restriction = %q, want %q", page, got[page], r)
		}
	}
}
