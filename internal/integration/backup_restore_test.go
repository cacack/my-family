package integration_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/auth"
	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/storage"
	"github.com/cacack/my-family/internal/testutil/pgtest"
)

// Backup and restore across the two SQL backends (#1642, ADR-010).
//
// An archive written from either backend restores into either: the restored
// log equals the source's event for event and position for position, the
// restored read model equals a fresh rebuild of the source, and the source's
// owner signs in to the restored instance.

const restorePassword = "correct horse battery staple"

// sqlBackends are the backends a backup is written from and restored into.
var sqlBackends = []struct {
	name string
	cfg  func(t *testing.T) *config.Config
}{
	{"SQLite", func(t *testing.T) *config.Config {
		return &config.Config{SQLitePath: filepath.Join(t.TempDir(), "myfamily.db")}
	}},
	{"Postgres", func(t *testing.T) *config.Config { return &config.Config{DatabaseURL: pgtest.NewDatabase(t)} }},
}

func TestBackupRestore_AcrossBackends(t *testing.T) {
	for _, from := range sqlBackends {
		for _, to := range sqlBackends {
			t.Run(from.name+"To"+to.name, func(t *testing.T) {
				ctx := context.Background()
				source := from.cfg(t)
				branchID := fillInstance(t, source)

				archive := filepath.Join(t.TempDir(), "backup.zip")
				writeArchive(t, source, archive)

				// The fresh rebuild of the source that the restore must equal.
				rebuilt := openStores(t, source, storage.AccessRebuild)
				if _, err := rebuilt.RebuildReadModel(ctx); err != nil {
					t.Fatalf("rebuild the source: %v", err)
				}
				wantLog := readLog(t, rebuilt.Events)
				wantDump := dumpReadModel(t, ctx, harnessStores(rebuilt), branchID)
				closeStores(t, rebuilt)

				target := to.cfg(t)
				result, err := storage.Restore(ctx, target, archive)
				if err != nil {
					t.Fatalf("Restore: %v", err)
				}
				if result.Manifest.EventCount != int64(len(wantLog)) {
					t.Errorf("restored %d events, the source holds %d", result.Manifest.EventCount, len(wantLog))
				}

				restored := openStores(t, target, storage.AccessServe)
				defer closeStores(t, restored)
				// PostgreSQL keeps microseconds, so a SQLite time loses its
				// nanoseconds on the way there.
				precision := time.Nanosecond
				if from.name != to.name {
					precision = time.Microsecond
				}
				assertSameLog(t, wantLog, readLog(t, restored.Events), precision)

				// The read model is compared on the source's backend, since the
				// two backends' read models are not compared row for row
				// anywhere else: across backends, the restored instance is
				// backed up and restored back first.
				compared := restored
				if from.name != to.name {
					back := filepath.Join(t.TempDir(), "back.zip")
					writeArchive(t, target, back)
					home := from.cfg(t)
					if _, err := storage.Restore(ctx, home, back); err != nil {
						t.Fatalf("restore back into %s: %v", from.name, err)
					}
					compared = openStores(t, home, storage.AccessServe)
					defer closeStores(t, compared)
				}
				assertSameDump(t, roundDump(t, wantDump, precision),
					roundDump(t, dumpReadModel(t, ctx, harnessStores(compared), branchID), precision))

				svc := auth.NewService(restored.Accounts)
				if _, _, err := svc.SignIn(ctx, "owner", restorePassword, "test"); err != nil {
					t.Errorf("the source's owner cannot sign in to the restored instance: %v", err)
				}
			})
		}
	}
}

