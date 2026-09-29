package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// BranchID is the branch SCOPE attached to reads, writes, and stored events
// (ADR-005). It is a distinct type from the branch ENTITY's identity
// (Branch.ID, a uuid.UUID) so that a scope can never be silently transposed
// with an entity id in a call — the compiler rejects the mismatch. Convert to
// the underlying uuid.UUID with UUID() at DB boundaries; wrap a branch entity
// id into a scope with BranchID(branch.ID).
type BranchID uuid.UUID

// UUID returns the underlying uuid.UUID for DB binds and comparisons.
func (b BranchID) UUID() uuid.UUID { return uuid.UUID(b) }

// String returns the canonical string form of the branch scope.
func (b BranchID) String() string { return uuid.UUID(b).String() }

// IsMain reports whether this scope is the reserved mainline.
func (b BranchID) IsMain() bool { return b == MainBranchID }

// MainBranchID is the reserved branch scope for the mainline of research.
// It is fixed as the zero UUID: a zero value means "main". Downstream code
// cites this constant rather than re-deciding the reserved value (ADR-005).
// uuid values can't be const, so this is a var — treat it as immutable.
var MainBranchID = BranchID(uuid.Nil)

// Branch validation errors.
var (
	ErrBranchNameRequired     = errors.New("branch name is required")
	ErrBranchNameTooLong      = errors.New("branch name must be 100 characters or less")
	ErrBranchDescTooLong      = errors.New("branch description must be 500 characters or less")
	ErrBranchInvalidStatus    = errors.New("branch status is invalid")
	ErrBranchMergeNoteTooLong = errors.New("branch merge note must be 1000 characters or less")
	// ErrResolutionRationaleTooLong is returned for a merge resolution whose
	// rationale exceeds MaxResolutionRationaleLength characters.
	ErrResolutionRationaleTooLong = errors.New("merge resolution rationale must be 1000 characters or less")

	// Research-record errors (#835).
	ErrBranchHypothesisTooLong     = errors.New("branch hypothesis must be 2000 characters or less")
	ErrBranchInvalidOutcome        = errors.New("branch outcome must be one of open, proved, disproved, inconclusive, superseded, abandoned")
	ErrBranchTooManySubjects       = errors.New("a branch may name at most 50 subjects")
	ErrBranchInvalidSubject        = errors.New("branch subject must be a person or family with an id")
	ErrBranchDuplicateSubject      = errors.New("branch subjects must not repeat")
	ErrBranchTooManyProofSummaries = errors.New("a branch may link at most 20 proof summaries")
	ErrBranchInvalidProofSummaryID = errors.New("branch proof summary ids must be non-empty")
	ErrBranchDuplicateProofSummary = errors.New("branch proof summary ids must not repeat")

	// Close errors (#836).
	ErrBranchInvalidCloseOutcome = errors.New("a branch is closed as disproved, inconclusive, superseded or abandoned")
	ErrBranchCloseReasonTooLong  = errors.New("branch close reason must be 2000 characters or less")
)

// MaxResolutionRationaleLength bounds the optional rationale a merge records
// for one conflict resolution (#828), in characters.
const MaxResolutionRationaleLength = 1000

// NormalizeResolutionRationales trims every rationale, drops the blank ones and
// rejects one longer than MaxResolutionRationaleLength characters. It returns
// nil when nothing is left, so an event carrying the result omits the field.
func NormalizeResolutionRationales(rationales map[uuid.UUID]string) (map[uuid.UUID]string, error) {
	var out map[uuid.UUID]string
	for streamID, rationale := range rationales {
		rationale = strings.TrimSpace(rationale)
		if rationale == "" {
			continue
		}
		if utf8.RuneCountInString(rationale) > MaxResolutionRationaleLength {
			return nil, fmt.Errorf("%w: stream %s", ErrResolutionRationaleTooLong, streamID)
		}
		if out == nil {
			out = make(map[uuid.UUID]string)
		}
		out[streamID] = rationale
	}
	return out, nil
}

