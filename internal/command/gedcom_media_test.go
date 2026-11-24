package command_test

import (
	"context"
	"errors"
	"io"
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

// mediaGedcom has an OBJE record linked from a person, a family and a source,
// and one linked from nothing (#1435).
const mediaGedcom = `0 HEAD
1 GEDC
2 VERS 7.0
0 @M1@ OBJE
1 RESN CONFIDENTIAL
1 FILE photos/wedding.jpg
2 FORM image/jpeg
3 MEDI PHOTO
2 TITL Wedding
0 @M2@ OBJE
1 FILE scans/unused.pdf
2 FORM application/pdf
2 TITL Unused
0 @S1@ SOUR
1 TITL Register
1 OBJE @M1@
0 @I1@ INDI
1 NAME John /Doe/
1 OBJE @M1@
1 OBJE @M1@
0 @F1@ FAM
1 HUSB @I1@
1 OBJE @M1@
0 TRLR
`

func TestImportGedcom_MediaOneItemPerOwner(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)

	result, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(mediaGedcom)})
	if err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}
	if result.MediaImported != 3 {
		t.Errorf("MediaImported = %d, want 3 (one per owner of @M1@)", result.MediaImported)
	}
	if !slices.ContainsFunc(result.Warnings, func(w string) bool { return strings.Contains(w, "@M2@") }) {
		t.Errorf("warnings %q lack the unlinked @M2@", result.Warnings)
	}

	persons, _, _ := readStore.ListPersons(ctx, repository.DefaultListOptions())
	families, _, _ := readStore.ListFamilies(ctx, repository.DefaultListOptions())
	sources, _, _ := readStore.ListSources(ctx, repository.DefaultListOptions())
	if len(persons) != 1 || len(families) != 1 || len(sources) != 1 {
		t.Fatalf("imported %d persons, %d families, %d sources; want 1 each", len(persons), len(families), len(sources))
	}
	for _, owner := range []struct {
		entityType string
		id         uuid.UUID
	}{{"person", persons[0].ID}, {"family", families[0].ID}, {"source", sources[0].ID}} {
		media, _, err := readStore.ListMediaForEntity(ctx, owner.entityType, owner.id, repository.DefaultListOptions())
		if err != nil || len(media) != 1 {
			t.Fatalf("%s media = %d, %v; want 1", owner.entityType, len(media), err)
		}
		m := media[0]
		if m.Title != "Wedding" || m.MediaType != domain.MediaPhoto || m.GedcomXref != "@M1@" ||
			len(m.Files) != 1 || m.Files[0].Path != "photos/wedding.jpg" || !m.Restriction.IsPrivate() {
			t.Errorf("%s media = %+v, want the private Wedding photo of @M1@", owner.entityType, m)
		}
	}
}

