package command_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
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
		BirthPlace:            "Boston",
		DeathPlace:            "Sydney",
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

	fam, err := h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &created.ID, MarriagePlace: "Springfield", MarriagePlaceCoordinates: pair("N39.7817", "W89.6501")})
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

			_, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", BirthPlace: "Boston", BirthPlaceCoordinates: tt.input})
			assertCoordinateError(t, "CreatePerson", err, tt.wantField)
			if persons, total, _ := rs.ListPersons(ctx, repository.ListOptions{Limit: 10}); total != 0 {
				t.Errorf("CreatePerson stored %d persons, want none", len(persons))
			}

			created, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", BirthPlace: "Boston", BirthPlaceCoordinates: pair("N1", "E1")})
			if err != nil {
				t.Fatalf("CreatePerson: %v", err)
			}
			_, err = h.UpdatePerson(ctx, command.UpdatePersonInput{ID: created.ID, Version: created.Version, BirthPlaceCoordinates: tt.input})
			assertCoordinateError(t, "UpdatePerson", err, tt.wantField)
			p, _ := rs.GetPerson(ctx, domain.MainBranchID, created.ID)
			if p.Version != created.Version || derefOr(p.BirthPlaceLat) != "N1" || derefOr(p.BirthPlaceLong) != "E1" {
				t.Errorf("UpdatePerson stored a change: version %d, %s %s", p.Version, derefOr(p.BirthPlaceLat), derefOr(p.BirthPlaceLong))
			}

			_, err = h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &created.ID, MarriagePlace: "Springfield", MarriagePlaceCoordinates: tt.input})
			assertCoordinateError(t, "CreateFamily", err, "marriage"+tt.wantField[len("birth"):])
			fam, err := h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &created.ID, MarriagePlace: "Springfield"})
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

	created, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", BirthPlace: "Boston", DeathPlace: "Sydney"})
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

	// Changing one half records the whole pair, so a merge cannot join one
	// side's latitude to the other's longitude.
	before := version
	update(command.UpdatePersonInput{BirthPlaceCoordinates: pair("N42.3601", "W70")})
	events, _ := es.ReadStream(ctx, created.ID)
	last := events[len(events)-1]
	decoded, err := last.DecodeEvent()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	changes := decoded.(domain.PersonUpdated).Changes
	if version != before+1 || len(changes) != 3 || changes["birth_place_lat"] != "N42.3601" || changes["birth_place_long"] != "W70" || changes["birth_place_origin"] == nil {
		t.Errorf("changes = %v, want the whole birth pair and its origin", changes)
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
	fam, err := h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &created.ID, MarriagePlace: "Springfield"})
	if err != nil {
		t.Fatalf("CreateFamily: %v", err)
	}
	res, err := h.UpdateFamily(ctx, command.UpdateFamilyInput{ID: fam.ID, Version: fam.Version, MarriagePlaceCoordinates: pair("N39.7817", "W89.6501")})
	if err != nil {
		t.Fatalf("UpdateFamily set: %v", err)
	}
	res, err = h.UpdateFamily(ctx, command.UpdateFamilyInput{ID: fam.ID, Version: res.Version, MarriagePlace: command.Set("Springfield, IL")})
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

	created, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", BirthPlace: "Boston", DeathPlace: "Sydney", BirthPlaceCoordinates: pair("N1", "E1")})
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

	fam, err := h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &created.ID, MarriagePlace: "Springfield", MarriagePlaceCoordinates: pair("N4", "E4")})
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

