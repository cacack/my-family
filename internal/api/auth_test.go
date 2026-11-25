package api_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/auth"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository/memory"
)

const ownerPassword = "correct horse battery"

// authFixture is a server in accounts mode over memory stores.
type authFixture struct {
	server   *api.Server
	auth     *auth.Service
	accounts *memory.AccountStore
	settings *memory.SettingsStore
	events   *memory.EventStore
	read     *memory.ReadModelStore
}

func newAuthFixture(t *testing.T) *authFixture {
	t.Helper()
	return newAuthFixtureWith(t, &config.Config{Port: 8080, LogFormat: "text", AuthMode: config.AuthModeAccounts})
}

func newAuthFixtureWith(t *testing.T, cfg *config.Config, opts ...api.ServerOption) *authFixture {
	t.Helper()
	events := memory.NewEventStore()
	accounts := memory.NewAccountStore()
	settings := memory.NewSettingsStore()
	svc := auth.NewService(accounts, auth.WithSettings(settings))
	read := memory.NewReadModelStore()
	opts = append([]api.ServerOption{api.WithAuth(svc), api.WithBranchStore(memory.NewBranchStore()), api.WithTreeSettings(accounts),
		api.WithSettingsStore(settings)}, opts...)
	server := api.NewServer(cfg, events, read, memory.NewSnapshotStore(events), nil, opts...)
	return &authFixture{server: server, auth: svc, accounts: accounts, settings: settings, events: events, read: read}
}

// request is one HTTP call to the fixture's server.
type request struct {
	method, path, body string
	cookie             *http.Cookie
	// more are cookies sent besides cookie.
	more   []*http.Cookie
	header map[string]string
	tls    bool
	// remoteAddr is the client's address, or httptest's default.
	remoteAddr string
}

func (f *authFixture) do(t *testing.T, r request) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader = http.NoBody
	if r.body != "" {
		body = strings.NewReader(r.body)
	}
	req := httptest.NewRequest(r.method, r.path, body)
	if r.body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if r.cookie != nil {
		req.AddCookie(r.cookie)
	}
	for _, c := range r.more {
		req.AddCookie(c)
	}
	for k, v := range r.header {
		req.Header.Set(k, v)
	}
	if r.tls {
		req.TLS = &tls.ConnectionState{}
	}
	if r.remoteAddr != "" {
		req.RemoteAddr = r.remoteAddr
	}
	rec := httptest.NewRecorder()
	f.server.Echo().ServeHTTP(rec, req)
	return rec
}

func (f *authFixture) createOwner(t *testing.T) {
	t.Helper()
	if _, err := f.auth.CreateOwner(context.Background(), auth.Actor{}, "ada", "Ada Lovelace", ownerPassword); err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
}

// signIn signs the owner in and returns the session cookie.
func (f *authFixture) signIn(t *testing.T) *http.Cookie {
	t.Helper()
	rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/sign-in", body: `{"username":"ada","password":"` + ownerPassword + `"}`})
	if rec.Code != http.StatusOK {
		t.Fatalf("sign-in: status %d, body %s", rec.Code, rec.Body)
	}
	return sessionCookieOf(t, rec)
}

func sessionCookieOf(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == "myfamily_session" {
			return c
		}
	}
	t.Fatalf("no session cookie in %v", rec.Header().Values("Set-Cookie"))
	return nil
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body %q: %v", rec.Body, err)
	}
	return body.Code
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body, err)
	}
	return v
}

func TestNewServer_RequiresAuthOutsideOpenMode(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("NewServer in accounts mode without WithAuth did not panic")
		}
	}()
	events := memory.NewEventStore()
	api.NewServer(&config.Config{}, events, memory.NewReadModelStore(), memory.NewSnapshotStore(events), nil)
}

