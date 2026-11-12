package gedcom_test

import (
	"bytes"
	"context"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cacack/gedcom-go/v3/decoder"
	gcgedcom "github.com/cacack/gedcom-go/v3/gedcom"
	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// notePointerTag is the tag a pointer to a shared note is written with at
// version: GEDCOM 7.0's NOTE takes text, so 7.0 points with SNOTE (#622).
func notePointerTag(version gcgedcom.Version) string {
	if version == gcgedcom.Version70 {
		return "SNOTE"
	}
	return "NOTE"
}

func TestExport_MultipleNotePointers(t *testing.T) {
	// The importer joins a record's NOTE values into one Notes string, so a
	// record with two shared-note pointers stores "@N1@\n\n@N2@". Export must
	// write one NOTE pointer per note, not one note with the second pointer
	// on a CONT line (#1201).
	readStore := memory.NewReadModelStore()
	ctx := context.Background()
	for _, n := range []struct{ xref, text string }{{"@N1@", "First note"}, {"@N2@", "Second note"}} {
		if err := readStore.SaveNote(ctx, domain.MainBranchID, &repository.NoteReadModel{
			ID: uuid.New(), Text: n.text, GedcomXref: n.xref,
		}); err != nil {
			t.Fatal(err)
		}
	}
	const twoPointers = "@N1@\n\n@N2@"
	const inlineOnly = "Line one\n\nLine two"
	if err := readStore.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
		ID: uuid.New(), GivenName: "Ann", Surname: "Smith", Notes: twoPointers,
	}); err != nil {
		t.Fatal(err)
	}
	if err := readStore.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
		ID: uuid.New(), GivenName: "Bob", Surname: "Jones", Notes: inlineOnly,
	}); err != nil {
		t.Fatal(err)
	}
	if err := readStore.SaveSource(ctx, domain.MainBranchID, &repository.SourceReadModel{
		ID: uuid.New(), SourceType: "book", Title: "Register", Notes: twoPointers,
	}); err != nil {
		t.Fatal(err)
	}
	if err := readStore.SaveRepository(ctx, &repository.RepositoryReadModel{
		ID: uuid.New(), Name: "Archive", Notes: twoPointers,
	}); err != nil {
		t.Fatal(err)
	}

	for _, version := range []gcgedcom.Version{gcgedcom.Version551, gcgedcom.Version70} {
		t.Run(string(version), func(t *testing.T) {
			buf := &bytes.Buffer{}
			if _, err := gedcom.NewExporter(readStore).ExportWithOptions(ctx, buf, gedcom.ExportOptions{TargetVersion: version}); err != nil {
				t.Fatalf("Export failed: %v", err)
			}
			output := buf.String()
			// Three records (person, source, repository) each point at both notes.
			ptr := notePointerTag(version)
			// 7.0 has no NOTE record: a plain note is written as SNOTE there.
			if want := "0 @N1@ " + ptr + " First note\n"; !strings.Contains(output, want) {
				t.Errorf("export lacks note record %q; got:\n%s", want, output)
			}
			for _, want := range []string{"1 " + ptr + " @N1@\n", "1 " + ptr + " @N2@\n"} {
				if got := strings.Count(output, want); got != 3 {
					t.Errorf("export has %d %q lines, want 3; got:\n%s", got, want, output)
				}
			}
			if strings.Contains(output, "CONT @N2@") {
				t.Errorf("a note pointer must not be written as CONT text; got:\n%s", output)
			}
			if !strings.Contains(output, "1 NOTE Line one\n2 CONT\n2 CONT Line two\n") {
				t.Errorf("an inline-only note should export unchanged; got:\n%s", output)
			}

			_, persons, _, sources, _, repos, _, _, _, _, _, _, _, err := gedcom.NewImporter().Import(ctx, strings.NewReader(output))
			if err != nil {
				t.Fatalf("Re-import failed: %v", err)
			}
			got := map[string]string{}
			for _, p := range persons {
				got[p.GivenName] = p.Notes
			}
			if got["Ann"] != twoPointers || got["Bob"] != inlineOnly {
				t.Errorf("re-imported person notes = %q, want Ann %q and Bob %q", got, twoPointers, inlineOnly)
			}
			if len(sources) != 1 || sources[0].Notes != twoPointers {
				t.Errorf("re-imported source notes = %+v, want %q", sources, twoPointers)
			}
			if len(repos) != 1 || repos[0].Notes != twoPointers {
				t.Errorf("re-imported repository notes = %+v, want %q", repos, twoPointers)
			}
		})
	}
}

