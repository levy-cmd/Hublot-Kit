package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
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

// v0.1.2 (audit, constat 3): the attempt is taken before the body is read,
// atomically. 40 logins of one IP in one burst, their bodies held back:
// 35 get rate_limited at once, only 5 tokens are ever checked.
func TestLoginReservesAttemptBeforeReadingBody(t *testing.T) {
	f := newFixture(t, false)
	const n = 40
	done := make(chan int, n)
	var bodies []*io.PipeWriter
	t.Cleanup(func() {
		for _, pw := range bodies {
			pw.CloseWithError(io.ErrClosedPipe)
		}
	})
	for i := 0; i < n; i++ {
		pr, pw := io.Pipe()
		bodies = append(bodies, pw)
		r := httptest.NewRequest("POST", "http://example.com/api/login", pr)
		r.RemoteAddr = "192.0.2.9:40000"
		r.Header.Set("Origin", "http://example.com")
		go func() {
			rec := httptest.NewRecorder()
			f.h.ServeHTTP(rec, r)
			done <- rec.Code
		}()
	}
	codes := map[int]int{}
	timeout := time.After(3 * time.Second)
wait:
	for codes[http.StatusTooManyRequests] < n-5 {
		select {
		case c := <-done:
			codes[c]++
		case <-timeout:
			break wait
		}
	}
	if codes[http.StatusTooManyRequests] != n-5 || len(codes) != 1 {
		t.Fatalf("avant tout corps : %v, attendu %d rate_limited (5 tentatives réservées)", codes, n-5)
	}
	for _, pw := range bodies {
		go func() { pw.Write([]byte(`{"token":"mauvais"}`)); pw.Close() }()
	}
	for i := 0; i < 5; i++ {
		select {
		case c := <-done:
			codes[c]++
		case <-time.After(3 * time.Second):
			t.Fatalf("les 5 tentatives réservées ne répondent pas : %v", codes)
		}
	}
	if codes[http.StatusUnauthorized] != 5 {
		t.Fatalf("%v, attendu 5 unauthorized", codes)
	}
	data, _ := os.ReadFile(f.auditP)
	if got := strings.Count(string(data), `"result":"refused"`); got != 5 {
		t.Fatalf("%d lignes refused, attendu 5", got)
	}
}

// The attempt taken by a good token is given back: it does not eat into the
// limit of the IP.
func TestLoginSuccessGivesAttemptBack(t *testing.T) {
	f := newFixture(t, false)
	for i := 0; i < 4; i++ {
		f.login(t, "mauvais")
	}
	if rec, c := f.login(t, f.token); rec.Code != http.StatusOK || c == nil {
		t.Fatalf("bon jeton après 4 échecs : %d", rec.Code)
	}
	if rec, _ := f.login(t, "mauvais"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("5e échec : %d, attendu 401 (la tentative réussie est rendue)", rec.Code)
	}
	if rec, _ := f.login(t, "mauvais"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("6e échec : %d, attendu 429", rec.Code)
	}
}

