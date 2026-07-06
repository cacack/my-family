package domain

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// DateQualifier represents the precision qualifier for a genealogical date.
type DateQualifier string

const (
	DateExact DateQualifier = "exact" // No qualifier, precise date
	DateAbout DateQualifier = "abt"   // About/approximately (ABT)
	DateCalc  DateQualifier = "cal"   // Calculated (CAL)
	DateEst   DateQualifier = "est"   // Estimated (EST)
	DateBef   DateQualifier = "bef"   // Before (BEF)
	DateAft   DateQualifier = "aft"   // After (AFT)
	DateBet   DateQualifier = "bet"   // Between (BET ... AND ...)
	DateFrom  DateQualifier = "from"  // From/to range (FROM ... TO ...)
	DateInt   DateQualifier = "int"   // Interpreted (INT <date> (<original phrase>))
)

// IsValid checks if the date qualifier is valid.
func (d DateQualifier) IsValid() bool {
	switch d {
	case DateExact, DateAbout, DateCalc, DateEst, DateBef, DateAft, DateBet, DateFrom, DateInt:
		return true
	default:
		return false
	}
}

// GenDate represents a genealogical date with flexible precision per GEDCOM 5.5 spec.
type GenDate struct {
	Raw       string        `json:"raw"`                // Original input string
	Qualifier DateQualifier `json:"qualifier"`          // Date qualifier
	Year      *int          `json:"year,omitempty"`     // Year (nil if unknown)
	Month     *int          `json:"month,omitempty"`    // Month 1-12 (nil if unknown)
	Day       *int          `json:"day,omitempty"`      // Day 1-31 (nil if unknown)
	Year2     *int          `json:"year2,omitempty"`    // End year for ranges
	Month2    *int          `json:"month2,omitempty"`   // End month for ranges
	Day2      *int          `json:"day2,omitempty"`     // End day for ranges
	Calendar  string        `json:"calendar,omitempty"` // DGREGORIAN (default), DJULIAN, etc.

	// InterpretedFrom holds the original ambiguous phrase for interpreted (INT)
	// dates, e.g. "about eighteen fifty" from "INT 1850 (about eighteen fifty)".
	// Preserved for research transparency so others can evaluate the interpretation.
	InterpretedFrom string `json:"interpreted_from,omitempty"`
}

// Calendar identifiers stored in GenDate.Calendar. These match the token inside
// a GEDCOM calendar escape sequence (e.g. "@#DJULIAN@" -> "DJULIAN").
const (
	CalendarGregorian = "DGREGORIAN" // Default, modern calendar
	CalendarJulian    = "DJULIAN"    // Julian calendar (pre-1752 English records)
	CalendarHebrew    = "DHEBREW"    // Hebrew calendar (Jewish records)
	CalendarFrench    = "DFRENCH R"  // French Republican calendar (1792-1805)
)

// calendarTokens is the set of recognized calendar escape tokens.
var calendarTokens = map[string]bool{
	CalendarGregorian: true,
	CalendarJulian:    true,
	CalendarHebrew:    true,
	CalendarFrench:    true,
}

// GEDCOM month abbreviations (Gregorian and Julian share these).
var monthMap = map[string]int{
	"JAN": 1, "FEB": 2, "MAR": 3, "APR": 4, "MAY": 5, "JUN": 6,
	"JUL": 7, "AUG": 8, "SEP": 9, "OCT": 10, "NOV": 11, "DEC": 12,
}

var reverseMonthMap = map[int]string{
	1: "JAN", 2: "FEB", 3: "MAR", 4: "APR", 5: "MAY", 6: "JUN",
	7: "JUL", 8: "AUG", 9: "SEP", 10: "OCT", 11: "NOV", 12: "DEC",
}

// hebrewMonthMap maps Hebrew month codes to month numbers (GEDCOM numbering,
// Tishrei=1). Adar II (ADS=7) only exists in leap years.
var hebrewMonthMap = map[string]int{
	"TSH": 1, "CSH": 2, "KSL": 3, "TVT": 4, "SHV": 5, "ADR": 6, "ADS": 7,
	"NSN": 8, "IYR": 9, "SVN": 10, "TMZ": 11, "AAV": 12, "ELL": 13,
}

