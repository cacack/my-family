package command

// White-box tests for the branch overlay version a branch append reports
// (#844). Every resolver in branchOverlayStreams is exercised directly: a
// cross-stream shadow of most aggregate types cannot be produced through the
// exported commands today (only a source's citation count writes one), and a
// resolver no test reaches is one that can silently read the wrong row.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestBranchOverlayStreams_ReadMainAndBranchRows(t *testing.T) {
	ctx := context.Background()
	branchID := domain.BranchID(uuid.New())

	// save writes one row of the aggregate on scope at version.
	type saver func(rs repository.ReadModelStore, scope domain.BranchID, id uuid.UUID, version int64) error
	savers := map[string]saver{
		"person": func(rs repository.ReadModelStore, scope domain.BranchID, id uuid.UUID, v int64) error {
			return rs.SavePerson(ctx, scope, &repository.PersonReadModel{ID: id, GivenName: "Alex", Version: v})
		},
		familyStreamType: func(rs repository.ReadModelStore, scope domain.BranchID, id uuid.UUID, v int64) error {
			return rs.SaveFamily(ctx, scope, &repository.FamilyReadModel{ID: id, Version: v})
		},
		"source": func(rs repository.ReadModelStore, scope domain.BranchID, id uuid.UUID, v int64) error {
			return rs.SaveSource(ctx, scope, &repository.SourceReadModel{ID: id, Title: "Register", Version: v})
		},
		"citation": func(rs repository.ReadModelStore, scope domain.BranchID, id uuid.UUID, v int64) error {
			return rs.SaveCitation(ctx, scope, &repository.CitationReadModel{ID: id, SourceID: uuid.New(), Version: v})
		},
		"note": func(rs repository.ReadModelStore, scope domain.BranchID, id uuid.UUID, v int64) error {
			return rs.SaveNote(ctx, scope, &repository.NoteReadModel{ID: id, Text: "note", Version: v})
		},
		"media": func(rs repository.ReadModelStore, scope domain.BranchID, id uuid.UUID, v int64) error {
			return rs.SaveMedia(ctx, scope, &repository.MediaReadModel{ID: id, EntityType: "person", EntityID: uuid.New(), Title: "photo", Version: v})
		},
		"association": func(rs repository.ReadModelStore, scope domain.BranchID, id uuid.UUID, v int64) error {
			return rs.SaveAssociation(ctx, scope, &repository.AssociationReadModel{ID: id, PersonID: uuid.New(), AssociateID: uuid.New(), Version: v})
		},
		"evidenceanalysis": func(rs repository.ReadModelStore, scope domain.BranchID, id uuid.UUID, v int64) error {
			return rs.SaveEvidenceAnalysis(ctx, scope, &repository.EvidenceAnalysisReadModel{ID: id, SubjectID: uuid.New(), Version: v})
		},
		"evidenceconflict": func(rs repository.ReadModelStore, scope domain.BranchID, id uuid.UUID, v int64) error {
			return rs.SaveEvidenceConflict(ctx, scope, &repository.EvidenceConflictReadModel{ID: id, SubjectID: uuid.New(), Version: v})
		},
		"researchlog": func(rs repository.ReadModelStore, scope domain.BranchID, id uuid.UUID, v int64) error {
			return rs.SaveResearchLog(ctx, scope, &repository.ResearchLogReadModel{ID: id, SubjectID: uuid.New(), Version: v})
		},
		"proofsummary": func(rs repository.ReadModelStore, scope domain.BranchID, id uuid.UUID, v int64) error {
			return rs.SaveProofSummary(ctx, scope, &repository.ProofSummaryReadModel{ID: id, SubjectID: uuid.New(), Version: v})
		},
	}
	if len(savers) != len(branchOverlayStreams) {
		t.Fatalf("test covers %d stream types, branchOverlayStreams has %d", len(savers), len(branchOverlayStreams))
	}

	for streamType, versions := range branchOverlayStreams {
		t.Run(streamType, func(t *testing.T) {
			save, ok := savers[streamType]
			if !ok {
				t.Fatalf("no saver for stream type %q", streamType)
			}
			rs := memory.NewReadModelStore()
			id := uuid.New()

			// No row anywhere: both versions are 0.
			if m, b, err := versions(ctx, rs, branchID, id); err != nil || m != 0 || b != 0 {
				t.Fatalf("versions of a missing row = %d, %d (err %v), want 0, 0", m, b, err)
			}
			// Main only: the branch reads main's row through the overlay.
			if err := save(rs, domain.MainBranchID, id, 3); err != nil {
				t.Fatalf("save main row: %v", err)
			}
			if m, b, err := versions(ctx, rs, branchID, id); err != nil || m != 3 || b != 3 {
				t.Fatalf("versions with main's row only = %d, %d (err %v), want 3, 3", m, b, err)
			}
			// A branch shadow row wins on the branch.
			if err := save(rs, branchID, id, 2); err != nil {
				t.Fatalf("save branch row: %v", err)
			}
			if m, b, err := versions(ctx, rs, branchID, id); err != nil || m != 3 || b != 2 {
				t.Fatalf("versions with a branch shadow = %d, %d (err %v), want 3, 2", m, b, err)
			}
		})
	}
}

