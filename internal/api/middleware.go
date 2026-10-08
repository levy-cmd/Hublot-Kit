package api

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/levy-cmd/hublot-kit/internal/auth"
)

const cookieName = "hublot_session"

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' blob: data:; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// originChecker enforces the CSRF rule of spec §7 on every non-GET/HEAD request.
type originChecker struct {
	public string // normalized --public-origin, or ""
	tls    bool
}

func (o originChecker) trusted(r *http.Request) string {
	if o.public != "" {
		return o.public
	}
	scheme := "http"
	if o.tls {
		scheme = "https"
	}
	return scheme + "://" + strings.ToLower(r.Host)
}

func (o originChecker) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		u, err := url.Parse(origin)
		if origin == "" || origin == "null" || err != nil || u.Scheme == "" || u.Host == "" || u.Path != "" {
			writeError(w, http.StatusForbidden, "forbidden_origin", "missing or malformed Origin")
			return
		}
		if strings.ToLower(u.Scheme)+"://"+strings.ToLower(u.Host) != o.trusted(r) {
			writeError(w, http.StatusForbidden, "forbidden_origin", "Origin "+origin+" is not "+o.trusted(r))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requireSession(s *auth.Sessions, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil || !s.Valid(c.Value) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP uses the TCP peer only; X-Forwarded-* headers are never trusted.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
