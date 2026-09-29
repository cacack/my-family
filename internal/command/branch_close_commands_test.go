package command_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestCloseBranch_RecordsOutcomeAndReason(t *testing.T) {
	ctx := context.Background()
	f := newBranchFixture()
	branch, err := f.handler.CreateBranchWithResearch(ctx, command.CreateBranchInput{
		Name:     "theory",
		Research: domain.BranchResearch{Hypothesis: "Was Ada the daughter?"},
	})
	if err != nil {
		t.Fatalf("CreateBranchWithResearch: %v", err)
	}

	if err := f.handler.CloseBranch(ctx, branch.ID, domain.BranchOutcomeDisproved, "Register names other parents"); err != nil {
		t.Fatalf("CloseBranch: %v", err)
	}

	stored, err := f.branchStore.Get(ctx, branch.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.Status != domain.BranchStatusArchived || stored.Outcome != domain.BranchOutcomeDisproved ||
		stored.CloseReason != "Register names other parents" || stored.ClosedAt == nil {
		t.Errorf("closed branch = %+v", stored)
	}
	if stored.Hypothesis != "Was Ada the daughter?" {
		t.Errorf("close lost the hypothesis: %q", stored.Hypothesis)
	}

	events, err := f.eventStore.ReadBranch(ctx, domain.BranchID(branch.ID), 0, 10)
	if err != nil {
		t.Fatalf("ReadBranch: %v", err)
	}
	last := events[len(events)-1]
	var payload map[string]any
	if err := json.Unmarshal(last.Data, &payload); err != nil {
		t.Fatalf("decode BranchDeleted: %v", err)
	}
	if last.EventType != "BranchDeleted" || payload["outcome"] != "disproved" || payload["reason"] != "Register names other parents" {
		t.Errorf("close event = %s %v", last.EventType, payload)
	}
}

func TestCloseBranch_Validation(t *testing.T) {
	ctx := context.Background()
	f := newBranchFixture()
	branch, err := f.handler.CreateBranch(ctx, "theory", "")
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	for _, outcome := range []domain.BranchOutcome{"", domain.BranchOutcomeOpen, domain.BranchOutcomeProved, "bogus"} {
		if err := f.handler.CloseBranch(ctx, branch.ID, outcome, ""); !errors.Is(err, domain.ErrBranchInvalidCloseOutcome) {
			t.Errorf("CloseBranch(%q) = %v, want ErrBranchInvalidCloseOutcome", outcome, err)
		}
	}
	long := strings.Repeat("é", domain.MaxBranchCloseReasonLength+1)
	if err := f.handler.CloseBranch(ctx, branch.ID, domain.BranchOutcomeAbandoned, long); !errors.Is(err, domain.ErrBranchCloseReasonTooLong) {
		t.Errorf("CloseBranch with a long reason = %v, want ErrBranchCloseReasonTooLong", err)
	}
	// Nothing was recorded by the refused closes.
	if stored, err := f.branchStore.Get(ctx, branch.ID); err != nil || stored.Status != domain.BranchStatusActive {
		t.Errorf("branch after refused closes = %+v, %v", stored, err)
	}
	if err := f.handler.CloseBranch(ctx, uuid.New(), domain.BranchOutcomeAbandoned, ""); !errors.Is(err, repository.ErrBranchNotFound) {
		t.Errorf("CloseBranch unknown = %v, want ErrBranchNotFound", err)
	}
	h := command.NewHandler(memory.NewEventStore(), memory.NewReadModelStore())
	if err := h.CloseBranch(ctx, branch.ID, domain.BranchOutcomeAbandoned, ""); !errors.Is(err, command.ErrBranchStoreRequired) {
		t.Errorf("CloseBranch without a store = %v, want ErrBranchStoreRequired", err)
	}
}

// TestDeleteBranch_KeepsRecordedVerdict: DELETE /branches/{id} records no
// outcome of its own, so a verdict the branch already holds survives the close
// and only a branch still open (or never given an outcome) reads as abandoned.
func TestDeleteBranch_KeepsRecordedVerdict(t *testing.T) {
	tests := []struct {
		name  string
		prior *domain.BranchOutcome
		want  domain.BranchOutcome
	}{
		{"no outcome set", nil, domain.BranchOutcomeAbandoned},
		{"open", ptrOutcome(domain.BranchOutcomeOpen), domain.BranchOutcomeAbandoned},
		{"disproved", ptrOutcome(domain.BranchOutcomeDisproved), domain.BranchOutcomeDisproved},
		{"inconclusive", ptrOutcome(domain.BranchOutcomeInconclusive), domain.BranchOutcomeInconclusive},
		{"superseded", ptrOutcome(domain.BranchOutcomeSuperseded), domain.BranchOutcomeSuperseded},
		{"proved", ptrOutcome(domain.BranchOutcomeProved), domain.BranchOutcomeProved},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			f := newBranchFixture()
			branch, err := f.handler.CreateBranch(ctx, "dropped", "")
			if err != nil {
				t.Fatalf("CreateBranch: %v", err)
			}
			if tt.prior != nil {
				if _, err := f.handler.UpdateBranch(ctx, command.UpdateBranchInput{BranchID: branch.ID, Outcome: tt.prior}); err != nil {
					t.Fatalf("UpdateBranch: %v", err)
				}
			}
			if err := f.handler.DeleteBranch(ctx, branch.ID); err != nil {
				t.Fatalf("DeleteBranch: %v", err)
			}
			stored, err := f.branchStore.Get(ctx, branch.ID)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if stored.Status != domain.BranchStatusArchived || stored.ClosedAt == nil || stored.CloseReason != "" {
				t.Errorf("deleted branch = %+v, want archived with a close time and no reason", stored)
			}
			if stored.Outcome != tt.want {
				t.Errorf("outcome = %q, want %q", stored.Outcome, tt.want)
			}

			// The event itself carries no outcome: the verdict is the
			// projection's to resolve, so a rebuild reaches the same answer.
			events, err := f.eventStore.ReadStream(ctx, branch.ID)
			if err != nil {
				t.Fatalf("ReadStream: %v", err)
			}
			last := events[len(events)-1]
			decoded, err := last.DecodeEvent()
			if err != nil {
				t.Fatalf("DecodeEvent: %v", err)
			}
			closed, ok := decoded.(domain.BranchDeleted)
			if !ok {
				t.Fatalf("last event = %T, want BranchDeleted", decoded)
			}
			if closed.Outcome != "" || closed.Reason != "" {
				t.Errorf("close event = %+v, want no outcome and no reason", closed)
			}
		})
	}
}

