package domain

import (
	"fmt"

	gedcomlib "github.com/cacack/gedcom-go/v2/gedcom"
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
// Range/period end dates (Year2/Month2/Day2) are converted as well.
//
// Returns an error if the calendar is unrecognized or the date lacks a year.
func (g GenDate) ToGregorian() (GenDate, error) {
	if g.Calendar == "" || g.Calendar == CalendarGregorian {
		return g, nil
	}

	cal, ok := domainToGedcomCalendar(g.Calendar)
	if !ok {
		return g, fmt.Errorf("unrecognized calendar: %q", g.Calendar)
	}

	year, month, day, err := gregorianComponents(cal, g.Year, g.Month, g.Day)
	if err != nil {
		return g, fmt.Errorf("converting %q to Gregorian: %w", g.Calendar, err)
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
		year2, month2, day2, err := gregorianComponents(cal, g.Year2, g.Month2, g.Day2)
		if err != nil {
			return g, fmt.Errorf("converting %q range end to Gregorian: %w", g.Calendar, err)
		}
		result.Year2, result.Month2, result.Day2 = year2, month2, day2
	}

	result.Raw = result.Format()
	return result, nil
}

// gregorianComponents converts a single (year, month, day) triple from the given
// gedcom-go calendar into Gregorian components, preserving partial-date
// precision. A nil year yields an error since a year is required to convert.
func gregorianComponents(cal gedcomlib.Calendar, year, month, day *int) (*int, *int, *int, error) {
	if year == nil {
		return nil, nil, nil, fmt.Errorf("year is missing")
	}

	src := &gedcomlib.Date{Calendar: cal, Year: *year}
	if month != nil {
		src.Month = *month
	}
	if day != nil {
		src.Day = *day
	}

	greg, err := src.ToGregorian()
	if err != nil {
		return nil, nil, nil, err
	}

	var gy, gm, gd *int
	if greg.Year != 0 {
		y := greg.Year
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
