package domain

import (
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
			name:      "lowercase input",
			input:     "abt 1850",
			wantQual:  DateAbout,
			wantYear:  intPtr(1850),
		},
		{
			name:      "full date with about",
			input:     "ABT 15 MAR 1875",
			wantQual:  DateAbout,
			wantYear:  intPtr(1875),
			wantMonth: intPtr(3),
			wantDay:   intPtr(15),
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

func TestGenDate_String(t *testing.T) {
	tests := []struct {
		name  string
		date  GenDate
		want  string
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

func TestGenDate_Before(t *testing.T) {
	date1 := GenDate{Year: intPtr(1850), Month: intPtr(1), Day: intPtr(1)}
	date2 := GenDate{Year: intPtr(1860), Month: intPtr(1), Day: intPtr(1)}

	if !date1.Before(date2) {
		t.Error("1850 should be before 1860")
	}
	if date2.Before(date1) {
		t.Error("1860 should not be before 1850")
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