func ptrOutcome(o domain.BranchOutcome) *domain.BranchOutcome { return &o }

// closedResearchFixture is a branch holding research logs of every promotion
// shape, closed as disproved.
type closedResearchFixture struct {
	f                                         *branchFixture
	branch                                    *domain.Branch
	person                                    uuid.UUID
	mainLog, promotable, branchOnly, withNote uuid.UUID
}

func newClosedResearchFixture(t *testing.T) closedResearchFixture {
	t.Helper()
	ctx := context.Background()
	f := newBranchFixture()
	person, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Placeholder"})
	if err != nil {
		t.Fatalf("CreatePerson: %v", err)
	}
	logInput := func(subject uuid.UUID, repo, notes string) command.CreateResearchLogInput {
		return command.CreateResearchLogInput{
			SubjectID: subject, SubjectType: "person", Repository: repo, SearchDescription: "Baptisms",
			Outcome: "not_found", Notes: notes, SearchDate: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
		}
	}
	mainLog, err := f.handler.CreateResearchLog(ctx, logInput(person.ID, "County archive", ""))
	if err != nil {
		t.Fatalf("CreateResearchLog main: %v", err)
	}
	branch, err := f.handler.CreateBranchWithResearch(ctx, command.CreateBranchInput{
		Name: "theory", Research: domain.BranchResearch{Hypothesis: "Was Ada the daughter?"},
	})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	onBranch := f.handler.WithBranch(branch)
	promotable, err := onBranch.CreateResearchLog(ctx, logInput(person.ID, "Parish registers", ""))
	if err != nil {
		t.Fatalf("CreateResearchLog branch: %v", err)
	}
	withNote, err := onBranch.CreateResearchLog(ctx, logInput(person.ID, "Bishop's transcripts", "Checked 1810-1820"))
	if err != nil {
		t.Fatalf("CreateResearchLog branch with note: %v", err)
	}
	branchPerson, err := onBranch.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Jamie", Surname: "Hypothetical"})
	if err != nil {
		t.Fatalf("CreatePerson on branch: %v", err)
	}
	branchOnly, err := onBranch.CreateResearchLog(ctx, logInput(branchPerson.ID, "Census index", ""))
	if err != nil {
		t.Fatalf("CreateResearchLog about branch person: %v", err)
	}
	notes := "Re-checked"
	if _, err := onBranch.UpdateResearchLog(ctx, command.UpdateResearchLogInput{ID: mainLog.ID, Notes: &notes, Version: mainLog.Version}); err != nil {
		t.Fatalf("UpdateResearchLog on branch: %v", err)
	}
	if err := f.handler.CloseBranch(ctx, branch.ID, domain.BranchOutcomeDisproved, "Other parents named"); err != nil {
		t.Fatalf("CloseBranch: %v", err)
	}
	return closedResearchFixture{
		f: f, branch: branch, person: person.ID,
		mainLog: mainLog.ID, promotable: promotable.ID, branchOnly: branchOnly.ID, withNote: withNote.ID,
	}
}

