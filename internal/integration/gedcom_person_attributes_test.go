package integration_test

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
)

// Every individual attribute tag in the event log (issue #1287).
//
// DSCR, NATI, CAST, PROP, SSN, IDNO, NCHI, NMR and FACT import as person
// attributes with their value, DATE, PLAC and TYPE, so a rebuild and a replay
// into an empty read model reproduce them, and an export writes each back
// under its own tag. An attribute that is not imported is warned about.

const personAttributesGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME A /B/
1 DSCR Tall, dark hair
2 DATE 1880
1 NATI American
2 PLAC Boston
1 CAST Brahmin
1 PROP Farm of 40 acres
2 DATE 1870
2 PLAC Salem
1 SSN 000-00-0000
1 IDNO 4711
2 TYPE Army serial number
1 NCHI 5
1 NMR 2
1 FACT Left-handed
2 TYPE Handedness
1 OCCU
0 TRLR
`

func TestGedcomImport_PersonAttributesSurviveRebuildAndExport(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			res, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(personAttributesGedcom)})
			if err != nil {
				t.Fatalf("import: %v", err)
			}
			if !slices.ContainsFunc(res.Warnings, func(w string) bool { return strings.Contains(w, "attribute OCCU has no value, not imported") }) {
				t.Errorf("warnings = %q, want one for the empty OCCU", res.Warnings)
			}

			// fact_type|value|date|place|lat|long|type|notes|cites
			want := []string{
				"person_caste|Brahmin|||<nil>|<nil>|||cites",
				"person_children_count|5|||<nil>|<nil>|||cites",
				"person_generic_fact|Left-handed|||<nil>|<nil>|Handedness||cites",
				"person_identification_number|4711|||<nil>|<nil>|Army serial number||cites",
				"person_marriage_count|2|||<nil>|<nil>|||cites",
				"person_nationality|American||Boston|<nil>|<nil>|||cites",
				"person_physical_description|Tall, dark hair|1880||<nil>|<nil>|||cites",
				"person_property|Farm of 40 acres|1870|Salem|<nil>|<nil>|||cites",
				"person_social_security_number|000-00-0000|||<nil>|<nil>|||cites",
			}
			sorted := func(rs repository.ReadModelStore) []string {
				got := attributeSummaries(t, ctx, rs)
				slices.Sort(got)
				return got
			}
			if got := sorted(st.read); !slices.Equal(got, want) {
				t.Fatalf("imported attributes = %q, want %q", got, want)
			}

			// A rebuild finds them in the log.
			if _, err := repository.RebuildReadModel(ctx, st.events, st.read, st.branches, st.snapshots); err != nil {
				t.Fatalf("rebuild: %v", err)
			}
			if got := sorted(st.read); !slices.Equal(got, want) {
				t.Errorf("rebuilt attributes = %q, want %q", got, want)
			}

			// Replay into an empty read model: the values must come from the log.
			fresh := backend.setup(t).read
			replayLog(t, ctx, st.events, fresh)
			if got := sorted(fresh); !slices.Equal(got, want) {
				t.Errorf("replayed attributes = %q, want %q", got, want)
			}

			var buf bytes.Buffer
			if _, err := gedcom.NewExporter(fresh).Export(ctx, &buf); err != nil {
				t.Fatalf("export: %v", err)
			}
			out := buf.String()
			for head, wantLines := range map[string][]string{
				"1 DSCR": {"1 DSCR Tall, dark hair", "2 DATE 1880"},
				"1 NATI": {"1 NATI American", "2 PLAC Boston"},
				"1 CAST": {"1 CAST Brahmin"},
				"1 PROP": {"1 PROP Farm of 40 acres", "2 DATE 1870", "2 PLAC Salem"},
				"1 SSN ": {"1 SSN 000-00-0000"},
				"1 IDNO": {"1 IDNO 4711", "2 TYPE Army serial number"},
				"1 NCHI": {"1 NCHI 5"},
				"1 NMR ": {"1 NMR 2"},
				"1 FACT": {"1 FACT Left-handed", "2 TYPE Handedness"},
				"1 OCCU": nil,
			} {
				if got := attributeLines(out, head); !slices.Equal(got, wantLines) {
					t.Errorf("%s lines = %q, want %q; export:\n%s", head, got, wantLines, out)
				}
			}
		})
	}
}
