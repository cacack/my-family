package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/auth"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// inviteAs has the owner (cookie) invite someone with role, accepts the
// invite as username and returns the new account's session cookie.
func (f *authFixture) inviteAs(t *testing.T, owner *http.Cookie, role, username string) *http.Cookie {
	t.Helper()
	rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/invites", body: `{"role":"` + role + `"}`, cookie: owner})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create invite: status %d, body %s", rec.Code, rec.Body)
	}
	link := decode[api.AccountLink](t, rec)
	rec = f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/invites/accept",
		body: `{"token":"` + link.Token + `","username":"` + username + `","display_name":"` + username + `","password":"` + ownerPassword + `"}`})
	if rec.Code != http.StatusCreated {
		t.Fatalf("accept invite: status %d, body %s", rec.Code, rec.Body)
	}
	if account := decode[api.Account](t, rec); string(account.Role) != role {
		t.Fatalf("accepted account role = %s, want %s", account.Role, role)
	}
	return sessionCookieOf(t, rec)
}

func (f *authFixture) users(t *testing.T, owner *http.Cookie) map[string]api.User {
	t.Helper()
	rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/users", cookie: owner})
	if rec.Code != http.StatusOK {
		t.Fatalf("list users: status %d, body %s", rec.Code, rec.Body)
	}
	users := map[string]api.User{}
	for _, u := range decode[api.UserList](t, rec).Items {
		users[u.Username] = u
	}
	return users
}

func TestUsers_InviteAcceptAndManage(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	owner := f.signIn(t)
	grace := f.inviteAs(t, owner, "editor", "grace")

	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/auth/me", cookie: grace}); rec.Code != http.StatusOK {
		t.Fatalf("the invited user is not signed in: %d", rec.Code)
	}
	users := f.users(t, owner)
	if len(users) != 2 || users["grace"].Role != "editor" || users["ada"].Role != "owner" || users["grace"].Disabled {
		t.Fatalf("users = %+v", users)
	}
	graceID := users["grace"].Id.String()
	adaID := users["ada"].Id.String()

	patch := func(id, body string) (int, string) {
		rec := f.do(t, request{method: http.MethodPatch, path: "/api/v1/users/" + id, body: body, cookie: owner})
		code := ""
		if rec.Code != http.StatusOK {
			code = errorCode(t, rec)
		}
		return rec.Code, code
	}
	for _, tc := range []struct {
		name, id, body string
		status         int
		code           string
	}{
		{"demote the last owner", adaID, `{"role":"viewer"}`, http.StatusConflict, api.CodeLastOwner},
		{"disable the last owner", adaID, `{"disabled":true}`, http.StatusConflict, api.CodeLastOwner},
		{"an unknown role", graceID, `{"role":"admin"}`, http.StatusBadRequest, ""},
		{"an unknown user", "00000000-0000-0000-0000-00000000abcd", `{"role":"viewer"}`, http.StatusNotFound, api.CodeNotFound},
		{"change nothing", graceID, `{}`, http.StatusOK, ""},
		{"make grace a viewer", graceID, `{"role":"viewer"}`, http.StatusOK, ""},
	} {
		status, code := patch(tc.id, tc.body)
		if status != tc.status || (tc.code != "" && code != tc.code) {
			t.Errorf("%s: %d %s, want %d %s", tc.name, status, code, tc.status, tc.code)
		}
	}
	if users := f.users(t, owner); users["grace"].Role != "viewer" {
		t.Errorf("grace's role = %s, want viewer", users["grace"].Role)
	}

	// Disabling ends grace's session and refuses her sign-in.
	if status, _ := patch(graceID, `{"disabled":true}`); status != http.StatusOK {
		t.Fatalf("disable: %d", status)
	}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/persons", cookie: grace}); rec.Code != http.StatusUnauthorized {
		t.Errorf("a disabled user's session: status %d, want 401", rec.Code)
	}
	rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/sign-in", body: `{"username":"grace","password":"` + ownerPassword + `"}`})
	if rec.Code != http.StatusForbidden || errorCode(t, rec) != api.CodeAccountDisabled {
		t.Errorf("sign-in of a disabled user: %d %s, want 403 %s", rec.Code, rec.Body, api.CodeAccountDisabled)
	}
	if status, _ := patch(graceID, `{"disabled":false}`); status != http.StatusOK {
		t.Fatalf("re-enable: %d", status)
	}
	grace = f.signInAs(t, "grace")

	// Only an owner manages users.
	for _, r := range []request{
		{method: http.MethodGet, path: "/api/v1/users"},
		{method: http.MethodPost, path: "/api/v1/invites", body: `{"role":"owner"}`},
		{method: http.MethodGet, path: "/api/v1/auth-audit"},
		{method: http.MethodPatch, path: "/api/v1/users/" + graceID, body: `{"role":"owner"}`},
	} {
		r.cookie = grace
		if rec := f.do(t, r); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as a viewer: status %d, want 403", r.method, r.path, rec.Code)
		}
	}
}

