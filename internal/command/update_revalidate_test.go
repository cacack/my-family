package command_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// Update commands validate the fields they change, as the create commands do,
// so an update can't put an invalid value (an empty name, an over-long field,
// an unknown enum) into the event stream (#527 item 7). Values GEDCOM import
// stores as written (LDS status, temple and place; repository and submitter
// names; association roles, phrases and self-references) are not
// re-validated when the update leaves them alone, so they can't block an
// unrelated edit.

func TestUpdateCommands_RejectInvalidResult(t *testing.T) {
	long := func(n int) string { return strings.Repeat("x", n) }

	// run submits the update and checks it is refused as invalid input with no
	// event appended to the entity's stream.
	run := func(t *testing.T, name string, update func(t *testing.T) (updateFixture[struct{}], error)) {
		t.Run(name, func(t *testing.T) {
			f, err := update(t)
			if !errors.Is(err, command.ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
			checkRecorded(t, f.events, f.stream, int(f.version), nil)
		})
	}

	run(t, "repository empty name", func(t *testing.T) (updateFixture[struct{}], error) {
		ctx, events, _, h := newTestHandler()
		created, err := h.CreateRepository(ctx, command.CreateRepositoryInput{Name: "Archives"})
		if err != nil {
			t.Fatalf("CreateRepository: %v", err)
		}
		_, err = h.UpdateRepository(ctx, command.UpdateRepositoryInput{ID: created.ID, Name: ptr(""), Version: created.Version})
		return updateFixture[struct{}]{events: events, stream: created.ID, version: created.Version}, err
	})

	run(t, "submitter over-long name", func(t *testing.T) (updateFixture[struct{}], error) {
		ctx, events, _, h := newTestHandler()
		created, err := h.CreateSubmitter(ctx, command.CreateSubmitterInput{Name: "Jane"})
		if err != nil {
			t.Fatalf("CreateSubmitter: %v", err)
		}
		_, err = h.UpdateSubmitter(ctx, command.UpdateSubmitterInput{ID: created.ID, Name: ptr(long(201)), Version: created.Version})
		return updateFixture[struct{}]{events: events, stream: created.ID, version: created.Version}, err
	})

	run(t, "media empty title", func(t *testing.T) (updateFixture[struct{}], error) {
		ctx, events, _, h := newTestHandler()
		person, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "John", Surname: "Doe"})
		if err != nil {
			t.Fatalf("CreatePerson: %v", err)
		}
		created, err := h.UploadMedia(ctx, command.UploadMediaInput{
			EntityType: "person", EntityID: person.ID, Title: "Portrait",
			Filename: "portrait.jpg", FileData: createTestJPEG(),
		})
		if err != nil {
			t.Fatalf("UploadMedia: %v", err)
		}
		_, err = h.UpdateMedia(ctx, command.UpdateMediaInput{ID: created.ID, Title: ptr(""), Version: created.Version})
		return updateFixture[struct{}]{events: events, stream: created.ID, version: created.Version}, err
	})

	run(t, "media unknown type", func(t *testing.T) (updateFixture[struct{}], error) {
		ctx, events, _, h := newTestHandler()
		person, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "John", Surname: "Doe"})
		if err != nil {
			t.Fatalf("CreatePerson: %v", err)
		}
		created, err := h.UploadMedia(ctx, command.UploadMediaInput{
			EntityType: "person", EntityID: person.ID, Title: "Portrait",
			Filename: "portrait.jpg", FileData: createTestJPEG(),
		})
		if err != nil {
			t.Fatalf("UploadMedia: %v", err)
		}
		_, err = h.UpdateMedia(ctx, command.UpdateMediaInput{ID: created.ID, MediaType: ptr("hologram"), Version: created.Version})
		return updateFixture[struct{}]{events: events, stream: created.ID, version: created.Version}, err
	})

	run(t, "LDS ordinance over-long temple", func(t *testing.T) (updateFixture[struct{}], error) {
		ctx, events, _, h := newTestHandler()
		person, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "John", Surname: "Doe"})
		if err != nil {
			t.Fatalf("CreatePerson: %v", err)
		}
		created, err := h.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{Type: domain.LDSBaptism, PersonID: &person.ID, Temple: "SLAKE"})
		if err != nil {
			t.Fatalf("CreateLDSOrdinance: %v", err)
		}
		_, err = h.UpdateLDSOrdinance(ctx, command.UpdateLDSOrdinanceInput{ID: created.ID, Temple: ptr(long(11)), Version: created.Version})
		return updateFixture[struct{}]{events: events, stream: created.ID, version: created.Version}, err
	})

	run(t, "LDS ordinance unknown status", func(t *testing.T) (updateFixture[struct{}], error) {
		ctx, events, _, h := newTestHandler()
		person, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "John", Surname: "Doe"})
		if err != nil {
			t.Fatalf("CreatePerson: %v", err)
		}
		created, err := h.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{Type: domain.LDSBaptism, PersonID: &person.ID})
		if err != nil {
			t.Fatalf("CreateLDSOrdinance: %v", err)
		}
		_, err = h.UpdateLDSOrdinance(ctx, command.UpdateLDSOrdinanceInput{ID: created.ID, Status: ptr("NOPE"), Version: created.Version})
		return updateFixture[struct{}]{events: events, stream: created.ID, version: created.Version}, err
	})

	association := func(t *testing.T, edit func(*command.UpdateAssociationInput)) (updateFixture[struct{}], error) {
		ctx, events, _, h := newTestHandler()
		p1, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "John", Surname: "Doe"})
		if err != nil {
			t.Fatalf("CreatePerson: %v", err)
		}
		p2, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Mary", Surname: "Jones"})
		if err != nil {
			t.Fatalf("CreatePerson: %v", err)
		}
		created, err := h.CreateAssociation(ctx, command.CreateAssociationInput{PersonID: p1.ID, AssociateID: p2.ID, Role: domain.RoleWitness})
		if err != nil {
			t.Fatalf("CreateAssociation: %v", err)
		}
		input := command.UpdateAssociationInput{ID: created.ID, Version: created.Version}
		edit(&input)
		_, err = h.UpdateAssociation(ctx, input)
		return updateFixture[struct{}]{events: events, stream: created.ID, version: created.Version}, err
	}
	run(t, "association empty role", func(t *testing.T) (updateFixture[struct{}], error) {
		return association(t, func(in *command.UpdateAssociationInput) { in.Role = ptr("") })
	})
	run(t, "association over-long role", func(t *testing.T) (updateFixture[struct{}], error) {
		return association(t, func(in *command.UpdateAssociationInput) { in.Role = ptr(long(101)) })
	})
	run(t, "association over-long phrase", func(t *testing.T) (updateFixture[struct{}], error) {
		return association(t, func(in *command.UpdateAssociationInput) { in.Phrase = ptr(long(501)) })
	})
}

