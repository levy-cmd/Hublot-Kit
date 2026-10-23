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

// v0.1.2 (audit, constat 3): 1 040 login.fail lines in 2 minutes. Across
// every IP, at most loginLogMax lines a minute, then one line says the rest
// is not written.
func TestLoginFailLogIsBounded(t *testing.T) {
	now := time.Now()
	logClock = func() time.Time { return now }
	t.Cleanup(func() { logClock = time.Now })
	f := newFixture(t, false)
	fail := func(ip string) {
		r := httptest.NewRequest("POST", "http://example.com/api/login", strings.NewReader(`{"token":"mauvais"}`))
		r.RemoteAddr = ip + ":1"
		r.Header.Set("Origin", "http://example.com")
		f.h.ServeHTTP(httptest.NewRecorder(), r)
	}
	for i := 0; i < 300; i++ {
		fail(fmt.Sprintf("10.0.%d.%d", i/200, i%200))
	}
	for i := 0; i < 10; i++ {
		fail("10.9.9.9") // rate_limited after 5
	}
	count := func() (lines, suppressed int) {
		data, _ := os.ReadFile(f.auditP)
		for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if strings.Contains(l, `"action":"login.fail"`) {
				lines++
				if strings.Contains(l, `"result":"suppressed"`) {
					suppressed++
				}
			}
		}
		return
	}
	lines, suppressed := count()
	if lines != loginLogMax+1 || suppressed != 1 {
		t.Fatalf("%d lignes login.fail dont %d suppressed, attendu %d + 1", lines, suppressed, loginLogMax)
	}
	now = now.Add(61 * time.Second)
	fail("10.8.8.8")
	if l, s := count(); l != lines+1 || s != 1 {
		t.Fatalf("minute suivante : %d lignes (%d suppressed), attendu %d", l, s, lines+1)
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
