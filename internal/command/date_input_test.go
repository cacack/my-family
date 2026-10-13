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

// A source's publish date is a strict date like every other entered date
// (issue #957); free text can still be kept as a date phrase.
func TestSource_RejectsInvalidPublishDate(t *testing.T) {
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)
	ctx := context.Background()

	_, err := handler.CreateSource(ctx, command.CreateSourceInput{Title: "Census", PublishDate: "31 FEB 1900"})
	requireDateError(t, err, "publish_date", "is not a real calendar date")

	_, err = handler.CreateSource(ctx, command.CreateSourceInput{Title: "Census", PublishDate: "garbage"})
	requireDateError(t, err, "publish_date", "is not a recognized date")

	created, err := handler.CreateSource(ctx, command.CreateSourceInput{Title: "Census", PublishDate: "15 Mar 1920"})
	if err != nil {
		t.Fatalf("CreateSource: %v", err)
	}
	source, _ := readStore.GetSource(ctx, domain.MainBranchID, created.ID)
	if source.PublishDateRaw != "15 MAR 1920" {
		t.Errorf("PublishDateRaw = %q, want 15 MAR 1920", source.PublishDateRaw)
	}

	_, err = handler.UpdateSource(ctx, command.UpdateSourceInput{ID: created.ID, PublishDate: strPtr("31 FEB 1900"), Version: created.Version})
	requireDateError(t, err, "publish_date", "is not a real calendar date")

	_, err = handler.UpdateSource(ctx, command.UpdateSourceInput{ID: created.ID, PublishDate: strPtr("garbage"), Version: created.Version})
	requireDateError(t, err, "publish_date", "is not a recognized date")

	// Retyping the stored date in English is not a change.
	same, err := handler.UpdateSource(ctx, command.UpdateSourceInput{ID: created.ID, PublishDate: strPtr("15 March 1920"), Version: created.Version})
	if err != nil {
		t.Fatalf("UpdateSource with the same date: %v", err)
	}
	if same.Version != created.Version {
		t.Errorf("version = %d, want unchanged %d", same.Version, created.Version)
	}

	if _, err := handler.UpdateSource(ctx, command.UpdateSourceInput{ID: created.ID, PublishDate: strPtr("(second edition, undated)"), Version: created.Version}); err != nil {
		t.Fatalf("UpdateSource with a date phrase: %v", err)
	}
	source, _ = readStore.GetSource(ctx, domain.MainBranchID, created.ID)
	if source.PublishDateRaw != "(second edition, undated)" {
		t.Errorf("PublishDateRaw = %q, want the phrase kept", source.PublishDateRaw)
	}
}

// Plain-English dates are stored in GEDCOM form on every date entry point
// (issue #978).
func TestDateInput_StoresEnglishAsGEDCOM(t *testing.T) {
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)
	ctx := context.Background()

	created, err := handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "John", BirthDate: "about 1850", DeathDate: "before Sept 1900"})
	if err != nil {
		t.Fatalf("CreatePerson: %v", err)
	}
	person, _ := readStore.GetPerson(ctx, domain.MainBranchID, created.ID)
	if person.BirthDateRaw != "ABT 1850" || person.DeathDateRaw != "BEF SEP 1900" {
		t.Errorf("created dates = %q, %q, want ABT 1850, BEF SEP 1900", person.BirthDateRaw, person.DeathDateRaw)
	}

	updated, err := handler.UpdatePerson(ctx, command.UpdatePersonInput{ID: created.ID, BirthDate: strPtr("circa 1851"), DeathDate: strPtr("between 1900 and 1905"), Version: created.Version})
	if err != nil {
		t.Fatalf("UpdatePerson: %v", err)
	}
	person, _ = readStore.GetPerson(ctx, domain.MainBranchID, created.ID)
	if person.BirthDateRaw != "ABT 1851" || person.DeathDateRaw != "BET 1900 AND 1905" {
		t.Errorf("updated dates = %q, %q, want ABT 1851, BET 1900 AND 1905", person.BirthDateRaw, person.DeathDateRaw)
	}

	// Typing the stored date in English again is not a change.
	same, err := handler.UpdatePerson(ctx, command.UpdatePersonInput{ID: created.ID, BirthDate: strPtr("c. 1851"), Version: updated.Version})
	if err != nil {
		t.Fatalf("UpdatePerson with the same date: %v", err)
	}
	if same.Version != updated.Version {
		t.Errorf("version = %d, want %d unchanged", same.Version, updated.Version)
	}

	family, err := handler.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &created.ID, MarriageDate: "5 June 1875"})
	if err != nil {
		t.Fatalf("CreateFamily: %v", err)
	}
	f, _ := readStore.GetFamily(ctx, domain.MainBranchID, family.ID)
	if f.MarriageDateRaw != "5 JUN 1875" {
		t.Errorf("created marriage date = %q, want 5 JUN 1875", f.MarriageDateRaw)
	}
	if _, err := handler.UpdateFamily(ctx, command.UpdateFamilyInput{ID: family.ID, MarriageDate: strPtr("after 1875"), Version: family.Version}); err != nil {
		t.Fatalf("UpdateFamily: %v", err)
	}
	f, _ = readStore.GetFamily(ctx, domain.MainBranchID, family.ID)
	if f.MarriageDateRaw != "AFT 1875" {
		t.Errorf("updated marriage date = %q, want AFT 1875", f.MarriageDateRaw)
	}

	ord, err := handler.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{Type: domain.LDSBaptism, PersonID: &created.ID, Date: "from 1860 to 1861"})
	if err != nil {
		t.Fatalf("CreateLDSOrdinance: %v", err)
	}
	o, _ := readStore.GetLDSOrdinance(ctx, ord.ID)
	if o.DateRaw != "FROM 1860 TO 1861" {
		t.Errorf("created ordinance date = %q, want FROM 1860 TO 1861", o.DateRaw)
	}
	if _, err := handler.UpdateLDSOrdinance(ctx, command.UpdateLDSOrdinanceInput{ID: ord.ID, Date: strPtr("about Jan 1862"), Version: ord.Version}); err != nil {
		t.Fatalf("UpdateLDSOrdinance: %v", err)
	}
	o, _ = readStore.GetLDSOrdinance(ctx, ord.ID)
	if o.DateRaw != "ABT JAN 1862" {
		t.Errorf("updated ordinance date = %q, want ABT JAN 1862", o.DateRaw)
	}
}

// A rejected date says whether it can be kept as a date phrase instead.
func TestDateInput_RejectionReportsUnreadable(t *testing.T) {
	handler := command.NewHandler(memory.NewEventStore(), memory.NewReadModelStore())
	ctx := context.Background()

	for input, want := range map[string]bool{"Sept 5, 1850": true, "31 February 1900": false} {
		_, err := handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "John", BirthDate: input})
		var dateErr *command.InvalidDateError
		var reason *domain.DateInputError
		if !errors.As(err, &dateErr) || !errors.As(err, &reason) {
			t.Fatalf("%q: err = %v, want *InvalidDateError wrapping *DateInputError", input, err)
		}
		if dateErr.Field != "birth_date" || reason.Unreadable != want {
			t.Errorf("%q: field %q unreadable %v, want birth_date %v", input, dateErr.Field, reason.Unreadable, want)
		}
	}
}
