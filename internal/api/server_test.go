package api

import (
	"bufio"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/audit"
	"github.com/levy-cmd/hublot-kit/internal/auth"
	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

type fixture struct {
	h      http.Handler
	token  string
	auditP string
	docker *dockerx.Fake
}

func newFixture(t *testing.T, readOnly bool) *fixture {
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
	fake := &dockerx.Fake{Containers: []dockerx.Container{{ID: "abc", Name: "web", Image: "nginx", State: "running", Ports: []dockerx.Port{}}}}
	h := New(Deps{
		Docker: fake, Verifier: v, Audit: al, Version: "test", ReadOnly: readOnly,
		Sessions: auth.NewSessions(auth.SessionTTL, time.Now),
		Limiter:  auth.NewLimiter(5, time.Minute, time.Now),
	})
	return &fixture{h: h, token: tok, auditP: ap, docker: fake}
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
