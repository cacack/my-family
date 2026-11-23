package gedcom_test

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	gedcomgo "github.com/cacack/gedcom-go/v3/gedcom"
	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// The importer reads an OBJE record's RESN and the person, family and source
// records that link to it, each once (#1435).
func TestImportMediaObject_RestrictionAndOwners(t *testing.T) {
	const input = `0 HEAD
1 GEDC
2 VERS 7.0
0 @M1@ OBJE
1 RESN LOCKED, PRIVACY
1 FILE a.jpg
2 FORM image/jpeg
0 @M2@ OBJE
1 FILE b.jpg
2 FORM image/jpeg
0 @I1@ INDI
1 NAME A /B/
1 OBJE @M1@
1 OBJE @M1@
0 @F1@ FAM
1 HUSB @I1@
1 OBJE @M1@
0 @S1@ SOUR
1 TITL S
1 OBJE @M1@
0 TRLR
`
	result, _, _, _, _, _, _, _, _, _, _, _, media, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(media) != 2 {
		t.Fatalf("len(media) = %d, want 2", len(media))
	}
	m1, m2 := media[0], media[1]
	if m1.Restriction != "LOCKED, PRIVACY" {
		t.Errorf("@M1@ restriction = %q, want LOCKED, PRIVACY", m1.Restriction)
	}
	want := []gedcom.MediaOwner{
		{EntityType: "person", EntityID: result.PersonXrefToID["@I1@"]},
		{EntityType: "family", EntityID: result.FamilyXrefToID["@F1@"]},
		{EntityType: "source", EntityID: result.SourceXrefToID["@S1@"]},
	}
	if !slices.Equal(m1.Owners, want) {
		t.Errorf("@M1@ owners = %+v, want %+v", m1.Owners, want)
	}
	if m2.Restriction != "" || len(m2.Owners) != 0 {
		t.Errorf("@M2@ = restriction %q, owners %+v; want neither", m2.Restriction, m2.Owners)
	}
}

// An inline OBJE link with no title takes its file's path as the title, and
// one with no FILE describes no media and is skipped, even with a 5.5 FORM
// beside it (#1479).
func TestImportInlineMedia_TitleFallback(t *testing.T) {
	const input = `0 HEAD
1 GEDC
2 VERS 5.5.1
0 @I1@ INDI
1 NAME A /B/
1 OBJE
2 FILE photos/a.jpg
3 FORM jpg
1 OBJE
2 TITL Nothing here
1 OBJE
2 FORM bmp
1 OBJE
2 FORM jpg
2 TITL Embedded
2 BLOB
3 CONT .HM.......k.1..F.jwA.Dzzzzw............A....1.........
0 TRLR
`
	result, _, _, _, _, _, _, _, _, _, _, _, media, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(media) != 1 {
		t.Fatalf("media = %+v, want one item", media)
	}
	m := media[0]
	want := []gedcom.MediaOwner{{EntityType: "person", EntityID: result.PersonXrefToID["@I1@"]}}
	if m.Title != "photos/a.jpg" || m.GedcomXref != "" || m.Format != "jpg" || !slices.Equal(m.Owners, want) {
		t.Errorf("inline media = %+v, want photos/a.jpg owned by @I1@ with no xref", m)
	}
}

// In the 5.5 layout a FORM with no FILE beside it decodes to a file with an
// empty path; such an inline OBJE names no file and is skipped (#1479).
func TestImportInlineMedia_FormWithoutFile55(t *testing.T) {
	const input = `0 HEAD
1 GEDC
2 VERS 5.5
0 @I1@ INDI
1 NAME A /B/
1 OBJE
2 FORM bmp
1 OBJE
2 FORM jpg
2 TITL Embedded
1 OBJE
2 FORM jpg
2 FILE photos/a.jpg
2 TITL Kept
0 TRLR
`
	_, _, _, _, _, _, _, _, _, _, _, _, media, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(media) != 1 || media[0].Title != "Kept" || media[0].FileRef != "photos/a.jpg" {
		t.Errorf("media = %+v, want only Kept", media)
	}
}

