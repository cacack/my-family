package domain

import (
	"testing"
	"time"
)

func TestRestriction_IsPrivate(t *testing.T) {
	tests := []struct {
		restriction Restriction
		want        bool
	}{
		{"", false},
		{"locked", false},
		{"confidential", true},
		{"privacy", true},
		{"CONFIDENTIAL, LOCKED", true},
		{"LOCKED, PRIVACY", true},
		{" locked , confidential ", true},
		{"unknown", false},
	}
	for _, tt := range tests {
		if got := tt.restriction.IsPrivate(); got != tt.want {
			t.Errorf("Restriction(%q).IsPrivate() = %v, want %v", tt.restriction, got, tt.want)
		}
	}
}

func TestRestriction_WithPrivate(t *testing.T) {
	tests := []struct {
		restriction Restriction
		private     bool
		want        Restriction
	}{
		{"", true, "confidential"},
		{"", false, ""},
		{"locked", true, "locked, confidential"},
		{"LOCKED", true, "LOCKED, CONFIDENTIAL"},
		{"confidential", true, "confidential"},
		{"privacy", true, "privacy"},
		{"confidential", false, ""},
		{"CONFIDENTIAL, LOCKED", false, "LOCKED"},
		{"locked, privacy", false, "locked"},
		{"locked", false, "locked"},
	}
	for _, tt := range tests {
		if got := tt.restriction.WithPrivate(tt.private); got != tt.want {
			t.Errorf("Restriction(%q).WithPrivate(%v) = %q, want %q", tt.restriction, tt.private, got, tt.want)
		}
	}
}

func TestLivingOverride_IsValid(t *testing.T) {
	for _, o := range []LivingOverride{LivingOverrideAuto, LivingOverrideLiving, LivingOverrideDeceased} {
		if !o.IsValid() {
			t.Errorf("%q should be valid", o)
		}
	}
	if LivingOverride("auto").IsValid() {
		t.Error(`"auto" is spelled as the empty override, so the word itself is not valid`)
	}
}

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestLatestDate(t *testing.T) {
	tests := []struct {
		date   string
		want   time.Time
		wantOK bool
	}{
		{"1900", day(1900, time.December, 31), true},
		{"FEB 1900", day(1900, time.February, 28), true},
		{"12 MAR 1900", day(1900, time.March, 12), true},
		{"ABT 1900", day(1900, time.December, 31), true},
		{"BEF 1900", day(1900, time.December, 31), true},
		{"BET 1900 AND 1920", day(1920, time.December, 31), true},
		{"FROM 1900 TO 1910", day(1910, time.December, 31), true},
		{"TO 1910", day(1910, time.December, 31), true},
		{"AFT 1900", time.Time{}, false},
		{"FROM 1900", time.Time{}, false},
		{"MAR", time.Time{}, false},
		// Other calendars give the last Gregorian day they can mean.
		{"@#DJULIAN@ 1 JAN 1700", day(1700, time.January, 11), true},
		{"@#DJULIAN@ FEB 1700", day(1700, time.March, 11), true}, // Julian 29 Feb 1700
		{"@#DHEBREW@ 15 NSN 5785", day(2025, time.April, 13), true},
		{"@#DHEBREW@ ELL 5785", day(2025, time.September, 22), true}, // 29 Elul
		{"@#DHEBREW@ 5500", day(1740, time.September, 21), true},
		{"@#DHEBREW@ AFT 5785", time.Time{}, false},
		{"@#DFRENCH R@ 1 VEND 1", day(1792, time.September, 22), true},
		{"@#DFRENCH R@ 1", day(1793, time.September, 21), true},
		{"@#DFRENCH R@ BET 1 VEND 1 AND 1 VEND 2", day(1793, time.September, 22), true},
		{"BET 1790 AND @#DFRENCH R@ 3", day(1795, time.September, 22), true}, // the range end's calendar
		{"44 BC", day(-43, time.December, 31), true},
	}
	for _, tt := range tests {
		gd := ParseGenDate(tt.date)
		got, ok := LatestDate(&gd)
		if ok != tt.wantOK || !got.Equal(tt.want) {
			t.Errorf("LatestDate(%q) = %v, %v; want %v, %v", tt.date, got, ok, tt.want, tt.wantOK)
		}
	}
	if _, ok := LatestDate(nil); ok {
		t.Error("LatestDate(nil) should report no date")
	}
}

func TestLivingReferenceDate(t *testing.T) {
	gd := func(s string) *GenDate { g := ParseGenDate(s); return &g }
	tests := []struct {
		name         string
		birth        *GenDate
		christenings []*GenDate
		want         *time.Time
	}{
		{"no dates", nil, nil, nil},
		{"birth wins", gd("1900"), []*GenDate{gd("1950")}, ptr(day(1900, time.December, 31))},
		{"baptism when no birth", nil, []*GenDate{gd("1901")}, ptr(day(1901, time.December, 31))},
		{"open-ended birth falls back", gd("AFT 1900"), []*GenDate{gd("1905")}, ptr(day(1905, time.December, 31))},
		{"latest christening", nil, []*GenDate{gd("1901"), gd("1903"), gd("AFT 1990")}, ptr(day(1903, time.December, 31))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LivingReferenceDate(tt.birth, tt.christenings)
			if (got == nil) != (tt.want == nil) || (got != nil && !got.Equal(*tt.want)) {
				t.Errorf("LivingReferenceDate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }

func TestLivingCutoff(t *testing.T) {
	now := time.Date(2026, time.November, 19, 15, 4, 5, 0, time.FixedZone("x", -5*3600))
	if got, want := LivingCutoff(now, 110), day(1916, time.November, 19); !got.Equal(want) {
		t.Errorf("LivingCutoff() = %v, want %v", got, want)
	}
}

func TestLivingFacts_IsLiving(t *testing.T) {
	cutoff := day(1916, time.November, 19)
	tests := []struct {
		name  string
		facts LivingFacts
		want  bool
	}{
		{"no facts is living", LivingFacts{}, true},
		{"death evidence", LivingFacts{DeathEvidence: true}, false},
		{"born after cutoff", LivingFacts{ReferenceDate: ptr(day(1950, time.January, 1))}, true},
		{"born on cutoff", LivingFacts{ReferenceDate: ptr(cutoff)}, true},
		{"born the day before cutoff", LivingFacts{ReferenceDate: ptr(cutoff.AddDate(0, 0, -1))}, false},
		{"override living beats death", LivingFacts{Override: LivingOverrideLiving, DeathEvidence: true}, true},
		{"override deceased beats youth", LivingFacts{Override: LivingOverrideDeceased, ReferenceDate: ptr(day(2000, time.January, 1))}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.facts.IsLiving(cutoff); got != tt.want {
				t.Errorf("IsLiving() = %v, want %v", got, tt.want)
			}
		})
	}
}
