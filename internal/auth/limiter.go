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
	mu        sync.Mutex
	window    time.Duration
	failures  map[string][]time.Time
	lastSweep time.Time
}

// limit is a key and the failures within the window that block it.
type limit struct {
	key string
	max int
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

// acquire admits an attempt unless one of limits is already at its max, and
// counts the admitted attempt as a failure against every limit until release
// withdraws it. Checking and counting under one lock means a burst of parallel
// attempts cannot all pass the check before any of them is counted.
func (l *limiter) acquire(now time.Time, limits ...limit) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	// Sweep keys whose failures have all expired, at most once a window, so
	// the map does not grow with every name ever tried and a failure does not
	// cost a pass over every key.
	if now.Sub(l.lastSweep) >= l.window {
		for key := range l.failures {
			l.recent(key, now)
		}
		l.lastSweep = now
	}
	for _, lim := range limits {
		if len(l.recent(lim.key, now)) >= lim.max {
			return false
		}
	}
	for _, lim := range limits {
		l.failures[lim.key] = append(l.failures[lim.key], now)
	}
	return true
}

// release withdraws an attempt acquire counted at now, for an attempt that
// succeeded.
func (l *limiter) release(now time.Time, keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, key := range keys {
		failures := l.failures[key]
		for i := len(failures) - 1; i >= 0; i-- {
			if failures[i].Equal(now) {
				failures = append(failures[:i], failures[i+1:]...)
				break
			}
		}
		if len(failures) == 0 {
			delete(l.failures, key)
		} else {
			l.failures[key] = failures
		}
	}
}

// reset forgets key's failures.
func (l *limiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, key)
}
