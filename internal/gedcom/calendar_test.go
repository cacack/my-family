package gedcom

import (
	"testing"

	"github.com/cacack/my-family/internal/domain"
)

func TestToGregorian(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantRaw string // Gregorian equivalent
	}{
		{
			name:    "julian to gregorian",
			input:   "@#DJULIAN@ 14 FEB 1689",
			wantRaw: "24 FEB 1689",
		},
		{
			name:    "hebrew passover 5785 to gregorian",
			input:   "@#DHEBREW@ 15 NSN 5785",
			wantRaw: "13 APR 2025",
		},
		{
			name:    "french republican epoch to gregorian",
			input:   "@#DFRENCH R@ 1 VEND 1",
			wantRaw: "22 SEP 1792",
		},
		{
			name:    "year only julian preserves year-only precision",
			input:   "@#DJULIAN@ 1600",
			wantRaw: "1600",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gd := domain.ParseGenDate(tt.input)
			got, err := ToGregorian(gd)
			if err != nil {
				t.Fatalf("ToGregorian() error = %v", err)
			}
			if got.Calendar != domain.CalendarGregorian {
				t.Errorf("Calendar = %q, want %q", got.Calendar, domain.CalendarGregorian)
			}
			if got.Raw != tt.wantRaw {
				t.Errorf("Raw = %q, want %q", got.Raw, tt.wantRaw)
			}
			// Converted dates must be comparable on the Gregorian timeline.
			if got.Year != nil && got.Month != nil && got.Day != nil && got.ToTime().IsZero() {
				t.Errorf("ToTime() unexpectedly zero for converted date %q", got.Raw)
			}
		})
	}
}

func TestToGregorian_GregorianUnchanged(t *testing.T) {
	gd := domain.ParseGenDate("25 DEC 2020")
	got, err := ToGregorian(gd)
	if err != nil {
		t.Fatalf("ToGregorian() error = %v", err)
	}
	if got.Raw != gd.Raw || got.Calendar != gd.Calendar {
		t.Errorf("Gregorian date changed: got %+v, want %+v", got, gd)
	}
}

func TestToGregorian_MissingYear(t *testing.T) {
	gd := domain.GenDate{Calendar: domain.CalendarJulian}
	if _, err := ToGregorian(gd); err == nil {
		t.Error("expected error for date without a year")
	}
}

func TestToGregorian_UnknownCalendar(t *testing.T) {
	gd := domain.GenDate{Calendar: "DBOGUS", Year: intPtrLocal(1600)}
	if _, err := ToGregorian(gd); err == nil {
		t.Error("expected error for unrecognized calendar")
	}
}

func intPtrLocal(i int) *int { return &i }
