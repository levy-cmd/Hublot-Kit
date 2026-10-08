package auth

import (
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
