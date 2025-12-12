package domain

import (
	"fmt"
	"regexp"
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
)

// IsValid checks if the date qualifier is valid.
func (d DateQualifier) IsValid() bool {
	switch d {
	case DateExact, DateAbout, DateCalc, DateEst, DateBef, DateAft, DateBet, DateFrom:
		return true
	default:
		return false
	}
}

// GenDate represents a genealogical date with flexible precision per GEDCOM 5.5 spec.
type GenDate struct {
	Raw       string        `json:"raw"`                 // Original input string
	Qualifier DateQualifier `json:"qualifier"`           // Date qualifier
	Year      *int          `json:"year,omitempty"`      // Year (nil if unknown)
	Month     *int          `json:"month,omitempty"`     // Month 1-12 (nil if unknown)
	Day       *int          `json:"day,omitempty"`       // Day 1-31 (nil if unknown)
	Year2     *int          `json:"year2,omitempty"`     // End year for ranges
	Month2    *int          `json:"month2,omitempty"`    // End month for ranges
	Day2      *int          `json:"day2,omitempty"`      // End day for ranges
	Calendar  string        `json:"calendar,omitempty"`  // DGREGORIAN (default), DJULIAN, etc.
}

// GEDCOM month abbreviations
var monthMap = map[string]int{
	"JAN": 1, "FEB": 2, "MAR": 3, "APR": 4, "MAY": 5, "JUN": 6,
	"JUL": 7, "AUG": 8, "SEP": 9, "OCT": 10, "NOV": 11, "DEC": 12,
}

var reverseMonthMap = map[int]string{
	1: "JAN", 2: "FEB", 3: "MAR", 4: "APR", 5: "MAY", 6: "JUN",
	7: "JUL", 8: "AUG", 9: "SEP", 10: "OCT", 11: "NOV", 12: "DEC",
}

// ParseGenDate parses a GEDCOM-format date string into a GenDate.
func ParseGenDate(s string) GenDate {
	s = strings.TrimSpace(s)
	if s == "" {
		return GenDate{}
	}

	gd := GenDate{
		Raw:       s,
		Qualifier: DateExact,
		Calendar:  "DGREGORIAN",
	}

	upper := strings.ToUpper(s)

	// Check for qualifiers
	switch {
	case strings.HasPrefix(upper, "ABT "):
		gd.Qualifier = DateAbout
		s = strings.TrimPrefix(upper, "ABT ")
	case strings.HasPrefix(upper, "ABOUT "):
		gd.Qualifier = DateAbout
		s = strings.TrimPrefix(upper, "ABOUT ")
	case strings.HasPrefix(upper, "CAL "):
		gd.Qualifier = DateCalc
		s = strings.TrimPrefix(upper, "CAL ")
	case strings.HasPrefix(upper, "EST "):
		gd.Qualifier = DateEst
		s = strings.TrimPrefix(upper, "EST ")
	case strings.HasPrefix(upper, "BEF "):
		gd.Qualifier = DateBef
		s = strings.TrimPrefix(upper, "BEF ")
	case strings.HasPrefix(upper, "BEFORE "):
		gd.Qualifier = DateBef
		s = strings.TrimPrefix(upper, "BEFORE ")
	case strings.HasPrefix(upper, "AFT "):
		gd.Qualifier = DateAft
		s = strings.TrimPrefix(upper, "AFT ")
	case strings.HasPrefix(upper, "AFTER "):
		gd.Qualifier = DateAft
		s = strings.TrimPrefix(upper, "AFTER ")
	case strings.HasPrefix(upper, "BET "):
		gd.Qualifier = DateBet
		s = strings.TrimPrefix(upper, "BET ")
	case strings.HasPrefix(upper, "FROM "):
		gd.Qualifier = DateFrom
		s = strings.TrimPrefix(upper, "FROM ")
	default:
		s = upper
	}

	// Handle ranges (BET ... AND ..., FROM ... TO ...)
	if gd.Qualifier == DateBet {
		parts := strings.SplitN(s, " AND ", 2)
		if len(parts) == 2 {
			parseSimpleDate(strings.TrimSpace(parts[0]), &gd.Year, &gd.Month, &gd.Day)
			parseSimpleDate(strings.TrimSpace(parts[1]), &gd.Year2, &gd.Month2, &gd.Day2)
			return gd
		}
	}
	if gd.Qualifier == DateFrom {
		parts := strings.SplitN(s, " TO ", 2)
		if len(parts) == 2 {
			parseSimpleDate(strings.TrimSpace(parts[0]), &gd.Year, &gd.Month, &gd.Day)
			parseSimpleDate(strings.TrimSpace(parts[1]), &gd.Year2, &gd.Month2, &gd.Day2)
			return gd
		}
	}

	// Parse simple date
	parseSimpleDate(s, &gd.Year, &gd.Month, &gd.Day)
	return gd
}

