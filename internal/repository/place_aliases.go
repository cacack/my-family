package repository

import "sort"

// PlaceAliasReadModel is a row of place_aliases (ADR-009 §3): the key of an
// absorbed place, the key its PlaceAliased names as stated, and the key it
// resolves to, which is never itself absorbed (PL-004). The table is
// MAIN-ONLY (PL-005). It holds keys, not IDs: a place's ID is PlaceID of its
// key.
type PlaceAliasReadModel struct {
	Key         string
	TargetKey   string
	ResolvedKey string
}

// PlaceAliases maps each absorbed key to the key it resolves to. A reader
// resolves a key in one step and never follows a chain (ADR-009 §3).
type PlaceAliases map[string]string

// PlaceAliasesByKey indexes the rows of place_aliases by absorbed key.
func PlaceAliasesByKey(rows []PlaceAliasReadModel) PlaceAliases {
	aliases := make(PlaceAliases, len(rows))
	for _, a := range rows {
		aliases[a.Key] = a.ResolvedKey
	}
	return aliases
}

// Resolve returns the key key resolves to: its target when it is absorbed,
// otherwise key itself.
func (a PlaceAliases) Resolve(key string) string {
	if target, ok := a[key]; ok {
		return target
	}
	return key
}

// PlaceAliasClosesCycle reports whether stating key as an alias of target,
// beside the stated aliases rows, would close a cycle: target is key, or
// target's stated chain leads back to key.
func PlaceAliasClosesCycle(rows []PlaceAliasReadModel, key, target string) bool {
	stated := statedPlaceAliases(rows)
	for next, ok := target, true; ok; next, ok = stated[next] {
		if next == key {
			return true
		}
	}
	return false
}

// StatePlaceAlias returns the rows of place_aliases after key is stated as an
// alias of target, replacing any alias key had, with every resolved key
// recomputed. A statement that would close a cycle (PlaceAliasClosesCycle)
// is ignored, so ok is false and rows come back unchanged: the projector
// applies alias events in log order, and the first of two concurrent
// statements wins (ADR-009 §3, PL-004).
func StatePlaceAlias(rows []PlaceAliasReadModel, key, target string) (_ []PlaceAliasReadModel, ok bool) {
	if PlaceAliasClosesCycle(rows, key, target) {
		return rows, false
	}
	stated := statedPlaceAliases(rows)
	stated[key] = target
	return resolvePlaceAliases(stated), true
}

// DropPlaceAlias returns the rows of place_aliases after key's alias is
// undone, with every resolved key recomputed: an alias whose stated chain
// passed through key resolves to key again.
func DropPlaceAlias(rows []PlaceAliasReadModel, key string) []PlaceAliasReadModel {
	stated := statedPlaceAliases(rows)
	delete(stated, key)
	return resolvePlaceAliases(stated)
}

// statedPlaceAliases maps each absorbed key to its stated target.
func statedPlaceAliases(rows []PlaceAliasReadModel) map[string]string {
	stated := make(map[string]string, len(rows))
	for _, a := range rows {
		stated[a.Key] = a.TargetKey
	}
	return stated
}

// resolvePlaceAliases resolves each stated alias to the end of its stated
// chain, a place that is not absorbed, and returns the rows sorted by key.
// stated holds no cycle: StatePlaceAlias never adds one.
func resolvePlaceAliases(stated map[string]string) []PlaceAliasReadModel {
	rows := make([]PlaceAliasReadModel, 0, len(stated))
	for key, target := range stated {
		resolved := target
		for next, ok := stated[resolved]; ok; next, ok = stated[resolved] {
			resolved = next
		}
		rows = append(rows, PlaceAliasReadModel{Key: key, TargetKey: target, ResolvedKey: resolved})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Key < rows[j].Key })
	return rows
}
