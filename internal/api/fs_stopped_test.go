package api

// File routes on a stopped container through the real default wiring of New
// (Views.For, tarindex), with a fake Docker.

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/files"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

var dbID = strings.Repeat("d0", 32)

type stEntry struct {
	name, body, link string
	typ              byte
}

func stTar(t *testing.T, es ...stEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range es {
		if err := tw.WriteHeader(&tar.Header{Name: e.name, Typeflag: e.typ, Linkname: e.link, Mode: 0o644, Size: int64(len(e.body))}); err != nil {
			t.Fatal(err)
		}
		io.WriteString(tw, e.body)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// stTarNames reads a tar to io.EOF; any other error fails.
func stTarNames(t *testing.T, b []byte) []string {
	t.Helper()
	var names []string
	tr := tar.NewReader(bytes.NewReader(b))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return names
		}
		if err != nil {
			t.Fatalf("tar : %v", err)
		}
		names = append(names, h.Name)
	}
}

func stoppedFake(t *testing.T, mounts ...dockerx.Mount) *dockerx.Fake {
	info := dockerx.ContainerInfo{ID: dbID, Name: "db", FinishedAt: "2026-10-09T10:00:00Z", Mounts: mounts}
	return &dockerx.Fake{
		Infos: map[string]dockerx.ContainerInfo{"db": info, dbID: info},
		Archives: map[string][]byte{
			dbID + ":/": stTar(t,
				stEntry{name: "/", typ: tar.TypeDir}, stEntry{name: "/etc/", typ: tar.TypeDir},
				stEntry{name: "/etc/hostname", body: "db\n", typ: tar.TypeReg},
				stEntry{name: "/etc/alias", link: "hostname", typ: tar.TypeSymlink},
				stEntry{name: "/conf", link: "etc", typ: tar.TypeSymlink}),
			dbID + ":/etc/hostname": stTar(t, stEntry{name: "hostname", body: "db\n", typ: tar.TypeReg}),
			dbID + ":/etc":          stTar(t, stEntry{name: "etc/", typ: tar.TypeDir}, stEntry{name: "etc/hostname", body: "db\n", typ: tar.TypeReg}),
		},
		Volumes: map[string]map[string]string{},
	}
}

// gatedAPI holds GET /archive of "/" until open is closed.
type gatedAPI struct {
	*dockerx.Fake
	open chan struct{}
}

