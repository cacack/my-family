package repository

import (
	"context"
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
}

// rebuildPageSize is how many log events RebuildReadModel reads per page.
const rebuildPageSize = 1000

// RebuildReadModel empties the read model and replays the whole event log into
// it, on every branch, so the read model matches the log again (#845). It is
// the repair for a projection failure. Run it with the server stopped: a write
// made during the rebuild may be lost from the read model.
//
// The branch and snapshot registries are not emptied: their projections are
// upserts and replay-safe, so the replay brings them in line as well.
//
// A few read-model fields are not in any event (GEDCOM import writes them
// straight to the read model, and brick walls are set there directly): person
// and life-event coordinates, brick walls and external identifiers. They
// cannot be rebuilt from the log, so they are read before the reset and written
// back once the entity's first event has been replayed, which is where import
// wrote them, and again at the end.
//
// An event that fails to replay is reported like a live projection failure
// (logged, and recorded as a marker) and the replay goes on, as live writes
// do, so one bad event does not leave the rest of the read model empty. The
// rebuild then returns an error wrapping the first failure.
func RebuildReadModel(ctx context.Context, events EventStore, rs ReadModelStore, branches BranchStore, snapshots SnapshotStore) (RebuildResult, error) {
	var result RebuildResult
	kept, err := captureUnloggedFields(ctx, rs)
	if err != nil {
		return result, fmt.Errorf("read fields that are not in the event log: %w", err)
	}
	if err := rs.ResetReadModel(ctx); err != nil {
		return result, fmt.Errorf("reset read model: %w", err)
	}

	projector := NewProjectorWithSnapshots(rs, branches, snapshots)
	replayErr := replayLog(ctx, events, rs, projector, kept, &result)

	// Write back the unlogged fields once more, now that every entity that
	// will exist does. An entity that is not in the read model is skipped.
	var restoreErr error
	for id, restore := range kept {
		if err := restore(ctx); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("restore unlogged fields of %s: %w", id, err))
		}
	}
	return result, errors.Join(replayErr, restoreErr)
}

// replayLog projects every logged event in position order, restoring an
// entity's unlogged fields right after its first main event. It stops only if
// the log cannot be read or an event cannot be decoded.
func replayLog(ctx context.Context, events EventStore, rs ReadModelStore, projector *Projector, kept unloggedFields, result *RebuildResult) error {
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
				continue
			}
			if restore, ok := kept[stored.StreamID]; ok && stored.BranchID.IsMain() && stored.Version == 1 {
				if err := restore(ctx); err != nil {
					return errors.Join(firstFailure, fmt.Errorf("restore unlogged fields of %s: %w", stored.StreamID, err))
				}
			}
		}
	}
}

// restoreFunc writes an entity's unlogged fields back onto its main row. It is
// idempotent and a no-op when the row does not exist.
type restoreFunc func(ctx context.Context) error

// unloggedFields collects one restoreFunc per entity id.
type unloggedFields map[uuid.UUID]restoreFunc

func (k unloggedFields) add(id uuid.UUID, fn restoreFunc) {
	if prev, ok := k[id]; ok {
		k[id] = func(ctx context.Context) error { return errors.Join(prev(ctx), fn(ctx)) }
		return
	}
	k[id] = fn
}

// captureUnloggedFields reads every main-branch value that no event carries
// and returns, per entity id, a function that writes it back.
func captureUnloggedFields(ctx context.Context, rs ReadModelStore) (unloggedFields, error) {
	kept := make(unloggedFields)
	if err := keepPersonFields(ctx, rs, kept); err != nil {
		return nil, err
	}
	if err := keepLifeEventCoordinates(ctx, rs, kept); err != nil {
		return nil, err
	}

	families, err := ListAll(ctx, rebuildPageSize, rs.ListFamilies)
	if err != nil {
		return nil, fmt.Errorf("list families: %w", err)
	}
	for i := range families {
		id := families[i].ID
		err := keepExternalIDs(ctx, kept, "family", id,
			func(ctx context.Context) ([]FamilyExternalIDReadModel, error) {
				return rs.GetFamilyExternalIDs(ctx, domain.MainBranchID, id)
			},
			func(ctx context.Context) (bool, error) {
				row, err := rs.GetFamily(ctx, domain.MainBranchID, id)
				return row != nil, err
			},
			func(ctx context.Context, ids []FamilyExternalIDReadModel) error {
				return rs.ReplaceFamilyExternalIDs(ctx, domain.MainBranchID, id, ids)
			})
		if err != nil {
			return nil, err
		}
	}

	sources, err := ListAll(ctx, rebuildPageSize, rs.ListSources)
	if err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}
	for i := range sources {
		id := sources[i].ID
		err := keepExternalIDs(ctx, kept, "source", id,
			func(ctx context.Context) ([]SourceExternalIDReadModel, error) {
				return rs.GetSourceExternalIDs(ctx, domain.MainBranchID, id)
			},
			func(ctx context.Context) (bool, error) {
				row, err := rs.GetSource(ctx, domain.MainBranchID, id)
				return row != nil, err
			},
			func(ctx context.Context, ids []SourceExternalIDReadModel) error {
				return rs.ReplaceSourceExternalIDs(ctx, domain.MainBranchID, id, ids)
			})
		if err != nil {
			return nil, err
		}
	}

	repos, err := ListAll(ctx, rebuildPageSize, rs.ListRepositories)
	if err != nil {
		return nil, fmt.Errorf("list repositories: %w", err)
	}
	for i := range repos {
		id := repos[i].ID
		err := keepExternalIDs(ctx, kept, "repository", id,
			func(ctx context.Context) ([]RepositoryExternalIDReadModel, error) {
				return rs.GetRepositoryExternalIDs(ctx, id)
			},
			func(ctx context.Context) (bool, error) {
				row, err := rs.GetRepository(ctx, id)
				return row != nil, err
			},
			func(ctx context.Context, ids []RepositoryExternalIDReadModel) error {
				return rs.ReplaceRepositoryExternalIDs(ctx, id, ids)
			})
		if err != nil {
			return nil, err
		}
	}
	return kept, nil
}

