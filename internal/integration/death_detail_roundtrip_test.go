package integration_test

import (
	"context"
	"strings"
	"testing"

	gcgedcom "github.com/cacack/gedcom-go/v3/gedcom"

	"github.com/cacack/my-family/internal/domain"
)

// A dated DEAT with a cause and an age survives import, export, import,
// export unchanged, with one death date and one cause (#1455).
func TestGedcomRoundTrip_RecordDeathKeepsCauseAndAge(t *testing.T) {
	const data = "0 HEAD\n1 GEDC\n2 VERS 7.0\n0 @I1@ INDI\n1 NAME Ann /Lee/\n1 DEAT\n2 DATE 1890\n2 CAUS Fever\n2 AGE 70y\n0 TRLR\n"
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			first := backend.setup(t)
			importInto(t, ctx, first, []byte(data))
			export1 := exportAt(t, ctx, first.read, gcgedcom.Version70)

			second := backend.setup(t)
			importInto(t, ctx, second, []byte(export1))
			if export2 := exportAt(t, ctx, second.read, gcgedcom.Version70); export2 != export1 {
				t.Errorf("second export differs from the first:\n%s\nfirst:\n%s", export2, export1)
			}
			if n := strings.Count(export1, "\n1 DEAT"); n != 1 {
				t.Errorf("export has %d DEAT blocks, want 1:\n%s", n, export1)
			}

			ann := findPerson(t, ctx, second.read, "Ann")
			if ann.DeathDateRaw != "1890" {
				t.Errorf("death date = %q, want 1890", ann.DeathDateRaw)
			}
			events, err := second.read.ListEventsForPerson(ctx, domain.MainBranchID, ann.ID)
			if err != nil {
				t.Fatalf("list events: %v", err)
			}
			var causes []string
			for _, e := range events {
				if e.FactType != domain.FactPersonDeath {
					continue
				}
				if e.DateRaw != "" {
					t.Errorf("death life event has date %q, want the record's only", e.DateRaw)
				}
				if e.Cause != "" {
					causes = append(causes, e.Cause)
				}
				if e.Age != "70y" {
					t.Errorf("age = %q, want 70y", e.Age)
				}
			}
			if len(causes) != 1 || causes[0] != "Fever" {
				t.Errorf("causes = %q, want [Fever]", causes)
			}
		})
	}
}