// nestedMediaGedcom links OBJE records below record level, under events, an
// attribute and citations, and describes media inline at record level and
// under an event (#1479). @M5@ is linked from John twice.
const nestedMediaGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
2 FORM LINEAGE-LINKED
1 CHAR UTF-8
0 @M1@ OBJE
1 FILE birth.jpg
2 FORM jpg
2 TITL Birth certificate
0 @M2@ OBJE
1 FILE occupation.jpg
2 FORM jpg
2 TITL Work photo
0 @M3@ OBJE
1 FILE marriage.jpg
2 FORM jpg
2 TITL Marriage record
0 @M4@ OBJE
1 FILE census.jpg
2 FORM jpg
2 TITL Census page
0 @M5@ OBJE
1 FILE portrait.jpg
2 FORM jpg
2 TITL Portrait
0 @M6@ OBJE
1 FILE banns.jpg
2 FORM jpg
2 TITL Banns
0 @S1@ SOUR
1 TITL Census
0 @I1@ INDI
1 NAME John /Doe/
1 OBJE @M5@
1 OBJE
2 FILE photos/john.jpg
3 FORM jpg
2 TITL John at ten
1 BIRT
2 DATE 1900
2 OBJE @M1@
2 OBJE @M5@
2 OBJE
3 FILE scans/baptism.pdf
4 FORM pdf
3 TITL Baptism entry
1 OCCU Farmer
2 OBJE @M2@
1 RESI
2 SOUR @S1@
3 OBJE @M4@
1 FAMS @F1@
0 @F1@ FAM
1 HUSB @I1@
1 MARR
2 OBJE @M3@
1 SOUR @S1@
2 OBJE @M6@
0 TRLR
`

func TestImportGedcom_MediaBelowRecordLevelAndInline(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)

	result, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(nestedMediaGedcom)})
	if err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}
	if slices.ContainsFunc(result.Warnings, func(w string) bool { return strings.Contains(w, "linked to no") }) {
		t.Errorf("warnings %q report a linked record as unlinked", result.Warnings)
	}
	if result.MediaImported != 8 {
		t.Errorf("MediaImported = %d, want 8", result.MediaImported)
	}

	persons, _, _ := readStore.ListPersons(ctx, repository.DefaultListOptions())
	families, _, _ := readStore.ListFamilies(ctx, repository.DefaultListOptions())
	if len(persons) != 1 || len(families) != 1 {
		t.Fatalf("imported %d persons, %d families; want 1 each", len(persons), len(families))
	}
	titles := func(entityType string, id uuid.UUID) map[string]repository.MediaReadModel {
		media, _, err := readStore.ListMediaForEntity(ctx, entityType, id, repository.DefaultListOptions())
		if err != nil {
			t.Fatalf("ListMediaForEntity(%s): %v", entityType, err)
		}
		byTitle := make(map[string]repository.MediaReadModel, len(media))
		for _, m := range media {
			byTitle[m.Title] = m
		}
		if len(byTitle) != len(media) {
			t.Errorf("%s holds %d items under %d titles, want one item per title", entityType, len(media), len(byTitle))
		}
		return byTitle
	}

	person := titles("person", persons[0].ID)
	for title, xref := range map[string]string{
		"Birth certificate": "@M1@", // under an event
		"Work photo":        "@M2@", // under an attribute
		"Census page":       "@M4@", // under an event's citation
		"Portrait":          "@M5@", // at record level and under an event
	} {
		if person[title].GedcomXref != xref {
			t.Errorf("person item %q = %+v, want the item of %s", title, person[title], xref)
		}
	}
	for title, path := range map[string]string{"John at ten": "photos/john.jpg", "Baptism entry": "scans/baptism.pdf"} {
		m, ok := person[title]
		if !ok || m.GedcomXref != "" || len(m.Files) != 1 || m.Files[0].Path != path || m.Format == "" {
			t.Errorf("person inline item %q = %+v, want %s with its format and no xref", title, m, path)
		}
	}
	if len(person) != 6 {
		t.Errorf("person holds %d items, want 6", len(person))
	}

	family := titles("family", families[0].ID)
	if family["Marriage record"].GedcomXref != "@M3@" || family["Banns"].GedcomXref != "@M6@" || len(family) != 2 {
		t.Errorf("family items = %+v, want @M3@ (under an event) and @M6@ (under a citation)", family)
	}
}

// failingStreamStore fails every append to streams of one type.
type failingStreamStore struct {
	*memory.EventStore
	streamType string
}

func (s failingStreamStore) Append(ctx context.Context, streamID uuid.UUID, streamType string, events []domain.Event, expectedVersion int64, scope repository.AppendScope) error {
	if streamType == s.streamType {
		return io.ErrUnexpectedEOF
	}
	return s.EventStore.Append(ctx, streamID, streamType, events, expectedVersion, scope)
}

func TestImportGedcom_MediaAppendFailureWarns(t *testing.T) {
	ctx := context.Background()
	handler := command.NewHandler(failingStreamStore{memory.NewEventStore(), "media"}, memory.NewReadModelStore())

	result, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(mediaGedcom)})
	if err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}
	if result.MediaImported != 0 {
		t.Errorf("MediaImported = %d, want 0", result.MediaImported)
	}
	if !slices.ContainsFunc(result.Warnings, func(w string) bool { return strings.Contains(w, "Failed to import media @M1@") }) {
		t.Errorf("warnings %q lack the failed media", result.Warnings)
	}
}

func TestImportGedcom_MediaSkipsFailedOwner(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(failingStreamStore{memory.NewEventStore(), "source"}, readStore)

	result, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(mediaGedcom)})
	if err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}
	if result.SourcesImported != 0 || result.MediaImported != 2 {
		t.Errorf("SourcesImported = %d, MediaImported = %d; want 0 and 2 (person and family only)", result.SourcesImported, result.MediaImported)
	}
	if !slices.ContainsFunc(result.Warnings, func(w string) bool {
		return strings.Contains(w, "Media @M1@") && strings.Contains(w, "source") && strings.Contains(w, "failed to import")
	}) {
		t.Errorf("warnings %q lack the media skipped for the failed source", result.Warnings)
	}
	persons, _, _ := readStore.ListPersons(ctx, repository.DefaultListOptions())
	if len(persons) != 1 {
		t.Fatalf("imported %d persons, want 1", len(persons))
	}
	media, _, err := readStore.ListMediaForEntity(ctx, "person", persons[0].ID, repository.DefaultListOptions())
	if err != nil || len(media) != 1 {
		t.Errorf("person media = %d, %v; want 1", len(media), err)
	}
}

// failingStreamOnce fails the next append to one stream, then appends as usual.
type failingStreamOnce struct {
	*memory.EventStore
	streamID *uuid.UUID
}

func (s failingStreamOnce) Append(ctx context.Context, streamID uuid.UUID, streamType string, events []domain.Event, expectedVersion int64, scope repository.AppendScope) error {
	if s.streamID != nil && *s.streamID == streamID {
		*s.streamID = uuid.Nil
		return io.ErrUnexpectedEOF
	}
	return s.EventStore.Append(ctx, streamID, streamType, events, expectedVersion, scope)
}

// groupMedia returns the items of the group of the media of person, family and
// source in mediaGedcom, failing unless they are one group of three.
func groupMedia(t *testing.T, ctx context.Context, readStore repository.ReadModelStore) []repository.MediaReadModel {
	t.Helper()
	persons, _, _ := readStore.ListPersons(ctx, repository.DefaultListOptions())
	media, _, _ := readStore.ListMediaForEntity(ctx, "person", persons[0].ID, repository.DefaultListOptions())
	if len(media) != 1 || media[0].GroupID == nil {
		t.Fatalf("person media = %+v, want one item in a group", media)
	}
	group, err := readStore.ListMediaInGroup(ctx, domain.MainBranchID, *media[0].GroupID)
	if err != nil || len(group) != 3 {
		t.Fatalf("group = %d items, %v; want 3", len(group), err)
	}
	return group
}

// The items an import makes of one OBJE record change their private mark
// together; an item of a record linked once is in no group (#1478).
func TestUpdateMedia_PrivateMarkCoversTheGroup(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)
	gedcomText := strings.Replace(mediaGedcom, "0 @S1@ SOUR", "0 @M3@ OBJE\n1 FILE solo.jpg\n2 FORM jpg\n0 @S1@ SOUR\n1 OBJE @M3@", 1)
	if _, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(gedcomText)}); err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}
	sources, _, _ := readStore.ListSources(ctx, repository.DefaultListOptions())
	sourceMedia, _, _ := readStore.ListMediaForEntity(ctx, "source", sources[0].ID, repository.DefaultListOptions())
	for _, m := range sourceMedia {
		if (m.GedcomXref == "@M3@") != (m.GroupID == nil) {
			t.Errorf("media %s group = %v; want a group for @M1@ only", m.GedcomXref, m.GroupID)
		}
	}

	for _, private := range []bool{false, true} {
		first := groupMedia(t, ctx, readStore)[0]
		if _, err := handler.UpdateMedia(ctx, command.UpdateMediaInput{ID: first.ID, Private: &private, Version: first.Version}); err != nil {
			t.Fatalf("UpdateMedia: %v", err)
		}
		for _, m := range groupMedia(t, ctx, readStore) {
			if m.Restriction.IsPrivate() != private {
				t.Errorf("after marking private %v: item of %s %s has restriction %q", private, m.EntityType, m.EntityID, m.Restriction)
			}
		}
	}
}

// A group item the update fails to mark is marked when the mark is sent
// again, though the item it was sent for is unchanged then.
func TestUpdateMedia_ResentPrivateMarkRepairsTheGroup(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	failing := failingStreamOnce{memory.NewEventStore(), new(uuid.UUID)}
	handler := command.NewHandler(failing, readStore)
	if _, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(mediaGedcom)}); err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}
	group := groupMedia(t, ctx, readStore)
	target := slices.IndexFunc(group, func(m repository.MediaReadModel) bool { return m.EntityType == "person" })
	other := (target + 1) % len(group)
	*failing.streamID = group[other].ID

	public := false
	if _, err := handler.UpdateMedia(ctx, command.UpdateMediaInput{ID: group[target].ID, Private: &public, Version: group[target].Version}); err == nil {
		t.Fatal("UpdateMedia succeeded, want the failed append of the other item")
	}
	if m, _ := readStore.GetMedia(ctx, domain.MainBranchID, group[other].ID); m == nil || !m.Restriction.IsPrivate() {
		t.Fatalf("other item = %+v, want it still private after the failure", m)
	}

	updated, _ := readStore.GetMedia(ctx, domain.MainBranchID, group[target].ID)
	if _, err := handler.UpdateMedia(ctx, command.UpdateMediaInput{ID: updated.ID, Private: &public, Version: updated.Version}); err != nil {
		t.Fatalf("resent UpdateMedia: %v", err)
	}
	for _, m := range groupMedia(t, ctx, readStore) {
		if m.Restriction.IsPrivate() {
			t.Errorf("item of %s has restriction %q, want none private", m.EntityType, m.Restriction)
		}
	}
}

// Rolling a group item back to a version with another private mark gives the
// mark to the whole group.
func TestRollbackMedia_PrivateMarkCoversTheGroup(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)
	if _, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(mediaGedcom)}); err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}
	target := groupMedia(t, ctx, readStore)[0].ID
	var privateVersion int64
	for _, private := range []bool{false, true, false} {
		m, _ := readStore.GetMedia(ctx, domain.MainBranchID, target)
		result, err := handler.UpdateMedia(ctx, command.UpdateMediaInput{ID: target, Private: &private, Version: m.Version})
		if err != nil {
			t.Fatalf("UpdateMedia: %v", err)
		}
		if private {
			privateVersion = result.Version
		}
	}

	if _, err := handler.RollbackMedia(ctx, target, privateVersion); err != nil {
		t.Fatalf("RollbackMedia: %v", err)
	}
	for _, m := range groupMedia(t, ctx, readStore) {
		if !m.Restriction.IsPrivate() {
			t.Errorf("item of %s has restriction %q, want private after the rollback", m.EntityType, m.Restriction)
		}
	}
}

// Rolling an imported private item back to version 1 makes it and its group
// private again after an editor unmarked them (#1508).
func TestRollbackMedia_ToImportedVersionRestoresThePrivateMark(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)
	if _, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(mediaGedcom)}); err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}
	target := groupMedia(t, ctx, readStore)[0]
	public := false
	if _, err := handler.UpdateMedia(ctx, command.UpdateMediaInput{ID: target.ID, Private: &public, Version: target.Version}); err != nil {
		t.Fatalf("UpdateMedia: %v", err)
	}
	for _, m := range groupMedia(t, ctx, readStore) {
		if m.Restriction.IsPrivate() {
			t.Fatalf("item of %s has restriction %q, want none private before the rollback", m.EntityType, m.Restriction)
		}
	}

	if _, err := handler.RollbackMedia(ctx, target.ID, 1); err != nil {
		t.Fatalf("RollbackMedia: %v", err)
	}
	if m, _ := readStore.GetMedia(ctx, domain.MainBranchID, target.ID); m == nil || !m.Restriction.IsPrivate() {
		t.Errorf("rolled back item = %+v, want it private", m)
	}
	for _, m := range groupMedia(t, ctx, readStore) {
		if !m.Restriction.IsPrivate() {
			t.Errorf("item of %s has restriction %q, want private after the rollback", m.EntityType, m.Restriction)
		}
	}
}

// survivorMediaCount imports ged into a fresh store and returns the number of
// media items the person named given holds, with that store.
func survivorMediaCount(t *testing.T, ged, given string) (int, *memory.ReadModelStore) {
	t.Helper()
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)
	if _, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(ged)}); err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}
	persons, _, _ := readStore.ListPersons(ctx, repository.DefaultListOptions())
	for _, p := range persons {
		if p.GivenName == given {
			media, _, err := readStore.ListMediaForEntity(ctx, "person", p.ID, repository.DefaultListOptions())
			if err != nil {
				t.Fatalf("ListMediaForEntity: %v", err)
			}
			return len(media), readStore
		}
	}
	t.Fatalf("no person named %s in\n%s", given, ged)
	return 0, nil
}

func exportGedcom(t *testing.T, readStore repository.ReadModelStore) string {
	t.Helper()
	var buf strings.Builder
	if _, err := gedcom.NewExporter(readStore).Export(context.Background(), &buf); err != nil {
		t.Fatalf("Export: %v", err)
	}
	return buf.String()
}

// Merging two people who both link one OBJE record leaves the survivor with
// two items of its group; export and re-import give the survivor the same
// number of items for that photo on every round trip (#1507).
func TestImportGedcom_MergedSharedMediaRoundTripIsStable(t *testing.T) {
	const ged = `0 HEAD
