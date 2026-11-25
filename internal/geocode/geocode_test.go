package geocode_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cacack/my-family/internal/geocode"
)

// stub is a Nominatim stand-in that records every request it gets.
type stub struct {
	*httptest.Server
	mu       sync.Mutex
	requests []*http.Request
	times    []time.Time
}

func newStub(t *testing.T, handler http.HandlerFunc) *stub {
	t.Helper()
	s := &stub{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests = append(s.requests, r)
		s.times = append(s.times, time.Now())
		s.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *stub) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

const bostonJSON = `[{"display_name":"Boston, Suffolk County, Massachusetts, United States","lat":"42.3554334","lon":"-71.060511",` +
	`"licence":"Data © OpenStreetMap contributors, ODbL 1.0. http://osm.org/copyright"},` +
	`{"display_name":"Boston, Lincolnshire, England","lat":"52.9776","lon":"-0.0237"},` +
	`{"display_name":"Bad","lat":"north","lon":"0"},` +
	`{"display_name":"Out of range","lat":"91","lon":"0"}]`

func answer(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

func newLookup(s *stub, opts ...geocode.Option) *geocode.Lookup {
	return geocode.NewLookup(geocode.NewNominatim(s.URL+"/", "ada@example.com", "1.2.3", s.Client()), opts...)
}

func TestNominatim_Request(t *testing.T) {
	s := newStub(t, answer(bostonJSON))
	got, err := newLookup(s, geocode.WithInterval(time.Millisecond)).Search(context.Background(), "  Boston,\tMA ")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if s.count() != 1 {
		t.Fatalf("requests = %d, want 1", s.count())
	}
	r := s.requests[0]
	if r.URL.Path != "/search" {
		t.Errorf("path = %q, want /search", r.URL.Path)
	}
	if ua := r.Header.Get("User-Agent"); !strings.Contains(ua, "ada@example.com") || !strings.Contains(ua, "my-family/1.2.3") {
		t.Errorf("User-Agent = %q, want the contact and version", ua)
	}
	if r.Header.Get("Referer") != "" || r.Header.Get("Cookie") != "" {
		t.Errorf("request carries Referer %q or Cookie %q", r.Header.Get("Referer"), r.Header.Get("Cookie"))
	}
	q := r.URL.Query()
	if q.Get("q") != "Boston, MA" {
		t.Errorf("q = %q, want the normalized place name", q.Get("q"))
	}
	// The place name is the only parameter that comes from the tree.
	for key := range q {
		if key != "q" && key != "format" && key != "limit" {
			t.Errorf("unexpected parameter %q", key)
		}
	}
	if q.Get("format") != "jsonv2" || q.Get("limit") != "5" {
		t.Errorf("format = %q, limit = %q", q.Get("format"), q.Get("limit"))
	}

	want := []geocode.Candidate{
		{DisplayName: "Boston, Suffolk County, Massachusetts, United States", Latitude: 42.3554334, Longitude: -71.060511,
			Query: "Boston, MA", Provider: "Nominatim", Attribution: "Data © OpenStreetMap contributors, ODbL 1.0. http://osm.org/copyright"},
		{DisplayName: "Boston, Lincolnshire, England", Latitude: 52.9776, Longitude: -0.0237,
			Query: "Boston, MA", Provider: "Nominatim", Attribution: "Data © OpenStreetMap contributors, ODbL 1.0. https://osm.org/copyright"},
	}
	if len(got) != len(want) {
		t.Fatalf("candidates = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("candidate %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestNominatim_LimitsCandidates(t *testing.T) {
	item := `{"display_name":"X","lat":"1","lon":"2"}`
	s := newStub(t, answer("["+strings.Repeat(item+",", 9)+item+"]"))
	got, err := newLookup(s).Search(context.Background(), "X")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != geocode.MaxCandidates {
		t.Errorf("candidates = %d, want %d", len(got), geocode.MaxCandidates)
	}
}

func TestLookup_CachesByName(t *testing.T) {
	s := newStub(t, answer(bostonJSON))
	l := newLookup(s)
	for _, name := range []string{"Boston, MA", "boston,  ma", "BOSTON, MA"} {
		if _, err := l.Search(context.Background(), name); err != nil {
			t.Fatalf("Search(%q): %v", name, err)
		}
	}
	if s.count() != 1 {
		t.Errorf("requests = %d, want 1 for one place name", s.count())
	}
}

func TestLookup_CachesEmptyResult(t *testing.T) {
	s := newStub(t, answer(`[]`))
	l := newLookup(s)
	for range 2 {
		got, err := l.Search(context.Background(), "Nowhere")
		if err != nil || len(got) != 0 {
			t.Fatalf("Search = %v, %v; want no candidates", got, err)
		}
	}
	if s.count() != 1 {
		t.Errorf("requests = %d, want 1", s.count())
	}
}

func TestLookup_SpacesRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("waits two seconds for the default rate limit")
	}
	s := newStub(t, answer(`[]`))
	l := newLookup(s) // default: one per second
	ctx := context.Background()
	var wg sync.WaitGroup
	for _, name := range []string{"A", "B", "C"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := l.Search(ctx, name); err != nil {
				t.Errorf("Search(%q): %v", name, err)
			}
		}()
	}
	wg.Wait()
	if s.count() != 3 {
		t.Fatalf("requests = %d, want 3", s.count())
	}
	for i := 1; i < len(s.times); i++ {
		// A little slack for the clock: rate.Limiter spaces them a second apart.
		if gap := s.times[i].Sub(s.times[i-1]); gap < geocode.DefaultInterval-50*time.Millisecond {
			t.Errorf("request %d came %v after the last, want at least %v", i, gap, geocode.DefaultInterval)
		}
	}
}

func TestLookup_Errors(t *testing.T) {
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	tests := []struct {
		name    string
		handler http.HandlerFunc
		query   string
		want    error
	}{
		{name: "server error", handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) },
			query: "A", want: geocode.ErrProvider},
		{name: "rate limited", handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) },
			query: "A", want: geocode.ErrProvider},
		{name: "not JSON", handler: answer("<html>"), query: "A", want: geocode.ErrProvider},
		{name: "no answer in time", handler: func(_ http.ResponseWriter, r *http.Request) {
			select {
			case <-block:
			case <-r.Context().Done():
			}
		}, query: "A", want: geocode.ErrTimeout},
		{name: "empty name", handler: answer(`[]`), query: " \t", want: geocode.ErrQuery},
		{name: "long name", handler: answer(`[]`), query: strings.Repeat("a", geocode.MaxQueryLength+1), want: geocode.ErrQuery},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStub(t, tt.handler)
			l := newLookup(s, geocode.WithTimeout(100*time.Millisecond), geocode.WithInterval(time.Millisecond))
			_, err := l.Search(context.Background(), tt.query)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Search error = %v, want %v", err, tt.want)
			}
			if tt.want == geocode.ErrQuery && s.count() != 0 {
				t.Errorf("requests = %d, want none for a refused name", s.count())
			}
			// An error is not cached: the next lookup asks again.
			if tt.want != geocode.ErrQuery {
				_, _ = l.Search(context.Background(), tt.query)
				if s.count() != 2 {
					t.Errorf("requests = %d, want 2 after a failure", s.count())
				}
			}
		})
	}
}