func TestPromoteBranchResearchLogs(t *testing.T) {
	ctx := context.Background()
	fx := newClosedResearchFixture(t)

	result, err := fx.f.handler.PromoteBranchResearchLogs(ctx, fx.branch.ID, nil)
	if err != nil {
		t.Fatalf("PromoteBranchResearchLogs: %v", err)
	}
	if len(result.Promoted) != 2 || result.Promoted[0] != fx.promotable || result.Promoted[1] != fx.withNote {
		t.Errorf("Promoted = %v, want [%s %s]", result.Promoted, fx.promotable, fx.withNote)
	}
	skipped := map[uuid.UUID]string{}
	for _, s := range result.Skipped {
		skipped[s.ID] = s.Reason
	}
	if skipped[fx.mainLog] != command.PromoteSkipNotCreatedOnBranch || skipped[fx.branchOnly] != command.PromoteSkipSubjectNotOnMain {
		t.Errorf("Skipped = %v", skipped)
	}

	promoted, err := fx.f.readStore.GetResearchLog(ctx, domain.MainBranchID, fx.withNote)
	if err != nil || promoted == nil {
		t.Fatalf("promoted log on main = %v, %v", promoted, err)
	}
	for _, want := range []string{"Checked 1810-1820", `"theory"`, "closed as disproved", "Was Ada the daughter?", "Other parents named"} {
		if !strings.Contains(promoted.Notes, want) {
			t.Errorf("promoted notes %q lack %q", promoted.Notes, want)
		}
	}
	if promoted.Outcome != domain.ResearchOutcomeNotFound || promoted.Repository != "Bishop's transcripts" {
		t.Errorf("promoted log = %+v", promoted)
	}

	// Selecting ids: an already promoted one and an unknown one are skipped.
	unknown := uuid.New()
	again, err := fx.f.handler.PromoteBranchResearchLogs(ctx, fx.branch.ID, []uuid.UUID{fx.promotable, unknown, unknown})
	if err != nil {
		t.Fatalf("second PromoteBranchResearchLogs: %v", err)
	}
	if len(again.Promoted) != 0 || len(again.Skipped) != 2 {
		t.Fatalf("second promotion = %+v", again)
	}
	reasons := map[uuid.UUID]string{}
	for _, s := range again.Skipped {
		reasons[s.ID] = s.Reason
	}
	if reasons[fx.promotable] != command.PromoteSkipAlreadyPromoted || reasons[unknown] != command.PromoteSkipNotFound {
		t.Errorf("second promotion skipped = %v", reasons)
	}
}

// mainCreatedEvents counts the ResearchLogCreated events main's stream holds
// for a log.
func mainCreatedEvents(t *testing.T, store *memory.EventStore, id uuid.UUID) int {
	t.Helper()
	events, err := store.ReadStream(context.Background(), id)
	if err != nil {
		t.Fatalf("ReadStream: %v", err)
	}
	n := 0
	for _, e := range events {
		if e.BranchID.IsMain() && e.EventType == "ResearchLogCreated" {
			n++
		}
	}
	return n
}

