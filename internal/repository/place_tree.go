package repository

import (
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// PlaceParents maps a place's key to the key of the explicit parent its facts
// state (ADR-009 §4). A key with no explicit parent is absent.
type PlaceParents map[string]string

// PlaceParentsByKey indexes the explicit parents of facts by place key.
func PlaceParentsByKey(facts []PlaceFactsReadModel) PlaceParents {
	parents := make(PlaceParents)
	for _, f := range facts {
		if f.ParentKey != "" {
			parents[f.Key] = f.ParentKey
		}
	}
	return parents
}

// Parent returns the key of the parent of the place with the resolved key key,
// resolved through aliases, or "" when it has none (ADR-009 §4). It is the
// place's explicit parent when one is stated, otherwise its default parent:
// key with its leftmost part removed. A one-part key has no default parent.
// An absorbed place's own explicit parent is never read, because key is
// always a resolved one.
func (p PlaceParents) Parent(key string, aliases PlaceAliases) string {
	if parent, ok := p[key]; ok {
		return aliases.Resolve(parent)
	}
	if i := strings.IndexByte(key, ','); i >= 0 {
		return aliases.Resolve(key[i+1:])
	}
	return ""
}

// leadsBack reports whether walking parents up from the resolved key start
// comes back to start. A cycle that does not pass through start ends the walk.
func (p PlaceParents) leadsBack(start string, aliases PlaceAliases) bool {
	seen := map[string]bool{}
	for k := p.Parent(start, aliases); k != "" && !seen[k]; k = p.Parent(k, aliases) {
		if k == start {
			return true
		}
		seen[k] = true
	}
	return false
}

// PlaceParentClosesCycle reports whether stating parent as the explicit parent
// of the place with key, beside the explicit parents and aliases already in
// force, would close a cycle, counted through aliases and default parents
// (ADR-009 §4): the place would be beneath itself.
func PlaceParentClosesCycle(parents PlaceParents, aliases PlaceAliases, key, parent string) bool {
	next := make(PlaceParents, len(parents)+1)
	for k, v := range parents {
		next[k] = v
	}
	next[key] = parent
	return next.leadsBack(aliases.Resolve(key), aliases)
}

// PlaceAliasClosesParentCycle reports whether the aliases after an alias
// statement, aliases, put the place target resolves to beneath itself
// (ADR-009 §4). Only the target's walk can change: every parent that named
// the absorbed key now names the target.
func PlaceAliasClosesParentCycle(parents PlaceParents, aliases PlaceAliases, target string) bool {
	return parents.leadsBack(aliases.Resolve(target), aliases)
}

// PlaceLevel is one level of the place browser: the places directly under a
// place, and the path from the top level down to that place (empty at the top
// level).
type PlaceLevel struct {
	Places []PlaceEntry
	Path   []PlaceEntry
}

// PersonPlaces is Count people born at the place recorded as BirthText, whose
// stored PlaceKey is BirthKey, and died at the one recorded as DeathText, whose
// stored PlaceKey is DeathKey. A key is "" for a place not recorded.
type PersonPlaces struct {
	BirthKey, BirthText string
	DeathKey, DeathText string
	Count               int
}

// PlaceTree is the place hierarchy of a set of people (ADR-009 §4): the places
// their birth and death keys resolve to and every place above one of those,
// each under its parent. It is built in Go from those places, the explicit
// parents and the aliases, so it is the same on every store (DB-001).
type PlaceTree struct {
	nodes map[string]*placeNode
	ids   map[uuid.UUID]string
	roots []string
}

type placeNode struct {
	key      string
	parent   string
	children []string
	keys     []string // the stored keys that resolve to this place
	count    int      // people with a place that resolves to it or to a place beneath it
	name     string
	fullName string
}

// BuildPlaceTree walks parents over the places the people's birth and death
// keys resolve to. A person counts once toward each place at or above either
// of their places, so a place's count is the people whose birth or death place
// resolves to it or to a place beneath it: the people GetPersonsByPlace lists
// behind it. A parent no person names directly is a place like any other. A
// place is shown by its display name when facts set one, otherwise by its
// most-used spelling (ties broken bytewise, each birth and death a use), or
// its key when no use spells it; at its level it shows that name's first part.
// A parent that closes a cycle, which only an alias or unalias stated after it
// can cause, is dropped at the cycle's bytewise smallest key, so the tree
// never depends on map order.
func BuildPlaceTree(people []PersonPlaces, facts []PlaceFactsReadModel, aliases PlaceAliases) *PlaceTree {
	parents := PlaceParentsByKey(facts)
	t := &PlaceTree{nodes: make(map[string]*placeNode), ids: make(map[uuid.UUID]string)}
	spellings := make(map[string]map[string]int)
	node := func(key string) *placeNode {
		n, ok := t.nodes[key]
		if !ok {
			n = &placeNode{key: key}
			t.nodes[key] = n
		}
		return n
	}
	for _, p := range people {
		for _, u := range [][2]string{{p.BirthKey, p.BirthText}, {p.DeathKey, p.DeathText}} {
			key, text := u[0], u[1]
			if key == "" {
				continue
			}
			n := node(aliases.Resolve(key))
			if !slices.Contains(n.keys, key) {
				n.keys = append(n.keys, key)
			}
			addSpellings(spellings, key, text, p.Count)
		}
	}

	// Add every parent, then drop the parent that closes each cycle.
	pending := make([]string, 0, len(t.nodes))
	for key := range t.nodes {
		pending = append(pending, key)
	}
	for len(pending) > 0 {
		n := t.nodes[pending[len(pending)-1]]
		pending = pending[:len(pending)-1]
		n.parent = parents.Parent(n.key, aliases)
		if n.parent == "" {
			continue
		}
		if _, ok := t.nodes[n.parent]; !ok {
			node(n.parent)
			pending = append(pending, n.parent)
		}
	}
	t.breakCycles()

	factsByKey := PlaceFactsByKey(facts)
	for key, n := range t.nodes {
		t.ids[PlaceID(key)] = key
		n.fullName = mostUsed(spellings[key])
		if len(spellings[key]) == 0 {
			n.fullName = key
		}
		n.name = strings.TrimSpace(strings.SplitN(n.fullName, ",", 2)[0])
		if f := factsByKey[key]; f != nil && f.Name != "" {
			n.name, n.fullName = f.Name, f.Name
		}
		if n.parent == "" {
			t.roots = append(t.roots, key)
		} else {
			p := t.nodes[n.parent]
			p.children = append(p.children, key)
		}
	}
	t.countPeople(people, aliases)
	for absorbed, target := range aliases {
		if _, ok := t.nodes[target]; ok {
			t.ids[PlaceID(absorbed)] = target
		}
	}
	return t
}

// addSpellings counts count uses of text, whose key is key, as the spelling of
// its place and, part by part, of each place its key names with leading parts
// removed. A text whose parts do not match its key's is spelled by the key.
func addSpellings(spellings map[string]map[string]int, key, text string, count int) {
	keyParts := strings.Split(key, ",")
	nameParts := placeParts(text, true)
	if len(nameParts) != len(keyParts) {
		nameParts = keyParts
	}
	for i := range keyParts {
		suffix := strings.Join(keyParts[i:], ",")
		if spellings[suffix] == nil {
			spellings[suffix] = make(map[string]int)
		}
		spellings[suffix][strings.Join(nameParts[i:], ", ")] += count
	}
}

// countPeople counts each person once at every place at or above either of
// their places, so a person born and died beneath one place counts there once.
func (t *PlaceTree) countPeople(people []PersonPlaces, aliases PlaceAliases) {
	for _, p := range people {
		under := make(map[string]bool)
		for _, k := range []string{p.BirthKey, p.DeathKey} {
			if k == "" {
				continue
			}
			for k = aliases.Resolve(k); k != "" && !under[k]; k = t.nodes[k].parent {
				under[k] = true
				t.nodes[k].count += p.Count
			}
		}
	}
}

// breakCycles makes the bytewise smallest key of every parent cycle a top-level
// place.
func (t *PlaceTree) breakCycles() {
	keys := make([]string, 0, len(t.nodes))
	for key := range t.nodes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	const (
		unvisited = iota
		onPath
		done
	)
	state := make(map[string]int, len(keys))
	for _, start := range keys {
		var path []string
		k := start
		for k != "" && state[k] == unvisited {
			state[k] = onPath
			path = append(path, k)
			k = t.nodes[k].parent
		}
		if k != "" && state[k] == onPath {
			smallest := k
			for c := t.nodes[k].parent; c != k; c = t.nodes[c].parent {
				smallest = min(smallest, c)
			}
			t.nodes[smallest].parent = ""
		}
		for _, p := range path {
			state[p] = done
		}
	}
}

// Level returns the places directly under the place with ID parent, or the
// top-level places when parent is uuid.Nil, sorted by name bytewise and then
// by key, with the path down to parent. An absorbed place's ID names the place
// it resolves to. ok is false when no place in the tree has the ID parent.
func (t *PlaceTree) Level(parent uuid.UUID) (level PlaceLevel, ok bool) {
	children := t.roots
	if parent != uuid.Nil {
		key, found := t.ids[parent]
		if !found {
			return PlaceLevel{Places: []PlaceEntry{}, Path: []PlaceEntry{}}, false
		}
		children = t.nodes[key].children
		for k := key; k != ""; k = t.nodes[k].parent {
			level.Path = append(level.Path, t.entry(k))
		}
		for i, j := 0, len(level.Path)-1; i < j; i, j = i+1, j-1 {
			level.Path[i], level.Path[j] = level.Path[j], level.Path[i]
		}
	}
	if level.Path == nil {
		level.Path = []PlaceEntry{}
	}
	level.Places = make([]PlaceEntry, 0, len(children))
	for _, key := range children {
		level.Places = append(level.Places, t.entry(key))
	}
	sort.Slice(level.Places, func(i, j int) bool {
		a, b := level.Places[i], level.Places[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return t.ids[a.ID] < t.ids[b.ID]
	})
	return level, true
}

// KeysUnder returns, sorted, the stored keys whose place is the place with ID
// id or a place beneath it: the people whose birth or death place has one of
// these keys are the people its count counts. An absorbed place's ID names the place it
// resolves to; an ID not in the tree has no keys.
func (t *PlaceTree) KeysUnder(id uuid.UUID) []string {
	key, ok := t.ids[id]
	if !ok {
		return nil
	}
	var keys []string
	stack := []string{key}
	for len(stack) > 0 {
		n := t.nodes[stack[len(stack)-1]]
		stack = stack[:len(stack)-1]
		keys = append(keys, n.keys...)
		stack = append(stack, n.children...)
	}
	sort.Strings(keys)
	return keys
}

// entry is the browser entry of the place with key.
func (t *PlaceTree) entry(key string) PlaceEntry {
	n := t.nodes[key]
	return PlaceEntry{ID: PlaceID(key), Name: n.name, FullName: n.fullName, Count: n.count, HasChildren: len(n.children) > 0}
}