// Research-record limits (#835). Lengths are counted in characters (runes),
// not bytes, so a hypothesis written in any script gets the same budget.
const (
	MaxBranchHypothesisLength = 2000
	MaxBranchSubjects         = 50
	MaxBranchProofSummaries   = 20
	// MaxBranchCloseReasonLength caps the reason recorded when a branch is
	// closed without merging (#836).
	MaxBranchCloseReasonLength = 2000
)

// BranchOutcome is the verdict a line of research reached (#835). It is kept
// independent of BranchStatus on purpose: status is the branch's lifecycle
// (can it still take writes?), outcome is what the research concluded. A
// merged branch is usually "proved", but a branch can be merged as
// "inconclusive" or archived as "disproved"; closing a branch without merging
// (#836) records one of the close outcomes (see IsCloseOutcome).
type BranchOutcome string

const (
	// BranchOutcomeOpen is the default: the question is still being worked.
	BranchOutcomeOpen         BranchOutcome = "open"
	BranchOutcomeProved       BranchOutcome = "proved"
	BranchOutcomeDisproved    BranchOutcome = "disproved"
	BranchOutcomeInconclusive BranchOutcome = "inconclusive"
	// BranchOutcomeSuperseded marks a question overtaken by other research.
	BranchOutcomeSuperseded BranchOutcome = "superseded"
	// BranchOutcomeAbandoned marks research stopped without a verdict (#836):
	// the line was dropped, not answered. The registry also records it for a
	// close that carried no outcome (a BranchDeleted written before #836) on a
	// branch that had reached no verdict, so no closed branch reads as "open".
	BranchOutcomeAbandoned BranchOutcome = "abandoned"
)

// BranchOutcomes lists every valid outcome in display order.
func BranchOutcomes() []BranchOutcome {
	return []BranchOutcome{
		BranchOutcomeOpen,
		BranchOutcomeProved,
		BranchOutcomeDisproved,
		BranchOutcomeInconclusive,
		BranchOutcomeSuperseded,
		BranchOutcomeAbandoned,
	}
}

// IsValid checks if the outcome value is one of the defined outcomes.
func (o BranchOutcome) IsValid() bool {
	switch o {
	case BranchOutcomeOpen, BranchOutcomeProved, BranchOutcomeDisproved,
		BranchOutcomeInconclusive, BranchOutcomeSuperseded, BranchOutcomeAbandoned:
		return true
	default:
		return false
	}
}

// IsCloseOutcome reports whether the outcome can be recorded when a branch is
// closed without merging (#836): disproved, inconclusive, superseded or
// abandoned. "open" is no verdict, and a "proved" branch is merged, not closed.
func (o BranchOutcome) IsCloseOutcome() bool {
	switch o {
	case BranchOutcomeDisproved, BranchOutcomeInconclusive,
		BranchOutcomeSuperseded, BranchOutcomeAbandoned:
		return true
	default:
		return false
	}
}

// ValidateBranchClose checks the outcome and reason given when a branch is
// closed (#836). The reason is optional; its length is counted in characters.
func ValidateBranchClose(outcome BranchOutcome, reason string) error {
	if !outcome.IsCloseOutcome() {
		return ErrBranchInvalidCloseOutcome
	}
	if utf8.RuneCountInString(reason) > MaxBranchCloseReasonLength {
		return ErrBranchCloseReasonTooLong
	}
	return nil
}

// OrDefault returns the outcome, or BranchOutcomeOpen when it is empty. A
// branch created before #835 carries no outcome in its event or its registry
// row; it is read as "open".
func (o BranchOutcome) OrDefault() BranchOutcome {
	if o == "" {
		return BranchOutcomeOpen
	}
	return o
}

// BranchSubjectType names the kind of record a branch's hypothesis concerns.
type BranchSubjectType string

const (
	BranchSubjectPerson BranchSubjectType = "person"
	BranchSubjectFamily BranchSubjectType = "family"
)

// IsValid checks if the subject type is person or family.
func (t BranchSubjectType) IsValid() bool {
	return t == BranchSubjectPerson || t == BranchSubjectFamily
}

