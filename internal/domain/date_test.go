package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestParseGenDate(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantQual  DateQualifier
		wantYear  *int
		wantMonth *int
		wantDay   *int
		wantYear2 *int
	}{
		{
			name:     "empty string",
			input:    "",
			wantQual: "",
		},
		{
			name:      "exact date with day month year",
			input:     "1 JAN 1850",
			wantQual:  DateExact,
			wantYear:  intPtr(1850),
			wantMonth: intPtr(1),
			wantDay:   intPtr(1),
		},
		{
			name:      "month and year only",
			input:     "JAN 1850",
			wantQual:  DateExact,
			wantYear:  intPtr(1850),
			wantMonth: intPtr(1),
		},
		{
			name:     "year only",
			input:    "1850",
			wantQual: DateExact,
			wantYear: intPtr(1850),
		},
		{
			name:     "about date",
			input:    "ABT 1850",
			wantQual: DateAbout,
			wantYear: intPtr(1850),
		},
		{
			name:     "calculated date",
			input:    "CAL 1850",
			wantQual: DateCalc,
			wantYear: intPtr(1850),
		},
		{
			name:     "estimated date",
			input:    "EST 1850",
			wantQual: DateEst,
			wantYear: intPtr(1850),
		},
		{
			name:     "before date",
			input:    "BEF 1850",
			wantQual: DateBef,
			wantYear: intPtr(1850),
		},
		{
			name:     "after date",
			input:    "AFT 1850",
			wantQual: DateAft,
			wantYear: intPtr(1850),
		},
		{
			name:      "between dates",
			input:     "BET 1850 AND 1860",
			wantQual:  DateBet,
			wantYear:  intPtr(1850),
			wantYear2: intPtr(1860),
		},
		{
			name:      "from to dates",
			input:     "FROM 1850 TO 1860",
			wantQual:  DateFrom,
			wantYear:  intPtr(1850),
			wantYear2: intPtr(1860),
		},
		{
			name:     "lowercase input",
			input:    "abt 1850",
			wantQual: DateAbout,
			wantYear: intPtr(1850),
		},
		{
			name:      "full date with about",
			input:     "ABT 15 MAR 1875",
			wantQual:  DateAbout,
			wantYear:  intPtr(1875),
			wantMonth: intPtr(3),
			wantDay:   intPtr(15),
		},
		{
			name:     "interpreted year with phrase",
			input:    "INT 1850 (about eighteen fifty)",
			wantQual: DateInt,
			wantYear: intPtr(1850),
		},
		{
			name:      "interpreted full date with phrase",
			input:     "INT 25 DEC 1850 (Christmas day)",
			wantQual:  DateInt,
			wantYear:  intPtr(1850),
			wantMonth: intPtr(12),
			wantDay:   intPtr(25),
		},
		{
			name:     "interpreted without phrase",
			input:    "INT 1850",
			wantQual: DateInt,
			wantYear: intPtr(1850),
		},
		// Dual years read as the later year (issue #958).
		{
			name:     "dual year",
			input:    "1750/51",
			wantQual: DateExact,
			wantYear: intPtr(1751),
		},
		{
			name:      "full date with dual year",
			input:     "1 JAN 1750/51",
			wantQual:  DateExact,
			wantYear:  intPtr(1751),
			wantMonth: intPtr(1),
			wantDay:   intPtr(1),
		},
		{
			name:     "dual year crossing a century",
			input:    "1699/00",
			wantQual: DateExact,
			wantYear: intPtr(1700),
		},
		{
			name:     "dual year written in full",
			input:    "ABT 1750/1751",
			wantQual: DateAbout,
			wantYear: intPtr(1751),
		},
		// BC years are negative.
		{
			name:      "BC full date",
			input:     "1 JAN 1850 BC",
			wantQual:  DateExact,
			wantYear:  intPtr(-1850),
			wantMonth: intPtr(1),
			wantDay:   intPtr(1),
		},
		{
			name:     "BCE year",
			input:    "44 b.c.e.",
			wantQual: DateExact,
			wantYear: intPtr(-44),
		},
		{
			name:      "range from BC to AD",
			input:     "BET 10 BC AND 10",
			wantQual:  DateBet,
			wantYear:  intPtr(-10),
			wantYear2: intPtr(10),
		},
		// A period end with no start.
		{
			name:     "TO only",
			input:    "TO 1860",
			wantQual: DateTo,
			wantYear: intPtr(1860),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseGenDate(tt.input)

			if got.Qualifier != tt.wantQual {
				t.Errorf("Qualifier = %v, want %v", got.Qualifier, tt.wantQual)
			}
			if !intPtrEqual(got.Year, tt.wantYear) {
				t.Errorf("Year = %v, want %v", ptrStr(got.Year), ptrStr(tt.wantYear))
			}
			if !intPtrEqual(got.Month, tt.wantMonth) {
				t.Errorf("Month = %v, want %v", ptrStr(got.Month), ptrStr(tt.wantMonth))
			}
			if !intPtrEqual(got.Day, tt.wantDay) {
				t.Errorf("Day = %v, want %v", ptrStr(got.Day), ptrStr(tt.wantDay))
			}
			if !intPtrEqual(got.Year2, tt.wantYear2) {
				t.Errorf("Year2 = %v, want %v", ptrStr(got.Year2), ptrStr(tt.wantYear2))
			}
		})
	}
}

