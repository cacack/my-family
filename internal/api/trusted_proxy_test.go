package api_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository/memory"
)

// proxy is the reverse proxy the trusted-proxy fixtures trust; 192.168.1.1
// is a private address outside TRUSTED_PROXIES, which must not be trusted.
const (
	trustedProxies = "10.0.0.0/8, 2001:db8::1"
	proxy          = "10.0.0.1:443"
)

func newProxiedAuthFixture(t *testing.T) *authFixture {
	t.Helper()
	return newAuthFixtureWith(t, &config.Config{Port: 8080, LogFormat: "text",
		AuthMode: config.AuthModeAccounts, TrustedProxies: trustedProxies})
}

// With TRUSTED_PROXIES set, the client address is the nearest X-Forwarded-For
// entry that is not a trusted proxy, and only when the peer is one; X-Real-IP
// is never read. Without it, see TestRealIPIgnoresForwardingHeaders.
func TestRealIP_TrustedProxies(t *testing.T) {
	f := newProxiedAuthFixture(t)
	tests := []struct {
		name, peer, xff, realIP, want string
	}{
		{name: "trusted peer, forwarded client", peer: proxy, xff: "203.0.113.7", want: "203.0.113.7"},
		{name: "trusted peer, client-sent entry before the proxy's", peer: proxy, xff: "198.51.100.9, 203.0.113.7", want: "203.0.113.7"},
		{name: "two trusted hops", peer: proxy, xff: "203.0.113.7, 10.0.0.2", want: "203.0.113.7"},
		{name: "trusted IPv6 address", peer: "[2001:db8::1]:443", xff: "203.0.113.7", want: "203.0.113.7"},
		{name: "trusted peer, no header", peer: proxy, want: "10.0.0.1"},
		{name: "trusted peer, unparsable header", peer: proxy, xff: "unknown", want: "10.0.0.1"},
		{name: "trusted peer, X-Real-IP only", peer: proxy, realIP: "203.0.113.7", want: "10.0.0.1"},
		{name: "untrusted public peer", peer: "198.51.100.1:1000", xff: "203.0.113.7", want: "198.51.100.1"},
		{name: "untrusted private peer", peer: "192.168.1.1:1000", xff: "203.0.113.7", want: "192.168.1.1"},
		{name: "untrusted loopback peer", peer: "127.0.0.1:1000", xff: "203.0.113.7", want: "127.0.0.1"},
		{name: "other address in the IPv6 range of one", peer: "[2001:db8::2]:443", xff: "203.0.113.7", want: "2001:db8::2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			req.RemoteAddr = tt.peer
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.realIP != "" {
				req.Header.Set("X-Real-IP", tt.realIP)
			}
			if got := f.server.Echo().NewContext(req, httptest.NewRecorder()).RealIP(); got != tt.want {
				t.Errorf("RealIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

// failSignIns makes n failed sign-ins, each for its own username so the
// per-username limit never applies, and returns the last answer's status.
func (f *authFixture) failSignIns(t *testing.T, n int, peer string, forwarded func(i int) string) int {
	t.Helper()
	status := 0
	for i := range n {
		rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/sign-in", remoteAddr: peer,
			header: map[string]string{"X-Forwarded-For": forwarded(i)},
			body:   `{"username":"nobody` + strconv.Itoa(i) + `","password":"wrong password"}`})
		status = rec.Code
	}
	return status
}

// signInLimit is the failed sign-ins a client may make before a 429
// (maxFailuresPerClient in internal/auth/service.go).
const signInLimit = 20

// Two clients behind one trusted proxy have their own failed-sign-in buckets,
// and a client cannot leave its bucket with a header entry of its own.
func TestTrustedProxy_SignInBucketPerForwardedClient(t *testing.T) {
	f := newProxiedAuthFixture(t)
	f.createOwner(t)
	spoofed := func(i int) string { return "198.51.100." + strconv.Itoa(i) + ", 203.0.113.1" }
	if got := f.failSignIns(t, signInLimit, proxy, spoofed); got != http.StatusUnauthorized {
		t.Fatalf("20th failed sign-in: %d, want 401", got)
	}
	if got := f.failSignIns(t, 1, proxy, spoofed); got != http.StatusTooManyRequests {
		t.Errorf("21st failed sign-in from the same client: %d, want 429", got)
	}
	other := func(int) string { return "203.0.113.2" }
	if got := f.failSignIns(t, 1, proxy, other); got != http.StatusUnauthorized {
		t.Errorf("another client behind the proxy: %d, want 401", got)
	}
}

// A forwarded header from a peer outside TRUSTED_PROXIES, private or not,
// does not change the client key: varying it does not escape the bucket.
func TestTrustedProxy_UntrustedPeerHeaderIgnored(t *testing.T) {
	for _, peer := range []string{"198.51.100.1:1000", "192.168.1.1:1000"} {
		t.Run(peer, func(t *testing.T) {
			f := newProxiedAuthFixture(t)
			f.createOwner(t)
			fresh := func(i int) string { return "203.0.113." + strconv.Itoa(i) }
			if got := f.failSignIns(t, signInLimit+1, peer, fresh); got != http.StatusTooManyRequests {
				t.Errorf("21st failed sign-in, each forwarded for another address: %d, want 429", got)
			}
		})
	}
}

// Two visitors behind one trusted proxy have their own costly-read buckets.
func TestTrustedProxy_VisitorReadBucketPerForwardedClient(t *testing.T) {
	f := newProxiedAuthFixture(t)
	f.createOwner(t)
	f.openToVisitors(t)
	search := func(client string) int {
		return f.do(t, request{method: http.MethodGet, path: "/api/v1/search?q=ada", remoteAddr: proxy,
			header: map[string]string{"X-Forwarded-For": client}}).Code
	}

	limited := 0
	for range 40 {
		if search("203.0.113.1") == http.StatusTooManyRequests {
			limited++
		}
	}
	if limited == 0 {
		t.Fatal("40 searches from one visitor behind the proxy: none limited")
	}
	if got := search("203.0.113.2"); got != http.StatusOK {
		t.Errorf("another visitor behind the proxy: %d, want 200", got)
	}
}

// An invalid TRUSTED_PROXIES never reaches a running server: Config.Validate
// refuses it at startup, and NewServer panics rather than trust no one quietly.
func TestNewServer_PanicsOnInvalidTrustedProxies(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("NewServer with TRUSTED_PROXIES=proxy.local did not panic")
		}
	}()
	events := memory.NewEventStore()
	api.NewServer(&config.Config{AuthMode: config.AuthModeOpen, TrustedProxies: "proxy.local"},
		events, memory.NewReadModelStore(), memory.NewSnapshotStore(events), nil)
}
