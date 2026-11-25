package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

// ProjectionFailure marks an event that is in the log but whose projection
// failed (#845). The read model is then behind the log until a rebuild.
type ProjectionFailure struct {
	StreamID  uuid.UUID       `json:"stream_id"`
	BranchID  domain.BranchID `json:"branch_id"`
	Version   int64           `json:"version"`
	EventType string          `json:"event_type"`
	Error     string          `json:"error"`
	FailedAt  time.Time       `json:"failed_at"`
}

// ReportProjectionFailure logs a projection failure with the event's
// coordinates and stores a marker for it, so `serve` can warn about the drift
// on its next start and `myfamily rebuild` can clear it. It never returns an
// error: the event is already durable, so failing to record the marker is
// only logged. The marker is written even if ctx was cancelled.
func ReportProjectionFailure(ctx context.Context, rs ReadModelStore, event domain.Event, version int64, branchID domain.BranchID, projectErr error) {
	failure := ProjectionFailure{
		StreamID:  event.AggregateID(),
		BranchID:  branchID,
		Version:   version,
		EventType: event.EventType(),
		Error:     projectErr.Error(),
		FailedAt:  time.Now().UTC(),
	}
	slog.Error("projection failed after append; read model is out of sync with the event log until `myfamily rebuild` runs",
		"event_type", failure.EventType,
		"stream_id", failure.StreamID.String(),
		"version", failure.Version,
		"branch_id", failure.BranchID.String(),
		"error", projectErr)
	if err := rs.RecordProjectionFailure(context.WithoutCancel(ctx), failure); err != nil {
		slog.Error("could not record projection failure marker",
			"stream_id", failure.StreamID.String(), "version", failure.Version, "error", err)
	}
}

// RebuildResult reports what RebuildReadModel replayed.
type RebuildResult struct {
	// Events is how many logged events were replayed, failed ones included.
	Events int
	// Failed is how many of them failed to project.
	Failed int
	// LastPosition is the log position of the last event replayed.
	LastPosition int64
	// Backfilled is how many entities had coordinates or EXIDs that only the
	// read model held (an import made before #1225), and so had them logged in
	// an ImportedValuesBackfilled event before the reset (#1251).
	Backfilled int
	// BackfilledBrickWalls is how many persons had a brick wall that only the
	// read model held (set before #802), and so had it logged in a
	// BrickWallBackfilled event before the reset.
	BackfilledBrickWalls int
	// BackfilledSnapshots is how many snapshots the registry held with no
	// event behind them (created before #624), and so had a SnapshotCreated
	// logged for them (#680).
	BackfilledSnapshots int
	// DroppedImported is how many main-branch coordinates and EXIDs the read
	// model held before the reset but not after the replay. The backfill
	// should leave none; the rebuild warns if it drops any all the same.
	DroppedImported int
}

// rebuildPageSize is how many log events RebuildReadModel reads per page.
const rebuildPageSize = 1000

