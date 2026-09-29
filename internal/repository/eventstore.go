// Package repository provides data access interfaces and implementations.
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

// Common errors for event store operations.
var (
	ErrStreamNotFound      = errors.New("stream not found")
	ErrConcurrencyConflict = errors.New("concurrency conflict: expected version mismatch")
	ErrEventNotFound       = errors.New("event not found")
)

// AppendScope is the branch scope of an append (ADR-005). It collapses the
// branch-related arguments into one value so Append's positional list stays
// readable as branching grows.
//
// BranchID names the branch the appended events belong to.
//
// A branch's FIRST write to an aggregate that already exists on main seeds the
// branch's version line from the version the branch's READ shows for that
// aggregate, so the branch continues the aggregate's numbering instead of
// restarting at 1 and the expected version a branch append accepts is exactly
// the version a branch read displayed (#844). Until the branch appends to an
// aggregate's stream, the read model serves one of two rows for it:
//
//   - main's CURRENT row, through the live copy-on-write overlay (ADR-005 §The
//     model). This is the usual case, and the seed is main's current stream
//     version, which the store reads inside the append itself. It is not main's
//     version as of the branch's fork point: seeding from the fork point made
//     every aggregate main edited after the fork uneditable from the branch.
//   - the branch's OWN shadow row, written by the projection of an event on
//     ANOTHER stream (a cross-stream shadow: a branch citation bumps its source's
//     citation count by saving a branch copy of the source row). That copy keeps
//     the version the source had when it was copied, and it keeps shadowing main
//     after main edits the source again. Only the read model knows it exists, so
//     the caller reports its version in OverlayVersion.
//
// OverlayVersion is therefore the version of the branch's cross-stream shadow
// row for the stream, or 0 when the branch reads the stream from main. It is
// consulted only for the branch's first append to the stream (see
// BranchSeedVersion); once the branch has appended, its own version line
// governs. Main's post-fork events still sit after the branch's base position,
// so the merge compare classifies them as main changes exactly as before.
// Nothing in the append depends on the fork point, so the scope carries none.
//
// The ZERO VALUE IS MainScope, and deliberately so: domain.MainBranchID is the
// zero UUID (ADR-005 §Sub-decision 3), so a forgotten scope argument falls back
// to the mainline rather than to an arbitrary branch. Code that must be
// branch-scoped therefore fails loudly (writes land on main) instead of
// silently corrupting some other branch's overlay.
type AppendScope struct {
	BranchID       domain.BranchID
	OverlayVersion int64
}

// ErrInvalidOverlayVersion is returned by a branch's first append to a stream
// when the scope's OverlayVersion names a version main's stream never reached.
// A cross-stream shadow row copies a row main served, so its version can never
// be ahead of main's; one that is signals a caller bug, not a stale client.
var ErrInvalidOverlayVersion = errors.New("overlay version is ahead of the stream's main version")

// BranchSeedVersion is the version a branch's FIRST append to a stream continues
// from, given main's current version of that stream (0 when main has none). It
// is the one seeding rule every EventStore backend applies (DB-001), so the
// backends differ only in how they read mainVersion: the branch's cross-stream
// shadow row version when scope reports one, otherwise main's current version —
// in both cases the version the branch's read shows (see AppendScope).
func BranchSeedVersion(mainVersion int64, scope AppendScope) (int64, error) {
	if scope.OverlayVersion <= 0 {
		return mainVersion, nil
	}
	if scope.OverlayVersion > mainVersion {
		return 0, fmt.Errorf("%w: overlay version %d, main version %d", ErrInvalidOverlayVersion, scope.OverlayVersion, mainVersion)
	}
	return scope.OverlayVersion, nil
}

// MainScope is the mainline append scope: the reserved main branch. Every
// non-branch call site passes this. Struct values can't be const, so this is a
// var — treat it as immutable. It is spelled out rather than left implicit even
// though AppendScope{} equals it (see the type's doc comment), because an
// explicit MainScope at a call site states intent.
var MainScope = AppendScope{BranchID: domain.MainBranchID}

