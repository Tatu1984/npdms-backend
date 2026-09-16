package middleware

import (
	"sync"
	"time"
)

// A rate limit that still applies when Redis does not.
//
// The limiter was written to fail open: on a Redis error it logged and let the
// request through. Redis was then absent on the deployed API — and so, in
// practice, was every rate limit on it. Twelve wrong-password attempts against
// a live account in a few seconds were all answered 401, none refused, against
// a documented limit of five a minute.
//
// Failing open on a read is defensible. Failing open on the sign-in endpoint is
// how an account is brute-forced, and the demo accounts on the staging
// deployment share one password. Failing CLOSED is not the answer either: a
// Redis outage would then lock every officer out of the platform, which is a
// worse day than a slow one.
//
// So the limit falls back to this: counted in the process's own memory. It is
// weaker than Redis — each instance counts separately, so an attacker reaching
// several instances gets a multiple of the limit — but it bounds the rate, it
// needs nothing deployed, and it cannot lock anybody out. Redis remains the
// right answer and the health endpoint says plainly when it is missing.

type memoryWindow struct {
	hits []time.Time
}

type memoryLimiter struct {
	mu      sync.Mutex
	windows map[string]*memoryWindow
	swept   time.Time
}

var fallbackLimiter = &memoryLimiter{
	windows: make(map[string]*memoryWindow),
	swept:   time.Now(),
}

// allow records a hit and reports whether it is within the limit, along with
// how many remain and when the window resets.
func (m *memoryLimiter) allow(key string, limit int, window time.Duration) (bool, int, time.Time) {
	now := time.Now()
	cutoff := now.Add(-window)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.sweep(now, window)

	w, ok := m.windows[key]
	if !ok {
		w = &memoryWindow{}
		m.windows[key] = w
	}

	// Drop the hits that have aged out of the window.
	kept := w.hits[:0]
	for _, hit := range w.hits {
		if hit.After(cutoff) {
			kept = append(kept, hit)
		}
	}
	w.hits = kept

	reset := now.Add(window)
	if len(w.hits) > 0 {
		reset = w.hits[0].Add(window)
	}

	if len(w.hits) >= limit {
		return false, 0, reset
	}

	w.hits = append(w.hits, now)
	return true, limit - len(w.hits), reset
}

// sweep discards keys nobody has touched for a while, so that a long-running
// process does not accumulate one entry per address it has ever seen. Called
// under the lock, at most once a minute.
func (m *memoryLimiter) sweep(now time.Time, window time.Duration) {
	if now.Sub(m.swept) < time.Minute {
		return
	}
	m.swept = now

	cutoff := now.Add(-window * 2)
	for key, w := range m.windows {
		if len(w.hits) == 0 || w.hits[len(w.hits)-1].Before(cutoff) {
			delete(m.windows, key)
		}
	}
}
