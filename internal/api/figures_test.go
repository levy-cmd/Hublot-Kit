package api

// Tests of GET /api/containers/{id}/figures (spec v0.2 §5.3): the key
// figures of the detail, each from its own source, at a bounded cost. The
// files count is read from an index already served, never built for it.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

func i64(n int64) *int64 { return &n }

// figuresFixture: the running "web" container of newFixture, image
// sha256:feed of 999 bytes, sizes 1234 and 5678.
func figuresFixture(t *testing.T) (*fixture, *http.Cookie) {
	t.Helper()
	f := newFixture(t, false)
	web := f.docker.Infos["web"]
	web.Details.ImageID = "sha256:feed"
	f.docker.Infos["web"], f.docker.Infos[webID] = web, web
	f.docker.ContainerSizes = map[string]dockerx.SizePair{webID: {RW: i64(1234), RootFs: i64(5678)}}
	f.docker.Images = map[string]dockerx.ImageInfo{"sha256:feed": {ID: "sha256:feed", Size: 999}}
	return f, f.session(t)
}

// fakeClock replaces figuresNow for one test.
func fakeClock(t *testing.T) *time.Time {
	t.Helper()
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	old := figuresNow
	figuresNow = func() time.Time { return now }
	t.Cleanup(func() { figuresNow = old })
	return &now
}

func TestFigures(t *testing.T) {
	f, c := figuresFixture(t)
	if rec := f.do("GET", "/api/containers/web/figures", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("sans session : %d", rec.Code)
	}
	rec := f.do("GET", "/api/containers/web/figures", "", c)
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK ||
		got != `{"sizeRw":1234,"sizeRootFs":5678,"imageSize":999,"files":null}` {
		t.Fatalf("%d %s", rec.Code, got)
	}
}

func TestFiguresNullSizes(t *testing.T) {
	f, c := figuresFixture(t)
	f.docker.ContainerSizes = nil
	f.docker.Images = nil // image removed (docker rmi -f): no size, not an error
	rec := f.do("GET", "/api/containers/web/figures", "", c)
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != http.StatusOK ||
		got != `{"sizeRw":null,"sizeRootFs":null,"imageSize":null,"files":null}` {
		t.Fatalf("%d %s", rec.Code, got)
	}
}

func TestFiguresErrors(t *testing.T) {
	f, c := figuresFixture(t)
	if rec := f.do("GET", "/api/containers/absent/figures", "", c); rec.Code != http.StatusNotFound || errCode(t, rec) != "not_found" {
		t.Errorf("absent : %d %s", rec.Code, rec.Body)
	}
	if rec := f.do("GET", "/api/containers/..x/figures", "", c); rec.Code != http.StatusNotFound {
		t.Errorf("identifiant invalide : %d", rec.Code)
	}
	f.docker.Err = dockerx.ErrUnavailable
	if rec := f.do("GET", "/api/containers/web/figures", "", c); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Docker indisponible : %d %s", rec.Code, rec.Body)
	}
}

// Spec v0.2 §5.3: Inspect with size=true at most once per minute and per
// container; the image size once per image ID (immutable).
func TestFiguresCache(t *testing.T) {
	now := fakeClock(t)
	f, c := figuresFixture(t)
	for range 3 {
		if rec := f.do("GET", "/api/containers/web/figures", "", c); rec.Code != http.StatusOK {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
	}
	if n := f.docker.SizeCalls.Load(); n != 1 {
		t.Errorf("%d Inspect avec size en une minute, attendu 1", n)
	}
	*now = now.Add(61 * time.Second)
	f.docker.ContainerSizes[webID] = dockerx.SizePair{RW: i64(2000), RootFs: i64(6000)}
	rec := f.do("GET", "/api/containers/web/figures", "", c)
	if !strings.Contains(rec.Body.String(), `"sizeRw":2000`) || f.docker.SizeCalls.Load() != 2 {
		t.Errorf("après 61 s : %s, %d appels", rec.Body, f.docker.SizeCalls.Load())
	}
	if n := f.docker.ImageCalls.Load(); n != 1 {
		t.Errorf("%d GET /images/{id}/json, attendu 1 (cache par ID d'image)", n)
	}
}

// Spec v0.2 §5.3: dockerd holds the container lock during an export, and
// the Inspect waits; figures answers 504 at its deadline.
func TestFiguresTimeout(t *testing.T) {
	old := figuresTimeout
	figuresTimeout = 50 * time.Millisecond
	t.Cleanup(func() { figuresTimeout = old })
	f, c := figuresFixture(t)
	f.docker.SizeGate = make(chan struct{})
	defer close(f.docker.SizeGate)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- f.do("GET", "/api/containers/web/figures", "", c) }()
	select {
	case rec := <-done:
		if rec.Code != http.StatusGatewayTimeout {
			t.Errorf("verrou de dockerd : %d %s", rec.Code, rec.Body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("figures attend sans délai le verrou de dockerd")
	}
}

// Spec v0.2 §5.3: the files count of a stopped container comes from its
// index already built; without one, files is null and nothing is exported.
func TestFiguresUsesCachedIndexOnly(t *testing.T) {
	fake := stoppedFake(t)
	f, c := stoppedFixture(t, fake, fake, fsview.Options{})
	rec := f.do("GET", "/api/containers/db/figures", "", c)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"files":null`) {
		t.Fatalf("sans index : %d %s", rec.Code, rec.Body)
	}
	if calls := fake.Calls(); len(calls) != 0 {
		t.Fatalf("EXPORT : figures a lancé %v", calls)
	}
	// The explorer builds and serves the index.
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		if r := f.do("GET", "/api/containers/db/fs?path=/etc", "", c); r.Code == http.StatusOK {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("index jamais prêt")
		}
	}
	rec = f.do("GET", "/api/containers/db/figures", "", c)
	var got struct {
		Files *struct {
			Count  int    `json:"count"`
			Source string `json:"source"`
			At     string `json:"at"`
		} `json:"files"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Files == nil {
		t.Fatalf("%d %s (%v)", rec.Code, rec.Body, err)
	}
	if at, err := time.Parse(time.RFC3339Nano, got.Files.At); got.Files.Count != 4 || got.Files.Source != "index" || err != nil ||
		time.Since(at) > time.Minute {
		t.Errorf("files : %+v", *got.Files)
	}
	if calls := fake.Calls(); len(calls) != 1 || calls[0] != dbID+":/" {
		t.Errorf("exports : %v, attendu le seul de l'explorateur", calls)
	}
}
