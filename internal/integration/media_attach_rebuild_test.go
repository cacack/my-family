package integration_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// attachGedcom has one OBJE record, linked from one person, which an import
// keeps as a reference with no content (#1477).
const attachGedcom = `0 HEAD
1 GEDC
2 VERS 7.0
0 @M1@ OBJE
1 FILE photos/wedding.jpg
2 FORM image/jpeg
2 TITL Wedding
0 @I1@ INDI
1 NAME John /Attach/
1 OBJE @M1@
0 TRLR
`

// TestAttachMediaContent_SurvivesRebuild (#1504): a file attached to an
// imported reference is an event in the item's log, so a read model rebuilt
// from the log serves the same bytes and thumbnail, and keeps the reference's
// GEDCOM link and file paths, on every backend.
func TestAttachMediaContent_SurvivesRebuild(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)

			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(attachGedcom)}); err != nil {
				t.Fatalf("import: %v", err)
			}
			persons, err := repository.ListAll(ctx, 100, st.read.ListPersons)
			if err != nil || len(persons) != 1 {
				t.Fatalf("imported persons = %v, %v; want one", persons, err)
			}
			items, _, err := st.read.ListMediaForEntity(ctx, "person", persons[0].ID, repository.DefaultListOptions())
			if err != nil || len(items) != 1 {
				t.Fatalf("imported media = %v, %v; want one", items, err)
			}
			id := items[0].ID
			png := parityPNG(t)
			if _, err := h.AttachMediaContent(ctx, command.AttachMediaContentInput{ID: id, Filename: "wedding.png", FileData: png}); err != nil {
				t.Fatalf("attach: %v", err)
			}

			check := func(t *testing.T, rs repository.ReadModelStore) {
				t.Helper()
				got, err := rs.GetMediaWithData(ctx, domain.MainBranchID, id)
				if err != nil || got == nil {
					t.Fatalf("GetMediaWithData = %v, %v", got, err)
				}
				if !bytes.Equal(got.FileData, png) || got.FileSize != int64(len(png)) || got.MimeType != "image/png" || got.Filename != "wedding.png" {
					t.Errorf("content = %d bytes (size %d, %s, %q), want the %d attached PNG bytes as wedding.png",
						len(got.FileData), got.FileSize, got.MimeType, got.Filename, len(png))
				}
				if got.GedcomXref != "@M1@" || len(got.Files) != 1 || got.Files[0].Path != "photos/wedding.jpg" || got.Title != "Wedding" {
					t.Errorf("item = %q %+v %q, want @M1@, photos/wedding.jpg and Wedding kept", got.GedcomXref, got.Files, got.Title)
				}
				if thumb, err := rs.GetMediaThumbnail(ctx, domain.MainBranchID, id); err != nil || len(thumb) == 0 {
					t.Errorf("thumbnail = %d bytes, %v; want one", len(thumb), err)
				}
			}
			t.Run("live", func(t *testing.T) { check(t, st.read) })

			fresh := backend.setup(t).read
			replayLog(t, ctx, st.events, fresh)
			t.Run("rebuilt", func(t *testing.T) { check(t, fresh) })
		})
	}
}