// fillInstance gives the instance at cfg a tree, a branch, a snapshot and an
// owner, and returns the branch's ID.
func fillInstance(t *testing.T, cfg *config.Config) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	st := openStores(t, cfg, storage.AccessServe)
	defer closeStores(t, st)
	h := command.NewHandlerWithBranches(st.Events, st.ReadModel, st.Branches, st.Snapshots)
	if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(rebuildGedcom)}); err != nil {
		t.Fatalf("import: %v", err)
	}
	persons, err := repository.ListAll(ctx, 100, st.ReadModel.ListPersons)
	if err != nil || len(persons) != 1 {
		t.Fatalf("imported persons = %v, %v; want one", persons, err)
	}
	john := persons[0]
	branch, err := h.CreateBranch(ctx, "research", "")
	if err != nil {
		t.Fatalf("create branch: %v", err)
	}
	notes := "branch only"
	if _, err := h.WithBranch(branch).UpdatePerson(ctx, command.UpdatePersonInput{ID: john.ID, Notes: command.Set(notes), Version: john.Version}); err != nil {
		t.Fatalf("branch update: %v", err)
	}
	if err := h.SetBrickWall(ctx, john.ID, "no parents found"); err != nil {
		t.Fatalf("set brick wall: %v", err)
	}
	if _, err := h.CreateSnapshot(ctx, "before backup", ""); err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	if _, err := auth.NewService(st.Accounts).CreateOwner(ctx, auth.CommandLine, "owner", "Owner", restorePassword); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	return branch.ID
}

func writeArchive(t *testing.T, cfg *config.Config, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := storage.Backup(context.Background(), cfg, f, "test"); err != nil {
		t.Fatalf("Backup: %v", err)
	}
}

func openStores(t *testing.T, cfg *config.Config, access storage.Access) *storage.Stores {
	t.Helper()
	st, err := storage.Open(cfg, access)
	if err != nil {
		t.Fatalf("open stores: %v", err)
	}
	return st
}

func closeStores(t *testing.T, st *storage.Stores) {
	t.Helper()
	if err := st.Close(); err != nil {
		t.Errorf("close stores: %v", err)
	}
}

func harnessStores(st *storage.Stores) stores {
	return stores{events: st.Events, read: st.ReadModel, snapshots: st.Snapshots, branches: st.Branches}
}

func readLog(t *testing.T, events repository.EventStore) []repository.StoredEvent {
	t.Helper()
	log, err := events.ReadAll(context.Background(), 0, 100000)
	if err != nil {
		t.Fatalf("read the log: %v", err)
	}
	return log
}

// assertSameLog compares two logs event for event, in order. Event data is
// compared as JSON values, since PostgreSQL's JSONB orders keys its own way.
func assertSameLog(t *testing.T, want, got []repository.StoredEvent, precision time.Duration) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("restored log holds %d events, want %d", len(got), len(want))
	}
	for i := range want {
		w, g := want[i], got[i]
		if w.ID != g.ID || w.StreamID != g.StreamID || w.StreamType != g.StreamType || w.BranchID != g.BranchID ||
			w.EventType != g.EventType || w.Version != g.Version || w.Position != g.Position ||
			!w.Timestamp.Round(precision).Equal(g.Timestamp.Round(precision)) {
			t.Errorf("event %d = %+v, want %+v", i, g, w)
			continue
		}
		if !sameJSON(t, w.Data, g.Data) || !sameJSON(t, w.Metadata, g.Metadata) {
			t.Errorf("event %d data = %s %s, want %s %s", i, g.Data, g.Metadata, w.Data, w.Metadata)
		}
	}
}

func sameJSON(t *testing.T, a, b json.RawMessage) bool {
	t.Helper()
	if len(a) == 0 || len(b) == 0 {
		return len(a) == len(b)
	}
	var av, bv any
	if err := json.Unmarshal(a, &av); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(av, bv)
}

// roundDump rounds every time in a read-model dump to precision.
func roundDump(t *testing.T, dump map[string]string, precision time.Duration) map[string]string {
	t.Helper()
	out := make(map[string]string, len(dump))
	for k, v := range dump {
		var generic any
		if err := json.Unmarshal([]byte(v), &generic); err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(roundTimes(generic, precision))
		if err != nil {
			t.Fatal(err)
		}
		out[k] = string(b)
	}
	return out
}

func roundTimes(v any, precision time.Duration) any {
	switch val := v.(type) {
	case map[string]any:
		for k, item := range val {
			val[k] = roundTimes(item, precision)
		}
	case []any:
		for i, item := range val {
			val[i] = roundTimes(item, precision)
		}
	case string:
		if ts, err := time.Parse(time.RFC3339Nano, val); err == nil {
			return ts.Round(precision).UTC().Format(time.RFC3339Nano)
		}
	}
	return v
}
