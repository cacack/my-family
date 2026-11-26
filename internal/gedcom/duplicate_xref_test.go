package gedcom_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/gedcom"
)

// TestImportDuplicateXrefReportedOnce checks that a redefined xref, which the
// decoder and the validator both find, reaches the import result only once,
// while the validator's other findings still do.
func TestImportDuplicateXrefReportedOnce(t *testing.T) {
	const gedcomData = `0 HEAD
1 GEDC
2 VERS 5.5
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
0 @I1@ INDI
1 NAME Johnny /Doe/
0 @I2@ INDI
1 NAME Kid /Doe/
0 @F1@ FAM
1 CHIL @I2@
0 TRLR
`
	result, _, _, _, _, _, _, _, _, _, _, _, _, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(gedcomData))
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	findings := slices.Concat(result.Warnings, result.Errors)
	if n := countContaining(findings, "DUPLICATE_XREF"); n != 1 {
		t.Errorf("DUPLICATE_XREF findings = %d, want 1; findings: %v", n, findings)
	}
	// The one-way CHIL link is a validator-only finding and must survive.
	if n := countContaining(result.Warnings, "@I2@ in family @F1@"); n != 1 {
		t.Errorf("one-way link warnings for @I2@ = %d, want 1; warnings: %v", n, result.Warnings)
	}
}