// EventStore provides append-only storage for domain events.
type EventStore interface {
	// Append adds events to a stream with optimistic concurrency control.
	// Returns ErrConcurrencyConflict if expectedVersion doesn't match the current
	// version OF THAT STREAM ON scope.BranchID. Use expectedVersion=-1 to expect no
	// prior events for the stream on that branch.
	//
	// scope (last param, matching Projector.Project's branch-last convention) tags
	// every appended event with its branch; pass MainScope for the mainline. Versioning is per-(streamID, branch): a
	// branch append never contends with main, and main never contends with a
	// branch. Divergence surfaces at merge time, not at write time (ADR-005).
	Append(ctx context.Context, streamID uuid.UUID, streamType string, events []domain.Event, expectedVersion int64, scope AppendScope) error

	// ReadStream reads all events for a specific aggregate, across ALL branches.
	// Branch-scoped writes reuse the same streamID as main (branch is carried on
	// StoredEvent.BranchID, not the stream id), so the returned slice interleaves
	// main and every branch's events for that aggregate. A caller wanting a single
	// branch's view must filter the result on StoredEvent.BranchID.
	ReadStream(ctx context.Context, streamID uuid.UUID) ([]StoredEvent, error)

	// ReadAll reads all events from a position for projection rebuilds.
	ReadAll(ctx context.Context, fromPosition int64, limit int) ([]StoredEvent, error)

	// ReadBranch reads a single branch's OWN events — those tagged with branchID,
	// which for a non-main branch means the branch's deltas only, not the main
	// events it is layered over. Results have position > fromPosition (exclusive,
	// matching ReadAll), are ordered by position ascending, and are capped at limit.
	// Pass domain.MainBranchID to read the mainline's own events.
	ReadBranch(ctx context.Context, branchID domain.BranchID, fromPosition int64, limit int) ([]StoredEvent, error)

	// ReadStreamsForBranch reads one branch's events for a SET of streams in a
	// single query — the set-based counterpart to ReadStream, for callers that
	// would otherwise loop it once per aggregate (branch compare, merge conflict
	// detection). Results are restricted to branchID, have position > fromPosition
	// (exclusive, matching ReadAll/ReadBranch), are ordered by position ascending,
	// and are capped at limit. Ordering and the cap are applied by the STORE, so
	// the limit bounds the work and not just the response. An empty streamIDs, or
	// a non-positive limit, yields no events and issues no query.
	ReadStreamsForBranch(ctx context.Context, streamIDs []uuid.UUID, branchID domain.BranchID, fromPosition int64, limit int) ([]StoredEvent, error)

	// GetStreamVersion returns the current version of a stream on a branch —
	// the max version for that (streamID, branchID) pair, or 0 if the branch has
	// no events for the stream. Pass domain.MainBranchID for the mainline.
	GetStreamVersion(ctx context.Context, streamID uuid.UUID, branchID domain.BranchID) (int64, error)

	// ReadByStream returns paginated events for a specific stream (entity) ON ONE
	// BRANCH. Results are ordered by version ascending.
	//
	// The branch filter is applied IN THE STORE, before pagination: HistoryPage
	// carries TotalCount and HasMore, so filtering after the fact would report
	// another branch's events in the totals of this one. Without the filter a
	// branch edit would surface in main's entity history, which is the isolation
	// leak ADR-005 exists to prevent.
	//
	// Parameters:
	//   - streamID: The UUID of the stream to query
	//   - branchID: The branch scope; pass domain.MainBranchID for the mainline
	//   - limit: Maximum number of events to return
	//   - offset: Number of events to skip (for pagination)
	// Returns a HistoryPage with events, total count, and hasMore flag.
	ReadByStream(ctx context.Context, streamID uuid.UUID, branchID domain.BranchID, limit, offset int) (*HistoryPage, error)

	// ReadGlobalHistory returns one page of the global change history, with
	// every filter it needs applied IN THE STORE, before pagination, so the page and
	// its TotalCount/HasMore are computed over the same set of events (#739).
	// See GlobalHistoryQuery for the filters. Results are ordered by timestamp,
	// then position, ascending. The timestamp is the event's record time: its
	// OccurredAt, except for a merge's replayed event, which is recorded at the
	// merge (domain.MergeProvenance), so it sorts and filters by when it
	// reached the mainline.
	ReadGlobalHistory(ctx context.Context, q GlobalHistoryQuery) (*HistoryPage, error)
}

// GlobalHistoryQuery filters and paginates ReadGlobalHistory. Every filter is
// evaluated by the store before LIMIT/OFFSET.
type GlobalHistoryQuery struct {
	// FromTime and ToTime bound the event timestamp, inclusive. A zero value
	// leaves that side unbounded.
	FromTime time.Time
	ToTime   time.Time
	// IncludeEventTypes, when non-empty, keeps only these event types.
	IncludeEventTypes []string
	// ExcludeEventTypes drops these event types. Applied after the include
	// list, so a type named in both is dropped.
	ExcludeEventTypes []string
	// BranchID, when non-nil, keeps only that branch's own events. Pass
	// &domain.MainBranchID for the mainline's history (ADR-005). Nil reads
	// every branch.
	BranchID *domain.BranchID
	// AnyBranchEventTypes are kept whatever branch they are on, even when
	// BranchID narrows the rest to one branch: the branch lifecycle events,
	// which live on each branch's own scope but belong in the mainline's
	// history (#832). Ignored when BranchID is nil. The type filters above
	// still apply to them.
	AnyBranchEventTypes []string
	// Limit caps the page (non-positive yields an empty page); Offset skips.
	Limit  int
	Offset int
}