// A promoted log later deleted on main must not come back when the branch's
// logs are promoted again: the read model no longer shows it, but its stream
// on main does, and the append claiming an empty stream is refused.
func TestPromoteBranchResearchLogs_DeletedOnMainIsNotResurrected(t *testing.T) {
	ctx := context.Background()
	fx := newClosedResearchFixture(t)
	if _, err := fx.f.handler.PromoteBranchResearchLogs(ctx, fx.branch.ID, []uuid.UUID{fx.promotable}); err != nil {
		t.Fatalf("PromoteBranchResearchLogs: %v", err)
	}
	promoted, err := fx.f.readStore.GetResearchLog(ctx, domain.MainBranchID, fx.promotable)
	if err != nil || promoted == nil {
		t.Fatalf("promoted log = %v, %v", promoted, err)
	}
	if err := fx.f.handler.DeleteResearchLog(ctx, fx.promotable, promoted.Version, "Not relevant after all"); err != nil {
		t.Fatalf("DeleteResearchLog on main: %v", err)
	}

	again, err := fx.f.handler.PromoteBranchResearchLogs(ctx, fx.branch.ID, nil)
	if err != nil {
		t.Fatalf("second PromoteBranchResearchLogs: %v", err)
	}
	for _, id := range again.Promoted {
		if id == fx.promotable {
			t.Errorf("a log deleted on main was promoted again: %+v", again)
		}
	}
	reason := ""
	for _, s := range again.Skipped {
		if s.ID == fx.promotable {
			reason = s.Reason
		}
	}
	if reason != command.PromoteSkipAlreadyPromoted {
		t.Errorf("deleted log skipped as %q, want %q", reason, command.PromoteSkipAlreadyPromoted)
	}
	if got, err := fx.f.readStore.GetResearchLog(ctx, domain.MainBranchID, fx.promotable); err != nil || got != nil {
		t.Errorf("deleted log on main after re-promotion = %+v, %v; want gone", got, err)
	}
	if n := mainCreatedEvents(t, fx.f.eventStore, fx.promotable); n != 1 {
		t.Errorf("main holds %d ResearchLogCreated events for the log, want 1", n)
	}
}

// Concurrent promotions of the same log write it to main exactly once; the
// loser reports it as already promoted.
func TestPromoteBranchResearchLogs_ConcurrentWritesOnce(t *testing.T) {
	ctx := context.Background()
	for run := 0; run < 20; run++ {
		fx := newClosedResearchFixture(t)
		const workers = 4
		results := make([]*command.PromoteResearchLogsResult, workers)
		errs := make([]error, workers)
		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				results[i], errs[i] = fx.f.handler.PromoteBranchResearchLogs(ctx, fx.branch.ID, []uuid.UUID{fx.promotable})
			}(i)
		}
		wg.Wait()
		promotedBy := 0
		for i := 0; i < workers; i++ {
			if errs[i] != nil {
				t.Fatalf("run %d worker %d: %v", run, i, errs[i])
			}
			if len(results[i].Promoted) == 1 {
				promotedBy++
			} else if len(results[i].Skipped) != 1 || results[i].Skipped[0].Reason != command.PromoteSkipAlreadyPromoted {
				t.Errorf("run %d worker %d result = %+v", run, i, results[i])
			}
		}
		if promotedBy != 1 {
			t.Errorf("run %d: %d workers reported the log promoted, want 1", run, promotedBy)
		}
		if n := mainCreatedEvents(t, fx.f.eventStore, fx.promotable); n != 1 {
			t.Fatalf("run %d: main holds %d ResearchLogCreated events for the log, want 1", run, n)
		}
	}
}

func TestPromoteBranchResearchLogs_TooManyIDs(t *testing.T) {
	fx := newClosedResearchFixture(t)
	ids := make([]uuid.UUID, command.MaxPromoteLogIDs+1)
	for i := range ids {
		ids[i] = uuid.New()
	}
	if _, err := fx.f.handler.PromoteBranchResearchLogs(context.Background(), fx.branch.ID, ids); !errors.Is(err, command.ErrTooManyPromoteLogIDs) {
		t.Errorf("promotion of %d ids = %v, want ErrTooManyPromoteLogIDs", len(ids), err)
	}
}

