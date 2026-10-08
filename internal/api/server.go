package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

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
}

type server struct{ d Deps }

func New(d Deps) http.Handler {
	s := &server{d: d}
	session := func(h http.HandlerFunc) http.Handler { return requireSession(d.Sessions, h) }

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", s.login)
	mux.Handle("POST /api/logout", session(s.logout))
	mux.Handle("GET /api/meta", session(s.meta))
	mux.Handle("GET /api/containers", session(s.containers))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "")
	})
	if d.Static != nil {
		mux.Handle("/", d.Static)
	}
	return securityHeaders(originChecker{public: d.PublicOrigin, tls: d.TLS}.wrap(mux))
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.d.Limiter.Allowed(ip) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "")
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body)
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
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.d.TLS,
		MaxAge: int(auth.SessionTTL.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.d.Sessions.Delete(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.d.TLS, MaxAge: -1})
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