// OBJE links under citations of a name, an association and an attribute make
// the person an owner (#1479).
func TestImportMediaObject_LinksUnderNameAssociationAttributeCitations(t *testing.T) {
	const input = `0 HEAD
1 GEDC
2 VERS 5.5.1
0 @M1@ OBJE
1 FILE name.jpg
0 @M2@ OBJE
1 FILE asso.jpg
0 @M3@ OBJE
1 FILE attr.jpg
0 @S1@ SOUR
1 TITL Register
0 @I2@ INDI
1 NAME God /Father/
0 @I1@ INDI
1 NAME A /B/
2 SOUR @S1@
3 OBJE @M1@
1 ASSO @I2@
2 RELA Godfather
2 SOUR @S1@
3 OBJE @M2@
1 OCCU Farmer
2 SOUR @S1@
3 OBJE @M3@
0 TRLR
`
	result, _, _, _, _, _, _, _, _, _, _, _, media, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	want := []gedcom.MediaOwner{{EntityType: "person", EntityID: result.PersonXrefToID["@I1@"]}}
	if len(media) != 3 {
		t.Fatalf("media = %+v, want three records", media)
	}
	for _, m := range media {
		if !slices.Equal(m.Owners, want) {
			t.Errorf("%s owners = %+v, want %+v", m.GedcomXref, m.Owners, want)
		}
	}
}

// A 5.5.1 OBJE record carries its restriction as _RESN, which the importer
// reads like RESN.
func TestImportMediaObject_ExtensionRestriction(t *testing.T) {
	const input = `0 HEAD
1 GEDC
2 VERS 5.5.1
0 @M1@ OBJE
1 FILE a.jpg
2 FORM jpg
1 _RESN confidential
0 TRLR
`
	_, _, _, _, _, _, _, _, _, _, _, _, media, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(media) != 1 || media[0].Restriction != "confidential" {
		t.Fatalf("media = %+v, want one with restriction confidential", media)
	}
}

// A 5.5 OBJE record gives its TITL and FORM at level 1, which the importer
// reads when its FILE gives none; a TITL or FORM under FILE takes precedence
// (#1755).
func TestImportMediaObject_RecordLevelTitleAndForm(t *testing.T) {
	const input = `0 HEAD
1 GEDC
2 VERS 5.5
0 @M1@ OBJE
1 FILE photo.jpg
1 FORM jpg
1 TITL Wedding photo
0 @M2@ OBJE
1 FORM bmp
1 TITL Record title
1 FILE scan.tif
2 FORM tif
2 TITL File title
0 @M3@ OBJE
1 TITL No file here
1 FORM pdf
0 @M4@ OBJE
1 FILE bare.jpg
0 TRLR
`
	_, _, _, _, _, _, _, _, _, _, _, _, media, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(media) != 4 {
		t.Fatalf("len(media) = %d, want 4", len(media))
	}
	tests := []struct {
		xref, title, format, fileFormat string
	}{
		{"@M1@", "Wedding photo", "jpg", "jpg"},
		{"@M2@", "File title", "tif", "tif"},
		{"@M3@", "No file here", "pdf", ""},
		{"@M4@", "@M4@", "", ""},
	}
	for i, tt := range tests {
		m := media[i]
		if m.GedcomXref != tt.xref || m.Title != tt.title || m.Format != tt.format || m.MimeType != tt.format {
			t.Errorf("media[%d] = xref %q, title %q, format %q, mime %q; want %q, %q, %q",
				i, m.GedcomXref, m.Title, m.Format, m.MimeType, tt.xref, tt.title, tt.format)
		}
		if len(m.Files) > 0 && m.Files[0].Format != tt.fileFormat {
			t.Errorf("%s file format = %q, want %q", tt.xref, m.Files[0].Format, tt.fileFormat)
		}
	}
}

