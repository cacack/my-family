package domain

import (
	"testing"
	"time"
)

func TestParseGenDate_Calendars(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantCalendar string
		wantQual     DateQualifier
		wantYear     *int
		wantMonth    *int
		wantDay      *int
	}{
		{
			name:         "gregorian default has no escape",
			input:        "25 DEC 2020",
			wantCalendar: CalendarGregorian,
			wantQual:     DateExact,
			wantYear:     intPtr(2020),
			wantMonth:    intPtr(12),
			wantDay:      intPtr(25),
		},
		{
			name:         "julian day month year",
			input:        "@#DJULIAN@ 14 FEB 1689",
			wantCalendar: CalendarJulian,
			wantQual:     DateExact,
			wantYear:     intPtr(1689),
			wantMonth:    intPtr(2),
			wantDay:      intPtr(14),
		},
		{
			name:         "hebrew month code",
			input:        "@#DHEBREW@ 15 NSN 5785",
			wantCalendar: CalendarHebrew,
			wantQual:     DateExact,
			wantYear:     intPtr(5785),
			wantMonth:    intPtr(8), // Nisan
			wantDay:      intPtr(15),
		},
		{
			name:         "hebrew thirteenth month",
			input:        "@#DHEBREW@ ELL 5785",
			wantCalendar: CalendarHebrew,
			wantQual:     DateExact,
			wantYear:     intPtr(5785),
			wantMonth:    intPtr(13), // Elul
		},
		{
			name:         "french republican with space in token",
			input:        "@#DFRENCH R@ 1 VEND 1",
			wantCalendar: CalendarFrench,
			wantQual:     DateExact,
			wantYear:     intPtr(1),
			wantMonth:    intPtr(1), // Vendemiaire
			wantDay:      intPtr(1),
		},
		{
			name:         "explicit gregorian escape",
			input:        "@#DGREGORIAN@ 4 JUL 1776",
			wantCalendar: CalendarGregorian,
			wantQual:     DateExact,
			wantYear:     intPtr(1776),
			wantMonth:    intPtr(7),
			wantDay:      intPtr(4),
		},
		{
			name:         "qualifier before calendar escape",
			input:        "ABT @#DJULIAN@ 1600",
			wantCalendar: CalendarJulian,
			wantQual:     DateAbout,
			wantYear:     intPtr(1600),
		},
		{
			name:         "unknown escape is left in place and ignored",
			input:        "@#DUNKNOWN@ 1600",
			wantCalendar: CalendarGregorian,
			wantQual:     DateExact,
			// The escape remains, so components do not parse.
		},
		{
			name:         "lowercase escape and month",
			input:        "@#djulian@ 14 feb 1689",
			wantCalendar: CalendarJulian,
			wantQual:     DateExact,
			wantYear:     intPtr(1689),
			wantMonth:    intPtr(2),
			wantDay:      intPtr(14),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseGenDate(tt.input)
			if got.Calendar != tt.wantCalendar {
				t.Errorf("Calendar = %q, want %q", got.Calendar, tt.wantCalendar)
			}
			if got.Qualifier != tt.wantQual {
				t.Errorf("Qualifier = %q, want %q", got.Qualifier, tt.wantQual)
			}
			assertIntPtr(t, "Year", got.Year, tt.wantYear)
			assertIntPtr(t, "Month", got.Month, tt.wantMonth)
			assertIntPtr(t, "Day", got.Day, tt.wantDay)
		})
	}
}

func TestParseGenDate_CalendarRange(t *testing.T) {
	got := ParseGenDate("BET @#DJULIAN@ 1600 AND 1700")
	if got.Calendar != CalendarJulian {
		t.Fatalf("Calendar = %q, want %q", got.Calendar, CalendarJulian)
	}
	if got.Qualifier != DateBet {
		t.Fatalf("Qualifier = %q, want %q", got.Qualifier, DateBet)
	}
	assertIntPtr(t, "Year", got.Year, intPtr(1600))
	assertIntPtr(t, "Year2", got.Year2, intPtr(1700))
}

