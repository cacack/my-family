package domain

import "testing"

// checkValueList asserts that IsValid accepts every listed value and rejects
// an unlisted one, so the list is the vocabulary validation enforces.
func checkValueList[T ~string](t *testing.T, values []T, isValid func(T) bool) {
	t.Helper()
	if len(values) == 0 {
		t.Fatal("empty value list")
	}
	for _, v := range values {
		if !isValid(v) {
			t.Errorf("listed value %q is not valid", v)
		}
	}
	if isValid("bogus") {
		t.Error(`unlisted value "bogus" is valid`)
	}
}

func TestValueListsMatchValidation(t *testing.T) {
	t.Run("BranchStatus", func(t *testing.T) {
		checkValueList(t, AllBranchStatuses(), BranchStatus.IsValid)
	})
	t.Run("BranchSubjectType", func(t *testing.T) {
		checkValueList(t, AllBranchSubjectTypes(), BranchSubjectType.IsValid)
	})
	t.Run("BranchOutcome", func(t *testing.T) {
		checkValueList(t, AllBranchOutcomes(), BranchOutcome.IsValid)
	})
	t.Run("BranchCloseOutcome", func(t *testing.T) {
		checkValueList(t, AllBranchCloseOutcomes(), BranchOutcome.IsCloseOutcome)
	})
	t.Run("LDSOrdinanceType", func(t *testing.T) {
		checkValueList(t, AllLDSOrdinanceTypes(), LDSOrdinanceType.IsValid)
	})
	t.Run("LDSOrdinanceStatus", func(t *testing.T) {
		checkValueList(t, AllLDSOrdinanceStatuses(), LDSOrdinanceStatus.IsValid)
		if !LDSOrdinanceStatus("").IsValid() {
			t.Error("an unset status is not valid")
		}
	})
	t.Run("DateQualifier", func(t *testing.T) {
		checkValueList(t, AllDateQualifiers(), DateQualifier.IsValid)
	})
	t.Run("ResearchLogSubjectType", func(t *testing.T) {
		checkValueList(t, AllResearchLogSubjectTypes(), ResearchLogSubjectType.IsValid)
	})
}
