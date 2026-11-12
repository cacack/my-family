package gedcom_test

import (
	"strings"
	"testing"
)

// A TYPE under BIRT, DEAT and MARR is kept on import and written back under
// the same event on export (issue #1284).
func TestImportExport_VitalEventTypeRoundTrip(t *testing.T) {
	out := importAndExport(t, `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
1 BIRT
2 TYPE Stillborn
1 DEAT
2 TYPE Infant death
0 @I2@ INDI
1 NAME Mary /Roe/
1 SEX F
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 MARR
2 TYPE Civil
0 TRLR
`)

	for _, want := range []string{
		"1 BIRT\n2 TYPE Stillborn\n",
		"1 DEAT\n2 TYPE Infant death\n",
		"1 MARR\n2 TYPE Civil\n",
	} {
		if !strings.Contains(strings.ReplaceAll(out, "\r\n", "\n"), want) {
			t.Errorf("export lacks %q:\n%s", want, out)
		}
	}
}
