package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Read-model schema versioning and the pre-#624 snapshot backfill (#680,
// ADR-006), on every backend (DB-001).

// TestReadModelSchema_NewDatabaseIsStamped proves a database with no stamp and
// an empty event log is taken as new: it is stamped and passes.
func TestReadModelSchema_NewDatabaseIsStamped(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			if err := st.read.SetSchemaVersion(ctx, 0); err != nil {
				t.Fatalf("clear stamp: %v", err)
			}
			if err := repository.CheckReadModelSchema(ctx, st.events, st.read); err != nil {
				t.Fatalf("check on a new database: %v", err)
			}
			if v, err := st.read.GetSchemaVersion(ctx); err != nil || v != repository.ReadModelSchemaVersion {
				t.Fatalf("stamp = %d, %v; want %d", v, err, repository.ReadModelSchemaVersion)
			}
		})
	}
}

// TestReadModelSchema_MismatchIsRepairedByRebuild proves a read model with
// events behind it and no stamp, or another version's stamp, is refused with
// an instruction to rebuild, and that the rebuild stamps it and keeps the data.
func TestReadModelSchema_MismatchIsRepairedByRebuild(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			ada, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Example"})
			if err != nil {
				t.Fatalf("create person: %v", err)
			}

			for _, stamped := range []int{0, repository.ReadModelSchemaVersion + 1} {
				if err := st.read.SetSchemaVersion(ctx, stamped); err != nil {
					t.Fatalf("stamp %d: %v", stamped, err)
				}
				err := repository.CheckReadModelSchema(ctx, st.events, st.read)
				if !errors.Is(err, repository.ErrReadModelSchemaMismatch) {
					t.Fatalf("check at version %d = %v, want ErrReadModelSchemaMismatch", stamped, err)
				}
				if v, _ := st.read.GetSchemaVersion(ctx); v != stamped {
					t.Errorf("check changed the stamp from %d to %d; it must never rebuild or restamp", stamped, v)
				}
			}

			if _, err := repository.RebuildReadModel(ctx, st.events, st.read, st.branches, st.snapshots); err != nil {
				t.Fatalf("rebuild: %v", err)
			}
			if err := repository.CheckReadModelSchema(ctx, st.events, st.read); err != nil {
				t.Fatalf("check after rebuild: %v", err)
			}
			if row, err := st.read.GetPerson(ctx, domain.MainBranchID, ada.ID); err != nil || row == nil || row.GivenName != "Ada" {
				t.Fatalf("person after rebuild = %+v, %v; want Ada", row, err)
			}
		})
	}
}

// TestRebuild_BackfillsSnapshotsCreatedBeforeTheLog proves a snapshot written
// straight to the registry, as before #624, gets a SnapshotCreated with its
// original time, so the log alone rebuilds it; and that a snapshot with an
// event is left alone.
func TestRebuild_BackfillsSnapshotsCreatedBeforeTheLog(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			if _, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Example"}); err != nil {
				t.Fatalf("create person: %v", err)
			}
			legacy, err := domain.NewSnapshot("before #624", "written to the registry only", 1)
			if err != nil {
				t.Fatalf("new snapshot: %v", err)
			}
			legacy.CreatedAt = time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)
			if err := st.snapshots.Create(ctx, legacy); err != nil {
				t.Fatalf("seed legacy snapshot: %v", err)
			}
			logged, err := h.CreateSnapshot(ctx, "logged", "")
			if err != nil {
				t.Fatalf("create snapshot: %v", err)
			}

			result, err := repository.RebuildReadModel(ctx, st.events, st.read, st.branches, st.snapshots)
			if err != nil {
				t.Fatalf("rebuild: %v", err)
			}
			if result.BackfilledSnapshots != 1 {
				t.Fatalf("BackfilledSnapshots = %d, want 1", result.BackfilledSnapshots)
			}
			stream, err := st.events.ReadStream(ctx, legacy.ID)
			if err != nil || len(stream) != 1 || stream[0].EventType != "SnapshotCreated" {
				t.Fatalf("legacy snapshot stream = %+v, %v; want one SnapshotCreated", stream, err)
			}
			decoded, err := stream[0].DecodeEvent()
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if e := decoded.(domain.SnapshotCreated); e.Name != legacy.Name || e.Position != legacy.Position ||
				e.Description != legacy.Description || !e.OccurredAt().Equal(legacy.CreatedAt) {
				t.Errorf("backfilled event = %+v, want the snapshot as created", e)
			}
			if stream, err := st.events.ReadStream(ctx, logged.ID); err != nil || len(stream) != 1 {
				t.Errorf("logged snapshot stream = %d events, %v; want its own event only", len(stream), err)
			}

			// The log now rebuilds the row on its own.
			if err := st.snapshots.Delete(ctx, legacy.ID); err != nil {
				t.Fatalf("drop registry row: %v", err)
			}
			result, err = repository.RebuildReadModel(ctx, st.events, st.read, st.branches, st.snapshots)
			if err != nil {
				t.Fatalf("second rebuild: %v", err)
			}
			if result.BackfilledSnapshots != 0 {
				t.Errorf("second rebuild BackfilledSnapshots = %d, want 0", result.BackfilledSnapshots)
			}
			got, err := st.snapshots.Get(ctx, legacy.ID)
			if err != nil || got.Name != legacy.Name || got.Position != legacy.Position || !got.BranchID.IsMain() ||
				!got.CreatedAt.Equal(legacy.CreatedAt) {
				t.Fatalf("snapshot replayed from the log = %+v, %v; want %+v", got, err, legacy)
			}
		})
	}
}
