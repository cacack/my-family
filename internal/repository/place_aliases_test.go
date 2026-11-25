package repository

import (
	"reflect"
	"testing"
)

// TestStatePlaceAlias covers the projector's alias rule (ADR-009 §3, PL-004):
// a statement that would close a cycle is ignored, a chain that concurrent
// commands let through is resolved to its end, and an unalias re-points every
// alias whose chain passed through the place.
func TestStatePlaceAlias(t *testing.T) {
	row := func(key, target, resolved string) PlaceAliasReadModel {
		return PlaceAliasReadModel{Key: key, TargetKey: target, ResolvedKey: resolved}
	}
	state := func(rows []PlaceAliasReadModel, key, target string, wantOK bool) []PlaceAliasReadModel {
		t.Helper()
		got, ok := StatePlaceAlias(rows, key, target)
		if ok != wantOK {
			t.Fatalf("StatePlaceAlias(%s -> %s) ok = %v, want %v", key, target, ok, wantOK)
		}
		return got
	}

	// a -> b, then b -> c (both passed their checks): a resolves to c.
	rows := state(nil, "a", "b", true)
	rows = state(rows, "b", "c", true)
	if want := []PlaceAliasReadModel{row("a", "b", "c"), row("b", "c", "c")}; !reflect.DeepEqual(rows, want) {
		t.Errorf("chain = %+v, want %+v", rows, want)
	}
	// c -> a would close a cycle through b; c -> c is the place itself.
	for _, target := range []string{"a", "b", "c"} {
		if got := state(rows, "c", target, false); !reflect.DeepEqual(got, rows) {
			t.Errorf("ignored c -> %s changed the rows to %+v", target, got)
		}
	}
	if !PlaceAliasClosesCycle(rows, "c", "a") || PlaceAliasClosesCycle(rows, "d", "a") {
		t.Error("PlaceAliasClosesCycle disagrees with StatePlaceAlias")
	}
	// Re-stating a moves it, and its chain with it.
	if got, want := state(rows, "a", "d", true), []PlaceAliasReadModel{row("a", "d", "d"), row("b", "c", "c")}; !reflect.DeepEqual(got, want) {
		t.Errorf("re-pointed = %+v, want %+v", got, want)
	}
	// Unaliasing b leaves a resolving to b again.
	if got, want := DropPlaceAlias(rows, "b"), []PlaceAliasReadModel{row("a", "b", "b")}; !reflect.DeepEqual(got, want) {
		t.Errorf("after unaliasing b = %+v, want %+v", got, want)
	}
	if got := DropPlaceAlias(rows, "x"); !reflect.DeepEqual(got, rows) {
		t.Errorf("unaliasing a place that is no alias = %+v, want the rows unchanged", got)
	}

	aliases := PlaceAliasesByKey(rows)
	if aliases.Resolve("a") != "c" || aliases.Resolve("c") != "c" || aliases.Resolve("z") != "z" {
		t.Errorf("Resolve = %q %q %q, want c c z", aliases.Resolve("a"), aliases.Resolve("c"), aliases.Resolve("z"))
	}
}

// TestPlaces_Aliases shows that only the whole key is joined: aliasing
// "hesse,germany" puts Darmstadt beneath Hessen without rewriting its key, the
// absorbed place keeps its own count, and a use counts once toward a place it
// reaches both directly and through an alias (ADR-009 §3, §4).
func TestPlaces_Aliases(t *testing.T) {
	uses := []PlaceUse{
		{Key: "darmstadt,hesse,germany", Text: "Darmstadt, Hesse, Germany", Count: 2},
		{Key: "hessen,germany", Text: "Hessen, Germany", Count: 1},
	}
	list := func(aliases PlaceAliases) map[string]PlaceReadModel {
		got := map[string]PlaceReadModel{}
		for _, p := range Places(uses, nil, aliases, Privacy{}) {
			got[p.Key] = p
		}
		return got
	}
	got := list(PlaceAliases{"hesse,germany": "hessen,germany"})
	if len(got) != 4 {
		t.Fatalf("Places = %+v, want darmstadt, hesse, hessen and germany", got)
	}
	if h := got["hessen,germany"]; h.Count != 3 || len(h.Aliases) != 1 || h.Aliases[0].Key != "hesse,germany" || h.Aliases[0].Name != "Hesse, Germany" {
		t.Errorf("hessen = %+v, want a count of 3 with hesse as its alias", h)
	}
	if h := got["hesse,germany"]; h.Count != 2 || h.ResolvesTo == nil || *h.ResolvesTo != PlaceID("hessen,germany") {
		t.Errorf("hesse = %+v, want its own count of 2, resolving to hessen", h)
	}
	if g := got["germany"]; g.Count != 3 {
		t.Errorf("germany = %+v, want a count of 3", g)
	}
	if _, ok := got["darmstadt,hessen,germany"]; ok {
		t.Error("the alias rewrote darmstadt's key; only the whole key is joined")
	}

	// Darmstadt absorbed into Germany sits beneath Germany, not Hessen, as in
	// the place browser; Germany is above both and counts each use once.
	got = list(PlaceAliases{"hesse,germany": "hessen,germany", "darmstadt,hesse,germany": "germany"})
	if d := got["darmstadt,hesse,germany"]; d.Count != 2 || d.ResolvesTo == nil || *d.ResolvesTo != PlaceID("germany") {
		t.Errorf("darmstadt = %+v, want its own count of 2, resolving to germany", d)
	}
	if h := got["hessen,germany"]; h.Count != 1 || len(h.Aliases) != 1 {
		t.Errorf("hessen = %+v, want a count of 1 with hesse as its alias", h)
	}
	if g := got["germany"]; g.Count != 3 || len(g.Aliases) != 1 {
		t.Errorf("germany = %+v, want a count of 3 with darmstadt as its alias", g)
	}

	// A target no use names takes its key as its name.
	only := Places([]PlaceUse{{Key: "koeln", Text: "Koeln", Count: 1}}, nil, PlaceAliases{"koeln": "cologne"}, Privacy{})
	if len(only) != 2 || only[1].Key != "cologne" || only[1].Name != "cologne" || only[1].Count != 1 {
		t.Errorf("Places with an unnamed target = %+v", only)
	}
}

// TestPlaces_AliasTargetBypassed shows that an absorbed place whose target no
// use reaches, because an explicit parent bypasses it, is listed on no place
// rather than on an unrelated one.
func TestPlaces_AliasTargetBypassed(t *testing.T) {
	uses := []PlaceUse{
		{Key: "darmstadt,hesse,germany", Text: "Darmstadt, Hesse, Germany", Count: 1},
		{Key: "aachen", Text: "Aachen", Count: 1},
	}
	facts := []PlaceFactsReadModel{{Key: "darmstadt,hesse,germany", ParentKey: "bavaria"}}
	aliases := PlaceAliases{"hesse,germany": "hessen,germany"}
	for _, p := range Places(uses, facts, aliases, Privacy{HidePrivate: true}) {
		if p.Key == "hessen,germany" {
			t.Errorf("hessen,germany is listed: no use reaches it")
		}
		if len(p.Aliases) != 0 {
			t.Errorf("%s lists aliases %v, want none", p.Key, p.Aliases)
		}
	}
}