func TestGenDate_Format_Calendars(t *testing.T) {
	tests := []struct {
		name string
		date GenDate
		want string
	}{
		{
			name: "julian full date emits escape",
			date: GenDate{Calendar: CalendarJulian, Qualifier: DateExact, Year: intPtr(1689), Month: intPtr(2), Day: intPtr(14)},
			want: "@#DJULIAN@ 14 FEB 1689",
		},
		{
			name: "hebrew date uses hebrew month code",
			date: GenDate{Calendar: CalendarHebrew, Qualifier: DateExact, Year: intPtr(5785), Month: intPtr(8), Day: intPtr(15)},
			want: "@#DHEBREW@ 15 NSN 5785",
		},
		{
			name: "french date uses french month code",
			date: GenDate{Calendar: CalendarFrench, Qualifier: DateExact, Year: intPtr(1), Month: intPtr(1), Day: intPtr(1)},
			want: "@#DFRENCH R@ 1 VEND 1",
		},
		{
			name: "qualifier precedes escape",
			date: GenDate{Calendar: CalendarJulian, Qualifier: DateAbout, Year: intPtr(1600)},
			want: "ABT @#DJULIAN@ 1600",
		},
		{
			name: "gregorian has no escape",
			date: GenDate{Calendar: CalendarGregorian, Qualifier: DateExact, Year: intPtr(2020), Month: intPtr(12), Day: intPtr(25)},
			want: "25 DEC 2020",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.date.Format(); got != tt.want {
				t.Errorf("Format() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestGenDate_CalendarRoundTrip ensures parse -> format reproduces the original
// GEDCOM string for historical calendars (export preserves escape sequences).
func TestGenDate_CalendarRoundTrip(t *testing.T) {
	inputs := []string{
		"@#DJULIAN@ 14 FEB 1689",
		"@#DHEBREW@ 15 NSN 5785",
		"@#DFRENCH R@ 1 VEND 1",
		"ABT @#DJULIAN@ 1600",
		"25 DEC 2020",
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			gd := ParseGenDate(in)
			// Raw preserves the exact original for export.
			if gd.String() != in {
				t.Errorf("String() = %q, want %q", gd.String(), in)
			}
			// Format reconstructs an equivalent string from components.
			if got := gd.Format(); got != in {
				t.Errorf("Format() = %q, want %q", got, in)
			}
		})
	}
}

func TestGenDate_Validate_Calendars(t *testing.T) {
	tests := []struct {
		name    string
		date    GenDate
		wantErr bool
	}{
		{
			name: "hebrew month 13 is valid",
			date: GenDate{Calendar: CalendarHebrew, Year: intPtr(5785), Month: intPtr(13)},
		},
		{
			name: "french month 13 is valid",
			date: GenDate{Calendar: CalendarFrench, Year: intPtr(1), Month: intPtr(13)},
		},
		{
			name:    "gregorian month 13 is invalid",
			date:    GenDate{Calendar: CalendarGregorian, Year: intPtr(1850), Month: intPtr(13)},
			wantErr: true,
		},
		{
			name:    "julian month 13 is invalid",
			date:    GenDate{Calendar: CalendarJulian, Year: intPtr(1600), Month: intPtr(13)},
			wantErr: true,
		},
		{
			name:    "hebrew month 14 is invalid",
			date:    GenDate{Calendar: CalendarHebrew, Year: intPtr(5785), Month: intPtr(14)},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.date.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestGenDate_ToTime_NonGregorian ensures historical-calendar dates are placed
// on the Gregorian timeline by converting first, so they sort correctly.
func TestGenDate_ToTime_NonGregorian(t *testing.T) {
	gd := ParseGenDate("@#DHEBREW@ 15 NSN 5785")
	got := gd.ToTime()
	// 15 Nisan 5785 (Hebrew) == 13 April 2025 (Gregorian).
	want := time.Date(2025, time.April, 13, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("ToTime() = %v, want %v", got, want)
	}
}

// TestGenDate_ToGregorian_Range ensures range/period end components are also
// converted rather than dropped, so BET/FROM ranges survive conversion.
func TestGenDate_ToGregorian_Range(t *testing.T) {
	gd := ParseGenDate("BET @#DJULIAN@ 1 JAN 1700 AND 31 DEC 1700")
	got, err := gd.ToGregorian()
	if err != nil {
		t.Fatalf("ToGregorian() error = %v", err)
	}
	if got.Calendar != CalendarGregorian {
		t.Errorf("Calendar = %q, want %q", got.Calendar, CalendarGregorian)
	}
	if got.Qualifier != DateBet {
		t.Errorf("Qualifier = %q, want %q", got.Qualifier, DateBet)
	}
	// Both endpoints must survive conversion (Julian 1700 is 11 days behind).
	if got.Year == nil || got.Year2 == nil {
		t.Fatalf("range endpoints dropped: got %+v", got)
	}
	if *got.Year != 1700 || *got.Year2 != 1701 {
		t.Errorf("years = %d..%d, want 1700..1701", *got.Year, *got.Year2)
	}
}

// TestGenDate_ToGregorian_UnknownCalendar ensures unrecognized calendars error.
func TestGenDate_ToGregorian_UnknownCalendar(t *testing.T) {
	gd := GenDate{Calendar: "DBOGUS", Year: intPtr(1600)}
	if _, err := gd.ToGregorian(); err == nil {
		t.Error("expected error for unrecognized calendar")
	}
}

func assertIntPtr(t *testing.T, field string, got, want *int) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("%s = %d, want nil", field, *got)
	case want != nil && got == nil:
		t.Errorf("%s = nil, want %d", field, *want)
	case want != nil && got != nil && *want != *got:
		t.Errorf("%s = %d, want %d", field, *got, *want)
	}
}