// mediaExportStore holds a person with an uploaded private photo and a
// family with an imported document whose title and type were edited since.
func mediaExportStore(t *testing.T) *memory.ReadModelStore {
	t.Helper()
	ctx := context.Background()
	rs := memory.NewReadModelStore()
	now := time.Now()
	person := &repository.PersonReadModel{ID: uuid.New(), GivenName: "Ann", Surname: "Lee", Version: 1, UpdatedAt: now}
	family := &repository.FamilyReadModel{ID: uuid.New(), Partner1ID: &person.ID, RelationshipType: domain.RelationMarriage, Version: 1, UpdatedAt: now}
	if err := rs.SavePerson(ctx, domain.MainBranchID, person); err != nil {
		t.Fatalf("SavePerson: %v", err)
	}
	if err := rs.SaveFamily(ctx, domain.MainBranchID, family); err != nil {
		t.Fatalf("SaveFamily: %v", err)
	}
	for _, m := range []*repository.MediaReadModel{
		{
			ID: uuid.New(), EntityType: "person", EntityID: person.ID, Title: "Portrait", MimeType: "image/png",
			MediaType: domain.MediaPhoto, Filename: "portrait.png", FileSize: 1, FileData: []byte{1},
			Restriction: "confidential", Version: 1, CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: uuid.New(), EntityType: "family", EntityID: family.ID, Title: "Marriage record", MediaType: domain.MediaPhoto,
			GedcomXref: "@M7@", Files: []domain.MediaFile{
				{Path: "scans/marriage.tif", Format: "tif", MediaType: "DOCUMENT", Title: "Old title"},
				{Path: "scans/marriage-back.tif", Format: "tif"},
			}, Version: 1, CreatedAt: now, UpdatedAt: now,
		},
	} {
		if err := rs.SaveMedia(ctx, domain.MainBranchID, m); err != nil {
			t.Fatalf("SaveMedia: %v", err)
		}
	}
	return rs
}

// recordOf returns the lines of out's level-0 record whose first line is head.
func recordOf(out, head string) []string {
	var record []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "0 ") {
			if len(record) > 0 && record[0] == head {
				return record
			}
			record = nil
		}
		record = append(record, line)
	}
	return nil
}

// A 5.5 export writes an OBJE record per media item, linked from its owner,
// with the item's title and type on the first file and its restriction as
// _RESN, and stays 5.5 (#1435).
func TestExport_MediaRecords(t *testing.T) {
	var buf bytes.Buffer
	result, err := gedcom.NewExporter(mediaExportStore(t)).Export(context.Background(), &buf)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	out := buf.String()
	if result.Version != gedcomgo.Version55 || result.MediaExported != 2 {
		t.Errorf("version %s, media exported %d; want 5.5 and 2", result.Version, result.MediaExported)
	}

	indi := strings.Join(recordOf(out, "0 @I1@ INDI"), "\n")
	fam := strings.Join(recordOf(out, "0 @F1@ FAM"), "\n")
	if !strings.Contains(indi, "1 OBJE @O1@") || !strings.Contains(fam, "1 OBJE @O2@") {
		t.Errorf("owners do not link their media:\n%s", out)
	}
	for head, want := range map[string]string{
		"0 @O1@ OBJE": "1 FILE portrait.png\n2 FORM image/png\n3 TYPE PHOTO\n2 TITL Portrait\n1 _RESN confidential",
		"0 @O2@ OBJE": "1 FILE scans/marriage.tif\n2 FORM tif\n3 TYPE PHOTO\n2 TITL Marriage record\n1 FILE scans/marriage-back.tif\n2 FORM tif",
	} {
		if got := strings.Join(recordOf(out, head), "\n"); got != head+"\n"+want {
			t.Errorf("%s exported as\n%s\nwant\n%s\n%s", head, got, head, want)
		}
	}
}