func TestUsers_NoSelfRegistration(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	for _, token := range []string{"", "made-up"} {
		rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/invites/accept",
			body: `{"token":"` + token + `","username":"eve","display_name":"Eve","password":"` + ownerPassword + `"}`})
		if rec.Code != http.StatusBadRequest || errorCode(t, rec) != api.CodeInvalidLink {
			t.Errorf("accept with token %q: %d %s, want 400 %s", token, rec.Code, rec.Body, api.CodeInvalidLink)
		}
	}
	for _, r := range []request{
		{method: http.MethodPost, path: "/api/v1/users", body: `{"username":"eve"}`},
		{method: http.MethodPost, path: "/api/v1/auth/register", body: `{"username":"eve"}`},
		{method: http.MethodPost, path: "/api/v1/auth/sign-up", body: `{"username":"eve"}`},
	} {
		if rec := f.do(t, r); rec.Code < 400 {
			t.Errorf("%s %s without a session: status %d, want a refusal", r.method, r.path, rec.Code)
		}
	}
	if users, _ := f.auth.Members(t.Context()); len(users) != 1 {
		t.Errorf("members = %d, want only the owner", len(users))
	}
}

func TestUsers_PasswordReset(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	owner := f.signIn(t)
	grace := f.inviteAs(t, owner, "viewer", "grace")
	graceID := f.users(t, owner)["grace"].Id.String()

	rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/users/" + graceID + "/password-reset", cookie: owner})
	if rec.Code != http.StatusCreated {
		t.Fatalf("issue reset: %d %s", rec.Code, rec.Body)
	}
	link := decode[api.AccountLink](t, rec)
	if rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/users/00000000-0000-0000-0000-00000000abcd/password-reset", cookie: owner}); rec.Code != http.StatusNotFound {
		t.Errorf("reset for an unknown user: status %d, want 404", rec.Code)
	}

	reset := func(token, password string) *httptest.ResponseRecorder {
		return f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/password-reset",
			body: `{"token":"` + token + `","password":"` + password + `"}`})
	}
	if rec := reset(link.Token, "short"); rec.Code != http.StatusBadRequest {
		t.Errorf("reset with a short password: status %d, want 400", rec.Code)
	}
	const newPassword = "a brand new password"
	if rec := reset(link.Token, newPassword); rec.Code != http.StatusNoContent {
		t.Fatalf("reset: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/auth/me", cookie: grace}); rec.Code != http.StatusUnauthorized {
		t.Errorf("session after a reset: status %d, want 401", rec.Code)
	}
	if rec := reset(link.Token, newPassword); rec.Code != http.StatusBadRequest || errorCode(t, rec) != api.CodeInvalidLink {
		t.Errorf("reset twice: %d %s, want 400 %s", rec.Code, rec.Body, api.CodeInvalidLink)
	}
	rec = f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/sign-in", body: `{"username":"grace","password":"` + newPassword + `"}`})
	if rec.Code != http.StatusOK {
		t.Errorf("sign-in with the new password: %d", rec.Code)
	}
}