// Coordinates are stored in canonical GEDCOM form: trimmed, with an
// upper-case direction letter (#1604).
func TestPlaceCoordinates_Canonical(t *testing.T) {
	ctx := context.Background()
	rs := memory.NewReadModelStore()
	h := command.NewHandler(memory.NewEventStore(), rs)

	created, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", BirthPlace: "Boston", BirthPlaceCoordinates: pair(" n42.3601\n", "w71.0589 ")})
	if err != nil {
		t.Fatalf("CreatePerson: %v", err)
	}
	p, _ := rs.GetPerson(ctx, domain.MainBranchID, created.ID)
	if derefOr(p.BirthPlaceLat) != "N42.3601" || derefOr(p.BirthPlaceLong) != "W71.0589" {
		t.Errorf("created coordinates = %q %q, want N42.3601 W71.0589", derefOr(p.BirthPlaceLat), derefOr(p.BirthPlaceLong))
	}

	// The same point typed another way is not a change.
	res, err := h.UpdatePerson(ctx, command.UpdatePersonInput{ID: created.ID, Version: created.Version, BirthPlaceCoordinates: pair("n42.3601", "w71.0589")})
	if err != nil || res.Version != created.Version {
		t.Errorf("UpdatePerson = %+v, %v; want no change", res, err)
	}
}

// Coordinates need a place name, as export and the map key them by it; a
// place set in the same update counts (#1604).
func TestPlaceCoordinates_NeedAPlace(t *testing.T) {
	ctx := context.Background()
	rs := memory.NewReadModelStore()
	h := command.NewHandler(memory.NewEventStore(), rs)

	_, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", BirthPlaceCoordinates: pair("N1", "E1")})
	assertCoordinateError(t, "CreatePerson", err, "birth_place")
	// Text whose parts are all empty names no place, so the map would drop
	// its pair (#1807).
	_, err = h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", BirthPlace: ", ,", BirthPlaceCoordinates: pair("N1", "E1")})
	assertCoordinateError(t, "CreatePerson under a nameless text", err, "birth_place")

	created, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", DeathPlace: "Sydney"})
	if err != nil {
		t.Fatalf("CreatePerson: %v", err)
	}
	_, err = h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &created.ID, MarriagePlaceCoordinates: pair("N1", "E1")})
	assertCoordinateError(t, "CreateFamily", err, "marriage_place")
	_, err = h.UpdatePerson(ctx, command.UpdatePersonInput{ID: created.ID, Version: created.Version, BirthPlaceCoordinates: pair("N1", "E1")})
	assertCoordinateError(t, "UpdatePerson", err, "birth_place")
	_, err = h.UpdatePerson(ctx, command.UpdatePersonInput{ID: created.ID, Version: created.Version, DeathPlace: command.Clear[string](), DeathPlaceCoordinates: pair("N1", "E1")})
	assertCoordinateError(t, "UpdatePerson clearing the place", err, "death_place")
	if _, err := h.UpdatePerson(ctx, command.UpdatePersonInput{ID: created.ID, Version: created.Version,
		BirthPlace: command.Set("Boston"), BirthPlaceCoordinates: pair("N1", "E1")}); err != nil {
		t.Errorf("UpdatePerson with a place: %v", err)
	}

	fam, err := h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &created.ID})
	if err != nil {
		t.Fatalf("CreateFamily: %v", err)
	}
	_, err = h.UpdateFamily(ctx, command.UpdateFamilyInput{ID: fam.ID, Version: fam.Version, MarriagePlaceCoordinates: pair("N1", "E1")})
	assertCoordinateError(t, "UpdateFamily", err, "marriage_place")
	if _, err := h.UpdateFamily(ctx, command.UpdateFamilyInput{ID: fam.ID, Version: fam.Version,
		MarriagePlace: command.Set("Springfield"), MarriagePlaceCoordinates: pair("N1", "E1")}); err != nil {
		t.Errorf("UpdateFamily with a place: %v", err)
	}
}

