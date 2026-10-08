package auth

import (
	"fmt"
	"testing"
	"time"
)

func TestLimiterBlocksAfterMaxFailures(t *testing.T) {
	c := newClock()
	l := NewLimiter(5, time.Minute, c.now)
	for i := 0; i < 5; i++ {
		if !l.Allowed("192.0.2.1") {
			t.Fatalf("attempt %d must be allowed", i+1)
		}
		l.Fail("192.0.2.1")
	}
	if l.Allowed("192.0.2.1") {
		t.Fatal("6th attempt within a minute must be refused")
	}
	if !l.Allowed("192.0.2.2") {
		t.Fatal("another IP is not affected")
	}
	c.add(time.Minute + time.Second)
	if !l.Allowed("192.0.2.1") {
		t.Fatal("window slid: allowed again")
	}
}

func TestLimiterSlidingWindow(t *testing.T) {
	c := newClock()
	l := NewLimiter(5, time.Minute, c.now)
	// Record 5 failures at t=0, t=20s, t=40s, t=50s, t=55s
	for i := 0; i < 5; i++ {
		if !l.Allowed("192.0.2.1") {
			t.Fatalf("attempt %d must be allowed", i+1)
		}
		l.Fail("192.0.2.1")
		if i < 4 {
			c.add(time.Second * time.Duration([]int{20, 20, 10, 5}[i]))
		}
	}
	// At t=55s, 5 failures in the window [0, 55] → blocked
	if l.Allowed("192.0.2.1") {
		t.Fatal("5 failures in window must block")
	}
	// Advance to t=61s: failure at t=0 expires (window is [1, 61])
	c.add(6 * time.Second)
	if !l.Allowed("192.0.2.1") {
		t.Fatal("after oldest expires, should be allowed")
	}
	// Record one more failure
	l.Fail("192.0.2.1")
	// Now we have 5 failures again → blocked
	if l.Allowed("192.0.2.1") {
		t.Fatal("after adding new failure, should be blocked")
	}
}

func TestLimiterCapsPerKey(t *testing.T) {
	c := newClock()
	l := NewLimiter(5, time.Minute, c.now)
	// Record 100 Fail calls on one key
	for i := 0; i < 100; i++ {
		l.Fail("192.0.2.1")
	}
	// Assert that slice is capped at max
	if len(l.fails["192.0.2.1"]) > 5 {
		t.Fatalf("slice should be capped at max=%d, got %d", 5, len(l.fails["192.0.2.1"]))
	}
}

func TestLimiterSweepsIdleKeys(t *testing.T) {
	c := newClock()
	// Use a test-only override for sweepThreshold
	oldThreshold := sweepThreshold
	sweepThreshold = 10
	t.Cleanup(func() {
		sweepThreshold = oldThreshold
	})

	l := NewLimiter(5, time.Minute, c.now)
	// Record 1 failure on 15 distinct keys
	for i := 0; i < 15; i++ {
		key := "192.0.2." + string(rune(i+1))
		l.Fail(key)
	}
	// Advance clock past the window
	c.add(time.Minute + time.Second)
	// Fail on a new key to trigger sweep
	l.Fail("192.0.3.1")
	// Old keys should be gone (pruned during sweep)
	for i := 0; i < 15; i++ {
		key := "192.0.2." + string(rune(i+1))
		if _, ok := l.fails[key]; ok {
			t.Fatalf("old key %s should have been swept", key)
		}
	}
}

func TestLimiterSweepsAtMostOncePerWindow(t *testing.T) {
	old := sweepThreshold
	sweepThreshold = 10
	t.Cleanup(func() { sweepThreshold = old })
	c := newClock()
	l := NewLimiter(5, time.Minute, c.now)
	add := func(prefix string, n int) {
		for i := 0; i < n; i++ {
			l.Fail(fmt.Sprintf("%s%d", prefix, i))
		}
	}
	add("a", 12) // sweep now
	c.add(5 * time.Second)
	add("b", 11) // no sweep: last one was 5 s ago
	c.add(56 * time.Second)
	add("c", 1) // sweep: a* expire
	if _, ok := l.fails["a0"]; ok {
		t.Fatal("a0 should have been swept")
	}
	c.add(5 * time.Second) // b* expired, but the last sweep was 5 s ago
	add("d", 1)
	if _, ok := l.fails["b0"]; !ok {
		t.Fatal("b0 swept again within the same window")
	}
	c.add(56 * time.Second)
	add("e", 1)
	if _, ok := l.fails["b0"]; ok {
		t.Fatal("b0 should have been swept one window later")
	}
}