var reverseHebrewMonthMap = reverseMonthCodes(hebrewMonthMap)

// frenchMonthMap maps French Republican month codes to month numbers. Month 13
// (COMP) is the complementary days (Sans-culottides).
//
//nolint:misspell // THER is the GEDCOM code for Thermidor, not "there"
var frenchMonthMap = map[string]int{
	"VEND": 1, "BRUM": 2, "FRIM": 3, "NIVO": 4, "PLUV": 5, "VENT": 6, "GERM": 7,
	"FLOR": 8, "PRAI": 9, "MESS": 10, "THER": 11, "FRUC": 12, "COMP": 13,
}

var reverseFrenchMonthMap = reverseMonthCodes(frenchMonthMap)

// reverseMonthCodes inverts a month-code map for formatting.
func reverseMonthCodes(m map[string]int) map[int]string {
	r := make(map[int]string, len(m))
	for code, num := range m {
		r[num] = code
	}
	return r
}

// monthMapFor returns the month-code lookup table for the given calendar.
func monthMapFor(calendar string) map[string]int {
	switch calendar {
	case CalendarHebrew:
		return hebrewMonthMap
	case CalendarFrench:
		return frenchMonthMap
	default: // Gregorian and Julian
		return monthMap
	}
}

// reverseMonthMapFor returns the month-number to code table for the given calendar.
func reverseMonthMapFor(calendar string) map[int]string {
	switch calendar {
	case CalendarHebrew:
		return reverseHebrewMonthMap
	case CalendarFrench:
		return reverseFrenchMonthMap
	default: // Gregorian and Julian
		return reverseMonthMap
	}
}

// maxMonthFor returns the highest valid month number for the given calendar.
// Hebrew and French Republican calendars have a 13th month.
func maxMonthFor(calendar string) int {
	if calendar == CalendarHebrew || calendar == CalendarFrench {
		return 13
	}
	return 12
}

// qualifierPrefixes maps GEDCOM date prefixes to their qualifiers.
var qualifierPrefixes = []struct {
	prefix    string
	qualifier DateQualifier
}{
	{"ABT ", DateAbout},
	{"ABOUT ", DateAbout},
	{"CAL ", DateCalc},
	{"EST ", DateEst},
	{"BEF ", DateBef},
	{"BEFORE ", DateBef},
	{"AFT ", DateAft},
	{"AFTER ", DateAft},
	{"BET ", DateBet},
	{"FROM ", DateFrom},
}

// ParseGenDate parses a GEDCOM-format date string into a GenDate. It detects and
// strips a leading calendar escape sequence (e.g. "@#DJULIAN@") into Calendar,
// interpreting month codes according to that calendar; unrecognized escapes are
// left in place and the date is treated as Gregorian.
func ParseGenDate(s string) GenDate {
	s = strings.TrimSpace(s)
	if s == "" {
		return GenDate{}
	}

	gd := GenDate{
		Raw:       s,
		Qualifier: DateExact,
		Calendar:  CalendarGregorian,
	}

	work := strings.ToUpper(s)

	// Extract a calendar escape sequence (e.g. "@#DJULIAN@") if present. It may
	// appear before or after a qualifier, so we strip it first.
	if cal, rest, ok := extractCalendarEscape(work); ok {
		gd.Calendar = cal
		work = rest
	}

	// Handle interpreted dates (INT <date> (<original phrase>)) before the other
	// qualifiers, since the parenthetical phrase must keep its original casing.
	if strings.HasPrefix(work, "INT ") {
		return parseInterpretedGenDate(s, gd)
	}

	// Check for qualifiers using table-driven lookup
	for _, qp := range qualifierPrefixes {
		if strings.HasPrefix(work, qp.prefix) {
			gd.Qualifier = qp.qualifier
			work = strings.TrimPrefix(work, qp.prefix)
			break
		}
	}

	// Handle ranges (BET ... AND ..., FROM ... TO ...)
	if gd.Qualifier == DateBet {
		if parts := strings.SplitN(work, " AND ", 2); len(parts) == 2 {
			parseSimpleDate(strings.TrimSpace(parts[0]), gd.Calendar, &gd.Year, &gd.Month, &gd.Day)
			parseSimpleDate(strings.TrimSpace(parts[1]), gd.Calendar, &gd.Year2, &gd.Month2, &gd.Day2)
			return gd
		}
	}
	if gd.Qualifier == DateFrom {
		if parts := strings.SplitN(work, " TO ", 2); len(parts) == 2 {
			parseSimpleDate(strings.TrimSpace(parts[0]), gd.Calendar, &gd.Year, &gd.Month, &gd.Day)
			parseSimpleDate(strings.TrimSpace(parts[1]), gd.Calendar, &gd.Year2, &gd.Month2, &gd.Day2)
			return gd
		}
	}

	// Parse simple date
	parseSimpleDate(work, gd.Calendar, &gd.Year, &gd.Month, &gd.Day)
	return gd
}

