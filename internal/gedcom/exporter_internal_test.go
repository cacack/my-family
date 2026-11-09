package gedcom

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cacack/gedcom-go/v3/encoder"
	"github.com/cacack/gedcom-go/v3/gedcom"
)

func TestXrefAllocator_Validity(t *testing.T) {
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
		if got := newXrefAllocator(nil).assign(tt.stored, "G", 1); got != tt.want {
			t.Errorf("assign(%q) = %q, want %q", tt.stored, got, tt.want)
		}
	}
}

func TestXrefAllocator_Unique(t *testing.T) {
	// A generated xref skips stored xrefs of any record type and xrefs already
	// given out; a duplicate stored xref is not reused (#1177).
	a := newXrefAllocator([]string{"@N2@", "@I1@", "@N 3@", "@N2@"})
	got := []string{
		a.assign("", "I", 1),     // @I1@ is a stored xref
		a.assign("@N2@", "N", 1), // keeps its stored xref
		a.assign("", "N", 2),     // @N2@ taken
		a.assign("@N2@", "N", 3), // duplicate stored xref
		a.assign("@N 3@", "N", 4),
	}
	want := []string{"@I2@", "@N2@", "@N3@", "@N4@", "@N5@"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("assign #%d = %q, want %q (all: %q)", i, got[i], want[i], got)
		}
	}
	if r := a.remap("@N2@\n\n@N 3@", noteJoin); r != "@N2@\n\n@N5@" {
		t.Errorf("remap = %q, want only the invalid xref replaced", r)
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
