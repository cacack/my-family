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
// cannot be rebuilt from the log, so they are read before the reset and
// written back: coordinates and external identifiers once the entity's first
// event has been replayed, which is where import wrote them, and everything
// again at the end. Brick walls are written back only at the end, so a branch
// row the replay copies from main does not pick up a brick wall set later.
//
// The rebuild is safe to abort. Every event is decoded before anything is
// deleted, so an event the replay could not read fails the rebuild while the
// read model is intact. The values above are saved as the rebuild state
// (SaveRebuildState) before the reset, and only a rebuild that succeeds clears
// it: while it exists `serve` warns that the read model is out of sync, and a
// rerun after an interrupted rebuild restores the values from it, since the
// reset has removed them from the read model.
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
	replayErr := replayLog(ctx, events, rs, projector, kept, &result)

	// Write back the unlogged fields once more, now that every entity that
	// will exist does. An entity that is not in the read model is skipped.
	var restoreErr error
	if ctx.Err() == nil {
		restoreErr = kept.restoreAll(ctx, rs)
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

// replayLog projects every logged event in position order, restoring an
// entity's imported fields right after its first main event. It stops only if
// the log cannot be read, an event cannot be decoded, a restore fails or ctx
// is done.
func replayLog(ctx context.Context, events EventStore, rs ReadModelStore, projector *Projector, kept *unloggedFields, result *RebuildResult) error {
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
				continue
			}
			if stored.BranchID.IsMain() && stored.Version == 1 {
				if err := kept.restoreImported(ctx, rs, stored.StreamID); err != nil {
					return errors.Join(firstFailure, err)
				}
			}
		}
	}
}

// unloggedFields holds the main-branch read-model values that no event
// carries, by entity id. It is saved as the rebuild state, hence the JSON tags.
type unloggedFields struct {
	PersonCoords          map[uuid.UUID]personCoords                    `json:"person_coords"`
	BrickWalls            map[uuid.UUID]brickWall                       `json:"brick_walls"`
	EventCoords           map[uuid.UUID]placeCoords                     `json:"event_coords"`
	PersonExternalIDs     map[uuid.UUID][]PersonExternalIDReadModel     `json:"person_external_ids"`
	FamilyExternalIDs     map[uuid.UUID][]FamilyExternalIDReadModel     `json:"family_external_ids"`
	SourceExternalIDs     map[uuid.UUID][]SourceExternalIDReadModel     `json:"source_external_ids"`
	RepositoryExternalIDs map[uuid.UUID][]RepositoryExternalIDReadModel `json:"repository_external_ids"`
}

type personCoords struct {
	BirthLat  *string `json:"birth_lat,omitempty"`
	BirthLong *string `json:"birth_long,omitempty"`
	DeathLat  *string `json:"death_lat,omitempty"`
	DeathLong *string `json:"death_long,omitempty"`
}

type placeCoords struct {
	Lat  *string `json:"lat,omitempty"`
	Long *string `json:"long,omitempty"`
}

