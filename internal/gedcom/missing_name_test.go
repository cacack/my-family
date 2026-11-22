package gedcom_test

import (
	"strings"
	"testing"
)

// A missing given name or a missing NAME survives export and re-import without
// an invented name (#1054).
func TestMissingName_RoundTrip(t *testing.T) {
	const data = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME /Tudor/
0 @I2@ INDI
1 SEX M
0 TRLR
`
	first := importAndExport(t, data)
	for i, out := range []string{first, importAndExport(t, first)} {
		var nameLines []string
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "1 NAME") {
				nameLines = append(nameLines, line)
			}
		}
		if len(nameLines) != 1 || nameLines[0] != "1 NAME /Tudor/" {
			t.Errorf("export %d: NAME lines = %q, want only \"1 NAME /Tudor/\"; export:\n%s", i+1, nameLines, out)
		}
		if strings.Contains(out, "Unknown") {
			t.Errorf("export %d invents the name Unknown; export:\n%s", i+1, out)
		}
	}
}
