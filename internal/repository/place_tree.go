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

// PlaceTree is the place hierarchy of a set of records (ADR-009 §4): the
// places their keys resolve to and every place above one of those, each under
// its parent. It is built in Go from the uses of the records, the explicit
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
	count    int      // records that resolve to it or to a place beneath it
	name     string
	fullName string
}

// BuildPlaceTree walks parents over the places the uses resolve to. A use
// counts toward the place its key resolves to and every place above it, so a
// place's count is the records that resolve to it or to a place beneath it. A
// parent no use names directly is a place like any other. A place is shown by
// its display name when facts set one, otherwise by its most-used spelling
// (ties broken bytewise), or its key when no use spells it; at its level it
// shows that name's first part. A parent that closes a cycle, which only an
// alias or unalias stated after it can cause, is dropped at the cycle's
// bytewise smallest key, so the tree never depends on map order.
func BuildPlaceTree(uses []PlaceUse, facts []PlaceFactsReadModel, aliases PlaceAliases) *PlaceTree {
	parents := PlaceParentsByKey(facts)
	t := &PlaceTree{nodes: make(map[string]*placeNode), ids: make(map[uuid.UUID]string)}
	spellings := make(map[string]map[string]int)
	own := make(map[string]int)
	node := func(key string) *placeNode {
		n, ok := t.nodes[key]
		if !ok {
			n = &placeNode{key: key}
			t.nodes[key] = n
		}
		return n
	}
	for _, u := range uses {
		if u.Key == "" {
			continue
		}
		n := node(aliases.Resolve(u.Key))
		if !slices.Contains(n.keys, u.Key) {
			n.keys = append(n.keys, u.Key)
		}
		own[n.key] += u.Count
		keyParts := strings.Split(u.Key, ",")
		nameParts := placeParts(u.Text, true)
		if len(nameParts) != len(keyParts) {
			nameParts = keyParts
		}
		for i := range keyParts {
			suffix := strings.Join(keyParts[i:], ",")
			if spellings[suffix] == nil {
				spellings[suffix] = make(map[string]int)
			}
			spellings[suffix][strings.Join(nameParts[i:], ", ")] += u.Count
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
		for k := key; k != ""; k = t.nodes[k].parent {
			t.nodes[k].count += own[key]
		}
	}
	for absorbed, target := range aliases {
		if _, ok := t.nodes[target]; ok {
			t.ids[PlaceID(absorbed)] = target
		}
	}
	return t
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
// id or a place beneath it: the records whose place has one of these keys are
// the records its count counts. An absorbed place's ID names the place it
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