// Clearing a place name, by null or "", also clears its pair, so no stored
// pair is left without a place (#1798). A pair stated beside the clear is
// still judged as given.
func TestPlaceCoordinates_ClearedWithPlace(t *testing.T) {
	ctx := context.Background()
	rs := memory.NewReadModelStore()
	h := command.NewHandler(memory.NewEventStore(), rs)

	created, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada",
		BirthPlace: "Boston", BirthPlaceCoordinates: pair("N42.3601", "W71.0589"),
		DeathPlace: "Sydney", DeathPlaceCoordinates: pair("S33.8688", "E151.2093")})
	if err != nil {
		t.Fatalf("CreatePerson: %v", err)
	}
	_, err = h.UpdatePerson(ctx, command.UpdatePersonInput{ID: created.ID, Version: created.Version,
		BirthPlace: command.Clear[string](), BirthPlaceCoordinates: pair("N1", "E1")})
	assertCoordinateError(t, "UpdatePerson setting a pair beside a cleared place", err, "birth_place")

	res, err := h.UpdatePerson(ctx, command.UpdatePersonInput{ID: created.ID, Version: created.Version,
		BirthPlace: command.Clear[string](), DeathPlace: command.Set("")})
	if err != nil {
		t.Fatalf("UpdatePerson clearing the places: %v", err)
	}
	p, _ := rs.GetPerson(ctx, domain.MainBranchID, created.ID)
	if p.BirthPlaceLat != nil || p.BirthPlaceLong != nil || p.BirthPlaceOrigin != nil ||
		p.DeathPlaceLat != nil || p.DeathPlaceLong != nil || p.DeathPlaceOrigin != nil {
		t.Errorf("pairs after clearing the places = %s %s %s %s, want none",
			derefOr(p.BirthPlaceLat), derefOr(p.BirthPlaceLong), derefOr(p.DeathPlaceLat), derefOr(p.DeathPlaceLong))
	}
	// With no pair left, a later update records nothing about one.
	next, err := h.UpdatePerson(ctx, command.UpdatePersonInput{ID: created.ID, Version: res.Version, BirthPlace: command.Set("")})
	if err != nil || next.Version != res.Version {
		t.Errorf("repeat clear = %v, %v; want no change at version %d", next, err, res.Version)
	}

	fam, err := h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &created.ID,
		MarriagePlace: "Springfield", MarriagePlaceCoordinates: pair("N39.7817", "W89.6501")})
	if err != nil {
		t.Fatalf("CreateFamily: %v", err)
	}
	if _, err := h.UpdateFamily(ctx, command.UpdateFamilyInput{ID: fam.ID, Version: fam.Version, MarriagePlace: command.Set("")}); err != nil {
		t.Fatalf("UpdateFamily clearing the place: %v", err)
	}
	f, _ := rs.GetFamily(ctx, domain.MainBranchID, fam.ID)
	if f.MarriagePlace != "" || f.MarriagePlaceLat != nil || f.MarriagePlaceLong != nil || f.MarriagePlaceOrigin != nil {
		t.Errorf("family after clearing the place = %q %s %s, want no place or pair",
			f.MarriagePlace, derefOr(f.MarriagePlaceLat), derefOr(f.MarriagePlaceLong))
	}
}

// A pair an import stored without a place name is kept by an update that
// leaves the place empty: only clearing a name clears its pair (#1798).
func TestPlaceCoordinates_NamelessPairKept(t *testing.T) {
	ctx := context.Background()
	rs := memory.NewReadModelStore()
	h := command.NewHandler(memory.NewEventStore(), rs)
	ged := "0 HEAD\n1 GEDC\n2 VERS 5.5.1\n1 CHAR UTF-8\n0 @I1@ INDI\n1 NAME Ada /Lovelace/\n" +
		"1 BIRT\n2 DATE 1815\n2 PLAC\n3 MAP\n4 LATI N51.5074\n4 LONG W0.1278\n0 TRLR\n"
	if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", FileSize: int64(len(ged)), Reader: strings.NewReader(ged)}); err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}
	persons, _, _ := rs.ListPersons(ctx, repository.ListOptions{Limit: 10})
	if len(persons) != 1 || persons[0].BirthPlace != "" || persons[0].BirthPlaceLat == nil {
		t.Fatalf("imported persons = %+v, want one with a nameless birth pair", persons)
	}
	p := persons[0]
	res, err := h.UpdatePerson(ctx, command.UpdatePersonInput{ID: p.ID, Version: p.Version, BirthPlace: command.Set("")})
	if err != nil || res.Version != p.Version {
		t.Errorf("update leaving the place empty = %v, %v; want no change at version %d", res, err, p.Version)
	}
	if _, err := h.UpdatePerson(ctx, command.UpdatePersonInput{ID: p.ID, Version: p.Version, Notes: command.Set("note")}); err != nil {
		t.Fatalf("UpdatePerson notes: %v", err)
	}
	got, _ := rs.GetPerson(ctx, domain.MainBranchID, p.ID)
	if derefOr(got.BirthPlaceLat) != "N51.5074" || derefOr(got.BirthPlaceLong) != "W0.1278" {
		t.Errorf("pair after a notes update = %s %s, want N51.5074 W0.1278", derefOr(got.BirthPlaceLat), derefOr(got.BirthPlaceLong))
	}
}

