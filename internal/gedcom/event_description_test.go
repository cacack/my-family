package gedcom_test

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// TestExport_NamedEventDescription checks that a named event's description,
// which GEDCOM has no line slot for, is exported as a NOTE under the event,
// that an EVEN keeps its descriptor on the EVEN line with no extra NOTE, and
// that export -> import -> export writes each event block unchanged, so the
// text is never duplicated (issue #1283).
func TestExport_NamedEventDescription(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()

	john, mary, fam := uuid.New(), uuid.New(), uuid.New()
	for _, p := range []*repository.PersonReadModel{
		{ID: john, GivenName: "John", Surname: "Doe", FullName: "John Doe", Gender: domain.GenderMale},
		{ID: mary, GivenName: "Mary", Surname: "Roe", FullName: "Mary Roe", Gender: domain.GenderFemale},
	} {
		if err := readStore.SavePerson(ctx, domain.MainBranchID, p); err != nil {
			t.Fatalf("save person: %v", err)
		}
	}
	if err := readStore.SaveFamily(ctx, domain.MainBranchID, &repository.FamilyReadModel{
		ID: fam, Partner1ID: &john, Partner2ID: &mary,
	}); err != nil {
		t.Fatalf("save family: %v", err)
	}
	for _, e := range []*repository.EventReadModel{
		{OwnerType: "person", OwnerID: john, FactType: domain.FactPersonBurial, DateRaw: "1920",
			Description: "Family plot, north corner"},
		{OwnerType: "person", OwnerID: mary, FactType: domain.FactPersonBurial, DateRaw: "1930",
			Description: "Beside her husband", Notes: "@N1@\n\nPlot bought 1880"},
		{OwnerType: "person", OwnerID: john, FactType: domain.FactPersonGenericEvent, DateRaw: "1890",
			Description: "Moved west", Notes: "By wagon"},
		{OwnerType: "family", OwnerID: fam, FactType: domain.FactFamilyDivorce, DateRaw: "1910",
			Description: "Uncontested"},
	} {
		e.ID, e.Version = uuid.New(), 1
		if err := readStore.SaveEvent(ctx, domain.MainBranchID, e); err != nil {
			t.Fatalf("save event: %v", err)
		}
	}
	if err := readStore.SaveNote(ctx, domain.MainBranchID, &repository.NoteReadModel{
		ID: uuid.New(), GedcomXref: "@N1@", Text: "Shared note", Version: 1,
	}); err != nil {
		t.Fatalf("save note: %v", err)
	}

	buf := &bytes.Buffer{}
	if _, err := gedcom.NewExporter(readStore).Export(ctx, buf); err != nil {
		t.Fatalf("export: %v", err)
	}
	first := strings.ReplaceAll(buf.String(), "\r\n", "\n")

	blocks := []struct{ marker, event string }{
		{"1 NAME John /Doe/", "1 BURI"},
		{"1 NAME Mary /Roe/", "1 BURI"},
		{"1 NAME John /Doe/", "1 EVEN Moved west"},
		{"1 HUSB ", "1 DIV"},
	}
	want := map[int][]string{
		0: {"2 NOTE Family plot, north corner"},
		1: {"2 NOTE @N1@", "2 NOTE Plot bought 1880", "3 CONT", "3 CONT Beside her husband"},
		2: {"2 NOTE By wagon"},
		3: {"2 NOTE Uncontested"},
	}
	for i, b := range blocks {
		if got := subNoteLines(first, b.marker, b.event); !slices.Equal(got, want[i]) {
			t.Errorf("NOTE lines under %q = %q, want %q; export:\n%s", b.event, got, want[i], first)
		}
	}
	if strings.Contains(first, "Moved west\n2 NOTE Moved west") || strings.Count(first, "Moved west") != 1 {
		t.Errorf("EVEN descriptor should appear once, on the EVEN line:\n%s", first)
	}

	second := strings.ReplaceAll(importAndExport(t, first), "\r\n", "\n")
	for _, b := range blocks {
		got, want := eventBlock(second, b.marker, b.event), eventBlock(first, b.marker, b.event)
		if want == nil || !slices.Equal(got, want) {
			t.Errorf("%q block changed on re-import:\nfirst:  %q\nsecond: %q", b.event, want, got)
		}
	}
	for _, text := range []string{"Family plot, north corner", "Beside her husband", "Uncontested"} {
		if n := strings.Count(second, text); n != 1 {
			t.Errorf("%q appears %d times after re-import, want 1:\n%s", text, n, second)
		}
	}
}

// eventBlock returns the line event and its subordinate lines in the record
// that has a line starting with marker, or nil if there is none.
func eventBlock(out, marker, event string) []string {
	for _, record := range strings.Split(out, "\n0 ") {
		lines := strings.Split(record, "\n")
		if !slices.ContainsFunc(lines, func(l string) bool { return strings.HasPrefix(l, marker) }) {
			continue
		}
		for i, line := range lines {
			if line != event {
				continue
			}
			end := i + 1
			for end < len(lines) && !strings.HasPrefix(lines[end], "1 ") {
				end++
			}
			return lines[i:end]
		}
	}
	return nil
}
