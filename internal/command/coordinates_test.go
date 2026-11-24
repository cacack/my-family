package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func pair(lat, long string) command.CoordinatesPatch {
	return command.CoordinatesPatch{Latitude: command.Set(lat), Longitude: command.Set(long)}
}

func derefOr(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

// Coordinates given on a create are stored with the person and family (#1604).
func TestCreate_PlaceCoordinates(t *testing.T) {
	ctx := context.Background()
	rs := memory.NewReadModelStore()
	h := command.NewHandler(memory.NewEventStore(), rs)

	created, err := h.CreatePerson(ctx, command.CreatePersonInput{
		GivenName:             "Ada",
		BirthPlaceCoordinates: pair("N42.3601", "W71.0589"),
		DeathPlaceCoordinates: pair("S33.8688", "E151.2093"),
	})
	if err != nil {
		t.Fatalf("CreatePerson: %v", err)
	}
	p, _ := rs.GetPerson(ctx, domain.MainBranchID, created.ID)
	if derefOr(p.BirthPlaceLat) != "N42.3601" || derefOr(p.BirthPlaceLong) != "W71.0589" ||
		derefOr(p.DeathPlaceLat) != "S33.8688" || derefOr(p.DeathPlaceLong) != "E151.2093" {
		t.Errorf("person coordinates = %s %s %s %s", derefOr(p.BirthPlaceLat), derefOr(p.BirthPlaceLong), derefOr(p.DeathPlaceLat), derefOr(p.DeathPlaceLong))
	}

	fam, err := h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &created.ID, MarriagePlaceCoordinates: pair("N39.7817", "W89.6501")})
	if err != nil {
		t.Fatalf("CreateFamily: %v", err)
	}
	f, _ := rs.GetFamily(ctx, domain.MainBranchID, fam.ID)
	if derefOr(f.MarriagePlaceLat) != "N39.7817" || derefOr(f.MarriagePlaceLong) != "W89.6501" {
		t.Errorf("family coordinates = %s %s", derefOr(f.MarriagePlaceLat), derefOr(f.MarriagePlaceLong))
	}
}

// A pair that is incomplete, on the wrong axis or out of range is rejected
// naming the field, and nothing is stored (#1604).
func TestPlaceCoordinates_Rejected(t *testing.T) {
	tests := []struct {
		name      string
		input     command.CoordinatesPatch
		wantField string
	}{
		{"latitude with E", pair("E42.3601", "W71.0589"), "birth_place_latitude"},
		{"longitude with N", pair("N42.3601", "N71.0589"), "birth_place_longitude"},
		{"latitude out of range", pair("N91", "W71.0589"), "birth_place_latitude"},
		{"longitude out of range", pair("N42.3601", "W181"), "birth_place_longitude"},
		{"latitude empty", pair("", "W71.0589"), "birth_place_latitude"},
		{"latitude alone", command.CoordinatesPatch{Latitude: command.Set("N42.3601")}, "birth_place_longitude"},
		{"longitude alone", command.CoordinatesPatch{Longitude: command.Set("W71.0589")}, "birth_place_latitude"},
		{"latitude cleared beside a longitude", command.CoordinatesPatch{Latitude: command.Clear[string](), Longitude: command.Set("W71.0589")}, "birth_place_latitude"},
		{"longitude cleared alone", command.CoordinatesPatch{Longitude: command.Clear[string]()}, "birth_place_latitude"},
		{"latitude cleared alone", command.CoordinatesPatch{Latitude: command.Clear[string]()}, "birth_place_longitude"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			rs := memory.NewReadModelStore()
			h := command.NewHandler(memory.NewEventStore(), rs)

			_, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", BirthPlaceCoordinates: tt.input})
			assertCoordinateError(t, "CreatePerson", err, tt.wantField)
			if persons, total, _ := rs.ListPersons(ctx, repository.ListOptions{Limit: 10}); total != 0 {
				t.Errorf("CreatePerson stored %d persons, want none", len(persons))
			}

			created, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", BirthPlaceCoordinates: pair("N1", "E1")})
			if err != nil {
				t.Fatalf("CreatePerson: %v", err)
			}
			_, err = h.UpdatePerson(ctx, command.UpdatePersonInput{ID: created.ID, Version: created.Version, BirthPlaceCoordinates: tt.input})
			assertCoordinateError(t, "UpdatePerson", err, tt.wantField)
			p, _ := rs.GetPerson(ctx, domain.MainBranchID, created.ID)
			if p.Version != created.Version || derefOr(p.BirthPlaceLat) != "N1" || derefOr(p.BirthPlaceLong) != "E1" {
				t.Errorf("UpdatePerson stored a change: version %d, %s %s", p.Version, derefOr(p.BirthPlaceLat), derefOr(p.BirthPlaceLong))
			}

			_, err = h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &created.ID, MarriagePlaceCoordinates: tt.input})
			assertCoordinateError(t, "CreateFamily", err, "marriage"+tt.wantField[len("birth"):])
			fam, err := h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &created.ID})
			if err != nil {
				t.Fatalf("CreateFamily: %v", err)
			}
			_, err = h.UpdateFamily(ctx, command.UpdateFamilyInput{ID: fam.ID, Version: fam.Version, MarriagePlaceCoordinates: tt.input})
			assertCoordinateError(t, "UpdateFamily", err, "marriage"+tt.wantField[len("birth"):])
			if f, _ := rs.GetFamily(ctx, domain.MainBranchID, fam.ID); f.Version != fam.Version {
				t.Errorf("UpdateFamily stored a change: version %d", f.Version)
			}
		})
	}
}

