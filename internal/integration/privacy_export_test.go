package integration_test

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/exporter"
	"github.com/cacack/my-family/internal/repository"
)

// The JSON, CSV and GEDCOM exports apply the exporting principal's privacy
// (ADR-008 §3–5, #1516), on every backend. The exports read the read model
// through the privacy decorator, so a principal who may not see living
// people or private records finds neither in any export, while an editor or
// owner gets the whole tree, the private marks written as RESN.

// exportPrivacyGedcom is a tree with a dead couple (Paul and Dora), a living
// wife and child of Paul's (Lena and Kit), a private person (Petra) in a
// private family, a private source cited on Dora's birth and a private note.
// Every name, date, place and text that one principal or another may not see
// is distinct, so a test can look for it in an export's text.
const exportPrivacyGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @S1@ SOUR
1 TITL Open register
0 @S2@ SOUR
1 TITL Sealed register
1 _RESN confidential
0 @N1@ NOTE Sealed note text
1 _RESN confidential
0 @I1@ INDI
1 NAME Dora /Deadwood/
1 SEX F
1 BIRT
2 DATE 2 FEB 1850
2 PLAC Oldtown
2 SOUR @S1@
3 PAGE dora page
2 SOUR @S2@
3 PAGE sealed page
1 DEAT
2 DATE 3 MAR 1920
1 NOTE @N1@
0 @I2@ INDI
1 NAME Paul /Deadwood/
1 SEX M
1 BIRT
2 DATE 4 APR 1849
2 PLAC Oldtown
1 DEAT
2 DATE 5 MAY 1919
0 @I3@ INDI
1 NAME Lena /Livingston/
1 NAME Lena /Maidenly/
2 TYPE birth
1 SEX F
1 BIRT
2 DATE 12 MAR 1990
2 PLAC Newtown
2 SOUR @S1@
3 PAGE lena page
1 OCCU Pilot
1 NOTE Lena diary entry
0 @I4@ INDI
1 NAME Kit /Livingston/
1 SEX M
1 BIRT
2 DATE 3 JUN 2015
2 PLAC Kidtown
0 @I5@ INDI
1 RESN confidential
1 NAME Petra /Secretson/
1 SEX F
1 BIRT
2 DATE 9 SEP 1851
2 PLAC Secretville
1 DEAT
2 DATE 5 OCT 1921
0 @F1@ FAM
1 HUSB @I2@
1 WIFE @I1@
1 MARR
2 DATE 6 JUN 1870
2 PLAC Oldchapel
0 @F2@ FAM
1 HUSB @I2@
1 WIFE @I3@
1 CHIL @I4@
1 MARR
2 DATE 7 JUL 2012
2 PLAC Weddingville
0 @F3@ FAM
1 RESN confidential
1 HUSB @I2@
1 WIFE @I5@
1 MARR
2 DATE 4 APR 1875
2 PLAC Hiddenchapel
0 TRLR
`

// Texts from exportPrivacyGedcom by who may see them.
var (
	// exportedToAll is what every principal's export carries.
	exportedToAll = []string{"Dora", "Paul", "Deadwood", "2 FEB 1850", "Oldtown", "Open register", "dora page", "Oldchapel", "6 JUN 1870"}
	// exportedLiving names, dates and places the living, or holds their facts.
	exportedLiving = []string{"Lena", "Kit", "Livingston", "Maidenly", "12 MAR 1990", "Newtown", "lena page", "Pilot",
		"Lena diary entry", "3 JUN 2015", "Kidtown", "7 JUL 2012", "Weddingville"}
	// exportedPrivate is the private records and what they hold.
	exportedPrivate = []string{"Petra", "Secretson", "9 SEP 1851", "Secretville", "5 OCT 1921", "Hiddenchapel", "4 APR 1875",
		"Sealed register", "sealed page", "Sealed note text"}
)

// exportPrincipal is a principal exporting the tree, and what it may see.
type exportPrincipal struct {
	name                    string
	ctx                     context.Context
	seesLiving, seesPrivate bool
}

func exportPrincipals() []exportPrincipal {
	as := func(p repository.Principal, viewersSeeLiving bool) context.Context {
		settings := repository.DefaultTreeSettings()
		settings.ViewersSeeLiving = viewersSeeLiving
		return repository.ContextWithTreeSettings(repository.ContextWithPrincipal(context.Background(), p), settings)
	}
	return []exportPrincipal{
		{"owner", as(repository.Principal{Role: repository.RoleOwner}, false), true, true},
		{"editor", as(repository.Principal{Role: repository.RoleEditor}, false), true, true},
		{"viewer", as(repository.Principal{Role: repository.RoleViewer}, true), true, false},
		{"viewer without living", as(repository.Principal{Role: repository.RoleViewer}, false), false, false},
		{"visitor", as(repository.Principal{Role: repository.RoleViewer, Anonymous: true}, true), false, false},
	}
}

// shownAndHidden splits the tree's texts into those p's export must carry
// and those it must not.
func (p exportPrincipal) shownAndHidden() (shown, hidden []string) {
	shown = slices.Clone(exportedToAll)
	for _, group := range []struct {
		texts []string
		sees  bool
	}{{exportedLiving, p.seesLiving}, {exportedPrivate, p.seesPrivate}} {
		if group.sees {
			shown = append(shown, group.texts...)
		} else {
			hidden = append(hidden, group.texts...)
		}
	}
	return shown, hidden
}

// exportBody returns the text of an export handler's response: the body of
// a CSV or GEDCOM response, the JSON of any other.
func exportBody(t *testing.T, resp any, err error) string {
	t.Helper()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if v := reflect.ValueOf(resp); v.Kind() == reflect.Struct {
		if f := v.FieldByName("Body"); f.IsValid() {
			body := f.Interface().(io.Reader)
			data, err := io.ReadAll(body)
			if err != nil {
				t.Fatalf("read export: %v", err)
			}
			return string(data)
		}
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("encode export %T: %v", resp, err)
	}
	return string(data)
}

// jsonExports returns each JSON export as ctx's principal makes it, by
// endpoint.
func jsonExports(t *testing.T, ctx context.Context, ss *api.StrictServer) map[string]string {
	t.Helper()
	out := map[string]string{}
	resp, err := ss.ExportTree(ctx, api.ExportTreeRequestObject{})
	out["tree"] = exportBody(t, resp, err)
	persons, err := ss.ExportPersons(ctx, api.ExportPersonsRequestObject{})
	out["persons"] = exportBody(t, persons, err)
	families, err := ss.ExportFamilies(ctx, api.ExportFamiliesRequestObject{})
	out["families"] = exportBody(t, families, err)
	sources, err := ss.ExportSources(ctx, api.ExportSourcesRequestObject{})
	out["sources"] = exportBody(t, sources, err)
	citations, err := ss.ExportCitations(ctx, api.ExportCitationsRequestObject{})
	out["citations"] = exportBody(t, citations, err)
	events, err := ss.ExportEvents(ctx, api.ExportEventsRequestObject{})
	out["events"] = exportBody(t, events, err)
	attributes, err := ss.ExportAttributes(ctx, api.ExportAttributesRequestObject{})
	out["attributes"] = exportBody(t, attributes, err)
	return out
}

// csvExports returns each CSV export, with every field it offers, as ctx's
// principal makes it, by endpoint.
func csvExports(t *testing.T, ctx context.Context, ss *api.StrictServer) map[string]string {
	t.Helper()
	format := api.ExportFormatCsv
	fields := func(available map[string]bool) *api.ExportFields {
		all := slices.Sorted(maps.Keys(available))
		return &all
	}
	out := map[string]string{}
	persons, err := ss.ExportPersons(ctx, api.ExportPersonsRequestObject{Params: api.ExportPersonsParams{Format: &format, Fields: fields(exporter.AvailablePersonFields)}})
	out["persons"] = exportBody(t, persons, err)
	families, err := ss.ExportFamilies(ctx, api.ExportFamiliesRequestObject{Params: api.ExportFamiliesParams{Format: &format, Fields: fields(exporter.AvailableFamilyFields)}})
	out["families"] = exportBody(t, families, err)
	sources, err := ss.ExportSources(ctx, api.ExportSourcesRequestObject{Params: api.ExportSourcesParams{Format: &format, Fields: fields(exporter.AvailableSourceFields)}})
	out["sources"] = exportBody(t, sources, err)
	citations, err := ss.ExportCitations(ctx, api.ExportCitationsRequestObject{Params: api.ExportCitationsParams{Format: &format, Fields: fields(exporter.AvailableCitationFields)}})
	out["citations"] = exportBody(t, citations, err)
	events, err := ss.ExportEvents(ctx, api.ExportEventsRequestObject{Params: api.ExportEventsParams{Format: &format, Fields: fields(exporter.AvailableEventFields)}})
	out["events"] = exportBody(t, events, err)
	attributes, err := ss.ExportAttributes(ctx, api.ExportAttributesRequestObject{Params: api.ExportAttributesParams{Format: &format, Fields: fields(exporter.AvailableAttributeFields)}})
	out["attributes"] = exportBody(t, attributes, err)
	return out
}

// gedcomExport returns the GEDCOM export as ctx's principal makes it.
func gedcomExport(t *testing.T, ctx context.Context, ss *api.StrictServer) map[string]string {
	t.Helper()
	resp, err := ss.ExportGedcom(ctx, api.ExportGedcomRequestObject{})
	return map[string]string{"gedcom": exportBody(t, resp, err)}
}

// notInJSONOrCSV is what the JSON and CSV exports carry for no one: name
// variants and shared notes.
var notInJSONOrCSV = []string{"Maidenly", "Sealed note text"}

// assertExportPrivacy checks that no export in bodies carries a text p may
// not see, and that together they carry every text p may see, but those of
// a kind the format does not export (notExported).
func assertExportPrivacy(t *testing.T, p exportPrincipal, bodies map[string]string, notExported ...string) {
	t.Helper()
	shown, hidden := p.shownAndHidden()
	shown = slices.DeleteFunc(shown, func(text string) bool { return slices.Contains(notExported, text) })
	for name, body := range bodies {
		for _, text := range hidden {
			if strings.Contains(body, text) {
				t.Errorf("%s export holds %q, which %s may not see:\n%s", name, text, p.name, body)
			}
		}
	}
	all := strings.Join(slices.Collect(maps.Values(bodies)), "\n")
	for _, text := range shown {
		if !strings.Contains(all, text) {
			t.Errorf("no export holds %q, which %s may see", text, p.name)
		}
	}
}

func TestPrivacy_ExportsApplyThePrincipalsPrivacy(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			if _, err := h.ImportGedcom(context.Background(), command.ImportGedcomInput{Filename: "p.ged", Reader: strings.NewReader(exportPrivacyGedcom)}); err != nil {
				t.Fatalf("import: %v", err)
			}
			ss := api.NewStrictServer(newServer(t, st))

			for _, p := range exportPrincipals() {
				t.Run(p.name, func(t *testing.T) {
					t.Run("JSON", func(t *testing.T) { assertExportPrivacy(t, p, jsonExports(t, p.ctx, ss), notInJSONOrCSV...) })
					t.Run("CSV", func(t *testing.T) { assertExportPrivacy(t, p, csvExports(t, p.ctx, ss), notInJSONOrCSV...) })
					t.Run("GEDCOM", func(t *testing.T) {
						out := gedcomExport(t, p.ctx, ss)
						assertExportPrivacy(t, p, out)
						assertGedcomPrivacyShape(t, p, out["gedcom"])
					})
				})
			}
		})
	}
}

// assertGedcomPrivacyShape checks how the GEDCOM export marks and shapes
// what p may see: an editor's or owner's carries the private marks as RESN;
// a principal who may not see the living gets them as placeholders, so the
// living family keeps its partners and child; the private family is absent
// for anyone who may not see private records, and so is Petra, whose only
// family it is.
func assertGedcomPrivacyShape(t *testing.T, p exportPrincipal, out string) {
	t.Helper()
	if p.seesPrivate {
		for _, want := range []struct{ record, line string }{
			{"Petra /Secretson/", "1 RESN confidential"},
			{"Sealed register", "1 _RESN confidential"},
			{"Sealed note text", "1 _RESN confidential"},
		} {
			if lines := recordLines(out, want.record); !slices.Contains(lines, want.line) {
				t.Errorf("record %q exported as %q, want a %q line", want.record, lines, want.line)
			}
		}
	}
	families := strings.Count(out, " FAM\n")
	placeholders := strings.Count(out, "1 NAME Living //")
	wantFamilies, wantPlaceholders := 3, 0
	if !p.seesPrivate {
		wantFamilies = 2
	}
	if !p.seesLiving {
		wantPlaceholders = 2 // Lena and Kit
	}
	if families != wantFamilies || placeholders != wantPlaceholders {
		t.Errorf("%s export has %d families and %d placeholders, want %d and %d:\n%s", p.name, families, placeholders, wantFamilies, wantPlaceholders, out)
	}
	// Every pointer resolves: the family of a placeholder still names its
	// partners and child.
	if got, want := strings.Count(out, "1 CHIL @"), 1; got != want {
		t.Errorf("%s export has %d CHIL lines, want %d:\n%s", p.name, got, want, out)
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && slices.Contains([]string{"HUSB", "WIFE", "CHIL", "FAMS", "FAMC"}, fields[1]) {
			if !strings.Contains(out, "0 "+fields[2]+" ") {
				t.Errorf("%s export: %q points to no record", p.name, line)
			}
		}
	}
}
