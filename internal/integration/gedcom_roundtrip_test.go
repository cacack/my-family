package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/cacack/gedcom-go/v3/decoder"
	gcgedcom "github.com/cacack/gedcom-go/v3/gedcom"
	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
)

// A GEDCOM file imported, exported at its own version and imported again
// holds the same tree (issue #622).
//
// The 7.0 fixture combines the 7.0 features import and export support: EXID
// on individuals, families, sources and repositories; SNOTE records (one
// multi-line, one with MIME, LANG and a translation) and SNOTE pointers; an
// INT date; and Julian, Hebrew and French Republican dates. The 5.5 fixture
// guards 5.5 compatibility. Each leg compares the two imports' trees with the
// ids, versions, timestamps and xrefs left out. The files themselves are not
// compared: export orders and numbers individuals afresh each time.

var roundTripFixtures = []struct {
	name    string
	path    string
	version gcgedcom.Version
}{
	{"7.0", "../../testdata/gedcom-7.0/comprehensive.ged", gcgedcom.Version70},
	{"5.5", "../../testdata/gedcom-5.5/comprehensive.ged", gcgedcom.Version55},
}

func TestGedcomRoundTrip_FixturesKeepTheirTree(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			for _, fixture := range roundTripFixtures {
				t.Run(fixture.name, func(t *testing.T) {
					ctx := context.Background()
					data, err := os.ReadFile(fixture.path)
					if err != nil {
						t.Fatalf("read fixture: %v", err)
					}

					first := backend.setup(t)
					result := importInto(t, ctx, first, data)
					if fixture.version == gcgedcom.Version70 {
						if len(result.Warnings) > 0 {
							t.Errorf("7.0 fixture import warnings = %q, want none", result.Warnings)
						}
						assertGedcom7FixtureImported(t, ctx, first.read)
					}
					exported := exportAt(t, ctx, first.read, fixture.version)
					if want := "2 VERS " + string(fixture.version) + "\n"; !strings.Contains(exported, want) {
						t.Fatalf("export lacks %q:\n%s", want, exported)
					}
					if fixture.version == gcgedcom.Version70 {
						assertSharedNotesExportedAsSNOTE(t, exported)
					}
					assertSameFamilyLinks(t, data, []byte(exported))

					second := backend.setup(t)
					importInto(t, ctx, second, []byte(exported))
					want, got := roundTripDump(t, ctx, first.read), roundTripDump(t, ctx, second.read)
					if len(want) == 0 {
						t.Fatal("precondition: the fixture imported nothing")
					}
					assertSameRoundTripDump(t, want, got)
				})
			}
		})
	}
}

// assertSameFamilyLinks checks the export keeps every FAMS (#1357) and FAMC
// (#1374) link of the fixture, and links each family member back to its
// family: a spouse by FAMS and a child by FAMC. A fixture may leave out a
// link its family record implies, so the expected links are those of either
// direction. Xrefs are renumbered on export, so each link is named by its
// tag, the individual and the family's partners.
func assertSameFamilyLinks(t *testing.T, fixture, exported []byte) {
	t.Helper()
	wantPointers, wantMembers := familyLinks(t, fixture)
	got, gotMembers := familyLinks(t, exported)
	want := slices.Compact(slices.Sorted(slices.Values(append(wantPointers, wantMembers...))))
	for _, tag := range []string{"FAMS ", "FAMC "} {
		if !slices.ContainsFunc(wantPointers, func(l string) bool { return strings.HasPrefix(l, tag) }) {
			t.Fatalf("precondition: the fixture has no %slinks", tag)
		}
	}
	if !slices.Equal(want, got) {
		t.Errorf("exported FAMS/FAMC links = %q, want %q", got, want)
	}
	if !slices.Equal(gotMembers, got) {
		t.Errorf("exported family members = %q, want the FAMS/FAMC links %q", gotMembers, got)
	}
}

