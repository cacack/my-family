package repository

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/google/uuid"
)

// treeLines walks tree from the top level and returns one line per place,
// indented by depth: name, full name and count.
func treeLines(t *testing.T, tree *PlaceTree) []string {
	t.Helper()
	var lines []string
	var walk func(parent uuid.UUID, indent string)
	walk = func(parent uuid.UUID, indent string) {
		level, ok := tree.Level(parent)
		if !ok {
			t.Fatalf("Level(%s) not found", parent)
		}
		for _, e := range level.Places {
			lines = append(lines, indent+e.Name+" | "+e.FullName+" | "+strconv.Itoa(e.Count))
			if e.HasChildren {
				walk(e.ID, indent+"  ")
			}
		}
	}
	walk(uuid.Nil, "")
	return lines
}

func TestBuildPlaceTree(t *testing.T) {
	people := []PersonPlaces{
		{BirthKey: "boston,ma", BirthText: "Boston, MA", Count: 2},
		{DeathKey: "boston,ma", DeathText: "boston,ma", Count: 1},
		// Born and died under Hessen: one person at Hessen and Germany.
		{BirthKey: "darmstadt,hessen,germany", BirthText: "Darmstadt, Hessen, Germany",
			DeathKey: "kassel,hesse,germany", DeathText: "Kassel, Hesse, Germany", Count: 1},
		// Born and died at Springfield: one person there.
		{BirthKey: "springfield", BirthText: "Springfield", DeathKey: "springfield", DeathText: "Springfield", Count: 1},
		// Born in Illinois and died in Boston: one person at each.
		{BirthKey: "chicago,illinois", BirthText: "Chicago, Illinois", DeathKey: "boston,ma", DeathText: "Boston, MA", Count: 1},
		{BirthKey: "a,b", BirthText: "x", Count: 1},
		{BirthKey: "", BirthText: " , ", Count: 9},
	}
	facts := []PlaceFactsReadModel{
		// Springfield, a one-part key, is put under Illinois by hand.
		{Key: "springfield", ParentKey: "illinois"},
		// Illinois's display name shows at its level and as its full name.
		{Key: "illinois", Name: "State of Illinois"},
		// An absorbed place's own parent is ignored.
		{Key: "hesse,germany", ParentKey: "boston,ma"},
	}
	aliases := PlaceAliases{"hesse,germany": "hessen,germany"}
	tree := BuildPlaceTree(people, facts, aliases)

	want := []string{
		"Germany | Germany | 1",
		"  Hessen | Hessen, Germany | 1",
		"    Darmstadt | Darmstadt, Hessen, Germany | 1",
		"    Kassel | Kassel, Hesse, Germany | 1",
		"MA | MA | 4",
		"  Boston | Boston, MA | 4",
		"State of Illinois | State of Illinois | 2",
		"  Chicago | Chicago, Illinois | 1",
		"  Springfield | Springfield | 1",
		// A use whose text and key disagree on the number of parts is named
		// by its key.
		"b | b | 1",
		"  a | a, b | 1",
	}
	if got := treeLines(t, tree); !reflect.DeepEqual(got, want) {
		t.Errorf("tree =\n  %q\nwant\n  %q", got, want)
	}

	// The path ends at the parent; an absorbed ID names its target.
	level, ok := tree.Level(PlaceID("hesse,germany"))
	if !ok || len(level.Path) != 2 || level.Path[0].ID != PlaceID("germany") || level.Path[1].ID != PlaceID("hessen,germany") {
		t.Errorf("Level(hesse) = %+v, %v; want the path Germany, Hessen", level, ok)
	}
	if level, ok := tree.Level(PlaceID("nowhere")); ok || len(level.Places) != 0 || len(level.Path) != 0 {
		t.Errorf("Level(nowhere) = %+v, %v; want an empty level", level, ok)
	}

	keys := []struct {
		id   uuid.UUID
		want []string
	}{
		{PlaceID("germany"), []string{"darmstadt,hessen,germany", "kassel,hesse,germany"}},
		{PlaceID("hesse,germany"), []string{"darmstadt,hessen,germany", "kassel,hesse,germany"}},
		{PlaceID("illinois"), []string{"chicago,illinois", "springfield"}},
		{PlaceID("boston,ma"), []string{"boston,ma"}},
		{PlaceID("nowhere"), nil},
		{uuid.Nil, nil},
	}
	for _, tt := range keys {
		if got := tree.KeysUnder(tt.id); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("KeysUnder(%s) = %q, want %q", tt.id, got, tt.want)
		}
	}
}

// A cycle that only an alias stated after a parent can close is broken at
// its smallest key, whatever the order of the people.
func TestBuildPlaceTree_BreaksCycles(t *testing.T) {
	people := []PersonPlaces{
		{BirthKey: "x", BirthText: "X", Count: 1},
		{BirthKey: "y", BirthText: "Y", Count: 1},
		{DeathKey: "berlin,germany", DeathText: "Berlin, Germany", Count: 1},
	}
	facts := []PlaceFactsReadModel{{Key: "x", ParentKey: "y"}, {Key: "y", ParentKey: "x"}}
	// Germany as an alias of Berlin puts Berlin under itself.
	aliases := PlaceAliases{"germany": "berlin,germany"}
	want := []string{
		"Berlin | Berlin, Germany | 1",
		"X | X | 2",
		"  Y | Y | 1",
	}
	for range 5 {
		if got := treeLines(t, BuildPlaceTree(people, facts, aliases)); !reflect.DeepEqual(got, want) {
			t.Fatalf("tree =\n  %q\nwant\n  %q", got, want)
		}
	}
}

