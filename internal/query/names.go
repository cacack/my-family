package query

import "strings"

// fullName joins a given name and surname with a single space, trimming the
// result. Either part may be empty; the helper centralizes the convention so
// adding middle names or supporting culture-specific orderings (e.g.,
// surname-first locales) is a single-site change rather than a grep.
func fullName(given, surname string) string {
	return strings.TrimSpace(given + " " + surname)
}
