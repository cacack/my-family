package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func setupSettingsServer(settings repository.SettingsStore) *api.Server {
	eventStore := memory.NewEventStore()
	return api.NewServer(&config.Config{Port: 8080, LogFormat: "text", AuthMode: config.AuthModeOpen}, eventStore,
		memory.NewReadModelStore(), memory.NewSnapshotStore(eventStore), nil, api.WithSettingsStore(settings))
}

func homePersonRequest(t *testing.T, server *api.Server, method, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, "/api/v1/settings/home-person", strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	var resp map[string]any
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	}
	return rec.Code, resp
}

func homePersonID(resp map[string]any) any {
	person, _ := resp["person"].(map[string]any)
	return person["id"]
}

func TestHomePerson_SetGetClear(t *testing.T) {
	server := setupSettingsServer(memory.NewSettingsStore())
	personID := createTestPerson(t, server, "Ada", "Home")["id"].(string)

	if code, resp := homePersonRequest(t, server, http.MethodGet, ""); code != http.StatusOK || resp["person"] != nil {
		t.Fatalf("GET unset = %d %v, want 200 with no person", code, resp)
	}

	code, resp := homePersonRequest(t, server, http.MethodPut, `{"person_id":"`+personID+`"}`)
	if code != http.StatusOK || homePersonID(resp) != personID {
		t.Fatalf("PUT = %d %v, want 200 naming %s", code, resp, personID)
	}
	if person, _ := resp["person"].(map[string]any); person["given_name"] != "Ada" {
		t.Errorf("PUT person = %v, want Ada", person)
	}

	if code, resp := homePersonRequest(t, server, http.MethodGet, ""); code != http.StatusOK || homePersonID(resp) != personID {
		t.Fatalf("GET = %d %v, want %s", code, resp, personID)
	}

	if code, _ := homePersonRequest(t, server, http.MethodDelete, ""); code != http.StatusNoContent {
		t.Fatalf("DELETE = %d, want 204", code)
	}
	if code, resp := homePersonRequest(t, server, http.MethodGet, ""); code != http.StatusOK || resp["person"] != nil {
		t.Errorf("GET after clear = %d %v, want no person", code, resp)
	}
	// Clearing again is not an error.
	if code, _ := homePersonRequest(t, server, http.MethodDelete, ""); code != http.StatusNoContent {
		t.Errorf("second DELETE = %d, want 204", code)
	}
}

