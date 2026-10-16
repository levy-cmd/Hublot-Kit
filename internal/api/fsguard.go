package api

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Spec §13: a FUSE mount of the container that never answers blocks the
// goroutine that reads it. The request must fail anyway, and such goroutines
// must not pile up: at most max operations per container (by full ID), each
// one given up after timeout. A slot is freed only when the work really ends:
// the goroutine of a given-up operation keeps it until it returns, and a
// download keeps it until its copy returns.
var (
	errBusy    = errors.New("too many file operations in progress on this container (stuck FUSE mount?)")
	errTimeout = errors.New("timed out (stuck FUSE mount?)")
)

type fsGuard struct {
	mu      sync.Mutex
	busy    map[string]int
	max     int
	timeout time.Duration
}

func newFSGuard(max int, timeout time.Duration) *fsGuard {
	return &fsGuard{busy: map[string]int{}, max: max, timeout: timeout}
}

// take reserves a slot of container id; the returned func frees it (once).
func (g *fsGuard) take(id string) (func(), bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.busy[id] >= g.max {
		return nil, false
	}
	g.busy[id]++
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			if g.busy[id]--; g.busy[id] <= 0 {
				delete(g.busy, id)
			}
		})
	}, true
}

func noop() {}

// guarded takes a slot of container id and runs fn in its own goroutine.
// Success: it returns fn's result and release, which the caller calls once it
// is done with the result (after streaming a download). Error: the slot is
// already free. Timeout (or end of ctx): errTimeout; the slot stays taken
// until fn returns, then fn's late result goes to cleanup (which closes what
// fn opened). release is never nil: defer it right away.
func guarded[T any](g *fsGuard, ctx context.Context, id string, fn func() (T, error), cleanup func(T)) (T, func(), error) {
	var zero T
	release, ok := g.take(id)
	if !ok {
		return zero, noop, errBusy
	}
	type result struct {
		v   T
		err error
	}
	done := make(chan result, 1)
	go func() {
		// A bare goroutine is not covered by net/http's per-handler recover.
		defer func() {
			if p := recover(); p != nil {
				done <- result{err: fmt.Errorf("panic: %v", p)}
			}
		}()
		v, err := fn()
		done <- result{v, err}
	}()
	timer := time.NewTimer(g.timeout)
	defer timer.Stop()
	select {
	case r := <-done:
		if r.err != nil {
			release()
			return zero, noop, r.err
		}
		return r.v, release, nil
	case <-timer.C:
	case <-ctx.Done():
	}
	go func() {
		if r := <-done; r.err == nil && cleanup != nil {
			cleanup(r.v)
		}
		release()
	}()
	return zero, noop, fmt.Errorf("%w (%s)", errTimeout, g.timeout)
}
