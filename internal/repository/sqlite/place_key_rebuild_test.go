package sqlite_test

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

// placeKeyGedcom records a place on each of the five place-bearing rows: a
// birth and death (persons), a burial (life_events), an occupation
// (attributes) and a marriage (families).
const placeKeyGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
1 BIRT
2 PLAC Boston, MA
1 DEAT
2 PLAC  Darmstadt,,,Germany
1 BURI
2 PLAC Oak Hill , BOSTON
1 OCCU Farmer
2 PLAC Kiel
1 FAMS @F1@
0 @I2@ INDI
1 NAME Ada /Doe/
1 FAMS @F1@
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 MARR
2 PLAC ΣΟΣ, Greece
0 TRLR
`

// placeKeyQueries read every key column beside its text (#1733).
var placeKeyQueries = []string{
	`SELECT birth_place, birth_place_key FROM persons WHERE birth_place IS NOT NULL AND birth_place != ''`,
	`SELECT death_place, death_place_key FROM persons WHERE death_place IS NOT NULL AND death_place != ''`,
	`SELECT marriage_place, marriage_place_key FROM families WHERE marriage_place IS NOT NULL AND marriage_place != ''`,
	`SELECT place, place_key FROM life_events WHERE place IS NOT NULL AND place != ''`,
	`SELECT place, place_key FROM attributes WHERE place IS NOT NULL AND place != ''`,
}

// TestRebuild_FillsPlaceKeys proves `myfamily rebuild` fills every place key
// column from the events already in the log, appending none (ADR-009 §7): the
// columns are emptied, as a database from before them has them, and the
// rebuild fills each with repository.PlaceKey of the text beside it.
func TestRebuild_FillsPlaceKeys(t *testing.T) {
	ctx := context.Background()
	db, cleanup := newTempDB(t)
	defer cleanup()
	read, err := sqlite.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("read model store: %v", err)
	}
	events, err := sqlite.NewEventStore(db)
	if err != nil {
		t.Fatalf("event store: %v", err)
	}
	branches, err := sqlite.NewBranchStore(db)
	if err != nil {
		t.Fatalf("branch store: %v", err)
	}
	snapshots, err := sqlite.NewSnapshotStore(db)
	if err != nil {
		t.Fatalf("snapshot store: %v", err)
	}
	h := command.NewHandlerWithBranches(events, read, branches, snapshots)
	if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(placeKeyGedcom)}); err != nil {
		t.Fatalf("import: %v", err)
	}
	for _, q := range []string{
		`UPDATE persons SET birth_place_key = NULL, death_place_key = NULL`,
		`UPDATE families SET marriage_place_key = NULL`,
		`UPDATE life_events SET place_key = NULL`,
		`UPDATE attributes SET place_key = NULL`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	logged, err := events.ReadAll(ctx, 0, 10000)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}

	if _, err := repository.RebuildReadModel(ctx, events, read, branches, snapshots); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	after, err := events.ReadAll(ctx, 0, 10000)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if len(after) != len(logged) {
		t.Errorf("rebuild appended %d events, want none", len(after)-len(logged))
	}
	for _, q := range placeKeyQueries {
		assertPlaceKeys(ctx, t, db, q)
	}
}

// assertPlaceKeys checks that q returns at least one (text, key) row and that
// each key is repository.PlaceKey of its text.
func assertPlaceKeys(ctx context.Context, t *testing.T, db *sql.DB, q string) {
	t.Helper()
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var text string
		var key sql.NullString
		if err := rows.Scan(&text, &key); err != nil {
			t.Fatalf("scan: %v", err)
		}
		n++
		if want := repository.PlaceKey(text); !key.Valid || key.String != want {
			t.Errorf("%s: key of %q = %+v, want %q", q, text, key, want)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	if n == 0 {
		t.Errorf("%s: no rows; the fixture must record this place", q)
	}
}

// TestRebuild_PlaceFacts shows that a rebuild drops and refills the places
// table from the place streams (ADR-009 §7): rebuilding twice gives the same
// rows as before, and the places table is reset with the read model.
func TestRebuild_PlaceFacts(t *testing.T) {
	ctx := context.Background()
	db, cleanup := newTempDB(t)
	defer cleanup()
	read, err := sqlite.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("read model store: %v", err)
	}
	events, err := sqlite.NewEventStore(db)
	if err != nil {
		t.Fatalf("event store: %v", err)
	}
	branches, err := sqlite.NewBranchStore(db)
	if err != nil {
		t.Fatalf("branch store: %v", err)
	}
	snapshots, err := sqlite.NewSnapshotStore(db)
	if err != nil {
		t.Fatalf("snapshot store: %v", err)
	}
	h := command.NewHandlerWithBranches(events, read, branches, snapshots)
	if _, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", BirthPlace: "Boston, MA"}); err != nil {
		t.Fatalf("create person: %v", err)
	}
	if _, err := h.UpdatePlace(ctx, command.UpdatePlaceInput{ID: repository.PlaceID("boston,ma"), Name: command.Set("Boston"),
		Coordinates: command.CoordinatesPatch{Latitude: command.Set("N42.3601"), Longitude: command.Set("W71.0589")}}); err != nil {
		t.Fatalf("update place: %v", err)
	}
	before, err := read.ListPlaceFacts(ctx, domain.MainBranchID)
	if err != nil || len(before) != 1 {
		t.Fatalf("places before rebuild = %+v, %v; want one row", before, err)
	}
	for i := range 2 {
		if _, err := repository.RebuildReadModel(ctx, events, read, branches, snapshots); err != nil {
			t.Fatalf("rebuild %d: %v", i+1, err)
		}
		after, err := read.ListPlaceFacts(ctx, domain.MainBranchID)
		if err != nil {
			t.Fatalf("list after rebuild %d: %v", i+1, err)
		}
		if !reflect.DeepEqual(after, before) {
			t.Errorf("places after rebuild %d =\n  %+v\nwant\n  %+v", i+1, after, before)
		}
	}
}
