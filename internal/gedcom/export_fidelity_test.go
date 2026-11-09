package gedcom_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	gcgedcom "github.com/cacack/gedcom-go/v3/gedcom"
	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

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
			for _, want := range []string{"1 NOTE @N1@\n", "1 NOTE @N2@\n"} {
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
			// Bob's multi-line note is not re-checked: the importer keeps only
			// a NOTE's first line (#1202).
			if got["Ann"] != twoPointers {
				t.Errorf("re-imported person notes = %q, want Ann %q", got, twoPointers)
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

	for _, tt := range []struct {
		version gcgedcom.Version
		line    string
	}{
		{gcgedcom.Version70, "1 EXID KWCJ-QN7\n2 TYPE https://www.familysearch.org/ark\n"},
		{gcgedcom.Version551, "1 _FSFTID KWCJ-QN7\n"},
	} {
		t.Run(string(tt.version), func(t *testing.T) {
			buf := &bytes.Buffer{}
			if _, err := gedcom.NewExporter(readStore).ExportWithOptions(ctx, buf, gedcom.ExportOptions{TargetVersion: tt.version}); err != nil {
				t.Fatalf("Export failed: %v", err)
			}
			output := buf.String()
			if !strings.Contains(output, tt.line) {
				t.Errorf("export should contain %q; got:\n%s", tt.line, output)
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