// A repository or submitter imported without a NAME keeps its empty name; an
// edit that leaves the name alone still succeeds.
func TestUpdateCommands_ImportedNameDoesNotBlockUnrelatedEdit(t *testing.T) {
	const ged = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @R1@ REPO
1 ADDR Main Street
0 @U1@ SUBM
1 ADDR Main Street
0 @I1@ INDI
1 NAME A /B/
0 TRLR
`
	ctx, _, read, h := newTestHandler()
	if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(ged)}); err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}
	opts := repository.ListOptions{Limit: 10}

	repos, _, err := read.ListRepositories(ctx, opts)
	if err != nil || len(repos) != 1 || repos[0].Name != "" {
		t.Fatalf("imported repositories = %+v, err %v; want one with an empty name", repos, err)
	}
	if _, err := h.UpdateRepository(ctx, command.UpdateRepositoryInput{ID: repos[0].ID, Notes: ptr("seen"), Version: repos[0].Version}); err != nil {
		t.Errorf("notes-only update of an imported repository: %v", err)
	}

	subs, _, err := read.ListSubmitters(ctx, opts)
	if err != nil || len(subs) != 1 || subs[0].Name != "" {
		t.Fatalf("imported submitters = %+v, err %v; want one with an empty name", subs, err)
	}
	if _, err := h.UpdateSubmitter(ctx, command.UpdateSubmitterInput{ID: subs[0].ID, Phone: []string{"555-0100"}, Version: subs[0].Version}); err != nil {
		t.Errorf("phone-only update of an imported submitter: %v", err)
	}
}

// Import stores an association's RELA/ROLE text, PHRASE and associate as
// written (#1355): an over-long role, an over-long phrase or a self-reference
// blocks no edit that leaves it alone, and a changed role or phrase is still
// validated.
func TestUpdateCommands_ImportedAssociationDoesNotBlockUnrelatedEdit(t *testing.T) {
	long := func(n int) string { return strings.Repeat("x", n) }
	ged := `0 HEAD
1 GEDC
2 VERS 7.0
0 @I1@ INDI
1 NAME Self /Ref/
1 ASSO @I1@
2 ROLE FRIEND
0 @I2@ INDI
1 NAME Long /Role/
1 ASSO @I1@
2 ROLE ` + long(120) + `
0 @I3@ INDI
1 NAME Long /Phrase/
1 ASSO @I1@
2 PHRASE ` + long(600) + `
2 ROLE FRIEND
0 TRLR
`
	// imported returns a fresh import's association for which match holds.
	imported := func(t *testing.T, match func(repository.AssociationReadModel) bool) (*command.Handler, *memory.EventStore, repository.AssociationReadModel) {
		t.Helper()
		ctx, events, read, h := newTestHandler()
		if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(ged)}); err != nil {
			t.Fatalf("ImportGedcom: %v", err)
		}
		assocs, _, err := read.ListAssociations(ctx, repository.ListOptions{Limit: 10})
		if err != nil {
			t.Fatalf("ListAssociations: %v", err)
		}
		for _, a := range assocs {
			if match(a) {
				return h, events, a
			}
		}
		t.Fatalf("no matching imported association in %+v", assocs)
		return nil, nil, repository.AssociationReadModel{}
	}
	selfRef := func(a repository.AssociationReadModel) bool { return a.PersonID == a.AssociateID }
	longRole := func(a repository.AssociationReadModel) bool { return len(a.Role) > 100 }
	longPhrase := func(a repository.AssociationReadModel) bool { return len(a.Phrase) > 500 }

	for name, match := range map[string]func(repository.AssociationReadModel) bool{
		"self-reference": selfRef, "over-long role": longRole, "over-long phrase": longPhrase,
	} {
		t.Run(name+" notes-only", func(t *testing.T) {
			h, events, a := imported(t, match)
			if _, err := h.UpdateAssociation(context.Background(), command.UpdateAssociationInput{ID: a.ID, Notes: ptr("seen"), Version: a.Version}); err != nil {
				t.Fatalf("notes-only update: %v", err)
			}
			checkRecorded(t, events, a.ID, int(a.Version), []string{"notes"})
		})
		t.Run(name+" note-IDs-only", func(t *testing.T) {
			h, events, a := imported(t, match)
			if _, err := h.UpdateAssociation(context.Background(), command.UpdateAssociationInput{ID: a.ID, NoteIDs: ptr([]uuid.UUID{uuid.New()}), Version: a.Version}); err != nil {
				t.Fatalf("note-IDs-only update: %v", err)
			}
			checkRecorded(t, events, a.ID, int(a.Version), []string{"note_ids"})
		})
	}

	t.Run("self-reference role change validates the role only", func(t *testing.T) {
		h, events, a := imported(t, selfRef)
		if _, err := h.UpdateAssociation(context.Background(), command.UpdateAssociationInput{ID: a.ID, Role: ptr(domain.RoleWitness), Version: a.Version}); err != nil {
			t.Fatalf("role update of a self-referencing association: %v", err)
		}
		checkRecorded(t, events, a.ID, int(a.Version), []string{"role"})
	})

	for name, edit := range map[string]func(*command.UpdateAssociationInput){
		"invalid role":   func(in *command.UpdateAssociationInput) { in.Role = ptr(long(101)) },
		"empty role":     func(in *command.UpdateAssociationInput) { in.Role = ptr("") },
		"invalid phrase": func(in *command.UpdateAssociationInput) { in.Phrase = ptr(long(501)) },
	} {
		t.Run("self-reference "+name+" refused", func(t *testing.T) {
			h, events, a := imported(t, selfRef)
			input := command.UpdateAssociationInput{ID: a.ID, Notes: ptr("seen"), Version: a.Version}
			edit(&input)
			_, err := h.UpdateAssociation(context.Background(), input)
			if !errors.Is(err, command.ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
			checkRecorded(t, events, a.ID, int(a.Version), nil)
		})
	}
}