// v0.1.2 (audit, constat 3, counter-audit): the login.fail lines are bounded
// per address block (an IPv4 address, an IPv6 /64), never for every IP
// together: one attacker cannot hide the failures of another. Past the
// budget, one "suppressed" line for the block; the next minute, one line
// counts what was not written.
func TestLoginFailLogPerBlock(t *testing.T) {
	now := time.Now()
	logClock = func() time.Time { return now }
	t.Cleanup(func() { logClock = time.Now })
	f := newFixture(t, false)
	fail := func(remote string) {
		r := httptest.NewRequest("POST", "http://example.com/api/login", strings.NewReader(`{"token":"mauvais"}`))
		r.RemoteAddr = remote
		r.Header.Set("Origin", "http://example.com")
		f.h.ServeHTTP(httptest.NewRecorder(), r)
	}
	for i := 1; i <= 200; i++ { // 200 addresses of one /64: each gets its own 5 attempts
		fail(fmt.Sprintf("[2001:db8:1:2::%x]:1", i))
	}
	fail("192.0.2.77:1") // another attacker, or a user who mistyped
	lines := func() []audit.Event {
		data, _ := os.ReadFile(f.auditP)
		var out []audit.Event
		for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			var e audit.Event
			if json.Unmarshal([]byte(l), &e) == nil && e.Action == "login.fail" {
				out = append(out, e)
			}
		}
		return out
	}
	block, suppressed, other := 0, 0, 0
	for _, e := range lines() {
		switch {
		case e.Result == "suppressed":
			suppressed++
		case strings.HasPrefix(e.Remote, "2001:db8:1:2:"):
			block++
		case e.Remote == "192.0.2.77":
			other++
		}
	}
	if block != loginLogPerBlock || suppressed != 1 || other != 1 {
		t.Fatalf("bloc %d lignes (attendu %d), suppressed %d (attendu 1), autre IP %d (attendu 1)", block, loginLogPerBlock, suppressed, other)
	}
	now = now.Add(61 * time.Second)
	fail("192.0.2.78:1")
	var summary *audit.Event
	for _, e := range lines() {
		if e.Result == "suppressed" && e.Remote == "" {
			summary = &e
		}
	}
	if want := fmt.Sprint(200 - loginLogPerBlock); summary == nil || !strings.Contains(summary.Error, want) {
		t.Fatalf("synthèse de la minute passée : %+v, attendu %s échecs non écrits", summary, want)
	}
}

// v0.1.2 (audit, constat 4): with DOCKER_HOST=tcp://user:password@…, the
// daemon's error names the password. It never reaches a response (fixed
// detail) nor the agent's log.
func TestDockerErrorsGiveFixedDetail(t *testing.T) {
	var logs bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	const secret = "s3cr3t-pw"
	msg := "Cannot connect to the Docker daemon at tcp://alice:" + secret + "@10.0.0.1:2375. Is the docker daemon running?"
	f := newFixture(t, false)
	c := f.session(t)
	check := func(what string, rec *httptest.ResponseRecorder) {
		t.Helper()
		if b := rec.Body.String(); strings.Contains(b, secret) || strings.Contains(b, "tcp://") {
			t.Errorf("%s : %d %s", what, rec.Code, b)
		}
	}
	f.docker.Err = errors.New(msg) // as the client gives it
	check("/api/containers (erreur brute)", f.do("GET", "/api/containers", "", c))
	for _, err := range []error{
		fmt.Errorf("%w: %s", dockerx.ErrUnavailable, msg),
		fmt.Errorf("%w: %s", dockerx.ErrRefused, msg),
		fmt.Errorf("%w: %s", dockerx.ErrPutInvalid, msg),
		fmt.Errorf("%w: %s", dockerx.ErrNoLogs, msg),
	} {
		f.docker.Err = err
		for _, p := range []string{"/api/containers", "/api/containers/web", "/api/containers/web/logs",
			"/api/containers/web/logs/download", "/api/containers/web/changes"} {
			check(p, f.do("GET", p, "", c))
		}
		check("env/reveal", f.do("POST", "/api/containers/web/env/reveal", `{"name":"X"}`, c))
	}
	// Past the first byte the reader of the logs fails: the stream reports it.
	f.docker.Err = nil
	f.docker.LogsErr = fmt.Errorf("%w: %s", dockerx.ErrUnavailable, msg)
	check("logs (Logs)", f.do("GET", "/api/containers/web/logs", "", c))
	check("logs/download (Logs)", f.do("GET", "/api/containers/web/logs/download", "", c))
	if strings.Contains(logs.String(), secret) {
		t.Errorf("journal de l'agent : %s", logs.String())
	}
}

// v0.1.2 (audit, constat 2): only ReadHeaderTimeout was set. The agent's
// server (ConfigureServer) bounds the reading of a body and the idle
// keep-alive, never the writing (logs stream, long downloads).
func TestConfigureServer(t *testing.T) {
	var srv http.Server
	ConfigureServer(&srv)
	if srv.ReadHeaderTimeout <= 0 || srv.ReadTimeout <= 0 || srv.IdleTimeout <= 0 || srv.WriteTimeout != 0 {
		t.Fatalf("délais : en-têtes %s, lecture %s, inactivité %s, écriture %s", srv.ReadHeaderTimeout, srv.ReadTimeout, srv.IdleTimeout, srv.WriteTimeout)
	}
}

