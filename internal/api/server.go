package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/audit"
	"github.com/levy-cmd/hublot-kit/internal/auth"
	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

type Deps struct {
	Docker       dockerx.Client
	Verifier     *auth.Verifier
	Sessions     *auth.Sessions
	Limiter      *auth.Limiter
	Audit        *audit.Log
	Version      string
	ReadOnly     bool
	TLS          bool
	PublicOrigin string       // normalized, or ""
	Static       http.Handler // embedded UI; nil in API tests
	ScriptHashes []string     // CSP sources of the UI's inline scripts (web.ScriptHashes)
}

// logClock is the clock used by throttleLog; tests may replace it before New.
var logClock = time.Now

// throttleSweepThreshold is the map size above which throttleLog drops stale IPs.
var throttleSweepThreshold = 1024

// throttleLog limits a repeated audit line to once per IP per minute.
type throttleLog struct {
	mu        sync.Mutex
	last      map[string]time.Time
	now       func() time.Time
	lastSweep time.Time
}

func (t *throttleLog) shouldLog(ip string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	if at, ok := t.last[ip]; ok && now.Sub(at) < time.Minute {
		return false
	}
	if len(t.last) > throttleSweepThreshold && now.Sub(t.lastSweep) >= time.Minute {
		t.lastSweep = now
		for k, at := range t.last {
			if now.Sub(at) >= time.Minute {
				delete(t.last, k)
			}
		}
	}
	t.last[ip] = now
	return true
}

type server struct {
	d        Deps
	throttle *throttleLog
}

func New(d Deps) http.Handler {
	s := &server{d: d, throttle: &throttleLog{last: map[string]time.Time{}, now: logClock}}
	session := func(h http.HandlerFunc) http.Handler { return requireSession(d.Sessions, h) }

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.logout)
	mux.Handle("GET /api/meta", session(s.meta))
	mux.Handle("GET /api/containers", session(s.containers))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "")
	})
	if d.Static != nil {
		mux.Handle("/", d.Static)
	}
	return securityHeaders(originChecker{public: d.PublicOrigin, tls: d.TLS}.wrap(mux), d.ScriptHashes)
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.d.Limiter.Allowed(ip) {
		if s.throttle.shouldLog(ip) {
			if err := s.d.Audit.Write(audit.Event{Action: "login.fail", Result: "rate_limited", Remote: ip}); err != nil {
				slog.Error("audit write", "err", err)
			}
		}
		writeError(w, http.StatusTooManyRequests, "rate_limited", "")
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
	err := dec.Decode(&body)
	if err == nil && dec.Decode(&struct{}{}) != io.EOF {
		err = errors.New("trailing data")
	}
	if err != nil || !s.d.Verifier.Check(strings.TrimSpace(body.Token)) {
		s.d.Limiter.Fail(ip)
		if err := s.d.Audit.Write(audit.Event{Action: "login.fail", Result: "refused", Remote: ip}); err != nil {
			slog.Error("audit write", "err", err)
		}
		writeError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}
	id, err := s.d.Sessions.Create()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: id, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.secureCookies(),
		MaxAge: int(auth.SessionTTL.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) secureCookies() bool {
	return s.d.TLS || strings.HasPrefix(s.d.PublicOrigin, "https://")
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.d.Sessions.Delete(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.secureCookies(), MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) meta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"version": s.d.Version, "readOnly": s.d.ReadOnly})
}

func (s *server) containers(w http.ResponseWriter, r *http.Request) {
	list, err := s.d.Docker.ListContainers(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "docker_unavailable", err.Error())
		return
	}
	if list == nil {
		list = []dockerx.Container{}
	}
	writeJSON(w, http.StatusOK, list)
}
