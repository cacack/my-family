// Package command provides CQRS command handlers for the genealogy application.
package command

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

// Rollback-related errors.
var (
	ErrRollbackInvalidVersion = errors.New("invalid rollback version: must be positive and less than current version")
	ErrRollbackDeletedEntity  = errors.New("cannot rollback a deleted entity")
	ErrRollbackNoChanges      = errors.New("rollback to current version is a no-op")

	// ErrRollbackNotBranchScoped is returned when a Rollback* command runs on a
	// branch-scoped handler. Rollback reads the current version and the deleted
	// check from main but appends through execute on the handler's scope, so on a
	// branch the expected version comes from main's counter while the append is
	// checked against the branch's independent one. Rolling back within a branch
	// is out of scope for issue #670, so refuse instead of mixing the two scopes.
	ErrRollbackNotBranchScoped = errors.New("rollback is not supported on a branch-scoped handler")
)

// Branch-scope errors.
var (
	// ErrEventTypeNotBranchAware is returned when a branch-scoped handler is
	// asked to emit an event the projector would write main-only. Failing here
	// is deliberate: the alternative is a branch edit silently landing on main.
	ErrEventTypeNotBranchAware = errors.New("event type is not branch-aware")
)

// branchAwareEventTypes is the set of event types whose Projector handlers
// thread the branch scope into their read-model *writes*. It is derived from
// internal/repository/projection.go, not from entity names: an event belongs
// here only when every store call its handler makes is branch-keyed (the
// copy-on-write overlay #669 added for persons, person names, families, family
// children and pedigree edges).
//
// Deliberately excluded despite their handlers taking a branchID:
//   - PersonMerged — branch-scoped for the slice writes, but it also rewrites
//     citations, life events, media, attributes, evidence and research rows that
//     are main-only, so a branch-scoped merge would mutate main.
//   - AssociationCreated, LDSOrdinanceCreated — they read persons on the branch
//     scope to denormalize a name, but save to main-only tables.
//
// Issue #676 (branch fan-out) grows this set as the remaining projections and
// read-model tables become branch-aware. It lives next to the guard that uses
// it so its coupling to the projector stays visible.
var branchAwareEventTypes = map[string]struct{}{
	"PersonCreated":           {},
	"PersonUpdated":           {},
	"PersonDeleted":           {},
	"FamilyCreated":           {},
	"FamilyUpdated":           {},
	"FamilyDeleted":           {},
	"ChildLinkedToFamily":     {},
	"ChildUnlinkedFromFamily": {},
	"NameAdded":               {},
	"NameUpdated":             {},
	"NameRemoved":             {},
}

// BranchAwareEventTypes returns the event types a branch-scoped handler may
// emit, sorted. It is the allowlist execute enforces: a command whose events
// are all in this set honors the branch scope, any other command fails with
// ErrEventTypeNotBranchAware. Exported so callers can discover the branch-aware
// surface — and tests can detect drift from internal/repository/projection.go —
// without reading the unexported map.
func BranchAwareEventTypes() []string {
	types := make([]string, 0, len(branchAwareEventTypes))
	for eventType := range branchAwareEventTypes {
		types = append(types, eventType)
	}
	sort.Strings(types)
	return types
}

// RollbackResult contains the result of a rollback operation.
type RollbackResult struct {
	EntityID   uuid.UUID      `json:"entity_id"`
	EntityType string         `json:"entity_type"`
	NewVersion int64          `json:"new_version"`
	Changes    map[string]any `json:"changes"`
}

// Handler processes commands and returns resulting domain events.
//
// A Handler carries a branch scope (ADR-005). Its zero value is the mainline:
// branchID is domain.MainBranchID and basePosition is 0, which is exactly
// repository.MainScope, so a handler built by any constructor behaves as it did
// before branches existed. WithBranch returns a scoped copy.
type Handler struct {
	eventStore      repository.EventStore
	readStore       repository.ReadModelStore
	branchStore     repository.BranchStore
	positions       MaxPositionReader
	projector       *repository.Projector
	rollbackService *query.RollbackService

	// branchService is built internally, like rollbackService, so MergeBranch
	// can reuse the query layer's merge plan (the same one CompareBranch shows
	// a reviewer) without any constructor gaining an argument.
	branchService *query.BranchService

	// Branch scope applied to every append and projection made through execute.
	branchID     domain.BranchID
	basePosition int64
}