// BranchSubject is one person or family a branch's hypothesis is about. The
// type travels with the id so a subject can be linked and resolved without
// probing both tables.
type BranchSubject struct {
	Type BranchSubjectType `json:"type"`
	ID   uuid.UUID         `json:"id"`
}

// BranchStatus represents the lifecycle state of a branch.
type BranchStatus string

const (
	BranchStatusActive BranchStatus = "active"
	BranchStatusMerged BranchStatus = "merged"
	// BranchStatusArchived is the terminal state a branch enters on delete/discard.
	// Note the deliberate vocabulary split: the lifecycle *event* is named
	// BranchDeleted (a "delete branch" action) but the resulting *status* is
	// "archived" — the branch record and its history are retained (append-only,
	// ES-002), only its overlay rows are purged. The UI calls the action "Close
	// branch" (#836): closing records an outcome and a reason on the event.
	BranchStatusArchived BranchStatus = "archived"
)

// IsValid checks if the branch status value is valid.
func (s BranchStatus) IsValid() bool {
	switch s {
	case BranchStatusActive, BranchStatusMerged, BranchStatusArchived:
		return true
	default:
		return false
	}
}

// Branch is a lightweight record marking an isolated line of research off a
// point on main. BasePosition is a main global Position — the same
// base-pointer concept as a Snapshot (ADR-005 §The model).
//
// Legal status transitions: active→merged (on a successful merge) and
// active→archived (on discard/delete). merged and archived are terminal —
// a branch in either state accepts no further writes.
type Branch struct {
	ID           uuid.UUID    `json:"id"`
	Name         string       `json:"name"`
	Description  string       `json:"description,omitempty"`
	BasePosition int64        `json:"base_position"`
	Status       BranchStatus `json:"status"`
	CreatedAt    time.Time    `json:"created_at"`

	// MergedAt and MergeNote are the registry's copy of the merge record, set
	// only on the active→merged transition so a merged branch can be reviewed
	// without replaying the log for its BranchMerged event (#55). They stay
	// nil/empty for active and archived branches — a pointer for MergedAt so
	// "never merged" is distinguishable from the zero time.
	MergedAt  *time.Time `json:"merged_at,omitempty"`
	MergeNote string     `json:"merge_note,omitempty"`

	// ClosedAt and CloseReason are the registry's copy of the close record
	// (#836), set on the active→archived transition from the BranchDeleted
	// event. ClosedAt is nil for active and merged branches; CloseReason is
	// empty when none was given (always, for a branch closed before #836).
	// The outcome the branch was closed with is stored in Outcome.
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
	CloseReason string     `json:"close_reason,omitempty"`

	// The research record (#835): the question the branch explores, the
	// persons/families it concerns, the verdict it reached and the proof
	// summaries that argue that verdict. Set at creation and edited through
	// BranchUpdated; Outcome is never empty on a validated branch.
	Hypothesis      string          `json:"hypothesis,omitempty"`
	Subjects        []BranchSubject `json:"subjects,omitempty"`
	Outcome         BranchOutcome   `json:"outcome"`
	ProofSummaryIDs []uuid.UUID     `json:"proof_summary_ids,omitempty"`
}

// BranchResearch is the editable research record of a branch (#835), bundled
// so the create command, the update event and the registry write carry the
// same shape.
type BranchResearch struct {
	Hypothesis      string
	Subjects        []BranchSubject
	Outcome         BranchOutcome
	ProofSummaryIDs []uuid.UUID
}

// Research returns the branch's research record. The slices are copied so a
// caller cannot mutate the branch through the result.
func (b *Branch) Research() BranchResearch {
	return BranchResearch{
		Hypothesis:      b.Hypothesis,
		Subjects:        append([]BranchSubject(nil), b.Subjects...),
		Outcome:         b.Outcome,
		ProofSummaryIDs: append([]uuid.UUID(nil), b.ProofSummaryIDs...),
	}
}

