package command_test

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
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