// NewHandler creates a new command handler. Its projector has no branch registry
// store, so branch-lifecycle events (if any were emitted) would no-op; production
// callers that need the registry use NewHandlerWithBranchStore instead.
func NewHandler(eventStore repository.EventStore, readStore repository.ReadModelStore) *Handler {
	return NewHandlerWithBranchStore(eventStore, readStore, nil)
}

// NewHandlerWithBranchStore creates a command handler whose projector routes
// branch-lifecycle events into the given branch registry store. branchStore may
// be nil (equivalent to NewHandler). It supplies no MaxPositionReader, so
// CreateBranch is unavailable — use NewHandlerWithBranches for that.
func NewHandlerWithBranchStore(eventStore repository.EventStore, readStore repository.ReadModelStore, branchStore repository.BranchStore) *Handler {
	return NewHandlerWithBranches(eventStore, readStore, branchStore, nil)
}

// NewHandlerWithBranches creates a command handler wired for the full branch
// lifecycle: branchStore is the branch registry (the projector writes it and
// DeleteBranch reads it), and positions reports the event log's current head,
// which becomes a new branch's base position. repository.SnapshotStore satisfies
// MaxPositionReader. Either argument may be nil; the branch commands then return
// a typed error rather than panicking.
func NewHandlerWithBranches(eventStore repository.EventStore, readStore repository.ReadModelStore, branchStore repository.BranchStore, positions MaxPositionReader) *Handler {
	return &Handler{
		eventStore:      eventStore,
		readStore:       readStore,
		branchStore:     branchStore,
		positions:       positions,
		projector:       repository.NewProjector(readStore, branchStore),
		rollbackService: query.NewRollbackService(eventStore, readStore),
		branchService:   query.NewBranchService(branchStore, eventStore, query.NewHistoryService(eventStore, readStore)),
	}
}

// NewHandlerWithRollbackService creates a new command handler with a custom rollback service.
// This is primarily useful for testing.
func NewHandlerWithRollbackService(eventStore repository.EventStore, readStore repository.ReadModelStore, rollbackService *query.RollbackService) *Handler {
	return &Handler{
		eventStore:      eventStore,
		readStore:       readStore,
		projector:       repository.NewProjector(readStore, nil),
		rollbackService: rollbackService,
		branchService:   query.NewBranchService(nil, eventStore, query.NewHistoryService(eventStore, readStore)),
	}
}

// WithBranch returns a shallow copy of the handler whose writes land on b
// instead of main (ADR-005). Scoping is a copy rather than a parameter so that
// none of the existing command signatures change, and rather than a
// context.Context value so the scope stays explicit at the call site:
//
//	res, err := handler.WithBranch(b).UpdatePerson(ctx, input)
//
// A nil branch returns the handler unchanged (still mainline).
//
// # What honors the scope
//
// Every command that routes through execute appends and projects on the branch.
// execute additionally rejects, with ErrEventTypeNotBranchAware, any event the
// projector would write main-only, so the branch-aware surface is exactly the
// commands whose events are all in BranchAwareEventTypes: CreatePerson,
// UpdatePerson, DeletePerson, AddName, UpdateName, DeleteName, CreateFamily,
// UpdateFamily, DeleteFamily, LinkChild and UnlinkChild.
//
// Every other entity command — sources, citations, media, notes, submitters,
// repositories, associations, LDS ordinances, evidence, research logs, proof
// summaries and MergePersons — routes through execute too, so on a branch it
// fails loudly rather than writing main. Issue #676 moves those onto the branch
// as their projections become branch-aware.
//
// # What ignores the scope
//
//   - ImportGedcom appends and projects with a hardcoded main scope, so it stays
//     mainline on a branch-scoped handler instead of failing. Importing onto a
//     branch is a stated non-goal of #670; the HTTP import route exposes no
//     ?branch= parameter.
//   - CreateBranch, DeleteBranch and MergeBranch take their target branch as an
//     argument and derive their own scopes from it, so the handler's scope is
//     irrelevant. MergeBranch derives two: the branch's own scope for the
//     BranchMerged claim, and repository.MainScope for the replay it writes to
//     main on the branch's behalf.
//
// # What is refused
//
// The Rollback* commands return ErrRollbackNotBranchScoped on a branch-scoped
// handler: they derive the expected version and the deleted check from main, so
// honoring the branch only on the append would mix the two scopes.
func (h *Handler) WithBranch(b *domain.Branch) *Handler {
	if b == nil {
		return h
	}
	scoped := *h
	scoped.branchID = domain.BranchID(b.ID)
	scoped.basePosition = b.BasePosition
	return &scoped
}

