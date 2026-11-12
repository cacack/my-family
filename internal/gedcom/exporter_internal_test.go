package gedcom

import (
	"bytes"
	"slices"
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
}

func TestPointerTargets(t *testing.T) {
	var warnings []string
	notes := newPointerTargets("NOTE", &warnings)
	notes.add("@N2@", "@N2@")
	notes.add("@N 3@", "@N5@") // invalid stored xref, replaced
	notes.add("@N4@", "@N7@")  // lost @N4@ to another record type
	notes.add("@N 3@", "@N6@") // duplicate: pointers keep naming the first
	notes.add("", "@N8@")

	if r := notes.remap("@N2@\n\n@N4@\n\ntext", noteJoin); r != "@N2@\n\n@N7@\n\ntext" {
		t.Errorf("remap = %q, want each NOTE pointer moved to its note's export xref", r)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %q, want none for unambiguous pointers", warnings)
	}
	// A pointer shared by two notes resolves to the first and warns once.
	if r := notes.remap("@N 3@", associationNoteJoin); r != "@N5@" {
		t.Errorf("remap duplicate invalid = %q, want the first record's @N5@", r)
	}
	notes.remap("@N 3@", noteJoin)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "@N 3@") {
		t.Errorf("warnings = %q, want one naming the shared xref @N 3@", warnings)
	}
}

func TestPointerTargets_RemapNotesDropsDangling(t *testing.T) {
	var warnings []string
	notes := newPointerTargets("NOTE", &warnings)
	notes.add("@N2@", "@N3@")

	if r := notes.remapNotes("@N9@\n\n@N2@\n\ntext", noteJoin, "INDI @I1@"); r != "@N3@\n\ntext" {
		t.Errorf("remapNotes = %q, want the dangling @N9@ dropped and @N2@ remapped", r)
	}
	if r := notes.remapNotes("@N9@", noteJoin, "FAM @F1@"); r != "" {
		t.Errorf("remapNotes = %q, want empty", r)
	}
	if len(warnings) != 2 || !strings.Contains(warnings[0], "INDI @I1@") || !strings.Contains(warnings[1], "FAM @F1@ points to NOTE @N9@") {
		t.Errorf("warnings = %q, want one per dangling pointer naming record and xref", warnings)
	}
}

func TestSplitNotes(t *testing.T) {
	tests := []struct {
		name, notes   string
		xrefs, inline []string
	}{
		{"empty", "", nil, nil},
		{"text only", "one\n\ntwo", nil, []string{"one\n\ntwo"}},
		{"pointers only", "@N1@\n\n@N2@", []string{"@N1@", "@N2@"}, nil},
		{"pointer then text", "@N1@\n\nsee baptism", []string{"@N1@"}, []string{"see baptism"}},
		// The encoder writes every pointer ahead of the inline notes, so
		// interleaved text cannot keep its place; each run of text between
		// pointers stays its own NOTE rather than merging into one.
		{"interleaved", "one\n\n@N1@\n\ntwo\n\nthree\n\n@N2@\n\nfour",
			[]string{"@N1@", "@N2@"}, []string{"one", "two\n\nthree", "four"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xrefs, inline := splitNotes(tt.notes, noteJoin)
			if !slices.Equal(xrefs, tt.xrefs) || !slices.Equal(inline, tt.inline) {
				t.Errorf("splitNotes(%q) = %q, %q; want %q, %q", tt.notes, xrefs, inline, tt.xrefs, tt.inline)
			}
		})
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