func TestAuth_SetupMode(t *testing.T) {
	f := newAuthFixture(t)

	for _, path := range []string{"/api/v1/persons", "/api/v1/config", "/api/v1/auth/me", "/api/v1/nonexistent"} {
		rec := f.do(t, request{method: http.MethodGet, path: path})
		if rec.Code != http.StatusServiceUnavailable || errorCode(t, rec) != api.CodeSetupRequired {
			t.Errorf("GET %s in setup mode: %d %s, want 503 SETUP_REQUIRED", path, rec.Code, rec.Body)
		}
	}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/health"}); rec.Code != http.StatusOK {
		t.Errorf("health in setup mode: %d", rec.Code)
	}
	status := decode[api.AuthStatus](t, f.do(t, request{method: http.MethodGet, path: "/api/v1/auth/status"}))
	if status.Mode != api.AuthStatusModeAccounts || !status.SetupRequired || status.Account != nil {
		t.Errorf("status = %+v, want accounts mode, setup required, no account", status)
	}

	token, err := f.auth.SetupToken(context.Background())
	if err != nil || token == "" {
		t.Fatalf("SetupToken = %q, %v", token, err)
	}
	setup := func(token, password string) *httptest.ResponseRecorder {
		return f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/setup",
			body: `{"token":"` + token + `","username":"Ada","display_name":"Ada Lovelace","password":"` + password + `"}`})
	}
	if rec := setup("wrong", ownerPassword); rec.Code != http.StatusForbidden || errorCode(t, rec) != api.CodeInvalidSetupToken {
		t.Errorf("setup with a wrong token: %d %s, want 403", rec.Code, rec.Body)
	}
	if rec := setup(token, "short"); rec.Code != http.StatusBadRequest {
		t.Errorf("setup with a short password: %d %s, want 400", rec.Code, rec.Body)
	}

	rec := setup(token, ownerPassword)
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup: %d %s", rec.Code, rec.Body)
	}
	account := decode[api.Account](t, rec)
	if account.Username != "ada" || account.DisplayName != "Ada Lovelace" {
		t.Errorf("setup account = %+v", account)
	}
	cookie := sessionCookieOf(t, rec)
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/persons", cookie: cookie}); rec.Code != http.StatusOK {
		t.Errorf("persons after setup: %d %s", rec.Code, rec.Body)
	}
	if rec := setup(token, ownerPassword); rec.Code != http.StatusConflict {
		t.Errorf("second setup: %d %s, want 409", rec.Code, rec.Body)
	}
}

func TestAuth_SessionCookieAttributes(t *testing.T) {
	tests := []struct {
		name       string
		tls        bool
		header     map[string]string
		wantSecure bool
	}{
		{name: "plain HTTP", wantSecure: false},
		{name: "TLS", tls: true, wantSecure: true},
		{name: "TLS at a proxy", header: map[string]string{"X-Forwarded-Proto": "https"}, wantSecure: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newAuthFixture(t)
			f.createOwner(t)
			rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/sign-in", tls: tt.tls, header: tt.header,
				body: `{"username":"ada","password":"` + ownerPassword + `"}`})
			if rec.Code != http.StatusOK {
				t.Fatalf("sign-in: %d %s", rec.Code, rec.Body)
			}
			cookie := sessionCookieOf(t, rec)
			if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Secure != tt.wantSecure || cookie.Path != "/" {
				t.Errorf("cookie = %+v, want HttpOnly, SameSite=Lax, Path=/, Secure=%v", cookie, tt.wantSecure)
			}
			if cookie.MaxAge != int(auth.AbsoluteTimeout.Seconds()) {
				t.Errorf("cookie MaxAge = %d, want the absolute session lifetime", cookie.MaxAge)
			}
		})
	}
}

func TestAuth_SignInAndOut(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)

	rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/persons"})
	if rec.Code != http.StatusUnauthorized || errorCode(t, rec) != api.CodeUnauthenticated {
		t.Errorf("persons without a session: %d %s, want 401 UNAUTHENTICATED", rec.Code, rec.Body)
	}
	forged := &http.Cookie{Name: "myfamily_session", Value: "forged"}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/persons", cookie: forged}); rec.Code != http.StatusUnauthorized {
		t.Errorf("persons with a forged session: %d, want 401", rec.Code)
	}
	status := decode[api.AuthStatus](t, f.do(t, request{method: http.MethodGet, path: "/api/v1/auth/status"}))
	if status.SetupRequired || status.Account != nil {
		t.Errorf("status signed out = %+v, want no setup and no account", status)
	}

	rec = f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/sign-in", body: `{"username":"ada","password":"wrong password"}`})
	if rec.Code != http.StatusUnauthorized || errorCode(t, rec) != api.CodeInvalidCredentials {
		t.Errorf("wrong password: %d %s, want 401 INVALID_CREDENTIALS", rec.Code, rec.Body)
	}

	cookie := f.signIn(t)
	me := f.do(t, request{method: http.MethodGet, path: "/api/v1/auth/me", cookie: cookie})
	if me.Code != http.StatusOK {
		t.Fatalf("me: %d %s", me.Code, me.Body)
	}
	if account := decode[api.Account](t, me); account.Username != "ada" || account.DisplayName != "Ada Lovelace" {
		t.Errorf("me = %+v", account)
	}
	status = decode[api.AuthStatus](t, f.do(t, request{method: http.MethodGet, path: "/api/v1/auth/status", cookie: cookie}))
	if status.Account == nil || status.Account.Username != "ada" {
		t.Errorf("status signed in = %+v, want the account", status)
	}

	out := f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/sign-out", cookie: cookie})
	if out.Code != http.StatusNoContent {
		t.Fatalf("sign-out: %d %s", out.Code, out.Body)
	}
	if cleared := sessionCookieOf(t, out); cleared.MaxAge >= 0 || cleared.Value != "" {
		t.Errorf("sign-out cookie = %+v, want it cleared", cleared)
	}
	// The session is revoked on the server: replaying the old cookie fails.
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/auth/me", cookie: cookie}); rec.Code != http.StatusUnauthorized {
		t.Errorf("me after sign-out with the old cookie: %d, want 401", rec.Code)
	}
	if rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/sign-out"}); rec.Code != http.StatusNoContent {
		t.Errorf("sign-out without a session: %d, want 204", rec.Code)
	}
}