func TestHomePerson_SetRejected(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"unknown person", `{"person_id":"` + uuid.NewString() + `"}`, http.StatusNotFound},
		{"nil id", `{"person_id":"` + uuid.Nil.String() + `"}`, http.StatusBadRequest},
		{"missing id", `{}`, http.StatusBadRequest},
		{"not a uuid", `{"person_id":"me"}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := memory.NewSettingsStore()
			server := setupSettingsServer(settings)
			if code, resp := homePersonRequest(t, server, http.MethodPut, tt.body); code != tt.want {
				t.Errorf("PUT %s = %d %v, want %d", tt.body, code, resp, tt.want)
			}
			if id, _ := settings.HomePerson(context.Background(), repository.SiteWide); id != uuid.Nil {
				t.Errorf("rejected PUT saved %v", id)
			}
		})
	}
}

// A home person deleted from the tree reads as unset rather than as an error.
func TestHomePerson_DeletedPersonReadsAsUnset(t *testing.T) {
	settings := memory.NewSettingsStore()
	server := setupSettingsServer(settings)
	if err := settings.SetHomePerson(context.Background(), repository.SiteWide, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if code, resp := homePersonRequest(t, server, http.MethodGet, ""); code != http.StatusOK || resp["person"] != nil {
		t.Errorf("GET = %d %v, want 200 with no person", code, resp)
	}
}

func TestHomePerson_Unavailable(t *testing.T) {
	server := setupTestServer()
	for _, tc := range []struct{ method, body string }{
		{http.MethodGet, ""},
		{http.MethodPut, `{"person_id":"` + uuid.NewString() + `"}`},
		{http.MethodDelete, ""},
	} {
		if code, resp := homePersonRequest(t, server, tc.method, tc.body); code != http.StatusServiceUnavailable || resp["code"] != "settings_unavailable" {
			t.Errorf("%s without a settings store = %d %v, want 503 settings_unavailable", tc.method, code, resp)
		}
	}
}

// failingSettings is a settings store whose every call fails.
type failingSettings struct{}

var errSettingsBroken = errors.New("settings table is locked")

func (failingSettings) HomePerson(context.Context, uuid.UUID) (uuid.UUID, error) {
	return uuid.Nil, errSettingsBroken
}

func (failingSettings) SetHomePerson(context.Context, uuid.UUID, uuid.UUID) error {
	return errSettingsBroken
}

func (failingSettings) SeedHomePerson(context.Context, uuid.UUID) error {
	return errSettingsBroken
}

func (failingSettings) MoveHomePersons(context.Context, map[uuid.UUID]uuid.UUID) error {
	return errSettingsBroken
}

func TestHomePerson_StoreErrors(t *testing.T) {
	server := setupSettingsServer(failingSettings{})
	personID := createTestPerson(t, server, "Ada", "Home")["id"].(string)
	for _, tc := range []struct{ method, body string }{
		{http.MethodGet, ""},
		{http.MethodPut, `{"person_id":"` + personID + `"}`},
		{http.MethodDelete, ""},
	} {
		if code, _ := homePersonRequest(t, server, tc.method, tc.body); code != http.StatusInternalServerError {
			t.Errorf("%s with a failing store = %d, want 500", tc.method, code)
		}
	}
}

func setupMergeSettingsServer(settings repository.SettingsStore) *api.Server {
	eventStore := memory.NewEventStore()
	return api.NewServer(&config.Config{Port: 8080, LogFormat: "text", AuthMode: config.AuthModeOpen}, eventStore,
		memory.NewReadModelStore(), memory.NewSnapshotStore(eventStore), nil,
		api.WithBranchStore(memory.NewBranchStore()), api.WithSettingsStore(settings))
}

// Merging the home person into a duplicate on the mainline moves "me" to the
// survivor; a branch-scoped merge leaves main, and so the setting, alone.
func TestHomePerson_FollowsMerge(t *testing.T) {
	tests := []struct {
		name     string
		batch    bool
		onBranch bool
		wantMove bool
	}{
		{"single merge", false, false, true},
		{"batch merge", true, false, true},
		{"branch merge", false, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			settings := memory.NewSettingsStore()
			server := setupMergeSettingsServer(settings)
			survivorID, sv := createMergeTestPerson(t, server, "Morgan", "Duplicate")
			mergedID, mv := createMergeTestPerson(t, server, "Morgan", "Duplicat")
			if err := settings.SetHomePerson(ctx, repository.SiteWide, uuid.MustParse(mergedID)); err != nil {
				t.Fatal(err)
			}

			merge := fmt.Sprintf(`{"survivor_id":%q,"merged_id":%q,"survivor_version":%d,"merged_version":%d}`, survivorID, mergedID, sv, mv)
			path, body := "/api/v1/persons/merge", merge
			if tt.batch {
				path, body = "/api/v1/persons/merge/batch", `{"merges":[`+merge+`]}`
			}
			if tt.onBranch {
				path += "?branch=" + createBranch(t, server, "same person")
			}
			if rec := do(t, server, http.MethodPost, path, body); rec.Code != http.StatusOK {
				t.Fatalf("merge = %d. Body: %s", rec.Code, rec.Body.String())
			}

			want := mergedID
			if tt.wantMove {
				want = survivorID
			}
			if got, _ := settings.HomePerson(ctx, repository.SiteWide); got.String() != want {
				t.Errorf("home person after merge = %s, want %s", got, want)
			}
		})
	}
}

// A settings failure after a merge has landed does not fail the merge.
func TestHomePerson_MergeSurvivesSettingsFailure(t *testing.T) {
	server := setupMergeSettingsServer(failingSettings{})
	survivorID, sv := createMergeTestPerson(t, server, "Morgan", "Duplicate")
	mergedID, mv := createMergeTestPerson(t, server, "Morgan", "Duplicat")
	body := fmt.Sprintf(`{"survivor_id":%q,"merged_id":%q,"survivor_version":%d,"merged_version":%d}`, survivorID, mergedID, sv, mv)
	if rec := do(t, server, http.MethodPost, "/api/v1/persons/merge", body); rec.Code != http.StatusOK {
		t.Errorf("merge with a failing settings store = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
}

// branchPersonMerge merges mergedID into survivorID on a branch and returns the
// survivor's version after it.
func branchPersonMerge(t *testing.T, server *api.Server, branchID, survivorID, mergedID string, sv, mv int64) int64 {
	t.Helper()
	body := fmt.Sprintf(`{"survivor_id":%q,"merged_id":%q,"survivor_version":%d,"merged_version":%d}`, survivorID, mergedID, sv, mv)
	rec := do(t, server, http.MethodPost, "/api/v1/persons/merge?branch="+branchID, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("branch person merge = %d. Body: %s", rec.Code, rec.Body.String())
	}
	person, _ := decodeJSON(t, rec)["person"].(map[string]any)
	return int64(person["version"].(float64))
}

// A person merge made on a research branch reaches main when the branch is
// merged (#1444): "me" follows it there, through a chain of merges, and a
// branch merge that does not merge "me" away leaves the setting alone.
func TestHomePerson_FollowsBranchMerge(t *testing.T) {
	tests := []struct {
		name string
		// setup creates its persons on main, forks a branch after them so main
		// does not change under it, and returns the branch, the home person and
		// the one GET /settings/home-person should name after the branch merge.
		setup func(t *testing.T, server *api.Server) (branchID, home, want string)
	}{
		{"home person merged away", func(t *testing.T, server *api.Server) (string, string, string) {
			survivorID, sv := createMergeTestPerson(t, server, "Morgan", "Duplicate")
			mergedID, mv := createMergeTestPerson(t, server, "Morgan", "Duplicat")
			branchID := createBranch(t, server, "same person")
			branchPersonMerge(t, server, branchID, survivorID, mergedID, sv, mv)
			return branchID, mergedID, survivorID
		}},
		{"home person merged away through a chain", func(t *testing.T, server *api.Server) (string, string, string) {
			firstID, fv := createMergeTestPerson(t, server, "Morgan", "Duplicat")
			middleID, mv := createMergeTestPerson(t, server, "Morgan", "Duplicate")
			lastID, lv := createMergeTestPerson(t, server, "Morgan", "Duplicates")
			branchID := createBranch(t, server, "same person")
			mv = branchPersonMerge(t, server, branchID, middleID, firstID, mv, fv)
			branchPersonMerge(t, server, branchID, lastID, middleID, lv, mv)
			return branchID, firstID, lastID
		}},
		{"home person is the survivor", func(t *testing.T, server *api.Server) (string, string, string) {
			survivorID, sv := createMergeTestPerson(t, server, "Morgan", "Duplicate")
			mergedID, mv := createMergeTestPerson(t, server, "Morgan", "Duplicat")
			branchID := createBranch(t, server, "same person")
			branchPersonMerge(t, server, branchID, survivorID, mergedID, sv, mv)
			return branchID, survivorID, survivorID
		}},
		{"home person not in the merge", func(t *testing.T, server *api.Server) (string, string, string) {
			homeID, _ := createMergeTestPerson(t, server, "Ada", "Home")
			survivorID, sv := createMergeTestPerson(t, server, "Morgan", "Duplicate")
			mergedID, mv := createMergeTestPerson(t, server, "Morgan", "Duplicat")
			branchID := createBranch(t, server, "same person")
			branchPersonMerge(t, server, branchID, survivorID, mergedID, sv, mv)
			return branchID, homeID, homeID
		}},
		{"branch without a person merge", func(t *testing.T, server *api.Server) (string, string, string) {
			homeID, _ := createMergeTestPerson(t, server, "Ada", "Home")
			branchID := createBranch(t, server, "same person")
			if rec := do(t, server, http.MethodPost, "/api/v1/persons?branch="+branchID, `{"given_name":"New","surname":"Cousin"}`); rec.Code != http.StatusCreated {
				t.Fatalf("branch create = %d. Body: %s", rec.Code, rec.Body.String())
			}
			return branchID, homeID, homeID
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := setupMergeSettingsServer(memory.NewSettingsStore())
			branchID, home, want := tt.setup(t, server)
			if code, resp := homePersonRequest(t, server, http.MethodPut, `{"person_id":"`+home+`"}`); code != http.StatusOK {
				t.Fatalf("PUT home person = %d %v", code, resp)
			}

			if rec := do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge", `{}`); rec.Code != http.StatusOK {
				t.Fatalf("branch merge = %d. Body: %s", rec.Code, rec.Body.String())
			}

			if code, resp := homePersonRequest(t, server, http.MethodGet, ""); code != http.StatusOK || homePersonID(resp) != want {
				t.Errorf("GET home person after branch merge = %d %v, want %s", code, resp, want)
			}
		})
	}
}

// A settings failure after a branch merge has landed does not fail the merge.
func TestHomePerson_BranchMergeSurvivesSettingsFailure(t *testing.T) {
	server := setupMergeSettingsServer(failingSettings{})
	survivorID, sv := createMergeTestPerson(t, server, "Morgan", "Duplicate")
	mergedID, mv := createMergeTestPerson(t, server, "Morgan", "Duplicat")
	branchID := createBranch(t, server, "same person")
	branchPersonMerge(t, server, branchID, survivorID, mergedID, sv, mv)
	if rec := do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge", `{}`); rec.Code != http.StatusOK {
		t.Errorf("branch merge with a failing settings store = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
}

// A branch merge whose replay fails part-way is finished by a resume, and "me"
// follows the person merge then (#1444).
func TestHomePerson_FollowsResumedBranchMerge(t *testing.T) {
	base := memory.NewEventStore()
	eventStore := &switchableReplayEventStore{EventStore: base}
	server := api.NewServer(&config.Config{Port: 8080, LogFormat: "text", AuthMode: config.AuthModeOpen}, eventStore,
		memory.NewReadModelStore(), memory.NewSnapshotStore(base), nil,
		api.WithBranchStore(memory.NewBranchStore()), api.WithSettingsStore(memory.NewSettingsStore()))
	survivorID, sv := createMergeTestPerson(t, server, "Morgan", "Duplicate")
	mergedID, mv := createMergeTestPerson(t, server, "Morgan", "Duplicat")
	branchID := createBranch(t, server, "same person")
	branchPersonMerge(t, server, branchID, survivorID, mergedID, sv, mv)
	if code, resp := homePersonRequest(t, server, http.MethodPut, `{"person_id":"`+mergedID+`"}`); code != http.StatusOK {
		t.Fatalf("PUT home person = %d %v", code, resp)
	}

	eventStore.failing = true
	if rec := do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge", `{}`); rec.Code != http.StatusInternalServerError {
		t.Fatalf("failing branch merge = %d, want 500. Body: %s", rec.Code, rec.Body.String())
	}
	eventStore.failing = false
	if rec := do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge/resume", ""); rec.Code != http.StatusOK {
		t.Fatalf("resume = %d. Body: %s", rec.Code, rec.Body.String())
	}

	if code, resp := homePersonRequest(t, server, http.MethodGet, ""); code != http.StatusOK || homePersonID(resp) != survivorID {
		t.Errorf("GET home person after resume = %d %v, want %s", code, resp, survivorID)
	}
}
