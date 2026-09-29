package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// memoized loads a key once, keeps typed-nil answers, and never keeps an error.
func TestMemoized(t *testing.T) {
	memo := make(scanMemo)
	key := scanKey{method: "m", id: uuid.New()}
	loads := 0
	boom := errors.New("boom")
	fail := func() (*repository.PersonReadModel, error) { loads++; return nil, boom }
	if _, err := memoized(memo, key, fail); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	none := func() (*repository.PersonReadModel, error) { loads++; return nil, nil }
	for i := 0; i < 3; i++ {
		got, err := memoized(memo, key, none)
		if err != nil || got != nil {
			t.Fatalf("got %v, %v; want nil, nil", got, err)
		}
	}
	if loads != 2 {
		t.Errorf("loads = %d, want 2 (the failed load is retried, the nil answer is kept)", loads)
	}
	other := scanKey{method: "m", id: key.id, extra: "x"}
	if _, err := memoized(memo, other, none); err != nil || loads != 3 {
		t.Errorf("a different key must load again: err = %v, loads = %d", err, loads)
	}
}

// Every memoized lookup answers a repeat from the memo: once the stores are
// gone, asking again still works.
func TestForBlockerScan_AnswersRepeatsFromTheMemo(t *testing.T) {
	ctx := context.Background()
	h := &Handler{eventStore: memory.NewEventStore(), readStore: memory.NewReadModelStore()}
	scan := h.forBlockerScan()
	id := uuid.New()
	main := domain.MainBranchID
	opts := repository.ListOptions{Limit: 10, BranchID: main}

	ask := func() error {
		if _, err := scan.readStore.GetPerson(ctx, main, id); err != nil {
			return err
		}
		if _, err := scan.readStore.GetFamily(ctx, main, id); err != nil {
			return err
		}
		if _, err := scan.readStore.GetSource(ctx, main, id); err != nil {
			return err
		}
		if _, err := scan.readStore.GetCitationsForSource(ctx, main, id); err != nil {
			return err
		}
		if _, _, err := scan.readStore.ListMediaForEntity(ctx, "person", id, opts); err != nil {
			return err
		}
		if _, err := scan.readStore.GetEvidenceAnalysis(ctx, main, id); err != nil {
			return err
		}
		if _, err := scan.readStore.GetEvidenceConflict(ctx, main, id); err != nil {
			return err
		}
		if _, err := scan.readStore.GetResearchLog(ctx, main, id); err != nil {
			return err
		}
		if _, err := scan.readStore.GetProofSummary(ctx, main, id); err != nil {
			return err
		}
		if _, err := scan.readStore.GetAnalysesBySubject(ctx, main, id); err != nil {
			return err
		}
		if _, err := scan.readStore.GetConflictsForSubject(ctx, main, id); err != nil {
			return err
		}
		if _, err := scan.readStore.GetResearchLogsForSubject(ctx, main, id); err != nil {
			return err
		}
		if _, err := scan.readStore.GetProofSummariesBySubject(ctx, main, id); err != nil {
			return err
		}
		if _, err := scan.eventStore.ReadStream(ctx, id); err != nil {
			return err
		}
		_, err := scan.eventStore.ReadStreamsForBranch(ctx, []uuid.UUID{id}, main, 0, 1)
		return err
	}
	if err := ask(); err != nil {
		t.Fatalf("first ask: %v", err)
	}
	// Drop the stores: a repeat that reached them would panic.
	scan.readStore.(*scanReadStore).ReadModelStore = nil
	scan.eventStore.(*scanEventStore).EventStore = nil
	if err := ask(); err != nil {
		t.Fatalf("repeat ask: %v", err)
	}
	// The original handler is untouched.
	if _, ok := h.readStore.(*scanReadStore); ok {
		t.Error("forBlockerScan wrapped the handler's own read store")
	}
}
