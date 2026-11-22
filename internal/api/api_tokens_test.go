package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// createToken has the user signed in with cookie create an API token named
// name and returns it.
func (f *authFixture) createToken(t *testing.T, cookie *http.Cookie, name string) api.CreatedAPIToken {
	t.Helper()
	rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/me/tokens", cookie: cookie, body: `{"name":"` + name + `"}`})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create token: status %d, body %s", rec.Code, rec.Body)
	}
	return decode[api.CreatedAPIToken](t, rec)
}

// bearer is the header a script sends its API token in.
func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func TestAPITokens_CreateListRevoke(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	owner := f.signIn(t)

	created := f.createToken(t, owner, "  nightly backup ")
	if created.Name != "nightly backup" || created.Token == "" {
		t.Fatalf("created token = %+v, want the trimmed name and a token", created)
	}
	if !strings.HasPrefix(created.Token, "mf_") {
		t.Errorf("token %q lacks the mf_ prefix", created.Token)
	}
	second := f.createToken(t, owner, "import script")

	rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/auth/me/tokens", cookie: owner})
	if rec.Code != http.StatusOK {
		t.Fatalf("list tokens: status %d, body %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), created.Token) || strings.Contains(rec.Body.String(), second.Token) {
		t.Errorf("the token list carries a token's secret: %s", rec.Body)
	}
	list := decode[api.APITokenList](t, rec).Items
	if len(list) != 2 || list[0].Id != second.Id || list[1].Id != created.Id {
		t.Fatalf("token list = %+v, want the two tokens, newest first", list)
	}
	if list[1].LastUsedAt != nil {
		t.Errorf("an unused token has last_used_at %v", list[1].LastUsedAt)
	}

	// Using the token records when it was last used.
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/persons", header: bearer(created.Token)}); rec.Code != http.StatusOK {
		t.Fatalf("read with the token: status %d, body %s", rec.Code, rec.Body)
	}
	list = decode[api.APITokenList](t, f.do(t, request{method: http.MethodGet, path: "/api/v1/auth/me/tokens", cookie: owner})).Items
	if list[1].LastUsedAt == nil {
		t.Error("after a request with the token, last_used_at is absent")
	}

	// Another user can neither see nor revoke the owner's tokens.
	grace := f.inviteAs(t, owner, "editor", "grace")
	if got := decode[api.APITokenList](t, f.do(t, request{method: http.MethodGet, path: "/api/v1/auth/me/tokens", cookie: grace})).Items; len(got) != 0 {
		t.Errorf("another user's token list = %+v, want empty", got)
	}
	if rec := f.do(t, request{method: http.MethodDelete, path: "/api/v1/auth/me/tokens/" + created.Id.String(), cookie: grace}); rec.Code != http.StatusNotFound {
		t.Errorf("revoke of another user's token: status %d, want 404", rec.Code)
	}

	if rec := f.do(t, request{method: http.MethodDelete, path: "/api/v1/auth/me/tokens/" + created.Id.String(), cookie: owner}); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke token: status %d, body %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, request{method: http.MethodDelete, path: "/api/v1/auth/me/tokens/" + created.Id.String(), cookie: owner}); rec.Code != http.StatusNotFound {
		t.Errorf("revoke twice: status %d, want 404", rec.Code)
	}
	list = decode[api.APITokenList](t, f.do(t, request{method: http.MethodGet, path: "/api/v1/auth/me/tokens", cookie: owner})).Items
	if len(list) != 1 || list[0].Id != second.Id {
		t.Errorf("after revoking, token list = %+v, want only %s", list, second.Id)
	}

	for _, name := range []string{"", "   ", strings.Repeat("x", 101)} {
		rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/me/tokens", cookie: owner, body: `{"name":"` + name + `"}`})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("create token named %q: status %d, want 400", name, rec.Code)
		}
	}
}

