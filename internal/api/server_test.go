package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/audit"
	"github.com/levy-cmd/hublot-kit/internal/auth"
	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

type fixture struct {
	h        http.Handler
	token    string
	auditP   string
	auditLog *audit.Log    // closed by a test to make audit writes fail
	docker   *dockerx.Fake // nil when the fixture talks to a real Docker
	// File routes: OpenFS returns fs (or openErr) and records the ID it got.
	fs       *fsview.MemFS
	views    *fsview.Views // nil: the default of New
	openErr  error
	mu       sync.Mutex // OpenFS runs in the guard's goroutines
	openedID string
}

// opened returns the ID the last OpenFS call received ("" if none).
func (f *fixture) opened() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.openedID
}

// webID is the full ID of the fake "web" container of newFixture.
const webID = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func newFixture(t *testing.T, readOnly bool) *fixture {
	t.Helper()
	web := dockerx.ContainerInfo{ID: webID, Name: "web", Running: true, Pid: 42}
	fake := &dockerx.Fake{
		Containers: []dockerx.Container{{ID: "abc", Name: "web", Image: "nginx", State: "running", Ports: []dockerx.Port{}}},
		Infos:      map[string]dockerx.ContainerInfo{"web": web, webID: web},
		Archives:   map[string][]byte{},
	}
	f := &fixture{docker: fake, fs: fsview.NewMemFS()}
	f.init(t, readOnly, fake, func(_ context.Context, id string) (fsview.FS, error) {
		f.mu.Lock()
		f.openedID = id
		f.mu.Unlock()
		if f.openErr != nil {
			return nil, f.openErr
		}
		return f.fs, nil
	})
	return f
}

// init builds the handler; openFS nil means the real fsview.For.
func (f *fixture) init(t *testing.T, readOnly bool, docker dockerx.Client, openFS func(context.Context, string) (fsview.FS, error)) {
	t.Helper()
	dir := t.TempDir()
	tok, _, err := auth.EnsureToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	v, err := auth.LoadVerifier(dir)
	if err != nil {
		t.Fatal(err)
	}
	ap := filepath.Join(dir, "audit.log")
	al, err := audit.Open(ap)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { al.Close() })
	f.h = New(Deps{
		Docker: docker, Views: f.views, OpenFS: openFS, Verifier: v, Audit: al, Version: "test", ReadOnly: readOnly,
		Sessions: auth.NewSessions(auth.SessionTTL, time.Now),
		Limiter:  auth.NewLimiter(5, time.Minute, time.Now),
	})
	f.token, f.auditP, f.auditLog = tok, ap, al
}

func (f *fixture) do(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://example.com"+path, strings.NewReader(body))
	r.RemoteAddr = "192.0.2.1:40000"
	if method != "GET" {
		r.Header.Set("Origin", "http://example.com")
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, r)
	return rec
}

func (f *fixture) login(t *testing.T, token string) (*httptest.ResponseRecorder, *http.Cookie) {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"token": token})
	rec := f.do("POST", "/api/login", string(b), nil)
	for _, c := range rec.Result().Cookies() {
		if c.Name == cookieName {
			return rec, c
		}
	}
	return rec, nil
}

func TestLoginSuccessSetsStrictCookie(t *testing.T) {
	f := newFixture(t, false)
	rec, c := f.login(t, f.token)
	if rec.Code != http.StatusOK || c == nil {
		t.Fatalf("login: %d %s", rec.Code, rec.Body)
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.Secure {
		t.Fatalf("cookie flags: %+v", c)
	}
}

func TestLoginTrimsToken(t *testing.T) {
	f := newFixture(t, false)
	if rec, c := f.login(t, "  "+f.token+"\n"); rec.Code != http.StatusOK || c == nil {
		t.Fatalf("pasted token with whitespace must work: %d", rec.Code)
	}
}

func TestLoginFailureIsAuditedAndLimited(t *testing.T) {
	f := newFixture(t, false)
	for i := 0; i < 5; i++ {
		if rec, _ := f.login(t, "mauvais"); rec.Code != http.StatusUnauthorized || errCode(t, rec) != "unauthorized" {
			t.Fatalf("attempt %d: %d", i+1, rec.Code)
		}
	}
	if rec, _ := f.login(t, f.token); rec.Code != http.StatusTooManyRequests || errCode(t, rec) != "rate_limited" {
		t.Fatalf("6th attempt: %d", rec.Code)
	}
	fh, err := os.Open(f.auditP)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	n := 0
	sc := bufio.NewScanner(fh)
	for sc.Scan() {
		var e audit.Event
		json.Unmarshal(sc.Bytes(), &e)
		if e.Action == "login.fail" && e.Remote == "192.0.2.1" && e.Result == "refused" {
			n++
		}
	}
	if n != 5 {
		t.Fatalf("want 5 login.fail lines, got %d", n)
	}
}

func TestContainersNeedsSession(t *testing.T) {
	f := newFixture(t, false)
	if rec := f.do("GET", "/api/containers", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rec.Code)
	}
	_, c := f.login(t, f.token)
	rec := f.do("GET", "/api/containers", "", c)
	var list []dockerx.Container
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &list) != nil || len(list) != 1 || list[0].Name != "web" {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
}

