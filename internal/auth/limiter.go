package auth

import (
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
	// Sweep idle keys if the map is getting large, at most once per window.
	if now := l.now(); len(l.fails) > sweepThreshold && now.Sub(l.lastSweep) >= l.window {
		l.lastSweep = now
		for k := range l.fails {
			l.prune(k)
		}
	}
}