func (g *gatedAPI) GetArchive(ctx context.Context, id, p string) (io.ReadCloser, error) {
	if p == "/" {
		select {
		case <-g.open:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return g.Fake.GetArchive(ctx, id, p)
}

func stoppedFixture(t *testing.T, docker dockerx.Client, fake *dockerx.Fake, opt fsview.Options) (*fixture, *http.Cookie) {
	t.Helper()
	f := &fixture{docker: fake, views: fsview.NewViews(docker, opt)}
	f.init(t, false, docker, nil) // nil: Views.For, the default wiring
	return f, f.session(t)
}

var stoppedRoutes = []string{
	"/fs?path=/etc", "/fs/file?path=/etc/hostname", "/fs/download?path=/etc",
	"/fs/download?path=/etc&check=1", "/fs/download?path=/etc/hostname",
}

func TestStoppedContainerRoutes(t *testing.T) {
	fake := stoppedFake(t)
	g := &gatedAPI{Fake: fake, open: make(chan struct{})}
	f, c := stoppedFixture(t, g, fake, fsview.Options{})
	for _, u := range stoppedRoutes {
		rec := f.do("GET", "/api/containers/db"+u, "", c)
		var body struct {
			Error    string   `json:"error"`
			Progress *float64 `json:"progress"`
		}
		json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != http.StatusAccepted || body.Error != "indexing" || body.Progress == nil || *body.Progress > 0.99 ||
			rec.Header().Get("Content-Disposition") != "" {
			t.Errorf("%s pendant l'indexation : %d %s", u, rec.Code, rec.Body)
		}
	}
	if rec := f.do("HEAD", "/api/containers/db/fs/download?path=/etc", "", c); rec.Code != http.StatusAccepted {
		t.Errorf("HEAD pendant l'indexation : %d", rec.Code)
	}
	close(g.open)
	var rec *httptest.ResponseRecorder
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		rec = f.do("GET", "/api/containers/db/fs?path=/etc", "", c)
		if rec.Code != http.StatusAccepted || time.Now().After(deadline) {
			break
		}
	}
	l := decodeListing(t, rec)
	if len(l.Entries) != 2 {
		t.Fatalf("listing : %+v", l)
	}
	// Review I2: check=1 and HEAD on a file judge from the index (spec §5.4: no GetArchive).
	if rec := f.do("GET", "/api/containers/db/fs/download?path=/etc/alias&check=1", "", c); rec.Code != http.StatusNoContent {
		t.Errorf("check=1 d'un fichier : %d %s", rec.Code, rec.Body)
	}
	if rec := f.do("HEAD", "/api/containers/db/fs/download?path=/etc/hostname", "", c); rec.Code != http.StatusNoContent {
		t.Errorf("HEAD d'un fichier : %d", rec.Code)
	}
	if calls := fake.Calls(); slices.Contains(calls, dbID+":/etc/hostname") {
		t.Errorf("check=1 a lu le fichier : %v", calls)
	}
	rec = f.do("GET", "/api/containers/db/fs/file?path=/etc/alias", "", c)
	if rec.Code != http.StatusOK || rec.Body.String() != "db\n" {
		t.Errorf("aperçu par un lien : %d %q", rec.Code, rec.Body)
	}
	if calls := fake.Calls(); slices.Contains(calls, dbID+":/etc/alias") || !slices.Contains(calls, dbID+":/etc/hostname") {
		t.Errorf("ÉVASION : seul le chemin résolu part vers Docker : %v", calls)
	}
	if rec := f.do("GET", "/api/containers/db/fs/download?path=/etc&check=1", "", c); rec.Code != http.StatusNoContent {
		t.Errorf("check=1 : %d %s", rec.Code, rec.Body)
	}
	rec = f.do("GET", "/api/containers/db/fs/download?path=/etc", "", c)
	if rec.Code != http.StatusOK || !slices.Contains(stTarNames(t, rec.Body.Bytes()), "etc/hostname") {
		t.Errorf("tar du dossier : %d", rec.Code)
	}
	if rec := f.do("GET", "/api/containers/db/fs/download?path=/conf&check=1", "", c); rec.Code != http.StatusUnprocessableEntity || errCode(t, rec) != "link_in_path" {
		t.Errorf("dossier atteint par un lien : %d %s", rec.Code, rec.Body)
	}
}

func TestStoppedHostBindRefusesEveryRoute(t *testing.T) {
	fake := stoppedFake(t, dockerx.Mount{Type: "bind", Source: "/proc", Destination: "/host/proc"})
	f, c := stoppedFixture(t, fake, fake, fsview.Options{})
	for _, u := range stoppedRoutes {
		rec := f.do("GET", "/api/containers/db"+u, "", c)
		// Decoded JSON: encoding/json escapes ">" as > in the raw body.
		var body struct{ Error, Detail string }
		json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != http.StatusForbidden || body.Error != "forbidden_path" || !strings.Contains(body.Detail, "bind /proc -> /host/proc") {
			t.Errorf("%s : %d %s", u, rec.Code, rec.Body)
		}
	}
	if calls := fake.Calls(); len(calls) != 0 {
		t.Errorf("ÉVASION : GetArchive %v", calls)
	}
}

func TestStoppedTooLarge(t *testing.T) {
	fake := stoppedFake(t)
	f, c := stoppedFixture(t, fake, fake, fsview.Options{Limits: fsview.IndexLimits{MaxBytes: 512}, FirstWait: 5 * time.Second})
	if rec := f.do("GET", "/api/containers/db/fs?path=/", "", c); rec.Code != http.StatusRequestEntityTooLarge || errCode(t, rec) != "too_large" {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
}

// Only the request that starts a build waits (FirstWait): polls answer at
// once, so they never pile up in the guard (4 slots per container).
func TestStoppedPollsDoNotWait(t *testing.T) {
	fake := stoppedFake(t)
	g := &gatedAPI{Fake: fake, open: make(chan struct{})}
	defer close(g.open)
	f, c := stoppedFixture(t, g, fake, fsview.Options{FirstWait: 300 * time.Millisecond})
	start := time.Now()
	if rec := f.do("GET", "/api/containers/db/fs?path=/", "", c); rec.Code != http.StatusAccepted {
		t.Fatalf("première requête : %d", rec.Code)
	}
	if d := time.Since(start); d < 300*time.Millisecond { // control: the starter does wait
		t.Fatalf("la requête qui lance la construction doit attendre FirstWait : %s", d)
	}
	for range 3 {
		start = time.Now()
		rec := f.do("GET", "/api/containers/db/fs?path=/", "", c)
		if d := time.Since(start); rec.Code != http.StatusAccepted || d > 150*time.Millisecond {
			t.Errorf("relance : %d en %s, attendu 202 immédiat", rec.Code, d)
		}
	}
}

// lockedAPI models dockerd: GET /archive holds the container lock until the
// tar is sent, and an Inspect of that container waits for it (measured on
// Docker Desktop 29.7.2: 10.6 s during a golang:1 export, 0.02 s at rest).
// It gives up after 2 s, like a client deadline, to keep a red run short.
type lockedAPI struct {
	*gatedAPI
	exporting atomic.Bool
}

func (l *lockedAPI) GetArchive(ctx context.Context, id, p string) (io.ReadCloser, error) {
	if p == "/" {
		l.exporting.Store(true)
	}
	return l.gatedAPI.GetArchive(ctx, id, p)
}

func (l *lockedAPI) waitLock(ctx context.Context) error {
	if !l.exporting.Load() {
		return nil
	}
	select {
	case <-l.open:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Second):
		return fmt.Errorf("%w : context deadline exceeded", dockerx.ErrUnavailable)
	}
}

func (l *lockedAPI) Inspect(ctx context.Context, id string) (dockerx.ContainerInfo, error) {
	if err := l.waitLock(ctx); err != nil {
		return dockerx.ContainerInfo{}, err
	}
	return l.gatedAPI.Inspect(ctx, id)
}

func (l *lockedAPI) SizeRootFs(ctx context.Context, id string) (int64, error) {
	if err := l.waitLock(ctx); err != nil {
		return 0, err
	}
	return l.gatedAPI.SizeRootFs(ctx, id)
}

// v0.1.1 (found on Docker Desktop with a real stack): while dockerd exports
// the container, every poll of a read route answers 202 in under a second,
// by name or full ID, never 503 docker_unavailable after 10 s.
func TestStoppedPollsDuringLockedExport(t *testing.T) {
	fake := stoppedFake(t)
	l := &lockedAPI{gatedAPI: &gatedAPI{Fake: fake, open: make(chan struct{})}}
	f, c := stoppedFixture(t, l, fake, fsview.Options{FirstWait: 100 * time.Millisecond})
	if rec := f.do("GET", "/api/containers/db/fs?path=/", "", c); rec.Code != http.StatusAccepted {
		t.Fatalf("première requête : %d %s", rec.Code, rec.Body)
	}
	for _, ref := range []string{"db", dbID} {
		for _, u := range stoppedRoutes {
			start := time.Now()
			rec := f.do("GET", "/api/containers/"+ref+u, "", c)
			if dt := time.Since(start); rec.Code != http.StatusAccepted || errCode(t, rec) != "indexing" || dt > time.Second {
				t.Errorf("relance %s%s pendant l'export : %d en %s %s", ref, u, rec.Code, dt, rec.Body)
			}
		}
		start := time.Now()
		if rec := f.do("HEAD", "/api/containers/"+ref+"/fs/download?path=/etc", "", c); rec.Code != http.StatusAccepted || time.Since(start) > time.Second {
			t.Errorf("HEAD %s pendant l'export : %d en %s", ref, rec.Code, time.Since(start))
		}
	}
	close(l.open)
	var rec *httptest.ResponseRecorder
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		rec = f.do("GET", "/api/containers/db/fs?path=/etc", "", c)
		if rec.Code != http.StatusAccepted || time.Now().After(deadline) {
			break
		}
	}
	if l := decodeListing(t, rec); len(l.Entries) != 2 {
		t.Fatalf("listing : %+v", l)
	}
}