func TestLookup_TimesOutWaitingForItsTurn(t *testing.T) {
	s := newStub(t, answer(`[]`))
	l := newLookup(s, geocode.WithTimeout(50*time.Millisecond))
	if _, err := l.Search(context.Background(), "A"); err != nil {
		t.Fatalf("Search(A): %v", err)
	}
	// The next slot is a second away, past the timeout.
	if _, err := l.Search(context.Background(), "B"); !errors.Is(err, geocode.ErrTimeout) {
		t.Errorf("Search(B) error = %v, want ErrTimeout", err)
	}
	if s.count() != 1 {
		t.Errorf("requests = %d, want 1", s.count())
	}
}

func TestLookup_CallerCancels(t *testing.T) {
	s := newStub(t, answer(`[]`))
	l := newLookup(s)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := l.Search(ctx, "A"); !errors.Is(err, context.Canceled) {
		t.Errorf("Search error = %v, want context.Canceled", err)
	}
}

func TestNominatim_Unreachable(t *testing.T) {
	s := newStub(t, answer(`[]`))
	url := s.URL
	s.Close()
	l := geocode.NewLookup(geocode.NewNominatim(url, "ada@example.com", "dev", nil))
	if _, err := l.Search(context.Background(), "A"); !errors.Is(err, geocode.ErrProvider) {
		t.Errorf("Search error = %v, want ErrProvider", err)
	}
}
