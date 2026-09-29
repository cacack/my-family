package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// A blocker scan (#831) runs the reference rules once for the resolutions it
// was given and then once more per include-fix candidate (suggestMergeFixes),
// and every run asks main the same questions: is this person there, which
// citations cite this source, which media hang off this owner, what did main
// write to these streams after that position. Main does not change during a
// scan (nothing is written until the scan has found no blocker), so the
// answers are memoized for the scan's lifetime: each distinct main lookup
// costs one query however many trial runs ask it.
//
// The memo is scoped to one scan and never outlives it, so it can never serve
// a stale answer to a later merge or precheck. Only successful answers are
// kept; an error is returned as-is and the next ask retries.

// scanKey identifies one lookup: the method, the branch it reads, the entity
// it is about, and anything else the query depends on.
type scanKey struct {
	method string
	branch domain.BranchID
	id     uuid.UUID
	extra  string
}

// scanMemo holds the memoized answers of one scan.
type scanMemo map[scanKey]any

// memoized returns the answer for key, loading it on first ask.
func memoized[T any](m scanMemo, key scanKey, load func() (T, error)) (T, error) {
	if v, ok := m[key].(T); ok {
		return v, nil
	}
	v, err := load()
	if err != nil {
		return v, err
	}
	m[key] = v
	return v, nil
}

// pair carries a two-value answer (a page and its total) through memoized.
type pair[A, B any] struct {
	a A
	b B
}

// scanReadStore memoizes the read-model lookups the reference rules make.
// Every other method passes straight through to the wrapped store.
type scanReadStore struct {
	repository.ReadModelStore
	memo scanMemo
}

func (s *scanReadStore) GetPerson(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.PersonReadModel, error) {
	return memoized(s.memo, scanKey{method: "GetPerson", branch: branchID, id: id}, func() (*repository.PersonReadModel, error) {
		return s.ReadModelStore.GetPerson(ctx, branchID, id)
	})
}

func (s *scanReadStore) GetFamily(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.FamilyReadModel, error) {
	return memoized(s.memo, scanKey{method: "GetFamily", branch: branchID, id: id}, func() (*repository.FamilyReadModel, error) {
		return s.ReadModelStore.GetFamily(ctx, branchID, id)
	})
}

func (s *scanReadStore) GetSource(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.SourceReadModel, error) {
	return memoized(s.memo, scanKey{method: "GetSource", branch: branchID, id: id}, func() (*repository.SourceReadModel, error) {
		return s.ReadModelStore.GetSource(ctx, branchID, id)
	})
}

func (s *scanReadStore) GetCitationsForSource(ctx context.Context, branchID domain.BranchID, sourceID uuid.UUID) ([]repository.CitationReadModel, error) {
	return memoized(s.memo, scanKey{method: "GetCitationsForSource", branch: branchID, id: sourceID}, func() ([]repository.CitationReadModel, error) {
		return s.ReadModelStore.GetCitationsForSource(ctx, branchID, sourceID)
	})
}

func (s *scanReadStore) ListMediaForEntity(ctx context.Context, entityType string, entityID uuid.UUID, opts repository.ListOptions) ([]repository.MediaReadModel, int, error) {
	key := scanKey{method: "ListMediaForEntity", branch: opts.BranchID, id: entityID, extra: fmt.Sprintf("%s|%+v", entityType, opts)}
	got, err := memoized(s.memo, key, func() (pair[[]repository.MediaReadModel, int], error) {
		page, total, err := s.ReadModelStore.ListMediaForEntity(ctx, entityType, entityID, opts)
		return pair[[]repository.MediaReadModel, int]{a: page, b: total}, err
	})
	return got.a, got.b, err
}

func (s *scanReadStore) GetEvidenceAnalysis(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.EvidenceAnalysisReadModel, error) {
	return memoized(s.memo, scanKey{method: "GetEvidenceAnalysis", branch: branchID, id: id}, func() (*repository.EvidenceAnalysisReadModel, error) {
		return s.ReadModelStore.GetEvidenceAnalysis(ctx, branchID, id)
	})
}

func (s *scanReadStore) GetEvidenceConflict(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.EvidenceConflictReadModel, error) {
	return memoized(s.memo, scanKey{method: "GetEvidenceConflict", branch: branchID, id: id}, func() (*repository.EvidenceConflictReadModel, error) {
		return s.ReadModelStore.GetEvidenceConflict(ctx, branchID, id)
	})
}

