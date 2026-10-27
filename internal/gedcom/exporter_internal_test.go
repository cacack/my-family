package gedcom

import "testing"

func TestExportXref(t *testing.T) {
	tests := []struct {
		stored, want string
	}{
		{"", "@G1@"},
		{"@N1@", "@N1@"},
		{"@N 1@", "@G1@"},
		{"@N1\n@", "@G1@"},
		{"@N1\r\n0 _FORGED@", "@G1@"},
		{"N1", "@G1@"},
		{"@VOID@", "@G1@"},
	}
	for _, tt := range tests {
		if got := exportXref(tt.stored, "@G1@"); got != tt.want {
			t.Errorf("exportXref(%q) = %q, want %q", tt.stored, got, tt.want)
		}
	}
}