// RebuildReadModel empties the read model and replays the whole event log into
// it, on every branch, so the read model matches the log again (#845). It is
// the repair for a projection failure. The caller must keep every other writer
// out for its duration; `myfamily rebuild` does so with the lock that `serve`
// also takes (#1232).
//
// The branch and snapshot registries are not emptied: their projections are
// upserts and replay-safe, so the replay brings them in line as well.
//
// Brick walls come from the log too (#802), but before #802 they were written
// only to the read model. Before the reset, the rebuild logs any brick wall the
// person's main stream does not carry yet, as one BrickWallBackfilled event,
// so the replay restores it.
//
// Place coordinates and EXIDs come from the log (#1225), but an import made
// before #1225 wrote them only to the read model. Before the reset, the
// rebuild logs any the log does not carry yet, one ImportedValuesBackfilled
// event per entity, so the replay restores them and later rebuilds need no
// backfill (#1251).
//
// Snapshots created before #624 were written only to the snapshot registry.
// The registry is not reset, but the rebuild logs a SnapshotCreated for each
// such snapshot, so the log alone can rebuild the registry too (#680).
//
// The reset drops and re-creates the read-model tables at this binary's
// schema and stamps ReadModelSchemaVersion, which is what `serve` asks for when
// the stamp does not match (#680, ADR-006).
//
// The rebuild is safe to abort. Every event is decoded before anything is
// deleted, so an event the replay could not read fails the rebuild while the
// read model is intact. The brick walls are saved as the rebuild state
// (SaveRebuildState) before the reset, and only a rebuild that succeeds clears
// it: while it exists `serve` warns that the read model is out of sync. A rerun
// after an interrupted rebuild finds its brick walls in the log; it logs those
// of the saved state as well, for a rebuild interrupted before #802, whose
// reset removed brick walls that were not logged.
//
// An event that fails to replay is reported like a live projection failure
// (logged, and recorded as a marker) and the replay goes on, as live writes
// do, so one bad event does not leave the rest of the read model empty. The
// rebuild then returns an error wrapping the first failure.
func RebuildReadModel(ctx context.Context, events EventStore, rs ReadModelStore, branches BranchStore, snapshots SnapshotStore) (RebuildResult, error) {
	var result RebuildResult
	if err := checkLogDecodes(ctx, events); err != nil {
		return result, err
	}
	pending, err := PendingBackfills(ctx, events, rs, branches, snapshots)
	if err != nil {
		return result, err
	}
	if err := appendBackfills(ctx, events, rs, branches, pending, &result); err != nil {
		return result, err
	}
	importedBefore, err := countImportedValues(ctx, rs)
	if err != nil {
		return result, fmt.Errorf("count coordinates and EXIDs: %w", err)
	}
	state, err := json.Marshal(pending.kept)
	if err != nil {
		return result, fmt.Errorf("encode rebuild state: %w", err)
	}
	if err := rs.SaveRebuildState(ctx, state); err != nil {
		return result, fmt.Errorf("save rebuild state: %w", err)
	}
	if err := rs.ResetReadModel(ctx); err != nil {
		return result, fmt.Errorf("reset read model: %w", err)
	}
	if err := rs.SetSchemaVersion(ctx, ReadModelSchemaVersion); err != nil {
		return result, fmt.Errorf("stamp read model schema version: %w", err)
	}

	projector := NewProjectorWithSnapshots(rs, branches, snapshots)
	replayErr := replayLog(ctx, events, rs, projector, &result)

	if ctx.Err() == nil {
		reportDroppedImported(ctx, rs, importedBefore, &result)
	}
	if replayErr != nil {
		return result, replayErr
	}
	if err := rs.ClearRebuildState(ctx); err != nil {
		return result, fmt.Errorf("clear rebuild state: %w", err)
	}
	return result, nil
}

// checkLogDecodes decodes every logged event, so a log the replay could not
// read fails the rebuild before the read model is reset.
func checkLogDecodes(ctx context.Context, events EventStore) error {
	var after int64
	for {
		page, err := events.ReadAll(ctx, after, rebuildPageSize)
		if err != nil {
			return fmt.Errorf("read event log after position %d: %w", after, err)
		}
		if len(page) == 0 {
			return nil
		}
		for i := range page {
			if _, err := page[i].DecodeEvent(); err != nil {
				return fmt.Errorf("decode %s at position %d: %w", page[i].EventType, page[i].Position, err)
			}
			after = page[i].Position
		}
	}
}

// replayLog projects every logged event in position order. It stops only if
// the log cannot be read, an event cannot be decoded or ctx is done.
func replayLog(ctx context.Context, events EventStore, rs ReadModelStore, projector *Projector, result *RebuildResult) error {
	var firstFailure error
	for {
		page, err := events.ReadAll(ctx, result.LastPosition, rebuildPageSize)
		if err != nil {
			return errors.Join(firstFailure, fmt.Errorf("read event log after position %d: %w", result.LastPosition, err))
		}
		if len(page) == 0 {
			if firstFailure != nil {
				return fmt.Errorf("%d of %d events failed to replay; first: %w", result.Failed, result.Events, firstFailure)
			}
			return nil
		}
		for i := range page {
			if err := ctx.Err(); err != nil {
				return errors.Join(firstFailure, fmt.Errorf("rebuild stopped after position %d: %w", result.LastPosition, err))
			}
			stored := &page[i]
			decoded, err := stored.DecodeEvent()
			if err != nil {
				return errors.Join(firstFailure, fmt.Errorf("decode %s at position %d: %w", stored.EventType, stored.Position, err))
			}
			result.Events++
			result.LastPosition = stored.Position
			if err := projector.Project(ctx, decoded, stored.Version, stored.BranchID); err != nil {
				ReportProjectionFailure(ctx, rs, decoded, stored.Version, stored.BranchID, err)
				result.Failed++
				if firstFailure == nil {
					firstFailure = fmt.Errorf("replay %s at position %d (stream %s, version %d): %w",
						stored.EventType, stored.Position, stored.StreamID, stored.Version, err)
				}
			}
		}
	}
}

// rebuildState is what a rebuild saves before its reset: the brick walls the
// main-branch read model held, by person id. Hence the JSON tags.
type rebuildState struct {
	BrickWalls map[uuid.UUID]brickWall `json:"brick_walls"`
}

