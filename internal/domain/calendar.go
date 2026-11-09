package domain

import (
	"fmt"

	gedcomlib "github.com/cacack/gedcom-go/v3/gedcom"
)

// domainToGedcomCalendar maps a domain calendar identifier to the gedcom-go
// calendar enum. The boolean is false for unrecognized calendars.
func domainToGedcomCalendar(calendar string) (gedcomlib.Calendar, bool) {
	switch calendar {
	case CalendarGregorian, "":
		return gedcomlib.CalendarGregorian, true
	case CalendarJulian:
		return gedcomlib.CalendarJulian, true
	case CalendarHebrew:
		return gedcomlib.CalendarHebrew, true
	case CalendarFrench:
		return gedcomlib.CalendarFrenchRepublican, true
	default:
		return gedcomlib.CalendarGregorian, false
	}
}

// ToGregorian converts a genealogical date expressed in a historical calendar
// (Julian, Hebrew, French Republican) into its equivalent in the Gregorian
// calendar, for comparison and sorting across calendar systems. Gregorian dates
// are returned unchanged.
//
// The calendrical arithmetic is delegated to gedcom-go. A year is required;
// partial dates (missing day or month) convert the components that are present.
// Range/period end dates (Year2/Month2/Day2) are converted as well, from the
// end's own calendar when it has one (Calendar2).
//
// Returns an error if a calendar is unrecognized or the date lacks a year.
func (g GenDate) ToGregorian() (GenDate, error) {
	if isGregorian(g.Calendar) && isGregorian(g.Calendar2) {
		return g, nil
	}

	year, month, day, err := gregorianComponentsFrom(g.Calendar, g.Year, g.Month, g.Day)
	if err != nil {
		return g, err
	}

	result := GenDate{
		Qualifier: g.Qualifier,
		Calendar:  CalendarGregorian,
		Year:      year,
		Month:     month,
		Day:       day,
	}

	// Convert the range/period end date when present so BET/FROM ranges remain
	// intact and comparable after conversion.
	if g.Year2 != nil {
		year2, month2, day2, err := gregorianComponentsFrom(g.endCalendar(), g.Year2, g.Month2, g.Day2)
		if err != nil {
			return g, fmt.Errorf("range end: %w", err)
		}
		result.Year2, result.Month2, result.Day2 = year2, month2, day2
	}

	result.Raw = result.Format()
	return result, nil
}

// endpointBounds returns the first and last day, as Julian day numbers, that
// one date in the given domain calendar can mean: a date without a day or
// month covers its whole month or year in that calendar, so Hebrew 5500 runs
// from September 1739 to September 1740. ok is false when the calendar is
// unrecognized, the year is missing, or gedcom-go can't place the date.
func endpointBounds(calendar string, year, month, day *int) (earliest, latest int, ok bool) {
	cal, known := domainToGedcomCalendar(calendar)
	if !known || year == nil {
		return 0, 0, false
	}
	return gedcomDate(cal, *year, month, day).Bounds()
}

// gedcomDate builds a gedcom-go date from domain components. The domain stores
// a BC year as negative; gedcom-go wants it positive with IsBC set.
func gedcomDate(cal gedcomlib.Calendar, year int, month, day *int) *gedcomlib.Date {
	d := &gedcomlib.Date{Calendar: cal, Year: year}
	if year < 0 {
		d.Year, d.IsBC = -year, true
	}
	if month != nil {
		d.Month = *month
	}
	if day != nil {
		d.Day = *day
	}
	return d
}

// calendarName returns a domain calendar's name for messages.
func calendarName(calendar string) string {
	switch calendar {
	case CalendarJulian:
		return "Julian"
	case CalendarHebrew:
		return "Hebrew"
	case CalendarFrench:
		return "French Republican"
	case CalendarGregorian, "":
		return "Gregorian"
	default:
		return calendar
	}
}

// isGregorian reports whether a domain calendar identifier is Gregorian; an
// empty identifier means Gregorian.
func isGregorian(calendar string) bool {
	return calendar == "" || calendar == CalendarGregorian
}

// gregorianComponentsFrom converts one (year, month, day) triple from the given
// domain calendar to Gregorian; a Gregorian triple is returned unchanged.
func gregorianComponentsFrom(calendar string, year, month, day *int) (*int, *int, *int, error) {
	if isGregorian(calendar) {
		return year, month, day, nil
	}
	cal, ok := domainToGedcomCalendar(calendar)
	if !ok {
		return nil, nil, nil, fmt.Errorf("unrecognized calendar: %q", calendar)
	}
	y, m, d, err := gregorianComponents(cal, year, month, day)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("converting %q to Gregorian: %w", calendar, err)
	}
	return y, m, d, nil
}

// gregorianComponents converts a single (year, month, day) triple from the given
// gedcom-go calendar into Gregorian components, preserving partial-date
// precision. A nil year yields an error since a year is required to convert.
func gregorianComponents(cal gedcomlib.Calendar, year, month, day *int) (*int, *int, *int, error) {
	if year == nil {
		return nil, nil, nil, fmt.Errorf("year is missing")
	}

	greg, err := gedcomDate(cal, *year, month, day).ToGregorian()
	if err != nil {
		return nil, nil, nil, err
	}

	var gy, gm, gd *int
	if greg.Year != 0 {
		y := greg.Year
		if greg.IsBC {
			y = -y
		}
		gy = &y
	}
	if greg.Month != 0 {
		m := greg.Month
		gm = &m
	}
	if greg.Day != 0 {
		d := greg.Day
		gd = &d
	}
	return gy, gm, gd, nil
}
