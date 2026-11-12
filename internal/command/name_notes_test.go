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

// A person_name citation may name one of the owner's names (#1165), and only
// one of the owner's.
func TestCreateCitation_FactIDPersonName(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)
	if _, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(citedFactsGedcom)}); err != nil {
		t.Fatalf("import: %v", err)
	}
	persons, err := repository.ListAll(ctx, 10, readStore.ListPersons)
	if err != nil || len(persons) != 2 {
		t.Fatalf("persons = %v, %v", persons, err)
	}
	a, c := persons[0].ID, persons[1].ID
	if persons[0].GivenName != "A" {
		a, c = c, a
	}
	names, _ := readStore.GetPersonNames(ctx, domain.MainBranchID, a)
	events, _ := readStore.ListEventsForPerson(ctx, domain.MainBranchID, a)
	sources, _ := repository.ListAll(ctx, 10, readStore.ListSources)
	if len(names) != 1 || len(events) != 1 || len(sources) != 1 {
		t.Fatalf("names %d, events %d, sources %d; want one each", len(names), len(events), len(sources))
	}
	nameID := names[0].ID

	input := func(factType domain.FactType, owner, factID uuid.UUID) command.CreateCitationInput {
		return command.CreateCitationInput{SourceID: sources[0].ID, FactType: string(factType), FactOwnerID: owner, FactID: &factID}
	}
	for name, bad := range map[string]command.CreateCitationInput{
		"name of another person": input(domain.FactPersonName, c, nameID),
		"name as another fact":   input(domain.FactPersonCensus, a, nameID),
		"life event as a name":   input(domain.FactPersonName, a, events[0].ID),
		"no such name":           input(domain.FactPersonName, a, uuid.New()),
	} {
		if _, err := handler.CreateCitation(ctx, bad); !errors.Is(err, command.ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}

	created, err := handler.CreateCitation(ctx, input(domain.FactPersonName, a, nameID))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := readStore.GetCitation(ctx, domain.MainBranchID, created.ID)
	if err != nil || got == nil || got.FactID == nil || *got.FactID != nameID {
		t.Errorf("citation = %+v, %v; want fact_id %s", got, err, nameID)
	}
}

// A name's notes are set by AddName and UpdateName, and are kept when the
// name is demoted from primary (#1165).
func TestNameNotes(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)
	person, err := handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "A", Surname: "B"})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	notesOf := func(id uuid.UUID) string {
		t.Helper()
		name, err := readStore.GetPersonName(ctx, domain.MainBranchID, id)
		if err != nil || name == nil {
			t.Fatalf("get name %s: %+v, %v", id, name, err)
		}
		return name.Notes
	}

	alias, err := handler.AddName(ctx, command.AddNameInput{PersonID: person.ID, GivenName: "A", Surname: "C", NameType: "aka", Notes: "As spelled in the will."})
	if err != nil {
		t.Fatalf("add name: %v", err)
	}
	if got := notesOf(alias.ID); got != "As spelled in the will." {
		t.Errorf("notes after add = %q", got)
	}

	notes := "Spelling varies."
	if _, err := handler.UpdateName(ctx, command.UpdateNameInput{PersonID: person.ID, NameID: alias.ID, Notes: &notes}); err != nil {
		t.Fatalf("update name: %v", err)
	}
	if got := notesOf(alias.ID); got != notes {
		t.Errorf("notes after update = %q, want %q", got, notes)
	}

	// Promoting the alias, then adding a new primary name, demotes the alias
	// with its notes intact.
	primary := true
	if _, err := handler.UpdateName(ctx, command.UpdateNameInput{PersonID: person.ID, NameID: alias.ID, IsPrimary: &primary}); err != nil {
		t.Fatalf("promote alias: %v", err)
	}
	if _, err := handler.AddName(ctx, command.AddNameInput{PersonID: person.ID, GivenName: "A", Surname: "D", IsPrimary: true}); err != nil {
		t.Fatalf("add primary: %v", err)
	}
	if got := notesOf(alias.ID); got != notes {
		t.Errorf("notes after demotion = %q, want %q", got, notes)
	}
}
