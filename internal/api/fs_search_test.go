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
	"net/url"
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

func searchURL(id, q string) string {
	return "/api/containers/" + id + "/fs/search?q=" + url.QueryEscape(q)
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
	if rec := f.do("GET", searchURL("db", "host"), "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("sans session : %d", rec.Code)
	}
	for _, q := range []string{"", "a", "é", strings.Repeat("a", 201), strings.Repeat("é", 201), "a\x00b"} {
		rec := f.do("GET", searchURL("db", q), "", c)
		if rec.Code != http.StatusBadRequest || errCode(t, rec) != "invalid_query" {
			t.Errorf("q de %d caractères : %d %s", len([]rune(q)), rec.Code, rec.Body)
		}
		if len(q) > 1 && strings.Contains(rec.Body.String(), q) {
			t.Errorf("la réponse répète la requête : %s", rec.Body)
		}
	}
	if rec := f.do("GET", "/api/containers/db/fs/search", "", c); rec.Code != http.StatusBadRequest || errCode(t, rec) != "invalid_query" {
		t.Errorf("sans q : %d %s", rec.Code, rec.Body)
	}
	if calls := fake.Calls(); len(calls) != 0 {
		t.Errorf("requête refusée, Docker sollicité : %v", calls)
	}
	for _, q := range []string{"ab", "éé", strings.Repeat("a", 200), strings.Repeat("é", 200)} {
		if rec := f.do("GET", searchURL("db", q), "", c); rec.Code != http.StatusOK {
			t.Errorf("q de %d caractères : %d %s", len([]rune(q)), rec.Code, rec.Body)
		}
	}
	if rec := f.do("GET", searchURL("absent", "host"), "", c); rec.Code != http.StatusNotFound || errCode(t, rec) != "not_found" {
		t.Errorf("conteneur absent : %d %s", rec.Code, rec.Body)
	}
	if rec := f.do("GET", searchURL("..x", "host"), "", c); rec.Code != http.StatusNotFound {
		t.Errorf("identifiant invalide : %d", rec.Code)
	}
}

// Q61: without an index already served, {"indexed": false} and nothing is
// exported; once the explorer has served it, the search reads it.
func TestFsSearchNeverBuilds(t *testing.T) {
	fake := stoppedFake(t)
	f, c := stoppedFixture(t, fake, fake, fsview.Options{})
	rec := f.do("GET", searchURL("db", "host"), "", c)
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK || got != `{"indexed":false,"results":[],"truncated":false}` {
		t.Fatalf("sans index : %d %s", rec.Code, got)
	}
	if calls := fake.Calls(); len(calls) != 0 {
		t.Fatalf("EXPORT : la recherche a lancé %v", calls)
	}
	serveIndex(t, f, c)
	rec = f.do("GET", searchURL("db", "HOST"), "", c)
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
	rec := f.do("GET", searchURL("db", "host"), "", c)
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
	rec := f.do("GET", searchURL("db", "host"), "", c)
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
	rec := f.do("GET", searchURL("db", "match-"), "", c)
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
		rec := f.do("GET", searchURL("db", q), "", c)
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("Cache-Control : %q", rec.Header().Get("Cache-Control"))
		}
	}
	fake.Err = dockerx.ErrUnavailable
	if rec := f.do("GET", searchURL("web", marker), "", c); rec.Code != http.StatusServiceUnavailable {
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