// A pair imported under text that names no place, such as ", ,", is kept by
// an update that leaves that text as it is (#1807).
func TestPlaceCoordinates_NamelessTextPairKept(t *testing.T) {
	ctx := context.Background()
	rs := memory.NewReadModelStore()
	h := command.NewHandler(memory.NewEventStore(), rs)
	ged := "0 HEAD\n1 GEDC\n2 VERS 5.5.1\n1 CHAR UTF-8\n0 @I1@ INDI\n1 NAME Ada /Lovelace/\n" +
		"1 BIRT\n2 DATE 1815\n2 PLAC , ,\n3 MAP\n4 LATI N51.5074\n4 LONG W0.1278\n0 TRLR\n"
	if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", FileSize: int64(len(ged)), Reader: strings.NewReader(ged)}); err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}
	persons, _, _ := rs.ListPersons(ctx, repository.ListOptions{Limit: 10})
	if len(persons) != 1 || persons[0].BirthPlaceLat == nil {
		t.Fatalf("imported persons = %+v, want one with a birth pair", persons)
	}
	p := persons[0]
	if _, err := h.UpdatePerson(ctx, command.UpdatePersonInput{ID: p.ID, Version: p.Version, Notes: command.Set("note")}); err != nil {
		t.Fatalf("UpdatePerson notes: %v", err)
	}
	got, _ := rs.GetPerson(ctx, domain.MainBranchID, p.ID)
	if derefOr(got.BirthPlaceLat) != "N51.5074" || derefOr(got.BirthPlaceLong) != "W0.1278" {
		t.Errorf("pair after a notes update = %s %s, want N51.5074 W0.1278", derefOr(got.BirthPlaceLat), derefOr(got.BirthPlaceLong))
	}
}