func TestAuth_SignInRateLimited(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	var rec *httptest.ResponseRecorder
	for range 6 {
		rec = f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/sign-in", body: `{"username":"ada","password":"wrong password"}`})
	}
	if rec.Code != http.StatusTooManyRequests || errorCode(t, rec) != api.CodeRateLimited {
		t.Errorf("sixth failed sign-in: %d %s, want 429 RATE_LIMITED", rec.Code, rec.Body)
	}
}

// A signed-in change is attributed to the session's user, and history names
// them.
func TestAuth_ChangesCarryTheSignedInUser(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	cookie := f.signIn(t)

	rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/persons", cookie: cookie, body: `{"given_name":"Charles","surname":"Babbage"}`})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create person: %d %s", rec.Code, rec.Body)
	}
	history := decode[api.ChangeHistoryResponse](t, f.do(t, request{method: http.MethodGet, path: "/api/v1/history", cookie: cookie}))
	if len(history.Items) == 0 {
		t.Fatal("no history entries")
	}
	entry := history.Items[0]
	me := decode[api.Account](t, f.do(t, request{method: http.MethodGet, path: "/api/v1/auth/me", cookie: cookie}))
	if entry.UserId == nil || *entry.UserId != me.Id.String() {
		t.Errorf("history user_id = %v, want %s", entry.UserId, me.Id)
	}
	if entry.UserName == nil || *entry.UserName != "Ada Lovelace" {
		t.Errorf("history user_name = %v, want Ada Lovelace", entry.UserName)
	}
}

// Neither the password, the session token nor an API token reaches the
// event log or a log line.
func TestAuth_SecretsStayOutOfLogs(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w // the request logger writes here; it is bound when the server is built
	var slogged bytes.Buffer
	prevSlog := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&slogged, nil)))
	f := newAuthFixture(t)

	token, _ := f.auth.SetupToken(context.Background())
	rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/setup",
		body: `{"token":"` + token + `","username":"ada","display_name":"Ada","password":"` + ownerPassword + `"}`})
	f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/sign-in", body: `{"username":"ada","password":"wrong password"}`})
	cookie := f.signIn(t)
	f.do(t, request{method: http.MethodPost, path: "/api/v1/persons", cookie: cookie, body: `{"given_name":"Charles"}`})
	apiToken := f.createToken(t, cookie, "script")
	f.do(t, request{method: http.MethodPost, path: "/api/v1/persons", header: bearer(apiToken.Token), body: `{"given_name":"Ada"}`})
	f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/sign-out", cookie: cookie})

	os.Stdout = stdout
	slog.SetDefault(prevSlog)
	_ = w.Close()
	logged, _ := io.ReadAll(r)
	logged = append(logged, slogged.Bytes()...)
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup: %d %s", rec.Code, rec.Body)
	}
	if len(logged) == 0 {
		t.Fatal("captured no log output: the check would prove nothing")
	}

	all, err := f.events.ReadAll(context.Background(), 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	var eventLog bytes.Buffer
	for _, evt := range all {
		eventLog.Write(evt.Data)
		eventLog.Write(evt.Metadata)
	}
	for name, secret := range map[string]string{"password": ownerPassword, "wrong password": "wrong password", "session token": cookie.Value, "API token": apiToken.Token} {
		if bytes.Contains(logged, []byte(secret)) {
			t.Errorf("the %s appears in a log line", name)
		}
		if bytes.Contains(eventLog.Bytes(), []byte(secret)) {
			t.Errorf("the %s appears in the event log", name)
		}
	}
}

