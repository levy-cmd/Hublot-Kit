package api

// File routes on a stopped container through the real default wiring of New
// (Views.For, tarindex), with a fake Docker.

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
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