// appendScope is the handler's branch scope in event-store form. For an
// unscoped handler this equals repository.MainScope.
func (h *Handler) appendScope() repository.AppendScope {
	return repository.AppendScope{BranchID: h.branchID, BasePosition: h.basePosition}
}

// execute is a helper that appends events, projects them, and returns the new version.
func (h *Handler) execute(ctx context.Context, streamID string, streamType string, events []domain.Event, expectedVersion int64) (int64, error) {
	// Parse stream ID as UUID
	id, err := parseUUID(streamID)
	if err != nil {
		return 0, err
	}

	// On a branch, refuse events the projector would write main-only: appending
	// them would tag the event with the branch but land the projection on main.
	if !h.branchID.IsMain() {
		for _, event := range events {
			if _, ok := branchAwareEventTypes[event.EventType()]; !ok {
				return 0, fmt.Errorf("%w: %s", ErrEventTypeNotBranchAware, event.EventType())
			}
		}
	}

	// Append events to the event store on the handler's branch scope.
	if err := h.eventStore.Append(ctx, id, streamType, events, expectedVersion, h.appendScope()); err != nil {
		return 0, err
	}

	// Project events to read model (synchronous for MVP)
	newVersion := expectedVersion
	if expectedVersion < 0 {
		newVersion = 0
	}
	for _, event := range events {
		newVersion++
		if err := h.projector.Project(ctx, event, newVersion, h.branchID); err != nil {
			// Projection can be rebuilt; ignore non-critical errors
			_ = err
		}
	}

	return newVersion, nil
}

// RollbackPerson rolls back a person to a specific version.
// It computes the changes needed and generates a compensating PersonUpdated event.
func (h *Handler) RollbackPerson(ctx context.Context, personID uuid.UUID, targetVersion int64) (*RollbackResult, error) {
	return h.rollbackEntity(ctx, "Person", personID, targetVersion, func(id uuid.UUID) (bool, error) {
		person, err := h.readStore.GetPerson(ctx, domain.MainBranchID, id)
		if err != nil {
			return false, err
		}
		return person == nil, nil
	})
}

// RollbackFamily rolls back a family to a specific version.
// It computes the changes needed and generates a compensating FamilyUpdated event.
func (h *Handler) RollbackFamily(ctx context.Context, familyID uuid.UUID, targetVersion int64) (*RollbackResult, error) {
	return h.rollbackEntity(ctx, "Family", familyID, targetVersion, func(id uuid.UUID) (bool, error) {
		family, err := h.readStore.GetFamily(ctx, domain.MainBranchID, id)
		if err != nil {
			return false, err
		}
		return family == nil, nil
	})
}

// RollbackSource rolls back a source to a specific version.
// It computes the changes needed and generates a compensating SourceUpdated event.
func (h *Handler) RollbackSource(ctx context.Context, sourceID uuid.UUID, targetVersion int64) (*RollbackResult, error) {
	return h.rollbackEntity(ctx, "Source", sourceID, targetVersion, func(id uuid.UUID) (bool, error) {
		source, err := h.readStore.GetSource(ctx, id)
		if err != nil {
			return false, err
		}
		return source == nil, nil
	})
}

