package gedcom

import (
	"github.com/cacack/my-family/internal/domain"
)

// ToGregorian converts a genealogical date expressed in a historical calendar
// (Julian, Hebrew, French Republican) into its equivalent in the Gregorian
// calendar. Gregorian dates are returned unchanged.
//
// The conversion lives on domain.GenDate so it is reachable from the read-model
// projection and query layers (which cannot import this package); this is a thin
// convenience wrapper for GEDCOM-side callers.
func ToGregorian(gd domain.GenDate) (domain.GenDate, error) {
	return gd.ToGregorian()
}