// parseSimpleDate parses a simple date like "1 JAN 1850", "JAN 1850", or "1850".
func parseSimpleDate(s string, year, month, day **int) {
	s = strings.TrimSpace(s)
	parts := strings.Fields(s)

	switch len(parts) {
	case 1:
		// Year only: "1850"
		if y, err := strconv.Atoi(parts[0]); err == nil {
			*year = &y
		}
	case 2:
		// Month Year: "JAN 1850"
		if m, ok := monthMap[parts[0]]; ok {
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
		if m, ok := monthMap[parts[1]]; ok {
			*month = &m
		}
		if y, err := strconv.Atoi(parts[2]); err == nil {
			*year = &y
		}
	}
}

// String returns the GEDCOM-format string representation.
func (g GenDate) String() string {
	if g.Raw != "" {
		return g.Raw
	}
	return g.Format()
}

// Format generates a GEDCOM-format date string from the parsed components.
func (g GenDate) Format() string {
	if g.Year == nil {
		return ""
	}

	var prefix string
	switch g.Qualifier {
	case DateAbout:
		prefix = "ABT "
	case DateCalc:
		prefix = "CAL "
	case DateEst:
		prefix = "EST "
	case DateBef:
		prefix = "BEF "
	case DateAft:
		prefix = "AFT "
	case DateBet:
		return fmt.Sprintf("BET %s AND %s", formatSimpleDate(g.Year, g.Month, g.Day), formatSimpleDate(g.Year2, g.Month2, g.Day2))
	case DateFrom:
		return fmt.Sprintf("FROM %s TO %s", formatSimpleDate(g.Year, g.Month, g.Day), formatSimpleDate(g.Year2, g.Month2, g.Day2))
	}

	return prefix + formatSimpleDate(g.Year, g.Month, g.Day)
}

func formatSimpleDate(year, month, day *int) string {
	if year == nil {
		return ""
	}
	var parts []string
	if day != nil {
		parts = append(parts, strconv.Itoa(*day))
	}
	if month != nil && *month >= 1 && *month <= 12 {
		parts = append(parts, reverseMonthMap[*month])
	}
	parts = append(parts, strconv.Itoa(*year))
	return strings.Join(parts, " ")
}

// IsEmpty returns true if the date has no meaningful data.
func (g GenDate) IsEmpty() bool {
	return g.Year == nil && g.Month == nil && g.Day == nil
}

// ToTime converts the GenDate to a time.Time for sorting purposes.
// Returns the earliest possible date based on the qualifier.
func (g GenDate) ToTime() time.Time {
	if g.Year == nil {
		return time.Time{}
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
func (g GenDate) SortDate() time.Time {
	return g.ToTime()
}

// Validate checks if the date components are valid.
func (g GenDate) Validate() error {
	if g.Month != nil && (*g.Month < 1 || *g.Month > 12) {
		return fmt.Errorf("invalid month: %d", *g.Month)
	}
	if g.Day != nil && (*g.Day < 1 || *g.Day > 31) {
		return fmt.Errorf("invalid day: %d", *g.Day)
	}
	if g.Month2 != nil && (*g.Month2 < 1 || *g.Month2 > 12) {
		return fmt.Errorf("invalid month2: %d", *g.Month2)
	}
	if g.Day2 != nil && (*g.Day2 < 1 || *g.Day2 > 31) {
		return fmt.Errorf("invalid day2: %d", *g.Day2)
	}
	return nil
}

// Before returns true if this date is before the other date.
func (g GenDate) Before(other GenDate) bool {
	return g.ToTime().Before(other.ToTime())
}

// After returns true if this date is after the other date.
func (g GenDate) After(other GenDate) bool {
	return g.ToTime().After(other.ToTime())
}

// Compile regex at package init time for performance.
var yearOnlyRegex = regexp.MustCompile(`^\d{4}$`)