1 GEDC
2 VERS 7.0
0 @M1@ OBJE
1 FILE photos/reunion.jpg
2 FORM image/jpeg
2 TITL Reunion
0 @I1@ INDI
1 NAME John /Doe/
1 OBJE @M1@
0 @I2@ INDI
1 NAME Johnny /Doe/
1 OBJE @M1@
0 TRLR
`
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)
	if _, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(ged)}); err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}
	persons, _, _ := readStore.ListPersons(ctx, repository.DefaultListOptions())
	if len(persons) != 2 {
		t.Fatalf("imported %d persons, want 2", len(persons))
	}
	survivor, merged := persons[0], persons[1]
	if survivor.GivenName != "John" {
		survivor, merged = merged, survivor
	}
	if _, err := handler.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID: survivor.ID, MergedID: merged.ID, SurvivorVersion: survivor.Version, MergedVersion: merged.Version,
	}); err != nil {
		t.Fatalf("MergePersons: %v", err)
	}
	held, _, _ := readStore.ListMediaForEntity(ctx, "person", survivor.ID, repository.DefaultListOptions())
	if len(held) != 2 {
		t.Fatalf("survivor holds %d items after the merge, want 2", len(held))
	}

	first := exportGedcom(t, readStore)
	if n := strings.Count(first, "1 OBJE @O1@"); n != 1 {
		t.Errorf("export links @O1@ %d times, want once:\n%s", n, first)
	}
	count1, store1 := survivorMediaCount(t, first, "John")
	count2, _ := survivorMediaCount(t, exportGedcom(t, store1), "John")
	if count1 != 1 || count2 != count1 {
		t.Errorf("survivor holds %d items after one round trip and %d after two, want 1 both times", count1, count2)
	}
}

// referenceOnlyGedcom has one OBJE record, linked from one person, which an
// import keeps as a reference with no content.
const referenceOnlyGedcom = `0 HEAD
1 GEDC
2 VERS 7.0
0 @M1@ OBJE
1 FILE photos/wedding.jpg
2 FORM image/jpeg
2 TITL Wedding
0 @I1@ INDI
1 NAME John /Doe/
1 OBJE @M1@
0 TRLR
`

// TestAttachMediaContent covers attaching a file to an imported reference
// (#1504): it fills the item in place once, and refuses everything else
// without changing it.
func TestAttachMediaContent(t *testing.T) {
	ctx := context.Background()
	events := memory.NewEventStore()
	reads := memory.NewReadModelStore()
	branches := memory.NewBranchStore()
	h := command.NewHandlerWithBranches(events, reads, branches, memory.NewSnapshotStore(events))
	if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(referenceOnlyGedcom)}); err != nil {
		t.Fatalf("import: %v", err)
	}
	persons, _, _ := reads.ListPersons(ctx, repository.DefaultListOptions())
	if len(persons) != 1 {
		t.Fatalf("imported %d persons, want 1", len(persons))
	}
	items, _, _ := reads.ListMediaForEntity(ctx, "person", persons[0].ID, repository.DefaultListOptions())
	if len(items) != 1 {
		t.Fatalf("imported %d media items, want 1", len(items))
	}
	ref := items[0]

	refusals := []struct {
		name  string
		input command.AttachMediaContentInput
		want  error
	}{
		{"unknown item", command.AttachMediaContentInput{ID: uuid.New(), FileData: createTestJPEG()}, command.ErrMediaNotFound},
		{"empty file", command.AttachMediaContentInput{ID: ref.ID}, command.ErrInvalidInput},
		{"unsupported file", command.AttachMediaContentInput{ID: ref.ID, FileData: []byte("plain text")}, command.ErrInvalidInput},
	}
	for _, tt := range refusals {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := h.AttachMediaContent(ctx, tt.input); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}

	branch, err := h.CreateBranch(ctx, "research", "")
	if err != nil {
		t.Fatalf("create branch: %v", err)
	}
	if _, err := h.WithBranch(branch).AttachMediaContent(ctx, command.AttachMediaContentInput{ID: ref.ID, FileData: createTestJPEG()}); !errors.Is(err, command.ErrEventTypeNotBranchAware) {
		t.Errorf("attach on a branch: err = %v, want %v", err, command.ErrEventTypeNotBranchAware)
	}
	if unchanged, _ := reads.GetMedia(ctx, domain.MainBranchID, ref.ID); unchanged.Version != ref.Version || unchanged.FileSize != 0 {
		t.Fatalf("after the refusals: version %d, size %d; want %d and 0", unchanged.Version, unchanged.FileSize, ref.Version)
	}

	jpegData := createTestJPEG()
	result, err := h.AttachMediaContent(ctx, command.AttachMediaContentInput{ID: ref.ID, Filename: "wedding.jpg", FileData: jpegData})
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	got, _ := reads.GetMedia(ctx, domain.MainBranchID, ref.ID)
	if result.Version != ref.Version+1 || got.Version != result.Version || got.FileSize != int64(len(jpegData)) ||
		got.MimeType != "image/jpeg" || got.Filename != "wedding.jpg" || !got.HasThumbnail {
		t.Errorf("attached = v%d (result v%d), %d bytes, %s, %q, thumbnail %v; want v%d, %d bytes, image/jpeg, wedding.jpg, a thumbnail",
			got.Version, result.Version, got.FileSize, got.MimeType, got.Filename, got.HasThumbnail, ref.Version+1, len(jpegData))
	}

	if _, err := h.AttachMediaContent(ctx, command.AttachMediaContentInput{ID: ref.ID, FileData: jpegData}); !errors.Is(err, command.ErrMediaHasContent) {
		t.Errorf("second attach: err = %v, want %v", err, command.ErrMediaHasContent)
	}
}

// fileLessMediaGedcom has an OBJE record with no FILE and one, as Ancestry
// writes, whose FILE is empty (#1585).
const fileLessMediaGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
0 @M1@ OBJE
0 @M2@ OBJE
1 FILE
2 FORM jpg
2 TITL Ancestry photo
0 @I1@ INDI
1 NAME A /B/
1 OBJE @M1@
1 OBJE @M2@
0 TRLR
`

