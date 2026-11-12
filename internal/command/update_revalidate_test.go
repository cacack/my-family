package command_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
)

// Update commands validate the entity as it would be after the update, as the
// create commands do, so an update can't put an invalid value (an empty name,
// an over-long field, an unknown enum) into the event stream (#527 item 7).

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
