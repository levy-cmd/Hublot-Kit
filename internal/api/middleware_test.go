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
	securityHeaders(ok, nil).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	h := rec.Header()
	if h.Get("Content-Security-Policy") != "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' blob: data:; frame-ancestors 'none'" ||
		h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("headers: %v", h)
	}
}

func TestSecurityHeadersScriptHashes(t *testing.T) {
	rec := httptest.NewRecorder()
	securityHeaders(ok, []string{"'sha256-AAA='", "'sha256-BBB='"}).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	want := "default-src 'self'; script-src 'self' 'sha256-AAA=' 'sha256-BBB='; style-src 'self' 'unsafe-inline'; img-src 'self' blob: data:; frame-ancestors 'none'"
	if got := rec.Header().Get("Content-Security-Policy"); got != want {
		t.Fatalf("CSP %q, want %q", got, want)
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
	// A plain-HTTP agent (without TLS) receives an https Origin from the browser:
	// this happens when the agent is behind a TLS reverse proxy without --public-origin.
	// The request is refused with the dedicated code.
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

func TestOriginHTTPMethods(t *testing.T) {
	// PUT, DELETE, PATCH, OPTIONS without Origin should be rejected
	methods := []string{"PUT", "DELETE", "PATCH", "OPTIONS"}
	for _, method := range methods {
		rec := httptest.NewRecorder()
		originChecker{}.wrap(ok).ServeHTTP(rec, originReq(method, ""))
		if rec.Code != http.StatusForbidden || errCode(t, rec) != "forbidden_origin" {
			t.Errorf("%s without Origin: want 403 forbidden_origin, got %d", method, rec.Code)
		}
	}
}

func TestOriginWithUserinfo(t *testing.T) {
	rec := httptest.NewRecorder()
	originChecker{}.wrap(ok).ServeHTTP(rec, originReq("POST", "http://evil@example.com"))
	if rec.Code != http.StatusForbidden || errCode(t, rec) != "forbidden_origin" {
		t.Errorf("Origin with userinfo: want 403 forbidden_origin, got %d", rec.Code)
	}
}

func TestOriginWithQueryFragmentTrailingSlash(t *testing.T) {
	cases := []struct {
		name, origin string
	}{
		{"query", "http://example.com?x"},
		{"fragment", "http://example.com#f"},
		{"trailing slash", "http://example.com/"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		originChecker{}.wrap(ok).ServeHTTP(rec, originReq("POST", tc.origin))
		if rec.Code != http.StatusForbidden || errCode(t, rec) != "forbidden_origin" {
			t.Errorf("%s: want 403 forbidden_origin, got %d", tc.name, rec.Code)
		}
	}
}

func TestOriginCaseInsensitiveHost(t *testing.T) {
	// Host "EXAMPLE.COM" with Origin "http://example.com" should pass
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "http://EXAMPLE.COM/api/login", nil)
	r.Host = "EXAMPLE.COM"
	r.Header.Set("Origin", "http://example.com")
	originChecker{}.wrap(ok).ServeHTTP(rec, r)
	if rec.Code != http.StatusNoContent {
		t.Errorf("case-insensitive host: want pass, got %d", rec.Code)
	}
}

func TestOriginIPv6(t *testing.T) {
	// Host "[::1]:7878" with Origin "http://[::1]:7878" should pass
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "http://[::1]:7878/api/login", nil)
	r.Host = "[::1]:7878"
	r.Header.Set("Origin", "http://[::1]:7878")
	originChecker{}.wrap(ok).ServeHTTP(rec, r)
	if rec.Code != http.StatusNoContent {
		t.Errorf("IPv6 with port: want pass, got %d", rec.Code)
	}
}

func TestOriginDefaultPortHTTP(t *testing.T) {
	// Host "example.com:80" with Origin "http://example.com" should pass
	// because :80 is the default port for http
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "http://example.com:80/api/login", nil)
	r.Host = "example.com:80"
	r.Header.Set("Origin", "http://example.com")
	originChecker{}.wrap(ok).ServeHTTP(rec, r)
	if rec.Code != http.StatusNoContent {
		t.Errorf("default port HTTP: want pass, got %d", rec.Code)
	}
}

func TestOriginDefaultPortHTTPS(t *testing.T) {
	// TLS + Host "example.com:443" with Origin "https://example.com" should pass
	// because :443 is the default port for https
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "https://example.com:443/api/login", nil)
	r.Host = "example.com:443"
	r.Header.Set("Origin", "https://example.com")
	originChecker{tls: true}.wrap(ok).ServeHTTP(rec, r)
	if rec.Code != http.StatusNoContent {
		t.Errorf("default port HTTPS: want pass, got %d", rec.Code)
	}
}

func TestRequireSessionInvalidCookie(t *testing.T) {
	s := auth.NewSessions(auth.SessionTTL, time.Now)
	h := requireSession(s, ok)

	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/meta", nil)
	r.AddCookie(&http.Cookie{Name: cookieName, Value: "invalid_session_id"})
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized || errCode(t, rec) != "unauthorized" {
		t.Fatalf("invalid cookie: got %d", rec.Code)
	}
}

func TestCacheControlHeader(t *testing.T) {
	// Cache-Control: no-store should be set for /api/* paths
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/containers", nil)
	securityHeaders(ok, nil).ServeHTTP(rec, r)
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control on /api/: want no-store, got %q", got)
	}

	// Cache-Control should not be set for non-/api paths
	rec = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/", nil)
	securityHeaders(ok, nil).ServeHTTP(rec, r)
	if got := rec.Header().Get("Cache-Control"); got != "" {
		t.Errorf("Cache-Control on /: want empty, got %q", got)
	}
}

func TestOriginIPv6DefaultPortHTTP(t *testing.T) {
	// Host "[::1]:80" with Origin "http://[::1]" should pass
	// because :80 is the default port for http (brackets must be preserved)
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "http://[::1]:80/api/login", nil)
	r.Host = "[::1]:80"
	r.Header.Set("Origin", "http://[::1]")
	originChecker{}.wrap(ok).ServeHTTP(rec, r)
	if rec.Code != http.StatusNoContent {
		t.Errorf("IPv6 default port HTTP: want pass, got %d", rec.Code)
	}
}

func TestOriginIPv6DefaultPortHTTPS(t *testing.T) {
	// TLS + Host "[::1]:443" with Origin "https://[::1]" should pass
	// because :443 is the default port for https (brackets must be preserved)
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "https://[::1]:443/api/login", nil)
	r.Host = "[::1]:443"
	r.Header.Set("Origin", "https://[::1]")
	originChecker{tls: true}.wrap(ok).ServeHTTP(rec, r)
	if rec.Code != http.StatusNoContent {
		t.Errorf("IPv6 default port HTTPS: want pass, got %d", rec.Code)
	}
}

func TestOriginIPv6NonDefaultPort(t *testing.T) {
	// Host "[::80]" with Origin "http://[::80]" should pass
	// (::80 is an IPv6 address, not a port; :80 is not present to strip)
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "http://[::80]/api/login", nil)
	r.Host = "[::80]"
	r.Header.Set("Origin", "http://[::80]")
	originChecker{}.wrap(ok).ServeHTTP(rec, r)
	if rec.Code != http.StatusNoContent {
		t.Errorf("IPv6 non-default port: want pass, got %d", rec.Code)
	}
}