// A record with no FILE imports as a document, with a warning naming it; one
// whose FILE is empty imports with no warning.
func TestImportGedcom_MediaWithoutFileReference(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)

	result, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(fileLessMediaGedcom)})
	if err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}
	if result.MediaImported != 2 {
		t.Errorf("MediaImported = %d, want 2", result.MediaImported)
	}
	warnsAbout := func(xref string) bool {
		return slices.ContainsFunc(result.Warnings, func(w string) bool {
			return strings.Contains(w, xref) && strings.Contains(w, "FILE")
		})
	}
	if !warnsAbout("@M1@") {
		t.Errorf("warnings %q lack @M1@'s missing FILE", result.Warnings)
	}
	if warnsAbout("@M2@") {
		t.Errorf("warnings %q name @M2@, whose FILE is empty but present", result.Warnings)
	}

	persons, _, _ := readStore.ListPersons(ctx, repository.DefaultListOptions())
	if len(persons) != 1 {
		t.Fatalf("imported %d persons, want 1", len(persons))
	}
	media, _, err := readStore.ListMediaForEntity(ctx, "person", persons[0].ID, repository.DefaultListOptions())
	if err != nil {
		t.Fatalf("ListMediaForEntity: %v", err)
	}
	byXref := map[string]repository.MediaReadModel{}
	for _, m := range media {
		byXref[m.GedcomXref] = m
	}
	if m := byXref["@M1@"]; m.MediaType != domain.MediaDocument || len(m.Files) != 0 {
		t.Errorf("@M1@ = %+v, want a document with no files", m)
	}
	if m := byXref["@M2@"]; m.MediaType != domain.MediaDocument || len(m.Files) != 1 || m.Files[0].Path != "" {
		t.Errorf("@M2@ = %+v, want a document with one empty file reference", m)
	}
}