// familyLinks returns, as sorted "TAG individual -> partners" strings, each
// FAMS and FAMC pointer on the individuals in data, and the same links as
// the family records imply them: FAMS for each HUSB and WIFE, FAMC for each
// CHIL.
func familyLinks(t *testing.T, data []byte) (pointers, members []string) {
	t.Helper()
	doc, err := decoder.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	name := func(xref string) string {
		if indi := doc.GetIndividual(xref); indi != nil {
			return indi.FullName()
		}
		return ""
	}
	link := func(tag, indi string, fam *gcgedcom.Family) string {
		return tag + " " + indi + " -> " + name(fam.Husband) + " & " + name(fam.Wife)
	}
	for _, indi := range doc.Individuals() {
		for tag, famLinks := range map[string][]gcgedcom.FamilyLink{"FAMS": indi.SpouseInFamilies, "FAMC": indi.ChildInFamilies} {
			for _, l := range famLinks {
				fam := doc.GetFamily(l.FamilyXRef)
				if fam == nil {
					t.Errorf("%s has %s %s, which is not a family record", indi.FullName(), tag, l.FamilyXRef)
					continue
				}
				pointers = append(pointers, link(tag, indi.FullName(), fam))
			}
		}
	}
	for _, fam := range doc.Families() {
		for _, partner := range []string{fam.Husband, fam.Wife} {
			if partner != "" {
				members = append(members, link("FAMS", name(partner), fam))
			}
		}
		for _, child := range fam.Children {
			members = append(members, link("FAMC", name(child), fam))
		}
	}
	sort.Strings(pointers)
	sort.Strings(members)
	return pointers, members
}

func importInto(t *testing.T, ctx context.Context, st stores, data []byte) *command.ImportGedcomResult {
	t.Helper()
	h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
	result, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: bytes.NewReader(data)})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("import errors = %q, want none", result.Errors)
	}
	return result
}

func exportAt(t *testing.T, ctx context.Context, rs repository.ReadModelStore, version gcgedcom.Version) string {
	t.Helper()
	var buf bytes.Buffer
	if _, err := gedcom.NewExporter(rs).ExportWithOptions(ctx, &buf, gedcom.ExportOptions{TargetVersion: version}); err != nil {
		t.Fatalf("export: %v", err)
	}
	return buf.String()
}

// noteRecordOrPointer matches a 5.5 NOTE record or NOTE pointer line. 7.0 has
// neither: its NOTE takes text, so another reader takes "NOTE @N1@" as the
// text "@N1@". The re-import cannot catch this, as it accepts both forms.
var noteRecordOrPointer = regexp.MustCompile(`(?m)^(0 @[^@]+@ NOTE( |$)|\d+ NOTE @[^@]+@$)`)

// assertSharedNotesExportedAsSNOTE checks a 7.0 export writes the fixture's
// shared notes and the pointers to them as SNOTE, as the fixture does.
func assertSharedNotesExportedAsSNOTE(t *testing.T, exported string) {
	t.Helper()
	for _, want := range []string{"\n0 @N1@ SNOTE ", "\n0 @N2@ SNOTE ", "\n1 SNOTE @N1@\n", "\n1 SNOTE @N2@\n", "\n2 SNOTE @N1@\n"} {
		if !strings.Contains(exported, want) {
			t.Errorf("7.0 export lacks %q:\n%s", strings.TrimSpace(want), exported)
		}
	}
	// Pointers under a NAME (#1165) and on a FAM record (#1267).
	for _, want := range []struct{ record, line string }{
		{"INDI\n1 NAME Anne /Hartwell/\n", "\n2 SNOTE @N2@\n"},
		{" FAM\n", "\n1 SNOTE @N2@\n"},
	} {
		if !slices.ContainsFunc(strings.Split(exported, "\n0 "), func(rec string) bool {
			return strings.Contains(rec, want.record) && strings.Contains(rec+"\n", want.line)
		}) {
			t.Errorf("7.0 export has no record with %q holding %q:\n%s", want.record, strings.TrimSpace(want.line), exported)
		}
	}
	if bad := noteRecordOrPointer.FindAllString(exported, -1); len(bad) > 0 {
		t.Errorf("7.0 export has NOTE records or pointers %q:\n%s", bad, exported)
	}
}