// serve starts f.h on a real server configured by ConfigureServer, with
// short delays.
func (f *fixture) serve(t *testing.T, read time.Duration) string {
	t.Helper()
	oldRead, oldIdle := readTimeout, idleTimeout
	readTimeout, idleTimeout = read, read
	t.Cleanup(func() { readTimeout, idleTimeout = oldRead, oldIdle })
	ts := httptest.NewUnstartedServer(f.h)
	ConfigureServer(ts.Config)
	ts.Start()
	t.Cleanup(ts.Close)
	return ts.Listener.Addr().String()
}

// closedWithin: the server closes conn (EOF or reset) within limit.
func closedWithin(conn net.Conn, limit time.Duration) bool {
	conn.SetReadDeadline(time.Now().Add(limit))
	_, err := io.Copy(io.Discard, conn)
	var ne net.Error
	return !(errors.As(err, &ne) && ne.Timeout())
}

// A body that never comes (the audit held 1 000 connections for 25 s with
// POST /api/login and Content-Length: 100), an idle keep-alive: the
// connection is closed.
func TestSlowClientsAreCut(t *testing.T) {
	f := newFixture(t, false)
	addr := f.serve(t, 300*time.Millisecond)
	for _, head := range []string{
		"POST /api/login HTTP/1.1\r\nHost: example.com\r\nOrigin: http://example.com\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n",
		// Without a session, the upload routes keep the short delay.
		"PUT /api/containers/web/fs/file?path=/x HTTP/1.1\r\nHost: example.com\r\nOrigin: http://example.com\r\nContent-Length: 100\r\n\r\n",
		"POST /api/containers/web/fs/upload?dir=/ HTTP/1.1\r\nHost: example.com\r\nOrigin: http://example.com\r\nContent-Length: 100\r\n\r\n",
		"GET /api/meta HTTP/1.1\r\nHost: example.com\r\nContent-Length: 100\r\n\r\n",
		"GET /api/meta HTTP/1.1\r\nHost: example.com\r\n\r\n", // answered, then idle
	} {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		conn.Write([]byte(head))
		if !closedWithin(conn, 3*time.Second) {
			t.Errorf("connexion encore ouverte après 3 s : %q", strings.SplitN(head, "\r\n", 2)[0])
		}
		conn.Close()
	}
}

// The read delay ends with the request's reading: a logs stream lives past
// it (net/http lifts it for its background read; guard against a delay set
// by hand, which would cancel the stream's context).
func TestLogsStreamOutlivesReadTimeout(t *testing.T) {
	f := newFixture(t, false)
	f.docker.LogStreams = map[string][]byte{webID: nil}
	f.docker.LogsGate = make(chan struct{})
	c := f.session(t)
	addr := f.serve(t, 200*time.Millisecond)
	req, _ := http.NewRequest("GET", "http://"+addr+"/api/containers/web/logs", nil)
	req.AddCookie(c)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	ended := make(chan error, 1)
	go func() { _, err := io.Copy(io.Discard, resp.Body); ended <- err }()
	select {
	case err := <-ended:
		t.Fatalf("flux terminé avant 1 s (%v) : le délai de lecture l'a coupé", err)
	case <-time.After(time.Second):
	}
	resp.Body.Close()
}