// A rollback to before a backfill keeps the imported coordinates it logged,
// which the record already held; a rollback to the backfill restores them
// (#1604).
func TestRollback_BackfilledCoordinates(t *testing.T) {
	ctx := context.Background()
	es := memory.NewEventStore()
	rs := memory.NewReadModelStore()
	h := command.NewHandler(es, rs)

	created, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", BirthPlace: "Boston"})
	if err != nil {
		t.Fatalf("CreatePerson: %v", err)
	}
	lat, long := "N1", "E1"
	backfill := domain.ImportedValuesBackfilled{BaseEvent: domain.NewBaseEvent(), EntityType: domain.BackfillPerson, EntityID: created.ID,
		BirthPlaceLat: &lat, BirthPlaceLong: &long}
	if err := es.Append(ctx, created.ID, "Person", []domain.Event{backfill}, created.Version, repository.MainScope); err != nil {
		t.Fatalf("append backfill: %v", err)
	}
	backfillVersion := created.Version + 1
	if err := repository.NewProjector(rs, nil).Project(ctx, backfill, backfillVersion, domain.MainBranchID); err != nil {
		t.Fatalf("project backfill: %v", err)
	}
	updated, err := h.UpdatePerson(ctx, command.UpdatePersonInput{ID: created.ID, Version: backfillVersion, Notes: command.Set("later")})
	if err != nil {
		t.Fatalf("UpdatePerson: %v", err)
	}
	birth := func() (string, string) {
		p, _ := rs.GetPerson(ctx, domain.MainBranchID, created.ID)
		return derefOr(p.BirthPlaceLat), derefOr(p.BirthPlaceLong)
	}

	if _, err := h.RollbackPerson(ctx, created.ID, created.Version); err != nil {
		t.Fatalf("RollbackPerson to before the backfill: %v", err)
	}
	if bl, bo := birth(); bl != lat || bo != long {
		t.Errorf("after rollback to before the backfill = %s %s, want the imported %s %s kept", bl, bo, lat, long)
	}

	p, _ := rs.GetPerson(ctx, domain.MainBranchID, created.ID)
	if _, err := h.UpdatePerson(ctx, command.UpdatePersonInput{ID: created.ID, Version: p.Version, BirthPlaceCoordinates: pair("N2", "E2")}); err != nil {
		t.Fatalf("UpdatePerson coordinates: %v", err)
	}
	if _, err := h.RollbackPerson(ctx, created.ID, updated.Version); err != nil {
		t.Fatalf("RollbackPerson to after the backfill: %v", err)
	}
	if bl, bo := birth(); bl != lat || bo != long {
		t.Errorf("after rollback to the backfilled state = %s %s, want %s %s", bl, bo, lat, long)
	}

	// A rollback to before the backfill keeps a later pair with its origin
	// (#1563).
	p, _ = rs.GetPerson(ctx, domain.MainBranchID, created.ID)
	copied := &domain.CoordinateOrigin{Source: domain.CoordinateCopied}
	later := pair("N3", "E3")
	later.Origin = copied
	if _, err := h.UpdatePerson(ctx, command.UpdatePersonInput{ID: created.ID, Version: p.Version, BirthPlaceCoordinates: later}); err != nil {
		t.Fatalf("UpdatePerson coordinates with an origin: %v", err)
	}
	if _, err := h.RollbackPerson(ctx, created.ID, created.Version); err != nil {
		t.Fatalf("RollbackPerson to before the backfill again: %v", err)
	}
	if p, _ := rs.GetPerson(ctx, domain.MainBranchID, created.ID); derefOr(p.BirthPlaceLat) != "N3" || !reflect.DeepEqual(p.BirthPlaceOrigin, copied) {
		t.Errorf("after rollback = %s, %v; want the later pair kept with its copied origin", derefOr(p.BirthPlaceLat), p.BirthPlaceOrigin)
	}
}

