package gedcom_test

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
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
		// One person, one death (#1456).
		{name: "two DEAT Y", deat: "1 DEAT Y\n1 DEAT Y\n", wantEvent: true},
		{name: "two DEAT Y, a cause and a citation", deat: "1 DEAT Y\n2 CAUS Fever\n1 DEAT Y\n2 SOUR @S1@\n", wantEvent: true, wantCause: "Fever", wantCitation: true},
		{name: "two DEAT Y with the same cause", deat: "1 DEAT Y\n2 CAUS Fever\n1 DEAT Y\n2 CAUS Fever\n", wantEvent: true, wantCause: "Fever"},
		{name: "DEAT Y with a cause after a dated DEAT", deat: "1 DEAT\n2 DATE 1890\n1 DEAT Y\n2 CAUS Fever\n", wantEvent: true, wantDeathDate: "1890", wantCause: "Fever"},
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

// The cause, age and address of a DEAT the person record holds are kept on a
// dateless, placeless death life event, and exported back into that DEAT
// (#1455).
func TestImport_RecordDeathKeepsCauseAndAge(t *testing.T) {
	tests := []struct {
		name      string
		deat      string
		wantEvent bool
		wantCause string
		wantAge   string
		wantAddr  string
	}{
		{name: "dated with cause and age", deat: "1 DEAT\n2 DATE 1890\n2 CAUS Fever\n2 AGE 70y\n", wantEvent: true, wantCause: "Fever", wantAge: "70y"},
		{name: "placed with a cause", deat: "1 DEAT\n2 PLAC Springfield\n2 CAUS Fever\n", wantEvent: true, wantCause: "Fever"},
		{name: "dated with an address", deat: "1 DEAT\n2 DATE 1890\n2 ADDR 1 Main St\n", wantEvent: true, wantAddr: "1 Main St"},
		{name: "dated only", deat: "1 DEAT\n2 DATE 1890\n"},
		{name: "cause on an earlier DEAT the record does not hold", deat: "1 DEAT\n2 DATE 1880\n2 CAUS Fever\n1 DEAT\n2 DATE 1890\n"},
		{name: "cause on the last of two DEATs", deat: "1 DEAT\n2 DATE 1880\n1 DEAT\n2 DATE 1890\n2 CAUS Fever\n", wantEvent: true, wantCause: "Fever"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := "0 HEAD\n1 GEDC\n2 VERS 7.0\n0 @S1@ SOUR\n1 TITL Register\n0 @I1@ INDI\n1 NAME A /B/\n" + tt.deat + "0 TRLR\n"
			_, persons, _, _, _, _, events, _, _, _, _, _, _, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(data))
			if err != nil {
				t.Fatalf("Import: %v", err)
			}
			if len(persons) != 1 || persons[0].DeathDate == "" && persons[0].DeathPlace == "" {
				t.Fatalf("persons = %+v, want one with a record death", persons)
			}
			var deaths []gedcom.EventData
			for _, e := range events {
				if e.FactType == domain.FactPersonDeath {
					deaths = append(deaths, e)
				}
			}
			if !tt.wantEvent {
				if len(deaths) != 0 {
					t.Fatalf("death life events = %+v, want none", deaths)
				}
				return
			}
			if len(deaths) != 1 {
				t.Fatalf("death life events = %+v, want one", deaths)
			}
			d := deaths[0]
			if d.Date != "" || d.Place != "" || d.IsNegated || d.Cause != tt.wantCause || d.Age != tt.wantAge {
				t.Errorf("death life event = %+v, want no date or place, cause %q, age %q", d, tt.wantCause, tt.wantAge)
			}
			if gotAddr := ""; d.Address != nil {
				gotAddr = d.Address.Line1
				if gotAddr != tt.wantAddr {
					t.Errorf("address = %q, want %q", gotAddr, tt.wantAddr)
				}
			} else if tt.wantAddr != "" {
				t.Errorf("address = nil, want %q", tt.wantAddr)
			}
		})
	}
}