func TestPlaceParentClosesCycle(t *testing.T) {
	parents := PlaceParents{"springfield": "illinois"}
	aliases := PlaceAliases{"il": "illinois"}
	tests := []struct {
		name        string
		key, parent string
		want        bool
	}{
		{"a place under itself", "illinois", "illinois", true},
		{"a place under its own alias", "illinois", "il", true},
		{"a default parent under its child", "germany", "darmstadt,germany", true},
		{"an explicit parent under its child", "illinois", "springfield", true},
		{"a parent named by an alias", "springfield", "il", false},
		{"an absorbed place, whose own parent is never read", "il", "springfield", false},
		{"a place under an unrelated place", "springfield", "usa", false},
		{"a place under its default grandparent", "a,b,c", "c", false},
		{"a parent above a cycle", "z", "y", false},
	}
	withCycle := PlaceParents{"springfield": "illinois", "x": "y", "y": "x"}
	for _, tt := range tests {
		p := parents
		if tt.key == "z" {
			p = withCycle
		}
		if got := PlaceParentClosesCycle(p, aliases, tt.key, tt.parent); got != tt.want {
			t.Errorf("%s: PlaceParentClosesCycle(%q, %q) = %v, want %v", tt.name, tt.key, tt.parent, got, tt.want)
		}
	}
	if parents["illinois"] != "" {
		t.Error("PlaceParentClosesCycle changed parents")
	}
}

func TestPlaceAliasClosesParentCycle(t *testing.T) {
	parents := PlaceParents{"springfield": "illinois"}
	tests := []struct {
		name    string
		aliases PlaceAliases
		target  string
		want    bool
	}{
		{"a parent aliased to its child", PlaceAliases{"germany": "berlin,germany"}, "berlin,germany", true},
		{"an explicit parent aliased to its child", PlaceAliases{"illinois": "springfield"}, "springfield", true},
		{"two spellings of one place", PlaceAliases{"hesse,germany": "hessen,germany"}, "hessen,germany", false},
	}
	for _, tt := range tests {
		if got := PlaceAliasClosesParentCycle(parents, tt.aliases, tt.target); got != tt.want {
			t.Errorf("%s: PlaceAliasClosesParentCycle = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestPlaceAliases_KeysOf(t *testing.T) {
	aliases := PlaceAliases{"koln": "cologne", "coeln": "cologne", "x": "y"}
	for _, key := range []string{"cologne", "koln"} {
		if got, want := aliases.KeysOf(key), []string{"coeln", "cologne", "koln"}; !reflect.DeepEqual(got, want) {
			t.Errorf("KeysOf(%q) = %q, want %q", key, got, want)
		}
	}
	if got := aliases.KeysOf("bonn"); !reflect.DeepEqual(got, []string{"bonn"}) {
		t.Errorf("KeysOf(bonn) = %q", got)
	}
}

// TestPlaces_FollowsExplicitParents shows that the list of places walks the
// place browser's hierarchy (ADR-009 §4): a use counts toward an explicit
// parent and every place above it, not toward the default parents its key
// would give, a parent no use names is listed by its key, and every place the
// tree holds is on the list with the same count when each person has one place.
func TestPlaces_FollowsExplicitParents(t *testing.T) {
	people := []PersonPlaces{
		{BirthKey: "springfield,sangamon", BirthText: "Springfield, Sangamon", Count: 2},
		{BirthKey: "chicago,illinois", BirthText: "Chicago, Illinois", Count: 1},
		{DeathKey: "x", DeathText: "X", Count: 1},
		{DeathKey: "y", DeathText: "Y", Count: 1},
	}
	facts := []PlaceFactsReadModel{
		{Key: "springfield,sangamon", ParentKey: "illinois,usa"},
		{Key: "x", ParentKey: "y"}, {Key: "y", ParentKey: "x"},
	}
	var uses []PlaceUse
	for _, p := range people {
		uses = append(uses, PlaceUse{Key: p.BirthKey + p.DeathKey, Text: p.BirthText + p.DeathText, Count: p.Count})
	}
	got := map[string]int{}
	for _, p := range Places(uses, facts, nil, Privacy{HidePrivate: true}) {
		got[p.Key] = p.Count
		if p.Key == "usa" && p.Name != "usa" {
			t.Errorf("usa is named %q, want its key: no use spells it", p.Name)
		}
	}
	want := map[string]int{
		"springfield,sangamon": 2, "illinois,usa": 2, "usa": 2,
		"chicago,illinois": 1, "illinois": 1,
		"x": 2, "y": 1, // the cycle is broken at x, as in the tree
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Places counts = %v, want %v", got, want)
	}

	tree := BuildPlaceTree(people, facts, nil)
	for key, count := range want {
		level, ok := tree.Level(PlaceID(key))
		if !ok {
			t.Errorf("%s is listed but not in the tree", key)
			continue
		}
		parent := uuid.Nil
		if n := len(level.Path); n > 1 {
			parent = level.Path[n-2].ID
		}
		siblings, _ := tree.Level(parent)
		for _, e := range siblings.Places {
			if e.ID == PlaceID(key) && e.Count != count {
				t.Errorf("tree count of %s = %d, list count %d", key, e.Count, count)
			}
		}
	}
}
