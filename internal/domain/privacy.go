package domain

import (
	"slices"
	"strings"
	"time"
)

// Restriction is a record's GEDCOM restriction notice (RESN), kept as
// recorded: one or more of "confidential", "locked" and "privacy", comma
// separated, in whichever case the file used (ADR-008 §2). Persons, families,
// sources, citations, notes and media carry one. The empty restriction is
// none.
type Restriction string

// The RESN values GEDCOM defines.
const (
	RestrictionConfidential = "confidential"
	RestrictionLocked       = "locked"
	RestrictionPrivacy      = "privacy"
)

// values returns the restriction's values, trimmed and lowercased.
func (r Restriction) values() []string {
	var out []string
	for _, v := range strings.Split(string(r), ",") {
		if v = strings.ToLower(strings.TrimSpace(v)); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// IsPrivate reports whether the record is private: its restriction holds
// "confidential" or "privacy". "locked" is an edit lock, not a visibility
// mark (ADR-008 §2).
func (r Restriction) IsPrivate() bool {
	values := r.values()
	return slices.Contains(values, RestrictionConfidential) || slices.Contains(values, RestrictionPrivacy)
}

// WithPrivate returns the restriction with the record marked private or not.
// Marking adds "confidential" (in upper case when the restriction is written
// in upper case, as GEDCOM 7.0 does); unmarking removes "confidential" and
// "privacy". Every other value, "locked" included, is kept as recorded.
func (r Restriction) WithPrivate(private bool) Restriction {
	if r.IsPrivate() == private {
		return r
	}
	if private {
		value := RestrictionConfidential
		if r != "" && string(r) == strings.ToUpper(string(r)) {
			value = strings.ToUpper(value)
		}
		if r == "" {
			return Restriction(value)
		}
		return r + Restriction(", "+value)
	}
	var kept []string
	for _, v := range strings.Split(string(r), ",") {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case RestrictionConfidential, RestrictionPrivacy, "":
		default:
			kept = append(kept, strings.TrimSpace(v))
		}
	}
	return Restriction(strings.Join(kept, ", "))
}

// LivingOverride is an editor's correction of the living rule for one person
// (ADR-008 §1). The empty value is auto: the rule decides.
type LivingOverride string

const (
	LivingOverrideAuto     LivingOverride = ""
	LivingOverrideLiving   LivingOverride = "living"
	LivingOverrideDeceased LivingOverride = "deceased"
)

// IsValid reports whether the value is auto, living or deceased.
func (o LivingOverride) IsValid() bool {
	return o == LivingOverrideAuto || o == LivingOverrideLiving || o == LivingOverrideDeceased
}

// DefaultLivingThresholdYears is N in the living rule: a person born more than
// N years ago is not presumed living. It is a tree setting whose store arrives
// with #1408; until then the default applies (ADR-008 §6).
const DefaultLivingThresholdYears = 110

// LivingFacts are the time-independent inputs of the living rule, which the
// projection stores on each person so that the read-time test is a single
// date comparison (ADR-008 §1).
type LivingFacts struct {
	Override LivingOverride
	// DeathEvidence is set when the person has a death, burial or cremation
	// that is not negated, with or without a date.
	DeathEvidence bool
	// ReferenceDate is the latest date the birth allows, or when there is no
	// birth date the christening or baptism date; nil when there is none.
	ReferenceDate *time.Time
}

// LivingCutoff is the reference date at or after which a person is presumed
// living: midnight UTC, thresholdYears before now's date.
func LivingCutoff(now time.Time, thresholdYears int) time.Time {
	y, m, d := now.UTC().Date()
	return time.Date(y-thresholdYears, m, d, 0, 0, 0, 0, time.UTC)
}

// IsLiving applies the living rule (ADR-008 §1). An override decides when set.
// Otherwise a person with death evidence is deceased, and one whose reference
// date is before cutoff (see LivingCutoff) is presumed deceased. Everyone
// else, including a person with no dates at all, is living: the rule fails
// safe.
func (f LivingFacts) IsLiving(cutoff time.Time) bool {
	switch f.Override {
	case LivingOverrideLiving:
		return true
	case LivingOverrideDeceased:
		return false
	}
	if f.DeathEvidence {
		return false
	}
	return f.ReferenceDate == nil || !f.ReferenceDate.Before(cutoff)
}

// LivingReferenceDate returns the date the living rule compares (ADR-008 §1):
// the latest date birth allows, or when birth gives none, the latest any of
// the christening or baptism dates allows. It is nil when none gives a date.
func LivingReferenceDate(birth *GenDate, christenings []*GenDate) *time.Time {
	if t, ok := LatestDate(birth); ok {
		return &t
	}
	var latest *time.Time
	for _, d := range christenings {
		if t, ok := LatestDate(d); ok && (latest == nil || t.After(*latest)) {
			latest = &t
		}
	}
	return latest
}

// LatestDate returns the last day a date can mean, as midnight UTC on the
// proleptic Gregorian calendar: the end of a range ("BET 1900 AND 1920" is 31
// Dec 1920), the date itself for "BEF 1900", the last day of the year or month
// a partial date names. ok is false for a date without a year and for one with
// no upper bound ("AFT 1900", or a FROM period with no TO), which the living
// rule treats as no date.
func LatestDate(g *GenDate) (time.Time, bool) {
	if g == nil || g.Year == nil {
		return time.Time{}, false
	}
	calendar, year, month, day := g.Calendar, g.Year, g.Month, g.Day
	switch {
	case g.Year2 != nil:
		calendar, year, month, day = g.endCalendar(), g.Year2, g.Month2, g.Day2
	case g.Qualifier == DateAft, g.Qualifier == DateFrom:
		return time.Time{}, false
	}
	_, latest, ok := endpointBounds(calendar, year, month, day)
	if !ok {
		return time.Time{}, false
	}
	return julianDayToTime(latest), true
}

// unixEpochJulianDay is the Julian day number of 1 January 1970.
const unixEpochJulianDay = 2440588

// julianDayToTime returns midnight UTC on the proleptic Gregorian day jdn.
func julianDayToTime(jdn int) time.Time {
	return time.Unix(int64(jdn-unixEpochJulianDay)*24*60*60, 0).UTC()
}