// assertGedcom7FixtureImported checks the first import kept what the 7.0
// fixture exercises, so the round trip cannot pass by losing it both times.
func assertGedcom7FixtureImported(t *testing.T, ctx context.Context, rs repository.ReadModelStore) {
	t.Helper()
	dump := roundTripDump(t, ctx, rs)
	for _, want := range []string{
		`person-exids/William Hartwell|JULIAN 14 FEB 1689`,
		`person-exids/Thomas Hartwell|ABT 1714`,
		`family-exids/William Hartwell|JULIAN 14 FEB 1689 + Sarah Cohen|HEBREW 15 NSN 5455`,
		`source-exids/Parish Register of St. Mary`,
		`repository-exids/State Archive`,
	} {
		if dump[want] == "" {
			t.Errorf("first import has no %s; dump keys = %v", want, sortedKeys(dump))
		}
	}

	notes, err := repository.ListAll(ctx, 100, rs.ListNotes)
	if err != nil {
		t.Fatalf("list notes: %v", err)
	}
	byXref := map[string]repository.NoteReadModel{}
	for _, n := range notes {
		byXref[n.GedcomXref] = n
	}
	if got := byXref["@N1@"].Text; got != "Family tradition holds that the Hartwell farm\nwas first settled in 1690.\n\nLater deeds confirm the purchase." {
		t.Errorf("multi-line SNOTE text = %q", got)
	}
	if n := byXref["@N2@"]; n.MIME != "text/html" || n.Language != "en" || len(n.Translations) != 1 {
		t.Errorf("SNOTE with metadata = %+v, want MIME, LANG and one translation", n)
	}

	william := findPerson(t, ctx, rs, "William")
	if william.Notes != "@N1@" {
		t.Errorf("William's Notes = %q, want his SNOTE pointer", william.Notes)
	}
	if gd := domain.ParseGenDate(william.DeathDateRaw); gd.Qualifier != domain.DateInt || gd.InterpretedFrom != "about seventeen fifty" {
		t.Errorf("William's death date %q parses to %+v, want an INT date with its phrase", william.DeathDateRaw, gd)
	}
	// Each calendar date sorts by its Gregorian equivalent.
	for given, want := range map[string]string{
		"William": "1689-02-24", // Julian 14 FEB 1689
		"Sarah":   "1695-03-31", // 15 Nisan 5455
		"Jacques": "1793-09-22", // 1 Vendemiaire II
	} {
		p := findPerson(t, ctx, rs, given)
		if got := sortDay(p.BirthDateSort); got != want {
			t.Errorf("%s's birth date %q sorts as %q, want %q", given, p.BirthDateRaw, got, want)
		}
	}
}

// roundTripDump renders the main-branch tree a GEDCOM import stores: people
// and their names, families and their children, life events, attributes,
// sources, citations, repositories, shared notes and external ids. A row is
// keyed by a label built from its content rather than its id, and rendered
// without ids, versions, timestamps or xrefs, with each id it refers to
// replaced by that row's label and each note pointer by the note's. Two
// imports of the same tree therefore dump the same, whatever ids they got.
func roundTripDump(t *testing.T, ctx context.Context, rs repository.ReadModelStore) map[string]string {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("dump round-trip tree: %v", err)
		}
	}
	labels := map[uuid.UUID]string{}
	label := func(id uuid.UUID, l string) {
		t.Helper()
		for other, existing := range labels {
			if existing == l && other != id {
				t.Fatalf("two rows share the label %q", l)
			}
		}
		labels[id] = l
	}
	type row struct {
		key string
		v   any
	}
	var rows []row
	add := func(key string, v any) { rows = append(rows, row{key, v}) }

	notes, err := repository.ListAll(ctx, 100, rs.ListNotes)
	must(err)
	noteLabels := map[string]string{}
	for _, n := range notes {
		first, _, _ := strings.Cut(n.Text, "\n")
		noteLabels[n.GedcomXref] = "note:" + first
		label(n.ID, first)
		add("note/"+first, n)
	}

	persons, err := repository.ListAll(ctx, 100, rs.ListPersons)
	must(err)
	for _, p := range persons {
		l := p.FullName + "|" + p.BirthDateRaw
		label(p.ID, l)
		add("person/"+l, p)
		add("person-sort/"+l, []string{sortDay(p.BirthDateSort), sortDay(p.DeathDateSort)})
		names, err := rs.GetPersonNames(ctx, domain.MainBranchID, p.ID)
		must(err)
		add("person-names/"+l, names)
		ids, err := rs.GetPersonExternalIDs(ctx, domain.MainBranchID, p.ID)
		must(err)
		if len(ids) > 0 {
			add("person-exids/"+l, ids)
		}
	}

	families, err := repository.ListAll(ctx, 100, rs.ListFamilies)
	must(err)
	for _, f := range families {
		l := labelOf(labels, f.Partner1ID) + " + " + labelOf(labels, f.Partner2ID)
		label(f.ID, l)
		add("family/"+l, f)
		add("family-sort/"+l, sortDay(f.MarriageDateSort))
		children, err := rs.GetFamilyChildren(ctx, domain.MainBranchID, f.ID)
		must(err)
		add("family-children/"+l, children)
		ids, err := rs.GetFamilyExternalIDs(ctx, domain.MainBranchID, f.ID)
		must(err)
		if len(ids) > 0 {
			add("family-exids/"+l, ids)
		}
	}

	events, err := repository.ListAll(ctx, 100, rs.ListEvents)
	must(err)
	for _, e := range events {
		l := labels[e.OwnerID] + " " + string(e.FactType) + " " + e.DateRaw
		label(e.ID, l)
		add("event/"+l, e)
		add("event-sort/"+l, sortDay(e.DateSort))
	}
	attributes, err := repository.ListAll(ctx, 100, rs.ListAttributes)
	must(err)
	for _, a := range attributes {
		l := labels[a.PersonID] + " " + string(a.FactType) + " " + a.Value
		label(a.ID, l)
		add("attribute/"+l, a)
	}

	repos, err := repository.ListAll(ctx, 100, rs.ListRepositories)
	must(err)
	for _, r := range repos {
		label(r.ID, r.Name)
		add("repository/"+r.Name, r)
		ids, err := rs.GetRepositoryExternalIDs(ctx, r.ID)
		must(err)
		if len(ids) > 0 {
			add("repository-exids/"+r.Name, ids)
		}
	}
	sources, err := repository.ListAll(ctx, 100, rs.ListSources)
	must(err)
	for _, s := range sources {
		label(s.ID, s.Title)
		add("source/"+s.Title, s)
		ids, err := rs.GetSourceExternalIDs(ctx, domain.MainBranchID, s.ID)
		must(err)
		if len(ids) > 0 {
			add("source-exids/"+s.Title, ids)
		}
	}
	citations, err := repository.ListAll(ctx, 100, rs.ListCitations)
	must(err)
	for _, c := range citations {
		l := labels[c.SourceID] + " " + c.Page + " for " + string(c.FactType) + " of " + labels[c.FactOwnerID]
		label(c.ID, l)
		add("citation/"+l, c)
	}

	out := map[string]string{}
	for _, r := range rows {
		if _, dup := out[r.key]; dup {
			t.Fatalf("two rows dump under %q", r.key)
		}
		out[r.key] = roundTripRowJSON(t, r.v, labels, noteLabels)
	}
	return out
}