// A token acts as its user with the role they hold at each request.
func TestAPITokens_ActAsTheirUserWithTheirCurrentRole(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	owner := f.signIn(t)
	editor := f.createToken(t, f.inviteAs(t, owner, "editor", "grace"), "editor script")
	viewer := f.createToken(t, f.inviteAs(t, owner, "viewer", "alan"), "viewer script")

	write := request{method: http.MethodPost, path: "/api/v1/persons", body: `{"given_name":"Charles","surname":"Babbage"}`}
	write.header = bearer(editor.Token)
	if rec := f.do(t, write); rec.Code != http.StatusCreated {
		t.Errorf("editor token write: status %d, body %s; want 201", rec.Code, rec.Body)
	}
	write.header = bearer(viewer.Token)
	if rec := f.do(t, write); !forbidden(rec) {
		t.Errorf("viewer token write: status %d, body %s; want 403 FORBIDDEN", rec.Code, rec.Body)
	}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/persons", header: bearer(viewer.Token)}); rec.Code != http.StatusOK {
		t.Errorf("viewer token read: status %d, body %s; want 200", rec.Code, rec.Body)
	}

	// Demoting the editor narrows their token at once.
	grace := f.users(t, owner)["grace"]
	if rec := f.do(t, request{method: http.MethodPatch, path: "/api/v1/users/" + grace.Id.String(), cookie: owner, body: `{"role":"viewer"}`}); rec.Code != http.StatusOK {
		t.Fatalf("demote grace: status %d, body %s", rec.Code, rec.Body)
	}
	write.header = bearer(editor.Token)
	if rec := f.do(t, write); !forbidden(rec) {
		t.Errorf("demoted user's token write: status %d, body %s; want 403 FORBIDDEN", rec.Code, rec.Body)
	}
}

// A revoked token, a token whose user is disabled, and one never issued get
// 401, even where visitors are let in: a script never silently becomes a
// visitor.
func TestAPITokens_RefusedWhenRevokedOrUserDisabled(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	owner := f.signIn(t)
	f.openToVisitors(t)
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/persons"}); rec.Code != http.StatusOK {
		t.Fatalf("visitors are not let in: status %d, body %s", rec.Code, rec.Body)
	}
	revoked := f.createToken(t, owner, "revoked")
	graceToken := f.createToken(t, f.inviteAs(t, owner, "editor", "grace"), "grace's script")

	read := func(token string) *request {
		return &request{method: http.MethodGet, path: "/api/v1/persons", header: bearer(token)}
	}
	for name, token := range map[string]string{"revoked": revoked.Token, "grace's": graceToken.Token} {
		if rec := f.do(t, *read(token)); rec.Code != http.StatusOK {
			t.Fatalf("%s token before: status %d, body %s", name, rec.Code, rec.Body)
		}
	}

	if rec := f.do(t, request{method: http.MethodDelete, path: "/api/v1/auth/me/tokens/" + revoked.Id.String(), cookie: owner}); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke: status %d, body %s", rec.Code, rec.Body)
	}
	grace := f.users(t, owner)["grace"]
	if rec := f.do(t, request{method: http.MethodPatch, path: "/api/v1/users/" + grace.Id.String(), cookie: owner, body: `{"disabled":true}`}); rec.Code != http.StatusOK {
		t.Fatalf("disable grace: status %d, body %s", rec.Code, rec.Body)
	}

	for name, token := range map[string]string{
		"revoked": revoked.Token, "disabled user's": graceToken.Token,
		"never issued": "mf_not-a-token", "unprefixed": "not-a-token", "empty": "",
	} {
		if rec := f.do(t, *read(token)); !unauthenticated(rec) {
			t.Errorf("%s token: status %d, body %s; want 401 UNAUTHENTICATED", name, rec.Code, rec.Body)
		}
	}

	// Re-enabling the user restores their token: disabling refuses it, it
	// does not revoke it.
	if rec := f.do(t, request{method: http.MethodPatch, path: "/api/v1/users/" + grace.Id.String(), cookie: owner, body: `{"disabled":false}`}); rec.Code != http.StatusOK {
		t.Fatalf("enable grace: status %d, body %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, *read(graceToken.Token)); rec.Code != http.StatusOK {
		t.Errorf("re-enabled user's token: status %d, body %s; want 200", rec.Code, rec.Body)
	}
}

