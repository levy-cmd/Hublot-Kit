package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/auth"
)

var ok = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct{ Error string }
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q is not an API error: %v", rec.Body.String(), err)
	}
	return body.Error
}

func originReq(method, origin string) *http.Request {
	r := httptest.NewRequest(method, "http://example.com/api/login", nil) // Host: example.com
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	return r
}

func TestSecurityHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	securityHeaders(ok).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	h := rec.Header()
	if h.Get("Content-Security-Policy") != "default-src 'self'; img-src 'self' blob: data:; frame-ancestors 'none'" ||
		h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("headers: %v", h)
	}
}

func TestOriginGETPassesWithoutOrigin(t *testing.T) {
	rec := httptest.NewRecorder()
	originChecker{}.wrap(ok).ServeHTTP(rec, originReq("GET", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("GET must pass, got %d", rec.Code)
	}
}

func TestOriginPOST(t *testing.T) {
	cases := []struct {
		name, origin string
		pass         bool
	}{
		{"matching", "http://example.com", true},
		{"missing", "", false},
		{"null", "null", false},
		{"other host", "http://evil.example", false},
		{"other port", "http://example.com:8080", false},
		{"malformed", "http//example.com", false},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		originChecker{}.wrap(ok).ServeHTTP(rec, originReq("POST", tc.origin))
		if tc.pass && rec.Code != http.StatusNoContent {
			t.Errorf("%s: want pass, got %d", tc.name, rec.Code)
		}
		if !tc.pass && (rec.Code != http.StatusForbidden || errCode(t, rec) != "forbidden_origin") {
			t.Errorf("%s: want 403 forbidden_origin, got %d", tc.name, rec.Code)
		}
	}
}

func TestOriginMismatchScheme(t *testing.T) {
	// Behind a TLS reverse proxy without --public-origin, the browser sends https
	// while the agent sees plain http: refused, with the dedicated code.
	rec := httptest.NewRecorder()
	originChecker{}.wrap(ok).ServeHTTP(rec, originReq("POST", "https://example.com"))
	if rec.Code != http.StatusForbidden || errCode(t, rec) != "forbidden_origin" {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestOriginPublicOriginWins(t *testing.T) {
	oc := originChecker{public: "https://hublot.example.fr"}
	rec := httptest.NewRecorder()
	r := originReq("POST", "https://hublot.example.fr")
	r.Header.Set("X-Forwarded-Host", "evil.example")
	oc.wrap(ok).ServeHTTP(rec, r)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("configured public origin must pass, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	oc.wrap(ok).ServeHTTP(rec, originReq("POST", "http://example.com"))
	if rec.Code != http.StatusForbidden {
		t.Fatal("Host-derived origin must not pass when --public-origin is set")
	}
}

func TestOriginTLSScheme(t *testing.T) {
	rec := httptest.NewRecorder()
	originChecker{tls: true}.wrap(ok).ServeHTTP(rec, originReq("POST", "https://example.com"))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("TLS agent: https origin must pass, got %d", rec.Code)
	}
}

func TestRequireSession(t *testing.T) {
	s := auth.NewSessions(auth.SessionTTL, time.Now)
	h := requireSession(s, ok)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/meta", nil))
	if rec.Code != http.StatusUnauthorized || errCode(t, rec) != "unauthorized" {
		t.Fatalf("no cookie: got %d", rec.Code)
	}

	id, _ := s.Create()
	r := httptest.NewRequest("GET", "/api/meta", nil)
	r.AddCookie(&http.Cookie{Name: cookieName, Value: id})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("valid cookie: got %d", rec.Code)
	}
}

func TestClientIPIgnoresForwardedFor(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.0.2.7:51000"
	r.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got := clientIP(r); got != "192.0.2.7" {
		t.Fatalf("clientIP = %q", got)
	}
}
