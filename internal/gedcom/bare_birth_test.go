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

// birthBlocks returns the level-1 BIRT and NO BIRT blocks of out, each with
// its substructure lines.
func birthBlocks(out string) []string {
	var blocks []string
	in := false
	for _, line := range strings.Split(out, "\n") {
		switch {
		case line == "1 BIRT" || strings.HasPrefix(line, "1 BIRT ") || strings.HasPrefix(line, "1 NO BIRT"):
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

// A dateless birth life event of a person with a dated record birth is
// exported into the record's BIRT, so a re-import keeps the date and puts the
// event's notes and citations on the record birth (#1714).
func TestRoundTrip_DatelessBirthEventBesideRecordBirth(t *testing.T) {
	tests := []struct {
		name      string
		event     repository.EventReadModel
		wantNotes string // the re-imported record birth notes
	}{
		{name: "with a citation", wantNotes: "Born at home"},
		{name: "with notes", event: repository.EventReadModel{Notes: "Midwife's record"}, wantNotes: "Born at home\n\nMidwife's record"},
		{name: "with a description", event: repository.EventReadModel{Description: "Twin"}, wantNotes: "Born at home\n\nTwin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			rs := memory.NewReadModelStore()
			person := &repository.PersonReadModel{ID: uuid.New(), GivenName: "A", Surname: "B", BirthDateRaw: "1850", BirthNotes: "Born at home", Version: 1}
			if err := rs.SavePerson(ctx, domain.MainBranchID, person); err != nil {
				t.Fatalf("SavePerson: %v", err)
			}
			source := &repository.SourceReadModel{ID: uuid.New(), Title: "Register", Version: 1}
			if err := rs.SaveSource(ctx, domain.MainBranchID, source); err != nil {
				t.Fatalf("SaveSource: %v", err)
			}
			event := tt.event
			event.ID, event.OwnerType, event.OwnerID, event.FactType = uuid.New(), "person", person.ID, domain.FactPersonBirth
			if err := rs.SaveEvent(ctx, domain.MainBranchID, &event); err != nil {
				t.Fatalf("SaveEvent: %v", err)
			}
			citation := &repository.CitationReadModel{ID: uuid.New(), SourceID: source.ID, FactType: domain.FactPersonBirth,
				FactOwnerID: person.ID, FactID: &event.ID, Page: "p. 7", Version: 1}
			if err := rs.SaveCitation(ctx, domain.MainBranchID, citation); err != nil {
				t.Fatalf("SaveCitation: %v", err)
			}

			var buf bytes.Buffer
			if _, err := gedcom.NewExporter(rs).Export(ctx, &buf); err != nil {
				t.Fatalf("Export: %v", err)
			}
			first := buf.String()
			blocks := birthBlocks(first)
			if len(blocks) != 1 || !strings.Contains(blocks[0], "2 DATE 1850") || !strings.Contains(blocks[0], "2 SOUR @") {
				t.Fatalf("birth blocks = %q, want one with the date and the citation; export:\n%s", blocks, first)
			}

			_, persons, _, _, citations, _, events, _, _, _, _, _, _, err := gedcom.NewImporter().Import(ctx, strings.NewReader(first))
			if err != nil {
				t.Fatalf("re-import: %v", err)
			}
			if len(persons) != 1 || persons[0].BirthDate != "1850" || persons[0].BirthNotes != tt.wantNotes {
				t.Fatalf("re-imported persons = %+v, want one born 1850 with notes %q", persons, tt.wantNotes)
			}
			for _, e := range events {
				if e.FactType == domain.FactPersonBirth {
					t.Errorf("re-imported birth life event %+v, want none", e)
				}
			}
			if len(citations) != 1 || citations[0].Page != "p. 7" || citations[0].FactID == nil || *citations[0].FactID != persons[0].ID {
				t.Errorf("re-imported citations = %+v, want one on the record birth %s", citations, persons[0].ID)
			}
			if second := importAndExport(t, first); second != first {
				t.Errorf("second export differs from the first:\n%s\nfirst:\n%s", second, first)
			}
		})
	}
}

// A dateless birth life event the record cannot hold, such as one with a
// TYPE, or a person with no record birth, keeps its own BIRT (#1714).
func TestExport_BirthEventNotMerged(t *testing.T) {
	tests := []struct {
		name   string
		person repository.PersonReadModel
		event  repository.EventReadModel
		want   int // birth blocks
	}{
		{name: "no record birth", want: 1},
		{name: "dated event", person: repository.PersonReadModel{BirthDateRaw: "1850"}, event: repository.EventReadModel{DateRaw: "1851"}, want: 2},
		{name: "event with a TYPE", person: repository.PersonReadModel{BirthDateRaw: "1850"}, event: repository.EventReadModel{TypeDetail: "Stillborn"}, want: 2},
		{name: "negated event", person: repository.PersonReadModel{BirthDateRaw: "1850"}, event: repository.EventReadModel{IsNegated: true}, want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			rs := memory.NewReadModelStore()
			person := tt.person
			person.ID, person.GivenName, person.Surname, person.Version = uuid.New(), "A", "B", 1
			if err := rs.SavePerson(ctx, domain.MainBranchID, &person); err != nil {
				t.Fatalf("SavePerson: %v", err)
			}
			event := tt.event
			event.ID, event.OwnerType, event.OwnerID, event.FactType = uuid.New(), "person", person.ID, domain.FactPersonBirth
			if err := rs.SaveEvent(ctx, domain.MainBranchID, &event); err != nil {
				t.Fatalf("SaveEvent: %v", err)
			}
			var buf bytes.Buffer
			if _, err := gedcom.NewExporter(rs).ExportWithOptions(ctx, &buf, gedcom.ExportOptions{TargetVersion: "7.0"}); err != nil {
				t.Fatalf("Export: %v", err)
			}
			if blocks := birthBlocks(buf.String()); len(blocks) != tt.want {
				t.Errorf("birth blocks = %q, want %d", blocks, tt.want)
			}
		})
	}
}

// Two bare "1 BIRT Y" lines of a person with no record birth are each kept as
// a birth life event, with its own citation (#1714).
func TestImport_TwoBareBirths(t *testing.T) {
	data := "0 HEAD\n1 GEDC\n2 VERS 7.0\n0 @S1@ SOUR\n1 TITL Register\n0 @I1@ INDI\n1 NAME A /B/\n" +
		"1 BIRT Y\n2 SOUR @S1@\n3 PAGE p. 1\n1 BIRT Y\n2 SOUR @S1@\n3 PAGE p. 2\n0 TRLR\n"
	_, persons, _, _, citations, _, events, _, _, _, _, _, _, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(data))
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(persons) != 1 || persons[0].BirthDate != "" {
		t.Fatalf("persons = %+v, want one with no record birth", persons)
	}
	births := map[uuid.UUID]bool{}
	for _, e := range events {
		if e.FactType == domain.FactPersonBirth && !e.IsNegated {
			births[e.ID] = true
		}
	}
	if len(births) != 2 {
		t.Fatalf("birth life events = %d, want 2", len(births))
	}
	pages := map[uuid.UUID]string{}
	for _, c := range citations {
		if c.FactID == nil || !births[*c.FactID] {
			t.Errorf("citation %+v is not on a birth life event", c)
			continue
		}
		pages[*c.FactID] = c.Page
	}
	if len(pages) != 2 {
		t.Errorf("citations by birth life event = %v, want one on each", pages)
	}
}