func TestExport_MixedNotePointerAndText(t *testing.T) {
	// A record with "1 NOTE @N1@" and "1 NOTE some text" stores
	// "@N1@\n\nsome text". Export must write a pointer line and a separate
	// inline note, not a pointer carrying the text on CONT lines, which
	// re-imports as the bare pointer (#1237).
	readStore := memory.NewReadModelStore()
	ctx := context.Background()
	if err := readStore.SaveNote(ctx, domain.MainBranchID, &repository.NoteReadModel{
		ID: uuid.New(), Text: "Shared note", GedcomXref: "@N1@",
	}); err != nil {
		t.Fatal(err)
	}
	const mixed = "@N1@\n\nsome text"
	// Inline-only text, multi-line (CONT) and over one line's length (CONC),
	// must still export exactly as before.
	long := strings.Repeat("word ", 60) + "end"
	inlineOnly := "Line one\n" + long
	if err := readStore.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
		ID: uuid.New(), GivenName: "Ann", Surname: "Smith", Notes: mixed,
	}); err != nil {
		t.Fatal(err)
	}
	if err := readStore.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
		ID: uuid.New(), GivenName: "Bob", Surname: "Jones", Notes: "plain text",
	}); err != nil {
		t.Fatal(err)
	}
	if err := readStore.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
		ID: uuid.New(), GivenName: "Cy", Surname: "Long", Notes: inlineOnly,
	}); err != nil {
		t.Fatal(err)
	}
	if err := readStore.SaveSource(ctx, domain.MainBranchID, &repository.SourceReadModel{
		ID: uuid.New(), SourceType: "book", Title: "Register", Notes: mixed,
	}); err != nil {
		t.Fatal(err)
	}
	if err := readStore.SaveRepository(ctx, &repository.RepositoryReadModel{
		ID: uuid.New(), Name: "Archive", Notes: mixed,
	}); err != nil {
		t.Fatal(err)
	}

	for _, version := range []gcgedcom.Version{gcgedcom.Version551, gcgedcom.Version70} {
		t.Run(string(version), func(t *testing.T) {
			buf := &bytes.Buffer{}
			if _, err := gedcom.NewExporter(readStore).ExportWithOptions(ctx, buf, gedcom.ExportOptions{TargetVersion: version}); err != nil {
				t.Fatalf("Export failed: %v", err)
			}
			output := buf.String()
			// Person, source and repository each write the pointer with no
			// subordinate CONT, then the text as its own NOTE.
			ptr := notePointerTag(version)
			if got := strings.Count(output, "1 "+ptr+" @N1@\n1 NOTE some text\n"); got != 3 {
				t.Errorf("export has %d pointer+text NOTE pairs, want 3; got:\n%s", got, output)
			}
			if strings.Contains(output, "1 "+ptr+" @N1@\n2 CONT") {
				t.Errorf("a note pointer must not carry CONT lines; got:\n%s", output)
			}
			if !strings.Contains(output, "1 NOTE plain text\n") {
				t.Errorf("a single-line inline note should export unchanged; got:\n%s", output)
			}
			// GEDCOM 7.0 has no CONC, so only 5.x splits the long line
			// (gedcom-go v3.4.0).
			wantConc := version != gcgedcom.Version70
			if !strings.Contains(output, "1 NOTE Line one\n2 CONT word ") || strings.Contains(output, "\n2 CONC ") != wantConc {
				t.Errorf("a multi-line inline note should export as one NOTE with CONT (and CONC: %v); got:\n%s", wantConc, output)
			}

			_, persons, _, sources, _, repos, _, _, _, _, _, _, _, err := gedcom.NewImporter().Import(ctx, strings.NewReader(output))
			if err != nil {
				t.Fatalf("Re-import failed: %v", err)
			}
			got := map[string]string{}
			for _, p := range persons {
				got[p.GivenName] = p.Notes
			}
			if got["Ann"] != mixed || got["Bob"] != "plain text" || got["Cy"] != inlineOnly {
				t.Errorf("re-imported person notes = %q, want Ann %q, Bob %q and Cy %q", got, mixed, "plain text", inlineOnly)
			}
			if len(sources) != 1 || sources[0].Notes != mixed {
				t.Errorf("re-imported source notes = %+v, want %q", sources, mixed)
			}
			if len(repos) != 1 || repos[0].Notes != mixed {
				t.Errorf("re-imported repository notes = %+v, want %q", repos, mixed)
			}
		})
	}
}

