package command

// Resume repair of GPS artifacts whose subject main merged away (#760 on top
// of #685): relinkMergedGPS and the landed-subject check it feeds.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// gpsFailStore fails every GPS artifact read and save.
type gpsFailStore struct{ repository.ReadModelStore }

var errGPSStore = errors.New("gps store failed")

func (gpsFailStore) GetEvidenceAnalysis(context.Context, domain.BranchID, uuid.UUID) (*repository.EvidenceAnalysisReadModel, error) {
	return nil, errGPSStore
}

func (gpsFailStore) GetEvidenceConflict(context.Context, domain.BranchID, uuid.UUID) (*repository.EvidenceConflictReadModel, error) {
	return nil, errGPSStore
}

func (gpsFailStore) GetResearchLog(context.Context, domain.BranchID, uuid.UUID) (*repository.ResearchLogReadModel, error) {
	return nil, errGPSStore
}

func (gpsFailStore) GetProofSummary(context.Context, domain.BranchID, uuid.UUID) (*repository.ProofSummaryReadModel, error) {
	return nil, errGPSStore
}

// gpsSaveFailStore reads through but fails every GPS artifact save.
type gpsSaveFailStore struct{ repository.ReadModelStore }

func (gpsSaveFailStore) SaveEvidenceAnalysis(context.Context, domain.BranchID, *repository.EvidenceAnalysisReadModel) error {
	return errGPSStore
}

func (gpsSaveFailStore) SaveEvidenceConflict(context.Context, domain.BranchID, *repository.EvidenceConflictReadModel) error {
	return errGPSStore
}

func (gpsSaveFailStore) SaveResearchLog(context.Context, domain.BranchID, *repository.ResearchLogReadModel) error {
	return errGPSStore
}

func (gpsSaveFailStore) SaveProofSummary(context.Context, domain.BranchID, *repository.ProofSummaryReadModel) error {
	return errGPSStore
}

// saveGPSRow writes a main row of the given GPS stream type about subjectID.
func saveGPSRow(t *testing.T, store repository.ReadModelStore, streamType string, id, subjectID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	main := domain.MainBranchID
	var err error
	switch streamType {
	case "EvidenceAnalysis":
		err = store.SaveEvidenceAnalysis(ctx, main, &repository.EvidenceAnalysisReadModel{ID: id, SubjectID: subjectID, Version: 4})
	case "EvidenceConflict":
		err = store.SaveEvidenceConflict(ctx, main, &repository.EvidenceConflictReadModel{ID: id, SubjectID: subjectID, Version: 4})
	case "ResearchLog":
		err = store.SaveResearchLog(ctx, main, &repository.ResearchLogReadModel{ID: id, SubjectID: subjectID, Repository: "Archive", Version: 4})
	default:
		err = store.SaveProofSummary(ctx, main, &repository.ProofSummaryReadModel{ID: id, SubjectID: subjectID, Version: 4})
	}
	if err != nil {
		t.Fatalf("saving %s: %v", streamType, err)
	}
}

// relinkMergedGPS re-points each GPS artifact type's re-projected row to the
// survivor with its version untouched, follows a merge of that survivor
// recorded after the resume's own scan, keeps a row main re-pointed elsewhere,
// and leaves a vanished row alone.
func TestRelinkMergedGPS(t *testing.T) {
	ctx := context.Background()
	for _, streamType := range gpsStreamTypes {
		t.Run(streamType, func(t *testing.T) {
			store := memory.NewReadModelStore()
			events := memory.NewEventStore()
			h := &Handler{readStore: store, eventStore: events}
			id, subject, first, second := uuid.New(), uuid.New(), uuid.New(), uuid.New()
			group := streamGroup{streamID: id, streamType: streamType}
			saveGPSRow(t, store, streamType, id, subject)

			// The resume's scan saw subject→first; first→second lands after it.
			appendMainMerge(t, events, first, subject)
			appendMainMerge(t, events, second, first)
			if err := h.relinkMergedGPS(ctx, group, mergeRelink{from: subject, target: first}); err != nil {
				t.Fatalf("relinkMergedGPS failed: %v", err)
			}
			row, found, err := h.mainGPSRow(ctx, group)
			if err != nil || !found {
				t.Fatalf("mainGPSRow = %v, %v", found, err)
			}
			if row.subjectID != second || row.version != 4 {
				t.Errorf("row = subject %s version %d; want the final survivor %s and version 4", row.subjectID, row.version, second)
			}

			// A row about someone else (main re-pointed it) is left alone.
			other := uuid.New()
			saveGPSRow(t, store, streamType, id, other)
			if err := h.relinkMergedGPS(ctx, group, mergeRelink{from: subject, target: second}); err != nil {
				t.Fatalf("relinkMergedGPS failed: %v", err)
			}
			if row, _, _ := h.mainGPSRow(ctx, group); row.subjectID != other {
				t.Errorf("re-pointed row moved to %s, want it kept on %s", row.subjectID, other)
			}

			failing := &Handler{readStore: gpsSaveFailStore{store}, eventStore: events}
			if err := failing.relinkMergedGPS(ctx, group, mergeRelink{from: other, target: second}); !errors.Is(err, errGPSStore) {
				t.Errorf("failed save: err = %v, want the store error", err)
			}
			failing = &Handler{readStore: gpsFailStore{store}, eventStore: events}
			if err := failing.relinkMergedGPS(ctx, group, mergeRelink{from: other, target: second}); !errors.Is(err, errGPSStore) {
				t.Errorf("failed read: err = %v, want the store error", err)
			}
			if err := failing.setMainGPSSubject(ctx, group, second); !errors.Is(err, errGPSStore) {
				t.Errorf("failed read before save: err = %v, want the store error", err)
			}
		})
	}

	store := memory.NewReadModelStore()
	h := &Handler{readStore: store, eventStore: memory.NewEventStore()}
	group := streamGroup{streamID: uuid.New(), streamType: "ResearchLog"}
	if err := h.relinkMergedGPS(ctx, group, mergeRelink{from: uuid.New(), target: uuid.New()}); err != nil {
		t.Errorf("vanished row: err = %v, want nil", err)
	}
}

