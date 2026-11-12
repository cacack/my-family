package gedcom_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// exidSpyStore counts per-record external-ID reads and can fail the batched
// ones, to pin how the exporter reads external IDs (#617).
type exidSpyStore struct {
	repository.ReadModelStore
	perRecordReads int
	failList       string // "person", "family", "source", "citation" or "repository"
}

var errListFailed = errors.New("external id read failed")

func (s *exidSpyStore) fail(kind string) error {
	if s.failList == kind {
		return errListFailed
	}
	return nil
}

func (s *exidSpyStore) GetPersonExternalIDs(ctx context.Context, b domain.BranchID, id uuid.UUID) ([]repository.PersonExternalIDReadModel, error) {
	s.perRecordReads++
	return s.ReadModelStore.GetPersonExternalIDs(ctx, b, id)
}

func (s *exidSpyStore) GetFamilyExternalIDs(ctx context.Context, b domain.BranchID, id uuid.UUID) ([]repository.FamilyExternalIDReadModel, error) {
	s.perRecordReads++
	return s.ReadModelStore.GetFamilyExternalIDs(ctx, b, id)
}

func (s *exidSpyStore) GetSourceExternalIDs(ctx context.Context, b domain.BranchID, id uuid.UUID) ([]repository.SourceExternalIDReadModel, error) {
	s.perRecordReads++
	return s.ReadModelStore.GetSourceExternalIDs(ctx, b, id)
}

func (s *exidSpyStore) GetCitationExternalIDs(ctx context.Context, b domain.BranchID, id uuid.UUID) ([]repository.CitationExternalIDReadModel, error) {
	s.perRecordReads++
	return s.ReadModelStore.GetCitationExternalIDs(ctx, b, id)
}

func (s *exidSpyStore) GetRepositoryExternalIDs(ctx context.Context, id uuid.UUID) ([]repository.RepositoryExternalIDReadModel, error) {
	s.perRecordReads++
	return s.ReadModelStore.GetRepositoryExternalIDs(ctx, id)
}

func (s *exidSpyStore) ListPersonExternalIDs(ctx context.Context, b domain.BranchID) (map[uuid.UUID][]repository.PersonExternalIDReadModel, error) {
	if err := s.fail("person"); err != nil {
		return nil, err
	}
	return s.ReadModelStore.ListPersonExternalIDs(ctx, b)
}

func (s *exidSpyStore) ListFamilyExternalIDs(ctx context.Context, b domain.BranchID) (map[uuid.UUID][]repository.FamilyExternalIDReadModel, error) {
	if err := s.fail("family"); err != nil {
		return nil, err
	}
	return s.ReadModelStore.ListFamilyExternalIDs(ctx, b)
}

func (s *exidSpyStore) ListSourceExternalIDs(ctx context.Context, b domain.BranchID) (map[uuid.UUID][]repository.SourceExternalIDReadModel, error) {
	if err := s.fail("source"); err != nil {
		return nil, err
	}
	return s.ReadModelStore.ListSourceExternalIDs(ctx, b)
}

func (s *exidSpyStore) ListCitationExternalIDs(ctx context.Context, b domain.BranchID) (map[uuid.UUID][]repository.CitationExternalIDReadModel, error) {
	if err := s.fail("citation"); err != nil {
		return nil, err
	}
	return s.ReadModelStore.ListCitationExternalIDs(ctx, b)
}

func (s *exidSpyStore) ListRepositoryExternalIDs(ctx context.Context) (map[uuid.UUID][]repository.RepositoryExternalIDReadModel, error) {
	if err := s.fail("repository"); err != nil {
		return nil, err
	}
	return s.ReadModelStore.ListRepositoryExternalIDs(ctx)
}

// seedExternalIDTree saves one record of each kind, each carrying an external
// ID, plus records without any.
func seedExternalIDTree(t *testing.T, rs *memory.ReadModelStore) {
	t.Helper()
	ctx := context.Background()
	main := domain.MainBranchID
	now := time.Now()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	repoID, sourceID, personID, familyID, citationID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	must(rs.SaveRepository(ctx, &repository.RepositoryReadModel{ID: repoID, Name: "Archive", Version: 1, UpdatedAt: now}))
	must(rs.ReplaceRepositoryExternalIDs(ctx, repoID, []repository.RepositoryExternalIDReadModel{{Value: "REPO-1", Type: "https://repo.example/"}}))
	must(rs.SaveSource(ctx, main, &repository.SourceReadModel{ID: sourceID, SourceType: domain.SourceBook, Title: "Census", Version: 1, UpdatedAt: now}))
	must(rs.ReplaceSourceExternalIDs(ctx, main, sourceID, []repository.SourceExternalIDReadModel{{Value: "SOUR-1", Type: "https://source.example/"}}))
	must(rs.SavePerson(ctx, main, &repository.PersonReadModel{ID: personID, GivenName: "Ada", Surname: "Byron", FullName: "Ada Byron", BirthDateRaw: "1815", Version: 1, UpdatedAt: now}))
	must(rs.ReplacePersonExternalIDs(ctx, main, personID, []repository.PersonExternalIDReadModel{{Value: "INDI-1", Type: "https://person.example/"}}))
	must(rs.SavePerson(ctx, main, &repository.PersonReadModel{ID: uuid.New(), GivenName: "No", Surname: "Ids", FullName: "No Ids", Version: 1, UpdatedAt: now}))
	must(rs.SaveFamily(ctx, main, &repository.FamilyReadModel{ID: familyID, Partner1ID: &personID, Version: 1, UpdatedAt: now}))
	must(rs.ReplaceFamilyExternalIDs(ctx, main, familyID, []repository.FamilyExternalIDReadModel{{Value: "FAM-1", Type: "https://family.example/"}}))
	must(rs.SaveCitation(ctx, main, &repository.CitationReadModel{ID: citationID, SourceID: sourceID, FactType: domain.FactPersonBirth, FactOwnerID: personID, Version: 1, CreatedAt: now}))
	must(rs.ReplaceCitationExternalIDs(ctx, main, citationID, []repository.CitationExternalIDReadModel{{Value: "1,7602::2771226", Type: "https://www.ancestry.com/apid"}}))
}

func TestExport_ReadsExternalIDsOncePerKind(t *testing.T) {
	rs := memory.NewReadModelStore()
	seedExternalIDTree(t, rs)
	spy := &exidSpyStore{ReadModelStore: rs}

	buf := &bytes.Buffer{}
	if _, err := gedcom.NewExporter(spy).Export(context.Background(), buf); err != nil {
		t.Fatalf("export: %v", err)
	}
	if spy.perRecordReads != 0 {
		t.Errorf("export made %d per-record external-ID reads, want 0 (one batched read per kind)", spy.perRecordReads)
	}
	for _, want := range []string{"1 EXID REPO-1", "1 EXID SOUR-1", "1 EXID INDI-1", "1 EXID FAM-1", "_APID 1,7602::2771226"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("export lacks %q; got:\n%s", want, buf.String())
		}
	}
}

func TestExport_FailsWhenExternalIDReadFails(t *testing.T) {
	for _, kind := range []string{"person", "family", "source", "citation", "repository"} {
		t.Run(kind, func(t *testing.T) {
			rs := memory.NewReadModelStore()
			seedExternalIDTree(t, rs)
			spy := &exidSpyStore{ReadModelStore: rs, failList: kind}

			_, err := gedcom.NewExporter(spy).Export(context.Background(), &bytes.Buffer{})
			if !errors.Is(err, errListFailed) {
				t.Fatalf("export error = %v, want the failed %s external-ID read", err, kind)
			}
		})
	}
}