func TestExport_GeneratedXrefsAreUnique(t *testing.T) {
	// A generated xref must skip every stored xref of any record type, so a
	// note stored as @N2@ and a note without a stored xref sorted second do
	// not both export as @N2@, and a source stored as @I1@ does not collide
	// with the first person (#1177).
	readStore := memory.NewReadModelStore()
	ctx := context.Background()
	storedNote := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	unstoredNote := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	for _, n := range []*repository.NoteReadModel{
		{ID: storedNote, Text: "Stored note", GedcomXref: "@N2@"},
		{ID: unstoredNote, Text: "Unstored note"},
	} {
		if err := readStore.SaveNote(ctx, domain.MainBranchID, n); err != nil {
			t.Fatal(err)
		}
	}
	if err := readStore.SaveSource(ctx, domain.MainBranchID, &repository.SourceReadModel{
		ID: uuid.New(), SourceType: "book", Title: "Register", GedcomXref: "@I1@",
	}); err != nil {
		t.Fatal(err)
	}
	if err := readStore.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
		ID: uuid.New(), GivenName: "Ann", Surname: "Smith", Notes: "@N2@",
	}); err != nil {
		t.Fatal(err)
	}

	buf := &bytes.Buffer{}
	if _, err := gedcom.NewExporter(readStore).Export(ctx, buf); err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	output := buf.String()
	seen := map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "0" && strings.HasPrefix(fields[1], "@") {
			if seen[fields[1]] {
				t.Errorf("xref %s is used by two records; got:\n%s", fields[1], output)
			}
			seen[fields[1]] = true
		}
	}
	for _, want := range []string{"0 @N2@ NOTE Stored note\n", "0 @I1@ SOUR\n", "0 @I2@ INDI\n"} {
		if !strings.Contains(output, want) {
			t.Errorf("export should contain %q; got:\n%s", want, output)
		}
	}

	result, persons, _, _, _, _, _, _, notes, _, _, _, _, err := gedcom.NewImporter().Import(ctx, strings.NewReader(output))
	if err != nil {
		t.Fatalf("Re-import failed: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("re-import has %d notes, want 2", len(notes))
	}
	if len(persons) != 1 || persons[0].Notes != "@N2@" {
		t.Fatalf("re-imported person notes = %+v, want @N2@", persons)
	}
	for _, n := range notes {
		if n.ID == result.NoteXrefToID["@N2@"] && n.Text != "Stored note" {
			t.Errorf("@N2@ resolves to %q, want the stored note", n.Text)
		}
	}
}

