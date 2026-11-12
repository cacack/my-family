package gedcom_test

import (
	"bytes"
	"context"
	"regexp"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// sourceCitationLine matches a source citation structure: a SOUR pointer at
// level 2 or deeper.
var sourceCitationLine = regexp.MustCompile(`(?m)^([2-9]|\d{2,}) SOUR @[^@]+@$`)

// CitationsExported counts the citations written: one on a life event and one
// on an attribute each once, one whose source is not exported not at all
// (#1354).
func TestExport_CitationsExportedMatchesWrittenCitations(t *testing.T) {
	ctx := context.Background()
	rs := memory.NewReadModelStore()
	main := domain.MainBranchID
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	sourceID := uuid.New()
	must(rs.SaveSource(ctx, main, &repository.SourceReadModel{ID: sourceID, SourceType: "book", Title: "Parish Register"}))
	personID := uuid.New()
	must(rs.SavePerson(ctx, main, &repository.PersonReadModel{ID: personID, GivenName: "Ann", Surname: "Lee", FullName: "Ann Lee", BirthDateRaw: "1830"}))

	burialID := uuid.New()
	must(rs.SaveEvent(ctx, main, &repository.EventReadModel{ID: burialID, OwnerType: "person", OwnerID: personID, FactType: domain.FactPersonBurial, DateRaw: "1900"}))
	occupationID := uuid.New()
	must(rs.SaveAttribute(ctx, main, &repository.AttributeReadModel{ID: occupationID, PersonID: personID, FactType: domain.FactPersonOccupation, Value: "Weaver"}))

	cite := func(factType domain.FactType, factID *uuid.UUID, source uuid.UUID) {
		must(rs.SaveCitation(ctx, main, &repository.CitationReadModel{ID: uuid.New(), SourceID: source, FactType: factType, FactOwnerID: personID, FactID: factID, Version: 1}))
	}
	cite(domain.FactPersonBurial, &burialID, sourceID)
	cite(domain.FactPersonOccupation, &occupationID, sourceID)
	cite(domain.FactPersonBirth, nil, uuid.New()) // its source is not exported

	var buf bytes.Buffer
	result, err := gedcom.NewExporter(rs).Export(ctx, &buf)
	if err != nil {
		t.Fatal(err)
	}
	written := len(sourceCitationLine.FindAllString(buf.String(), -1))
	if written != 2 {
		t.Fatalf("precondition: %d citation lines written, want 2:\n%s", written, buf.String())
	}
	if result.CitationsExported != written {
		t.Errorf("CitationsExported = %d, want %d (the citation lines written)", result.CitationsExported, written)
	}
}
