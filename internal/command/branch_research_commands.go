package command

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

// Branch research-record errors (#835).
var (
	// ErrBranchSubjectNotFound is returned when a subject names a person or
	// family that is not visible on the branch (or, at creation, on main).
	ErrBranchSubjectNotFound = errors.New("branch subject not found")

	// ErrBranchProofSummaryNotFound is returned when a proof summary id names
	// a proof summary that is not visible on the branch.
	ErrBranchProofSummaryNotFound = errors.New("branch proof summary not found")

	// ErrBranchUpdateEmpty is returned when an update names no field at all.
	ErrBranchUpdateEmpty = errors.New("branch update names no fields")

	// ErrBranchFieldLocked is returned when an update to a merged branch
	// touches anything but its outcome. A merged branch is a closed record: its
	// question, subjects and evidence are what the merge was reviewed against,
	// so they stay as merged. Only the verdict may still be recorded — the
	// usual case being "proved" set once the merge has landed.
	ErrBranchFieldLocked = errors.New("a merged branch accepts only an outcome change")
)

// Field names recorded in BranchUpdated.ChangedFields. They match the API's
// JSON field names.
const (
	branchFieldDescription     = "description"
	branchFieldHypothesis      = "hypothesis"
	branchFieldSubjects        = "subjects"
	branchFieldOutcome         = "outcome"
	branchFieldProofSummaryIDs = "proof_summary_ids"
)

// CreateBranchInput is the full input of CreateBranchWithResearch: the
// branch's name and description plus its research record (#835). Outcome may
// be empty, meaning open.
type CreateBranchInput struct {
	Name        string
	Description string
	Research    domain.BranchResearch
}

// UpdateBranchInput is a partial update of a branch's description and
// research record (#835): a nil field is left as it is, a non-nil one
// replaces the stored value (an empty string or empty slice clears it).
type UpdateBranchInput struct {
	BranchID        uuid.UUID
	Description     *string
	Hypothesis      *string
	Subjects        *[]domain.BranchSubject
	Outcome         *domain.BranchOutcome
	ProofSummaryIDs *[]uuid.UUID
}

// UpdateBranch edits a branch's description and research record (#835) by
// appending a BranchUpdated event to the branch's own stream; the registry row
// is rewritten by that event's projection, never directly.
//
// What may change depends on the branch's status:
//   - active: every field.
//   - merged: only the outcome (ErrBranchFieldLocked otherwise), so the verdict
//     can be recorded once the merge has landed.
//   - archived: nothing (ErrBranchNotActive).
//
// Subjects and proof summary ids that the update ADDS must be visible on the
// branch — resolved through its overlay, so a person created on the branch
// counts and one deleted on it does not. Ids already on the branch are not
// re-checked, so a subject deleted since it was linked does not block an
// unrelated edit.
//
// An update that changes nothing appends no event and returns the branch as
// it stands.
func (h *Handler) UpdateBranch(ctx context.Context, input UpdateBranchInput) (*domain.Branch, error) {
	if h.branchStore == nil {
		return nil, ErrBranchStoreRequired
	}
	if input.isEmpty() {
		return nil, ErrBranchUpdateEmpty
	}
	// A supplied outcome must be one of the defined values. The empty string
	// is refused here even though a stored empty outcome reads as open: that
	// default exists for pre-#835 rows, and an explicit "" would otherwise
	// count as a change from "open" and record an edit that changes nothing,
	// or quietly reset a merged branch's verdict.
	if input.Outcome != nil && !input.Outcome.IsValid() {
		return nil, domain.ErrBranchInvalidOutcome
	}

	// Pin the branch stream's version BEFORE reading the branch. BranchUpdated
	// carries the full post-edit state, so every decision below (the status
	// rule, the diff, the reference checks) is made against the registry row as
	// read here; appending at a version read afterwards would let a rival
	// write that lands in between — another edit, a merge claim, a delete — be
	// silently overwritten. Pinned first, such a rival makes the append fail
	// with repository.ErrConcurrencyConflict instead. The registry row is
	// projected synchronously from this same stream, so a version read before
	// the row can never be newer than the row.
	expectedVersion, err := h.eventStore.GetStreamVersion(ctx, input.BranchID, domain.BranchID(input.BranchID))
	if err != nil {
		return nil, fmt.Errorf("getting branch stream version: %w", err)
	}

	branch, err := h.branchStore.Get(ctx, input.BranchID)
	if err != nil {
		return nil, err // includes repository.ErrBranchNotFound
	}
	// Refuse rather than fall back to -1 for an empty stream, for the reason
	// claimMerge gives: every backend skips the optimistic-concurrency check
	// for a negative expected version, so -1 would turn this guard off. A
	// registry row is only ever written by projecting the BranchCreated already
	// on this stream, so the version is at least 1.
	if expectedVersion == 0 {
		return nil, fmt.Errorf(
			"branch %s has a registry row but no events on its own stream; refusing to update without the concurrency guard",
			branch.ID)
	}
	if err := input.allowedOn(branch); err != nil {
		return nil, err
	}

	updated, changed := input.applyTo(branch)
	if err := updated.Validate(); err != nil {
		return nil, err
	}
	if len(changed) == 0 {
		return branch, nil
	}

	if err := h.checkBranchReferences(ctx, domain.BranchID(branch.ID),
		addedSubjects(branch.Subjects, updated.Subjects),
		addedIDs(branch.ProofSummaryIDs, updated.ProofSummaryIDs)); err != nil {
		return nil, err
	}

	if err := h.appendBranchUpdated(ctx, updated, changed, expectedVersion); err != nil {
		return nil, err
	}
	return h.branchStore.Get(ctx, branch.ID)
}