func assertCoordinateError(t *testing.T, op string, err error, field string) {
	t.Helper()
	var coordErr *command.InvalidCoordinateError
	if !errors.As(err, &coordErr) || coordErr.Field != field || !errors.Is(err, command.ErrInvalidInput) {
		t.Errorf("%s error = %v, want an invalid input naming %s", op, err, field)
	}
}

// On update, coordinates left out are unchanged and a cleared pair is removed;
// only the halves that change are recorded (#1604).
func TestUpdate_PlaceCoordinates(t *testing.T) {
	ctx := context.Background()
	es := memory.NewEventStore()
	rs := memory.NewReadModelStore()
	h := command.NewHandler(es, rs)

	created, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada"})
	if err != nil {
		t.Fatalf("CreatePerson: %v", err)
	}
	version := created.Version
	update := func(in command.UpdatePersonInput) {
		t.Helper()
		in.ID, in.Version = created.ID, version
		res, err := h.UpdatePerson(ctx, in)
		if err != nil {
			t.Fatalf("UpdatePerson: %v", err)
		}
		version = res.Version
	}
	get := func() (string, string, string, string) {
		p, _ := rs.GetPerson(ctx, domain.MainBranchID, created.ID)
		return derefOr(p.BirthPlaceLat), derefOr(p.BirthPlaceLong), derefOr(p.DeathPlaceLat), derefOr(p.DeathPlaceLong)
	}

	update(command.UpdatePersonInput{BirthPlaceCoordinates: pair("N42.3601", "W71.0589"), DeathPlaceCoordinates: pair("N1", "E2")})
	if bl, bo, dl, do := get(); bl != "N42.3601" || bo != "W71.0589" || dl != "N1" || do != "E2" {
		t.Fatalf("after set = %s %s %s %s", bl, bo, dl, do)
	}

	update(command.UpdatePersonInput{Notes: command.Set("unrelated")})
	if bl, bo, _, _ := get(); bl != "N42.3601" || bo != "W71.0589" {
		t.Errorf("omitted pair changed to %s %s", bl, bo)
	}

	// Changing one half records only that half.
	before := version
	update(command.UpdatePersonInput{BirthPlaceCoordinates: pair("N42.3601", "W70")})
	events, _ := es.ReadStream(ctx, created.ID)
	last := events[len(events)-1]
	decoded, err := last.DecodeEvent()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	changes := decoded.(domain.PersonUpdated).Changes
	if version != before+1 || len(changes) != 1 || changes["birth_place_long"] != "W70" {
		t.Errorf("changes = %v, want only birth_place_long", changes)
	}

	clearPair := command.CoordinatesPatch{Latitude: command.Clear[string](), Longitude: command.Clear[string]()}
	update(command.UpdatePersonInput{BirthPlaceCoordinates: clearPair})
	if bl, bo, dl, do := get(); bl != "<nil>" || bo != "<nil>" || dl != "N1" || do != "E2" {
		t.Errorf("after clear = %s %s %s %s, want birth cleared and death kept", bl, bo, dl, do)
	}

	// Clearing a pair that is not set is not a change.
	before = version
	update(command.UpdatePersonInput{BirthPlaceCoordinates: clearPair})
	if version != before {
		t.Errorf("clearing an unset pair moved the version %d -> %d", before, version)
	}

	// Family: set, keep, clear.
	fam, err := h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &created.ID})
	if err != nil {
		t.Fatalf("CreateFamily: %v", err)
	}
	res, err := h.UpdateFamily(ctx, command.UpdateFamilyInput{ID: fam.ID, Version: fam.Version, MarriagePlaceCoordinates: pair("N39.7817", "W89.6501")})
	if err != nil {
		t.Fatalf("UpdateFamily set: %v", err)
	}
	res, err = h.UpdateFamily(ctx, command.UpdateFamilyInput{ID: fam.ID, Version: res.Version, MarriagePlace: command.Set("Springfield")})
	if err != nil {
		t.Fatalf("UpdateFamily place: %v", err)
	}
	f, _ := rs.GetFamily(ctx, domain.MainBranchID, fam.ID)
	if derefOr(f.MarriagePlaceLat) != "N39.7817" || derefOr(f.MarriagePlaceLong) != "W89.6501" {
		t.Errorf("family coordinates = %s %s", derefOr(f.MarriagePlaceLat), derefOr(f.MarriagePlaceLong))
	}
	if _, err := h.UpdateFamily(ctx, command.UpdateFamilyInput{ID: fam.ID, Version: res.Version, MarriagePlaceCoordinates: clearPair}); err != nil {
		t.Fatalf("UpdateFamily clear: %v", err)
	}
	f, _ = rs.GetFamily(ctx, domain.MainBranchID, fam.ID)
	if f.MarriagePlaceLat != nil || f.MarriagePlaceLong != nil {
		t.Errorf("family coordinates after clear = %s %s", derefOr(f.MarriagePlaceLat), derefOr(f.MarriagePlaceLong))
	}
}