func TestAuth_RefusesCrossOriginWrites(t *testing.T) {
	cfg := &config.Config{Port: 8080, LogFormat: "text", AuthMode: config.AuthModeOpen}
	events := memory.NewEventStore()
	server := api.NewServer(cfg, events, memory.NewReadModelStore(), memory.NewSnapshotStore(events), nil)
	f := &authFixture{server: server}

	tests := []struct {
		name   string
		method string
		header map[string]string
		want   int
	}{
		{"same origin", http.MethodPost, map[string]string{"Origin": "http://example.com"}, http.StatusCreated},
		{"no Origin (not a browser)", http.MethodPost, nil, http.StatusCreated},
		{"proxy-forwarded host", http.MethodPost, map[string]string{"Origin": "https://tree.example", "X-Forwarded-Host": "tree.example"}, http.StatusCreated},
		{"another site", http.MethodPost, map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden},
		{"another port", http.MethodPost, map[string]string{"Origin": "http://example.com:8081"}, http.StatusForbidden},
		{"opaque origin", http.MethodPost, map[string]string{"Origin": "null"}, http.StatusForbidden},
		{"a read from another site", http.MethodGet, map[string]string{"Origin": "https://evil.example"}, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := request{method: tt.method, path: "/api/v1/persons", header: tt.header}
			if tt.method == http.MethodPost {
				r.body = `{"given_name":"Ada"}`
			}
			rec := f.do(t, r)
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d (%s)", rec.Code, tt.want, rec.Body)
			}
			if tt.want == http.StatusForbidden && errorCode(t, rec) != api.CodeForbidden {
				t.Errorf("code = %s, want FORBIDDEN", errorCode(t, rec))
			}
		})
	}
}

func TestAuth_OpenMode(t *testing.T) {
	for _, cfg := range []*config.Config{
		{AuthMode: config.AuthModeOpen},
		{DemoMode: true},
	} {
		events := memory.NewEventStore()
		server := api.NewServer(cfg, events, memory.NewReadModelStore(), memory.NewSnapshotStore(events), nil)
		f := &authFixture{server: server, events: events}

		status := decode[api.AuthStatus](t, f.do(t, request{method: http.MethodGet, path: "/api/v1/auth/status"}))
		if status.Mode != api.AuthStatusModeOpen || status.SetupRequired {
			t.Errorf("open mode status = %+v", status)
		}
		rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/persons", body: `{"given_name":"Ada"}`})
		if rec.Code != http.StatusCreated {
			t.Errorf("create person without a session in open mode: %d %s", rec.Code, rec.Body)
		}
		history := decode[api.ChangeHistoryResponse](t, f.do(t, request{method: http.MethodGet, path: "/api/v1/history"}))
		if len(history.Items) == 0 {
			t.Error("open mode history is empty")
		}
		for _, entry := range history.Items {
			if entry.UserId != nil {
				t.Errorf("open mode change %s %s records user %s, want none", entry.EntityType, entry.Action, *entry.UserId)
			}
		}
		// Every account operation answers open mode alike (#1423).
		for _, r := range []request{
			{method: http.MethodPost, path: "/api/v1/auth/sign-in", body: `{"username":"ada","password":"x"}`},
			{method: http.MethodPost, path: "/api/v1/auth/setup", body: `{"token":"t","username":"ada","display_name":"Ada","password":"` + ownerPassword + `"}`},
			{method: http.MethodGet, path: "/api/v1/auth/me"},
		} {
			rec := f.do(t, r)
			if rec.Code != http.StatusConflict {
				t.Errorf("%s %s in open mode: %d, want 409", r.method, r.path, rec.Code)
				continue
			}
			if got := decode[api.Error](t, rec); got.Code != api.CodeConflict {
				t.Errorf("%s %s in open mode: code %q, want %q", r.method, r.path, got.Code, api.CodeConflict)
			}
		}
		if rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/sign-out"}); rec.Code != http.StatusNoContent {
			t.Errorf("sign-out in open mode: %d, want 204", rec.Code)
		}
	}
}
