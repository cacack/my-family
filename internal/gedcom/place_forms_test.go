package gedcom_test

import (
	"bytes"
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	gcgedcom "github.com/cacack/gedcom-go/v3/gedcom"
	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// importPlaces imports each file in turn into one in-memory tree and returns
// the handler, the store and the warnings of each import.
func importPlaces(t *testing.T, files ...string) (*command.Handler, *memory.ReadModelStore, [][]string) {
	t.Helper()
	store := memory.NewReadModelStore()
	h := command.NewHandler(memory.NewEventStore(), store)
	var warnings [][]string
	for _, data := range files {
		result, err := h.ImportGedcom(context.Background(), command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(data)})
		if err != nil {
			t.Fatalf("import: %v", err)
		}
		warnings = append(warnings, result.Warnings)
	}
	return h, store, warnings
}

// formWarnings returns the warnings about a place form that was not applied.
func formWarnings(warnings []string) []string {
	return slices.DeleteFunc(slices.Clone(warnings), func(w string) bool { return !strings.Contains(w, "has the form") })
}

func exportStore(t *testing.T, rs repository.ReadModelStore, version gcgedcom.Version) string {
	t.Helper()
	var buf bytes.Buffer
	if _, err := gedcom.NewExporter(rs).ExportWithOptions(context.Background(), &buf, gedcom.ExportOptions{TargetVersion: version}); err != nil {
		t.Fatalf("export: %v", err)
	}
	return buf.String()
}

// Import files a form on the place its text's key names, under the text's
// positional key: the PLAC's own FORM, otherwise HEAD.PLAC.FORM. The first
// form in file order wins; a different one for a layout that has a form, from
// the same file or a stored one, is a warning and is not applied (ADR-009 §8).
func TestImport_PlaceFormsAreFiledByPartLayout(t *testing.T) {
	const head = "0 HEAD\n1 GEDC\n2 VERS 5.5.1\n1 CHAR UTF-8\n1 PLAC\n2 FORM City, County, State, Country\n"
	first := head +
		"0 @I1@ INDI\n1 NAME Ada /L/\n1 BIRT\n2 PLAC Darmstadt,,,Germany\n" +
		"1 DEAT\n2 PLAC Darmstadt, Germany\n3 FORM City, Country\n" +
		"0 @I2@ INDI\n1 NAME Bea /L/\n1 BIRT\n2 PLAC darmstadt, , ,GERMANY\n3 FORM Stadt, Kreis, Land, Staat\n" +
		"1 RESI\n2 PLAC Kiel, Germany\n" +
		"0 TRLR\n"
	second := "0 HEAD\n1 GEDC\n2 VERS 5.5.1\n1 CHAR UTF-8\n" +
		"0 @I1@ INDI\n1 NAME Cy /L/\n1 BIRT\n2 PLAC Darmstadt, Germany\n3 FORM Town, Country\n" +
		"1 DEAT\n2 PLAC Hamburg\n3 FORM City\n0 TRLR\n"
	_, store, warnings := importPlaces(t, first, second)

	darmstadt, kiel, hamburg := repository.PlaceID("darmstadt,germany"), repository.PlaceID("kiel,germany"), repository.PlaceID("hamburg")
	want := map[uuid.UUID][]repository.PlaceFormReadModel{
		darmstadt: {
			{PlaceID: darmstadt, PositionalKey: "darmstadt,,,germany", Form: "City, County, State, Country"},
			{PlaceID: darmstadt, PositionalKey: "darmstadt,germany", Form: "City, Country"},
		},
		kiel:    {{PlaceID: kiel, PositionalKey: "kiel,germany", Form: "City, County, State, Country"}},
		hamburg: {{PlaceID: hamburg, PositionalKey: "hamburg", Form: "City"}},
	}
	got, err := store.ListPlaceForms(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("forms = %+v\nwant %+v", got, want)
	}
	if w := formWarnings(warnings[0]); len(w) != 1 || !strings.Contains(w[0], `Individual @I2@: BIRT place "darmstadt, , ,GERMANY" has the form "Stadt, Kreis, Land, Staat"`) ||
		!strings.Contains(w[0], `"City, County, State, Country"`) {
		t.Errorf("first import's form warnings = %q, want one for @I2@'s birth place", w)
	}
	if w := formWarnings(warnings[1]); len(w) != 1 || !strings.Contains(w[0], `"Town, Country"`) {
		t.Errorf("second import's form warnings = %q, want one for the stored Darmstadt, Germany form", w)
	}
}