// failingSourceStore fails every source read, to prove a read error on the
// overlay lookup aborts the append instead of seeding from the wrong version.
type failingSourceStore struct {
	repository.ReadModelStore
	failOn domain.BranchID
}

var errSourceRead = errors.New("source read failed")

func (s failingSourceStore) GetSource(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.SourceReadModel, error) {
	if branchID == s.failOn {
		return nil, errSourceRead
	}
	return s.ReadModelStore.GetSource(ctx, branchID, id)
}

func TestBranchOverlayVersion(t *testing.T) {
	ctx := context.Background()
	branch := &domain.Branch{ID: uuid.New(), Name: "overlay"}
	branchID := domain.BranchID(branch.ID)
	rs := memory.NewReadModelStore()
	h := NewHandler(memory.NewEventStore(), rs).WithBranch(branch)

	id := uuid.New()
	if err := rs.SaveSource(ctx, domain.MainBranchID, &repository.SourceReadModel{ID: id, Title: "Register", Version: 3}); err != nil {
		t.Fatalf("save main source: %v", err)
	}

	if v, err := h.branchOverlayVersion(ctx, "Repository", id); err != nil || v != 0 {
		t.Fatalf("overlay version of a stream type with no resolver = %d (err %v), want 0", v, err)
	}
	if v, err := h.branchOverlayVersion(ctx, "Source", id); err != nil || v != 0 {
		t.Fatalf("overlay version when the branch reads main's row = %d (err %v), want 0", v, err)
	}
	if err := rs.SaveSource(ctx, branchID, &repository.SourceReadModel{ID: id, Title: "Register", Version: 3}); err != nil {
		t.Fatalf("save branch source: %v", err)
	}
	if v, err := h.branchOverlayVersion(ctx, "Source", id); err != nil || v != 0 {
		t.Fatalf("overlay version of a shadow at main's version = %d (err %v), want 0", v, err)
	}
	if err := rs.SaveSource(ctx, branchID, &repository.SourceReadModel{ID: id, Title: "Register", Version: 2}); err != nil {
		t.Fatalf("save older branch source: %v", err)
	}
	if v, err := h.branchOverlayVersion(ctx, "Source", id); err != nil || v != 2 {
		t.Fatalf("overlay version of a shadow behind main = %d (err %v), want 2", v, err)
	}

	for _, failOn := range []domain.BranchID{domain.MainBranchID, branchID} {
		failing := NewHandler(memory.NewEventStore(), failingSourceStore{ReadModelStore: rs, failOn: failOn}).WithBranch(branch)
		if _, err := failing.branchOverlayVersion(ctx, "Source", id); !errors.Is(err, errSourceRead) {
			t.Fatalf("overlay version with a failing read on %s: want errSourceRead, got %v", failOn, err)
		}
		_, err := failing.execute(ctx, id.String(), "Source",
			[]domain.Event{domain.NewSourceUpdated(id, map[string]any{"notes": "x"})}, 2)
		if !errors.Is(err, errSourceRead) {
			t.Fatalf("branch execute with a failing overlay read on %s: want errSourceRead, got %v", failOn, err)
		}
	}
}