type brickWall struct {
	Note       string     `json:"note,omitempty"`
	Since      *time.Time `json:"since,omitempty"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}

func newUnloggedFields() *unloggedFields {
	return &unloggedFields{
		PersonCoords:          map[uuid.UUID]personCoords{},
		BrickWalls:            map[uuid.UUID]brickWall{},
		EventCoords:           map[uuid.UUID]placeCoords{},
		PersonExternalIDs:     map[uuid.UUID][]PersonExternalIDReadModel{},
		FamilyExternalIDs:     map[uuid.UUID][]FamilyExternalIDReadModel{},
		SourceExternalIDs:     map[uuid.UUID][]SourceExternalIDReadModel{},
		RepositoryExternalIDs: map[uuid.UUID][]RepositoryExternalIDReadModel{},
	}
}

// merge copies every value of src into k, replacing k's value for the same
// entity and field group.
func (k *unloggedFields) merge(src *unloggedFields) {
	mergeInto(k.PersonCoords, src.PersonCoords)
	mergeInto(k.BrickWalls, src.BrickWalls)
	mergeInto(k.EventCoords, src.EventCoords)
	mergeInto(k.PersonExternalIDs, src.PersonExternalIDs)
	mergeInto(k.FamilyExternalIDs, src.FamilyExternalIDs)
	mergeInto(k.SourceExternalIDs, src.SourceExternalIDs)
	mergeInto(k.RepositoryExternalIDs, src.RepositoryExternalIDs)
}

func mergeInto[V any](dst, src map[uuid.UUID]V) {
	for id, v := range src {
		dst[id] = v
	}
}

// loadUnloggedFields returns the values to carry over: those saved by an
// unfinished earlier rebuild, whose reset removed them from the read model,
// overlaid with those the read model holds now.
func loadUnloggedFields(ctx context.Context, rs ReadModelStore) (*unloggedFields, error) {
	kept := newUnloggedFields()
	saved, err := rs.GetRebuildState(ctx)
	if err != nil {
		return nil, fmt.Errorf("read rebuild state: %w", err)
	}
	if saved != nil {
		var prior unloggedFields
		if err := json.Unmarshal(saved, &prior); err != nil {
			return nil, fmt.Errorf("decode rebuild state: %w", err)
		}
		slog.Warn("an earlier rebuild did not finish; restoring the values it saved before its reset")
		kept.merge(&prior)
	}
	live, err := captureUnloggedFields(ctx, rs)
	if err != nil {
		return nil, fmt.Errorf("read fields that are not in the event log: %w", err)
	}
	kept.merge(live)
	return kept, nil
}

// captureUnloggedFields reads every main-branch value that no event carries.
func captureUnloggedFields(ctx context.Context, rs ReadModelStore) (*unloggedFields, error) {
	kept := newUnloggedFields()
	if err := capturePersonsAndLifeEvents(ctx, rs, kept); err != nil {
		return nil, err
	}

	families, err := ListAll(ctx, rebuildPageSize, rs.ListFamilies)
	if err != nil {
		return nil, fmt.Errorf("list families: %w", err)
	}
	for i := range families {
		id := families[i].ID
		if err := keepExternalIDs(kept.FamilyExternalIDs, "family", id, func() ([]FamilyExternalIDReadModel, error) {
			return rs.GetFamilyExternalIDs(ctx, domain.MainBranchID, id)
		}); err != nil {
			return nil, err
		}
	}

	sources, err := ListAll(ctx, rebuildPageSize, rs.ListSources)
	if err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}
	for i := range sources {
		id := sources[i].ID
		if err := keepExternalIDs(kept.SourceExternalIDs, "source", id, func() ([]SourceExternalIDReadModel, error) {
			return rs.GetSourceExternalIDs(ctx, domain.MainBranchID, id)
		}); err != nil {
			return nil, err
		}
	}

	repos, err := ListAll(ctx, rebuildPageSize, rs.ListRepositories)
	if err != nil {
		return nil, fmt.Errorf("list repositories: %w", err)
	}
	for i := range repos {
		id := repos[i].ID
		if err := keepExternalIDs(kept.RepositoryExternalIDs, "repository", id, func() ([]RepositoryExternalIDReadModel, error) {
			return rs.GetRepositoryExternalIDs(ctx, id)
		}); err != nil {
			return nil, err
		}
	}
	return kept, nil
}

// capturePersonsAndLifeEvents reads each main person's coordinates, brick
// wall and EXIDs, and each main life event's coordinates, into kept.
func capturePersonsAndLifeEvents(ctx context.Context, rs ReadModelStore, kept *unloggedFields) error {
	persons, err := ListAll(ctx, rebuildPageSize, rs.ListPersons)
	if err != nil {
		return fmt.Errorf("list persons: %w", err)
	}
	for i := range persons {
		p := &persons[i]
		if p.BirthPlaceLat != nil || p.BirthPlaceLong != nil || p.DeathPlaceLat != nil || p.DeathPlaceLong != nil {
			kept.PersonCoords[p.ID] = personCoords{BirthLat: p.BirthPlaceLat, BirthLong: p.BirthPlaceLong, DeathLat: p.DeathPlaceLat, DeathLong: p.DeathPlaceLong}
		}
		if p.BrickWallNote != "" || p.BrickWallSince != nil || p.BrickWallResolvedAt != nil {
			kept.BrickWalls[p.ID] = brickWall{Note: p.BrickWallNote, Since: p.BrickWallSince, ResolvedAt: p.BrickWallResolvedAt}
		}
		if err := keepExternalIDs(kept.PersonExternalIDs, "person", p.ID, func() ([]PersonExternalIDReadModel, error) {
			return rs.GetPersonExternalIDs(ctx, domain.MainBranchID, p.ID)
		}); err != nil {
			return err
		}
	}

	lifeEvents, err := ListAll(ctx, rebuildPageSize, rs.ListEvents)
	if err != nil {
		return fmt.Errorf("list life events: %w", err)
	}
	for i := range lifeEvents {
		if e := &lifeEvents[i]; e.PlaceLat != nil || e.PlaceLong != nil {
			kept.EventCoords[e.ID] = placeCoords{Lat: e.PlaceLat, Long: e.PlaceLong}
		}
	}
	return nil
}

// keepExternalIDs records one entity's EXIDs in into, if it has any.
func keepExternalIDs[T any](into map[uuid.UUID][]T, kind string, id uuid.UUID, get func() ([]T, error)) error {
	ids, err := get()
	if err != nil {
		return fmt.Errorf("%s %s external ids: %w", kind, id, err)
	}
	if len(ids) > 0 {
		into[id] = ids
	}
	return nil
}

// restoreImported writes back the coordinates and EXIDs kept for id, the
// values GEDCOM import writes right after the entity's first event. Each
// write is a no-op when the entity's main row does not exist.
func (k *unloggedFields) restoreImported(ctx context.Context, rs ReadModelStore, id uuid.UUID) error {
	var errs []error
	if c, ok := k.PersonCoords[id]; ok {
		errs = append(errs, updateMainPerson(ctx, rs, id, func(p *PersonReadModel) {
			p.BirthPlaceLat, p.BirthPlaceLong, p.DeathPlaceLat, p.DeathPlaceLong = c.BirthLat, c.BirthLong, c.DeathLat, c.DeathLong
		}))
	}
	if c, ok := k.EventCoords[id]; ok {
		errs = append(errs, restoreEventCoords(ctx, rs, id, c))
	}
	if ids, ok := k.PersonExternalIDs[id]; ok {
		errs = append(errs, replaceIfExists(func() (bool, error) {
			row, err := rs.GetPerson(ctx, domain.MainBranchID, id)
			return row != nil, err
		}, func() error { return rs.ReplacePersonExternalIDs(ctx, domain.MainBranchID, id, ids) }))
	}
	if ids, ok := k.FamilyExternalIDs[id]; ok {
		errs = append(errs, replaceIfExists(func() (bool, error) {
			row, err := rs.GetFamily(ctx, domain.MainBranchID, id)
			return row != nil, err
		}, func() error { return rs.ReplaceFamilyExternalIDs(ctx, domain.MainBranchID, id, ids) }))
	}
	if ids, ok := k.SourceExternalIDs[id]; ok {
		errs = append(errs, replaceIfExists(func() (bool, error) {
			row, err := rs.GetSource(ctx, domain.MainBranchID, id)
			return row != nil, err
		}, func() error { return rs.ReplaceSourceExternalIDs(ctx, domain.MainBranchID, id, ids) }))
	}
	if ids, ok := k.RepositoryExternalIDs[id]; ok {
		errs = append(errs, replaceIfExists(func() (bool, error) {
			row, err := rs.GetRepository(ctx, id)
			return row != nil, err
		}, func() error { return rs.ReplaceRepositoryExternalIDs(ctx, id, ids) }))
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("restore unlogged fields of %s: %w", id, err)
	}
	return nil
}

// restoreAll writes back every kept value, brick walls included, onto the
// main rows that exist.
func (k *unloggedFields) restoreAll(ctx context.Context, rs ReadModelStore) error {
	ids := map[uuid.UUID]struct{}{}
	addKeys(ids, k.PersonCoords)
	addKeys(ids, k.EventCoords)
	addKeys(ids, k.PersonExternalIDs)
	addKeys(ids, k.FamilyExternalIDs)
	addKeys(ids, k.SourceExternalIDs)
	addKeys(ids, k.RepositoryExternalIDs)
	var errs []error
	for id := range ids {
		errs = append(errs, k.restoreImported(ctx, rs, id))
	}
	for id, bw := range k.BrickWalls {
		if err := updateMainPerson(ctx, rs, id, func(p *PersonReadModel) {
			p.BrickWallNote, p.BrickWallSince, p.BrickWallResolvedAt = bw.Note, bw.Since, bw.ResolvedAt
		}); err != nil {
			errs = append(errs, fmt.Errorf("restore brick wall of %s: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

func addKeys[V any](into map[uuid.UUID]struct{}, from map[uuid.UUID]V) {
	for id := range from {
		into[id] = struct{}{}
	}
}

// updateMainPerson applies set to the person's main row, if it exists.
func updateMainPerson(ctx context.Context, rs ReadModelStore, id uuid.UUID, set func(*PersonReadModel)) error {
	row, err := rs.GetPerson(ctx, domain.MainBranchID, id)
	if err != nil || row == nil {
		return err
	}
	set(row)
	return rs.SavePerson(ctx, domain.MainBranchID, row)
}

// restoreEventCoords writes a life event's coordinates back, if it exists.
func restoreEventCoords(ctx context.Context, rs ReadModelStore, id uuid.UUID, c placeCoords) error {
	row, err := rs.GetEvent(ctx, domain.MainBranchID, id)
	if err != nil || row == nil {
		return err
	}
	row.PlaceLat, row.PlaceLong = c.Lat, c.Long
	return rs.SaveEvent(ctx, domain.MainBranchID, row)
}

// replaceIfExists calls replace when exists reports the entity's row exists.
func replaceIfExists(exists func() (bool, error), replace func() error) error {
	ok, err := exists()
	if err != nil || !ok {
		return err
	}
	return replace()
}