type brickWall struct {
	Note       string     `json:"note,omitempty"`
	Since      *time.Time `json:"since,omitempty"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}

// loadBrickWalls returns the brick walls to keep: those saved by an unfinished
// earlier rebuild, whose reset removed them from the read model, overlaid with
// those the read model holds now. A person with no brick wall since is left
// out, as GetBrickWalls leaves them out.
func loadBrickWalls(ctx context.Context, rs ReadModelStore) (*rebuildState, error) {
	kept := &rebuildState{BrickWalls: map[uuid.UUID]brickWall{}}
	saved, err := rs.GetRebuildState(ctx)
	if err != nil {
		return nil, fmt.Errorf("read rebuild state: %w", err)
	}
	if saved != nil {
		var prior rebuildState
		if err := json.Unmarshal(saved, &prior); err != nil {
			return nil, fmt.Errorf("decode rebuild state: %w", err)
		}
		slog.Warn("an earlier rebuild did not finish; logging the brick walls it saved before its reset")
		for id, bw := range prior.BrickWalls {
			if bw.Since != nil {
				kept.BrickWalls[id] = bw
			}
		}
	}
	persons, err := ListAll(ctx, rebuildPageSize, rs.ListPersons)
	if err != nil {
		return nil, fmt.Errorf("read brick walls: list persons: %w", err)
	}
	for i := range persons {
		if p := &persons[i]; p.BrickWallSince != nil {
			kept.BrickWalls[p.ID] = brickWall{Note: p.BrickWallNote, Since: p.BrickWallSince, ResolvedAt: p.BrickWallResolvedAt}
		}
	}
	return kept, nil
}

// snapshotStreamType is the stream type of a snapshot's own stream; it matches
// the command package's.
const snapshotStreamType = "snapshot"

// Backfills is what only the read model and the snapshot registry hold, so
// that a rebuild must log it before its reset and a backup, which carries the
// log, would lose it (ADR-010 §6): coordinates and EXIDs imported before
// #1225, brick walls marked before #802, snapshots created before #624.
// PendingBackfills decides it; RebuildReadModel logs it, and the backup
// refuses a database that holds any.
type Backfills struct {
	// ImportedValues holds one event per entity whose coordinates or EXIDs its
	// main stream does not carry (#1251, #1255).
	ImportedValues []domain.ImportedValuesBackfilled
	// BrickWalls holds one event per person whose brick wall its main stream
	// does not carry (#802).
	BrickWalls []domain.BrickWallBackfilled
	// Snapshots holds the mainline snapshots with no event behind them (#680).
	Snapshots []*domain.Snapshot
	// kept is the brick walls a rebuild saves as its state before the reset.
	kept *rebuildState
}

// Len is how many events logging the backfills would append.
func (b *Backfills) Len() int {
	return len(b.ImportedValues) + len(b.BrickWalls) + len(b.Snapshots)
}

// PendingBackfills returns what the read model and the snapshot registry hold
// that no event records. It only reads. The events it returns have no
// BaseEvent yet: logging them gives each its own.
func PendingBackfills(ctx context.Context, events EventStore, rs ReadModelStore, branches BranchStore, snapshots SnapshotStore) (*Backfills, error) {
	var b Backfills
	var err error
	if b.ImportedValues, err = pendingImportedValues(ctx, events, rs, branches); err != nil {
		return nil, fmt.Errorf("find imported coordinates and EXIDs to log: %w", err)
	}
	if b.kept, err = loadBrickWalls(ctx, rs); err != nil {
		return nil, err
	}
	if b.BrickWalls, err = pendingBrickWalls(ctx, events, b.kept); err != nil {
		return nil, fmt.Errorf("find brick walls to log: %w", err)
	}
	if b.Snapshots, err = pendingSnapshots(ctx, events, snapshots); err != nil {
		return nil, fmt.Errorf("find snapshots to log: %w", err)
	}
	return &b, nil
}

// appendBackfills logs pending and counts what it appends in result. Each
// backfill on an entity's stream is projected as soon as it is appended, so
// the row's version follows its stream even if the rebuild stops before its
// reset. Every append is durable on its own, so an interrupted backfill is
// finished by the next rebuild.
func appendBackfills(ctx context.Context, events EventStore, rs ReadModelStore, branches BranchStore, pending *Backfills, result *RebuildResult) error {
	projector := NewProjector(rs, branches)
	for _, backfill := range pending.ImportedValues {
		backfill.BaseEvent = domain.NewBaseEvent()
		if err := appendProjected(ctx, events, rs, projector, backfill.EntityID, backfillStreamType(backfill.EntityType), backfill); err != nil {
			return fmt.Errorf("log imported coordinates and EXIDs: append backfill for %s %s: %w", backfill.EntityType, backfill.EntityID, err)
		}
		result.Backfilled++
	}
	if result.Backfilled > 0 {
		slog.Info("logged place coordinates and EXIDs that an import before #1225 left only in the read model",
			"entities", result.Backfilled)
	}

	for _, wall := range pending.BrickWalls {
		wall.BaseEvent = domain.NewBaseEvent()
		if err := appendProjected(ctx, events, rs, projector, wall.PersonID, "Person", wall); err != nil {
			return fmt.Errorf("log brick walls: append brick wall for person %s: %w", wall.PersonID, err)
		}
		result.BackfilledBrickWalls++
	}
	if result.BackfilledBrickWalls > 0 {
		slog.Info("logged brick walls that were set before brick walls were kept in the event log", "persons", result.BackfilledBrickWalls)
	}

	// The registry is not reset, so a snapshot's event is not projected here.
	for _, snap := range pending.Snapshots {
		event := domain.NewSnapshotCreated(snap)
		event.Timestamp = snap.CreatedAt
		if err := events.Append(ctx, snap.ID, snapshotStreamType, []domain.Event{event}, -1, MainScope); err != nil {
			return fmt.Errorf("log snapshots: append snapshot %s: %w", snap.ID, err)
		}
		result.BackfilledSnapshots++
	}
	if result.BackfilledSnapshots > 0 {
		slog.Info("logged snapshots that were created before snapshots were kept in the event log", "snapshots", result.BackfilledSnapshots)
	}
	return nil
}

// appendProjected appends event to the end of id's main stream and projects
// it. The version is read when appending, as an earlier backfill may have
// appended to the same stream.
func appendProjected(ctx context.Context, events EventStore, rs ReadModelStore, projector *Projector, id uuid.UUID, streamType string, event domain.Event) error {
	version, err := mainStreamVersion(ctx, events, id)
	if err != nil {
		return err
	}
	if err := events.Append(ctx, id, streamType, []domain.Event{event}, version, MainScope); err != nil {
		return err
	}
	if err := projector.Project(ctx, event, version+1, domain.MainBranchID); err != nil {
		ReportProjectionFailure(ctx, rs, event, version+1, domain.MainBranchID, err)
	}
	return nil
}

// mainStreamVersion returns the version of id's main stream.
func mainStreamVersion(ctx context.Context, events EventStore, id uuid.UUID) (int64, error) {
	stored, err := events.ReadStream(ctx, id)
	if err != nil {
		return 0, fmt.Errorf("read stream %s: %w", id, err)
	}
	var version int64
	for i := range stored {
		if stored[i].BranchID.IsMain() {
			version = max(version, stored[i].Version)
		}
	}
	return version, nil
}

// pendingBrickWalls returns a BrickWallBackfilled for each person whose kept
// brick wall their main stream does not carry (#802). A stream that carries
// any brick-wall event already states the person's status, as each such event
// states it whole. A person with no main stream is skipped.
func pendingBrickWalls(ctx context.Context, events EventStore, kept *rebuildState) ([]domain.BrickWallBackfilled, error) {
	var pending []domain.BrickWallBackfilled
	for id, bw := range kept.BrickWalls {
		version, carried, err := mainStreamCarriesBrickWall(ctx, events, id)
		if err != nil {
			return nil, err
		}
		if carried || version == 0 {
			continue
		}
		pending = append(pending, domain.BrickWallBackfilled{PersonID: id, Note: bw.Note, Since: *bw.Since, ResolvedAt: bw.ResolvedAt})
	}
	return pending, nil
}

// pendingSnapshots returns each mainline snapshot whose stream is empty
// (#680). Only a snapshot created before #624 has no event, and every
// snapshot was a mainline one until #839. Its event keeps the snapshot's
// original creation time, so the replay re-projects the same registry row.
func pendingSnapshots(ctx context.Context, events EventStore, snapshots SnapshotStore) ([]*domain.Snapshot, error) {
	if snapshots == nil {
		return nil, nil
	}
	list, err := snapshots.List(ctx, domain.MainBranchID)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	var pending []*domain.Snapshot
	for _, snap := range list {
		stored, err := events.ReadStream(ctx, snap.ID)
		if err != nil {
			return nil, fmt.Errorf("read stream %s: %w", snap.ID, err)
		}
		if len(stored) == 0 {
			pending = append(pending, snap)
		}
	}
	return pending, nil
}

// mainStreamCarriesBrickWall reads a person's main stream and reports its
// version and whether it holds a brick-wall event.
func mainStreamCarriesBrickWall(ctx context.Context, events EventStore, id uuid.UUID) (version int64, carried bool, err error) {
	stored, err := events.ReadStream(ctx, id)
	if err != nil {
		return 0, false, fmt.Errorf("read stream %s: %w", id, err)
	}
	for i := range stored {
		s := &stored[i]
		if !s.BranchID.IsMain() {
			continue
		}
		version = max(version, s.Version)
		carried = carried || s.EventType == "BrickWallSet" || s.EventType == "BrickWallResolved" || s.EventType == "BrickWallBackfilled"
	}
	return version, carried, nil
}

// countImportedValues counts the coordinates and EXIDs on main-branch rows.
func countImportedValues(ctx context.Context, rs ReadModelStore) (int, error) {
	n := 0
	persons, err := ListAll(ctx, rebuildPageSize, rs.ListPersons)
	if err != nil {
		return 0, fmt.Errorf("list persons: %w", err)
	}
	for i := range persons {
		p := &persons[i]
		n += countSet(p.BirthPlaceLat, p.BirthPlaceLong, p.DeathPlaceLat, p.DeathPlaceLong)
		ids, err := rs.GetPersonExternalIDs(ctx, domain.MainBranchID, p.ID)
		if err != nil {
			return 0, fmt.Errorf("person %s exids: %w", p.ID, err)
		}
		n += len(ids)
	}
	lifeEvents, err := ListAll(ctx, rebuildPageSize, rs.ListEvents)
	if err != nil {
		return 0, fmt.Errorf("list life events: %w", err)
	}
	for i := range lifeEvents {
		n += countSet(lifeEvents[i].PlaceLat, lifeEvents[i].PlaceLong)
	}
	families, err := ListAll(ctx, rebuildPageSize, rs.ListFamilies)
	if err != nil {
		return 0, fmt.Errorf("list families: %w", err)
	}
	for i := range families {
		n += countSet(families[i].MarriagePlaceLat, families[i].MarriagePlaceLong)
		ids, err := rs.GetFamilyExternalIDs(ctx, domain.MainBranchID, families[i].ID)
		if err != nil {
			return 0, fmt.Errorf("family %s exids: %w", families[i].ID, err)
		}
		n += len(ids)
	}
	sources, err := ListAll(ctx, rebuildPageSize, rs.ListSources)
	if err != nil {
		return 0, fmt.Errorf("list sources: %w", err)
	}
	for i := range sources {
		ids, err := rs.GetSourceExternalIDs(ctx, domain.MainBranchID, sources[i].ID)
		if err != nil {
			return 0, fmt.Errorf("source %s exids: %w", sources[i].ID, err)
		}
		n += len(ids)
	}
	repos, err := ListAll(ctx, rebuildPageSize, rs.ListRepositories)
	if err != nil {
		return 0, fmt.Errorf("list repositories: %w", err)
	}
	for i := range repos {
		ids, err := rs.GetRepositoryExternalIDs(ctx, repos[i].ID)
		if err != nil {
			return 0, fmt.Errorf("repository %s exids: %w", repos[i].ID, err)
		}
		n += len(ids)
	}
	return n, nil
}

func countSet(values ...*string) int {
	n := 0
	for _, v := range values {
		if v != nil {
			n++
		}
	}
	return n
}

// reportDroppedImported recounts the coordinates and EXIDs after the replay
// and warns if fewer are left than before the reset. The backfill should
// leave none to drop; this is the check that it did. A recount that
// fails is only logged: the replay is done, and the count is advisory.
func reportDroppedImported(ctx context.Context, rs ReadModelStore, before int, result *RebuildResult) {
	after, err := countImportedValues(ctx, rs)
	if err != nil {
		slog.Warn("could not check whether the rebuild dropped imported coordinates or EXIDs", "error", err)
		return
	}
	if after >= before {
		return
	}
	result.DroppedImported = before - after
	slog.Warn("the rebuild dropped imported place coordinates or EXIDs that the event log does not carry; "+
		"re-import the GEDCOM file to restore them",
		"dropped", result.DroppedImported)
}

// pendingImportedValues returns an ImportedValuesBackfilled for each
// main-branch entity whose read model holds coordinates or EXIDs that its main
// stream does not carry, and for each entity main has deleted whose branch
// copy holds such coordinates (#1251, #1255). A row with no main stream behind
// it has nothing to append to; the replay drops it and the recount reports
// what it held.
func pendingImportedValues(ctx context.Context, events EventStore, rs ReadModelStore, branches BranchStore) ([]domain.ImportedValuesBackfilled, error) {
	found, err := coordinatesInReadModel(ctx, rs)
	if err != nil {
		return nil, err
	}
	exids, err := exidsInReadModel(ctx, rs)
	if err != nil {
		return nil, err
	}
	found = append(found, exids...)
	deleted, err := coordinatesMainDeleted(ctx, events, rs, branches)
	if err != nil {
		return nil, err
	}
	found = append(found, deleted...)

	var pending []domain.ImportedValuesBackfilled
	for _, backfill := range found {
		version, carried, err := mainStreamCarriesImportedValues(ctx, events, backfill)
		if err != nil {
			return nil, err
		}
		if carried || version == 0 {
			continue
		}
		if backfill.EntityType == domain.BackfillPerson {
			if backfill.BranchSetCoordinates, err = branchSetCoordinates(ctx, events, backfill.EntityID); err != nil {
				return nil, err
			}
		}
		pending = append(pending, backfill)
	}
	return pending, nil
}

// backfillStreamType is the stream type GEDCOM import gave the entity's stream:
// "event" for a life event, the entity type otherwise. The stream already
// exists, so the store keeps its type either way; this keeps them consistent.
func backfillStreamType(entityType string) string {
	if entityType == domain.BackfillLifeEvent {
		return "event"
	}
	return entityType
}

// coordinatesInReadModel returns a backfill for every main-branch person with
// coordinates or EXIDs, and every life event with coordinates, in the read
// model.
func coordinatesInReadModel(ctx context.Context, rs ReadModelStore) ([]domain.ImportedValuesBackfilled, error) {
	var pending []domain.ImportedValuesBackfilled
	persons, err := ListAll(ctx, rebuildPageSize, rs.ListPersons)
	if err != nil {
		return nil, fmt.Errorf("list persons: %w", err)
	}
	for i := range persons {
		p := &persons[i]
		ids, err := rs.GetPersonExternalIDs(ctx, domain.MainBranchID, p.ID)
		if err != nil {
			return nil, fmt.Errorf("person %s exids: %w", p.ID, err)
		}
		if countSet(p.BirthPlaceLat, p.BirthPlaceLong, p.DeathPlaceLat, p.DeathPlaceLong)+len(ids) > 0 {
			pending = append(pending, domain.ImportedValuesBackfilled{
				EntityType: domain.BackfillPerson, EntityID: p.ID,
				BirthPlaceLat: p.BirthPlaceLat, BirthPlaceLong: p.BirthPlaceLong,
				DeathPlaceLat: p.DeathPlaceLat, DeathPlaceLong: p.DeathPlaceLong,
				ExternalIDs: toExternalIDs(ids, func(x PersonExternalIDReadModel) (string, string) { return x.Value, x.Type }),
			})
		}
	}
	lifeEvents, err := ListAll(ctx, rebuildPageSize, rs.ListEvents)
	if err != nil {
		return nil, fmt.Errorf("list life events: %w", err)
	}
	for i := range lifeEvents {
		if e := &lifeEvents[i]; countSet(e.PlaceLat, e.PlaceLong) > 0 {
			pending = append(pending, domain.ImportedValuesBackfilled{
				EntityType: domain.BackfillLifeEvent, EntityID: e.ID, PlaceLat: e.PlaceLat, PlaceLong: e.PlaceLong,
			})
		}
	}
	return pending, nil
}

// coordinatesMainDeleted returns a backfill for every person and life event
// that main has deleted but a research branch still holds its own copy of,
// with the coordinates of that copy (#1255). A copy's coordinates came from
// main's row except those its branch set or cleared with an update (#1604),
// which its log carries, so the backfill takes each coordinate from a copy
// whose branch did not (#1797). Every such copy holds main's, and the first
// one found stands for all.
// The backfill goes on main's stream, after its delete; its projection finds
// no main row and fills in the branch copies alone.
func coordinatesMainDeleted(ctx context.Context, events EventStore, rs ReadModelStore, branches BranchStore) ([]domain.ImportedValuesBackfilled, error) {
	if branches == nil {
		return nil, nil
	}
	all, err := branches.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}
	var pending []domain.ImportedValuesBackfilled
	persons := map[uuid.UUID]*domain.ImportedValuesBackfilled{}
	var personOrder []uuid.UUID
	seen := map[uuid.UUID]bool{}
	for _, b := range all {
		branchID := domain.BranchID(b.ID)
		copies, err := ListAllOn(ctx, branchID, rebuildPageSize, rs.ListPersons)
		if err != nil {
			return nil, fmt.Errorf("list persons on branch %s: %w", b.ID, err)
		}
		for i := range copies {
			p := &copies[i]
			if countSet(p.BirthPlaceLat, p.BirthPlaceLong, p.DeathPlaceLat, p.DeathPlaceLong) == 0 {
				continue
			}
			onMain, err := rs.GetPerson(ctx, domain.MainBranchID, p.ID)
			if err != nil {
				return nil, fmt.Errorf("person %s on main: %w", p.ID, err)
			}
			if onMain != nil {
				continue
			}
			branchSet, err := branchSetCoordinates(ctx, events, p.ID)
			if err != nil {
				return nil, err
			}
			backfill, ok := persons[p.ID]
			if !ok {
				backfill = &domain.ImportedValuesBackfilled{EntityType: domain.BackfillPerson, EntityID: p.ID}
				persons[p.ID] = backfill
				personOrder = append(personOrder, p.ID)
			}
			takeMainCoordinates(backfill, p, branchSet[b.ID.String()])
		}
		lifeEvents, err := ListAllOn(ctx, branchID, rebuildPageSize, rs.ListEvents)
		if err != nil {
			return nil, fmt.Errorf("list life events on branch %s: %w", b.ID, err)
		}
		for i := range lifeEvents {
			e := &lifeEvents[i]
			if seen[e.ID] || countSet(e.PlaceLat, e.PlaceLong) == 0 {
				continue
			}
			onMain, err := rs.GetEvent(ctx, domain.MainBranchID, e.ID)
			if err != nil {
				return nil, fmt.Errorf("life event %s on main: %w", e.ID, err)
			}
			if onMain != nil {
				continue
			}
			seen[e.ID] = true
			pending = append(pending, domain.ImportedValuesBackfilled{
				EntityType: domain.BackfillLifeEvent, EntityID: e.ID, PlaceLat: e.PlaceLat, PlaceLong: e.PlaceLong,
			})
		}
	}
	for _, id := range personOrder {
		if b := persons[id]; countSet(b.BirthPlaceLat, b.BirthPlaceLong, b.DeathPlaceLat, b.DeathPlaceLong) > 0 {
			pending = append(pending, *b)
		}
	}
	return pending, nil
}

// takeMainCoordinates fills in each coordinate the backfill lacks from a
// branch copy of the person, past those its branch set or cleared itself
// (branchSet), which are not main's (#1797).
func takeMainCoordinates(backfill *domain.ImportedValuesBackfilled, row *PersonReadModel, branchSet []string) {
	for _, c := range []struct {
		key      string
		backfill **string
		row      *string
	}{
		{"birth_place_lat", &backfill.BirthPlaceLat, row.BirthPlaceLat},
		{"birth_place_long", &backfill.BirthPlaceLong, row.BirthPlaceLong},
		{"death_place_lat", &backfill.DeathPlaceLat, row.DeathPlaceLat},
		{"death_place_long", &backfill.DeathPlaceLong, row.DeathPlaceLong},
	} {
		if *c.backfill == nil && !slices.Contains(branchSet, c.key) {
			*c.backfill = c.row
		}
	}
}

// exidsInReadModel returns a backfill for every main-branch family, source and
// repository with EXIDs in the read model.
func exidsInReadModel(ctx context.Context, rs ReadModelStore) ([]domain.ImportedValuesBackfilled, error) {
	var pending []domain.ImportedValuesBackfilled
	families, err := ListAll(ctx, rebuildPageSize, rs.ListFamilies)
	if err != nil {
		return nil, fmt.Errorf("list families: %w", err)
	}
	for i := range families {
		ids, err := rs.GetFamilyExternalIDs(ctx, domain.MainBranchID, families[i].ID)
		if err != nil {
			return nil, fmt.Errorf("family %s exids: %w", families[i].ID, err)
		}
		if len(ids) > 0 {
			pending = append(pending, domain.ImportedValuesBackfilled{EntityType: domain.BackfillFamily, EntityID: families[i].ID,
				ExternalIDs: toExternalIDs(ids, func(x FamilyExternalIDReadModel) (string, string) { return x.Value, x.Type })})
		}
	}
	sources, err := ListAll(ctx, rebuildPageSize, rs.ListSources)
	if err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}
	for i := range sources {
		ids, err := rs.GetSourceExternalIDs(ctx, domain.MainBranchID, sources[i].ID)
		if err != nil {
			return nil, fmt.Errorf("source %s exids: %w", sources[i].ID, err)
		}
		if len(ids) > 0 {
			pending = append(pending, domain.ImportedValuesBackfilled{EntityType: domain.BackfillSource, EntityID: sources[i].ID,
				ExternalIDs: toExternalIDs(ids, func(x SourceExternalIDReadModel) (string, string) { return x.Value, x.Type })})
		}
	}
	repos, err := ListAll(ctx, rebuildPageSize, rs.ListRepositories)
	if err != nil {
		return nil, fmt.Errorf("list repositories: %w", err)
	}
	for i := range repos {
		ids, err := rs.GetRepositoryExternalIDs(ctx, repos[i].ID)
		if err != nil {
			return nil, fmt.Errorf("repository %s exids: %w", repos[i].ID, err)
		}
		if len(ids) > 0 {
			pending = append(pending, domain.ImportedValuesBackfilled{EntityType: domain.BackfillRepository, EntityID: repos[i].ID,
				ExternalIDs: toExternalIDs(ids, func(x RepositoryExternalIDReadModel) (string, string) { return x.Value, x.Type })})
		}
	}
	return pending, nil
}

// toExternalIDs turns read-model EXID rows back into the values an event
// carries, in row order; the inverse of externalIDRows.
func toExternalIDs[T any](rows []T, valueType func(T) (string, string)) []domain.ExternalIdentifier {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]domain.ExternalIdentifier, len(rows))
	for i, row := range rows {
		ids[i].Value, ids[i].Type = valueType(row)
	}
	return ids
}

// mainStreamCarriesImportedValues reads the backfill's entity's main stream
// and reports its version and whether its events carry every value the
// backfill holds. A created event or backfill that sets any carries them all
// (#1225); an update that sets or clears coordinates (#1604) carries only the
// pair it names, so a backfill still logs the imported values beside it.
func mainStreamCarriesImportedValues(ctx context.Context, events EventStore, backfill domain.ImportedValuesBackfilled) (version int64, carried bool, err error) {
	stored, err := events.ReadStream(ctx, backfill.EntityID)
	if err != nil {
		return 0, false, fmt.Errorf("read stream %s: %w", backfill.EntityID, err)
	}
	updated := map[string]bool{}
	for i := range stored {
		s := &stored[i]
		if !s.BranchID.IsMain() {
			continue
		}
		version = max(version, s.Version)
		decoded, err := s.DecodeEvent()
		if err != nil {
			return 0, false, fmt.Errorf("decode %s at position %d: %w", s.EventType, s.Position, err)
		}
		carried = carried || setsImportedValues(decoded)
		for _, key := range updatedCoordinateKeys(decoded) {
			updated[key] = true
		}
	}
	return version, carried || updatesCarry(backfill, updated), nil
}

// updatesCarry reports whether updates set every value the backfill holds:
// each of its coordinates is under a key in updated, and it holds no EXIDs,
// which no update sets.
func updatesCarry(backfill domain.ImportedValuesBackfilled, updated map[string]bool) bool {
	if len(backfill.ExternalIDs) > 0 || backfill.PlaceLat != nil || backfill.PlaceLong != nil {
		return false
	}
	for key, v := range map[string]*string{
		"birth_place_lat": backfill.BirthPlaceLat, "birth_place_long": backfill.BirthPlaceLong,
		"death_place_lat": backfill.DeathPlaceLat, "death_place_long": backfill.DeathPlaceLong,
	} {
		if v != nil && !updated[key] {
			return false
		}
	}
	return true
}

// branchSetCoordinates returns, by branch id, the coordinate change keys that
// updates on each research branch set or cleared on the person (#1604), or nil
// if none did.
func branchSetCoordinates(ctx context.Context, events EventStore, id uuid.UUID) (map[string][]string, error) {
	stored, err := events.ReadStream(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("read stream %s: %w", id, err)
	}
	var set map[string][]string
	for i := range stored {
		s := &stored[i]
		if s.BranchID.IsMain() {
			continue
		}
		decoded, err := s.DecodeEvent()
		if err != nil {
			return nil, fmt.Errorf("decode %s at position %d: %w", s.EventType, s.Position, err)
		}
		for _, key := range updatedCoordinateKeys(decoded) {
			branch := s.BranchID.String()
			if !slices.Contains(set[branch], key) {
				if set == nil {
					set = map[string][]string{}
				}
				set[branch] = append(set[branch], key)
			}
		}
	}
	return set, nil
}

// updatedCoordinateKeys returns the coordinate change keys a person or family
// update sets or clears (#1604), or a person merge resolves onto its survivor,
// whose stream it lands on (#1799).
func updatedCoordinateKeys(event domain.Event) []string {
	personKeys := []string{"birth_place_lat", "birth_place_long", "death_place_lat", "death_place_long"}
	var changes map[string]any
	var keys []string
	switch e := event.(type) {
	case domain.PersonUpdated:
		changes, keys = e.Changes, personKeys
	case domain.PersonMerged:
		changes, keys = e.ResolvedFields, personKeys
	case domain.FamilyUpdated:
		changes, keys = e.Changes, []string{"marriage_place_lat", "marriage_place_long"}
	}
	var set []string
	for _, k := range keys {
		if _, ok := changes[k]; ok {
			set = append(set, k)
		}
	}
	return set
}

// setsImportedValues reports whether a logged event sets the whole of an
// entity's imported place coordinates and EXIDs: the created events (#1225)
// and the backfill.
func setsImportedValues(event domain.Event) bool {
	switch e := event.(type) {
	case domain.ImportedValuesBackfilled:
		return true
	case domain.PersonCreated:
		return countSet(e.BirthPlaceLat, e.BirthPlaceLong, e.DeathPlaceLat, e.DeathPlaceLong)+len(e.ExternalIDs) > 0
	case domain.LifeEventCreated:
		return countSet(e.PlaceLat, e.PlaceLong) > 0
	case domain.FamilyCreated:
		return countSet(e.MarriagePlaceLat, e.MarriagePlaceLong)+len(e.ExternalIDs) > 0
	case domain.SourceCreated:
		return len(e.ExternalIDs) > 0
	case domain.RepositoryCreated:
		return len(e.ExternalIDs) > 0
	}
	return false
}