// A 7.0 export writes the restriction as the RESN 7.0 defines on OBJE.
func TestExport_MediaRestrictionIn70(t *testing.T) {
	var buf bytes.Buffer
	if _, err := gedcom.NewExporter(mediaExportStore(t)).ExportWithOptions(context.Background(), &buf, gedcom.ExportOptions{TargetVersion: gedcomgo.Version70}); err != nil {
		t.Fatalf("export: %v", err)
	}
	obj := recordOf(buf.String(), "0 @O1@ OBJE")
	if !slices.Contains(obj, "1 RESN confidential") || slices.Contains(obj, "1 _RESN confidential") {
		t.Errorf("7.0 OBJE record = %q, want RESN and no _RESN", obj)
	}
}

// The exported media come back on import with their title, files and
// restriction.
func TestExport_MediaRecordsReimport(t *testing.T) {
	for _, version := range []gedcomgo.Version{gedcomgo.Version55, gedcomgo.Version551, gedcomgo.Version70} {
		t.Run(string(version), func(t *testing.T) {
			var buf bytes.Buffer
			if _, err := gedcom.NewExporter(mediaExportStore(t)).ExportWithOptions(context.Background(), &buf, gedcom.ExportOptions{TargetVersion: version}); err != nil {
				t.Fatalf("export: %v", err)
			}
			_, _, _, _, _, _, _, _, _, _, _, _, media, err := gedcom.NewImporter().Import(context.Background(), &buf)
			if err != nil {
				t.Fatalf("import: %v", err)
			}
			byTitle := map[string]gedcom.MediaData{}
			for _, m := range media {
				byTitle[m.Title] = m
			}
			portrait, record := byTitle["Portrait"], byTitle["Marriage record"]
			if portrait.Restriction != "confidential" || portrait.MediaType != domain.MediaPhoto || len(portrait.Owners) != 1 ||
				len(portrait.Files) != 1 || portrait.Files[0].Path != "portrait.png" {
				t.Errorf("Portrait = %+v, want the private photo portrait.png with one owner", portrait)
			}
			if record.Restriction != "" || record.MediaType != domain.MediaPhoto || len(record.Owners) != 1 || len(record.Files) != 2 {
				t.Errorf("Marriage record = %+v, want the photo with two files and one owner", record)
			}
		})
	}
}

// An item with no file references is written with a FILE naming its filename,
// or, if it has content, its title; one imported with no FILE, and an empty
// file reference, as Ancestry writes, are written with an empty FILE (#1585).
func TestExport_MediaWithoutFileReference(t *testing.T) {
	ctx := context.Background()
	rs := memory.NewReadModelStore()
	now := time.Now()
	person := &repository.PersonReadModel{ID: uuid.New(), GivenName: "Ann", Surname: "Lee", Version: 1, UpdatedAt: now}
	if err := rs.SavePerson(ctx, domain.MainBranchID, person); err != nil {
		t.Fatalf("SavePerson: %v", err)
	}
	for _, m := range []*repository.MediaReadModel{
		{Title: "Scan", MimeType: "image/png", MediaType: domain.MediaPhoto, FileSize: 1, FileData: []byte{1}},
		{Title: "Ancestry photo", MediaType: domain.MediaDocument, GedcomXref: "@M2@", Files: []domain.MediaFile{{Format: "jpg"}}},
		{Title: "@M1@", MediaType: domain.MediaDocument, GedcomXref: "@M1@"},
	} {
		m.ID, m.EntityType, m.EntityID, m.Version, m.CreatedAt, m.UpdatedAt = uuid.New(), "person", person.ID, 1, now, now
		if err := rs.SaveMedia(ctx, domain.MainBranchID, m); err != nil {
			t.Fatalf("SaveMedia: %v", err)
		}
	}

	var buf bytes.Buffer
	if _, err := gedcom.NewExporter(rs).Export(ctx, &buf); err != nil {
		t.Fatalf("export: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"1 FILE Scan\n2 FORM image/png\n3 TYPE PHOTO\n2 TITL Scan",
		"1 FILE\n2 FORM jpg\n2 TITL Ancestry photo",
		"1 FILE\n2 TITL @@M1@",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("export lacks\n%s\nin\n%s", want, out)
		}
	}
}

