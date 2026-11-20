package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

// The event-output redactor (ADR-008 §5b, #1515) on every backend: for a
// viewer who may not see living people, snapshot comparison, branch
// comparison, the merge review (conflicts and merge record) and restore
// points leave out a living person's changes, while a dead person's stay.
// The data is written through the API as the owner and read back through the
// query services over the privacy decorator, as the server wires them.
func TestPrivacy_EventLogSurfacesOnEveryBackend(t *testing.T) {
	settings := repository.DefaultTreeSettings()
	settings.ViewersSeeLiving = false
	strictViewer := repository.ContextWithTreeSettings(
		repository.ContextWithPrincipal(context.Background(), repository.Principal{Role: repository.RoleViewer}), settings)
	editor := repository.ContextWithPrincipal(context.Background(), repository.Principal{Role: repository.RoleEditor})

	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			st := backend.setup(t)
			server := newServer(t, st)

			dead := createPerson(t, server, "Dora", "Open")
			deadPath := "/api/v1/persons/" + dead
			mustDo(t, server, http.MethodPut, deadPath,
				fmt.Sprintf(`{"death_date":"1 JAN 1900","version":%d}`, entityVersion(t, server, deadPath, "")), http.StatusOK)
			living := createPerson(t, server, "Lena", "Hidden")
			livingPath := "/api/v1/persons/" + living
			snapshot := mustString(t, mustDo(t, server, http.MethodPost, "/api/v1/snapshots", `{"name":"before"}`, http.StatusCreated), "id")

			branch := createBranch(t, server, "redaction")
			updateSurname(t, server, deadPath, branch, "Branchopen")
			updateSurname(t, server, livingPath, branch, "Branchhidden")
			updateSurname(t, server, deadPath, "", "Mainopen")
			updateSurname(t, server, livingPath, "", "Mainhidden")

			privacy := query.NewPrivacyStore(st.read)
			history := query.NewHistoryService(st.events, privacy)
			history.UseBranchStore(st.branches)
			snapshots := query.NewSnapshotService(st.snapshots, st.events, history)
			branches := query.NewBranchService(st.branches, st.events, history)
			rollback := query.NewRollbackService(st.events, privacy)
			snapshotID, branchID := uuid.MustParse(snapshot), uuid.MustParse(branch)

			// shows reports whether v, marshalled, mentions the dead person
			// and the living one.
			shows := func(t *testing.T, v any) (deadShown, livingShown bool) {
				t.Helper()
				raw, err := json.Marshal(v)
				if err != nil {
					t.Fatal(err)
				}
				text := strings.ToLower(string(raw))
				return strings.Contains(text, "open"), strings.Contains(text, "hidden") || strings.Contains(text, "lena")
			}
			surfaces := func(t *testing.T, ctx context.Context) map[string]any {
				t.Helper()
				comparison, err := snapshots.CompareSnapshotToCurrent(ctx, domain.MainBranchID, snapshotID)
				if err != nil {
					t.Fatal(err)
				}
				compare, err := branches.CompareBranch(ctx, branchID)
				if err != nil {
					t.Fatal(err)
				}
				return map[string]any{
					"snapshot comparison":     comparison.Changes,
					"branch changes":          compare.BranchChanges,
					"main changes":            compare.MainChanges,
					"merge plan (conflicts)":  compare.Conflicts,
					"restore points (person)": restorePoints(ctx, t, rollback, dead, living),
				}
			}

			for name, got := range surfaces(t, strictViewer) {
				if deadShown, livingShown := shows(t, got); !deadShown || livingShown {
					t.Errorf("viewer without living: %s shows the dead person %v and the living one %v; want only the dead: %+v",
						name, deadShown, livingShown, got)
				}
			}
			for name, got := range surfaces(t, editor) {
				if deadShown, livingShown := shows(t, got); !deadShown || !livingShown {
					t.Errorf("editor: %s shows the dead person %v and the living one %v; want both", name, deadShown, livingShown)
				}
			}

			// After the merge, the record of its decisions is redacted too.
			mustDo(t, server, http.MethodPost, mergePath(branch),
				mergeBody("settled", resolution(dead, "branch"), resolution(living, "branch")), http.StatusOK)
			for ctx, want := range map[context.Context][]string{strictViewer: {dead}, editor: {dead, living}} {
				compare, err := branches.CompareBranch(ctx, branchID)
				if err != nil {
					t.Fatal(err)
				}
				if compare.MergeRecord == nil {
					t.Fatal("merged branch has no merge record")
				}
				var decided []string
				for _, d := range compare.MergeRecord.Decisions {
					decided = append(decided, d.StreamID.String())
				}
				slices.Sort(decided)
				slices.Sort(want)
				if !slices.Equal(decided, want) {
					t.Errorf("merge record decisions = %v, want %v", decided, want)
				}
			}
		})
	}
}

// restorePoints reads both people's restore points: the summaries of each,
// keyed so the dead person's read "open" and the living one's "hidden".
func restorePoints(ctx context.Context, t *testing.T, rollback *query.RollbackService, dead, living string) map[string]int {
	t.Helper()
	out := map[string]int{}
	for key, id := range map[string]string{"open": dead, "hidden": living} {
		result, err := rollback.GetRestorePoints(ctx, "Person", uuid.MustParse(id), 20, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.RestorePoints) > 0 {
			out[key] = len(result.RestorePoints)
		}
	}
	return out
}
