package repository

import "strings"

// NormalizePlace is the form every backend compares place strings in for the
// place hierarchy and place lookups (#925): parts are split on ",", trimmed of
// spaces, empty parts are dropped, and the rest are joined with ",". So
// "Darmstadt,,,Germany", "Darmstadt, Germany" and " Darmstadt , Germany, "
// all normalize to "Darmstadt,Germany", and a hierarchy level is never empty.
// The SQLite store calls it through a registered SQL function and the memory
// store calls it directly; the PostgreSQL store has an equivalent SQL
// expression, held to it by the shared place-hierarchy parity test.
func NormalizePlace(place string) string {
	parts := strings.Split(place, ",")
	kept := parts[:0]
	for _, p := range parts {
		if p = strings.Trim(p, " "); p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, ",")
}
