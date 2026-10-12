package api_test

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

// rivalChildLinkEventStore lets a test land a rival branch child link on a
// family's stream between the link command's read and its append, so the
// command's expected version is genuinely stale and the store itself refuses it.
// The rival goes straight to the log, below the projector, so the read model
// never shows it; only its version bump matters, hence the random child id.
type rivalChildLinkEventStore struct {
	repository.EventStore
	rival bool
}

func (s *rivalChildLinkEventStore) Append(ctx context.Context, streamID uuid.UUID, streamType string,
	events []domain.Event, expectedVersion int64, scope repository.AppendScope,
) error {
	if s.rival && !scope.BranchID.IsMain() && len(events) == 1 && events[0].EventType() == "ChildLinkedToFamily" {
		s.rival = false
		rival := domain.NewChildLinkedToFamily(domain.NewFamilyChild(streamID, uuid.New(), domain.ChildBiological))
		if err := s.EventStore.Append(ctx, streamID, streamType, []domain.Event{rival}, expectedVersion, scope); err != nil {
			return fmt.Errorf("rival append: %w", err)
		}
	}
	return s.EventStore.Append(ctx, streamID, streamType, events, expectedVersion, scope)
}

// branchChildLinkBackends builds a branch-capable server over each read-model
// backend the branch overlay must agree on (#990).
var branchChildLinkBackends = []struct {
	name  string
	setup func(t *testing.T) (*api.Server, *rivalChildLinkEventStore)
}{
	{"memory", func(t *testing.T) (*api.Server, *rivalChildLinkEventStore) {
		base := memory.NewEventStore()
		events := &rivalChildLinkEventStore{EventStore: base}
		server := api.NewServer(&config.Config{Port: 8080, LogFormat: "text"}, events,
			memory.NewReadModelStore(), memory.NewSnapshotStore(base), nil,
			api.WithBranchStore(memory.NewBranchStore()))
		return server, events
	}},
	{"sqlite", func(t *testing.T) (*api.Server, *rivalChildLinkEventStore) {
		db, err := sqlite.OpenDB(filepath.Join(t.TempDir(), "branch-child-links.db"))
		if err != nil {
			t.Fatalf("open database: %v", err)
		}
		t.Cleanup(func() { db.Close() })
		base, err := sqlite.NewEventStore(db)
		if err != nil {
			t.Fatalf("event store: %v", err)
		}
		readStore, err := sqlite.NewReadModelStore(db)
		if err != nil {
			t.Fatalf("read model store: %v", err)
		}
		snapshots, err := sqlite.NewSnapshotStore(db)
		if err != nil {
			t.Fatalf("snapshot store: %v", err)
		}
		branches, err := sqlite.NewBranchStore(db)
		if err != nil {
			t.Fatalf("branch store: %v", err)
		}
		events := &rivalChildLinkEventStore{EventStore: base}
		server := api.NewServer(&config.Config{Port: 8080, LogFormat: "text"}, events,
			readStore, snapshots, nil, api.WithBranchStore(branches))
		return server, events
	}},
}

