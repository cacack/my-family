package command_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository/memory"
)

// requireDateError asserts err is an invalid-input error naming the field and
// the date problem, not a downstream comparison (issue #904).
func requireDateError(t *testing.T, err error, field, problem string) {
	t.Helper()
	if !errors.Is(err, command.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	if want := field + ": "; !strings.Contains(err.Error(), want) {
		t.Errorf("err = %q, want it to name %q", err, field)
	}
	if !strings.Contains(err.Error(), problem) {
		t.Errorf("err = %q, want it to contain %q", err, problem)
	}
}

func TestCreatePerson_RejectsInvalidDates(t *testing.T) {
	tests := []struct {
		name    string
		input   command.CreatePersonInput
		field   string
		problem string
	}{
		{
			name:    "unrecognized birth date",
			input:   command.CreatePersonInput{GivenName: "John", BirthDate: "garbage"},
			field:   "birth_date",
			problem: "is not a recognized date",
		},
		{
			name:    "ISO birth date",
			input:   command.CreatePersonInput{GivenName: "John", BirthDate: "1850-01-01"},
			field:   "birth_date",
			problem: "is not a recognized date",
		},
		{
			name:    "impossible birth date",
			input:   command.CreatePersonInput{GivenName: "John", BirthDate: "31 FEB 1900"},
			field:   "birth_date",
			problem: "is not a real calendar date",
		},
		{
			name:    "year-less death date is reported as such, not as before birth",
			input:   command.CreatePersonInput{GivenName: "John", BirthDate: "1 JAN 1850", DeathDate: "31 FEB 19xx"},
			field:   "death_date",
			problem: "is not a recognized date",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := command.NewHandler(memory.NewEventStore(), memory.NewReadModelStore())
			_, err := handler.CreatePerson(context.Background(), tt.input)
			requireDateError(t, err, tt.field, tt.problem)
		})
	}
}

func TestCreatePerson_AcceptsDatePhrase(t *testing.T) {
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)
	ctx := context.Background()

	result, err := handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "John", BirthDate: "(during the war)"})
	if err != nil {
		t.Fatalf("CreatePerson with a date phrase: %v", err)
	}
	person, _ := readStore.GetPerson(ctx, domain.MainBranchID, result.ID)
	if person == nil || person.BirthDateRaw != "(during the war)" {
		t.Errorf("BirthDateRaw = %+v, want the phrase kept", person)
	}
}

func TestUpdatePerson_RejectsInvalidDates(t *testing.T) {
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)
	ctx := context.Background()

	created, err := handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "John", BirthDate: "1 JAN 1850"})
	if err != nil {
		t.Fatalf("CreatePerson: %v", err)
	}

	_, err = handler.UpdatePerson(ctx, command.UpdatePersonInput{ID: created.ID, BirthDate: strPtr("garbage"), Version: created.Version})
	requireDateError(t, err, "birth_date", "is not a recognized date")

	_, err = handler.UpdatePerson(ctx, command.UpdatePersonInput{ID: created.ID, DeathDate: strPtr("31 FEB 19xx"), Version: created.Version})
	requireDateError(t, err, "death_date", "is not a recognized date")
}

// A stored date the parser can't read (e.g. from an older import) must not
// block editing some other field.
func TestUpdatePerson_UnreadableStoredDateDoesNotBlockOtherEdits(t *testing.T) {
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)
	ctx := context.Background()

	created, err := handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "John"})
	if err != nil {
		t.Fatalf("CreatePerson: %v", err)
	}
	person, _ := readStore.GetPerson(ctx, domain.MainBranchID, created.ID)
	person.BirthDateRaw = "31 FEB 1900"
	if err := readStore.SavePerson(ctx, domain.MainBranchID, person); err != nil {
		t.Fatalf("SavePerson: %v", err)
	}

	if _, err := handler.UpdatePerson(ctx, command.UpdatePersonInput{ID: created.ID, Notes: strPtr("edited"), Version: created.Version}); err != nil {
		t.Errorf("UpdatePerson of notes = %v, want nil", err)
	}
}

func TestFamily_RejectsInvalidMarriageDate(t *testing.T) {
	handler := command.NewHandler(memory.NewEventStore(), memory.NewReadModelStore())
	ctx := context.Background()

	p1, err := handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "John"})
	if err != nil {
		t.Fatalf("CreatePerson: %v", err)
	}

	_, err = handler.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &p1.ID, MarriageDate: "garbage"})
	requireDateError(t, err, "marriage_date", "is not a recognized date")

	family, err := handler.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &p1.ID, MarriageDate: "1 JUN 1875"})
	if err != nil {
		t.Fatalf("CreateFamily: %v", err)
	}
	_, err = handler.UpdateFamily(ctx, command.UpdateFamilyInput{ID: family.ID, MarriageDate: strPtr("31 FEB 1900"), Version: family.Version})
	requireDateError(t, err, "marriage_date", "is not a real calendar date")

	// Clearing the date stays allowed.
	if _, err := handler.UpdateFamily(ctx, command.UpdateFamilyInput{ID: family.ID, MarriageDate: strPtr(""), Version: family.Version}); err != nil {
		t.Errorf("clearing marriage_date = %v, want nil", err)
	}
}

func TestLDSOrdinance_RejectsInvalidDate(t *testing.T) {
	handler := command.NewHandler(memory.NewEventStore(), memory.NewReadModelStore())
	ctx := context.Background()

	p1, err := handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "John"})
	if err != nil {
		t.Fatalf("CreatePerson: %v", err)
	}

	_, err = handler.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{Type: domain.LDSBaptism, PersonID: &p1.ID, Date: "1850-01-01"})
	requireDateError(t, err, "date", "is not a recognized date")

	ord, err := handler.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{Type: domain.LDSBaptism, PersonID: &p1.ID, Date: "15 JAN 1900"})
	if err != nil {
		t.Fatalf("CreateLDSOrdinance: %v", err)
	}
	_, err = handler.UpdateLDSOrdinance(ctx, command.UpdateLDSOrdinanceInput{ID: ord.ID, Date: strPtr("garbage"), Version: ord.Version})
	requireDateError(t, err, "date", "is not a recognized date")
}