func TestPromoteBranchResearchLogs_SubjectDeletedOnMain(t *testing.T) {
	ctx := context.Background()
	fx := newClosedResearchFixture(t)
	person, err := fx.f.readStore.GetPerson(ctx, domain.MainBranchID, fx.person)
	if err != nil || person == nil {
		t.Fatalf("GetPerson: %v %v", person, err)
	}
	if err := fx.f.handler.DeletePerson(ctx, command.DeletePersonInput{ID: fx.person, Version: person.Version}); err != nil {
		t.Fatalf("DeletePerson: %v", err)
	}
	result, err := fx.f.handler.PromoteBranchResearchLogs(ctx, fx.branch.ID, []uuid.UUID{fx.promotable})
	if err != nil {
		t.Fatalf("PromoteBranchResearchLogs: %v", err)
	}
	if len(result.Promoted) != 0 || len(result.Skipped) != 1 || result.Skipped[0].Reason != command.PromoteSkipSubjectNotOnMain {
		t.Errorf("promotion over a deleted subject = %+v", result)
	}
}

func TestPromoteBranchResearchLogs_Refusals(t *testing.T) {
	ctx := context.Background()
	f := newBranchFixture()
	active, err := f.handler.CreateBranch(ctx, "active", "")
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if _, err := f.handler.PromoteBranchResearchLogs(ctx, active.ID, nil); !errors.Is(err, command.ErrBranchNotClosed) {
		t.Errorf("promote from an active branch = %v, want ErrBranchNotClosed", err)
	}
	if _, err := f.handler.PromoteBranchResearchLogs(ctx, uuid.New(), nil); !errors.Is(err, repository.ErrBranchNotFound) {
		t.Errorf("promote from an unknown branch = %v, want ErrBranchNotFound", err)
	}
	if _, err := f.handler.WithBranch(active).PromoteBranchResearchLogs(ctx, active.ID, nil); !errors.Is(err, command.ErrPromoteNotOnMain) {
		t.Errorf("promote on a branch-scoped handler = %v, want ErrPromoteNotOnMain", err)
	}
	h := command.NewHandler(memory.NewEventStore(), memory.NewReadModelStore())
	if _, err := h.PromoteBranchResearchLogs(ctx, active.ID, nil); !errors.Is(err, command.ErrBranchStoreRequired) {
		t.Errorf("promote without a store = %v, want ErrBranchStoreRequired", err)
	}
}

// failingResearchReads fails the main research-log lookup the promotion makes
// before writing.
type failingResearchReads struct {
	repository.ReadModelStore
	failLogs, failPersons bool
}

func (s *failingResearchReads) GetResearchLog(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.ResearchLogReadModel, error) {
	if s.failLogs && branchID.IsMain() {
		return nil, errors.New("injected research log read failure")
	}
	return s.ReadModelStore.GetResearchLog(ctx, branchID, id)
}

func (s *failingResearchReads) GetPerson(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.PersonReadModel, error) {
	if s.failPersons && branchID.IsMain() {
		return nil, errors.New("injected person read failure")
	}
	return s.ReadModelStore.GetPerson(ctx, branchID, id)
}

func TestPromoteBranchResearchLogs_ReadErrors(t *testing.T) {
	ctx := context.Background()
	fx := newClosedResearchFixture(t)
	for _, tc := range []struct {
		name  string
		reads *failingResearchReads
	}{
		{"research log", &failingResearchReads{ReadModelStore: fx.f.readStore, failLogs: true}},
		{"subject", &failingResearchReads{ReadModelStore: fx.f.readStore, failPersons: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := command.NewHandlerWithBranches(fx.f.eventStore, tc.reads, fx.f.branchStore, memory.NewSnapshotStore(fx.f.eventStore))
			if _, err := h.PromoteBranchResearchLogs(ctx, fx.branch.ID, []uuid.UUID{fx.promotable}); err == nil {
				t.Error("promotion over a failing read succeeded, want an error")
			}
		})
	}
}