func TestUsers_ListAndRevokePendingLinks(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	owner := f.signIn(t)
	f.inviteAs(t, owner, "viewer", "grace")
	graceID := f.users(t, owner)["grace"].Id

	rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/invites", body: `{"role":"editor"}`, cookie: owner})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create invite: %d %s", rec.Code, rec.Body)
	}
	invite := decode[api.AccountLink](t, rec)
	rec = f.do(t, request{method: http.MethodPost, path: "/api/v1/users/" + graceID.String() + "/password-reset", cookie: owner})
	if rec.Code != http.StatusCreated {
		t.Fatalf("issue reset: %d %s", rec.Code, rec.Body)
	}
	reset := decode[api.AccountLink](t, rec)

	// The lists name each link's issuer and user, and never its token.
	rec = f.do(t, request{method: http.MethodGet, path: "/api/v1/invites", cookie: owner})
	if rec.Code != http.StatusOK {
		t.Fatalf("list invites: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), invite.Token) {
		t.Errorf("the invite list shows the token: %s", rec.Body)
	}
	invites := decode[api.PendingInviteList](t, rec).Items
	if len(invites) != 1 || invites[0].Role != api.PendingInviteRoleEditor || invites[0].CreatedByName == nil ||
		*invites[0].CreatedByName != "Ada Lovelace" || !invites[0].ExpiresAt.Equal(invite.ExpiresAt) || invites[0].Id != invite.Id {
		t.Fatalf("pending invites = %+v, want Ada's editor invite (grace's was accepted)", invites)
	}
	rec = f.do(t, request{method: http.MethodGet, path: "/api/v1/password-resets", cookie: owner})
	if rec.Code != http.StatusOK {
		t.Fatalf("list resets: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), reset.Token) {
		t.Errorf("the reset list shows the token: %s", rec.Body)
	}
	resets := decode[api.PendingPasswordResetList](t, rec).Items
	if len(resets) != 1 || resets[0].UserId != graceID || resets[0].UserName == nil || *resets[0].UserName != "grace" ||
		resets[0].CreatedByName == nil || *resets[0].CreatedByName != "Ada Lovelace" || !resets[0].ExpiresAt.Equal(reset.ExpiresAt) ||
		resets[0].Id != reset.Id {
		t.Fatalf("pending resets = %+v, want grace's reset issued by Ada", resets)
	}

	// Editors and viewers may neither list nor revoke.
	for _, role := range []string{"editor", "viewer"} {
		cookie := f.inviteAs(t, owner, role, "a-"+role)
		for _, r := range []request{
			{method: http.MethodGet, path: "/api/v1/invites"},
			{method: http.MethodDelete, path: "/api/v1/invites/" + invites[0].Id},
			{method: http.MethodGet, path: "/api/v1/password-resets"},
			{method: http.MethodDelete, path: "/api/v1/password-resets/" + resets[0].Id},
		} {
			r.cookie = cookie
			if rec := f.do(t, r); rec.Code != http.StatusForbidden {
				t.Errorf("%s %s as %s: status %d, want 403", r.method, r.path, role, rec.Code)
			}
		}
	}

	// A revoked link fails as an expired one does, and cannot be revoked again.
	for _, path := range []string{"/api/v1/invites/" + invites[0].Id, "/api/v1/password-resets/" + resets[0].Id} {
		if rec := f.do(t, request{method: http.MethodDelete, path: path, cookie: owner}); rec.Code != http.StatusNoContent {
			t.Fatalf("revoke %s: %d %s", path, rec.Code, rec.Body)
		}
		if rec := f.do(t, request{method: http.MethodDelete, path: path, cookie: owner}); rec.Code != http.StatusNotFound || errorCode(t, rec) != api.CodeNotFound {
			t.Errorf("revoke %s twice: %d %s, want 404", path, rec.Code, rec.Body)
		}
	}
	rec = f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/invites/accept",
		body: `{"token":"` + invite.Token + `","username":"eve","display_name":"Eve","password":"` + ownerPassword + `"}`})
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != api.CodeInvalidLink {
		t.Errorf("accept a revoked invite: %d %s, want 400 %s", rec.Code, rec.Body, api.CodeInvalidLink)
	}
	rec = f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/password-reset",
		body: `{"token":"` + reset.Token + `","password":"` + ownerPassword + `"}`})
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != api.CodeInvalidLink {
		t.Errorf("use a revoked reset: %d %s, want 400 %s", rec.Code, rec.Body, api.CodeInvalidLink)
	}
	for _, path := range []string{"/api/v1/invites", "/api/v1/password-resets"} {
		rec := f.do(t, request{method: http.MethodGet, path: path, cookie: owner})
		if !strings.Contains(rec.Body.String(), `"items":[]`) {
			t.Errorf("GET %s after revoking: %s, want no items", path, rec.Body)
		}
	}

	// Each revocation is in the auth audit log.
	rec = f.do(t, request{method: http.MethodGet, path: "/api/v1/auth-audit?limit=2", cookie: owner})
	audit := decode[api.AuthAuditList](t, rec).Items
	if len(audit) != 2 || audit[0].Action != api.AuthAuditEntryActionPasswordResetRevoked || audit[0].Username != "grace" ||
		audit[1].Action != api.AuthAuditEntryActionInviteRevoked || audit[1].Detail != "editor" ||
		audit[1].ActorName == nil || *audit[1].ActorName != "Ada Lovelace" {
		t.Errorf("newest audit entries = %+v, want the two revocations", audit)
	}
}

