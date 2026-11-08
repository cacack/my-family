package query

import (
	"testing"

	"github.com/cacack/gedcom-go/v3/gedcom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Ages the validator computes across BC and AD count no year 0: 10 BC to
// AD 10 is 19 years (#1066).
func TestValidatorDate_YearsAcrossBCAndAD(t *testing.T) {
	tests := []struct {
		from, to string
		want     int
	}{
		{"10 BC", "10", 19},
		{"1 JAN 10 BC", "1 JAN 10", 19},
		{"2 BC", "1", 2},
		{"44 BC", "10 BC", 34},
		{"1850", "1920", 70},
	}
	for _, tt := range tests {
		t.Run(tt.from+" to "+tt.to, func(t *testing.T) {
			years, _, err := gedcom.YearsBetween(validatorDate(tt.from), validatorDate(tt.to))
			require.NoError(t, err)
			assert.Equal(t, tt.want, years)
		})
	}
}

func TestValidatorDate(t *testing.T) {
	d := validatorDate("15 MAR 44 BC")
	require.NotNil(t, d)
	assert.Equal(t, gedcom.Date{Original: "15 MAR 44 BC", Year: 44, Month: 3, Day: 15, IsBC: true}, *d)

	assert.True(t, validatorDate("10 BC").IsBefore(validatorDate("10")))
	assert.True(t, validatorDate("44 BC").IsBefore(validatorDate("10 BC")))

	assert.Nil(t, validatorDate("(about the time of the war)"))
	assert.Nil(t, validatorDate(""))
}