// The items of a media group are written as one OBJE record linked from each
// owner, except an item edited apart from the others, which is written as its
// own record (#1478).
func TestExport_MediaGroupRecords(t *testing.T) {
	ctx := context.Background()
	rs := memory.NewReadModelStore()
	now := time.Now()
	group := uuid.New()
	for i, title := range []string{"Reunion", "Reunion", "Reunion (cropped)"} {
		person := &repository.PersonReadModel{ID: uuid.New(), GivenName: "P", Surname: strings.Repeat("x", i+1), Version: 1, UpdatedAt: now}
		if err := rs.SavePerson(ctx, domain.MainBranchID, person); err != nil {
			t.Fatalf("SavePerson: %v", err)
		}
		m := &repository.MediaReadModel{ID: uuid.New(), EntityType: "person", EntityID: person.ID, Title: title, MediaType: domain.MediaPhoto,
			GedcomXref: "@M1@", Files: []domain.MediaFile{{Path: "photos/reunion.jpg", Format: "jpg"}},
			Restriction: "confidential", GroupID: &group, Version: 1, CreatedAt: now, UpdatedAt: now}
		if err := rs.SaveMedia(ctx, domain.MainBranchID, m); err != nil {
			t.Fatalf("SaveMedia: %v", err)
		}
	}

	var buf bytes.Buffer
	result, err := gedcom.NewExporter(rs).Export(ctx, &buf)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	out := buf.String()
	if result.MediaExported != 2 {
		t.Errorf("media exported %d, want 2 (the shared record and the edited copy):\n%s", result.MediaExported, out)
	}
	links := map[string]int{}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "1 OBJE @") {
			links[line]++
		}
	}
	for _, xref := range []string{"@O1@", "@O2@"} {
		want := 2
		if slices.Contains(recordOf(out, "0 "+xref+" OBJE"), "2 TITL Reunion (cropped)") {
			want = 1
		}
		if links["1 OBJE "+xref] != want {
			t.Errorf("%s is linked %d times, want %d:\n%s", xref, links["1 OBJE "+xref], want, out)
		}
	}
}

// A person holding two alike items of a group, as a person merge can leave,
// links the group's record once, and a re-import gives that person one item
// (#1507).
func TestExport_MediaGroupLinkedOncePerOwner(t *testing.T) {
	ctx := context.Background()
	rs := memory.NewReadModelStore()
	now := time.Now()
	group := uuid.New()
	survivor := &repository.PersonReadModel{ID: uuid.New(), GivenName: "Ann", Surname: "Lee", Version: 1, UpdatedAt: now}
	other := &repository.PersonReadModel{ID: uuid.New(), GivenName: "Bob", Surname: "Lee", Version: 1, UpdatedAt: now}
	for _, p := range []*repository.PersonReadModel{survivor, other} {
		if err := rs.SavePerson(ctx, domain.MainBranchID, p); err != nil {
			t.Fatalf("SavePerson: %v", err)
		}
	}
	for _, owner := range []uuid.UUID{survivor.ID, survivor.ID, other.ID} {
		m := &repository.MediaReadModel{ID: uuid.New(), EntityType: "person", EntityID: owner, Title: "Reunion", MediaType: domain.MediaPhoto,
			GedcomXref: "@M1@", Files: []domain.MediaFile{{Path: "photos/reunion.jpg", Format: "jpg"}},
			GroupID: &group, Version: 1, CreatedAt: now, UpdatedAt: now}
		if err := rs.SaveMedia(ctx, domain.MainBranchID, m); err != nil {
			t.Fatalf("SaveMedia: %v", err)
		}
	}

	var buf bytes.Buffer
	result, err := gedcom.NewExporter(rs).Export(ctx, &buf)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	out := buf.String()
	if result.MediaExported != 1 {
		t.Fatalf("media exported %d, want 1:\n%s", result.MediaExported, out)
	}
	for _, head := range []string{"0 @I1@ INDI", "0 @I2@ INDI"} {
		var links int
		for _, line := range recordOf(out, head) {
			if line == "1 OBJE @O1@" {
				links++
			}
		}
		if links != 1 {
			t.Errorf("%s links @O1@ %d times, want 1:\n%s", head, links, out)
		}
	}

	imported, _, _, _, _, _, _, _, _, _, _, _, media, err := gedcom.NewImporter().Import(ctx, strings.NewReader(out))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(media) != 1 || len(media[0].Owners) != 2 {
		t.Fatalf("re-imported media = %+v, want one record with two owners", media)
	}
	for _, xref := range []string{"@I1@", "@I2@"} {
		id := imported.PersonXrefToID[xref]
		if n := slices.IndexFunc(media[0].Owners, func(o gedcom.MediaOwner) bool { return o.EntityID == id }); n < 0 {
			t.Errorf("%s is not an owner of the re-imported record: %+v", xref, media[0].Owners)
		}
	}
}

