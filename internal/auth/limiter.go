package auth

import (
	"sync"
	"time"
)

// limiter counts recent failures per key and refuses a key that has failed
// too often within the window. It lives in memory: a restart forgets it,
// which is acceptable for slowing down guessing, and a server instance is
// the only one answering its sign-ins.
type limiter struct {
	mu       sync.Mutex
	window   time.Duration
	failures map[string][]time.Time
}

func newLimiter(window time.Duration) *limiter {
	return &limiter{window: window, failures: make(map[string][]time.Time)}
}

// recent returns key's failures still inside the window at now, dropping
// older ones. The caller holds mu.
func (l *limiter) recent(key string, now time.Time) []time.Time {
	kept := l.failures[key][:0]
	for _, at := range l.failures[key] {
		if now.Sub(at) < l.window {
			kept = append(kept, at)
		}
	}
	if len(kept) == 0 {
		delete(l.failures, key)
		return nil
	}
	l.failures[key] = kept
	return kept
}

// blocked reports whether key has failed max or more times within the window.
func (l *limiter) blocked(key string, maxFailures int, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(key, now)) >= maxFailures
}

// fail records a failure for each of keys. It also sweeps keys whose failures
// have all expired, so the map does not grow with every name ever tried.
func (l *limiter) fail(now time.Time, keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for key := range l.failures {
		l.recent(key, now)
	}
	for _, key := range keys {
		l.failures[key] = append(l.failures[key], now)
	}
}

// reset forgets key's failures.
func (l *limiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, key)
}
