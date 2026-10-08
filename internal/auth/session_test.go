package auth

import (
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time      { return c.t }
func (c *clock) add(d time.Duration) { c.t = c.t.Add(d) }
func newClock() *clock               { return &clock{t: time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)} }

func TestSessionLifecycle(t *testing.T) {
	c := newClock()
	s := NewSessions(SessionTTL, c.now)
	id, err := s.Create()
	if err != nil || len(id) != 43 {
		t.Fatalf("bad id %q %v", id, err)
	}
	if !s.Valid(id) || s.Valid("autre") || s.Valid("") {
		t.Fatal("only the created id is valid")
	}
	c.add(SessionTTL - time.Second)
	if !s.Valid(id) {
		t.Fatal("still valid just before expiry")
	}
	c.add(2 * time.Second)
	if s.Valid(id) {
		t.Fatal("expired after 12 h")
	}
}

func TestSessionDelete(t *testing.T) {
	s := NewSessions(SessionTTL, newClock().now)
	id, _ := s.Create()
	s.Delete(id)
	if s.Valid(id) {
		t.Fatal("deleted session must be invalid")
	}
}
