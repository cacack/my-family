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
	// DroppedImported is how many main-branch coordinates and EXIDs the read
	// model held before the reset but not after the replay. They belong to
	// entities imported before #1225, whose events do not carry them; the
	// rebuild does not restore them yet (#1251), so it warns when it drops any.
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
// Brick walls are the one read-model value no event carries (they are set
// there directly; #802), so they cannot be rebuilt from the log. They are read
// before the reset and written back once the replay is done, so a branch row
// the replay copies from main does not pick up a brick wall set later.
//
// The rebuild is safe to abort. Every event is decoded before anything is
// deleted, so an event the replay could not read fails the rebuild while the
// read model is intact. The brick walls are saved as the rebuild state
// (SaveRebuildState) before the reset, and only a rebuild that succeeds clears
// it: while it exists `serve` warns that the read model is out of sync, and a
// rerun after an interrupted rebuild restores them from it, since the reset
// has removed them from the read model.
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
	kept, err := loadUnloggedFields(ctx, rs)
	if err != nil {
		return result, err
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

	// Write back the brick walls, now that every person that will exist does.
	// A person who is not in the read model is skipped.
	var restoreErr error
	if ctx.Err() == nil {
		restoreErr = kept.restore(ctx, rs)
	}
	if ctx.Err() == nil {
		reportDroppedImported(ctx, rs, importedBefore, &result)
	}
	if err := errors.Join(replayErr, restoreErr); err != nil {
		return result, err
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

// unloggedFields holds the main-branch read-model values that no event
// carries, by person id. It is saved as the rebuild state, hence the JSON tags.
type unloggedFields struct {
	BrickWalls map[uuid.UUID]brickWall `json:"brick_walls"`
}

type brickWall struct {
	Note       string     `json:"note,omitempty"`
	Since      *time.Time `json:"since,omitempty"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}

// loadUnloggedFields returns the values to carry over: those saved by an
// unfinished earlier rebuild, whose reset removed them from the read model,
// overlaid with those the read model holds now.
func loadUnloggedFields(ctx context.Context, rs ReadModelStore) (*unloggedFields, error) {
	kept := &unloggedFields{BrickWalls: map[uuid.UUID]brickWall{}}
	saved, err := rs.GetRebuildState(ctx)
	if err != nil {
		return nil, fmt.Errorf("read rebuild state: %w", err)
	}
	if saved != nil {
		var prior unloggedFields
		if err := json.Unmarshal(saved, &prior); err != nil {
			return nil, fmt.Errorf("decode rebuild state: %w", err)
		}
		slog.Warn("an earlier rebuild did not finish; restoring the brick walls it saved before its reset")
		for id, bw := range prior.BrickWalls {
			kept.BrickWalls[id] = bw
		}
	}
	persons, err := ListAll(ctx, rebuildPageSize, rs.ListPersons)
	if err != nil {
		return nil, fmt.Errorf("read brick walls: list persons: %w", err)
	}
	for i := range persons {
		if p := &persons[i]; p.BrickWallNote != "" || p.BrickWallSince != nil || p.BrickWallResolvedAt != nil {
			kept.BrickWalls[p.ID] = brickWall{Note: p.BrickWallNote, Since: p.BrickWallSince, ResolvedAt: p.BrickWallResolvedAt}
		}
	}
	return kept, nil
}

// restore writes every kept brick wall back onto its person's main row, if
// that row exists.
func (k *unloggedFields) restore(ctx context.Context, rs ReadModelStore) error {
	var errs []error
	for id, bw := range k.BrickWalls {
		row, err := rs.GetPerson(ctx, domain.MainBranchID, id)
		if err == nil && row != nil {
			row.BrickWallNote, row.BrickWallSince, row.BrickWallResolvedAt = bw.Note, bw.Since, bw.ResolvedAt
			err = rs.SavePerson(ctx, domain.MainBranchID, row)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("restore brick wall of %s: %w", id, err))
		}
	}
	return errors.Join(errs...)
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
// and warns if fewer are left than before the reset (#1251). A recount that
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
	slog.Warn("the rebuild dropped imported place coordinates or EXIDs that the event log does not carry "+
		"(entities imported before #1225); re-import the GEDCOM file to restore them",
		"dropped", result.DroppedImported)
}