// A survivor merged again on every scan cannot hold the repair forever.
func TestRelinkMergedGPS_GivesUpOnEndlessMerges(t *testing.T) {
	ctx := context.Background()
	store := memory.NewReadModelStore()
	events := memory.NewEventStore()
	chain := []uuid.UUID{uuid.New()}
	for i := 0; i < reprojectAttempts+1; i++ {
		next := uuid.New()
		appendMainMerge(t, events, next, chain[len(chain)-1])
		chain = append(chain, next)
	}
	id := uuid.New()
	saveGPSRow(t, store, "ProofSummary", id, chain[0])
	calls := 0
	h := &Handler{readStore: store, eventStore: &steppingMergeStore{EventStore: events, calls: &calls}}
	err := h.relinkMerged(ctx, streamGroup{streamID: id, streamType: "ProofSummary"}, mergeRelink{from: chain[0], target: chain[0]})
	if !errors.Is(err, errReprojectRaced) {
		t.Errorf("endless merges: err = %v, want errReprojectRaced", err)
	}
}

// researchLogGroup is a landed research log stream about subjectID.
func researchLogGroup(t *testing.T, id, subjectID uuid.UUID) streamGroup {
	t.Helper()
	data, err := json.Marshal(domain.NewResearchLogCreated(&domain.ResearchLog{ID: id, SubjectID: subjectID, Repository: "Archive"}))
	if err != nil {
		t.Fatalf("encoding ResearchLogCreated failed: %v", err)
	}
	return streamGroup{streamID: id, streamType: "ResearchLog", events: []repository.StoredEvent{
		{StreamID: id, StreamType: "ResearchLog", EventType: "ResearchLogCreated", Data: data},
	}}
}

// A landed GPS artifact may not be left about a subject main does not have by
// this call's own "main" resolution; the subject checked is main's row's, or,
// with no row, the survivor the repair re-points it to.
func TestCheckLandedGPSSubjects(t *testing.T) {
	ctx := context.Background()
	store := memory.NewReadModelStore()
	h := &Handler{readStore: store}
	logID, subject, survivor := uuid.New(), uuid.New(), uuid.New()
	groups := []streamGroup{personGroup(subject, "PersonCreated", "PersonDeleted"), researchLogGroup(t, logID, subject)}
	view := resumeView{landed: map[uuid.UUID]bool{logID: true}}
	mainRes := func(id uuid.UUID) map[uuid.UUID]MergeResolution {
		return map[uuid.UUID]MergeResolution{id: ResolveMain}
	}

	if err := h.checkLandedGPSSubjects(ctx, groups, view, mainRes(subject)); !errors.Is(err, ErrMergeDanglingReference) {
		t.Errorf("subject main lacks, resolved to main: err = %v, want ErrMergeDanglingReference", err)
	}
	if err := h.checkLandedGPSSubjects(ctx, groups, view, map[uuid.UUID]MergeResolution{subject: ResolveBranch}); err != nil {
		t.Errorf("subject resolved to branch: err = %v, want nil", err)
	}
	removed := resumeView{landed: view.landed, removed: map[uuid.UUID]bool{logID: true}}
	if err := h.checkLandedGPSSubjects(ctx, groups, removed, mainRes(subject)); err != nil {
		t.Errorf("log main removed: err = %v, want nil", err)
	}

	// With no main row and the subject merged away, the survivor counts.
	relinked := resumeView{landed: view.landed, relinked: map[uuid.UUID]uuid.UUID{logID: survivor}}
	if err := h.checkLandedGPSSubjects(ctx, groups, relinked, mainRes(subject)); err != nil {
		t.Errorf("merged-away subject resolved to main: err = %v, want nil", err)
	}
	if err := h.checkLandedGPSSubjects(ctx, groups, relinked, mainRes(survivor)); !errors.Is(err, ErrMergeDanglingReference) {
		t.Errorf("survivor main lacks, resolved to main: err = %v, want ErrMergeDanglingReference", err)
	}

	// Main's row names the subject checked; a subject main has is fine.
	saveGPSRow(t, store, "ResearchLog", logID, survivor)
	if err := store.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{ID: survivor, GivenName: "Sam", Version: 1}); err != nil {
		t.Fatalf("SavePerson failed: %v", err)
	}
	if err := h.checkLandedGPSSubjects(ctx, groups, view, mainRes(survivor)); err != nil {
		t.Errorf("row's subject main has: err = %v, want nil", err)
	}

	failing := &Handler{readStore: gpsFailStore{store}}
	if err := failing.checkLandedGPSSubjects(ctx, groups, view, nil); !errors.Is(err, errGPSStore) {
		t.Errorf("failed row read: err = %v, want the store error", err)
	}
	bad := streamGroup{streamID: logID, streamType: "ResearchLog", events: []repository.StoredEvent{{StreamID: logID, EventType: "ResearchLogCreated", Data: []byte(`{not json`)}}}
	if err := h.checkLandedGPSSubjects(ctx, []streamGroup{bad}, view, nil); err == nil || errors.Is(err, ErrMergeDanglingReference) {
		t.Errorf("undecodable landed log: err = %v, want a decode error", err)
	}
}