// The citations of a bare "1 BIRT Y" beside a dated BIRT are on the record
// birth, whichever comes first (#1714).
func TestImport_BareBirthCitationsBesideRecordBirth(t *testing.T) {
	tests := []struct {
		name string
		birt string
	}{
		{name: "after the dated BIRT", birt: "1 BIRT\n2 DATE 1850\n1 BIRT Y\n2 SOUR @S1@\n"},
		{name: "before the dated BIRT", birt: "1 BIRT Y\n2 SOUR @S1@\n1 BIRT\n2 DATE 1850\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := "0 HEAD\n1 GEDC\n2 VERS 7.0\n0 @S1@ SOUR\n1 TITL Register\n0 @I1@ INDI\n1 NAME A /B/\n" + tt.birt + "0 TRLR\n"
			_, persons, _, _, citations, _, _, _, _, _, _, _, _, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(data))
			if err != nil {
				t.Fatalf("Import: %v", err)
			}
			if len(persons) != 1 || persons[0].BirthDate != "1850" {
				t.Fatalf("persons = %+v, want one born 1850", persons)
			}
			if len(citations) != 1 || citations[0].FactID == nil || *citations[0].FactID != persons[0].ID {
				t.Errorf("citations = %+v, want one on the record birth %s", citations, persons[0].ID)
			}
		})
	}
}
