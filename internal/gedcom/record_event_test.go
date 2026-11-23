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

// importLifeEvents imports data and returns its one person and the person's
// life events of factType.
func importLifeEvents(t *testing.T, data string, factType domain.FactType) (gedcom.PersonData, []gedcom.EventData) {
	t.Helper()
	_, persons, _, _, _, _, events, _, _, _, _, _, _, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(data))
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(persons) != 1 {
		t.Fatalf("persons = %+v, want one", persons)
	}
	var matched []gedcom.EventData
	for _, e := range events {
		if e.FactType == factType {
			matched = append(matched, e)
		}
	}
	return persons[0], matched
}

// A later DEAT the record could hold does not replace the dated record death:
// the record keeps its date and descriptor, and the later one is a death life
// event, through a round trip with a stable export (#1766).
func TestRoundTrip_RecordDeathBesideLaterDeath(t *testing.T) {
	data := "0 HEAD\n1 GEDC\n2 VERS 7.0\n0 @I1@ INDI\n1 NAME A /B/\n" +
		"1 DEAT Drowned\n2 DATE 1890\n2 CAUS Fever\n1 DEAT Shot\n0 TRLR\n"
	first := importAndExport(t, data)

	person, deaths := importLifeEvents(t, first, domain.FactPersonDeath)
	if person.DeathDate != "1890" {
		t.Fatalf("re-imported person = %+v, want death date 1890; export:\n%s", person, first)
	}
	// The two descriptors conflict, so the exporter writes one into the
	// record's DEAT and the other into a "1 DEAT Y", whichever event it
	// lists first: each ends up in the death notes or on a death event.
	kept := map[string]bool{person.DeathNotes: true}
	var cause bool
	for _, e := range deaths {
		kept[e.Notes], kept[e.Description] = true, true
		cause = cause || e.Cause == "Fever"
	}
	if !cause || !kept["Drowned"] || !kept["Shot"] {
		t.Errorf("re-imported death notes %q, death events %+v; want cause %q, %q and %q kept; export:\n%s",
			person.DeathNotes, deaths, "Fever", "Drowned", "Shot", first)
	}
	if second := importAndExport(t, first); second != first {
		t.Errorf("second export differs from the first:\n%s\nfirst:\n%s", second, first)
	}
}

// A later undated BIRT with only a descriptor or a note does not replace the
// dated record birth: it is a birth life event, which the exporter writes
// into the record's BIRT, so a re-import has it in the birth notes (#1766).
func TestRoundTrip_RecordBirthBesideLaterBirth(t *testing.T) {
	tests := []struct {
		name  string
		later string
		want  gedcom.EventData // the later birth's life event
		text  string
	}{
		{name: "descriptor", later: "1 BIRT Other\n", want: gedcom.EventData{Description: "Other"}, text: "Other"},
		{name: "note", later: "1 BIRT\n2 NOTE x\n", want: gedcom.EventData{Notes: "x"}, text: "x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := "0 HEAD\n1 GEDC\n2 VERS 7.0\n0 @I1@ INDI\n1 NAME A /B/\n1 BIRT\n2 DATE 1890\n" + tt.later + "0 TRLR\n"
			person, births := importLifeEvents(t, data, domain.FactPersonBirth)
			if person.BirthDate != "1890" || person.BirthNotes != "" {
				t.Fatalf("person = %+v, want birth date 1890 and no birth notes", person)
			}
			if len(births) != 1 || births[0].Description != tt.want.Description || births[0].Notes != tt.want.Notes || births[0].Date != "" {
				t.Fatalf("birth life events = %+v, want one with description %q and notes %q", births, tt.want.Description, tt.want.Notes)
			}

			first := importAndExport(t, data)
			person, births = importLifeEvents(t, first, domain.FactPersonBirth)
			if person.BirthDate != "1890" || person.BirthNotes != tt.text || len(births) != 0 {
				t.Fatalf("re-imported person = %+v, births %+v, want birth date 1890, birth notes %q and no birth life event; export:\n%s",
					person, births, tt.text, first)
			}
			if second := importAndExport(t, first); second != first {
				t.Errorf("second export differs from the first:\n%s\nfirst:\n%s", second, first)
			}
		})
	}
}