func TestUsers_UpdateOwnAccount(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	owner := f.signIn(t)
	other := f.signIn(t)

	rec := f.do(t, request{method: http.MethodPatch, path: "/api/v1/auth/me", body: `{"display_name":"Countess Ada"}`, cookie: owner})
	if rec.Code != http.StatusOK || decode[api.Account](t, rec).DisplayName != "Countess Ada" {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	rec = f.do(t, request{method: http.MethodPatch, path: "/api/v1/auth/me", body: `{"current_password":"wrong password","new_password":"a brand new password"}`, cookie: owner})
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != api.CodeInvalidCredentials {
		t.Errorf("wrong current password: %d %s", rec.Code, rec.Body)
	}
	rec = f.do(t, request{method: http.MethodPatch, path: "/api/v1/auth/me", body: `{"display_name":""}`, cookie: owner})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty display name: status %d, want 400", rec.Code)
	}
	rec = f.do(t, request{method: http.MethodPatch, path: "/api/v1/auth/me", body: `{"current_password":"` + ownerPassword + `","new_password":"a brand new password"}`, cookie: owner})
	if rec.Code != http.StatusOK {
		t.Fatalf("change password: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/auth/me", cookie: owner}); rec.Code != http.StatusOK {
		t.Errorf("the session that changed the password: status %d, want 200", rec.Code)
	}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/auth/me", cookie: other}); rec.Code != http.StatusUnauthorized {
		t.Errorf("another session after a password change: status %d, want 401", rec.Code)
	}
}

func TestUsers_AuthAudit(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	owner := f.signIn(t)
	f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/sign-in", body: `{"username":"ada","password":"wrong password"}`})
	f.inviteAs(t, owner, "viewer", "grace")

	rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/auth-audit?limit=2", cookie: owner})
	if rec.Code != http.StatusOK {
		t.Fatalf("list audit: %d %s", rec.Code, rec.Body)
	}
	list := decode[api.AuthAuditList](t, rec)
	// user_created, sign_in, sign_in_failed, invite_created, invite_accepted
	if list.Total != 5 || len(list.Items) != 2 {
		t.Fatalf("audit = %d of %d, want 2 of 5: %+v", len(list.Items), list.Total, list.Items)
	}
	accepted, invited := list.Items[0], list.Items[1]
	if accepted.Action != api.AuthAuditEntryActionInviteAccepted || accepted.Username != "grace" || accepted.ActorName == nil || *accepted.ActorName != "grace" {
		t.Errorf("newest entry = %+v, want grace accepting", accepted)
	}
	if invited.Action != api.AuthAuditEntryActionInviteCreated || invited.Detail != "viewer" || invited.ActorName == nil || *invited.ActorName != "Ada Lovelace" {
		t.Errorf("second entry = %+v, want Ada's invite", invited)
	}
	rec = f.do(t, request{method: http.MethodGet, path: "/api/v1/auth-audit?offset=2", cookie: owner})
	items := decode[api.AuthAuditList](t, rec).Items
	if len(items) != 3 || items[0].Action != api.AuthAuditEntryActionSignInFailed || items[0].ActorId != nil || items[0].Username != "ada" {
		t.Errorf("older entries = %+v, want the failed sign-in first", items)
	}
}

