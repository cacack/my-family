package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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
	backfilled, err := backfillImportedValues(ctx, events, rs, branches)
	result.Backfilled = backfilled
	if err != nil {
		return result, fmt.Errorf("log imported coordinates and EXIDs: %w", err)
	}
	kept, err := loadBrickWalls(ctx, rs)
	if err != nil {
		return result, err
	}
	walls, err := backfillBrickWalls(ctx, events, rs, kept)
	result.BackfilledBrickWalls = walls
	if err != nil {
		return result, fmt.Errorf("log brick walls: %w", err)
	}
	importedBefore, err := countImportedValues(ctx, rs)
	if err != nil {
		return result, fmt.Errorf("count coordinates and EXIDs: %w", err)
	}
	state, err := json.Marshal(kept)
	if err != nil {
		return result, fmt.Errorf("encode rebuild state: %w", err)
	}
	if err := rs.SaveRebuildState(ctx, state); err != nil {
		return result, fmt.Errorf("save rebuild state: %w", err)
	}
	if err := rs.ResetReadModel(ctx); err != nil {
		return result, fmt.Errorf("reset read model: %w", err)
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

// backfillBrickWalls appends a BrickWallBackfilled event to the main stream of
// each person whose kept brick wall that
// stream does not carry, and returns how many it appended (#802). A stream
// that carries any brick-wall event already states the person's status, as
// each such event states it whole. A person with no main stream is skipped.
// Every append is durable on its own, so an interrupted backfill is finished
// by the next rebuild.
func backfillBrickWalls(ctx context.Context, events EventStore, rs ReadModelStore, kept *rebuildState) (int, error) {
	projector := NewProjector(rs, nil)
	appended := 0
	for id, bw := range kept.BrickWalls {
		version, carried, err := mainStreamCarriesBrickWall(ctx, events, id)
		if err != nil {
			return appended, err
		}
		if carried || version == 0 {
			continue
		}
		event := domain.BrickWallBackfilled{BaseEvent: domain.NewBaseEvent(), PersonID: id, Note: bw.Note, Since: *bw.Since, ResolvedAt: bw.ResolvedAt}
		if err := events.Append(ctx, id, "Person", []domain.Event{event}, version, MainScope); err != nil {
			return appended, fmt.Errorf("append brick wall for person %s: %w", id, err)
		}
		appended++
		if err := projector.Project(ctx, event, version+1, domain.MainBranchID); err != nil {
			ReportProjectionFailure(ctx, rs, event, version+1, domain.MainBranchID, err)
		}
	}
	if appended > 0 {
		slog.Info("logged brick walls that were set before brick walls were kept in the event log", "persons", appended)
	}
	return appended, nil
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

// backfillImportedValues appends an ImportedValuesBackfilled event to each
// main-branch entity whose read model holds coordinates or EXIDs that its
// main stream does not carry, and to each entity main has deleted whose
// branch copy holds such coordinates, and returns how many it appended
// (#1251, #1255).
// Every append is durable on its own, so an interrupted backfill is finished
// by the next rebuild.
func backfillImportedValues(ctx context.Context, events EventStore, rs ReadModelStore, branches BranchStore) (int, error) {
	pending, err := coordinatesInReadModel(ctx, rs)
	if err != nil {
		return 0, err
	}
	exids, err := exidsInReadModel(ctx, rs)
	if err != nil {
		return 0, err
	}
	pending = append(pending, exids...)
	deleted, err := coordinatesMainDeleted(ctx, rs, branches)
	if err != nil {
		return 0, err
	}
	pending = append(pending, deleted...)

	// Each backfill is projected as soon as it is appended, so the row's
	// version follows its stream even if the rebuild stops before its reset.
	projector := NewProjector(rs, branches)
	appended := 0
	for _, backfill := range pending {
		version, carried, err := mainStreamCarriesImportedValues(ctx, events, backfill.EntityID)
		if err != nil {
			return appended, err
		}
		// A row with no main stream behind it has nothing to append to; the
		// replay drops it and the recount reports what it held.
		if carried || version == 0 {
			continue
		}
		backfill.BaseEvent = domain.NewBaseEvent()
		if err := events.Append(ctx, backfill.EntityID, backfillStreamType(backfill.EntityType), []domain.Event{backfill}, version, MainScope); err != nil {
			return appended, fmt.Errorf("append backfill for %s %s: %w", backfill.EntityType, backfill.EntityID, err)
		}
		appended++
		if err := projector.Project(ctx, backfill, version+1, domain.MainBranchID); err != nil {
			ReportProjectionFailure(ctx, rs, backfill, version+1, domain.MainBranchID, err)
		}
	}
	if appended > 0 {
		slog.Info("logged place coordinates and EXIDs that an import before #1225 left only in the read model",
			"entities", appended)
	}
	return appended, nil
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
// with the coordinates of that copy (#1255). A branch copy's coordinates can
// only have come from main's row, as no event but the backfill sets them, so
// every branch holds the same ones and the first copy found stands for all.
// The backfill goes on main's stream, after its delete; its projection finds
// no main row and fills in the branch copies alone.
func coordinatesMainDeleted(ctx context.Context, rs ReadModelStore, branches BranchStore) ([]domain.ImportedValuesBackfilled, error) {
	if branches == nil {
		return nil, nil
	}
	all, err := branches.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}
	var pending []domain.ImportedValuesBackfilled
	seen := map[uuid.UUID]bool{}
	for _, b := range all {
		branchID := domain.BranchID(b.ID)
		persons, err := ListAllOn(ctx, branchID, rebuildPageSize, rs.ListPersons)
		if err != nil {
			return nil, fmt.Errorf("list persons on branch %s: %w", b.ID, err)
		}
		for i := range persons {
			p := &persons[i]
			if seen[p.ID] || countSet(p.BirthPlaceLat, p.BirthPlaceLong, p.DeathPlaceLat, p.DeathPlaceLong) == 0 {
				continue
			}
			onMain, err := rs.GetPerson(ctx, domain.MainBranchID, p.ID)
			if err != nil {
				return nil, fmt.Errorf("person %s on main: %w", p.ID, err)
			}
			if onMain != nil {
				continue
			}
			seen[p.ID] = true
			pending = append(pending, domain.ImportedValuesBackfilled{
				EntityType: domain.BackfillPerson, EntityID: p.ID,
				BirthPlaceLat: p.BirthPlaceLat, BirthPlaceLong: p.BirthPlaceLong,
				DeathPlaceLat: p.DeathPlaceLat, DeathPlaceLong: p.DeathPlaceLong,
			})
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
	return pending, nil
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

// mainStreamCarriesImportedValues reads an entity's main stream and reports
// its version and whether any of its events sets coordinates or EXIDs.
func mainStreamCarriesImportedValues(ctx context.Context, events EventStore, id uuid.UUID) (version int64, carried bool, err error) {
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
		decoded, err := s.DecodeEvent()
		if err != nil {
			return 0, false, fmt.Errorf("decode %s at position %d: %w", s.EventType, s.Position, err)
		}
		carried = carried || setsImportedValues(decoded)
	}
	return version, carried, nil
}

// setsImportedValues reports whether a logged event sets place coordinates or
// EXIDs. Only the created events (#1225) and the backfill do.
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
