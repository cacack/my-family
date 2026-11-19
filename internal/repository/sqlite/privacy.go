package sqlite

import (
	"strings"

	"github.com/cacack/my-family/internal/repository"
)

// The SQL form of the privacy rule (ADR-008 §5): conditions that keep only the
// rows a repository.Privacy shows, built from the same fields its Go methods
// read. The integration suite's shared table of cases holds the two forms to
// the same answers. The conditions name columns unqualified, so they go where
// one table's (or overlay's) columns are in scope. They carry no placeholders:
// the only value, the living cutoff, is a date the server computes, written as
// a literal so the conditions slot into statements with any bind order.

// notPrivateSQL is the negation of domain.Restriction.IsPrivate on col: the
// comma-separated values, lowercased with spaces removed, hold neither
// "confidential" nor "privacy".
func notPrivateSQL(col string) string {
	norm := "',' || REPLACE(LOWER(COALESCE(" + col + ", '')), ' ', '') || ','"
	return "NOT (" + norm + " LIKE '%,confidential,%' OR " + norm + " LIKE '%,privacy,%')"
}

// recordPrivacySQL returns " AND " and the condition that keeps only records
// p does not hide, or "" when p hides none.
func recordPrivacySQL(p repository.Privacy) string {
	if !p.HidePrivate {
		return ""
	}
	return " AND " + notPrivateSQL("restriction")
}

// personPrivacySQL returns " AND " and the conditions that keep only the people
// p does not protect (repository.Privacy.Protects), or "" when it protects no
// one. A person is not living when the override says deceased, or else, with
// no override saying living, when there is death evidence or the reference
// date is before the cutoff; a NULL reference date is no date, so living.
func personPrivacySQL(p repository.Privacy) string {
	var conds []string
	if p.HidePrivate {
		conds = append(conds, notPrivateSQL("restriction"))
	}
	if p.HideLiving {
		cutoff := formatSortDate(p.LivingCutoff, sortDateLayout)
		conds = append(conds, "(COALESCE(living_override, '') = 'deceased' OR (COALESCE(living_override, '') <> 'living' AND "+
			"(death_evidence = 1 OR living_ref_date < '"+cutoff+"')))")
	}
	if len(conds) == 0 {
		return ""
	}
	return " AND " + strings.Join(conds, " AND ")
}
