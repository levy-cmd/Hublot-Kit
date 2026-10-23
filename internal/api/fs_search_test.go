package api

// Tests of GET /api/containers/{id}/fs/search (spec v0.2 §5.8, Q61),
// written before the code: the file search reads only the index already
// served to the explorer, never builds one, is bounded, and leaves no
// trace of the query.

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

type searchJSON struct {
	Indexed bool `json:"indexed"`
	Results []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	} `json:"results"`
	Truncated bool `json:"truncated"`
}

// search posts {"q": q} to the search of container id (review of tranche
// 3, Codex: the query is in the body, never in the URL that a reverse proxy
// logs). f.do adds the Origin of every non-GET request.
func search(f *fixture, id, q string, c *http.Cookie) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"q": q})
	return f.do("POST", "/api/containers/"+id+"/fs/search", string(body), c)
}

// serveIndex polls the explorer until the index of db is built and served.
func serveIndex(t *testing.T, f *fixture, c *http.Cookie) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		if r := f.do("GET", "/api/containers/db/fs?path=/etc", "", c); r.Code == http.StatusOK {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("index jamais prêt")
		}
	}
}

// Spec v0.2 §5.8: a query of 2 to 200 characters, else 400 invalid_query,
// checked before anything is asked of Docker.
func TestFsSearchQueryBounds(t *testing.T) {
	fake := stoppedFake(t)
	f, c := stoppedFixture(t, fake, fake, fsview.Options{})
	if rec := search(f, "db", "host", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("sans session : %d", rec.Code)
	}
	for _, q := range []string{"", "a", "é", strings.Repeat("a", 201), strings.Repeat("é", 201), "a\x00b"} {
		rec := search(f, "db", q, c)
		if rec.Code != http.StatusBadRequest || errCode(t, rec) != "invalid_query" {
			t.Errorf("q de %d caractères : %d %s", len([]rune(q)), rec.Code, rec.Body)
		}
		if len(q) > 1 && strings.Contains(rec.Body.String(), q) {
			t.Errorf("la réponse répète la requête : %s", rec.Body)
		}
	}
	for _, body := range []string{"", "{", `{"q": 12}`, `["ab"]`, `{"q":"` + strings.Repeat("a", 5000) + `"}`} {
		if rec := f.do("POST", "/api/containers/db/fs/search", body, c); rec.Code != http.StatusBadRequest || errCode(t, rec) != "invalid_query" {
			t.Errorf("corps %.20q : %d %s", body, rec.Code, rec.Body)
		}
	}
	if calls := fake.Calls(); len(calls) != 0 {
		t.Errorf("requête refusée, Docker sollicité : %v", calls)
	}
	for _, q := range []string{"ab", "éé", strings.Repeat("a", 200), strings.Repeat("é", 200)} {
		if rec := search(f, "db", q, c); rec.Code != http.StatusOK {
			t.Errorf("q de %d caractères : %d %s", len([]rune(q)), rec.Code, rec.Body)
		}
	}
	if rec := search(f, "absent", "host", c); rec.Code != http.StatusNotFound || errCode(t, rec) != "not_found" {
		t.Errorf("conteneur absent : %d %s", rec.Code, rec.Body)
	}
	if rec := search(f, "..x", "host", c); rec.Code != http.StatusNotFound {
		t.Errorf("identifiant invalide : %d", rec.Code)
	}
}

// Review of tranche 3 (Codex): the query never goes in a URL. The route
// is a POST, under the Origin check of every POST (spec §7); a GET, with or
// without ?q=, is not served.
func TestFsSearchPostOnly(t *testing.T) {
	fake := stoppedFake(t)
	f, c := stoppedFixture(t, fake, fake, fsview.Options{})
	for _, u := range []string{"/api/containers/db/fs/search", "/api/containers/db/fs/search?q=host"} {
		// 404 of the /api/ fallback: no GET route under this path.
		if rec := f.do("GET", u, "", c); rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "indexed") {
			t.Errorf("GET %s : %d %s", u, rec.Code, rec.Body)
		}
	}
	for _, origin := range []string{"", "http://evil.example"} {
		r := httptest.NewRequest("POST", "http://example.com/api/containers/db/fs/search", strings.NewReader(`{"q":"host"}`))
		r.RemoteAddr = "192.0.2.1:40000"
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		r.AddCookie(c)
		rec := httptest.NewRecorder()
		f.h.ServeHTTP(rec, r)
		if rec.Code != http.StatusForbidden || errCode(t, rec) != "forbidden_origin" {
			t.Errorf("Origin %q : %d %s", origin, rec.Code, rec.Body)
		}
	}
	if rec := search(f, "db", "host", c); rec.Code != http.StatusOK {
		t.Errorf("POST : %d %s", rec.Code, rec.Body)
	}
}