// parseInterpretedGenDate parses an interpreted date of the form
// "INT <date> (<original phrase>)". The date portion is parsed normally and the
// parenthetical phrase is preserved (with its original casing) in InterpretedFrom.
// The "INT " prefix has already been detected by the caller; s is the original
// (case-preserving) input and gd carries Raw/Calendar defaults.
func parseInterpretedGenDate(s string, gd GenDate) GenDate {
	gd.Qualifier = DateInt

	rest := strings.TrimSpace(s[len("INT "):])
	datePart := rest
	if start := strings.Index(rest, "("); start != -1 {
		datePart = strings.TrimSpace(rest[:start])
		phrase := rest[start+1:]
		if end := strings.LastIndex(phrase, ")"); end != -1 {
			phrase = phrase[:end]
		}
		gd.InterpretedFrom = phrase
	}

	parseSimpleDate(strings.ToUpper(datePart), gd.Calendar, &gd.Year, &gd.Month, &gd.Day)
	return gd
}

// extractCalendarEscape finds and removes a GEDCOM calendar escape sequence
// (e.g. "@#DJULIAN@") from an upper-cased date string. It returns the calendar
// token, the remaining string with whitespace normalized, and whether a known
// escape was found. Unknown escapes are left in place and reported as not found.
func extractCalendarEscape(s string) (calendar, rest string, found bool) {
	start := strings.Index(s, "@#")
	if start == -1 {
		return "", s, false
	}
	end := strings.Index(s[start+2:], "@")
	if end == -1 {
		return "", s, false
	}
	token := s[start+2 : start+2+end]
	if !calendarTokens[token] {
		return "", s, false
	}
	remainder := s[:start] + " " + s[start+2+end+1:]
	return token, strings.Join(strings.Fields(remainder), " "), true
}

// parseSimpleDate parses a simple date like "1 JAN 1850", "JAN 1850", or "1850".
// Month codes are interpreted according to the given calendar system.
func parseSimpleDate(s, calendar string, year, month, day **int) {
	s = strings.TrimSpace(s)
	parts := strings.Fields(s)
	months := monthMapFor(calendar)

	switch len(parts) {
	case 1:
		// Year only: "1850"
		if y, err := strconv.Atoi(parts[0]); err == nil {
			*year = &y
		}
	case 2:
		// Month Year: "JAN 1850"
		if m, ok := months[parts[0]]; ok {
			*month = &m
			if y, err := strconv.Atoi(parts[1]); err == nil {
				*year = &y
			}
		}
	case 3:
		// Day Month Year: "1 JAN 1850"
		if d, err := strconv.Atoi(parts[0]); err == nil {
			*day = &d
		}
		if m, ok := months[parts[1]]; ok {
			*month = &m
		}
		if y, err := strconv.Atoi(parts[2]); err == nil {
			*year = &y
		}
	}
}

// String returns the GEDCOM-format string representation.
func (g *GenDate) String() string {
	if g.Raw != "" {
		return g.Raw
	}
	return g.Format()
}