// Media linked below record level or inline imports as items of the record
// that contains the link; an export writes each as an OBJE record linked from
// that owner at record level, and re-importing it gives each owner the same
// number of items (#1479).
func TestExport_NestedAndInlineMediaRoundTrip(t *testing.T) {
	const input = `0 HEAD
1 GEDC
2 VERS 5.5.1
2 FORM LINEAGE-LINKED
1 CHAR UTF-8
0 @M1@ OBJE
1 FILE birth.jpg
2 FORM jpg
2 TITL Birth certificate
0 @M2@ OBJE
1 FILE marriage.jpg
2 FORM jpg
2 TITL Marriage record
0 @M3@ OBJE
1 FILE census.jpg
2 FORM jpg
2 TITL Census page
0 @S1@ SOUR
1 TITL Census
0 @I1@ INDI
1 NAME John /Doe/
1 OBJE @M1@
1 OBJE
2 FILE photos/john.jpg
3 FORM jpg
2 TITL John at ten
1 BIRT
2 OBJE @M1@
2 SOUR @S1@
3 OBJE @M3@
1 OCCU Farmer
2 OBJE
3 FILE scans/work.pdf
4 FORM pdf
3 TITL Work record
1 FAMS @F1@
0 @F1@ FAM
1 HUSB @I1@
1 MARR
2 OBJE @M2@
0 TRLR
`
	ctx := context.Background()
	rs := memory.NewReadModelStore()
	if _, err := command.NewHandler(memory.NewEventStore(), rs).ImportGedcom(ctx, command.ImportGedcomInput{
		Filename: "m.ged", Reader: strings.NewReader(input),
	}); err != nil {
		t.Fatalf("import: %v", err)
	}
	const personItems, familyItems = 4, 1 // @M1@, @M3@ and two inline; @M2@

	for _, version := range []gedcomgo.Version{gedcomgo.Version55, gedcomgo.Version551, gedcomgo.Version70} {
		t.Run(string(version), func(t *testing.T) {
			var buf bytes.Buffer
			result, err := gedcom.NewExporter(rs).ExportWithOptions(ctx, &buf, gedcom.ExportOptions{TargetVersion: version})
			if err != nil {
				t.Fatalf("export: %v", err)
			}
			out := buf.String()
			if result.MediaExported != personItems+familyItems {
				t.Errorf("media exported %d, want %d:\n%s", result.MediaExported, personItems+familyItems, out)
			}
			links := func(head string) int {
				n := 0
				for _, line := range recordOf(out, head) {
					if strings.HasPrefix(line, "1 OBJE @") {
						n++
					}
				}
				return n
			}
			if p, f := links("0 @I1@ INDI"), links("0 @F1@ FAM"); p != personItems || f != familyItems {
				t.Errorf("record-level OBJE links: person %d, family %d; want %d and %d:\n%s", p, f, personItems, familyItems, out)
			}

			imported, _, _, _, _, _, _, _, _, _, _, _, media, err := gedcom.NewImporter().Import(ctx, strings.NewReader(out))
			if err != nil {
				t.Fatalf("re-import: %v", err)
			}
			perOwner := map[uuid.UUID]int{}
			for _, m := range media {
				for _, o := range m.Owners {
					perOwner[o.EntityID]++
				}
			}
			if p, f := perOwner[imported.PersonXrefToID["@I1@"]], perOwner[imported.FamilyXrefToID["@F1@"]]; p != personItems || f != familyItems {
				t.Errorf("re-imported items: person %d, family %d; want %d and %d", p, f, personItems, familyItems)
			}
		})
	}
}

