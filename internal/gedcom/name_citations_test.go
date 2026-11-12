package gedcom_test

import (
	"bytes"
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// The SOURs and NOTEs under a person's NAME are imported against that name
// and exported under it again (#1165).

const nameSourcesNotesFixture = "../../testdata/gedcom-5.5/name-sources-notes.ged"

func readNameSourcesNotes(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(nameSourcesNotesFixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(data)
}

// nameSubLines maps each "1 NAME" line of the INDI records in out to its SOUR
// and NOTE lines, with their own subordinate lines, in file order.
func nameSubLines(out string) map[string][]string {
	blocks := map[string][]string{}
	name, keep := "", false
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "0 ") || strings.HasPrefix(line, "1 "):
			name, keep = "", false
			if strings.HasPrefix(line, "1 NAME ") {
				name = line
				blocks[name] = []string{}
			}
		case name == "":
		case strings.HasPrefix(line, "2 "):
			keep = strings.HasPrefix(line, "2 SOUR ") || strings.HasPrefix(line, "2 NOTE ")
			if keep {
				blocks[name] = append(blocks[name], line)
			}
		case keep:
			blocks[name] = append(blocks[name], line)
		}
	}
	return blocks
}

func TestImport_NameSourcesAndNotes(t *testing.T) {
	ctx := context.Background()
	rs := memory.NewReadModelStore()
	result, err := command.NewHandler(memory.NewEventStore(), rs).ImportGedcom(ctx, command.ImportGedcomInput{
		Filename: "t.ged", Reader: strings.NewReader(readNameSourcesNotes(t)),
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	for _, w := range result.Warnings {
		if strings.Contains(w, "NAME") {
			t.Errorf("warning about a NAME line: %q", w)
		}
	}

	persons, _, err := rs.ListPersons(ctx, repository.ListOptions{Limit: 10})
	if err != nil || len(persons) != 1 {
		t.Fatalf("persons = %d, %v; want 1", len(persons), err)
	}
	person := persons[0]
	names, err := rs.GetPersonNames(ctx, domain.MainBranchID, person.ID)
	if err != nil {
		t.Fatalf("get names: %v", err)
	}
	nameIDs := map[string]uuid.UUID{}
	notes := map[string]string{}
	for _, n := range names {
		nameIDs[n.Surname] = n.ID
		notes[n.Surname] = n.Notes
	}
	if got, want := notes["de Vermandois"], "Spelling varies."; got != want {
		t.Errorf("de Vermandois notes = %q, want %q", got, want)
	}
	if got, want := notes["of Vermandois"], "@N1@\n\nUsed in the charter."; got != want {
		t.Errorf("of Vermandois notes = %q, want %q", got, want)
	}

	sourceIDs := map[uuid.UUID]string{}
	sources, _, err := rs.ListSources(ctx, repository.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	for _, s := range sources {
		sourceIDs[s.ID] = s.GedcomXref
	}

	citations, err := rs.GetCitationsForFact(ctx, domain.MainBranchID, domain.FactPersonName, person.ID)
	if err != nil {
		t.Fatalf("get citations: %v", err)
	}
	type cited struct {
		source, page string
		evidence     domain.EvidenceType
		name         uuid.UUID
	}
	var got []cited
	for _, c := range citations {
		if c.FactID == nil {
			t.Fatalf("citation %s has no fact_id", c.ID)
		}
		got = append(got, cited{sourceIDs[c.SourceID], c.Page, c.EvidenceType, *c.FactID})
	}
	want := []cited{
		{"@S1@", "Folio 12", domain.EvidenceDirect, nameIDs["de Vermandois"]},
		{"@S2@", "Line 4", domain.EvidenceIndirect, nameIDs["of Vermandois"]},
		{"@S1@", "Folio 30", "", nameIDs["of Vermandois"]},
	}
	sortCited := func(c []cited) {
		slices.SortFunc(c, func(a, b cited) int { return strings.Compare(a.page, b.page) })
	}
	sortCited(got)
	sortCited(want)
	if !slices.Equal(got, want) {
		t.Errorf("name citations = %+v, want %+v", got, want)
	}
}

func TestRoundTrip_NameSourcesAndNotes(t *testing.T) {
	in := readNameSourcesNotes(t)
	out := importAndExport(t, in)
	want := nameSubLines(in)
	got := nameSubLines(out)
	if len(got) != len(want) {
		t.Fatalf("NAME lines = %d, want %d; export:\n%s", len(got), len(want), out)
	}
	for name, lines := range want {
		if !slices.Equal(got[name], lines) {
			t.Errorf("%s: SOUR/NOTE lines = %q, want %q; export:\n%s", name, got[name], lines, out)
		}
	}
}

// A person_name citation with no fact_id, or one naming a name the person no
// longer has, is written under the primary NAME; one naming a name goes under
// that NAME.
func TestExport_NameCitationPlacement(t *testing.T) {
	ctx := context.Background()
	rs := memory.NewReadModelStore()
	main := domain.MainBranchID
	personID := uuid.New()
	if err := rs.SavePerson(ctx, main, &repository.PersonReadModel{ID: personID, GivenName: "A", Surname: "B", Version: 1}); err != nil {
		t.Fatalf("save person: %v", err)
	}
	primary := repository.PersonNameReadModel{ID: uuid.New(), PersonID: personID, GivenName: "A", Surname: "B", IsPrimary: true}
	alias := repository.PersonNameReadModel{ID: uuid.New(), PersonID: personID, GivenName: "A", Surname: "C", NameType: domain.NameTypeAKA}
	for _, n := range []repository.PersonNameReadModel{alias, primary} {
		if err := rs.SavePersonName(ctx, main, &n); err != nil {
			t.Fatalf("save name: %v", err)
		}
	}
	sourceID := uuid.New()
	if err := rs.SaveSource(ctx, main, &repository.SourceReadModel{ID: sourceID, Title: "Register", GedcomXref: "@S1@", Version: 1}); err != nil {
		t.Fatalf("save source: %v", err)
	}
	gone := uuid.New()
	for page, factID := range map[string]*uuid.UUID{"unlinked": nil, "gone": &gone, "alias": &alias.ID} {
		c := &repository.CitationReadModel{ID: uuid.New(), SourceID: sourceID, FactType: domain.FactPersonName,
			FactOwnerID: personID, FactID: factID, Page: page, Version: 1}
		if err := rs.SaveCitation(ctx, main, c); err != nil {
			t.Fatalf("save citation: %v", err)
		}
	}

	var buf bytes.Buffer
	result, err := gedcom.NewExporter(rs).Export(ctx, &buf)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if result.CitationsExported != 3 {
		t.Errorf("CitationsExported = %d, want 3", result.CitationsExported)
	}
	pages := func(name string) []string {
		var p []string
		for _, line := range nameSubLines(buf.String())[name] {
			if v, ok := strings.CutPrefix(line, "3 PAGE "); ok {
				p = append(p, v)
			}
		}
		slices.Sort(p)
		return p
	}
	if got, want := pages("1 NAME A /B/"), []string{"gone", "unlinked"}; !slices.Equal(got, want) {
		t.Errorf("primary NAME citations = %q, want %q; export:\n%s", got, want, buf.String())
	}
	if got, want := pages("1 NAME A /C/"), []string{"alias"}; !slices.Equal(got, want) {
		t.Errorf("alias NAME citations = %q, want %q; export:\n%s", got, want, buf.String())
	}
}