// isEmpty reports whether the update names no field at all.
func (input UpdateBranchInput) isEmpty() bool {
	return input.Description == nil && input.Hypothesis == nil && input.Subjects == nil &&
		input.Outcome == nil && input.ProofSummaryIDs == nil
}

// allowedOn enforces the per-status rule documented on UpdateBranch.
func (input UpdateBranchInput) allowedOn(branch *domain.Branch) error {
	switch branch.Status {
	case domain.BranchStatusActive:
		return nil
	case domain.BranchStatusMerged:
		if input.Description != nil || input.Hypothesis != nil || input.Subjects != nil || input.ProofSummaryIDs != nil {
			return ErrBranchFieldLocked
		}
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrBranchNotActive, branch.Status)
	}
}

// applyTo returns a copy of branch with the update applied, and the names of
// the fields whose value actually changed.
func (input UpdateBranchInput) applyTo(branch *domain.Branch) (*domain.Branch, []string) {
	updated := *branch
	updated.ApplyResearch(branch.Research())
	var changed []string

	if input.Description != nil && *input.Description != branch.Description {
		updated.Description = *input.Description
		changed = append(changed, branchFieldDescription)
	}
	if input.Hypothesis != nil && *input.Hypothesis != branch.Hypothesis {
		updated.Hypothesis = *input.Hypothesis
		changed = append(changed, branchFieldHypothesis)
	}
	if input.Subjects != nil && !slices.Equal(*input.Subjects, branch.Subjects) {
		updated.Subjects = append([]domain.BranchSubject(nil), (*input.Subjects)...)
		changed = append(changed, branchFieldSubjects)
	}
	if input.Outcome != nil && *input.Outcome != branch.Outcome {
		updated.Outcome = *input.Outcome
		changed = append(changed, branchFieldOutcome)
	}
	if input.ProofSummaryIDs != nil && !slices.Equal(*input.ProofSummaryIDs, branch.ProofSummaryIDs) {
		updated.ProofSummaryIDs = append([]uuid.UUID(nil), (*input.ProofSummaryIDs)...)
		changed = append(changed, branchFieldProofSummaryIDs)
	}
	return &updated, changed
}

// appendBranchUpdated appends and projects the BranchUpdated event for the
// edited branch on the branch's own stream, at expectedVersion — the version
// the edit was decided against. A rival append since then fails the call with
// repository.ErrConcurrencyConflict (wrapped).
func (h *Handler) appendBranchUpdated(ctx context.Context, updated *domain.Branch, changed []string, expectedVersion int64) error {
	scope := branchScope(updated)
	event := domain.NewBranchUpdated(updated, changed)
	if err := h.eventStore.Append(ctx, updated.ID, branchStreamType, []domain.Event{event}, expectedVersion, scope); err != nil {
		return fmt.Errorf("appending branch updated event: %w", err)
	}
	if err := h.projector.Project(ctx, event, expectedVersion+1, scope.BranchID); err != nil {
		return fmt.Errorf("projecting branch updated event: %w", err)
	}
	return nil
}

// checkBranchReferences verifies that every subject and proof summary id is
// visible on scope. Persons and families are each read with one batched
// lookup; proof summaries, capped at domain.MaxBranchProofSummaries, one at a
// time.
func (h *Handler) checkBranchReferences(ctx context.Context, scope domain.BranchID, subjects []domain.BranchSubject, proofSummaryIDs []uuid.UUID) error {
	var personIDs, familyIDs []uuid.UUID
	for _, subject := range subjects {
		switch subject.Type {
		case domain.BranchSubjectPerson:
			personIDs = append(personIDs, subject.ID)
		case domain.BranchSubjectFamily:
			familyIDs = append(familyIDs, subject.ID)
		}
	}

	if len(personIDs) > 0 {
		persons, err := h.readStore.GetPersonsByIDs(ctx, scope, personIDs)
		if err != nil {
			return fmt.Errorf("looking up branch subjects: %w", err)
		}
		found := make(map[uuid.UUID]struct{}, len(persons))
		for i := range persons {
			found[persons[i].ID] = struct{}{}
		}
		for _, id := range personIDs {
			if _, ok := found[id]; !ok {
				return fmt.Errorf("%w: person %s", ErrBranchSubjectNotFound, id)
			}
		}
	}

	if len(familyIDs) > 0 {
		families, err := h.readStore.GetFamiliesByIDs(ctx, scope, familyIDs)
		if err != nil {
			return fmt.Errorf("looking up branch subjects: %w", err)
		}
		found := make(map[uuid.UUID]struct{}, len(families))
		for i := range families {
			found[families[i].ID] = struct{}{}
		}
		for _, id := range familyIDs {
			if _, ok := found[id]; !ok {
				return fmt.Errorf("%w: family %s", ErrBranchSubjectNotFound, id)
			}
		}
	}

	for _, id := range proofSummaryIDs {
		summary, err := h.readStore.GetProofSummary(ctx, scope, id)
		if err != nil {
			return fmt.Errorf("looking up branch proof summaries: %w", err)
		}
		if summary == nil {
			return fmt.Errorf("%w: %s", ErrBranchProofSummaryNotFound, id)
		}
	}
	return nil
}

// addedSubjects returns the entries of next that are not in prev.
func addedSubjects(prev, next []domain.BranchSubject) []domain.BranchSubject {
	var added []domain.BranchSubject
	for _, subject := range next {
		if !slices.Contains(prev, subject) {
			added = append(added, subject)
		}
	}
	return added
}

// addedIDs returns the ids in next that are not in prev.
func addedIDs(prev, next []uuid.UUID) []uuid.UUID {
	var added []uuid.UUID
	for _, id := range next {
		if !slices.Contains(prev, id) {
			added = append(added, id)
		}
	}
	return added
}