// deathBlocks returns each "1 DEAT" or "1 NO DEAT" block of out, its lines
// joined by "\n".
func deathBlocks(out string) []string {
	var blocks []string
	in := false
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "1 DEAT") || strings.HasPrefix(line, "1 NO DEAT"):
			blocks = append(blocks, line)
			in = true
		case in && line != "" && line[0] > '1':
			blocks[len(blocks)-1] += "\n" + line
		default:
			in = false
		}
	}
	return blocks
}

func TestExport_RecordDeathMergesCauseAndAge(t *testing.T) {
	data := "0 HEAD\n1 GEDC\n2 VERS 7.0\n0 @S1@ SOUR\n1 TITL Register\n0 @I1@ INDI\n1 NAME A /B/\n" +
		"1 DEAT\n2 DATE 1890\n2 CAUS Fever\n2 AGE 70y\n2 SOUR @S1@\n0 TRLR\n"
	first := importAndExport(t, data)
	blocks := deathBlocks(first)
	if len(blocks) != 1 {
		t.Fatalf("death blocks = %q, want one; export:\n%s", blocks, first)
	}
	for _, want := range []string{"2 DATE 1890", "2 CAUS Fever", "2 AGE 70y", "2 SOUR @S"} {
		if !strings.Contains(blocks[0]+"\n", want) {
			t.Errorf("DEAT block %q lacks %q", blocks[0], want)
		}
	}
	if strings.Count(blocks[0], "2 SOUR") != 1 {
		t.Errorf("DEAT block %q, want its citation once", blocks[0])
	}
	if second := importAndExport(t, first); second != first {
		t.Errorf("second export differs from the first:\n%s\nfirst:\n%s", second, first)
	}
}

// A record death keeps a death life event that has more than a cause, age or
// address, or one of several, as a DEAT of its own.
func TestExport_RecordDeathKeepsOtherDeathEvents(t *testing.T) {
	tests := []struct {
		name   string
		events []repository.EventReadModel
		want   int // death blocks
	}{
		{name: "merged", events: []repository.EventReadModel{{Cause: "Fever"}}, want: 1},
		{name: "with notes", events: []repository.EventReadModel{{Cause: "Fever", Notes: "n"}}, want: 2},
		{name: "dated", events: []repository.EventReadModel{{Cause: "Fever", DateRaw: "1891"}}, want: 2},
		{name: "two", events: []repository.EventReadModel{{Cause: "Fever"}, {Age: "70y"}}, want: 3},
		{name: "negated", events: []repository.EventReadModel{{IsNegated: true}}, want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			rs := memory.NewReadModelStore()
			person := &repository.PersonReadModel{ID: uuid.New(), GivenName: "A", Surname: "B", DeathDateRaw: "1890", Version: 1}
			if err := rs.SavePerson(ctx, domain.MainBranchID, person); err != nil {
				t.Fatalf("SavePerson: %v", err)
			}
			for _, event := range tt.events {
				event.ID, event.OwnerType, event.OwnerID, event.FactType = uuid.New(), "person", person.ID, domain.FactPersonDeath
				if err := rs.SaveEvent(ctx, domain.MainBranchID, &event); err != nil {
					t.Fatalf("SaveEvent: %v", err)
				}
			}
			var buf bytes.Buffer
			if _, err := gedcom.NewExporter(rs).ExportWithOptions(ctx, &buf, gedcom.ExportOptions{TargetVersion: "7.0"}); err != nil {
				t.Fatalf("Export: %v", err)
			}
			if blocks := deathBlocks(buf.String()); len(blocks) != tt.want {
				t.Errorf("death blocks = %q, want %d", blocks, tt.want)
			}
		})
	}
}

