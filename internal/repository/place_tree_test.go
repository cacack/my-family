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
	uses := []PlaceUse{
		{Key: "boston,ma", Text: "Boston, MA", Count: 2},
		{Key: "boston,ma", Text: "boston,ma", Count: 1},
		{Key: "darmstadt,hessen,germany", Text: "Darmstadt, Hessen, Germany", Count: 1},
		{Key: "kassel,hesse,germany", Text: "Kassel, Hesse, Germany", Count: 1},
		{Key: "springfield", Text: "Springfield", Count: 1},
		{Key: "chicago,illinois", Text: "Chicago, Illinois", Count: 1},
		{Key: "a,b", Text: "x", Count: 1},
		{Key: "", Text: " , ", Count: 9},
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
	tree := BuildPlaceTree(uses, facts, aliases)

	want := []string{
		"Germany | Germany | 2",
		"  Hessen | Hessen, Germany | 2",
		"    Darmstadt | Darmstadt, Hessen, Germany | 1",
		"    Kassel | Kassel, Hesse, Germany | 1",
		"MA | MA | 3",
		"  Boston | Boston, MA | 3",
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
// its smallest key, whatever the order of the uses.
func TestBuildPlaceTree_BreaksCycles(t *testing.T) {
	uses := []PlaceUse{
		{Key: "x", Text: "X", Count: 1},
		{Key: "y", Text: "Y", Count: 1},
		{Key: "berlin,germany", Text: "Berlin, Germany", Count: 1},
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
		if got := treeLines(t, BuildPlaceTree(uses, facts, aliases)); !reflect.DeepEqual(got, want) {
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
