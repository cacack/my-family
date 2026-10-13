package sqlite

import (
	"testing"
	"time"
)

// TestSortDate_RoundTripAndTextOrder proves formatSortDate's text orders like
// the times it encodes (SQLite compares the TEXT columns byte-wise) and that
// parseSortDate reads every value back, BC included (#1065).
func TestSortDate_RoundTripAndTextOrder(t *testing.T) {
	ordered := []time.Time{
		time.Date(-9999, time.January, 1, 0, 0, 0, 0, time.UTC),
		time.Date(-1850, time.January, 1, 0, 0, 0, 0, time.UTC),
		time.Date(-1850, time.December, 31, 0, 0, 0, 0, time.UTC),
		time.Date(-44, time.February, 29, 0, 0, 0, 0, time.UTC), // leap in Go's proleptic calendar
		time.Date(-44, time.March, 15, 0, 0, 0, 0, time.UTC),
		time.Date(-1, time.December, 31, 0, 0, 0, 0, time.UTC),
		time.Date(0, time.June, 1, 0, 0, 0, 0, time.UTC),
		time.Date(10, time.January, 1, 0, 0, 0, 0, time.UTC),
		time.Date(1750, time.January, 1, 0, 0, 0, 0, time.UTC),
		time.Date(1850, time.January, 1, 0, 0, 0, 0, time.UTC),
	}
	for _, layout := range []string{sortDateLayout, time.RFC3339} {
		t.Run(layout, func(t *testing.T) {
			prev := ""
			for _, want := range ordered {
				s := formatSortDate(want, layout)
				if prev != "" && prev >= s {
					t.Errorf("%q (earlier) does not sort before %q (%v)", prev, s, want)
				}
				prev = s

				got, err := parseSortDate(s, layout)
				if err != nil {
					t.Fatalf("parseSortDate(%q): %v", s, err)
				}
				if !got.Equal(want) {
					t.Errorf("parseSortDate(%q) = %v, want %v", s, got, want)
				}
			}
		})
	}
}

func TestFormatSortDate_Encoding(t *testing.T) {
	tests := []struct {
		in     time.Time
		layout string
		want   string
	}{
		{time.Date(1850, 1, 2, 0, 0, 0, 0, time.UTC), sortDateLayout, "1850-01-02"},
		{time.Date(-44, 3, 15, 0, 0, 0, 0, time.UTC), sortDateLayout, "-9956-03-15"},
		{time.Date(-1850, 1, 1, 0, 0, 0, 0, time.UTC), sortDateLayout, "-8150-01-01"},
		{time.Date(-44, 3, 15, 0, 0, 0, 0, time.UTC), time.RFC3339, "-9956-03-15T00:00:00Z"},
		// Before 10000 BC everything sorts as 10000 BC.
		{time.Date(-12000, 1, 1, 0, 0, 0, 0, time.UTC), sortDateLayout, "-0000-01-01"},
	}
	for _, tt := range tests {
		if got := formatSortDate(tt.in, tt.layout); got != tt.want {
			t.Errorf("formatSortDate(%v, %q) = %q, want %q", tt.in, tt.layout, got, tt.want)
		}
	}
}

func TestParseSortDate_Invalid(t *testing.T) {
	for _, s := range []string{"-", "-12", "-abcd-01-01", "-9956-13-01", "not a date"} {
		if _, err := parseSortDate(s, sortDateLayout); err == nil {
			t.Errorf("parseSortDate(%q) succeeded, want an error", s)
		}
	}
}