// A logged-in edit whose body comes slowly is read whole: the session
// extends the delay of the upload routes.
func TestSlowEditWithSessionIsRead(t *testing.T) {
	f, c := writeFixture(t, false)
	tag := f.do("GET", "/api/containers/web/fs/file?path=/etc/app.conf", "", c).Header().Get("ETag")
	addr := f.serve(t, 300*time.Millisecond)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	body := "10.0.0.1\n"
	fmt.Fprintf(conn, "PUT /api/containers/web/fs/file?path=/etc/app.conf HTTP/1.1\r\nHost: example.com\r\nOrigin: http://example.com\r\n"+
		"Cookie: %s=%s\r\nIf-Match: %s\r\nContent-Length: %d\r\n\r\n", c.Name, c.Value, tag, len(body))
	for _, part := range []string{body[:3], body[3:6], body[6:]} {
		time.Sleep(250 * time.Millisecond) // 750 ms in all, past the 300 ms delay
		conn.Write([]byte(part))
	}
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("pas de réponse : %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("édition lente : %d %s", resp.StatusCode, b)
	}
}

// stallWrites shortens writeStallTimeout for one test.
func stallWrites(t *testing.T, d time.Duration) {
	t.Helper()
	old := writeStallTimeout
	writeStallTimeout = d
	t.Cleanup(func() { writeStallTimeout = old })
}

// stalledGet sends a GET with the session cookie, reads the first bytes of
// the answer, then never reads again (a download paused in the browser). The
// small receive buffer fills fast.
func stalledGet(t *testing.T, addr, target string, c *http.Cookie) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	conn.(*net.TCPConn).SetReadBuffer(4096)
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: example.com\r\nCookie: %s=%s\r\n\r\n", target, c.Name, c.Value)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(conn, make([]byte, 512)); err != nil {
		t.Fatalf("%s : %v", target, err)
	}
	return conn
}

// v0.1.2 (counter-audit, constat 1): io.Copy of a download had no write
// deadline. Four downloads whose client stopped reading held the four slots
// of the container, and the listing answered 503 (on a stopped container,
// dockerd's lock stayed taken). A write that makes no progress for
// writeStallTimeout ends the download and frees its slot.
func TestStalledDownloadsFreeTheirSlots(t *testing.T) {
	stallWrites(t, 300*time.Millisecond)
	f, c := writeFixture(t, false)
	f.fs.AddFile("/big", bytes.Repeat([]byte("x"), 64<<20))
	addr := f.serve(t, 5*time.Second)
	for i := 0; i < fsMaxPerContainer; i++ {
		stalledGet(t, addr, "/api/containers/web/fs/download?path=/big", c)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		rec := f.do("GET", "/api/containers/web/fs?path=/", "", c)
		if rec.Code == http.StatusOK {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("créneaux jamais rendus : %d %s", rec.Code, rec.Body)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// waitCloses waits until Docker's logs stream was closed n times.
func waitCloses(t *testing.T, f *fixture, n int32, what string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for f.docker.LogsCloses.Load() < n {
		if time.Now().After(deadline) {
			t.Fatalf("%s : flux de Docker jamais fermé (%d fermetures)", what, f.docker.LogsCloses.Load())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// bigLogs: n lines of size bytes, in Docker's multiplexed frames (stdout).
func bigLogs(n, size int) []byte {
	line := append([]byte("2026-10-23T10:00:00.000000000Z "), bytes.Repeat([]byte("l"), size)...)
	line = append(line, '\n')
	frame := append([]byte{1, 0, 0, 0, byte(len(line) >> 24), byte(len(line) >> 16), byte(len(line) >> 8), byte(len(line))}, line...)
	return bytes.Repeat(frame, n)
}

// The logs download and the live stream: a client that stops reading keeps
// neither a slot nor Docker's stream (the gate holds the stream open, as a
// container that keeps running).
func TestStalledLogsEnd(t *testing.T) {
	stallWrites(t, 300*time.Millisecond)
	f := newFixture(t, false)
	f.docker.LogStreams = map[string][]byte{webID: bigLogs(4000, 16000)}
	f.docker.LogsGate = make(chan struct{})
	t.Cleanup(func() { close(f.docker.LogsGate) })
	c := f.session(t)
	addr := f.serve(t, 5*time.Second)
	stalledGet(t, addr, "/api/containers/web/logs/download", c)
	waitCloses(t, f, 1, "téléchargement des logs")
	stalledGet(t, addr, "/api/containers/web/logs", c)
	waitCloses(t, f, 2, "flux SSE")
}
