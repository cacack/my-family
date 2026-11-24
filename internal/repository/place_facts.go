package repository

import (
	"cmp"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

// PlaceFactsReadModel is what PlaceUpdated events have stated about a place
// (ADR-009 §2): a row of the places table. It is MAIN-ONLY (PL-005): the table
// has no branch_id, and a branch reads main's facts. Name is "" when no
// display name is set; Latitude and Longitude are both set or both nil, with
// Origin the pair's origin.
type PlaceFactsReadModel struct {
	ID        uuid.UUID
	Key       string
	Name      string
	Latitude  *string
	Longitude *string
	Origin    *domain.CoordinateOrigin
	Version   int64
	UpdatedAt time.Time
}

// HasCoordinates reports whether the place has a coordinate pair.
func (f *PlaceFactsReadModel) HasCoordinates() bool {
	return f != nil && pairSet(f.Latitude, f.Longitude)
}

// pairSet reports whether both halves of a pair are set.
func pairSet(lat, long *string) bool {
	return lat != nil && long != nil && *lat != "" && *long != ""
}

// EffectiveCoordinates is the pair a record is read at (ADR-009 §5): its own
// pair when both halves are set, otherwise its place's pair, otherwise none
// (ok false). A half-set own pair counts as unset. A parent place's pair is
// never inherited. The origin is the chosen pair's, read as imported when
// none was stored (domain.EffectiveCoordinateOrigin).
type EffectiveCoordinates struct {
	Latitude  string
	Longitude string
	Origin    domain.CoordinateOrigin
	// FromPlace reports that the pair is the place's, not the record's own.
	FromPlace bool
}

// ResolveCoordinates applies the effective-coordinate rule to a record's own
// pair lat, long with origin, and the facts of its place (nil when it has
// none).
func ResolveCoordinates(lat, long *string, origin *domain.CoordinateOrigin, place *PlaceFactsReadModel) (EffectiveCoordinates, bool) {
	if pairSet(lat, long) {
		return EffectiveCoordinates{Latitude: *lat, Longitude: *long, Origin: *domain.EffectiveCoordinateOrigin(lat, long, origin)}, true
	}
	if place.HasCoordinates() {
		return EffectiveCoordinates{
			Latitude: *place.Latitude, Longitude: *place.Longitude,
			Origin:    *domain.EffectiveCoordinateOrigin(place.Latitude, place.Longitude, place.Origin),
			FromPlace: true,
		}, true
	}
	return EffectiveCoordinates{}, false
}

// PlaceFactsByKey indexes facts by place key.
func PlaceFactsByKey(facts []PlaceFactsReadModel) map[string]*PlaceFactsReadModel {
	byKey := make(map[string]*PlaceFactsReadModel, len(facts))
	for i := range facts {
		byKey[facts[i].Key] = &facts[i]
	}
	return byKey
}

// MapUse is one birth or death place of a person the map may show: the
// recorded text and its key, the record's own pair and origin, and the facts
// of its place (nil when it has none).
type MapUse struct {
	Person    MapPerson
	EventType string
	Text      string
	Key       string
	Latitude  *string
	Longitude *string
	Origin    *domain.CoordinateOrigin
	Place     *PlaceFactsReadModel
}

// MapLocations groups the uses that have effective coordinates
// (ResolveCoordinates) by place key, event type and effective pair (ADR-009
// §9): two spellings of one place are one pin, and a record with a pair of its
// own, different from its place's, gets a pin of its own. A location is named
// by its place's display name, or else the spelling most of its uses give
// (ties broken bytewise), and carries the place's ID. A pair the map cannot
// parse is skipped (ParseMapCoordinates). Locations sort by place, event type,
// latitude and longitude; each keeps its persons in the order of uses. It is
// shared by every store so the map is the same on each (DB-001).
func MapLocations(uses []MapUse) []MapLocation {
	type locKey struct {
		key, eventType string
		lat, lon       float64
	}
	type locData struct {
		place     *PlaceFactsReadModel
		spellings map[string]int
		persons   []MapPerson
		origins   []domain.CoordinateSource
	}
	agg := make(map[locKey]*locData)
	var order []locKey
	for _, u := range uses {
		eff, ok := ResolveCoordinates(u.Latitude, u.Longitude, u.Origin, u.Place)
		if !ok {
			continue
		}
		lat, lon, ok := ParseMapCoordinates(u.Person.ID, u.EventType, eff.Latitude, eff.Longitude)
		if !ok {
			continue
		}
		k := locKey{key: u.Key, eventType: u.EventType, lat: lat, lon: lon}
		d, ok := agg[k]
		if !ok {
			d = &locData{place: u.Place, spellings: make(map[string]int)}
			agg[k] = d
			order = append(order, k)
		}
		d.spellings[u.Text]++
		d.persons = append(d.persons, u.Person)
		d.origins = AddMapOrigin(d.origins, &eff.Origin)
	}

	results := make([]MapLocation, 0, len(order))
	for _, k := range order {
		d := agg[k]
		name := mostUsed(d.spellings)
		if d.place != nil && d.place.Name != "" {
			name = d.place.Name
		}
		var id uuid.UUID
		if k.key != "" {
			id = PlaceID(k.key)
		}
		results = append(results, MapLocation{
			PlaceID:   id,
			Place:     name,
			Latitude:  k.lat,
			Longitude: k.lon,
			EventType: k.eventType,
			Count:     len(d.persons),
			Persons:   d.persons,
			Origins:   d.origins,
		})
	}
	slices.SortStableFunc(results, func(a, b MapLocation) int {
		return cmp.Or(cmp.Compare(a.Place, b.Place), cmp.Compare(a.EventType, b.EventType),
			cmp.Compare(a.Latitude, b.Latitude), cmp.Compare(a.Longitude, b.Longitude))
	})
	return results
}