// familyChildIDs returns the person ids of a family's children in the given
// scope ("" for the mainline).
func familyChildIDs(t *testing.T, server *api.Server, familyID, branchID string) []string {
	t.Helper()
	path := "/api/v1/families/" + familyID
	if branchID != "" {
		path += "?branch=" + branchID
	}
	rec := do(t, server, http.MethodGet, path, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200. Body: %s", path, rec.Code, rec.Body.String())
	}
	children, _ := decodeJSON(t, rec)["children"].([]any)
	ids := make([]string, 0, len(children))
	for _, c := range children {
		id, _ := c.(map[string]any)["person_id"].(string)
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// TestBranchChildLinks_SecondLink pins #990: a second, different child links to
// the same family on a branch, and the branch's family read lists both. A
// branch-only link then unlinks on the branch.
func TestBranchChildLinks_SecondLink(t *testing.T) {
	for _, backend := range branchChildLinkBackends {
		t.Run(backend.name, func(t *testing.T) {
			server, _ := backend.setup(t)
			familyID := createFamily(t, server,
				createPerson(t, server, "John", "Smith"), createPerson(t, server, "Mary", "Jones"))
			child1 := createPerson(t, server, "Kid", "One")
			child2 := createPerson(t, server, "Kid", "Two")
			branchID := createBranch(t, server, "two children")

			for _, child := range []string{child1, child2} {
				rec := do(t, server, http.MethodPost,
					fmt.Sprintf("/api/v1/families/%s/children?branch=%s", familyID, branchID),
					fmt.Sprintf(`{"person_id":%q}`, child))
				if rec.Code != http.StatusCreated {
					t.Fatalf("Branch link of %s: status = %d, want 201. Body: %s", child, rec.Code, rec.Body.String())
				}
			}

			want := []string{child1, child2}
			slices.Sort(want)
			if got := familyChildIDs(t, server, familyID, branchID); !slices.Equal(got, want) {
				t.Errorf("Branch family children = %v, want %v", got, want)
			}
			if got := familyChildIDs(t, server, familyID, ""); len(got) != 0 {
				t.Errorf("Main family children = %v, want none - the branch links leaked", got)
			}

			// A branch-only link unlinks on the branch, at the version the links left.
			rec := do(t, server, http.MethodDelete,
				fmt.Sprintf("/api/v1/families/%s/children/%s?branch=%s", familyID, child1, branchID), "")
			if rec.Code != http.StatusNoContent {
				t.Fatalf("Branch unlink: status = %d, want 204. Body: %s", rec.Code, rec.Body.String())
			}
			if got := familyChildIDs(t, server, familyID, branchID); !slices.Equal(got, []string{child2}) {
				t.Errorf("Branch family children after unlink = %v, want [%s]", got, child2)
			}
		})
	}
}

// TestBranchChildLinks_StaleVersionIs409 pins that branch-scoped reads did not
// switch off optimistic locking: a family version that really is stale on the
// branch still refuses the link with 409 VERSION_CONFLICT, recording nothing.
func TestBranchChildLinks_StaleVersionIs409(t *testing.T) {
	for _, backend := range branchChildLinkBackends {
		t.Run(backend.name, func(t *testing.T) {
			server, events := backend.setup(t)
			familyID := createFamily(t, server,
				createPerson(t, server, "John", "Smith"), createPerson(t, server, "Mary", "Jones"))
			child1 := createPerson(t, server, "Kid", "One")
			child2 := createPerson(t, server, "Kid", "Two")
			branchID := createBranch(t, server, "rival edit")
			childrenPath := fmt.Sprintf("/api/v1/families/%s/children?branch=%s", familyID, branchID)

			if rec := do(t, server, http.MethodPost, childrenPath,
				fmt.Sprintf(`{"person_id":%q}`, child1)); rec.Code != http.StatusCreated {
				t.Fatalf("First branch link: status = %d, want 201. Body: %s", rec.Code, rec.Body.String())
			}

			events.rival = true
			rec := do(t, server, http.MethodPost, childrenPath, fmt.Sprintf(`{"person_id":%q}`, child2))
			if events.rival {
				t.Fatal("rival write never fired - the link did not reach a branch append")
			}
			if rec.Code != http.StatusConflict {
				t.Fatalf("Stale branch link: status = %d, want 409. Body: %s", rec.Code, rec.Body.String())
			}
			if code := decodeJSON(t, rec)["code"]; code != api.CodeVersionConflict {
				t.Errorf("code = %v, want %s", code, api.CodeVersionConflict)
			}
			if got := familyChildIDs(t, server, familyID, branchID); !slices.Equal(got, []string{child1}) {
				t.Errorf("Branch family children after refused link = %v, want [%s]", got, child1)
			}
		})
	}
}
