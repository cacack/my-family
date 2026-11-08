package gedcom

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cacack/gedcom-go/v3/encoder"
	"github.com/cacack/gedcom-go/v3/gedcom"
)

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

// TestEncodeOptions_KeepVendorTags guards the options every export encode
// uses: a vendor "_" tag survives a plain 7.0 and 5.5.1 encode, not only the
// downgrade path (#875). The read model carries no vendor ID to export yet
// (#1175), so this drives the encoder directly.
func TestEncodeOptions_KeepVendorTags(t *testing.T) {
	doc := &gedcom.Document{
		Header: &gedcom.Header{Version: gedcom.Version70},
		Records: []*gedcom.Record{{
			XRef: "@I1@", Type: gedcom.RecordTypeIndividual,
			Entity: &gedcom.Individual{XRef: "@I1@", FamilySearchID: "KWCB-QZ1"},
		}},
	}
	for _, version := range []gedcom.Version{gedcom.Version70, gedcom.Version551} {
		var buf bytes.Buffer
		if err := encoder.EncodeWithOptions(&buf, doc, encodeOptions(version)); err != nil {
			t.Fatalf("encode %s: %v", version, err)
		}
		if !strings.Contains(buf.String(), "1 _FSFTID KWCB-QZ1\n") {
			t.Errorf("%s encode dropped the vendor tag; got:\n%s", version, buf.String())
		}
	}
}