// A pair given with no origin is manual, one given with an origin keeps it,
// and an update records the origin with the pair, even when only the origin
// changes (#1563).
func TestPlaceCoordinates_Origin(t *testing.T) {
	ctx := context.Background()
	es := memory.NewEventStore()
	rs := memory.NewReadModelStore()
	h := command.NewHandler(es, rs)
	manual := &domain.CoordinateOrigin{Source: domain.CoordinateManual}
	geocoded := &domain.CoordinateOrigin{Source: domain.CoordinateGeocoded, Provider: "Nominatim", Precision: domain.MatchFull}
	withOrigin := func(lat, long string, origin *domain.CoordinateOrigin) command.CoordinatesPatch {
		p := pair(lat, long)
		p.Origin = origin
		return p
	}

	created, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", BirthPlace: "Boston", DeathPlace: "Sydney",
		BirthPlaceCoordinates: pair("N1", "E1"),
		DeathPlaceCoordinates: withOrigin("N2", "E2", &domain.CoordinateOrigin{Source: domain.CoordinateGeocoded, Provider: " Nominatim ", Precision: domain.MatchFull})})
	if err != nil {
		t.Fatalf("CreatePerson: %v", err)
	}
	get := func() *repository.PersonReadModel {
		p, _ := rs.GetPerson(ctx, domain.MainBranchID, created.ID)
		return p
	}
	if p := get(); !reflect.DeepEqual(p.BirthPlaceOrigin, manual) || !reflect.DeepEqual(p.DeathPlaceOrigin, geocoded) {
		t.Errorf("created origins = %v, %v; want manual and the geocoded one, provider trimmed", p.BirthPlaceOrigin, p.DeathPlaceOrigin)
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

	// The same pair again with no origin is not a change: a form that
	// resaves a geocoded pair does not make it manual.
	before := version
	update(command.UpdatePersonInput{DeathPlaceCoordinates: pair("N2", "E2")})
	if version != before || !reflect.DeepEqual(get().DeathPlaceOrigin, geocoded) {
		t.Errorf("resaving the pair: version %d -> %d, origin %v", before, version, get().DeathPlaceOrigin)
	}
	// The same pair with a new origin records the origin with the pair, so a
	// branch merge contests the three together.
	update(command.UpdatePersonInput{DeathPlaceCoordinates: withOrigin("N2", "E2", manual)})
	if version != before+1 || !reflect.DeepEqual(get().DeathPlaceOrigin, manual) {
		t.Errorf("restating the origin: version %d -> %d, origin %v", before, version, get().DeathPlaceOrigin)
	}
	events, _ := es.ReadStream(ctx, created.ID)
	decoded, err := events[len(events)-1].DecodeEvent()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if changes := decoded.(domain.PersonUpdated).Changes; len(changes) != 3 || changes["death_place_lat"] != "N2" || changes["death_place_long"] != "E2" {
		t.Errorf("origin-only changes = %v, want the pair recorded with its origin", changes)
	}
	// A changed pair with no origin is manual.
	update(command.UpdatePersonInput{BirthPlaceCoordinates: withOrigin("N1", "E1", geocoded)})
	update(command.UpdatePersonInput{BirthPlaceCoordinates: pair("N3", "E3")})
	if !reflect.DeepEqual(get().BirthPlaceOrigin, manual) {
		t.Errorf("changed pair origin = %v, want manual", get().BirthPlaceOrigin)
	}
	// A cleared pair clears its origin.
	update(command.UpdatePersonInput{BirthPlaceCoordinates: command.CoordinatesPatch{Latitude: command.Clear[string](), Longitude: command.Clear[string]()}})
	if p := get(); p.BirthPlaceLat != nil || p.BirthPlaceOrigin != nil {
		t.Errorf("after clear = %s, %v; want no pair and no origin", derefOr(p.BirthPlaceLat), p.BirthPlaceOrigin)
	}

	fam, err := h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &created.ID, MarriagePlace: "Springfield",
		MarriagePlaceCoordinates: withOrigin("N4", "E4", &domain.CoordinateOrigin{Source: domain.CoordinateCopied})})
	if err != nil {
		t.Fatalf("CreateFamily: %v", err)
	}
	if f, _ := rs.GetFamily(ctx, domain.MainBranchID, fam.ID); f.MarriagePlaceOrigin == nil || f.MarriagePlaceOrigin.Source != domain.CoordinateCopied {
		t.Errorf("family origin = %v, want copied", f.MarriagePlaceOrigin)
	}
	if _, err := h.UpdateFamily(ctx, command.UpdateFamilyInput{ID: fam.ID, Version: fam.Version, MarriagePlaceCoordinates: pair("N5", "E5")}); err != nil {
		t.Fatalf("UpdateFamily: %v", err)
	}
	if f, _ := rs.GetFamily(ctx, domain.MainBranchID, fam.ID); !reflect.DeepEqual(f.MarriagePlaceOrigin, manual) {
		t.Errorf("family origin after update = %v, want manual", f.MarriagePlaceOrigin)
	}
}

