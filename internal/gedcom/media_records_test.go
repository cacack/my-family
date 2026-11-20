package gedcom_test

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	gedcomgo "github.com/cacack/gedcom-go/v3/gedcom"
	"github.com/google/uuid"

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