// ApplyResearch overwrites the branch's research record with r, copying its
// slices. An empty outcome becomes BranchOutcomeOpen.
func (b *Branch) ApplyResearch(r BranchResearch) {
	b.Hypothesis = r.Hypothesis
	b.Subjects = append([]BranchSubject(nil), r.Subjects...)
	b.Outcome = r.Outcome.OrDefault()
	b.ProofSummaryIDs = append([]uuid.UUID(nil), r.ProofSummaryIDs...)
}

// Validate checks the research record's field values: the hypothesis length,
// the outcome, and that subjects and proof summary ids are well formed, within
// their caps and free of repeats. It does not check that the referenced
// records exist — that needs the read model and is the command's job.
func (r BranchResearch) Validate() error {
	if utf8.RuneCountInString(r.Hypothesis) > MaxBranchHypothesisLength {
		return ErrBranchHypothesisTooLong
	}
	if !r.Outcome.IsValid() {
		return ErrBranchInvalidOutcome
	}
	if len(r.Subjects) > MaxBranchSubjects {
		return ErrBranchTooManySubjects
	}
	seenSubjects := make(map[BranchSubject]struct{}, len(r.Subjects))
	for _, subject := range r.Subjects {
		if !subject.Type.IsValid() || subject.ID == uuid.Nil {
			return ErrBranchInvalidSubject
		}
		if _, dup := seenSubjects[subject]; dup {
			return ErrBranchDuplicateSubject
		}
		seenSubjects[subject] = struct{}{}
	}
	if len(r.ProofSummaryIDs) > MaxBranchProofSummaries {
		return ErrBranchTooManyProofSummaries
	}
	seenProofs := make(map[uuid.UUID]struct{}, len(r.ProofSummaryIDs))
	for _, id := range r.ProofSummaryIDs {
		if id == uuid.Nil {
			return ErrBranchInvalidProofSummaryID
		}
		if _, dup := seenProofs[id]; dup {
			return ErrBranchDuplicateProofSummary
		}
		seenProofs[id] = struct{}{}
	}
	return nil
}

// NewBranch creates a new active Branch with validation.
func NewBranch(name, description string, basePosition int64) (*Branch, error) {
	b := &Branch{
		ID:           uuid.New(),
		Name:         name,
		Description:  description,
		BasePosition: basePosition,
		Status:       BranchStatusActive,
		Outcome:      BranchOutcomeOpen,
		CreatedAt:    time.Now().UTC(),
	}

	if err := b.Validate(); err != nil {
		return nil, err
	}

	return b, nil
}

// Validate checks that the branch has valid field values. Text limits count
// characters (runes), not bytes, matching the API's maxLength and the
// PostgreSQL VARCHAR columns.
func (b *Branch) Validate() error {
	if b.Name == "" {
		return ErrBranchNameRequired
	}
	if utf8.RuneCountInString(b.Name) > 100 {
		return ErrBranchNameTooLong
	}
	if utf8.RuneCountInString(b.Description) > 500 {
		return ErrBranchDescTooLong
	}
	if !b.Status.IsValid() {
		return ErrBranchInvalidStatus
	}
	if utf8.RuneCountInString(b.MergeNote) > 1000 {
		return ErrBranchMergeNoteTooLong
	}
	if utf8.RuneCountInString(b.CloseReason) > MaxBranchCloseReasonLength {
		return ErrBranchCloseReasonTooLong
	}
	// An empty outcome is a pre-#835 branch and reads as open.
	research := b.Research()
	research.Outcome = research.Outcome.OrDefault()
	return research.Validate()
}

// NewBranchWithResearch creates a new active Branch carrying a research record
// (#835). An empty outcome defaults to open.
func NewBranchWithResearch(name, description string, basePosition int64, research BranchResearch) (*Branch, error) {
	b, err := NewBranch(name, description, basePosition)
	if err != nil {
		return nil, err
	}
	b.ApplyResearch(research)
	if err := b.Validate(); err != nil {
		return nil, err
	}
	return b, nil
}