func TestExport_NotePointerResolvesToANoteRecord(t *testing.T) {
	// A source and a note both stored as @N2@: the source keeps @N2@, so a
	// person's NOTE @N2@ must follow the note to its fresh xref rather than
	// point at the SOUR record (#1228).
	readStore := memory.NewReadModelStore()
	ctx := context.Background()
	if err := readStore.SaveSource(ctx, domain.MainBranchID, &repository.SourceReadModel{
		ID: uuid.MustParse("00000000-0000-0000-0000-000000000001"), SourceType: "book", Title: "Register", GedcomXref: "@N2@",
	}); err != nil {
		t.Fatal(err)
	}
	if err := readStore.SaveNote(ctx, domain.MainBranchID, &repository.NoteReadModel{
		ID: uuid.MustParse("00000000-0000-0000-0000-000000000002"), Text: "The note", GedcomXref: "@N2@",
	}); err != nil {
		t.Fatal(err)
	}
	if err := readStore.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
		ID: uuid.New(), GivenName: "Ann", Surname: "Smith", Notes: "@N2@",
	}); err != nil {
		t.Fatal(err)
	}

	buf := &bytes.Buffer{}
	exportResult, err := gedcom.NewExporter(readStore).Export(ctx, buf)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	if len(exportResult.Warnings) != 0 {
		t.Errorf("warnings = %q, want none: only one NOTE holds @N2@", exportResult.Warnings)
	}
	result, persons, _, _, _, _, _, _, notes, _, _, _, _, err := gedcom.NewImporter().Import(ctx, strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("Re-import failed: %v", err)
	}
	if len(persons) != 1 || len(notes) != 1 {
		t.Fatalf("re-import has %d persons and %d notes, want 1 each; got:\n%s", len(persons), len(notes), buf.String())
	}
	if id, ok := result.NoteXrefToID[persons[0].Notes]; !ok || id != notes[0].ID {
		t.Errorf("person's NOTE pointer %q does not resolve to the NOTE record; got:\n%s", persons[0].Notes, buf.String())
	}
}

func TestExport_AmbiguousNotePointerWarns(t *testing.T) {
	// Two notes stored as @N1@ (data imported before #1228): a pointer to
	// @N1@ cannot be resolved, so the export says so and names the xref.
	readStore := memory.NewReadModelStore()
	ctx := context.Background()
	for _, n := range []*repository.NoteReadModel{
		{ID: uuid.New(), Text: "First", GedcomXref: "@N1@"},
		{ID: uuid.New(), Text: "Second", GedcomXref: "@N1@"},
	} {
		if err := readStore.SaveNote(ctx, domain.MainBranchID, n); err != nil {
			t.Fatal(err)
		}
	}
	for _, given := range []string{"Ann", "Bob"} {
		if err := readStore.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
			ID: uuid.New(), GivenName: given, Surname: "Smith", Notes: "@N1@",
		}); err != nil {
			t.Fatal(err)
		}
	}

	exportResult, err := gedcom.NewExporter(readStore).Export(ctx, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	if len(exportResult.Warnings) != 1 || !strings.Contains(exportResult.Warnings[0], "@N1@") {
		t.Errorf("warnings = %q, want one naming @N1@", exportResult.Warnings)
	}
}

func TestExport_FamilySearchIDRoundTrip(t *testing.T) {
	// The importer reads _FSFTID; the person's FamilySearch ID must survive
	// export as a FamilySearch EXID at 7.0 and as _FSFTID at 5.5.1, and
	// re-import to the same IDs (#1175).
	data, err := os.ReadFile("../../testdata/gedcom-5.5/familysearch-sample.ged")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, persons, _, _, _, _, _, _, _, _, _, _, _, err := gedcom.NewImporter().Import(ctx, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	readStore := memory.NewReadModelStore()
	want := map[string]string{}
	for _, p := range persons {
		if err := readStore.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
			ID: p.ID, GivenName: p.GivenName, Surname: p.Surname,
		}); err != nil {
			t.Fatal(err)
		}
		ids := make([]repository.PersonExternalIDReadModel, len(p.ExternalIDs))
		for i, ext := range p.ExternalIDs {
			ids[i] = repository.PersonExternalIDReadModel{PersonID: p.ID, Sequence: i, Value: ext.Value, Type: ext.Type}
		}
		if err := readStore.ReplacePersonExternalIDs(ctx, domain.MainBranchID, p.ID, ids); err != nil {
			t.Fatal(err)
		}
		want[p.GivenName] = externalIDs(p.ExternalIDs)
	}
	if want["John"] != "KWCJ-QN7|https://www.familysearch.org/ark" || want["Jane"] != "ABCD-123|https://www.familysearch.org/ark" || want["William"] != "" {
		t.Fatalf("import external IDs = %q, want the FamilySearch IDs", want)
	}

	// A FamilySearch ID alone does not upgrade the default export to 7.0:
	// 5.5 carries it losslessly as _FSFTID, as it did before the ID was kept.
	for _, tt := range []struct {
		name    string
		version gcgedcom.Version
		vers    string
		line    string
	}{
		{"default", "", "2 VERS 5.5\n", "1 _FSFTID KWCJ-QN7\n"},
		{"7.0", gcgedcom.Version70, "2 VERS 7.0\n", "1 EXID KWCJ-QN7\n2 TYPE https://www.familysearch.org/ark\n"},
		{"5.5.1", gcgedcom.Version551, "2 VERS 5.5.1\n", "1 _FSFTID KWCJ-QN7\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			result, err := gedcom.NewExporter(readStore).ExportWithOptions(ctx, buf, gedcom.ExportOptions{TargetVersion: tt.version})
			if err != nil {
				t.Fatalf("Export failed: %v", err)
			}
			output := buf.String()
			for _, want := range []string{tt.vers, tt.line} {
				if !strings.Contains(output, want) {
					t.Errorf("export should contain %q; got:\n%s", want, output)
				}
			}
			if len(result.DataLoss) != 0 {
				t.Errorf("export reports data loss %+v, want none", result.DataLoss)
			}
			_, again, _, _, _, _, _, _, _, _, _, _, _, err := gedcom.NewImporter().Import(ctx, strings.NewReader(output))
			if err != nil {
				t.Fatalf("Re-import failed: %v", err)
			}
			got := map[string]string{}
			for _, p := range again {
				got[p.GivenName] = externalIDs(p.ExternalIDs)
			}
			for name, w := range want {
				if got[name] != w {
					t.Errorf("re-imported %s external IDs = %q, want %q", name, got[name], w)
				}
			}
		})
	}
}

