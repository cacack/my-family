package query_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
)

// A death before birth is an error only when both dates are exact: an ABT,
// CAL or EST date on either side caps it at a warning (gedcom-go#632, #875).
func TestValidation_ApproximateDateOrderingIsWarning(t *testing.T) {
	tests := []struct {
		birth, death, want string
	}{
		{"1900", "1899", "error"},
		{"ABT 1900", "ABT 1899", "warning"},
		{"CAL 1900", "1899", "warning"},
		{"1900", "EST 1899", "warning"},
	}
	for _, tt := range tests {
		t.Run(tt.birth+" / "+tt.death, func(t *testing.T) {
			f := newHealthFixture()
			f.savePerson(t, domain.MainBranchID, uuid.New(), "Ann", tt.birth, tt.death)
			page, err := f.validation.GetValidationIssues(f.ctx, "", 0, 0)
			require.NoError(t, err)
			var got []string
			for _, issue := range page.Issues {
				if issue.Code == "DEATH_BEFORE_BIRTH" {
					got = append(got, issue.Severity)
				}
			}
			assert.Equal(t, []string{tt.want}, got)
		})
	}
}