// mediaListCounter counts the media list calls an export makes.
type mediaListCounter struct {
	repository.ReadModelStore
	calls int
}

func (c *mediaListCounter) ListMediaForEntity(ctx context.Context, entityType string, entityID uuid.UUID, opts repository.ListOptions) ([]repository.MediaReadModel, int, error) {
	c.calls++
	return c.ReadModelStore.ListMediaForEntity(ctx, entityType, entityID, opts)
}

func (c *mediaListCounter) ListMediaInGroup(ctx context.Context, branchID domain.BranchID, groupID uuid.UUID) ([]repository.MediaReadModel, error) {
	c.calls++
	return c.ReadModelStore.ListMediaInGroup(ctx, branchID, groupID)
}

func (c *mediaListCounter) ListAllMedia(ctx context.Context, branchID domain.BranchID) ([]repository.MediaReadModel, error) {
	c.calls++
	return c.ReadModelStore.ListAllMedia(ctx, branchID)
}

// An export lists media a number of times that does not grow with the number
// of people, families and sources (#1480).
func TestExport_MediaReadsDoNotGrowWithOwners(t *testing.T) {
	exportCalls := func(people int) int {
		t.Helper()
		ctx := context.Background()
		rs := memory.NewReadModelStore()
		now := time.Now()
		var first uuid.UUID
		for i := range people {
			p := &repository.PersonReadModel{ID: uuid.New(), GivenName: fmt.Sprintf("P%d", i), Surname: "Lee", Version: 1, UpdatedAt: now}
			if err := rs.SavePerson(ctx, domain.MainBranchID, p); err != nil {
				t.Fatalf("SavePerson: %v", err)
			}
			if i == 0 {
				first = p.ID
			}
			if i%2 == 1 {
				f := &repository.FamilyReadModel{ID: uuid.New(), Partner1ID: &p.ID, Version: 1, UpdatedAt: now}
				if err := rs.SaveFamily(ctx, domain.MainBranchID, f); err != nil {
					t.Fatalf("SaveFamily: %v", err)
				}
			}
		}
		photo := &repository.MediaReadModel{ID: uuid.New(), EntityType: "person", EntityID: first, Title: "Portrait",
			MediaType: domain.MediaPhoto, Filename: "portrait.png", Version: 1, CreatedAt: now, UpdatedAt: now}
		if err := rs.SaveMedia(ctx, domain.MainBranchID, photo); err != nil {
			t.Fatalf("SaveMedia: %v", err)
		}
		counter := &mediaListCounter{ReadModelStore: rs}
		var buf bytes.Buffer
		result, err := gedcom.NewExporter(counter).Export(ctx, &buf)
		if err != nil {
			t.Fatalf("export: %v", err)
		}
		if result.PersonsExported != people || result.MediaExported != 1 {
			t.Errorf("exported %d people and %d media, want %d and 1", result.PersonsExported, result.MediaExported, people)
		}
		return counter.calls
	}
	small, large := exportCalls(5), exportCalls(50)
	if small != large {
		t.Errorf("an export of 5 people lists media %d times, of 50 people %d times; want the same", small, large)
	}
}

