package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// An update may leave both name parts empty only when they already were (an
// imported person with no NAME stays editable); it may not make a named
// person nameless (#1709).
func TestUpdatePerson_NamelessRule(t *testing.T) {
	empty := ""
	tests := []struct {
		name           string
		given, surname string
		input          command.UpdatePersonInput
		wantInvalid    bool
		wantGiven      string
		wantSurname    string
	}{
		{
			name: "named person cleared of both parts", given: "John", surname: "Doe",
			input:       command.UpdatePersonInput{GivenName: &empty, Surname: command.Set(empty)},
			wantInvalid: true, wantGiven: "John", wantSurname: "Doe",
		},
		{
			name: "given-name-only person cleared of its given name", given: "Madonna",
			input:       command.UpdatePersonInput{GivenName: &empty},
			wantInvalid: true, wantGiven: "Madonna",
		},
		{
			name: "named person cleared of its given name keeps the surname", given: "John", surname: "Doe",
			input:     command.UpdatePersonInput{GivenName: &empty},
			wantGiven: "", wantSurname: "Doe",
		},
		{
			name:  "nameless person edited and left nameless",
			input: command.UpdatePersonInput{GivenName: &empty, Surname: command.Set(empty), Notes: command.Set("No name in the source")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventStore := memory.NewEventStore()
			readStore := memory.NewReadModelStore()
			handler := command.NewHandler(eventStore, readStore)
			ctx := context.Background()

			id := createPersonWithoutNames(t, eventStore, readStore, tt.given, tt.surname)
			input := tt.input
			input.ID = id
			input.Version = 1
			_, err := handler.UpdatePerson(ctx, input)
			if tt.wantInvalid {
				if !errors.Is(err, command.ErrInvalidInput) {
					t.Fatalf("UpdatePerson error = %v, want ErrInvalidInput", err)
				}
			} else if err != nil {
				t.Fatalf("UpdatePerson failed: %v", err)
			}

			got, _ := readStore.GetPerson(ctx, domain.MainBranchID, id)
			if got.GivenName != tt.wantGiven || got.Surname != tt.wantSurname {
				t.Errorf("name = %q/%q, want %q/%q", got.GivenName, got.Surname, tt.wantGiven, tt.wantSurname)
			}
		})
	}
}

// addNameDirectly records a NameAdded without AddName's validation, as an
// import of `1 NAME //` can leave a name with neither part.
func addNameDirectly(t *testing.T, eventStore repository.EventStore, readStore repository.ReadModelStore, personID uuid.UUID, given, surname string, primary bool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	person, err := readStore.GetPerson(ctx, domain.MainBranchID, personID)
	if err != nil || person == nil {
		t.Fatalf("get person: %v", err)
	}
	pn := domain.NewPersonName(personID, given, surname)
	pn.IsPrimary = primary
	event := domain.NewNameAdded(pn)
	if err := eventStore.Append(ctx, personID, "Person", []domain.Event{event}, person.Version, repository.AppendScope{}); err != nil {
		t.Fatalf("append NameAdded: %v", err)
	}
	if err := repository.NewProjector(readStore, nil).Project(ctx, event, person.Version+1, domain.MainBranchID); err != nil {
		t.Fatalf("project NameAdded: %v", err)
	}
	return pn.ID
}

func TestUpdateName_NamelessRule(t *testing.T) {
	empty := ""
	notes := "No name in the source"
	tests := []struct {
		name           string
		given, surname string
		input          command.UpdateNameInput
		wantInvalid    bool
		wantGiven      string
		wantSurname    string
	}{
		{
			name: "named name cleared of both parts", given: "John", surname: "Doe",
			input:       command.UpdateNameInput{GivenName: &empty, Surname: &empty},
			wantInvalid: true, wantGiven: "John", wantSurname: "Doe",
		},
		{
			name: "named name cleared of its given name keeps the surname", given: "John", surname: "Doe",
			input:       command.UpdateNameInput{GivenName: &empty},
			wantSurname: "Doe",
		},
		{
			name:  "nameless name edited and left nameless",
			input: command.UpdateNameInput{GivenName: &empty, Surname: &empty, Notes: command.Set(notes)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventStore := memory.NewEventStore()
			readStore := memory.NewReadModelStore()
			handler := command.NewHandler(eventStore, readStore)
			ctx := context.Background()

			personID := createPersonWithoutNames(t, eventStore, readStore, tt.given, tt.surname)
			nameID := addNameDirectly(t, eventStore, readStore, personID, tt.given, tt.surname, true)
			input := tt.input
			input.PersonID = personID
			input.NameID = nameID
			_, err := handler.UpdateName(ctx, input)
			if tt.wantInvalid {
				if !errors.Is(err, command.ErrInvalidInput) {
					t.Fatalf("UpdateName error = %v, want ErrInvalidInput", err)
				}
			} else if err != nil {
				t.Fatalf("UpdateName failed: %v", err)
			}

			got, _ := readStore.GetPersonName(ctx, domain.MainBranchID, nameID)
			if got.GivenName != tt.wantGiven || got.Surname != tt.wantSurname {
				t.Errorf("name = %q/%q, want %q/%q", got.GivenName, got.Surname, tt.wantGiven, tt.wantSurname)
			}
		})
	}
}

// Making a nameless alternate name primary would copy its empty parts onto a
// named person, so it is refused like clearing the person's name (#1709).
func TestUpdateName_PromotingNamelessNameKeepsPersonNamed(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	personID := createPersonWithoutNames(t, eventStore, readStore, "John", "Doe")
	addNameDirectly(t, eventStore, readStore, personID, "John", "Doe", true)
	namelessID := addNameDirectly(t, eventStore, readStore, personID, "", "", false)

	primary := true
	_, err := handler.UpdateName(ctx, command.UpdateNameInput{PersonID: personID, NameID: namelessID, IsPrimary: &primary})
	if !errors.Is(err, command.ErrInvalidInput) {
		t.Fatalf("UpdateName error = %v, want ErrInvalidInput", err)
	}
	got, _ := readStore.GetPerson(ctx, domain.MainBranchID, personID)
	if got.GivenName != "John" || got.Surname != "Doe" {
		t.Errorf("person name = %q/%q, want John/Doe", got.GivenName, got.Surname)
	}
}
