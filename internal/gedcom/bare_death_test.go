package gedcom_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// A DEAT that asserts only that the death happened is kept as a death life
// event, which the living rule counts as death evidence (#1436).
func TestImport_BareDeath(t *testing.T) {
	tests := []struct {
		name          string
		deat          string // the individual's DEAT lines
		wantEvent     bool   // imported as a death life event
		wantDeathDate string // the person record's death date
		wantCause     string // the life event's cause
		wantCitation  bool   // the life event carries the DEAT's citation
	}{
		{name: "DEAT Y", deat: "1 DEAT Y\n", wantEvent: true},
		{name: "DEAT with no payload", deat: "1 DEAT\n", wantEvent: true},
		{name: "DEAT Y with a cause", deat: "1 DEAT Y\n2 CAUS Fever\n", wantEvent: true, wantCause: "Fever"},
		{name: "dated DEAT", deat: "1 DEAT\n2 DATE 1890\n", wantDeathDate: "1890"},
		{name: "DEAT Y with a citation", deat: "1 DEAT Y\n2 SOUR @S1@\n", wantEvent: true, wantCitation: true},
		{name: "DEAT with a citation only", deat: "1 DEAT\n2 SOUR @S1@\n", wantEvent: true, wantCitation: true},
		{name: "DEAT Y after a dated DEAT", deat: "1 DEAT\n2 DATE 1890\n1 DEAT Y\n", wantDeathDate: "1890"},
		{name: "DEAT Y before a dated DEAT", deat: "1 DEAT Y\n1 DEAT\n2 DATE 1890\n", wantDeathDate: "1890"},
		{name: "NO DEAT", deat: "1 NO DEAT\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := "0 HEAD\n1 GEDC\n2 VERS 7.0\n0 @S1@ SOUR\n1 TITL Register\n0 @I1@ INDI\n1 NAME A /B/\n" + tt.deat + "0 TRLR\n"
			_, persons, _, _, citations, _, events, _, _, _, _, _, _, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(data))
			if err != nil {
				t.Fatalf("Import: %v", err)
			}
			if len(persons) != 1 {
				t.Fatalf("persons = %d, want 1", len(persons))
			}
			if got := persons[0].DeathDate; got != tt.wantDeathDate {
				t.Errorf("DeathDate = %q, want %q", got, tt.wantDeathDate)
			}
			var deaths []gedcom.EventData
			for _, e := range events {
				if e.FactType == domain.FactPersonDeath && !e.IsNegated {
					deaths = append(deaths, e)
				}
			}
			if got := len(deaths) == 1; got != tt.wantEvent || len(deaths) > 1 {
				t.Fatalf("death life events = %+v, want one: %v", deaths, tt.wantEvent)
			}
			if tt.wantEvent && deaths[0].Cause != tt.wantCause {
				t.Errorf("Cause = %q, want %q", deaths[0].Cause, tt.wantCause)
			}
			if tt.wantCitation {
				if len(citations) != 1 || citations[0].FactID == nil || *citations[0].FactID != deaths[0].ID {
					t.Errorf("citations = %+v, want one on the death life event %s", citations, deaths[0].ID)
				}
			}
		})
	}
}

// A death life event with no date or place is exported as "1 DEAT Y", which
// imports back as a bare death (#1436).
func TestExport_BareDeathWritesOccurrenceFlag(t *testing.T) {
	tests := []struct {
		name     string
		event    repository.EventReadModel
		wantLine string
	}{
		{name: "bare", wantLine: "1 DEAT Y"},
		{name: "with a cause", event: repository.EventReadModel{Cause: "Fever"}, wantLine: "1 DEAT Y"},
		{name: "dated", event: repository.EventReadModel{DateRaw: "1890"}, wantLine: "1 DEAT"},
		{name: "with a place", event: repository.EventReadModel{Place: "Springfield"}, wantLine: "1 DEAT"},
		{name: "negated", event: repository.EventReadModel{IsNegated: true}, wantLine: "1 NO DEAT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			rs := memory.NewReadModelStore()
			person := &repository.PersonReadModel{ID: uuid.New(), GivenName: "A", Surname: "B", Version: 1}
			if err := rs.SavePerson(ctx, domain.MainBranchID, person); err != nil {
				t.Fatalf("SavePerson: %v", err)
			}
			event := tt.event
			event.ID, event.OwnerType, event.OwnerID, event.FactType = uuid.New(), "person", person.ID, domain.FactPersonDeath
			if err := rs.SaveEvent(ctx, domain.MainBranchID, &event); err != nil {
				t.Fatalf("SaveEvent: %v", err)
			}

			var buf bytes.Buffer
			if _, err := gedcom.NewExporter(rs).ExportWithOptions(ctx, &buf, gedcom.ExportOptions{TargetVersion: "7.0"}); err != nil {
				t.Fatalf("Export: %v", err)
			}
			var deaths []string
			for _, line := range strings.Split(buf.String(), "\n") {
				if strings.HasPrefix(line, "1 DEAT") || strings.HasPrefix(line, "1 NO DEAT") {
					deaths = append(deaths, line)
				}
			}
			if len(deaths) != 1 || deaths[0] != tt.wantLine {
				t.Errorf("death lines = %q, want [%q]; export:\n%s", deaths, tt.wantLine, buf.String())
			}
		})
	}
}