// Bare deaths whose details conflict are each kept, so neither is dropped
// (#1456).
func TestImport_BareDeathsWithConflictingDetails(t *testing.T) {
	tests := []struct {
		name       string
		deat       string
		wantCauses []string
	}{
		{name: "two causes", deat: "1 DEAT Y\n2 CAUS Fever\n1 DEAT Y\n2 CAUS Cholera\n", wantCauses: []string{"Fever", "Cholera"}},
		{name: "a cause that conflicts with the record's", deat: "1 DEAT\n2 DATE 1890\n2 CAUS Fever\n1 DEAT Y\n2 CAUS Cholera\n1 DEAT Y\n2 AGE 70y\n", wantCauses: []string{"Fever", "Cholera"}},
		{name: "a descriptor", deat: "1 DEAT Y\n2 CAUS Fever\n1 DEAT Drowned\n", wantCauses: []string{"Fever", ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := "0 HEAD\n1 GEDC\n2 VERS 7.0\n0 @I1@ INDI\n1 NAME A /B/\n" + tt.deat + "0 TRLR\n"
			_, _, _, _, _, _, events, _, _, _, _, _, _, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(data))
			if err != nil {
				t.Fatalf("Import: %v", err)
			}
			var causes []string
			for _, e := range events {
				if e.FactType == domain.FactPersonDeath {
					causes = append(causes, e.Cause)
				}
			}
			if !slices.Equal(causes, tt.wantCauses) {
				t.Errorf("death life event causes = %q, want %q", causes, tt.wantCauses)
			}
		})
	}
}

// A file that records a death's date and its cause in separate DEATs exports
// one DEAT with both (#1456).
func TestExport_DatedAndBareDeathWriteOneDeat(t *testing.T) {
	data := "0 HEAD\n1 GEDC\n2 VERS 7.0\n0 @I1@ INDI\n1 NAME A /B/\n" +
		"1 DEAT\n2 DATE 1890\n1 DEAT Y\n2 CAUS Fever\n0 TRLR\n"
	out := importAndExport(t, data)
	blocks := deathBlocks(out)
	if len(blocks) != 1 || !strings.Contains(blocks[0], "2 DATE 1890") || !strings.Contains(blocks[0], "2 CAUS Fever") {
		t.Errorf("death blocks = %q, want one with the date and the cause; export:\n%s", blocks, out)
	}
}

// A bare death imported with a cause, age and address, whose person is later
// given a death date, keeps all three through an export and a re-import
// (#1456).
func TestRoundTrip_RecordDeathAddedAfterBareDeath(t *testing.T) {
	ctx := context.Background()
	data := "0 HEAD\n1 GEDC\n2 VERS 7.0\n0 @I1@ INDI\n1 NAME A /B/\n" +
		"1 DEAT Y\n2 CAUS Fever\n2 AGE 70y\n2 ADDR 1 Main St\n0 TRLR\n"
	rs := memory.NewReadModelStore()
	h := command.NewHandler(memory.NewEventStore(), rs)
	if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(data)}); err != nil {
		t.Fatalf("import: %v", err)
	}
	persons, _, err := rs.ListPersons(ctx, repository.ListOptions{Limit: 10})
	if err != nil || len(persons) != 1 {
		t.Fatalf("ListPersons = %+v, %v; want one", persons, err)
	}
	if _, err := h.UpdatePerson(ctx, command.UpdatePersonInput{ID: persons[0].ID, DeathDate: command.Set("1890"), Version: persons[0].Version}); err != nil {
		t.Fatalf("UpdatePerson: %v", err)
	}
	var buf bytes.Buffer
	if _, err := gedcom.NewExporter(rs).Export(ctx, &buf); err != nil {
		t.Fatalf("export: %v", err)
	}
	first := buf.String()
	if blocks := deathBlocks(first); len(blocks) != 1 {
		t.Errorf("death blocks = %q, want one", blocks)
	}

	_, persons2, _, _, _, _, events, _, _, _, _, _, _, err := gedcom.NewImporter().Import(ctx, strings.NewReader(first))
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if len(persons2) != 1 || persons2[0].DeathDate != "1890" {
		t.Fatalf("re-imported persons = %+v, want one who died 1890", persons2)
	}
	var deaths []gedcom.EventData
	for _, e := range events {
		if e.FactType == domain.FactPersonDeath {
			deaths = append(deaths, e)
		}
	}
	if len(deaths) != 1 || deaths[0].Cause != "Fever" || deaths[0].Age != "70y" ||
		deaths[0].Address == nil || deaths[0].Address.Line1 != "1 Main St" {
		t.Errorf("re-imported death life events = %+v, want one with cause, age and address", deaths)
	}
	if second := importAndExport(t, first); second != first {
		t.Errorf("second export differs from the first:\n%s\nfirst:\n%s", second, first)
	}
}
