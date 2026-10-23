package auth

import (
	"slices"
	"sync"
	"time"
)

var sweepThreshold = 1024

// Limiter counts failures per key over a sliding window.
type Limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	now    func() time.Time
	fails  map[string][]time.Time
	// lastSweep bounds the full-map sweep to once per window.
	lastSweep time.Time
}

func NewLimiter(max int, window time.Duration, now func() time.Time) *Limiter {
	return &Limiter{max: max, window: window, now: now, fails: map[string][]time.Time{}}
}

func (l *Limiter) prune(key string) []time.Time {
	cutoff := l.now().Add(-l.window)
	kept := l.fails[key][:0]
	for _, t := range l.fails[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.fails, key)
		return nil
	}
	l.fails[key] = kept
	return kept
}

func (l *Limiter) Allowed(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prune(key)) < l.max
}

func (l *Limiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	pruned := l.prune(key)
	if len(pruned) >= l.max {
		// Keep only the newest max-1 timestamps, drop the oldest
		pruned = pruned[len(pruned)-l.max+1:]
	}
	l.fails[key] = append(pruned, l.now())
	l.sweep()
}

// Take checks and counts an attempt in one step (v0.1.2, audit: checked
// before the body, counted after it, a burst got 40 tokens checked instead
// of 5). ok false: the limit is reached, nothing is counted. The attempt
// counts as a failure until Refund gives it back.
func (l *Limiter) Take(key string) (at time.Time, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.prune(key)) >= l.max {
		return time.Time{}, false
	}
	at = l.now()
	l.fails[key] = append(l.fails[key], at)
	l.sweep()
	return at, true
}

// Refund gives back the attempt Take counted at at (a good token).
func (l *Limiter) Refund(key string, at time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	ts := l.fails[key]
	if i := slices.IndexFunc(ts, at.Equal); i >= 0 {
		ts = slices.Delete(ts, i, i+1)
		if len(ts) == 0 {
			delete(l.fails, key)
		} else {
			l.fails[key] = ts
		}
	}
}

// sweep drops idle keys if the map is getting large, at most once per
// window. l.mu is held.
func (l *Limiter) sweep() {
	if now := l.now(); len(l.fails) > sweepThreshold && now.Sub(l.lastSweep) >= l.window {
		l.lastSweep = now
		for k := range l.fails {
			l.prune(k)
		}
	}
}