// Import adds a PLAC's EXIDs to its place once each, in file order; export
// writes them under every event of the place in a 7.0 file, and a 5.5 export,
// which has no EXID, leaves them out.
func TestRoundTrip_PlaceExternalIDs(t *testing.T) {
	const data = "0 HEAD\n1 GEDC\n2 VERS 7.0\n" +
		"0 @I1@ INDI\n1 NAME Ada /L/\n" +
		"1 BIRT\n2 PLAC Darmstadt, Germany\n3 EXID Q2973\n4 TYPE https://www.wikidata.org/entity\n" +
		"1 DEAT\n2 PLAC darmstadt, germany\n3 EXID DARTADJO49EV\n4 TYPE https://gov.genealogy.net\n3 EXID Q2973\n4 TYPE https://www.wikidata.org/entity\n" +
		"1 RESI\n2 PLAC Darmstadt, Germany\n" +
		"0 TRLR\n"
	_, store, _ := importPlaces(t, data)
	darmstadt := repository.PlaceID("darmstadt,germany")
	ids, err := store.ListPlaceExternalIDs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []repository.PlaceExternalIDReadModel{
		{PlaceID: darmstadt, Sequence: 0, Value: "Q2973", Type: "https://www.wikidata.org/entity"},
		{PlaceID: darmstadt, Sequence: 1, Value: "DARTADJO49EV", Type: "https://gov.genealogy.net"},
	}
	if !reflect.DeepEqual(ids[darmstadt], want) {
		t.Errorf("external IDs = %+v, want %+v", ids[darmstadt], want)
	}

	const exids = "3 EXID Q2973\n4 TYPE https://www.wikidata.org/entity\n3 EXID DARTADJO49EV\n4 TYPE https://gov.genealogy.net\n"
	out := exportStore(t, store, "")
	for _, place := range []string{"2 PLAC Darmstadt, Germany\n", "2 PLAC darmstadt, germany\n"} {
		if !strings.Contains(out, place+exids) {
			t.Errorf("7.0 export lacks the place's EXIDs under %q:\n%s", place, out)
		}
	}
	if got := strings.Count(out, "3 EXID"); got != 6 {
		t.Errorf("7.0 export writes %d place EXIDs, want 6 (two under each of three events):\n%s", got, out)
	}
	if _, again, _ := importPlaces(t, out); !reflect.DeepEqual(mustListExternalIDs(t, again)[darmstadt], want) {
		t.Errorf("re-import external IDs = %+v, want %+v", mustListExternalIDs(t, again)[darmstadt], want)
	}
	if out := exportStore(t, store, gcgedcom.Version55); strings.Contains(out, "EXID") {
		t.Errorf("5.5 export holds an EXID:\n%s", out)
	}
}

func mustListExternalIDs(t *testing.T, rs repository.ReadModelStore) map[uuid.UUID][]repository.PlaceExternalIDReadModel {
	t.Helper()
	ids, err := rs.ListPlaceExternalIDs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

// Export writes a PLAC.FORM under an event only when its place holds a form
// for the event's positional key, so a form is never written against a text
// with a different number of parts (ADR-009 §8).
func TestExport_PlaceFormFollowsThePartLayout(t *testing.T) {
	const data = "0 HEAD\n1 GEDC\n2 VERS 5.5.1\n1 CHAR UTF-8\n" +
		"0 @I1@ INDI\n1 NAME Ada /L/\n" +
		"1 BIRT\n2 PLAC Darmstadt,,,Germany\n3 FORM City, County, State, Country\n" +
		"1 DEAT\n2 PLAC Darmstadt, Germany\n" +
		"0 TRLR\n"
	_, store, _ := importPlaces(t, data)
	out := exportStore(t, store, "")
	if !strings.Contains(out, "2 PLAC Darmstadt,,,Germany\n3 FORM City, County, State, Country\n") {
		t.Errorf("export lacks the form under the four-part text:\n%s", out)
	}
	if !strings.Contains(out, "2 PLAC Darmstadt, Germany\n") || strings.Contains(out, "2 PLAC Darmstadt, Germany\n3 FORM") {
		t.Errorf("export writes a form under the two-part text, or lacks it:\n%s", out)
	}
	if got := strings.Count(out, "3 FORM"); got != 1 {
		t.Errorf("export writes %d forms, want 1:\n%s", got, out)
	}
}

// A record with no pair of its own is exported at its place's pair, read
// through the place's alias, while a record with its own pair keeps it
// (ADR-009 §5, §8).
func TestExport_RecordsInheritTheirPlacesPair(t *testing.T) {
	const data = "0 HEAD\n1 GEDC\n2 VERS 5.5.1\n1 CHAR UTF-8\n" +
		"0 @I1@ INDI\n1 NAME Ada /L/\n" +
		"1 BIRT\n2 PLAC Köln, Germany\n" +
		"1 DEAT\n2 PLAC Cologne, Germany\n" +
		"1 BURI\n2 PLAC Cologne, Germany\n3 MAP\n4 LATI N50.9\n4 LONG E6.9\n" +
		"0 TRLR\n"
	h, store, _ := importPlaces(t, data)
	ctx := context.Background()
	cologne, koln := repository.PlaceID("cologne,germany"), repository.PlaceID("köln,germany")
	lat, long := "N50.9375", "E6.9603"
	if _, err := h.UpdatePlace(ctx, command.UpdatePlaceInput{ID: cologne, Coordinates: command.CoordinatesPatch{
		Latitude: command.Set(lat), Longitude: command.Set(long)}}); err != nil {
		t.Fatalf("UpdatePlace: %v", err)
	}
	if _, err := h.AliasPlace(ctx, command.AliasPlaceInput{ID: koln, TargetID: cologne}); err != nil {
		t.Fatalf("AliasPlace: %v", err)
	}
	out := exportStore(t, store, "")
	pair := "3 MAP\n4 LATI N50.9375\n4 LONG E6.9603\n"
	for _, want := range []string{"2 PLAC Köln, Germany\n" + pair, "1 DEAT\n2 PLAC Cologne, Germany\n" + pair,
		"1 BURI\n2 PLAC Cologne, Germany\n3 MAP\n4 LATI N50.9\n4 LONG E6.9\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("export lacks\n%s\ngot:\n%s", want, out)
		}
	}
}
