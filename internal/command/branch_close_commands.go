package command

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

// Research-log promotion errors (#836).
var (
	// ErrBranchNotClosed is returned when research logs are promoted from a
	// branch that is not closed (archived). An active branch's logs reach main
	// by merging it; a merged branch's are already there.
	ErrBranchNotClosed = errors.New("branch is not closed")

	// ErrPromoteNotOnMain is returned when the promotion runs on a
	// branch-scoped handler: promotion writes to the mainline.
	ErrPromoteNotOnMain = errors.New("research logs are promoted to the mainline only")

	// ErrTooManyPromoteLogIDs is returned when a promotion names more than
	// MaxPromoteLogIDs research logs.
	ErrTooManyPromoteLogIDs = fmt.Errorf("a promotion may name at most %d research logs", MaxPromoteLogIDs)
)

// MaxPromoteLogIDs caps how many research logs one promotion may name. It
// mirrors maxItems on PromoteResearchLogsRequest.log_ids in openapi.yaml.
const MaxPromoteLogIDs = 500

// Reasons a research log is skipped by PromoteBranchResearchLogs.
const (
	// PromoteSkipNotFound: the id is not a research log the branch left.
	PromoteSkipNotFound = "not_found"
	// PromoteSkipNotCreatedOnBranch: the log is a mainline entry the branch
	// only edited; it is on main already, and its edit is not replayed.
	PromoteSkipNotCreatedOnBranch = "not_created_on_branch"
	// PromoteSkipAlreadyPromoted: the log was promoted before. It may be on
	// main still, or have been deleted there since; either way it is not
	// written again.
	PromoteSkipAlreadyPromoted = "already_promoted"
	// PromoteSkipSubjectNotOnMain: the log's subject does not exist on main
	// (created only on the branch, or since deleted), so promoting it would
	// leave a dangling reference.
	PromoteSkipSubjectNotOnMain = "subject_not_on_main"
)

// promoteExpectedVersion is the version a promoted log's stream must have on
// main: none. Unlike -1 (any version), it makes the event store refuse a
// second ResearchLogCreated on main for the same id.
const promoteExpectedVersion int64 = 0

// PromoteSkipped is one research log PromoteBranchResearchLogs did not copy.
type PromoteSkipped struct {
	ID     uuid.UUID
	Reason string
}

// PromoteResearchLogsResult reports what a promotion copied and what it left.
type PromoteResearchLogsResult struct {
	Promoted []uuid.UUID
	Skipped  []PromoteSkipped
	// Truncated passes through the archive's flag: the branch has more events
	// than the reconstruction reads, so some logs may not have been offered.
	Truncated bool
}

// PromoteBranchResearchLogs copies research logs a closed branch recorded to
// the mainline (#836, ADR-005 §Closing a branch), so a negative search keeps
// its place in main's reasonably exhaustive search after the branch is gone.
//
// The logs are read from the branch's reconstructed research archive (the
// overlay is purged on close). logIDs selects which to promote; nil or empty
// promotes every eligible one; more than MaxPromoteLogIDs is refused with
// ErrTooManyPromoteLogIDs. Each promoted log is written as an ordinary
// ResearchLogCreated on main, keeping the branch entry's id, and appended with
// an expected version of 0 on main — so a second promotion of the same log,
// concurrent or later, and even after the promoted log was deleted on main, is
// refused by the event store and reported as already promoted. It carries a
// note recording the branch, the outcome it was
// closed with and the reason.
//
// Dangling references are never created: a log whose subject does not exist
// on main is skipped, as is a log the branch only edited (it is main's own).
// Logs are promoted one at a time; a failure other than a skip stops the
// promotion and returns the error with the logs promoted so far.
func (h *Handler) PromoteBranchResearchLogs(ctx context.Context, branchID uuid.UUID, logIDs []uuid.UUID) (*PromoteResearchLogsResult, error) {
	if h.branchStore == nil {
		return nil, ErrBranchStoreRequired
	}
	if !h.branchID.IsMain() {
		return nil, ErrPromoteNotOnMain
	}
	if len(logIDs) > MaxPromoteLogIDs {
		return nil, ErrTooManyPromoteLogIDs
	}
	branch, err := h.branchStore.Get(ctx, branchID)
	if err != nil {
		return nil, err // includes repository.ErrBranchNotFound
	}
	if branch.Status != domain.BranchStatusArchived {
		return nil, fmt.Errorf("%w: %s", ErrBranchNotClosed, branch.Status)
	}

	archive, err := h.branchService.BranchResearchArchive(ctx, branchID)
	if err != nil {
		return nil, fmt.Errorf("reconstructing branch research: %w", err)
	}

	result := &PromoteResearchLogsResult{Truncated: archive.Truncated}
	selected, missing := selectArchivedLogs(archive.ResearchLogs, logIDs)
	for _, id := range missing {
		result.Skipped = append(result.Skipped, PromoteSkipped{ID: id, Reason: PromoteSkipNotFound})
	}

	for _, entry := range selected {
		reason, err := h.promoteResearchLog(ctx, branch, entry)
		if err != nil {
			return result, err
		}
		if reason != "" {
			result.Skipped = append(result.Skipped, PromoteSkipped{ID: entry.ID, Reason: reason})
			continue
		}
		result.Promoted = append(result.Promoted, entry.ID)
	}
	return result, nil
}

