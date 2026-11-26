package gedcom_test

import (
	"context"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/repository"
)

// A MAP holding only a LATI or only a LONG is kept on the record on import
// and written back under its PLAC on export, that half alone, rather than
// dropped (ADR-009 §8, #1862).
func TestRoundTrip_HalfCoordinatePair(t *testing.T) {
	const fam = "0 @I1@ INDI\n1 NAME Ada /L/\n1 FAMS @F1@\n0 @F1@ FAM\n1 WIFE @I1@"
	shapes := []struct {
		name   string
		level0 string
		record string // the record's lines up to and including the PLAC line
		block  string // the exported lines before the MAP
	}{
		{name: "record birth", level0: "0 @I1@ INDI", record: "1 NAME Ada /L/\n1 BIRT\n2 PLAC London\n", block: "1 BIRT\n2 PLAC London\n"},
		{name: "record death", level0: "0 @I1@ INDI", record: "1 NAME Ada /L/\n1 DEAT\n2 DATE 1852\n2 PLAC London\n", block: "2 PLAC London\n"},
		{name: "life event", level0: "0 @I1@ INDI", record: "1 NAME Ada /L/\n1 CHR\n2 DATE 1815\n2 PLAC London\n", block: "2 PLAC London\n"},
		{name: "attribute", level0: "0 @I1@ INDI", record: "1 NAME Ada /L/\n1 OCCU Mathematician\n2 PLAC London\n", block: "1 OCCU Mathematician\n2 PLAC London\n"},
		{name: "record marriage", level0: fam, record: "1 MARR\n2 PLAC London\n", block: "1 MARR\n2 PLAC London\n"},
		{name: "family event", level0: fam, record: "1 DIV\n2 DATE 1860\n2 PLAC London\n", block: "2 PLAC London\n"},
		{name: "nameless record birth", level0: "0 @I1@ INDI", record: "1 NAME Ada /L/\n1 BIRT\n2 PLAC\n", block: "1 BIRT\n2 PLAC\n"},
	}
	for _, half := range []string{"4 LATI N51.5074\n", "4 LONG W0.1278\n"} {
		for _, s := range shapes {
			t.Run(s.name+" "+half[2:6], func(t *testing.T) {
				data := "0 HEAD\n1 GEDC\n2 VERS 5.5.1\n1 CHAR UTF-8\n" + s.level0 + "\n" + s.record + "3 MAP\n" + half + "0 TRLR\n"
				first := importAndExport(t, data)
				if want := "\n" + s.block + "3 MAP\n" + half; !strings.Contains(first, want) {
					t.Errorf("export lacks\n%s\ngot:\n%s", want, first)
				}
				if second := importAndExport(t, first); second != first {
					t.Errorf("re-export differs:\n%s\nfirst:\n%s", second, first)
				}
			})
		}
	}
}

// A record's own half pair is written in place of its place's pair: it is
// the record's own coordinate, and filling the other half from the place
// would write a pair nobody recorded (ADR-009 §8, #1862).
func TestExport_HalfPairNotFilledFromPlace(t *testing.T) {
	const data = "0 HEAD\n1 GEDC\n2 VERS 5.5.1\n1 CHAR UTF-8\n" +
		"0 @I1@ INDI\n1 NAME Ada /L/\n" +
		"1 BIRT\n2 PLAC Cologne, Germany\n3 MAP\n4 LATI N50.9\n" +
		"1 DEAT\n2 PLAC Cologne, Germany\n" +
		"0 TRLR\n"
	h, store, _ := importPlaces(t, data)
	if _, err := h.UpdatePlace(context.Background(), command.UpdatePlaceInput{ID: repository.PlaceID("cologne,germany"),
		Coordinates: command.CoordinatesPatch{Latitude: command.Set("N50.9375"), Longitude: command.Set("E6.9603")}}); err != nil {
		t.Fatalf("UpdatePlace: %v", err)
	}
	out := exportStore(t, store, "")
	for _, want := range []string{
		"1 BIRT\n2 PLAC Cologne, Germany\n3 MAP\n4 LATI N50.9\n1 DEAT\n",
		"1 DEAT\n2 PLAC Cologne, Germany\n3 MAP\n4 LATI N50.9375\n4 LONG E6.9603\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("export lacks\n%s\ngot:\n%s", want, out)
		}
	}
}