// keepPersonFields keeps each main person's coordinates, brick wall and EXIDs.
func keepPersonFields(ctx context.Context, rs ReadModelStore, kept unloggedFields) error {
	persons, err := ListAll(ctx, rebuildPageSize, rs.ListPersons)
	if err != nil {
		return fmt.Errorf("list persons: %w", err)
	}
	for i := range persons {
		p := persons[i]
		if p.BirthPlaceLat != nil || p.BirthPlaceLong != nil || p.DeathPlaceLat != nil || p.DeathPlaceLong != nil ||
			p.BrickWallNote != "" || p.BrickWallSince != nil || p.BrickWallResolvedAt != nil {
			kept.add(p.ID, func(ctx context.Context) error {
				row, err := rs.GetPerson(ctx, domain.MainBranchID, p.ID)
				if err != nil || row == nil {
					return err
				}
				row.BirthPlaceLat, row.BirthPlaceLong = p.BirthPlaceLat, p.BirthPlaceLong
				row.DeathPlaceLat, row.DeathPlaceLong = p.DeathPlaceLat, p.DeathPlaceLong
				row.BrickWallNote, row.BrickWallSince, row.BrickWallResolvedAt = p.BrickWallNote, p.BrickWallSince, p.BrickWallResolvedAt
				return rs.SavePerson(ctx, domain.MainBranchID, row)
			})
		}
		err := keepExternalIDs(ctx, kept, "person", p.ID,
			func(ctx context.Context) ([]PersonExternalIDReadModel, error) {
				return rs.GetPersonExternalIDs(ctx, domain.MainBranchID, p.ID)
			},
			func(ctx context.Context) (bool, error) {
				row, err := rs.GetPerson(ctx, domain.MainBranchID, p.ID)
				return row != nil, err
			},
			func(ctx context.Context, ids []PersonExternalIDReadModel) error {
				return rs.ReplacePersonExternalIDs(ctx, domain.MainBranchID, p.ID, ids)
			})
		if err != nil {
			return err
		}
	}
	return nil
}

// keepLifeEventCoordinates keeps each main life event's place coordinates.
func keepLifeEventCoordinates(ctx context.Context, rs ReadModelStore, kept unloggedFields) error {
	lifeEvents, err := ListAll(ctx, rebuildPageSize, rs.ListEvents)
	if err != nil {
		return fmt.Errorf("list life events: %w", err)
	}
	for i := range lifeEvents {
		e := lifeEvents[i]
		if e.PlaceLat == nil && e.PlaceLong == nil {
			continue
		}
		kept.add(e.ID, func(ctx context.Context) error {
			row, err := rs.GetEvent(ctx, domain.MainBranchID, e.ID)
			if err != nil || row == nil {
				return err
			}
			row.PlaceLat, row.PlaceLong = e.PlaceLat, e.PlaceLong
			return rs.SaveEvent(ctx, domain.MainBranchID, row)
		})
	}
	return nil
}

// keepExternalIDs keeps one entity's EXIDs, if it has any, to be written back
// while the entity exists.
func keepExternalIDs[T any](ctx context.Context, kept unloggedFields, kind string, id uuid.UUID,
	get func(context.Context) ([]T, error),
	exists func(context.Context) (bool, error),
	replace func(context.Context, []T) error,
) error {
	ids, err := get(ctx)
	if err != nil {
		return fmt.Errorf("%s %s external ids: %w", kind, id, err)
	}
	if len(ids) == 0 {
		return nil
	}
	kept.add(id, func(ctx context.Context) error {
		ok, err := exists(ctx)
		if err != nil || !ok {
			return err
		}
		return replace(ctx, ids)
	})
	return nil
}