// The record holds the first dated BIRT or DEAT, whatever comes before it,
// and each other one the record could hold is a life event with its date,
// place, notes and cause (#1766).
func TestImport_RecordEventIsFirstDated(t *testing.T) {
	tests := []struct {
		name      string
		events    string
		factType  domain.FactType
		wantDate  string // the record's
		wantNotes string // the record's
		want      gedcom.EventData
	}{
		{name: "undated birth first", events: "1 BIRT\n2 NOTE x\n1 BIRT\n2 DATE 1890\n", factType: domain.FactPersonBirth,
			wantDate: "1890", want: gedcom.EventData{Notes: "x"}},
		{name: "two dated births", events: "1 BIRT\n2 DATE 1890\n2 NOTE a\n1 BIRT\n2 DATE 1891\n2 PLAC Springfield\n2 NOTE b\n",
			factType: domain.FactPersonBirth, wantDate: "1890", wantNotes: "a", want: gedcom.EventData{Date: "1891", Place: "Springfield", Notes: "b"}},
		{name: "undated death first", events: "1 DEAT Y\n2 NOTE x\n1 DEAT\n2 DATE 1890\n", factType: domain.FactPersonDeath,
			wantDate: "1890", want: gedcom.EventData{Notes: "x"}},
		{name: "two dated deaths", events: "1 DEAT\n2 DATE 1890\n1 DEAT\n2 DATE 1891\n2 CAUS Fever\n", factType: domain.FactPersonDeath,
			wantDate: "1890", want: gedcom.EventData{Date: "1891", Cause: "Fever"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := "0 HEAD\n1 GEDC\n2 VERS 7.0\n0 @I1@ INDI\n1 NAME A /B/\n" + tt.events + "0 TRLR\n"
			person, events := importLifeEvents(t, data, tt.factType)
			date, notes := person.BirthDate, person.BirthNotes
			if tt.factType == domain.FactPersonDeath {
				date, notes = person.DeathDate, person.DeathNotes
			}
			if date != tt.wantDate || notes != tt.wantNotes {
				t.Errorf("record date %q, notes %q; want %q, %q", date, notes, tt.wantDate, tt.wantNotes)
			}
			if len(events) != 1 {
				t.Fatalf("life events = %+v, want one", events)
			}
			e := events[0]
			if e.Date != tt.want.Date || e.Place != tt.want.Place || e.Notes != tt.want.Notes || e.Cause != tt.want.Cause {
				t.Errorf("life event = %+v, want date %q, place %q, notes %q, cause %q", e, tt.want.Date, tt.want.Place, tt.want.Notes, tt.want.Cause)
			}
			if second, first := importAndExport(t, importAndExport(t, data)), importAndExport(t, data); second != first {
				t.Errorf("second export differs from the first:\n%s\nfirst:\n%s", second, first)
			}
		})
	}
}

// A dateless birth life event beside a dated record birth that the exporter
// writes as a BIRT of its own, such as one with a TYPE, an age, a cause or an
// address, round-trips with the record's date and the event's detail kept
// (#1766).
func TestRoundTrip_BirthEventBesideRecordBirth(t *testing.T) {
	tests := []struct {
		name  string
		event repository.EventReadModel
		check func(e gedcom.EventData) bool
	}{
		{name: "type", event: repository.EventReadModel{TypeDetail: "Stillborn"}, check: func(e gedcom.EventData) bool { return e.TypeDetail == "Stillborn" }},
		{name: "age", event: repository.EventReadModel{Age: "0"}, check: func(e gedcom.EventData) bool { return e.Age == "0" }},
		{name: "cause", event: repository.EventReadModel{Cause: "Breech"}, check: func(e gedcom.EventData) bool { return e.Cause == "Breech" }},
		{name: "address", event: repository.EventReadModel{Address: &domain.Address{Line1: "1 Main St"}},
			check: func(e gedcom.EventData) bool { return e.Address != nil && e.Address.Line1 == "1 Main St" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			rs := memory.NewReadModelStore()
			person := &repository.PersonReadModel{ID: uuid.New(), GivenName: "A", Surname: "B", BirthDateRaw: "1850", Version: 1}
			if err := rs.SavePerson(ctx, domain.MainBranchID, person); err != nil {
				t.Fatalf("SavePerson: %v", err)
			}
			event := tt.event
			event.ID, event.OwnerType, event.OwnerID, event.FactType = uuid.New(), "person", person.ID, domain.FactPersonBirth
			if err := rs.SaveEvent(ctx, domain.MainBranchID, &event); err != nil {
				t.Fatalf("SaveEvent: %v", err)
			}
			var buf bytes.Buffer
			if _, err := gedcom.NewExporter(rs).Export(ctx, &buf); err != nil {
				t.Fatalf("Export: %v", err)
			}
			first := buf.String()

			imported, births := importLifeEvents(t, first, domain.FactPersonBirth)
			if imported.BirthDate != "1850" {
				t.Errorf("re-imported birth date = %q, want 1850; export:\n%s", imported.BirthDate, first)
			}
			if len(births) != 1 || !tt.check(births[0]) {
				t.Errorf("re-imported birth life events = %+v, want one with the event's detail; export:\n%s", births, first)
			}
			if second := importAndExport(t, first); second != first {
				t.Errorf("second export differs from the first:\n%s\nfirst:\n%s", second, first)
			}
		})
	}
}