// The audit list clamps its page: a limit over the most one page holds is cut
// to it, and a limit or offset below the minimum takes the default, so no
// request reads the whole log at once.
func TestUsers_AuthAuditClampsThePage(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	owner := f.signIn(t)
	at := time.Now()
	for i := range auth.MaxAuthAuditLimit + 20 {
		entry := repository.AuthAuditEntry{At: at.Add(time.Duration(i) * time.Second), Action: repository.AuthActionSignInFailed, Username: "u" + strconv.Itoa(i)}
		if err := f.accounts.AppendAuthAudit(context.Background(), &entry); err != nil {
			t.Fatalf("AppendAuthAudit: %v", err)
		}
	}
	for _, tc := range []struct {
		query string
		want  int
	}{
		{"limit=100000", auth.MaxAuthAuditLimit},
		{"limit=" + strconv.Itoa(auth.MaxAuthAuditLimit+1), auth.MaxAuthAuditLimit},
		{"limit=0", auth.DefaultAuthAuditLimit},
		{"limit=-1", auth.DefaultAuthAuditLimit},
		{"", auth.DefaultAuthAuditLimit},
		{"limit=3&offset=-5", 3},
	} {
		rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/auth-audit?" + tc.query, cookie: owner})
		if rec.Code != http.StatusOK {
			t.Fatalf("?%s: %d %s", tc.query, rec.Code, rec.Body)
		}
		list := decode[api.AuthAuditList](t, rec)
		if len(list.Items) != tc.want || list.Total <= auth.MaxAuthAuditLimit {
			t.Errorf("?%s: %d of %d entries, want %d of more than %d", tc.query, len(list.Items), list.Total, tc.want, auth.MaxAuthAuditLimit)
		}
	}
	newest := decode[api.AuthAuditList](t, f.do(t, request{method: http.MethodGet, path: "/api/v1/auth-audit?limit=1&offset=-5", cookie: owner}))
	if len(newest.Items) != 1 || newest.Items[0].Username != "u"+strconv.Itoa(auth.MaxAuthAuditLimit+19) {
		t.Errorf("a negative offset: %+v, want the newest entry", newest.Items)
	}
}

func TestUsers_OpenModeHasNoAccounts(t *testing.T) {
	cfg := &config.Config{Port: 8080, LogFormat: "text", AuthMode: config.AuthModeOpen}
	events := memory.NewEventStore()
	server := api.NewServer(cfg, events, memory.NewReadModelStore(), memory.NewSnapshotStore(events), nil,
		api.WithBranchStore(memory.NewBranchStore()))
	f := &authFixture{server: server}
	for _, r := range []request{
		{method: http.MethodGet, path: "/api/v1/users"},
		{method: http.MethodPatch, path: "/api/v1/users/00000000-0000-0000-0000-00000000abcd", body: `{"role":"viewer"}`},
		{method: http.MethodPost, path: "/api/v1/users/00000000-0000-0000-0000-00000000abcd/password-reset"},
		{method: http.MethodPost, path: "/api/v1/invites", body: `{"role":"viewer"}`},
		{method: http.MethodGet, path: "/api/v1/auth-audit"},
		{method: http.MethodGet, path: "/api/v1/invites"},
		{method: http.MethodDelete, path: "/api/v1/invites/x"},
		{method: http.MethodGet, path: "/api/v1/password-resets"},
		{method: http.MethodDelete, path: "/api/v1/password-resets/x"},
		{method: http.MethodPost, path: "/api/v1/auth/invites/accept", body: `{"token":"x","username":"eve","display_name":"Eve","password":"long enough"}`},
		{method: http.MethodPost, path: "/api/v1/auth/password-reset", body: `{"token":"x","password":"long enough"}`},
		{method: http.MethodPatch, path: "/api/v1/auth/me", body: `{"display_name":"x"}`},
	} {
		if rec := f.do(t, r); rec.Code != http.StatusConflict {
			t.Errorf("%s %s in open mode: status %d %s, want 409", r.method, r.path, rec.Code, strings.TrimSpace(rec.Body.String()))
		}
	}
}

func TestUsers_InvalidLinksAreRateLimited(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	var rec *httptest.ResponseRecorder
	for i := 0; i < 20; i++ {
		rec = f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/password-reset", body: `{"token":"wrong","password":"a brand new password"}`})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("attempt %d: status %d, want 400", i, rec.Code)
		}
	}
	rec = f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/password-reset", body: `{"token":"wrong","password":"a brand new password"}`})
	if rec.Code != http.StatusTooManyRequests || errorCode(t, rec) != api.CodeRateLimited {
		t.Errorf("reset after 20 invalid links: %d %s, want 429", rec.Code, rec.Body)
	}
	rec = f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/invites/accept",
		body: `{"token":"wrong","username":"eve","display_name":"Eve","password":"a brand new password"}`})
	if rec.Code != http.StatusTooManyRequests || errorCode(t, rec) != api.CodeRateLimited {
		t.Errorf("accept after 20 invalid links: %d %s, want 429", rec.Code, rec.Body)
	}
}