// roundTripVolatileKeys are the row fields an import sets anew each time.
var roundTripVolatileKeys = map[string]bool{"id": true, "version": true, "created_at": true, "updated_at": true, "gedcom_xref": true}

var notePointer = regexp.MustCompile(`@[^@\s]+@`)

func roundTripRowJSON(t *testing.T, v any, labels map[uuid.UUID]string, noteLabels map[string]string) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal row: %v", err)
	}
	var generic any
	if err := json.Unmarshal(b, &generic); err != nil {
		t.Fatalf("unmarshal row: %v", err)
	}
	var walk func(any) any
	walk = func(v any) any {
		switch val := v.(type) {
		case map[string]any:
			for k, item := range val {
				if roundTripVolatileKeys[k] {
					delete(val, k)
					continue
				}
				val[k] = walk(item)
			}
			return val
		case []any:
			for i, item := range val {
				val[i] = walk(item)
			}
			return val
		case string:
			if id, err := uuid.Parse(val); err == nil {
				l, ok := labels[id]
				if !ok {
					t.Fatalf("row %s refers to %s, which the dump has no row for", b, id)
				}
				return l
			}
			return notePointer.ReplaceAllStringFunc(val, func(xref string) string {
				if l, ok := noteLabels[xref]; ok {
					return l
				}
				return xref
			})
		default:
			return v
		}
	}
	out, err := json.Marshal(walk(generic))
	if err != nil {
		t.Fatalf("re-marshal row: %v", err)
	}
	return string(out)
}

func assertSameRoundTripDump(t *testing.T, want, got map[string]string) {
	t.Helper()
	for _, k := range sortedKeys(want, got) {
		if want[k] != got[k] {
			t.Errorf("%s differs after the round trip\n first import:  %s\n second import: %s", k, want[k], got[k])
		}
	}
}

func sortedKeys(maps ...map[string]string) []string {
	seen := map[string]bool{}
	var keys []string
	for _, m := range maps {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	sort.Strings(keys)
	return keys
}

func labelOf(labels map[uuid.UUID]string, id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return labels[*id]
}

func sortDay(ts *time.Time) string {
	if ts == nil {
		return ""
	}
	return ts.UTC().Format("2006-01-02")
}
