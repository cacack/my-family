package domain

import "testing"

func FuzzParseGenDate(f *testing.F) {
	// Exact dates
	f.Add("25 DEC 2020")
	f.Add("1 JAN 1850")
	f.Add("15 MAR 1900")

	// Month + year
	f.Add("JAN 1850")
	f.Add("DEC 2020")

	// Year only
	f.Add("1850")
	f.Add("2020")

	// Qualifiers
	f.Add("ABT 1850")
	f.Add("ABOUT 1900")
	f.Add("CAL 1850")
	f.Add("EST 1900")
	f.Add("BEF 1 JAN 1850")
	f.Add("BEFORE 1900")
	f.Add("AFT 25 DEC 2020")
	f.Add("AFTER 1850")

	// Ranges
	f.Add("BET 1850 AND 1860")
	f.Add("BET 1 JAN 1850 AND 31 DEC 1860")
	f.Add("FROM 1880 TO 1920")
	f.Add("FROM JAN 1880 TO DEC 1920")

	// Edge cases
	f.Add("")
	f.Add("   ")
	f.Add("not a date")
	f.Add("0")
	f.Add("-1")
	f.Add("99999")
	f.Add("BET AND")
	f.Add("FROM TO")
	f.Add("BET 1850 AND")
	f.Add("FROM 1880 TO")
	f.Add("ABT")
	f.Add("BEF")
	f.Add("1 13 2020")                  // invalid month
	f.Add("32 JAN 2020")                // invalid day
	f.Add("JAN")                        // month without year
	f.Add("1 JAN")                      // day month without year
	f.Add("BET 1850 AND 1860 AND 1870") // extra AND

	f.Fuzz(func(t *testing.T, s string) {
		// ParseGenDate must not panic on any input.
		// Errors are acceptable; panics are not.
		gd := ParseGenDate(s)

		// Exercise methods that operate on the result
		gd.IsEmpty()
		gd.ToTime()
		gd.SortDate()
		_ = gd.String()
		gd.Format()
		_ = gd.Validate()
		_ = gd.Qualifier.IsValid()
	})
}
