package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// orderAndCount orders every list for review and counts the validation issues
// by severity, whatever order the findings were collected in (#838).
func TestBranchHealth_OrderAndCount(t *testing.T) {
	issue := func(key, severity, code, record string) BranchValidationIssue {
		return BranchValidationIssue{
			Key:                   key,
			ValidationIssueResult: ValidationIssueResult{Severity: severity, Code: code},
			RecordName:            record,
		}
	}
	quality := func(key, name string) BranchQualityIssue {
		return BranchQualityIssue{Key: key, PersonName: name}
	}
	duplicate := func(key string, confidence float64) BranchDuplicate {
		return BranchDuplicate{Key: key, DuplicateResult: DuplicateResult{Confidence: confidence}}
	}

	health := &BranchHealth{
		ValidationIssues: []BranchValidationIssue{
			issue("k9", "info", "NO_SOURCES", "Ada"),
			issue("k8", "warning", "IMPOSSIBLE_AGE", "Bea"),
			issue("k7", "error", "DEATH_BEFORE_BIRTH", "Bea"),
			issue("k6", "warning", "IMPOSSIBLE_AGE", "Ada"),
			issue("k5", "error", "CHILD_BEFORE_PARENT", "Cal"),
			issue("k4", "error", "DEATH_BEFORE_BIRTH", "Ada"),
			issue("k2", "error", "DEATH_BEFORE_BIRTH", "Ada"),
			issue("k3", "custom", "OTHER", "Ada"),
		},
		QualityIssues: []BranchQualityIssue{
			quality("q3", "Bea"), quality("q2", "Ada"), quality("q1", "Ada"),
		},
		Duplicates: []BranchDuplicate{
			duplicate("d3", 0.7), duplicate("d2", 0.9), duplicate("d1", 0.7),
		},
	}
	health.orderAndCount()

	var keys []string
	for _, i := range health.ValidationIssues {
		keys = append(keys, i.Key)
	}
	// Errors (by code, then record name, then key), then warnings (by record
	// name), then everything else as information (by code).
	assert.Equal(t, []string{"k5", "k2", "k4", "k7", "k6", "k8", "k9", "k3"}, keys)
	assert.Equal(t, 4, health.ErrorCount)
	assert.Equal(t, 2, health.WarningCount)
	assert.Equal(t, 2, health.InfoCount, "an unknown severity counts as information")

	keys = nil
	for _, q := range health.QualityIssues {
		keys = append(keys, q.Key)
	}
	assert.Equal(t, []string{"q1", "q2", "q3"}, keys, "by person name, then key")

	keys = nil
	for _, d := range health.Duplicates {
		keys = append(keys, d.Key)
	}
	assert.Equal(t, []string{"d2", "d1", "d3"}, keys, "most confident first, then key")
}