// An item's crop, title and description are exported, the crop on its
// owner's link as GEDCOM 7.0, and import back equal (#1584).
func TestExport_MediaCropTitleAndDescriptionRoundTrip(t *testing.T) {
	ctx := context.Background()
	rs := memory.NewReadModelStore()
	now := time.Now()
	person := &repository.PersonReadModel{ID: uuid.New(), GivenName: "Ann", Surname: "Lee", Version: 1, UpdatedAt: now}
	if err := rs.SavePerson(ctx, domain.MainBranchID, person); err != nil {
		t.Fatalf("SavePerson: %v", err)
	}
	item := &repository.MediaReadModel{
		ID: uuid.New(), EntityType: "person", EntityID: person.ID, Title: "Ann at the reunion",
		Description: "Taken in 1910.\n\nSecond from the left.", MediaType: domain.MediaPhoto,
		Files:    []domain.MediaFile{{Path: "photos/reunion.jpg", Format: "image/jpeg", Title: "Reunion"}},
		CropLeft: new(20), CropTop: new(10), CropWidth: new(80), CropHeight: new(100),
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := rs.SaveMedia(ctx, domain.MainBranchID, item); err != nil {
		t.Fatalf("SaveMedia: %v", err)
	}

	var buf bytes.Buffer
	result, err := gedcom.NewExporter(rs).Export(ctx, &buf)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	out := buf.String()
	if result.Version != gedcomgo.Version70 {
		t.Errorf("version = %s, want 7.0 for the crop", result.Version)
	}
	link := "1 OBJE @O1@\n2 CROP\n3 TOP 10\n3 LEFT 20\n3 HEIGHT 100\n3 WIDTH 80"
	if indi := strings.Join(recordOf(out, "0 @I1@ INDI"), "\n"); !strings.Contains(indi, link) {
		t.Errorf("person record =\n%s\nwant the link\n%s", indi, link)
	}

	_, _, _, _, _, _, _, _, _, _, _, _, media, err := gedcom.NewImporter().Import(ctx, &buf)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(media) != 1 || len(media[0].Owners) != 1 {
		t.Fatalf("media = %+v, want one record with one owner", media)
	}
	owner := media[0].Owners[0]
	if title, description := media[0].OwnerText(owner); title != item.Title || description != item.Description {
		t.Errorf("re-imported title %q, description %q; want %q, %q", title, description, item.Title, item.Description)
	}
	for name, pair := range map[string][2]*int{
		"left": {owner.CropLeft, item.CropLeft}, "top": {owner.CropTop, item.CropTop},
		"width": {owner.CropWidth, item.CropWidth}, "height": {owner.CropHeight, item.CropHeight},
	} {
		if pair[0] == nil || *pair[0] != *pair[1] {
			t.Errorf("re-imported crop %s = %v, want %d", name, pair[0], *pair[1])
		}
	}
}

// An item with no crop is linked with no CROP, and the export stays 5.5; its
// description is the record's NOTE.
func TestExport_MediaDescriptionWithoutCrop(t *testing.T) {
	ctx := context.Background()
	rs := memory.NewReadModelStore()
	now := time.Now()
	person := &repository.PersonReadModel{ID: uuid.New(), GivenName: "Ann", Surname: "Lee", Version: 1, UpdatedAt: now}
	if err := rs.SavePerson(ctx, domain.MainBranchID, person); err != nil {
		t.Fatalf("SavePerson: %v", err)
	}
	if err := rs.SaveMedia(ctx, domain.MainBranchID, &repository.MediaReadModel{
		ID: uuid.New(), EntityType: "person", EntityID: person.ID, Title: "Portrait", Description: "Studio portrait.",
		MimeType: "image/png", MediaType: domain.MediaPhoto, Filename: "portrait.png", FileSize: 1, FileData: []byte{1},
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveMedia: %v", err)
	}
	var buf bytes.Buffer
	result, err := gedcom.NewExporter(rs).Export(ctx, &buf)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	out := buf.String()
	if result.Version != gedcomgo.Version55 || strings.Contains(out, "CROP") {
		t.Errorf("version %s with CROP %t, want 5.5 and no CROP", result.Version, strings.Contains(out, "CROP"))
	}
	if obj := recordOf(out, "0 @O1@ OBJE"); !slices.Contains(obj, "1 NOTE Studio portrait.") {
		t.Errorf("OBJE record = %q, want the description as its NOTE", obj)
	}
}
