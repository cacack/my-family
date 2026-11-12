package integration_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	gcgedcom "github.com/cacack/gedcom-go/v3/gedcom"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
)

// Address text survives an import → export → import round trip (issue #1291).
//
// Since gedcom-go v3.6.0 the ADDR payload, the only address text in GEDCOM
// 7.0 and the common case in 5.x, is in Address.Full, and Line1-Line3 hold
// only ADR1-ADR3. Ann's residence and the repository give their address only
// as the payload, Bob's residence only as ADR1.

const addressGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Ann /Doe/
1 RESI
2 DATE 1880
2 ADDR 12 Mill St
3 CONT Apt 4
3 CITY Lowell
0 @I2@ INDI
1 NAME Bob /Doe/
1 RESI
2 DATE 1890
2 ADDR
3 ADR1 5 Elm St
3 CITY Boston
0 @R1@ REPO
1 NAME County Archive
1 ADDR 1 Archive Way
2 CITY Salem
0 TRLR
`

func TestGedcomImport_AddressTextSurvivesRoundTrip(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			first := importAndReplay(t, ctx, backend, addressGedcom)
			assertImportedAddresses(t, ctx, first)

			for _, version := range []gcgedcom.Version{gcgedcom.Version551, gcgedcom.Version70} {
				t.Run(string(version), func(t *testing.T) {
					var buf bytes.Buffer
					if _, err := gedcom.NewExporter(first.read).ExportWithOptions(ctx, &buf, gedcom.ExportOptions{TargetVersion: version}); err != nil {
						t.Fatalf("export: %v", err)
					}
					out := buf.String()
					// The text is written as the ADDR payload, not as ADR1
					// under an empty ADDR.
					for _, want := range []string{"2 ADDR 12 Mill St\n3 CONT Apt 4\n", "2 ADDR 5 Elm St\n", "1 ADDR 1 Archive Way\n"} {
						if !strings.Contains(out, want) {
							t.Errorf("export lacks %q:\n%s", want, out)
						}
					}
					if strings.Contains(out, "ADR1") {
						t.Errorf("export writes ADR1:\n%s", out)
					}

					assertImportedAddresses(t, ctx, importAndReplay(t, ctx, backend, out))
				})
			}
		})
	}
}

// importAndReplay imports text into a new store of backend, then replays the
// log into an empty read model, so what it returns comes from the events.
func importAndReplay(t *testing.T, ctx context.Context, backend backend, text string) stores {
	t.Helper()
	st := backend.setup(t)
	h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
	if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(text)}); err != nil {
		t.Fatalf("import: %v", err)
	}
	fresh := backend.setup(t)
	replayLog(t, ctx, st.events, fresh.read)
	return fresh
}

func assertImportedAddresses(t *testing.T, ctx context.Context, st stores) {
	t.Helper()
	for _, tc := range []struct {
		given string
		want  domain.Address
	}{
		{"Ann", domain.Address{Line1: "12 Mill St", Line2: "Apt 4", City: "Lowell"}},
		{"Bob", domain.Address{Line1: "5 Elm St", City: "Boston"}},
	} {
		person := findPerson(t, ctx, st.read, tc.given)
		events, err := st.read.ListEventsForPerson(ctx, domain.MainBranchID, person.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) != 1 || events[0].Address == nil || *events[0].Address != tc.want {
			t.Errorf("%s's residence = %+v, want address %+v", tc.given, events, tc.want)
		}
	}

	repos, _, err := st.read.ListRepositories(ctx, repository.ListOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	want := domain.Address{Line1: "1 Archive Way", City: "Salem"}
	if len(repos) != 1 || repos[0].Address == nil || *repos[0].Address != want {
		t.Errorf("repositories = %+v, want one with address %+v", repos, want)
	}
}
