package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

// Common errors for branch store operations.
var (
	ErrBranchNotFound = errors.New("branch not found")
)

// BranchStore provides storage for the branch registry read model. It is the
// branch analog of SnapshotStore: the projection writes it from BranchCreated
// events and queries read it.
type BranchStore interface {
	// Create stores a new branch.
	Create(ctx context.Context, branch *domain.Branch) error

	// Upsert stores a branch, inserting it or updating an existing row with the
	// same ID. Used by the projection, which may replay events idempotently.
	Upsert(ctx context.Context, branch *domain.Branch) error

	// Get retrieves a branch by ID, returning ErrBranchNotFound when missing.
	Get(ctx context.Context, id uuid.UUID) (*domain.Branch, error)

	// List retrieves all branches ordered by created_at DESC.
	List(ctx context.Context) ([]*domain.Branch, error)

	// Delete removes a branch by ID, returning ErrBranchNotFound when missing.
	Delete(ctx context.Context, id uuid.UUID) error

	// UpdateStatus changes a branch's status, returning ErrBranchNotFound when
	// missing. Used by the deleted projection's archive transition.
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.BranchStatus) error

	// UpdateDetails overwrites a branch's description and research record
	// (#835) — hypothesis, subjects, outcome and proof summary ids — leaving
	// name, base position, status and the merge record untouched. Returns
	// ErrBranchNotFound when missing. Written only by the BranchUpdated
	// projection.
	UpdateDetails(ctx context.Context, id uuid.UUID, description string, research domain.BranchResearch) error

	// MarkMerged records the merge: it sets the status to merged and writes the
	// merge timestamp and note in one atomic write, returning ErrBranchNotFound
	// when missing.
	//
	// This is a distinct method rather than a wider UpdateStatus because the two
	// terminal transitions differ in kind: archiving carries no metadata, while a
	// merged branch must never be recorded without its timestamp (issue #55,
	// "merge history preserved"). Folding the metadata into UpdateStatus would
	// make it optional at every call site and let a merge land with a nil
	// MergedAt; a separate method makes the record impossible to omit.
	MarkMerged(ctx context.Context, id uuid.UUID, mergedAt time.Time, note string) error

	// MarkClosed records a close without merging (#836): it sets the status to
	// archived and writes the close timestamp and reason in one atomic write,
	// returning ErrBranchNotFound when missing. A non-empty outcome also
	// overwrites the branch's outcome; an empty one (a BranchDeleted written
	// before #836, which recorded no outcome) keeps a verdict the branch had
	// and turns "open" (or unset) into abandoned: a closed branch is never open. Written only
	// by the BranchDeleted projection. Like MarkMerged it is separate from
	// UpdateStatus so the close record cannot be omitted from the transition.
	MarkClosed(ctx context.Context, id uuid.UUID, closedAt time.Time, outcome domain.BranchOutcome, reason string) error
}

// BranchListsJSON is the stored form of a branch's research-record lists
// (#835): subjects and proof summary ids, each JSON-encoded, or "" for an
// empty list so both SQL stores can bind it as NULL.
type BranchListsJSON struct {
	Subjects        string
	ProofSummaryIDs string
}

// EncodeBranchLists JSON-encodes the research record's lists for storage.
func EncodeBranchLists(research domain.BranchResearch) (BranchListsJSON, error) {
	var out BranchListsJSON
	if len(research.Subjects) > 0 {
		b, err := json.Marshal(research.Subjects)
		if err != nil {
			return BranchListsJSON{}, fmt.Errorf("encode branch subjects: %w", err)
		}
		out.Subjects = string(b)
	}
	if len(research.ProofSummaryIDs) > 0 {
		b, err := json.Marshal(research.ProofSummaryIDs)
		if err != nil {
			return BranchListsJSON{}, fmt.Errorf("encode branch proof summary ids: %w", err)
		}
		out.ProofSummaryIDs = string(b)
	}
	return out, nil
}

// DecodeBranchResearch rebuilds a branch's research record from its stored
// columns. An empty (NULL) list decodes to nil and an empty outcome — a row
// written before #835 — to open.
func DecodeBranchResearch(hypothesis, outcome string, lists BranchListsJSON) (domain.BranchResearch, error) {
	research := domain.BranchResearch{
		Hypothesis: hypothesis,
		Outcome:    domain.BranchOutcome(outcome).OrDefault(),
	}
	if lists.Subjects != "" {
		if err := json.Unmarshal([]byte(lists.Subjects), &research.Subjects); err != nil {
			return domain.BranchResearch{}, fmt.Errorf("decode branch subjects: %w", err)
		}
	}
	if lists.ProofSummaryIDs != "" {
		if err := json.Unmarshal([]byte(lists.ProofSummaryIDs), &research.ProofSummaryIDs); err != nil {
			return domain.BranchResearch{}, fmt.Errorf("decode branch proof summary ids: %w", err)
		}
	}
	if len(research.Subjects) == 0 {
		research.Subjects = nil
	}
	if len(research.ProofSummaryIDs) == 0 {
		research.ProofSummaryIDs = nil
	}
	return research, nil
}