// An OBJE whose title is over the 500-character limit imports with its title
// shortened to fit, ending in "…", and kept whole at the start of its
// description, with a warning naming it (#1761). A title of 500 multi-byte
// characters imports unchanged (#1750).
func TestImportGedcom_MediaTitleOverLimitIsShortened(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)

	fits := strings.Repeat("é", 500)
	long := strings.Repeat("é", 501)
	ged := "0 HEAD\n1 GEDC\n2 VERS 5.5.1\n" +
		"0 @M1@ OBJE\n1 FILE a.jpg\n2 TITL " + fits + "\n" +
		"0 @M2@ OBJE\n1 FILE b.jpg\n2 TITL " + long + "\n1 NOTE A record note.\n" +
		"0 @I1@ INDI\n1 NAME A /B/\n1 OBJE @M1@\n1 OBJE @M2@\n0 TRLR\n"
	result, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(ged)})
	if err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}
	if result.MediaImported != 2 {
		t.Errorf("MediaImported = %d, want 2", result.MediaImported)
	}
	want := "Media @M2@ title is over 500 characters; shortened, with the full title kept at the start of its description"
	if !slices.Contains(result.Warnings, want) {
		t.Errorf("warnings %q lack %q", result.Warnings, want)
	}
	if slices.ContainsFunc(result.Warnings, func(w string) bool { return strings.Contains(w, "@M1@") }) {
		t.Errorf("warnings %q name @M1@, whose title fits", result.Warnings)
	}

	persons, _, _ := readStore.ListPersons(ctx, repository.DefaultListOptions())
	if len(persons) != 1 {
		t.Fatalf("imported %d persons, want 1", len(persons))
	}
	media, _, err := readStore.ListMediaForEntity(ctx, "person", persons[0].ID, repository.DefaultListOptions())
	if err != nil {
		t.Fatalf("ListMediaForEntity: %v", err)
	}
	byXref := map[string]repository.MediaReadModel{}
	for _, m := range media {
		byXref[m.GedcomXref] = m
	}
	if m := byXref["@M1@"]; m.Title != fits || m.Description != "" {
		t.Errorf("@M1@ title %q, description %q; want the title unchanged and no description", m.Title, m.Description)
	}
	m, ok := byXref["@M2@"]
	if !ok {
		t.Fatalf("media = %+v, want @M2@ imported", media)
	}
	if wantTitle := strings.Repeat("é", 499) + "…"; m.Title != wantTitle {
		t.Errorf("@M2@ title = %q, want %q", m.Title, wantTitle)
	}
	if wantDesc := long + "\n\nA record note."; m.Description != wantDesc {
		t.Errorf("@M2@ description = %q, want %q", m.Description, wantDesc)
	}
	if len(m.Files) != 1 || m.Files[0].Path != "b.jpg" {
		t.Errorf("@M2@ files = %+v, want b.jpg", m.Files)
	}
	item := domain.NewMedia(m.Title, m.EntityType, m.EntityID)
	item.Description = m.Description
	if err := item.Validate(); err != nil {
		t.Errorf("@M2@ as stored fails validation: %v", err)
	}
}

