// Package geocode looks up candidate coordinates for a place name at an
// outside geocoding provider (#1562). It is opt-in: the server builds a Lookup
// only when GEOCODER_ENABLED is set, because a lookup sends the place name to
// the provider. A lookup stores nothing.
package geocode

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/time/rate"
)

// MaxCandidates is the most candidates a lookup returns.
const MaxCandidates = 5

// MaxQueryLength is the longest place name, in characters, a lookup accepts.
const MaxQueryLength = 255

// Defaults for a Lookup built without options.
const (
	// DefaultInterval spaces outbound requests at one per second, the
	// public Nominatim endpoint's absolute maximum.
	DefaultInterval = time.Second
	// DefaultTimeout bounds one lookup, waiting for its turn included.
	DefaultTimeout = 10 * time.Second
	// maxCacheEntries bounds the cache; a full cache forgets one entry.
	maxCacheEntries = 1024
)

// Candidate is one location a provider matched to a place name.
type Candidate struct {
	DisplayName string
	// Latitude and Longitude are in decimal degrees.
	Latitude  float64
	Longitude float64
	// Query is the text that matched. Today it is always the place name
	// looked up; once fallback matching exists it may be a shorter form of it
	// (#1563).
	Query string
	// Provider names the provider, and Attribution is the credit its data
	// license requires wherever the candidate is shown.
	Provider    string
	Attribution string
}

// Provider is a geocoding service. Search returns at most limit candidates for
// query, none when nothing matched, ErrTimeout when the service did not answer
// in time, and ErrProvider when it failed.
type Provider interface {
	Search(ctx context.Context, query string, limit int) ([]Candidate, error)
}

// The errors a lookup returns besides the caller's own context error.
var (
	ErrTimeout  = errors.New("geocoder timed out")
	ErrProvider = errors.New("geocoder failed")
	// ErrQuery is a place name that is empty or longer than MaxQueryLength.
	ErrQuery = fmt.Errorf("place name must be 1 to %d characters", MaxQueryLength)
)

// Lookup fronts a Provider with an in-memory cache and a rate limit, so that a
// place name is sent once per process and requests are spaced at no more than
// one per interval.
type Lookup struct {
	provider Provider
	timeout  time.Duration
	limiter  *rate.Limiter
	// turn admits one provider lookup at a time, so a repeat of a name in
	// flight is answered from the cache rather than sent again.
	turn chan struct{}
	// mu guards cache, so a cached name is answered without waiting for turn.
	mu    sync.RWMutex
	cache map[string][]Candidate
}

// Option configures a Lookup.
type Option func(*Lookup)

// WithInterval spaces outbound requests at least d apart.
func WithInterval(d time.Duration) Option {
	return func(l *Lookup) { l.limiter = rate.NewLimiter(rate.Every(d), 1) }
}

// WithTimeout bounds one lookup, waiting for its turn included.
func WithTimeout(d time.Duration) Option {
	return func(l *Lookup) { l.timeout = d }
}

// NewLookup returns a Lookup over provider.
func NewLookup(provider Provider, opts ...Option) *Lookup {
	l := &Lookup{
		provider: provider,
		timeout:  DefaultTimeout,
		limiter:  rate.NewLimiter(rate.Every(DefaultInterval), 1),
		turn:     make(chan struct{}, 1),
		cache:    make(map[string][]Candidate),
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Search returns at most MaxCandidates candidates for the place name.
func (l *Lookup) Search(ctx context.Context, name string) ([]Candidate, error) {
	query := strings.Join(strings.Fields(name), " ")
	if query == "" || utf8.RuneCountInString(query) > MaxQueryLength {
		return nil, ErrQuery
	}
	key := strings.ToLower(query)
	if candidates, ok := l.cached(key); ok {
		return candidates, nil
	}

	lookupCtx, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()
	timedOut := func(err error) error {
		if ctx.Err() == nil && errors.Is(lookupCtx.Err(), context.DeadlineExceeded) {
			return ErrTimeout
		}
		return err
	}

	select {
	case l.turn <- struct{}{}:
	case <-lookupCtx.Done():
		return nil, timedOut(lookupCtx.Err())
	}
	defer func() { <-l.turn }()

	// A lookup of the same name may have filled the cache while this one
	// waited for its turn.
	if candidates, ok := l.cached(key); ok {
		return candidates, nil
	}
	if err := l.limiter.Wait(lookupCtx); err != nil {
		// Wait refuses at once when the deadline falls before the next
		// slot, so a refusal with time left is a timeout too.
		if ctx.Err() == nil {
			return nil, ErrTimeout
		}
		return nil, err
	}
	candidates, err := l.provider.Search(lookupCtx, query, MaxCandidates)
	if err != nil {
		return nil, timedOut(err)
	}
	if len(candidates) > MaxCandidates {
		candidates = candidates[:MaxCandidates]
	}
	l.mu.Lock()
	if len(l.cache) >= maxCacheEntries {
		for k := range l.cache {
			delete(l.cache, k)
			break
		}
	}
	l.cache[key] = candidates
	l.mu.Unlock()
	return candidates, nil
}

// cached returns the cached candidates for key, if any.
func (l *Lookup) cached(key string) ([]Candidate, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	candidates, ok := l.cache[key]
	return candidates, ok
}