func TestImport_FamilySearchIDNotDuplicated(t *testing.T) {
	// A person carrying both a FamilySearch EXID and the same _FSFTID keeps
	// one FamilySearch external ID (#1175).
	in := "0 HEAD\n1 GEDC\n2 VERS 7.0\n0 @I1@ INDI\n1 NAME A /B/\n" +
		"1 EXID KWCJ-QN7\n2 TYPE http://www.familysearch.org/ark\n1 _FSFTID KWCJ-QN7\n0 TRLR\n"
	_, persons, _, _, _, _, _, _, _, _, _, _, _, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(in))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if len(persons) != 1 {
		t.Fatalf("len(persons) = %d, want 1", len(persons))
	}
	if got := externalIDs(persons[0].ExternalIDs); got != "KWCJ-QN7|http://www.familysearch.org/ark" {
		t.Errorf("external IDs = %q, want the one EXID", got)
	}
}

// externalIDs renders ids as "value|type" pairs joined by ",".
func externalIDs(ids []domain.ExternalIdentifier) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = id.Value + "|" + id.Type
	}
	return strings.Join(parts, ",")
}

func TestExport_CitationAncestryAPIDRoundTrip(t *testing.T) {
	// An imported citation _APID is kept as a citation external ID; export
	// writes it back as _APID at every version, and re-import gives the same
	// IDs (#1175).
	data, err := os.ReadFile("../../testdata/gedcom-5.5/ancestry-sample.ged")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, _, _, _, citations, _, _, _, _, _, _, _, _, err := gedcom.NewImporter().Import(ctx, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	want := citationExternalIDsByFact(citations)
	const apidType = "|https://www.ancestry.com/apid"
	if want[string(domain.FactPersonBirth)] != "1,7602::2771226"+apidType ||
		want[string(domain.FactPersonDeath)] != "1,9024::12345678"+apidType ||
		want[string(domain.FactFamilyMarriage)] != "1,5678::87654321"+apidType {
		t.Fatalf("import citation external IDs = %q, want the APIDs", want)
	}
	readStore := memory.NewReadModelStore()
	if _, err := command.NewHandler(memory.NewEventStore(), readStore).ImportGedcom(ctx, command.ImportGedcomInput{
		Filename: "ancestry-sample.ged", Reader: bytes.NewReader(data),
	}); err != nil {
		t.Fatalf("ImportGedcom failed: %v", err)
	}

	for _, version := range []gcgedcom.Version{"", gcgedcom.Version551, gcgedcom.Version70} {
		t.Run("version "+string(version), func(t *testing.T) {
			buf := &bytes.Buffer{}
			if _, err := gedcom.NewExporter(readStore).ExportWithOptions(ctx, buf, gedcom.ExportOptions{TargetVersion: version}); err != nil {
				t.Fatalf("Export failed: %v", err)
			}
			output := buf.String()
			for _, line := range []string{"3 _APID 1,7602::2771226\n", "3 _APID 1,9024::12345678\n", "3 _APID 1,5678::87654321\n"} {
				if !strings.Contains(output, line) {
					t.Errorf("export should contain %q; got:\n%s", line, output)
				}
			}
			_, _, _, _, again, _, _, _, _, _, _, _, _, err := gedcom.NewImporter().Import(ctx, strings.NewReader(output))
			if err != nil {
				t.Fatalf("Re-import failed: %v", err)
			}
			if got := citationExternalIDsByFact(again); !maps.Equal(got, want) {
				t.Errorf("re-imported citation external IDs = %q, want %q", got, want)
			}
		})
	}
}

// citationExternalIDsByFact renders each citation's external IDs, keyed by
// its fact type.
func citationExternalIDsByFact(citations []gedcom.CitationData) map[string]string {
	out := map[string]string{}
	for _, c := range citations {
		out[c.FactType] = externalIDs(c.ExternalIDs)
	}
	return out
}

func TestNestedNotes_RoundTrip(t *testing.T) {
	// A NOTE nested under an ASSO or event is not a record note, so the
	// re-exported INDI carries exactly the one level-1 NOTE it had (#1246).
	ctx := context.Background()
	_, persons, _, _, _, _, _, _, notes, _, assocs, _, _, err := gedcom.NewImporter().Import(ctx, strings.NewReader(nestedNotesGedcom))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	readStore := memory.NewReadModelStore()
	for _, p := range persons {
		if err := readStore.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
			ID: p.ID, GivenName: p.GivenName, Surname: p.Surname, Notes: p.Notes,
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range notes {
		if err := readStore.SaveNote(ctx, domain.MainBranchID, &repository.NoteReadModel{
			ID: n.ID, Text: n.Text, GedcomXref: n.GedcomXref,
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, a := range assocs {
		if err := readStore.SaveAssociation(ctx, domain.MainBranchID, &repository.AssociationReadModel{
			ID: a.ID, PersonID: a.PersonID, AssociateID: a.AssociateID, Role: a.Role, Notes: a.Notes, Version: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}

	buf := &bytes.Buffer{}
	if _, err := gedcom.NewExporter(readStore).Export(ctx, buf); err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	output := buf.String()

	// Collect the level-1 NOTE lines of Ann's INDI record.
	var annNotes []string
	inAnn := false
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "0 ") {
			inAnn = false
		}
		if strings.HasPrefix(line, "1 NAME Ann ") {
			inAnn = true
		}
		if inAnn && strings.HasPrefix(line, "1 NOTE") {
			annNotes = append(annNotes, line)
		}
	}
	if len(annNotes) != 1 || annNotes[0] != "1 NOTE @N1@" {
		t.Errorf("Ann's INDI level-1 NOTE lines = %q, want exactly [\"1 NOTE @N1@\"]; got:\n%s", annNotes, output)
	}
	if !strings.Contains(output, "2 NOTE @N3@\n") {
		t.Errorf("the association should keep its NOTE @N3@; got:\n%s", output)
	}
}

func TestExport_ChildPedigreeRoundTrips(t *testing.T) {
	// Every child is exported with a FAMC back to its family (#1374); only an
	// adopted or foster child's carries PEDI, so re-import keeps the
	// relationship (#622). A child row of a family that is not exported gets
	// no FAMC.
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	father, adopted, foster, born := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for id, name := range map[uuid.UUID]string{father: "Father", adopted: "Adopted", foster: "Foster", born: "Born"} {
		if err := readStore.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
			ID: id, GivenName: name, Surname: "Smith", FullName: name + " Smith",
		}); err != nil {
			t.Fatal(err)
		}
	}
	familyID := uuid.New()
	if err := readStore.SaveFamily(ctx, domain.MainBranchID, &repository.FamilyReadModel{ID: familyID, Partner1ID: &father}); err != nil {
		t.Fatal(err)
	}
	want := map[string]domain.ChildRelationType{"Adopted": domain.ChildAdopted, "Foster": domain.ChildFoster, "Born": domain.ChildBiological}
	for id, rel := range map[uuid.UUID]domain.ChildRelationType{adopted: domain.ChildAdopted, foster: domain.ChildFoster, born: domain.ChildBiological} {
		if err := readStore.SaveFamilyChild(ctx, domain.MainBranchID, &repository.FamilyChildReadModel{
			FamilyID: familyID, PersonID: id, RelationshipType: rel,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := readStore.SaveFamilyChild(ctx, domain.MainBranchID, &repository.FamilyChildReadModel{
		FamilyID: uuid.New(), PersonID: born, RelationshipType: domain.ChildBiological,
	}); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		version         gcgedcom.Version
		adoptedPedigree string
	}{
		{gcgedcom.Version551, "adopted"},
		{gcgedcom.Version70, "ADOPTED"},
	} {
		t.Run(string(tc.version), func(t *testing.T) {
			buf := &bytes.Buffer{}
			if _, err := gedcom.NewExporter(readStore).ExportWithOptions(ctx, buf, gedcom.ExportOptions{TargetVersion: tc.version}); err != nil {
				t.Fatalf("Export failed: %v", err)
			}
			output := buf.String()
			if !strings.Contains(output, "1 FAMC @F1@\n2 PEDI "+tc.adoptedPedigree+"\n") {
				t.Errorf("export lacks the adopted child's FAMC with PEDI %s; got:\n%s", tc.adoptedPedigree, output)
			}
			if !strings.Contains(output, "1 NAME Born /Smith/\n1 FAMC @F1@\n0 ") {
				t.Errorf("export lacks the biological child's FAMC without PEDI; got:\n%s", output)
			}
			if got := strings.Count(output, "1 FAMC "); got != 3 {
				t.Errorf("export has %d FAMC lines, want 3 (one per exported child link); got:\n%s", got, output)
			}
			if got := strings.Count(output, "2 PEDI "); got != 2 {
				t.Errorf("export has %d PEDI lines, want 2 (adopted and foster only); got:\n%s", got, output)
			}

			_, persons, families, _, _, _, _, _, _, _, _, _, _, err := gedcom.NewImporter().Import(ctx, strings.NewReader(output))
			if err != nil {
				t.Fatalf("Re-import failed: %v", err)
			}
			if len(families) != 1 {
				t.Fatalf("re-imported %d families, want 1", len(families))
			}
			names := map[uuid.UUID]string{}
			for _, p := range persons {
				names[p.ID] = p.GivenName
			}
			got := map[string]domain.ChildRelationType{}
			for i, id := range families[0].ChildIDs {
				got[names[id]] = families[0].ChildRelTypes[i]
			}
			if !maps.Equal(got, want) {
				t.Errorf("re-imported child relationships = %v, want %v", got, want)
			}
		})
	}
}

func TestExport_SpouseFamilyLinks(t *testing.T) {
	// Each partner of a family is exported with a FAMS pointing back to it,
	// one per family in family record order, at 5.5.1 and 7.0 (#1357).
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	husband, first, second := uuid.New(), uuid.New(), uuid.New()
	for id, name := range map[uuid.UUID]string{husband: "Husband", first: "First", second: "Second"} {
		if err := readStore.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
			ID: id, GivenName: name, Surname: "Smith", FullName: name + " Smith",
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Family ids are ordered so the second marriage is exported first.
	firstMarriage := uuid.MustParse("80000000-0000-0000-0000-000000000000")
	secondMarriage := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	missing := uuid.New() // a partner with no person record
	for _, f := range []repository.FamilyReadModel{
		{ID: firstMarriage, Partner1ID: &husband, Partner2ID: &first},
		{ID: secondMarriage, Partner1ID: &husband, Partner2ID: &second},
		{ID: uuid.MustParse("ffffffff-0000-0000-0000-000000000000"), Partner1ID: &missing},
	} {
		if err := readStore.SaveFamily(ctx, domain.MainBranchID, &f); err != nil {
			t.Fatal(err)
		}
	}

	for _, version := range []gcgedcom.Version{gcgedcom.Version551, gcgedcom.Version70} {
		t.Run(string(version), func(t *testing.T) {
			buf := &bytes.Buffer{}
			if _, err := gedcom.NewExporter(readStore).ExportWithOptions(ctx, buf, gedcom.ExportOptions{TargetVersion: version}); err != nil {
				t.Fatalf("Export failed: %v", err)
			}
			doc, err := decoder.Decode(strings.NewReader(buf.String()))
			if err != nil {
				t.Fatalf("decode export: %v", err)
			}
			got := map[string][]string{}
			for _, indi := range doc.Individuals() {
				for _, link := range indi.SpouseInFamilies {
					if doc.GetFamily(link.FamilyXRef) == nil {
						t.Errorf("%s has FAMS %s, which is not an exported family", indi.FullName(), link.FamilyXRef)
					}
					got[indi.FullName()] = append(got[indi.FullName()], link.FamilyXRef)
				}
			}
			// Families are exported in id order: @F1@ is the second marriage.
			want := map[string][]string{
				"Husband /Smith/": {"@F1@", "@F2@"},
				"Second /Smith/":  {"@F1@"},
				"First /Smith/":   {"@F2@"},
			}
			if !maps.EqualFunc(got, want, slices.Equal) {
				t.Errorf("FAMS links = %v, want %v; got:\n%s", got, want, buf.String())
			}
			// Every partner of every exported family points back to it.
			for _, fam := range doc.Families() {
				for _, partner := range []string{fam.Husband, fam.Wife} {
					indi := doc.GetIndividual(partner)
					if partner != "" && !slices.ContainsFunc(indi.SpouseInFamilies, func(l gcgedcom.FamilyLink) bool { return l.FamilyXRef == fam.XRef }) {
						t.Errorf("partner %s of %s has no FAMS back to it", partner, fam.XRef)
					}
				}
			}
		})
	}
}

func TestExport_DanglingNotePointerDropped(t *testing.T) {
	// A NOTE pointer to no exported note is dropped with a warning naming
	// the record and the pointer: at 7.0 it would otherwise be written as
	// "NOTE @N9@", read as the text "@N9@" (#1353). It arises from a broken
	// reference kept by import (@N9@) and from a note deleted since (@N1@).
	const file = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Ann /Smith/
1 NOTE @N9@
0 @F1@ FAM
1 HUSB @I1@
1 NOTE @N1@
1 NOTE @N2@
0 @N1@ NOTE Deleted note
0 @N2@ NOTE Kept note
0 TRLR
`
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)
	if _, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{
		Filename: "dangling.ged", Reader: strings.NewReader(file),
	}); err != nil {
		t.Fatalf("ImportGedcom failed: %v", err)
	}
	notes, _, err := readStore.ListNotes(ctx, repository.ListOptions{BranchID: domain.MainBranchID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range notes {
		if n.GedcomXref == "@N1@" {
			if err := handler.DeleteNote(ctx, n.ID, n.Version, "test"); err != nil {
				t.Fatalf("DeleteNote failed: %v", err)
			}
		}
	}

	for _, version := range []gcgedcom.Version{gcgedcom.Version551, gcgedcom.Version70} {
		t.Run(string(version), func(t *testing.T) {
			buf := &bytes.Buffer{}
			res, err := gedcom.NewExporter(readStore).ExportWithOptions(ctx, buf, gedcom.ExportOptions{TargetVersion: version})
			if err != nil {
				t.Fatalf("Export failed: %v", err)
			}
			output := buf.String()
			for _, gone := range []string{"@N9@", "@N1@"} {
				if strings.Contains(output, gone) {
					t.Errorf("export should drop the dangling pointer %s; got:\n%s", gone, output)
				}
			}
			if want := "1 " + notePointerTag(version) + " @N2@\n"; !strings.Contains(output, want) {
				t.Errorf("export should keep %q; got:\n%s", want, output)
			}
			for _, want := range []string{"INDI @I1@ points to NOTE @N9@", "FAM @F1@ points to NOTE @N1@"} {
				if !slices.ContainsFunc(res.Warnings, func(w string) bool { return strings.HasPrefix(w, want) }) {
					t.Errorf("warnings = %q, want one starting %q", res.Warnings, want)
				}
			}
		})
	}
}
