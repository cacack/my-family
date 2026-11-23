package repository

import (
	"sort"
	"strings"

	"github.com/google/uuid"
)

// PlaceKey is the matching key of a recorded place text (ADR-009 §1), the only
// place-matching rule in the project (PL-002). The text is split on ",", each
// part is trimmed of white space (strings.TrimSpace), empty parts are dropped,
// each code point is case-folded by foldRune, and the parts are joined with ",".
// So "Boston, MA", " BOSTON ,ma " and "Boston,MA" all have the key "boston,ma",
// and "Darmstadt,,,Germany" is "darmstadt,germany". Text whose parts are all
// empty has the key "", which names no place.
//
// The rule is frozen once place events exist, because place IDs derive from it
// (PL-003): TestPlaceKey pins its vectors and TestPlaceKeyFoldTable its fold of
// every code point.
func PlaceKey(text string) string {
	return strings.Join(foldParts(placeParts(text, true)), ",")
}

// PositionalPlaceKey is PlaceKey without dropping empty parts, so it keeps the
// part layout of a text: "Darmstadt,,,Germany" is "darmstadt,,,germany" and
// "Boston," is "boston,". It identifies nothing; it says which texts share a
// layout, which is what a PLAC.FORM is filed under (ADR-009 §1). Text with no
// PlaceKey has no positional key either.
func PositionalPlaceKey(text string) string {
	if PlaceKey(text) == "" {
		return ""
	}
	return strings.Join(foldParts(placeParts(text, false)), ",")
}

// PlaceUnder reports whether a record whose place has key is at or under the
// place with key place: its key is that key or ends with "," and that key
// (ADR-009 §4, default parents). So "york" is not under "new york,ny", and
// nothing is under "".
func PlaceUnder(key, place string) bool {
	return place != "" && (key == place || strings.HasSuffix(key, ","+place))
}

// placeParts splits text on "," and trims each part of white space, dropping
// the empty ones when dropEmpty is set. The parts keep their recorded case.
func placeParts(text string, dropEmpty bool) []string {
	parts := strings.Split(text, ",")
	kept := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" || !dropEmpty {
			kept = append(kept, p)
		}
	}
	return kept
}

// foldParts case-folds each part in place and returns parts.
func foldParts(parts []string) []string {
	for i, p := range parts {
		parts[i] = strings.Map(foldRune, p)
	}
	return parts
}

// foldRune is step 4 of PlaceKey: the representative of r's unicode.SimpleFold
// orbit, which is its smallest lowercase letter (unicode.IsLower), or its
// smallest code point when the orbit has none. There is no locale and no full
// folding, so "ß" stays "ß" and "İ" (which has no simple folding) stays "İ".
//
// It reads the pinned placeFoldTable, not the unicode package, because Go's
// Unicode tables change between releases (Go 1.27 adds case pairs Go 1.26
// lacks) and a key must not depend on the Go a binary was built with.
func foldRune(r rune) rune {
	i := sort.Search(len(placeFoldTable), func(i int) bool { return placeFoldTable[i][0] >= r })
	if i < len(placeFoldTable) && placeFoldTable[i][0] == r {
		return placeFoldTable[i][1]
	}
	return r
}

// PlaceUse is one recorded spelling of a place and the number of records that
// use it. Text is the text as recorded and Key its stored PlaceKey.
type PlaceUse struct {
	Key   string
	Text  string
	Count int
}

// PlaceLevels builds the place browser's level directly under parent from the
// uses of the records it browses; parent "" is the top level. A level is one
// more part of the key than parent, and a record counts toward the level it is
// under (PlaceUnder), so a level's count is its records and its persons are
// GetPersonsByPlace of its FullName. A level shows the most-used spelling of
// its part (ties broken bytewise), and levels sort by that name bytewise. It is
// shared by every store so the browser is the same on each (DB-001).
func PlaceLevels(uses []PlaceUse, parent string) []PlaceEntry {
	parentKey := PlaceKey(parent)
	if parent != "" && parentKey == "" {
		return nil
	}
	depth := 0
	if parentKey != "" {
		depth = strings.Count(parentKey, ",") + 1
	}

	type level struct {
		spellings   map[string]int
		count       int
		hasChildren bool
	}
	levels := make(map[string]*level)
	for _, u := range uses {
		if u.Key == "" || u.Key == parentKey || parentKey != "" && !PlaceUnder(u.Key, parentKey) {
			continue
		}
		keyParts := strings.Split(u.Key, ",")
		i := len(keyParts) - depth - 1
		levelKey := strings.Join(keyParts[i:], ",")
		name := keyParts[i]
		if textParts := placeParts(u.Text, true); len(textParts) == len(keyParts) {
			name = textParts[i]
		}
		l, ok := levels[levelKey]
		if !ok {
			l = &level{spellings: make(map[string]int)}
			levels[levelKey] = l
		}
		l.spellings[name] += u.Count
		l.count += u.Count
		l.hasChildren = l.hasChildren || i > 0
	}

	entries := make([]PlaceEntry, 0, len(levels))
	for _, l := range levels {
		name := mostUsed(l.spellings)
		fullName := name
		if parent != "" {
			fullName = name + ", " + parent
		}
		entries = append(entries, PlaceEntry{Name: name, FullName: fullName, Count: l.count, HasChildren: l.hasChildren})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries
}

// CemeteryUse is one person buried or cremated at a place recorded as Text,
// whose stored PlaceKey is Key.
type CemeteryUse struct {
	Key     string
	Text    string
	OwnerID uuid.UUID
}

// CemeteryIndex groups burial and cremation uses by place key: one entry per
// key, named by its most-used spelling (ties broken bytewise) and counting its
// distinct owners, sorted by name bytewise. Shared by every store (DB-001).
func CemeteryIndex(uses []CemeteryUse) []CemeteryEntry {
	type cemetery struct {
		spellings map[string]int
		owners    map[uuid.UUID]struct{}
	}
	byKey := make(map[string]*cemetery)
	for _, u := range uses {
		if u.Key == "" {
			continue
		}
		c, ok := byKey[u.Key]
		if !ok {
			c = &cemetery{spellings: make(map[string]int), owners: make(map[uuid.UUID]struct{})}
			byKey[u.Key] = c
		}
		c.spellings[u.Text]++
		c.owners[u.OwnerID] = struct{}{}
	}
	entries := make([]CemeteryEntry, 0, len(byKey))
	for _, c := range byKey {
		entries = append(entries, CemeteryEntry{Place: mostUsed(c.spellings), Count: len(c.owners)})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Place < entries[j].Place })
	return entries
}

// mostUsed returns the spelling with the highest count, the bytewise smallest
// among equals.
func mostUsed(spellings map[string]int) string {
	best, bestCount := "", -1
	for s, n := range spellings {
		if n > bestCount || n == bestCount && s < best {
			best, bestCount = s, n
		}
	}
	return best
}