// Format generates a GEDCOM-format date string from the parsed components.
func (g *GenDate) Format() string {
	if g.Year == nil {
		return ""
	}

	// GEDCOM order is qualifier then calendar escape, e.g. "ABT @#DJULIAN@ 1600".
	var qualPrefix string
	switch g.Qualifier {
	case DateAbout:
		qualPrefix = "ABT "
	case DateCalc:
		qualPrefix = "CAL "
	case DateEst:
		qualPrefix = "EST "
	case DateBef:
		qualPrefix = "BEF "
	case DateAft:
		qualPrefix = "AFT "
	case DateBet:
		return fmt.Sprintf("BET %s AND %s", g.calendarEscape()+formatSimpleDate(g.Calendar, g.Year, g.Month, g.Day), formatSimpleDate(g.Calendar, g.Year2, g.Month2, g.Day2))
	case DateFrom:
		return fmt.Sprintf("FROM %s TO %s", g.calendarEscape()+formatSimpleDate(g.Calendar, g.Year, g.Month, g.Day), formatSimpleDate(g.Calendar, g.Year2, g.Month2, g.Day2))
	case DateInt:
		datePart := g.calendarEscape() + formatSimpleDate(g.Calendar, g.Year, g.Month, g.Day)
		if g.InterpretedFrom != "" {
			return fmt.Sprintf("INT %s (%s)", datePart, g.InterpretedFrom)
		}
		return "INT " + datePart
	}

	return qualPrefix + g.calendarEscape() + formatSimpleDate(g.Calendar, g.Year, g.Month, g.Day)
}

// calendarEscape returns the GEDCOM escape prefix (with trailing space) for a
// non-Gregorian calendar, or an empty string for Gregorian/unset calendars.
func (g *GenDate) calendarEscape() string {
	if g.Calendar == "" || g.Calendar == CalendarGregorian {
		return ""
	}
	return "@#" + g.Calendar + "@ "
}

func formatSimpleDate(calendar string, year, month, day *int) string {
	if year == nil {
		return ""
	}
	months := reverseMonthMapFor(calendar)
	var parts []string
	if day != nil {
		parts = append(parts, strconv.Itoa(*day))
	}
	if month != nil {
		if code, ok := months[*month]; ok {
			parts = append(parts, code)
		}
	}
	parts = append(parts, strconv.Itoa(*year))
	return strings.Join(parts, " ")
}

// IsEmpty returns true if the date has no meaningful data.
func (g *GenDate) IsEmpty() bool {
	return g.Year == nil && g.Month == nil && g.Day == nil
}

// ToTime converts the GenDate to a time.Time for sorting purposes.
// Returns the earliest possible date based on the qualifier.
func (g *GenDate) ToTime() time.Time {
	if g.Year == nil {
		return time.Time{}
	}
	// Historical (non-Gregorian) calendar dates cannot be placed on a Gregorian
	// timeline directly, so convert them first. A conversion failure (e.g. a
	// malformed date) sorts as unknown rather than at an arbitrary instant.
	if g.Calendar != "" && g.Calendar != CalendarGregorian {
		greg, err := g.ToGregorian()
		if err != nil {
			return time.Time{}
		}
		return greg.ToTime()
	}
	year := *g.Year
	month := time.January
	day := 1
	if g.Month != nil {
		month = time.Month(*g.Month)
	}
	if g.Day != nil {
		day = *g.Day
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// SortDate returns a date suitable for sorting comparisons.
// For "before" dates, returns the date itself.
// For "after" dates, returns the date itself.
// For ranges, returns the start date.
func (g *GenDate) SortDate() time.Time {
	return g.ToTime()
}

// Validate checks if the date components are valid.
func (g *GenDate) Validate() error {
	maxMonth := maxMonthFor(g.Calendar)
	if g.Month != nil && (*g.Month < 1 || *g.Month > maxMonth) {
		return fmt.Errorf("invalid month: %d", *g.Month)
	}
	if g.Day != nil && (*g.Day < 1 || *g.Day > 31) {
		return fmt.Errorf("invalid day: %d", *g.Day)
	}
	if g.Month2 != nil && (*g.Month2 < 1 || *g.Month2 > maxMonth) {
		return fmt.Errorf("invalid month2: %d", *g.Month2)
	}
	if g.Day2 != nil && (*g.Day2 < 1 || *g.Day2 > 31) {
		return fmt.Errorf("invalid day2: %d", *g.Day2)
	}
	return nil
}

// Before returns true if this date is before the other date.
func (g *GenDate) Before(other *GenDate) bool {
	return g.ToTime().Before(other.ToTime())
}

// After returns true if this date is after the other date.
func (g *GenDate) After(other *GenDate) bool {
	return g.ToTime().After(other.ToTime())
}
