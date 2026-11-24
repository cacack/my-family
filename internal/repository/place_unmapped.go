package repository

import (
	"cmp"
	"slices"
	"strings"

	"github.com/google/uuid"
)

// UnmappedPlace is an entry of the no-coordinates worklist (ADR-009 §9): a
// place with Count records that have no effective coordinates. Name is the
// place's display name when one is set, otherwise Spelling: the most-used
// spelling among those records (ties broken bytewise), its parts joined with
// ", ". Suggestions are the distinct pairs that other records at the place
// carry as their own.
type UnmappedPlace struct {
	ID          uuid.UUID
	Key         string
	Name        string
	Spelling    string
	Count       int
	Suggestions []CoordinateSuggestion
}

// CoordinateSuggestion is a pair, as recorded, that Count records at a place
// carry as their own.
type CoordinateSuggestion struct {
	Latitude  string
	Longitude string
	Count     int
}

// UnmappedPlaces builds the no-coordinates worklist from the uses of the
// records a principal may see, with their own pairs, and the facts and
// aliases of places. A use belongs to the place its key resolves to, so two
// texts are one place when they have one PlaceKey or the user joined them
// with an alias, and never by any looser match. A use without effective
// coordinates (ResolveCoordinates) counts toward its place's entry; a use
// with a pair of its own suggests that pair. A place's parents are not
// involved: neither their uses nor their pairs count. Entries sort by count,
// highest first, then by name and key bytewise; suggestions by count, highest
// first, then by latitude and longitude bytewise. It is shared by every store
// so the list is the same on each (DB-001).
func UnmappedPlaces(uses []PlaceUse, facts []PlaceFactsReadModel, aliases PlaceAliases) []UnmappedPlace {
	type pair struct{ lat, long string }
	type place struct {
		missing   int
		spellings map[string]int
		pairs     map[pair]int
	}
	factsByKey := PlaceFactsByKey(facts)
	byKey := make(map[string]*place)
	for _, u := range uses {
		key := aliases.Resolve(u.Key)
		if key == "" {
			continue
		}
		p, ok := byKey[key]
		if !ok {
			p = &place{spellings: make(map[string]int), pairs: make(map[pair]int)}
			byKey[key] = p
		}
		eff, mapped := ResolveCoordinates(u.Latitude, u.Longitude, nil, factsByKey[key])
		switch {
		case mapped && !eff.FromPlace:
			p.pairs[pair{eff.Latitude, eff.Longitude}] += u.Count
		case !mapped:
			p.missing += u.Count
			p.spellings[strings.Join(placeParts(u.Text, true), ", ")] += u.Count
		}
	}

	var entries []UnmappedPlace
	for key, p := range byKey {
		if p.missing == 0 {
			continue
		}
		spelling := mostUsed(p.spellings)
		e := UnmappedPlace{ID: PlaceID(key), Key: key, Name: spelling, Spelling: spelling, Count: p.missing}
		if f := factsByKey[key]; f != nil && f.Name != "" {
			e.Name = f.Name
		}
		for pr, n := range p.pairs {
			e.Suggestions = append(e.Suggestions, CoordinateSuggestion{Latitude: pr.lat, Longitude: pr.long, Count: n})
		}
		slices.SortFunc(e.Suggestions, func(a, b CoordinateSuggestion) int {
			return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Latitude, b.Latitude), cmp.Compare(a.Longitude, b.Longitude))
		})
		entries = append(entries, e)
	}
	slices.SortFunc(entries, func(a, b UnmappedPlace) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Name, b.Name), cmp.Compare(a.Key, b.Key))
	})
	return entries
}