func TestParseGenDate_Interpreted(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantFrom string
	}{
		{
			name:     "phrase preserves original casing",
			input:    "INT 1850 (about eighteen fifty)",
			wantFrom: "about eighteen fifty",
		},
		{
			name:     "full date with phrase",
			input:    "INT 25 DEC 1850 (Christmas day)",
			wantFrom: "Christmas day",
		},
		{
			name:     "no parenthetical phrase",
			input:    "INT 1850",
			wantFrom: "",
		},
		{
			name:     "nested parentheses keep inner content",
			input:    "INT 1850 (guessed (best effort))",
			wantFrom: "guessed (best effort)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseGenDate(tt.input)
			if got.Qualifier != DateInt {
				t.Errorf("Qualifier = %v, want %v", got.Qualifier, DateInt)
			}
			if got.InterpretedFrom != tt.wantFrom {
				t.Errorf("InterpretedFrom = %q, want %q", got.InterpretedFrom, tt.wantFrom)
			}
			// Raw must be preserved verbatim for GEDCOM round-trip.
			if got.Raw != tt.input {
				t.Errorf("Raw = %q, want %q", got.Raw, tt.input)
			}
		})
	}
}

func TestGenDate_String(t *testing.T) {
	tests := []struct {
		name string
		date GenDate
		want string
	}{
		{
			name: "preserves raw string",
			date: GenDate{Raw: "ABT 1850", Qualifier: DateAbout, Year: intPtr(1850)},
			want: "ABT 1850",
		},
		{
			name: "formats from components when no raw",
			date: GenDate{Qualifier: DateExact, Year: intPtr(1850), Month: intPtr(1), Day: intPtr(1)},
			want: "1 JAN 1850",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.date.String()
			if got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenDate_IsEmpty(t *testing.T) {
	tests := []struct {
		name string
		date GenDate
		want bool
	}{
		{
			name: "empty date",
			date: GenDate{},
			want: true,
		},
		{
			name: "date with year",
			date: GenDate{Year: intPtr(1850)},
			want: false,
		},
		{
			name: "date with only month",
			date: GenDate{Month: intPtr(1)},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.date.IsEmpty()
			if got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenDate_Validate(t *testing.T) {
	tests := []struct {
		name    string
		date    GenDate
		wantErr bool
	}{
		{
			name: "valid date",
			date: GenDate{Year: intPtr(1850), Month: intPtr(1), Day: intPtr(1)},
		},
		{
			name:    "invalid month",
			date:    GenDate{Year: intPtr(1850), Month: intPtr(13)},
			wantErr: true,
		},
		{
			name:    "invalid day",
			date:    GenDate{Year: intPtr(1850), Month: intPtr(1), Day: intPtr(32)},
			wantErr: true,
		},
		{
			name:    "month zero",
			date:    GenDate{Year: intPtr(1850), Month: intPtr(0)},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.date.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateDate(t *testing.T) {
	tests := []struct {
		input   string
		wantErr string // substring of the error; empty means valid
	}{
		{input: ""},
		{input: "   "},
		{input: "1850"},
		{input: "JAN 1850"},
		{input: "1 jan 1850"},
		{input: "ABT 1850"},
		{input: "BEF 1 JAN 1850"},
		{input: "BET 1850 AND 1860"},
		{input: "FROM 1850 TO 1860"},
		{input: "FROM 1850"},
		{input: "AFT JAN 1850"},
		{input: "EST 1850"},
		{input: "CAL 1850"},
		{input: "INT 1850 (about eighteen fifty)"},
		{input: "(about the time of the war)"},
		{input: "29 FEB 2000"},
		{input: "@#DJULIAN@ 29 FEB 1700"},
		{input: "@#DHEBREW@ 1 TSH 5600"},
		{input: "@#DFRENCH R@ 1 VEND 10"},
		{input: "1750/51"},
		{input: "1 JAN 1750/51"},
		{input: "1 JAN 1850 BC"},
		{input: "TO 1860"},
		{input: "garbage", wantErr: `"garbage" is not a recognized date`},
		{input: "1850-01-01", wantErr: `"1850-01-01" is not a recognized date`},
		{input: "31 FEB 19xx", wantErr: `"31 FEB 19xx" is not a recognized date`},
		{input: "31 FEB", wantErr: "is not a recognized date"},
		{input: "BET 1850", wantErr: "is not a recognized date"},
		{input: "1 January 1850", wantErr: "is not a recognized date"},
		{input: "31 FEB 1900", wantErr: `"31 FEB 1900" is not a real calendar date (February has 28 days in 1900, got day 31)`},
		{input: "29 FEB 1900", wantErr: "is not a real calendar date"},
		{input: "32 JAN 1850", wantErr: "is not a real calendar date"},
		{input: "BET 1850 AND 30 FEB 1860", wantErr: "is not a real calendar date"},
		// The end date's reason reads like a start date's, without gedcom-go's
		// "invalid end date: invalid date: " prefixes (#875).
		{input: "BET 1900 AND 31 FEB 1901", wantErr: `"BET 1900 AND 31 FEB 1901" is not a real calendar date (February has 28 days in 1901, got day 31)`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			err := validateDate(tt.input, tt.input)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("validateDate(%q) = %v, want nil", tt.input, err)
				}
				// An accepted date must be stored with a year (issue #904).
				in := strings.TrimSpace(tt.input)
				if in != "" && !strings.HasPrefix(in, "(") && ParseGenDate(tt.input).Year == nil {
					t.Errorf("ParseGenDate(%q).Year = nil for an accepted date", tt.input)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("validateDate(%q) = %v, want error containing %q", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDate_Unreadable(t *testing.T) {
	tests := []struct {
		input          string
		wantUnreadable bool
	}{
		{input: "garbage", wantUnreadable: true},
		{input: "31 FEB 1900", wantUnreadable: false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var dateErr *DateInputError
			if err := validateDate(tt.input, tt.input); !errors.As(err, &dateErr) {
				t.Fatalf("validateDate(%q) = %v, want *DateInputError", tt.input, err)
			}
			if dateErr.Unreadable != tt.wantUnreadable {
				t.Errorf("Unreadable = %v, want %v", dateErr.Unreadable, tt.wantUnreadable)
			}
		})
	}
}

func TestNormalizeDateInput(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr string // substring of the error; empty means valid
	}{
		// Qualifiers.
		{input: "about 1850", want: "ABT 1850"},
		{input: "About 1850", want: "ABT 1850"},
		{input: "circa 1850", want: "ABT 1850"},
		{input: "c. 1850", want: "ABT 1850"},
		{input: "c 1850", want: "ABT 1850"},
		{input: "ca. 1850", want: "ABT 1850"},
		{input: "ca 1850", want: "ABT 1850"},
		{input: "abt 1850", want: "ABT 1850"},
		{input: "before 1900", want: "BEF 1900"},
		{input: "bef. 1900", want: "BEF 1900"},
		{input: "after 1900", want: "AFT 1900"},
		{input: "est 1850", want: "EST 1850"},
		// Ranges and periods.
		{input: "between 1840 and 1850", want: "BET 1840 AND 1850"},
		{input: "Between March 1840 and 5 June 1850", want: "BET MAR 1840 AND 5 JUN 1850"},
		{input: "from 1840 to 1850", want: "FROM 1840 TO 1850"},
		{input: "from 1840", want: "FROM 1840"},
		{input: "to 1860", want: "TO 1860"},
		// Dual years.
		{input: "about 1750/51", want: "ABT 1750/51"},
		// Month names, full and abbreviated.
		{input: "Sept 1850", want: "SEP 1850"},
		{input: "sept. 1850", want: "SEP 1850"},
		{input: "5 September 1850", want: "5 SEP 1850"},
		{input: "1 January 1850", want: "1 JAN 1850"},
		{input: "Feb. 1850", want: "FEB 1850"},
		{input: "about 12 May 1850", want: "ABT 12 MAY 1850"},
		{input: "before  1  dec  1900", want: "BEF 1 DEC 1900"},
		// BC era markers, in any case.
		{input: "1 Jan 1850 bc", want: "1 JAN 1850 BC"},
		{input: "44 BC", want: "44 BC"},
		{input: "about 44 bce", want: "ABT 44 BCE"},
		{input: "between 50 bc and 10", want: "BET 50 BC AND 10"},
		// GEDCOM input is stored in GEDCOM case.
		{input: "1 jan 1850", want: "1 JAN 1850"},
		{input: "ABT 1850", want: "ABT 1850"},
		// Left as typed: phrases, interpreted dates, calendar escapes, empty.
		{input: "", want: ""},
		{input: "(about the time of the war)", want: "(about the time of the war)"},
		{input: "INT 1850 (about eighteen fifty)", want: "INT 1850 (about eighteen fifty)"},
		{input: "@#DJULIAN@ 29 FEB 1700", want: "@#DJULIAN@ 29 FEB 1700"},
		// Still rejected, naming the actual problem and quoting what was typed.
		{input: "31 February 1900", wantErr: `"31 February 1900" is not a real calendar date (February has 28 days`},
		{input: "garbage", wantErr: `"garbage" is not a recognized date`},
		{input: "Sept 5, 1850", wantErr: `"Sept 5, 1850" is not a recognized date`},
		{input: "September 5 1850", wantErr: `"September 5 1850" is not a recognized date`},
		{input: "1850 and 1860", wantErr: "is not a recognized date"},
		{input: "1840 c", wantErr: "is not a recognized date"},
		{input: "between 1840", wantErr: `"between 1840" is not a recognized date`},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := NormalizeDateInput(tt.input)
			if tt.wantErr == "" {
				if err != nil || got != tt.want {
					t.Errorf("NormalizeDateInput(%q) = %q, %v, want %q, nil", tt.input, got, err, tt.want)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("NormalizeDateInput(%q) error = %v, want error containing %q", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestGenDate_Before(t *testing.T) {
	date1 := GenDate{Year: intPtr(1850), Month: intPtr(1), Day: intPtr(1)}
	date2 := GenDate{Year: intPtr(1860), Month: intPtr(1), Day: intPtr(1)}

	if !date1.Before(&date2) {
		t.Error("1850 should be before 1860")
	}
	if date2.Before(&date1) {
		t.Error("1860 should not be before 1850")
	}
}

func TestGenDate_After(t *testing.T) {
	date1 := GenDate{Year: intPtr(1850), Month: intPtr(1), Day: intPtr(1)}
	date2 := GenDate{Year: intPtr(1860), Month: intPtr(1), Day: intPtr(1)}

	if !date2.After(&date1) {
		t.Error("1860 should be after 1850")
	}
	if date1.After(&date2) {
		t.Error("1850 should not be after 1860")
	}
}

func TestGenDate_Format(t *testing.T) {
	tests := []struct {
		name string
		date GenDate
		want string
	}{
		{
			name: "exact date with day month year",
			date: GenDate{Qualifier: DateExact, Year: intPtr(1850), Month: intPtr(1), Day: intPtr(1)},
			want: "1 JAN 1850",
		},
		{
			name: "about date",
			date: GenDate{Qualifier: DateAbout, Year: intPtr(1850)},
			want: "ABT 1850",
		},
		{
			name: "before date",
			date: GenDate{Qualifier: DateBef, Year: intPtr(1850)},
			want: "BEF 1850",
		},
		{
			name: "after date",
			date: GenDate{Qualifier: DateAft, Year: intPtr(1850)},
			want: "AFT 1850",
		},
		{
			name: "between dates",
			date: GenDate{Qualifier: DateBet, Year: intPtr(1850), Year2: intPtr(1860)},
			want: "BET 1850 AND 1860",
		},
		{
			name: "from to dates",
			date: GenDate{Qualifier: DateFrom, Year: intPtr(1850), Year2: intPtr(1860)},
			want: "FROM 1850 TO 1860",
		},
		{
			name: "interpreted date with phrase",
			date: GenDate{Qualifier: DateInt, Year: intPtr(1850), InterpretedFrom: "about eighteen fifty"},
			want: "INT 1850 (about eighteen fifty)",
		},
		{
			name: "to date",
			date: GenDate{Qualifier: DateTo, Year: intPtr(1860)},
			want: "TO 1860",
		},
		{
			name: "BC date",
			date: GenDate{Qualifier: DateExact, Year: intPtr(-44), Month: intPtr(3), Day: intPtr(15)},
			want: "15 MAR 44 BC",
		},
		{
			name: "interpreted date without phrase",
			date: GenDate{Qualifier: DateInt, Year: intPtr(1850)},
			want: "INT 1850",
		},
		{
			name: "empty date",
			date: GenDate{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.date.Format()
			if got != tt.want {
				t.Errorf("Format() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestParseGenDate_SortOrder checks that dual-year, BC and TO-only dates sort
// among ordinary dates by the year they are read as (issue #958).
func TestParseGenDate_SortOrder(t *testing.T) {
	ordered := []string{"1 JAN 1850 BC", "44 BC", "10", "1750", "1 JAN 1750/51", "FEB 1751", "TO 1860"}
	for i := 1; i < len(ordered); i++ {
		prev, cur := ParseGenDate(ordered[i-1]), ParseGenDate(ordered[i])
		if !prev.Before(&cur) {
			t.Errorf("%q does not sort before %q", ordered[i-1], ordered[i])
		}
	}
}

func TestGenDate_SortDate(t *testing.T) {
	date := GenDate{Year: intPtr(1850), Month: intPtr(1), Day: intPtr(1)}
	sortDate := date.SortDate()
	expected := date.ToTime()

	if !sortDate.Equal(expected) {
		t.Errorf("SortDate() = %v, want %v", sortDate, expected)
	}
}

func TestGenDate_SortDate_BC(t *testing.T) {
	for _, s := range []string{"44 BC", "@#DJULIAN@ 15 MAR 44 BC"} {
		gd := ParseGenDate(s)
		if got := gd.SortDate(); got.Year() >= 1 {
			t.Errorf("SortDate(%q) = %v, want a year before 1 AD", s, got)
		}
	}
}

func TestGenDate_SortDate_ClampsToEarliest(t *testing.T) {
	for _, s := range []string{"5000 BC", "4713 BC"} {
		gd := ParseGenDate(s)
		if got := gd.SortDate(); !got.Equal(EarliestSortDate) {
			t.Errorf("SortDate(%q) = %v, want %v", s, got, EarliestSortDate)
		}
	}
	gd := ParseGenDate("4712 BC")
	if got := gd.SortDate(); !got.Equal(gd.ToTime()) {
		t.Errorf("SortDate(4712 BC) = %v, want %v", got, gd.ToTime())
	}
}

func TestGenDate_ToTime_JulianBC(t *testing.T) {
	gd := ParseGenDate("@#DJULIAN@ 15 MAR 44 BC")
	if got := gd.ToTime().Year(); got >= 0 {
		t.Errorf("ToTime().Year() = %d, want a BC (negative) year", got)
	}
	greg, err := gd.ToGregorian()
	if err != nil {
		t.Fatalf("ToGregorian: %v", err)
	}
	if greg.Year == nil || *greg.Year != -44 {
		t.Errorf("ToGregorian().Year = %v, want -44", greg.Year)
	}
	if !strings.HasSuffix(greg.Raw, "44 BC") {
		t.Errorf("ToGregorian().Raw = %q, want a 44 BC date", greg.Raw)
	}
}

func TestDateQualifier_IsValid(t *testing.T) {
	tests := []struct {
		name      string
		qualifier DateQualifier
		want      bool
	}{
		{
			name:      "exact is valid",
			qualifier: DateExact,
			want:      true,
		},
		{
			name:      "about is valid",
			qualifier: DateAbout,
			want:      true,
		},
		{
			name:      "calculated is valid",
			qualifier: DateCalc,
			want:      true,
		},
		{
			name:      "estimated is valid",
			qualifier: DateEst,
			want:      true,
		},
		{
			name:      "before is valid",
			qualifier: DateBef,
			want:      true,
		},
		{
			name:      "after is valid",
			qualifier: DateAft,
			want:      true,
		},
		{
			name:      "between is valid",
			qualifier: DateBet,
			want:      true,
		},
		{
			name:      "from is valid",
			qualifier: DateFrom,
			want:      true,
		},
		{
			name:      "interpreted is valid",
			qualifier: DateInt,
			want:      true,
		},
		{
			name:      "to is valid",
			qualifier: DateTo,
			want:      true,
		},
		{
			name:      "invalid qualifier",
			qualifier: "invalid",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.qualifier.IsValid(); got != tt.want {
				t.Errorf("DateQualifier(%q).IsValid() = %v, want %v", tt.qualifier, got, tt.want)
			}
		})
	}
}

// Helper functions
func intPtr(i int) *int {
	return &i
}

func intPtrEqual(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func ptrStr(p *int) string {
	if p == nil {
		return "nil"
	}
	return string(rune(*p + '0'))
}
