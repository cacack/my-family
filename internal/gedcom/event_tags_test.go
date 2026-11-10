package gedcom_test

import (
	"strings"
	"testing"
)

// TestEventTags_RoundTrip checks that a named individual event tag, an event's
// TYPE and an EVEN's descriptor survive import and export (issue #1276).
func TestEventTags_RoundTrip(t *testing.T) {
	out := importAndExport(t, `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
1 GRAD
2 DATE 1900
1 BURI
2 TYPE Sea burial
1 EVEN Moved west
2 TYPE Migration
2 DATE 1890
0 @I2@ INDI
1 NAME Mary /Roe/
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 ENGA
2 TYPE Formal
0 TRLR
`)

	for _, want := range []string{
		"1 GRAD\n2 DATE 1900\n",
		"1 BURI\n2 TYPE Sea burial\n",
		"1 EVEN Moved west\n2 DATE 1890\n2 TYPE Migration\n",
		"1 ENGA\n2 TYPE Formal\n",
	} {
		if !strings.Contains(strings.ReplaceAll(out, "\r\n", "\n"), want) {
			t.Errorf("export lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "1 EVEN\n") || strings.Contains(out, "1 EVEN\r\n") {
		t.Errorf("export has a bare 1 EVEN:\n%s", out)
	}
}