// A link's over-long TITL, which replaces the record's for that owner, is
// shortened the same way, ahead of the link's own note (#1761).
func TestImportGedcom_MediaLinkTitleOverLimitIsShortened(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)

	long := strings.Repeat("x", 600)
	ged := "0 HEAD\n1 GEDC\n2 VERS 7.0\n" +
		"0 @M1@ OBJE\n1 FILE a.jpg\n2 FORM image/jpeg\n2 TITL Short\n" +
		"0 @I1@ INDI\n1 NAME A /B/\n1 OBJE @M1@\n2 TITL " + long + "\n2 NOTE Link note.\n0 TRLR\n"
	result, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(ged)})
	if err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}
	if result.MediaImported != 1 {
		t.Fatalf("MediaImported = %d, want 1 (warnings %q)", result.MediaImported, result.Warnings)
	}
	persons, _, _ := readStore.ListPersons(ctx, repository.DefaultListOptions())
	media, _, err := readStore.ListMediaForEntity(ctx, "person", persons[0].ID, repository.DefaultListOptions())
	if err != nil || len(media) != 1 {
		t.Fatalf("media = %+v, %v; want one item", media, err)
	}
	if got, want := media[0].Title, strings.Repeat("x", 499)+"…"; got != want {
		t.Errorf("title = %q, want %q", got, want)
	}
	if got, want := media[0].Description, long+"\n\nLink note."; got != want {
		t.Errorf("description = %q, want %q", got, want)
	}
}

// linkDetailGedcom links @M1@ from John with a crop, a title and a note, and
// from Mary with none; Will's inline link has its own (#1584).
const linkDetailGedcom = `0 HEAD
1 GEDC
2 VERS 7.0
0 @M1@ OBJE
1 FILE photos/reunion.jpg
2 FORM image/jpeg
2 TITL Reunion
1 NOTE Taken at the 1910 reunion.
0 @I1@ INDI
1 NAME John /Doe/
1 OBJE @M1@
2 CROP
3 TOP 10
3 LEFT 20
3 HEIGHT 100
3 WIDTH 80
2 TITL John at the reunion
2 NOTE Second from the left.
0 @I2@ INDI
1 NAME Mary /Roe/
1 OBJE @M1@
0 @I3@ INDI
1 NAME Will /Doe/
1 OBJE
2 FILE photos/will.jpg
3 FORM image/jpeg
2 TITL Will at twenty
2 CROP
3 TOP 5
3 LEFT 6
2 NOTE On the farm.
0 TRLR
`

// personMedia returns the one media item of the person named given.
func personMedia(t *testing.T, ctx context.Context, readStore repository.ReadModelStore, given string) repository.MediaReadModel {
	t.Helper()
	persons, _, _ := readStore.ListPersons(ctx, repository.DefaultListOptions())
	for _, p := range persons {
		if p.GivenName == given {
			media, _, err := readStore.ListMediaForEntity(ctx, "person", p.ID, repository.DefaultListOptions())
			if err != nil || len(media) != 1 {
				t.Fatalf("%s media = %+v, %v; want one item", given, media, err)
			}
			return media[0]
		}
	}
	t.Fatalf("no person named %s", given)
	return repository.MediaReadModel{}
}

func cropOf(m repository.MediaReadModel) [4]*int {
	return [4]*int{m.CropTop, m.CropLeft, m.CropHeight, m.CropWidth}
}

func wantCrop(t *testing.T, name string, m repository.MediaReadModel, want [4]*int) {
	t.Helper()
	got := cropOf(m)
	for i, field := range []string{"top", "left", "height", "width"} {
		if (got[i] == nil) != (want[i] == nil) || (got[i] != nil && *got[i] != *want[i]) {
			t.Errorf("%s crop %s = %v, want %v", name, field, deref(got[i]), deref(want[i]))
		}
	}
}

func deref(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

// A link's CROP, TITL and NOTE reach only the item of the owner whose link
// carries them, for pointer and inline links alike (#1584).
func TestImportGedcom_MediaLinkTitleNotesAndCrop(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)
	if _, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(linkDetailGedcom)}); err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}

	john := personMedia(t, ctx, readStore, "John")
	if john.Title != "John at the reunion" || john.Description != "Taken at the 1910 reunion.\n\nSecond from the left." {
		t.Errorf("John's item = title %q, description %q; want the link's title and both notes", john.Title, john.Description)
	}
	wantCrop(t, "John's item", john, [4]*int{new(10), new(20), new(100), new(80)})

	mary := personMedia(t, ctx, readStore, "Mary")
	if mary.Title != "Reunion" || mary.Description != "Taken at the 1910 reunion." {
		t.Errorf("Mary's item = title %q, description %q; want the file's title and the record's note", mary.Title, mary.Description)
	}
	wantCrop(t, "Mary's item", mary, [4]*int{})

	will := personMedia(t, ctx, readStore, "Will")
	if will.Title != "Will at twenty" || will.Description != "On the farm." {
		t.Errorf("Will's item = title %q, description %q; want the inline link's title and note", will.Title, will.Description)
	}
	wantCrop(t, "Will's item", will, [4]*int{new(5), new(6), nil, nil})
}