// Writes keep the order of spec §5.3 while the container is indexed: size,
// name and the Inspect checks answer before any indexing (Views.Pending is
// for read routes only). The fake's Inspect does not wait here: under a real
// dockerd lock, a write waits for the end of the export (documented limit).
func TestStoppedWritesDuringIndexingKeepTheirOrder(t *testing.T) {
	fake := stoppedFake(t)
	g := &gatedAPI{Fake: fake, open: make(chan struct{})}
	defer close(g.open)
	f, c := stoppedFixture(t, g, fake, fsview.Options{FirstWait: 50 * time.Millisecond})
	if rec := f.do("GET", "/api/containers/db/fs?path=/", "", c); rec.Code != http.StatusAccepted {
		t.Fatalf("première requête : %d %s", rec.Code, rec.Body)
	}
	if err := f.views.Pending("db"); !errors.Is(err, fsview.ErrIndexing) {
		t.Fatalf("témoin : construction attendue en cours : %v", err)
	}
	cases := []struct {
		name   string
		rec    *httptest.ResponseRecorder
		status int
		code   string
	}{
		{"édition trop grosse", f.send("PUT", "/api/containers/db/fs/file?path=/etc/hostname",
			bytes.Repeat([]byte("x"), files.MaxEditBytes+1), c, map[string]string{"If-Match": `"x"`}), 413, "too_large"},
		{"édition de /etc/hosts", f.send("PUT", "/api/containers/db/fs/file?path=/etc/hosts",
			[]byte("x"), c, map[string]string{"If-Match": `"x"`}), 403, "forbidden_path"},
		{"envoi nommé a/b", f.upload(t, c, "db", "/etc", "a/b", []byte("x"), "*"), 400, "invalid_name"},
	}
	for _, tc := range cases {
		if tc.rec.Code != tc.status || errCode(t, tc.rec) != tc.code {
			t.Errorf("%s pendant l'indexation : %d %s, attendu %d %s", tc.name, tc.rec.Code, tc.rec.Body, tc.status, tc.code)
		}
	}
}

func TestIndexingErrorBody(t *testing.T) {
	rec := httptest.NewRecorder()
	writeFSError(rec, &fsview.IndexingError{Progress: 0.42})
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"progress":0.42`) || errCode(t, rec) != "indexing" {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	if !errors.Is(&fsview.IndexingError{}, fsview.ErrIndexing) {
		t.Error("IndexingError doit envelopper ErrIndexing")
	}
}