// StoredEvent represents an event as stored in the event store.
type StoredEvent struct {
	ID         uuid.UUID       `json:"id"`
	StreamID   uuid.UUID       `json:"stream_id"`
	StreamType string          `json:"stream_type"`
	BranchID   domain.BranchID `json:"branch_id"`
	EventType  string          `json:"event_type"`
	Data       json.RawMessage `json:"data"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
	Version    int64           `json:"version"`
	Position   int64           `json:"position"`
	Timestamp  time.Time       `json:"timestamp"`
}

// HistoryPage represents a paginated result set of events for history queries.
type HistoryPage struct {
	Events     []StoredEvent `json:"events"`
	TotalCount int           `json:"total_count"`
	HasMore    bool          `json:"has_more"`
}

// DecodeEvent decodes the stored event data into a domain event.
func (e *StoredEvent) DecodeEvent() (domain.Event, error) {
	switch e.EventType {
	case "PersonCreated":
		var event domain.PersonCreated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "PersonUpdated":
		var event domain.PersonUpdated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "PersonDeleted":
		var event domain.PersonDeleted
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "FamilyCreated":
		var event domain.FamilyCreated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "FamilyUpdated":
		var event domain.FamilyUpdated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "ChildLinkedToFamily":
		var event domain.ChildLinkedToFamily
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "ChildUnlinkedFromFamily":
		var event domain.ChildUnlinkedFromFamily
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "FamilyDeleted":
		var event domain.FamilyDeleted
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "GedcomImported":
		var event domain.GedcomImported
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "SourceCreated":
		var event domain.SourceCreated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "SourceUpdated":
		var event domain.SourceUpdated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "SourceDeleted":
		var event domain.SourceDeleted
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "CitationCreated":
		var event domain.CitationCreated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "CitationUpdated":
		var event domain.CitationUpdated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "CitationDeleted":
		var event domain.CitationDeleted
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "MediaCreated":
		var event domain.MediaCreated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "MediaUpdated":
		var event domain.MediaUpdated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "MediaDeleted":
		var event domain.MediaDeleted
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "NameAdded":
		var event domain.NameAdded
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "NameUpdated":
		var event domain.NameUpdated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "NameRemoved":
		var event domain.NameRemoved
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "SnapshotCreated":
		var event domain.SnapshotCreated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "SnapshotDeleted":
		var event domain.SnapshotDeleted
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "BranchCreated":
		var event domain.BranchCreated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "BranchDeleted":
		var event domain.BranchDeleted
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "BranchMerged":
		var event domain.BranchMerged
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "BranchMergeResumed":
		var event domain.BranchMergeResumed
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "PersonMerged":
		var event domain.PersonMerged
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "NoteCreated":
		var event domain.NoteCreated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "NoteUpdated":
		var event domain.NoteUpdated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "NoteDeleted":
		var event domain.NoteDeleted
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "SubmitterCreated":
		var event domain.SubmitterCreated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "SubmitterUpdated":
		var event domain.SubmitterUpdated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "SubmitterDeleted":
		var event domain.SubmitterDeleted
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "AssociationCreated":
		var event domain.AssociationCreated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "AssociationUpdated":
		var event domain.AssociationUpdated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "AssociationDeleted":
		var event domain.AssociationDeleted
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "LDSOrdinanceCreated":
		var event domain.LDSOrdinanceCreated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "LDSOrdinanceUpdated":
		var event domain.LDSOrdinanceUpdated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "LDSOrdinanceDeleted":
		var event domain.LDSOrdinanceDeleted
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "RepositoryCreated":
		var event domain.RepositoryCreated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "RepositoryUpdated":
		var event domain.RepositoryUpdated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "RepositoryDeleted":
		var event domain.RepositoryDeleted
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "LifeEventCreated":
		var event domain.LifeEventCreated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "LifeEventUpdated":
		var event domain.LifeEventUpdated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "LifeEventDeleted":
		var event domain.LifeEventDeleted
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "AttributeCreated":
		var event domain.AttributeCreated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "AttributeUpdated":
		var event domain.AttributeUpdated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "AttributeDeleted":
		var event domain.AttributeDeleted
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "EvidenceAnalysisCreated":
		var event domain.EvidenceAnalysisCreated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "EvidenceAnalysisUpdated":
		var event domain.EvidenceAnalysisUpdated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "EvidenceAnalysisDeleted":
		var event domain.EvidenceAnalysisDeleted
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "EvidenceConflictDetected":
		var event domain.EvidenceConflictDetected
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "EvidenceConflictResolved":
		var event domain.EvidenceConflictResolved
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "ResearchLogCreated":
		var event domain.ResearchLogCreated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "ResearchLogUpdated":
		var event domain.ResearchLogUpdated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "ResearchLogDeleted":
		var event domain.ResearchLogDeleted
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "ProofSummaryCreated":
		var event domain.ProofSummaryCreated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "ProofSummaryUpdated":
		var event domain.ProofSummaryUpdated
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	case "ProofSummaryDeleted":
		var event domain.ProofSummaryDeleted
		if err := json.Unmarshal(e.Data, &event); err != nil {
			return nil, err
		}
		return event, nil
	default:
		return nil, errors.New("unknown event type: " + e.EventType)
	}
}

// EncodeEvent creates a StoredEvent from a domain event.
func EncodeEvent(streamID uuid.UUID, streamType string, event domain.Event, version, position int64) (StoredEvent, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return StoredEvent{}, err
	}

	return StoredEvent{
		ID:         uuid.New(),
		StreamID:   streamID,
		StreamType: streamType,
		EventType:  event.EventType(),
		Data:       data,
		Version:    version,
		Position:   position,
		Timestamp:  event.OccurredAt(),
	}, nil
}