// A rollback restores the coordinates a record was created with, and clears
// ones set after its target version (#1604).
func TestRollback_PlaceCoordinates(t *testing.T) {
	ctx := context.Background()
	rs := memory.NewReadModelStore()
	h := command.NewHandler(memory.NewEventStore(), rs)

	created, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", BirthPlaceCoordinates: pair("N1", "E1")})
	if err != nil {
		t.Fatalf("CreatePerson: %v", err)
	}
	if _, err := h.UpdatePerson(ctx, command.UpdatePersonInput{ID: created.ID, Version: created.Version,
		BirthPlaceCoordinates: pair("N2", "E2"), DeathPlaceCoordinates: pair("N3", "E3")}); err != nil {
		t.Fatalf("UpdatePerson: %v", err)
	}
	if _, err := h.RollbackPerson(ctx, created.ID, 1); err != nil {
		t.Fatalf("RollbackPerson: %v", err)
	}
	p, _ := rs.GetPerson(ctx, domain.MainBranchID, created.ID)
	if derefOr(p.BirthPlaceLat) != "N1" || derefOr(p.BirthPlaceLong) != "E1" || p.DeathPlaceLat != nil || p.DeathPlaceLong != nil {
		t.Errorf("after rollback = %s %s %s %s, want the created birth pair and no death pair",
			derefOr(p.BirthPlaceLat), derefOr(p.BirthPlaceLong), derefOr(p.DeathPlaceLat), derefOr(p.DeathPlaceLong))
	}

	fam, err := h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &created.ID, MarriagePlaceCoordinates: pair("N4", "E4")})
	if err != nil {
		t.Fatalf("CreateFamily: %v", err)
	}
	if _, err := h.UpdateFamily(ctx, command.UpdateFamilyInput{ID: fam.ID, Version: fam.Version, MarriagePlaceCoordinates: pair("N5", "E5")}); err != nil {
		t.Fatalf("UpdateFamily: %v", err)
	}
	if _, err := h.RollbackFamily(ctx, fam.ID, 1); err != nil {
		t.Fatalf("RollbackFamily: %v", err)
	}
	if f, _ := rs.GetFamily(ctx, domain.MainBranchID, fam.ID); derefOr(f.MarriagePlaceLat) != "N4" || derefOr(f.MarriagePlaceLong) != "E4" {
		t.Errorf("family after rollback = %s %s, want N4 E4", derefOr(f.MarriagePlaceLat), derefOr(f.MarriagePlaceLong))
	}
}