// A change made with a token names its user in the history, and creating
// and revoking a token are audited.
func TestAPITokens_AttributedAndAudited(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	owner := f.signIn(t)
	created := f.createToken(t, owner, "nightly backup")

	rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/persons", header: bearer(created.Token), body: `{"given_name":"Charles"}`})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create person with the token: %d %s", rec.Code, rec.Body)
	}
	me := decode[api.Account](t, f.do(t, request{method: http.MethodGet, path: "/api/v1/auth/me", cookie: owner}))
	history := decode[api.ChangeHistoryResponse](t, f.do(t, request{method: http.MethodGet, path: "/api/v1/history", cookie: owner}))
	if len(history.Items) == 0 {
		t.Fatal("no history entries")
	}
	if entry := history.Items[0]; entry.UserId == nil || *entry.UserId != me.Id.String() {
		t.Errorf("history user_id = %v, want the token's user %s", entry.UserId, me.Id)
	}

	if rec := f.do(t, request{method: http.MethodDelete, path: "/api/v1/auth/me/tokens/" + created.Id.String(), cookie: owner}); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke: status %d, body %s", rec.Code, rec.Body)
	}
	entries, err := f.accounts.ListAuthAudit(context.Background(), 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := map[repository.AuthAction]repository.AuthAuditEntry{}
	for _, e := range entries {
		got[e.Action] = e
	}
	for _, action := range []repository.AuthAction{repository.AuthActionAPITokenCreated, repository.AuthActionAPITokenRevoked} {
		e, ok := got[action]
		if !ok {
			t.Errorf("no %s audit entry in %+v", action, entries)
			continue
		}
		if e.ActorID != me.Id || e.SubjectID != me.Id || e.Detail != "nightly backup" || e.Username != "ada" {
			t.Errorf("%s audit entry = %+v, want actor and subject %s, detail the token's name", action, e, me.Id)
		}
		if strings.Contains(e.Detail, created.Token) {
			t.Errorf("%s audit entry carries the token", action)
		}
	}
}

// A token cannot manage tokens, so a leaked one cannot outlive its own
// revocation, and never previews the tree as someone else.
func TestAPITokens_SessionOnlyAndNoPreview(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	owner := f.signIn(t)
	created := f.createToken(t, owner, "script")
	header := bearer(created.Token)

	for _, r := range []request{
		{method: http.MethodGet, path: "/api/v1/auth/me/tokens"},
		{method: http.MethodPost, path: "/api/v1/auth/me/tokens", body: `{"name":"another"}`},
		{method: http.MethodDelete, path: "/api/v1/auth/me/tokens/" + created.Id.String()},
	} {
		r.header = header
		if rec := f.do(t, r); !forbidden(rec) {
			t.Errorf("%s %s with a token: status %d, body %s; want 403 FORBIDDEN", r.method, r.path, rec.Code, rec.Body)
		}
	}

	// The owner's preview cookie, sent along, narrows nothing: a visitor
	// preview would refuse the change log.
	preview := &http.Cookie{Name: "myfamily_preview", Value: "visitor"}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/history", header: header, cookie: preview}); rec.Code != http.StatusOK {
		t.Errorf("history with a token and a preview cookie: status %d, body %s; want 200", rec.Code, rec.Body)
	}
	// A token wins over a session cookie sent with it.
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/auth/me/tokens", header: header, cookie: owner}); !forbidden(rec) {
		t.Errorf("token management with a token and a session: status %d; want 403", rec.Code)
	}
}

// In open mode there are no accounts: token management is refused and a
// bearer header changes nothing.
func TestAPITokens_OpenMode(t *testing.T) {
	events := memory.NewEventStore()
	server := api.NewServer(&config.Config{AuthMode: config.AuthModeOpen}, events, memory.NewReadModelStore(), memory.NewSnapshotStore(events), nil)
	f := &authFixture{server: server, events: events}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/auth/me/tokens"}); rec.Code != http.StatusConflict {
		t.Errorf("list tokens in open mode: status %d, want 409", rec.Code)
	}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/persons", header: bearer("mf_anything")}); rec.Code != http.StatusOK {
		t.Errorf("read with a bearer header in open mode: status %d, want 200", rec.Code)
	}
}