// RollbackCitation rolls back a citation to a specific version.
// It computes the changes needed and generates a compensating CitationUpdated event.
func (h *Handler) RollbackCitation(ctx context.Context, citationID uuid.UUID, targetVersion int64) (*RollbackResult, error) {
	return h.rollbackEntity(ctx, "Citation", citationID, targetVersion, func(id uuid.UUID) (bool, error) {
		citation, err := h.readStore.GetCitation(ctx, id)
		if err != nil {
			return false, err
		}
		return citation == nil, nil
	})
}

// rollbackEntity is a generic helper that handles the rollback logic for any entity type.
// The isDeleted function checks if the entity is currently deleted in the read model.
func (h *Handler) rollbackEntity(ctx context.Context, entityType string, entityID uuid.UUID, targetVersion int64, isDeleted func(uuid.UUID) (bool, error)) (*RollbackResult, error) {
	// Rollback is mainline-only (issue #670): the version and deleted checks below
	// read main while execute would append on the handler's scope, so a
	// branch-scoped rollback would compare a main-derived version against the
	// branch's own counter. Refuse before doing any of that work.
	if !h.branchID.IsMain() {
		return nil, ErrRollbackNotBranchScoped
	}

	// Validate target version is positive
	if targetVersion < 1 {
		return nil, ErrRollbackInvalidVersion
	}

	// Get current version from event store, on main per the guard above.
	currentVersion, err := h.eventStore.GetStreamVersion(ctx, entityID, domain.MainBranchID)
	if err != nil {
		if errors.Is(err, repository.ErrStreamNotFound) {
			return nil, query.ErrNoEvents
		}
		return nil, fmt.Errorf("getting stream version: %w", err)
	}

	// Validate target version is less than current version
	if targetVersion >= currentVersion {
		if targetVersion == currentVersion {
			return nil, ErrRollbackNoChanges
		}
		return nil, ErrRollbackInvalidVersion
	}

	// Check if entity is currently deleted (follow-up feature to handle recreation)
	deleted, err := isDeleted(entityID)
	if err != nil {
		return nil, fmt.Errorf("checking if entity is deleted: %w", err)
	}
	if deleted {
		return nil, ErrRollbackDeletedEntity
	}

	// Compute rollback changes using RollbackService
	changes, err := h.rollbackService.ComputeRollbackChanges(ctx, entityType, entityID, targetVersion)
	if err != nil {
		return nil, fmt.Errorf("computing rollback changes: %w", err)
	}

	// If no changes needed, this is a no-op
	if len(changes.Changes) == 0 {
		return &RollbackResult{
			EntityID:   entityID,
			EntityType: entityType,
			NewVersion: currentVersion,
			Changes:    changes.Changes,
		}, nil
	}

	// Create compensating event based on entity type
	var event domain.Event
	switch entityType {
	case "Person":
		event = domain.NewPersonUpdated(entityID, changes.Changes)
	case "Family":
		event = domain.NewFamilyUpdated(entityID, changes.Changes)
	case "Source":
		event = domain.NewSourceUpdated(entityID, changes.Changes)
	case "Citation":
		event = domain.NewCitationUpdated(entityID, changes.Changes)
	case "Media":
		event = domain.NewMediaUpdated(entityID, changes.Changes)
	default:
		return nil, errors.New("unsupported entity type for rollback: " + entityType)
	}

	// Append event with optimistic locking
	newVersion, err := h.execute(ctx, entityID.String(), entityType, []domain.Event{event}, currentVersion)
	if err != nil {
		return nil, fmt.Errorf("executing rollback event: %w", err)
	}

	return &RollbackResult{
		EntityID:   entityID,
		EntityType: entityType,
		NewVersion: newVersion,
		Changes:    changes.Changes,
	}, nil
}