// Export writes the link detail of an imported group's items on their links,
// so a re-import gives every item back its title, description and crop and
// keeps John's and Mary's items one group (#1478, #1584).
func TestImportGedcom_MediaLinkDetailRoundTripKeepsTheGroup(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)
	if _, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(linkDetailGedcom)}); err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}
	out := exportGedcom(t, readStore)
	if n := strings.Count(out, " OBJE\n"); n != 2 {
		t.Errorf("export writes %d OBJE records, want 2 (the shared one and Will's):\n%s", n, out)
	}

	again := memory.NewReadModelStore()
	if _, err := command.NewHandler(memory.NewEventStore(), again).ImportGedcom(ctx, command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(out)}); err != nil {
		t.Fatalf("re-import: %v", err)
	}
	for _, given := range []string{"John", "Mary", "Will"} {
		before, after := personMedia(t, ctx, readStore, given), personMedia(t, ctx, again, given)
		if after.Title != before.Title || after.Description != before.Description {
			t.Errorf("%s's item = title %q, description %q; want %q, %q", given, after.Title, after.Description, before.Title, before.Description)
		}
		wantCrop(t, given+"'s re-imported item", after, cropOf(before))
	}
	john, mary := personMedia(t, ctx, again, "John"), personMedia(t, ctx, again, "Mary")
	if john.GroupID == nil || mary.GroupID == nil || *john.GroupID != *mary.GroupID {
		t.Errorf("re-imported groups John %v, Mary %v; want one group", john.GroupID, mary.GroupID)
	}
}

// sharedNoteMediaGedcom links @M1@, whose record points to shared note @N1@,
// from John with a link pointing to @N2@ and from Mary with a plain link, in
// GEDCOM 7.0 (SNOTE) and 5.5.1 (NOTE) form (#1756).
var sharedNoteMediaGedcom = map[string]string{
	"7.0": `0 HEAD
1 GEDC
2 VERS 7.0
0 @N1@ SNOTE Taken at the 1910 reunion.
0 @N2@ SNOTE Second from the left.
0 @M1@ OBJE
1 FILE photos/reunion.jpg
2 FORM image/jpeg
2 TITL Reunion
1 SNOTE @N1@
1 NOTE Scanned in 2001.
0 @I1@ INDI
1 NAME John /Doe/
1 OBJE @M1@
2 SNOTE @N2@
0 @I2@ INDI
1 NAME Mary /Roe/
1 OBJE @M1@
0 TRLR
`,
	"5.5.1": `0 HEAD
1 GEDC
2 VERS 5.5.1
2 FORM LINEAGE-LINKED
1 CHAR UTF-8
0 @N1@ NOTE Taken at the 1910 reunion.
0 @N2@ NOTE Second from the left.
0 @M1@ OBJE
1 FILE photos/reunion.jpg
2 FORM jpg
2 TITL Reunion
1 NOTE @N1@
1 NOTE Scanned in 2001.
0 @I1@ INDI
1 NAME John /Doe/
1 OBJE @M1@
2 NOTE @N2@
0 @I2@ INDI
1 NAME Mary /Roe/
1 OBJE @M1@
0 TRLR
`,
}

// A shared note an OBJE record or link points to gives its text to the
// description of every owner's item, or of the linking owner's only, and
// export and re-import keep it (#1756).
func TestImportGedcom_MediaSharedNotes(t *testing.T) {
	const (
		wantJohn = "Taken at the 1910 reunion.\n\nScanned in 2001.\n\nSecond from the left."
		wantMary = "Taken at the 1910 reunion.\n\nScanned in 2001."
	)
	for version, ged := range sharedNoteMediaGedcom {
		t.Run(version, func(t *testing.T) {
			ctx := context.Background()
			readStore := memory.NewReadModelStore()
			result, err := command.NewHandler(memory.NewEventStore(), readStore).ImportGedcom(ctx, command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(ged)})
			if err != nil {
				t.Fatalf("ImportGedcom: %v", err)
			}
			if slices.ContainsFunc(result.Warnings, func(w string) bool { return strings.Contains(w, "points to no note") }) {
				t.Errorf("warnings %q report a note record as missing", result.Warnings)
			}
			// The notes are in the descriptions, so none is reported lost (#1773).
			if slices.ContainsFunc(result.Warnings, func(w string) bool { return strings.Contains(w, "not imported") }) {
				t.Errorf("warnings %q report an imported note as not imported", result.Warnings)
			}
			if got := personMedia(t, ctx, readStore, "John").Description; got != wantJohn {
				t.Errorf("John's description = %q, want %q", got, wantJohn)
			}
			if got := personMedia(t, ctx, readStore, "Mary").Description; got != wantMary {
				t.Errorf("Mary's description = %q, want %q", got, wantMary)
			}

			again := memory.NewReadModelStore()
			if _, err := command.NewHandler(memory.NewEventStore(), again).ImportGedcom(ctx, command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(exportGedcom(t, readStore))}); err != nil {
				t.Fatalf("re-import: %v", err)
			}
			if got := personMedia(t, ctx, again, "John").Description; got != wantJohn {
				t.Errorf("re-imported John's description = %q, want %q", got, wantJohn)
			}
			if got := personMedia(t, ctx, again, "Mary").Description; got != wantMary {
				t.Errorf("re-imported Mary's description = %q, want %q", got, wantMary)
			}
		})
	}
}