// An origin that is not valid, or given without a set pair, is rejected
// naming the origin field (#1563).
func TestPlaceCoordinates_OriginRejected(t *testing.T) {
	tests := []struct {
		name  string
		input command.CoordinatesPatch
	}{
		{"geocoded without a provider", command.CoordinatesPatch{Latitude: command.Set("N1"), Longitude: command.Set("E1"),
			Origin: &domain.CoordinateOrigin{Source: domain.CoordinateGeocoded, Precision: domain.MatchFull}}},
		{"geocoded with a blank provider", command.CoordinatesPatch{Latitude: command.Set("N1"), Longitude: command.Set("E1"),
			Origin: &domain.CoordinateOrigin{Source: domain.CoordinateGeocoded, Provider: "  ", Precision: domain.MatchFull}}},
		{"geocoded without a precision", command.CoordinatesPatch{Latitude: command.Set("N1"), Longitude: command.Set("E1"),
			Origin: &domain.CoordinateOrigin{Source: domain.CoordinateGeocoded, Provider: "Nominatim"}}},
		{"manual with a provider", command.CoordinatesPatch{Latitude: command.Set("N1"), Longitude: command.Set("E1"),
			Origin: &domain.CoordinateOrigin{Source: domain.CoordinateManual, Provider: "Nominatim"}}},
		{"unknown source", command.CoordinatesPatch{Latitude: command.Set("N1"), Longitude: command.Set("E1"),
			Origin: &domain.CoordinateOrigin{Source: "guessed"}}},
		{"origin without a pair", command.CoordinatesPatch{Origin: &domain.CoordinateOrigin{Source: domain.CoordinateManual}}},
		{"origin beside a cleared pair", command.CoordinatesPatch{Latitude: command.Clear[string](), Longitude: command.Clear[string](),
			Origin: &domain.CoordinateOrigin{Source: domain.CoordinateManual}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			rs := memory.NewReadModelStore()
			h := command.NewHandler(memory.NewEventStore(), rs)
			_, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", BirthPlace: "Boston", BirthPlaceCoordinates: tt.input})
			assertCoordinateError(t, "CreatePerson", err, "birth_place_coordinates_origin")
			created, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", BirthPlace: "Boston", BirthPlaceCoordinates: pair("N1", "E1")})
			if err != nil {
				t.Fatalf("CreatePerson: %v", err)
			}
			_, err = h.UpdatePerson(ctx, command.UpdatePersonInput{ID: created.ID, Version: created.Version, BirthPlaceCoordinates: tt.input})
			assertCoordinateError(t, "UpdatePerson", err, "birth_place_coordinates_origin")
		})
	}
}

// A rollback restores the origin a pair was created with (#1563).
func TestRollback_CoordinateOrigin(t *testing.T) {
	ctx := context.Background()
	rs := memory.NewReadModelStore()
	h := command.NewHandler(memory.NewEventStore(), rs)
	geocoded := &domain.CoordinateOrigin{Source: domain.CoordinateGeocoded, Provider: "Nominatim", Precision: domain.MatchPartial}

	in := pair("N1", "E1")
	in.Origin = geocoded
	created, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", BirthPlace: "Boston", BirthPlaceCoordinates: in})
	if err != nil {
		t.Fatalf("CreatePerson: %v", err)
	}
	if _, err := h.UpdatePerson(ctx, command.UpdatePersonInput{ID: created.ID, Version: created.Version, BirthPlaceCoordinates: pair("N2", "E2")}); err != nil {
		t.Fatalf("UpdatePerson: %v", err)
	}
	if _, err := h.RollbackPerson(ctx, created.ID, 1); err != nil {
		t.Fatalf("RollbackPerson: %v", err)
	}
	p, _ := rs.GetPerson(ctx, domain.MainBranchID, created.ID)
	if derefOr(p.BirthPlaceLat) != "N1" || !reflect.DeepEqual(p.BirthPlaceOrigin, geocoded) {
		t.Errorf("after rollback = %s, %v; want N1 and the geocoded origin", derefOr(p.BirthPlaceLat), p.BirthPlaceOrigin)
	}
}