// selectArchivedLogs returns the archived logs named by ids, in archive order,
// and the ids that name none. Empty ids selects every log.
func selectArchivedLogs(logs []query.ArchivedResearchLog, ids []uuid.UUID) ([]query.ArchivedResearchLog, []uuid.UUID) {
	if len(ids) == 0 {
		return logs, nil
	}
	wanted := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	var selected []query.ArchivedResearchLog
	for _, entry := range logs {
		if wanted[entry.ID] {
			selected = append(selected, entry)
			delete(wanted, entry.ID)
		}
	}
	var missing []uuid.UUID
	for _, id := range ids {
		if wanted[id] {
			missing = append(missing, id)
			delete(wanted, id)
		}
	}
	return selected, missing
}

// promoteResearchLog writes one archived log to main. It returns a skip
// reason, or "" once the log is promoted.
func (h *Handler) promoteResearchLog(ctx context.Context, branch *domain.Branch, entry query.ArchivedResearchLog) (string, error) {
	if !entry.CreatedOnBranch {
		return PromoteSkipNotCreatedOnBranch, nil
	}
	existing, err := h.readStore.GetResearchLog(ctx, domain.MainBranchID, entry.ID)
	if err != nil {
		return "", fmt.Errorf("checking research log %s on main: %w", entry.ID, err)
	}
	if existing != nil {
		return PromoteSkipAlreadyPromoted, nil
	}
	onMain, err := h.subjectOnMain(ctx, entry.SubjectType, entry.SubjectID)
	if err != nil {
		return "", err
	}
	if !onMain {
		return PromoteSkipSubjectNotOnMain, nil
	}

	notes := ""
	if entry.Notes != nil {
		notes = *entry.Notes
	}
	log := &domain.ResearchLog{
		ID:                entry.ID,
		SubjectID:         entry.SubjectID,
		SubjectType:       entry.SubjectType,
		Repository:        entry.Repository,
		SearchDescription: entry.SearchDescription,
		Outcome:           domain.ResearchOutcome(entry.Outcome),
		Notes:             promotedLogNotes(notes, branch),
		SearchDate:        entry.SearchDate,
		Version:           1,
	}
	if err := log.Validate(); err != nil {
		return "", fmt.Errorf("%w: research log %s: %v", ErrInvalidInput, entry.ID, err)
	}
	event := domain.NewResearchLogCreated(log)
	// Expected version 0 claims "main holds no events for this log". The event
	// store refuses the append when main already does: a rival promotion that
	// landed between the read-model check and here, or an earlier promotion
	// whose log has since been deleted on main (the read model no longer shows
	// it, but its stream does). Either way the log is not written again.
	if _, err := h.execute(ctx, log.ID.String(), "ResearchLog", []domain.Event{event}, promoteExpectedVersion); err != nil {
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			return PromoteSkipAlreadyPromoted, nil
		}
		return "", fmt.Errorf("promoting research log %s: %w", entry.ID, err)
	}
	return "", nil
}

// subjectOnMain reports whether a research log's subject exists on main.
func (h *Handler) subjectOnMain(ctx context.Context, subjectType string, id uuid.UUID) (bool, error) {
	switch subjectType {
	case "person":
		p, err := h.readStore.GetPerson(ctx, domain.MainBranchID, id)
		if err != nil {
			return false, fmt.Errorf("checking person %s on main: %w", id, err)
		}
		return p != nil, nil
	case "family":
		f, err := h.readStore.GetFamily(ctx, domain.MainBranchID, id)
		if err != nil {
			return false, fmt.Errorf("checking family %s on main: %w", id, err)
		}
		return f != nil, nil
	default:
		return false, nil
	}
}

// promotedLogNotes appends the provenance of a promoted log to its notes: the
// branch it was recorded on, the outcome the branch was closed with and the
// reason, so the entry reads correctly on main without the branch.
func promotedLogNotes(notes string, branch *domain.Branch) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(notes))
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	fmt.Fprintf(&b, "Recorded on research branch %q, closed as %s.", branch.Name, branch.Outcome.OrDefault())
	if hypothesis := strings.TrimSpace(branch.Hypothesis); hypothesis != "" {
		fmt.Fprintf(&b, " Hypothesis: %s", hypothesis)
	}
	if reason := strings.TrimSpace(branch.CloseReason); reason != "" {
		fmt.Fprintf(&b, " Reason: %s", reason)
	}
	return b.String()
}