func (s *scanReadStore) GetResearchLog(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.ResearchLogReadModel, error) {
	return memoized(s.memo, scanKey{method: "GetResearchLog", branch: branchID, id: id}, func() (*repository.ResearchLogReadModel, error) {
		return s.ReadModelStore.GetResearchLog(ctx, branchID, id)
	})
}

func (s *scanReadStore) GetProofSummary(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.ProofSummaryReadModel, error) {
	return memoized(s.memo, scanKey{method: "GetProofSummary", branch: branchID, id: id}, func() (*repository.ProofSummaryReadModel, error) {
		return s.ReadModelStore.GetProofSummary(ctx, branchID, id)
	})
}

func (s *scanReadStore) GetAnalysesBySubject(ctx context.Context, branchID domain.BranchID, subjectID uuid.UUID) ([]repository.EvidenceAnalysisReadModel, error) {
	return memoized(s.memo, scanKey{method: "GetAnalysesBySubject", branch: branchID, id: subjectID}, func() ([]repository.EvidenceAnalysisReadModel, error) {
		return s.ReadModelStore.GetAnalysesBySubject(ctx, branchID, subjectID)
	})
}

func (s *scanReadStore) GetConflictsForSubject(ctx context.Context, branchID domain.BranchID, subjectID uuid.UUID) ([]repository.EvidenceConflictReadModel, error) {
	return memoized(s.memo, scanKey{method: "GetConflictsForSubject", branch: branchID, id: subjectID}, func() ([]repository.EvidenceConflictReadModel, error) {
		return s.ReadModelStore.GetConflictsForSubject(ctx, branchID, subjectID)
	})
}

func (s *scanReadStore) GetResearchLogsForSubject(ctx context.Context, branchID domain.BranchID, subjectID uuid.UUID) ([]repository.ResearchLogReadModel, error) {
	return memoized(s.memo, scanKey{method: "GetResearchLogsForSubject", branch: branchID, id: subjectID}, func() ([]repository.ResearchLogReadModel, error) {
		return s.ReadModelStore.GetResearchLogsForSubject(ctx, branchID, subjectID)
	})
}

func (s *scanReadStore) GetProofSummariesBySubject(ctx context.Context, branchID domain.BranchID, subjectID uuid.UUID) ([]repository.ProofSummaryReadModel, error) {
	return memoized(s.memo, scanKey{method: "GetProofSummariesBySubject", branch: branchID, id: subjectID}, func() ([]repository.ProofSummaryReadModel, error) {
		return s.ReadModelStore.GetProofSummariesBySubject(ctx, branchID, subjectID)
	})
}

// scanEventStore memoizes the event-log reads the reference rules make. Every
// other method (appends included) passes straight through.
type scanEventStore struct {
	repository.EventStore
	memo scanMemo
}

func (s *scanEventStore) ReadStream(ctx context.Context, streamID uuid.UUID) ([]repository.StoredEvent, error) {
	return memoized(s.memo, scanKey{method: "ReadStream", id: streamID}, func() ([]repository.StoredEvent, error) {
		return s.EventStore.ReadStream(ctx, streamID)
	})
}

func (s *scanEventStore) ReadStreamsForBranch(ctx context.Context, streamIDs []uuid.UUID, branchID domain.BranchID, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
	var ids strings.Builder
	for _, id := range streamIDs {
		ids.WriteString(id.String())
		ids.WriteByte(',')
	}
	key := scanKey{method: "ReadStreamsForBranch", branch: branchID, extra: fmt.Sprintf("%s|%d|%d", ids.String(), fromPosition, limit)}
	return memoized(s.memo, key, func() ([]repository.StoredEvent, error) {
		return s.EventStore.ReadStreamsForBranch(ctx, streamIDs, branchID, fromPosition, limit)
	})
}

// forBlockerScan returns a copy of h whose read model and event log memoize
// the lookups of one blocker scan. The copy is for reading only: it is
// handed to the scan and dropped with it.
func (h *Handler) forBlockerScan() *Handler {
	memo := make(scanMemo)
	scan := *h
	scan.readStore = &scanReadStore{ReadModelStore: h.readStore, memo: memo}
	scan.eventStore = &scanEventStore{EventStore: h.eventStore, memo: memo}
	return &scan
}