// A note under an OBJE link the import does not keep (a second link from the
// same owner, even with the first's note, a link to no OBJE record, an inline
// link with no file, a link in a submitter record) is still reported as not
// imported; the notes of the links it keeps are not, escaped "@@", padded
// pointer and CONT lines included (#1773).
func TestImportGedcom_MediaDroppedLinkNotesWarn(t *testing.T) {
	ged := "0 HEAD\n1 GEDC\n2 VERS 5.5.1\n" +
		"0 @M1@ OBJE\n1 FILE a.jpg\n2 FORM jpg\n" +
		"0 @I1@ INDI\n1 NAME John /Doe/\n" +
		"1 OBJE @M1@\n2 NOTE First link.\n" +
		"1 BIRT\n2 OBJE @M1@\n3 NOTE Second link.\n" +
		"1 DEAT\n2 OBJE @M1@\n3 NOTE First link.\n" +
		"1 OBJE\n2 FILE b.jpg\n3 FORM jpg\n2 NOTE Inline with a file.\n3 CONT Second line.\n" +
		"1 OBJE\n2 FORM jpg\n2 NOTE Inline with no file.\n" +
		"1 OBJE @M2@\n2 NOTE Write to jo@@example.com\n2 NOTE  @N1@\n" +
		"1 OBJE @M9@\n2 NOTE No such record.\n" +
		"0 @M2@ OBJE\n1 FILE c.jpg\n2 FORM jpg\n" +
		"0 @N1@ NOTE Shared.\n" +
		"0 @U1@ SUBM\n1 NAME Ann Smith\n1 OBJE @M1@\n2 NOTE Submitter link.\n0 TRLR\n"
	result, err := command.NewHandler(memory.NewEventStore(), memory.NewReadModelStore()).ImportGedcom(context.Background(), command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(ged)})
	if err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}
	var got []string
	for _, w := range result.Warnings {
		if strings.Contains(w, "not imported") {
			got = append(got, w)
		}
	}
	want := []string{
		`Individual @I1@: NOTE under BIRT.OBJE not imported: "Second link."`,
		`Individual @I1@: NOTE under DEAT.OBJE not imported: "First link."`,
		`Individual @I1@: NOTE under OBJE not imported: "Inline with no file."`,
		`Individual @I1@: NOTE under OBJE not imported: "No such record."`,
		`Submitter @U1@: NOTE under OBJE not imported: "Submitter link."`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("not-imported warnings =\n%q\nwant\n%q", got, want)
	}
}

// A media note pointer to no note record is left out of the description with
// a warning naming where it is (#1756).
func TestImportGedcom_MediaNotePointerToNoRecordWarns(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	ged := "0 HEAD\n1 GEDC\n2 VERS 7.0\n" +
		"0 @M1@ OBJE\n1 FILE a.jpg\n2 FORM image/jpeg\n1 SNOTE @N8@\n1 NOTE Kept.\n" +
		"0 @I1@ INDI\n1 NAME John /Doe/\n1 OBJE @M1@\n2 SNOTE @N9@\n0 TRLR\n"
	result, err := command.NewHandler(memory.NewEventStore(), readStore).ImportGedcom(ctx, command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(ged)})
	if err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}
	for _, want := range []string{
		"Media @M1@: OBJE note @N8@ points to no note record; not imported",
		"Individual @I1@: OBJE note @N9@ points to no note record; not imported",
	} {
		if !slices.Contains(result.Warnings, want) {
			t.Errorf("warnings %q lack %q", result.Warnings, want)
		}
	}
	if got := personMedia(t, ctx, readStore, "John").Description; got != "Kept." {
		t.Errorf("John's description = %q, want %q", got, "Kept.")
	}
}

// A private shared note an OBJE record or link points to keeps its text out
// of the media description, which carries no note restriction, with a
// warning; the note itself is still imported, private (#1756).
func TestImportGedcom_MediaPrivateSharedNoteLeftOut(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	ged := "0 HEAD\n1 GEDC\n2 VERS 7.0\n" +
		"0 @N1@ SNOTE Adopted.\n1 _RESN confidential\n" +
		"0 @N2@ SNOTE Birth mother named.\n1 RESN PRIVACY\n" +
		"0 @M1@ OBJE\n1 FILE a.jpg\n2 FORM image/jpeg\n1 SNOTE @N1@\n1 NOTE Kept.\n" +
		"0 @I1@ INDI\n1 NAME John /Doe/\n1 OBJE @M1@\n2 SNOTE @N2@\n0 TRLR\n"
	result, err := command.NewHandler(memory.NewEventStore(), readStore).ImportGedcom(ctx, command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(ged)})
	if err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}
	for _, want := range []string{
		"Media @M1@: OBJE note @N1@ is private; its text is left out of the media description",
		"Individual @I1@: OBJE note @N2@ is private; its text is left out of the media description",
	} {
		if !slices.Contains(result.Warnings, want) {
			t.Errorf("warnings %q lack %q", result.Warnings, want)
		}
	}
	if got := personMedia(t, ctx, readStore, "John").Description; got != "Kept." {
		t.Errorf("John's description = %q, want %q", got, "Kept.")
	}
	notes, _, err := readStore.ListNotes(ctx, repository.DefaultListOptions())
	if err != nil || len(notes) != 2 {
		t.Fatalf("notes = %d, %v; want 2", len(notes), err)
	}
	for _, n := range notes {
		if !n.Restriction.IsPrivate() {
			t.Errorf("note %q restriction = %q, want private", n.Text, n.Restriction)
		}
	}
}

// The crop is part of the imported version 1, so a rollback to it restores
// a crop an editor cleared (#1584).
func TestRollbackMedia_ToImportedVersionRestoresTheCrop(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)
	if _, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(linkDetailGedcom)}); err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}
	john := personMedia(t, ctx, readStore, "John")
	if _, err := handler.UpdateMedia(ctx, command.UpdateMediaInput{
		ID: john.ID, Version: john.Version,
		CropTop: command.Clear[int](), CropLeft: command.Set(1), CropHeight: command.Clear[int](), CropWidth: command.Clear[int](),
	}); err != nil {
		t.Fatalf("UpdateMedia: %v", err)
	}
	wantCrop(t, "edited item", personMedia(t, ctx, readStore, "John"), [4]*int{nil, new(1), nil, nil})

	if _, err := handler.RollbackMedia(ctx, john.ID, 1); err != nil {
		t.Fatalf("RollbackMedia: %v", err)
	}
	wantCrop(t, "rolled back item", personMedia(t, ctx, readStore, "John"), [4]*int{new(10), new(20), new(100), new(80)})
}