// Q61: without an index already served, {"indexed": false} and nothing is
// exported; once the explorer has served it, the search reads it.
func TestFsSearchNeverBuilds(t *testing.T) {
	fake := stoppedFake(t)
	f, c := stoppedFixture(t, fake, fake, fsview.Options{})
	rec := search(f, "db", "host", c)
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != `{"indexed":false,"results":[],"truncated":false}` {
		t.Fatalf("sans index : %d %s", rec.Code, got)
	}
	if calls := fake.Calls(); len(calls) != 0 {
		t.Fatalf("EXPORT : la recherche a lancé %v", calls)
	}
	serveIndex(t, f, c)
	rec = search(f, "db", "HOST", c)
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK ||
		got != `{"indexed":true,"results":[{"path":"/etc/hostname","type":"file"}],"truncated":false}` {
		t.Errorf("index servi : %d %s", rec.Code, got)
	}
	if calls := fake.Calls(); len(calls) != 1 || calls[0] != dbID+":/" {
		t.Errorf("exports : %v, attendu le seul de l'explorateur", calls)
	}
}

// During the build the explorer started, the search answers at once,
// indexed false: never a 202 to poll, never an Inspect that waits for the
// lock dockerd holds during the export (v0.1.1).
func TestFsSearchDuringIndexing(t *testing.T) {
	fake := stoppedFake(t)
	g := &inspectCounter{gatedAPI: &gatedAPI{Fake: fake, open: make(chan struct{})}}
	f, c := stoppedFixture(t, g, fake, fsview.Options{})
	if rec := f.do("GET", "/api/containers/db/fs?path=/etc", "", c); rec.Code != http.StatusAccepted {
		t.Fatalf("explorateur : %d %s", rec.Code, rec.Body)
	}
	before := g.n.Load()
	rec := search(f, "db", "host", c)
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != `{"indexed":false,"results":[],"truncated":false}` {
		t.Errorf("pendant l'indexation : %d %s", rec.Code, got)
	}
	if n := g.n.Load() - before; n != 0 {
		t.Errorf("%d Inspect pendant l'export", n)
	}
	close(g.open)
}

// inspectCounter counts the Inspect calls of a gatedAPI.
type inspectCounter struct {
	*gatedAPI
	n atomic.Int32
}

func (i *inspectCounter) Inspect(ctx context.Context, id string) (dockerx.ContainerInfo, error) {
	i.n.Add(1)
	return i.gatedAPI.Inspect(ctx, id)
}

// A running container has no index (procfs): indexed false, nothing opened.
func TestFsSearchRunning(t *testing.T) {
	fake := stoppedFake(t)
	info := fake.Infos["db"]
	info.Running, info.Pid = true, 42
	fake.Infos["db"], fake.Infos[dbID] = info, info
	f, c := stoppedFixture(t, fake, fake, fsview.Options{})
	rec := search(f, "db", "host", c)
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != `{"indexed":false,"results":[],"truncated":false}` {
		t.Errorf("conteneur actif : %d %s", rec.Code, got)
	}
	if calls := fake.Calls(); len(calls) != 0 {
		t.Errorf("Docker sollicité : %v", calls)
	}
}

// Spec v0.2 §5.8: 200 results at most, then truncated.
func TestFsSearchLimit(t *testing.T) {
	fake := stoppedFake(t)
	es := []stEntry{{name: "/", typ: tar.TypeDir}, {name: "/etc/", typ: tar.TypeDir}, {name: "/data/", typ: tar.TypeDir}}
	for i := range 250 {
		es = append(es, stEntry{name: fmt.Sprintf("/data/match-%03d", i), typ: tar.TypeReg})
	}
	fake.Archives[dbID+":/"] = stTar(t, es...)
	f, c := stoppedFixture(t, fake, fake, fsview.Options{})
	serveIndex(t, f, c)
	var got searchJSON
	rec := search(f, "db", "match-", c)
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if !got.Indexed || !got.Truncated || len(got.Results) != 200 {
		t.Errorf("indexed=%v truncated=%v, %d résultats", got.Indexed, got.Truncated, len(got.Results))
	}
}

// Spec v0.2 §5.8: the agent never logs the query, nor writes it to the
// audit log; the answer is never cached by the browser.
func TestFsSearchNoTrace(t *testing.T) {
	var logs bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	fake := stoppedFake(t)
	f, c := stoppedFixture(t, fake, fake, fsview.Options{})
	serveIndex(t, f, c)
	const marker = "zq-stripe_live_key"
	for _, q := range []string{marker, marker + strings.Repeat("x", 200)} {
		rec := search(f, "db", q, c)
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("Cache-Control : %q", rec.Header().Get("Cache-Control"))
		}
	}
	fake.Err = dockerx.ErrUnavailable
	if rec := search(f, "web", marker, c); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Docker indisponible : %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(logs.String(), marker) {
		t.Errorf("la requête est dans le journal :\n%s", logs.String())
	}
	data, _ := os.ReadFile(f.auditP)
	if strings.Contains(string(data), marker) {
		t.Errorf("la requête est dans l'audit : %s", data)
	}
}