func TestContainersDockerDown(t *testing.T) {
	f := newFixture(t, false)
	f.docker.Err = errors.New("dial unix /var/run/docker.sock: connect: no such file or directory")
	_, c := f.login(t, f.token)
	rec := f.do("GET", "/api/containers", "", c)
	if rec.Code != http.StatusServiceUnavailable || errCode(t, rec) != "docker_unavailable" {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}

func TestContainersEmptyIsArray(t *testing.T) {
	f := newFixture(t, false)
	f.docker.Containers = nil
	_, c := f.login(t, f.token)
	if body := strings.TrimSpace(f.do("GET", "/api/containers", "", c).Body.String()); body != "[]" {
		t.Fatalf("want [], got %s", body)
	}
}

func TestMetaAndLogout(t *testing.T) {
	f := newFixture(t, true)
	_, c := f.login(t, f.token)
	rec := f.do("GET", "/api/meta", "", c)
	var m struct {
		Version  string `json:"version"`
		ReadOnly bool   `json:"readOnly"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &m) != nil || m.Version != "test" || !m.ReadOnly {
		t.Fatalf("meta: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do("POST", "/api/logout", "", c); rec.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", rec.Code)
	}
	if rec := f.do("GET", "/api/meta", "", c); rec.Code != http.StatusUnauthorized {
		t.Fatalf("after logout: %d", rec.Code)
	}
}

func TestUnknownAPIRouteIsJSON404(t *testing.T) {
	f := newFixture(t, false)
	rec := f.do("GET", "/api/nope", "", nil)
	if rec.Code != http.StatusNotFound || errCode(t, rec) != "not_found" {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}

func TestLoginRequiresOrigin(t *testing.T) {
	f := newFixture(t, false)
	r := httptest.NewRequest("POST", "http://example.com/api/login", strings.NewReader(`{"token":"`+f.token+`"}`))
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, r)
	if rec.Code != http.StatusForbidden || errCode(t, rec) != "forbidden_origin" {
		t.Fatalf("got %d", rec.Code)
	}
}

func cookieOf(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == cookieName {
			return c
		}
	}
	return nil
}

func TestLoginCookieMaxAge(t *testing.T) {
	f := newFixture(t, false)
	_, c := f.login(t, f.token)
	if c == nil || c.MaxAge != int(auth.SessionTTL.Seconds()) {
		t.Fatalf("cookie: %+v", c)
	}
}

func TestLogoutClearsCookieAndIsIdempotent(t *testing.T) {
	f := newFixture(t, false)
	rec := f.do("POST", "/api/logout", "", &http.Cookie{Name: cookieName, Value: "inconnu"})
	c := cookieOf(rec)
	if rec.Code != http.StatusNoContent || c == nil || c.MaxAge >= 0 {
		t.Fatalf("logout: %d %+v", rec.Code, c)
	}
	rec = f.do("POST", "/api/logout", "", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout without cookie: %d", rec.Code)
	}
}

func TestLoginTrailingGarbageRejected(t *testing.T) {
	f := newFixture(t, false)
	rec := f.do("POST", "/api/login", `{"token":"`+f.token+`"}garbage`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestRateLimitedIsAudited(t *testing.T) {
	f := newFixture(t, false)
	for i := 0; i < 6; i++ {
		f.login(t, "mauvais")
	}
	data, _ := os.ReadFile(f.auditP)
	if !strings.Contains(string(data), `"result":"rate_limited"`) {
		t.Fatalf("no rate_limited line: %s", data)
	}
}

func TestSecureCookieBehindTLS(t *testing.T) {
	for _, tc := range []struct {
		name   string
		tls    bool
		public string
		origin string
	}{
		{"tls", true, "", "https://example.com"},
		{"public https", false, "https://x.example", "https://x.example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tok, _, _ := auth.EnsureToken(dir)
			v, _ := auth.LoadVerifier(dir)
			al, err := audit.Open(filepath.Join(dir, "audit.log"))
			if err != nil {
				t.Fatal(err)
			}
			defer al.Close()
			h := New(Deps{Docker: &dockerx.Fake{}, Verifier: v, Audit: al, TLS: tc.tls, PublicOrigin: tc.public,
				Sessions: auth.NewSessions(auth.SessionTTL, time.Now), Limiter: auth.NewLimiter(5, time.Minute, time.Now)})
			r := httptest.NewRequest("POST", "http://example.com/api/login", strings.NewReader(`{"token":"`+tok+`"}`))
			r.Header.Set("Origin", tc.origin)
			if tc.tls {
				r = httptest.NewRequest("POST", "https://example.com/api/login", strings.NewReader(`{"token":"`+tok+`"}`))
				r.Header.Set("Origin", tc.origin)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			c := cookieOf(rec)
			if rec.Code != http.StatusOK || c == nil || !c.Secure {
				t.Fatalf("%d %+v", rec.Code, c)
			}
		})
	}
}

func TestRateLimitedAuditThrottled(t *testing.T) {
	now := time.Now()
	logClock = func() time.Time { return now }
	t.Cleanup(func() { logClock = time.Now })
	f := newFixture(t, false)
	for i := 0; i < 5; i++ {
		f.login(t, "mauvais")
	}
	count := func() int {
		data, _ := os.ReadFile(f.auditP)
		return strings.Count(string(data), `"result":"rate_limited"`)
	}
	for i := 0; i < 10; i++ {
		f.login(t, "mauvais")
	}
	if n := count(); n != 1 {
		t.Fatalf("want 1 line, got %d", n)
	}
	now = now.Add(61 * time.Second)
	f.login(t, "mauvais")
	if n := count(); n != 2 {
		t.Fatalf("want 2 lines after a minute, got %d", n)
	}
}

func TestCSPCarriesScriptHashesFromDeps(t *testing.T) {
	h := New(Deps{Docker: &dockerx.Fake{}, ScriptHashes: []string{"'sha256-Zm9v'"}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/nope", nil))
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self' 'sha256-Zm9v';") {
		t.Fatalf("CSP %q lacks the hash", csp)
	}
}

func TestThrottleLogSweepsAtMostOncePerWindow(t *testing.T) {
	old := throttleSweepThreshold
	throttleSweepThreshold = 10
	t.Cleanup(func() { throttleSweepThreshold = old })
	t0 := time.Now()
	now := t0
	tl := &throttleLog{last: map[string]time.Time{}, now: func() time.Time { return now }}
	add := func(prefix string, n int) {
		for i := 0; i < n; i++ {
			tl.shouldLog(fmt.Sprintf("%s%d", prefix, i))
		}
	}
	add("a", 12) // sweep at t0
	now = t0.Add(5 * time.Second)
	add("b", 11) // no sweep: last one was 5 s ago
	now = t0.Add(61 * time.Second)
	add("c", 1) // sweep: a* expire
	if _, ok := tl.last["a0"]; ok {
		t.Fatal("a0 should have been swept")
	}
	now = t0.Add(66 * time.Second) // b* expired, but the last sweep was 5 s ago
	add("d", 1)
	if _, ok := tl.last["b0"]; !ok {
		t.Fatal("b0 swept again within the same window")
	}
	now = t0.Add(122 * time.Second)
	add("e", 1)
	if _, ok := tl.last["b0"]; ok {
		t.Fatal("b0 should have been swept one window later")
	}
}
