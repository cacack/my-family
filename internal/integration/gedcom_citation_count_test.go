package integration_test

import (
	"bytes"
	"context"
	"os"
	"regexp"
	"testing"

	"github.com/cacack/my-family/internal/gedcom"
)

// sourceCitationLine matches a source citation structure: a SOUR pointer at
// level 2 or deeper.
var sourceCitationLine = regexp.MustCompile(`(?m)^([2-9]|\d{2,}) SOUR @[^@]+@$`)

// An export's CitationsExported equals the source citations it writes, for
// each comprehensive fixture (#1354). Each export writes the citations its
// fixture has.
func TestGedcomExport_CitationsExportedMatchesFile(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			for _, fixture := range roundTripFixtures {
				t.Run(fixture.name, func(t *testing.T) {
					ctx := context.Background()
					data, err := os.ReadFile(fixture.path)
					if err != nil {
						t.Fatalf("read fixture: %v", err)
					}
					st := backend.setup(t)
					importInto(t, ctx, st, data)

					var buf bytes.Buffer
					result, err := gedcom.NewExporter(st.read).ExportWithOptions(ctx, &buf, gedcom.ExportOptions{TargetVersion: fixture.version})
					if err != nil {
						t.Fatalf("export: %v", err)
					}
					// The 5.5 fixture has no citations, the 7.0 one has three.
					written := len(sourceCitationLine.FindAllString(buf.String(), -1))
					if want := len(sourceCitationLine.FindAll(data, -1)); written != want {
						t.Fatalf("precondition: export wrote %d citation lines, the fixture has %d", written, want)
					}
					if result.CitationsExported != written {
						t.Errorf("CitationsExported = %d, want %d (the citation lines written)", result.CitationsExported, written)
					}
				})
			}
		})
	}
}
