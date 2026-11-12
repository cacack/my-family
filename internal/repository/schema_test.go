package repository_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// TestCheckReadModelSchema_ReportsStoreErrors proves a check that cannot read
// the stamp or the log, or cannot stamp a new database, fails rather than
// letting a server start on an unknown schema (#680).
func TestCheckReadModelSchema_ReportsStoreErrors(t *testing.T) {
	ctx := context.Background()
	for _, op := range []string{"get version", "stamp"} {
		s := newRebuildStores()
		if err := repository.CheckReadModelSchema(ctx, s.events, &failingRead{ReadModelStore: s.read, fail: op}); !errors.Is(err, errRebuildTest) {
			t.Errorf("%s: check = %v, want the injected failure", op, err)
		}
	}
	s := newRebuildStores()
	if err := repository.CheckReadModelSchema(ctx, failingEvents{s.events}, s.read); !errors.Is(err, errRebuildTest) {
		t.Errorf("log read: check = %v, want the injected failure", err)
	}
}

// TestCheckReadModelSchema_MismatchNamesTheRebuild proves the refusal tells the
// operator both versions and what to run.
func TestCheckReadModelSchema_MismatchNamesTheRebuild(t *testing.T) {
	ctx := context.Background()
	s := newRebuildStores()
	if err := s.read.SetSchemaVersion(ctx, repository.ReadModelSchemaVersion+1); err != nil {
		t.Fatalf("stamp: %v", err)
	}
	err := repository.CheckReadModelSchema(ctx, s.events, s.read)
	if !errors.Is(err, repository.ErrReadModelSchemaMismatch) {
		t.Fatalf("check = %v, want ErrReadModelSchemaMismatch", err)
	}
	for _, want := range []string{"myfamily rebuild", "version 2", "version 1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// failingSnapshots fails the named snapshot-registry or stream operation.
type failingSnapshots struct {
	repository.SnapshotStore
	fail string
}

func (f failingSnapshots) List(ctx context.Context, branchID domain.BranchID) ([]*domain.Snapshot, error) {
	if f.fail == "list snapshots" {
		return nil, errRebuildTest
	}
	return f.SnapshotStore.List(ctx, branchID)
}

// failingStreams fails ReadStream or Append.
type failingStreams struct {
	repository.EventStore
	fail string
}

func (f failingStreams) ReadStream(ctx context.Context, id uuid.UUID) ([]repository.StoredEvent, error) {
	if f.fail == "read stream" {
		return nil, errRebuildTest
	}
	return f.EventStore.ReadStream(ctx, id)
}

func (f failingStreams) Append(ctx context.Context, id uuid.UUID, streamType string, events []domain.Event, expected int64, scope repository.AppendScope) error {
	if f.fail == "append" && streamType == "snapshot" {
		return errRebuildTest
	}
	return f.EventStore.Append(ctx, id, streamType, events, expected, scope)
}

// TestRebuildReadModel_SnapshotBackfillReportsErrors proves the rebuild stops
// before its reset when it cannot log a pre-#624 snapshot (#680).
func TestRebuildReadModel_SnapshotBackfillReportsErrors(t *testing.T) {
	for _, op := range []string{"list snapshots", "read stream", "append"} {
		t.Run(op, func(t *testing.T) {
			ctx := context.Background()
			s := newRebuildStores()
			h := command.NewHandlerWithBranches(s.events, s.read, s.branches, s.snapshots)
			if _, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Example"}); err != nil {
				t.Fatalf("create person: %v", err)
			}
			legacy, err := domain.NewSnapshot("legacy", "", 1)
			if err != nil {
				t.Fatalf("new snapshot: %v", err)
			}
			if err := s.snapshots.Create(ctx, legacy); err != nil {
				t.Fatalf("seed snapshot: %v", err)
			}
			read := &failingRead{ReadModelStore: s.read}
			_, err = repository.RebuildReadModel(ctx, failingStreams{s.events, op}, read, s.branches, failingSnapshots{s.snapshots, op})
			if !errors.Is(err, errRebuildTest) {
				t.Fatalf("rebuild = %v, want the injected %q failure", err, op)
			}
			if read.didReset {
				t.Error("the rebuild reset the read model after a failed backfill")
			}
		})
	}
}

// TestRebuildReadModel_NoSnapshotStore proves a rebuild without a snapshot
// registry skips the snapshot backfill.
func TestRebuildReadModel_NoSnapshotStore(t *testing.T) {
	s := newRebuildStores()
	result, err := repository.RebuildReadModel(context.Background(), s.events, s.read, s.branches, nil)
	if err != nil || result.BackfilledSnapshots != 0 {
		t.Fatalf("rebuild = %+v, %v; want no backfill and no error", result, err)
	}
}

// TestDecodeEvent_AppliesUpcaster proves DecodeEvent runs an event type's
// upcaster on the stored data before decoding it, and reports its failure
// (#680, ADR-006).
func TestDecodeEvent_AppliesUpcaster(t *testing.T) {
	restore := repository.SetEventUpcasterForTest("PersonCreated", func(data json.RawMessage) (json.RawMessage, error) {
		var old map[string]any
		if err := json.Unmarshal(data, &old); err != nil {
			return nil, err
		}
		if first, ok := old["first_name"]; ok {
			old["given_name"] = first
			delete(old, "first_name")
		}
		return json.Marshal(old)
	})
	defer restore()

	stored := repository.StoredEvent{
		EventType: "PersonCreated",
		Data:      json.RawMessage(`{"id":"` + uuid.NewString() + `","timestamp":"` + time.Now().UTC().Format(time.RFC3339) + `","person_id":"` + uuid.NewString() + `","first_name":"Ada","surname":"Example"}`),
	}
	decoded, err := stored.DecodeEvent()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := decoded.(domain.PersonCreated).GivenName; got != "Ada" {
		t.Errorf("GivenName = %q, want the upcast first_name", got)
	}

	repository.SetEventUpcasterForTest("PersonCreated", func(json.RawMessage) (json.RawMessage, error) {
		return nil, errRebuildTest
	})
	if _, err := stored.DecodeEvent(); !errors.Is(err, errRebuildTest) {
		t.Errorf("decode with a failing upcaster = %v, want its error", err)
	}
}
