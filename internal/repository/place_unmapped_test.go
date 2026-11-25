package repository

import (
	"reflect"
	"testing"
)

// TestUnmappedPlaces is the no-coordinates worklist (ADR-009 §9): one entry
// per resolved place with records lacking effective coordinates, suggesting
// the pairs other records there carry.
func TestUnmappedPlaces(t *testing.T) {
	use := func(text string, count int, pair ...string) PlaceUse {
		u := PlaceUse{Key: PlaceKey(text), Text: text, Count: count}
		if len(pair) == 2 {
			u.Latitude, u.Longitude = &pair[0], &pair[1]
		}
		return u
	}
	entry := func(key, name string, count int, suggestions ...CoordinateSuggestion) UnmappedPlace {
		return UnmappedPlace{ID: PlaceID(key), Key: key, Name: name, Spelling: name, Count: count, Suggestions: suggestions}
	}
	mapped := []PlaceFactsReadModel{{Key: "salem,ma", Latitude: strPtr("N42.5"), Longitude: strPtr("W70.9")}}

	tests := []struct {
		name    string
		uses    []PlaceUse
		facts   []PlaceFactsReadModel
		aliases PlaceAliases
		want    []UnmappedPlace
	}{
		{name: "no uses", want: nil},
		{
			name: "spellings with one key are one entry and a comma matters",
			uses: []PlaceUse{
				use("Boston, MA", 2), use(" boston, ma ", 1), use("Boston,MA", 1, "N42.36", "W71.06"),
				use("Boston MA", 1),
			},
			want: []UnmappedPlace{
				entry("boston,ma", "Boston, MA", 3, CoordinateSuggestion{"N42.36", "W71.06", 1}),
				entry("boston ma", "Boston MA", 1),
			},
		},
		{
			name: "suggestions are distinct pairs, most used first",
			uses: []PlaceUse{
				use("Kiel", 1), use("Kiel", 1, "N54.3", "E10.1"), use("KIEL", 2, "N54.32", "E10.13"),
				use("kiel", 1, "N54.3", "E10.1"), use("kiel", 1, "N54.2", "E10.1"),
			},
			want: []UnmappedPlace{entry("kiel", "Kiel", 1,
				CoordinateSuggestion{"N54.3", "E10.1", 2}, CoordinateSuggestion{"N54.32", "E10.13", 2},
				CoordinateSuggestion{"N54.2", "E10.1", 1})},
		},
		{
			name:  "a place's pair maps its records, and a half-set own pair counts as none",
			uses:  []PlaceUse{use("Salem, MA", 3), use("Lynn, MA", 1, "N42.4", "")},
			facts: mapped,
			want:  []UnmappedPlace{entry("lynn,ma", "Lynn, MA", 1)},
		},
		{
			name: "a place whose every record has its own pair is not listed",
			uses: []PlaceUse{use("Lynn, MA", 2, "N42.4", "W70.9")},
			want: nil,
		},
		{
			name:    "an alias joins two keys, and its target's pair maps the absorbed key's records",
			uses:    []PlaceUse{use("Köln", 1), use("Cologne", 1, "N50.9", "E6.9"), use("Old Salem", 2), use("Salem", 1)},
			facts:   []PlaceFactsReadModel{{Key: "salem", Latitude: strPtr("N1"), Longitude: strPtr("E1")}},
			aliases: PlaceAliases{"cologne": "köln", "old salem": "salem"},
			want:    []UnmappedPlace{entry("köln", "Köln", 1, CoordinateSuggestion{"N50.9", "E6.9", 1})},
		},
		{
			name:  "a display name names the entry; the spelling stays recorded; the version is the place's",
			uses:  []PlaceUse{use("  Worcester ,MA", 2), use("worcester, ma", 1)},
			facts: []PlaceFactsReadModel{{Key: "worcester,ma", Name: "Worcester", Version: 2}},
			want:  []UnmappedPlace{{ID: PlaceID("worcester,ma"), Key: "worcester,ma", Name: "Worcester", Spelling: "Worcester, MA", Count: 3, Version: 2}},
		},
		{
			name: "entries sort by count, then name; a parent is its own entry",
			uses: []PlaceUse{use("Ayr", 1), use("Boston, MA", 1), use("MA", 1), use("Zed", 2), use("", 4)},
			want: []UnmappedPlace{
				entry("zed", "Zed", 2), entry("ayr", "Ayr", 1), entry("boston,ma", "Boston, MA", 1), entry("ma", "MA", 1),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UnmappedPlaces(tt.uses, tt.facts, tt.aliases)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("UnmappedPlaces =\n  %+v\nwant\n  %+v", got, tt.want)
			}
		})
	}
}
