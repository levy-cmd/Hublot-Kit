package auth

import (
	"sync"
	"time"
)

const SessionTTL = 12 * time.Hour

type Sessions struct {
	mu      sync.Mutex
	ttl     time.Duration
	now     func() time.Time
	expires map[string]time.Time
}

func NewSessions(ttl time.Duration, now func() time.Time) *Sessions {
	return &Sessions{ttl: ttl, now: now, expires: map[string]time.Time{}}
}

func (s *Sessions) Create() (string, error) {
	id, err := GenerateToken()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for k, exp := range s.expires { // purge expired sessions
		if !now.Before(exp) {
			delete(s.expires, k)
		}
	}
	s.expires[id] = now.Add(s.ttl)
	return id, nil
}

func (s *Sessions) Valid(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.expires[id]
	if !ok {
		return false
	}
	if !s.now().Before(exp) {
		delete(s.expires, id)
		return false
	}
	return true
}

func (s *Sessions) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.expires, id)
}
