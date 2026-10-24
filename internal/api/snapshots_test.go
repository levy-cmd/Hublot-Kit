package api

// Tests of the snapshot routes (spec v0.2 §5.7, §6, §7), written before the
// code: 202 of the indexing form, one at a time, journal before the write,
// allowed read-only (Q57), retention (Q66), never a mac in a response.

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/audit"
	"github.com/levy-cmd/hublot-kit/internal/auth"
	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
	"github.com/levy-cmd/hublot-kit/internal/snapshot"
)

// memWalker is a MemFS that walks like procfs (sorted, depth first, open
// for regular files only); gate, when set, holds the walk before its first
// entry; walks counts the walks, read the bytes read.
type memWalker struct {
	*fsview.MemFS
	gate  chan struct{}
	walks atomic.Int32
	read  atomic.Int64
}

func (m *memWalker) Walk(ctx context.Context, fn func(string, fsview.Entry, func() (io.ReadCloser, error)) error) error {
	m.walks.Add(1)
	if m.gate != nil {
		select {
		case <-m.gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	var ps []string
	for p := range m.Entries {
		ps = append(ps, p)
	}
	sort.Slice(ps, func(i, j int) bool { return snapshot.ComparePaths(ps[i], ps[j]) < 0 })
	for _, p := range ps {
		e := m.Entries[p]
		var open func() (io.ReadCloser, error)
		if e.Type == fsview.TypeFile {
			open = func() (io.ReadCloser, error) {
				rc, err := m.MemFS.Open(ctx, p)
				if err != nil {
					return nil, err
				}
				b, _ := io.ReadAll(rc)
				m.read.Add(int64(len(b)))
				return io.NopCloser(strings.NewReader(string(b))), nil
			}
		}
		if err := fn(p, e, open); err != nil {
			return err
		}
	}
	return nil
}

// snapFixture: web runs (OpenFS gives walker), db is stopped (its archive,
// and the real Views over the fake), both in the container list.
type snapFixture struct {
	*fixture
	walker *memWalker
	store  *snapshot.Store
	data   string
	srv    *server
}

// lastServer is the server New built last (the api tests are sequential).
var lastServer atomic.Pointer[server]

func init() {
	moduleRoutes = append(moduleRoutes, func(s *server, _ *http.ServeMux) { lastServer.Store(s) })
}

// snapJob is the heavy job in progress, nil if none.
func (sf *snapFixture) snapJob() (*snapJob, bool) { return sf.srv.snap.current("") }

func newSnapFixture(t *testing.T, readOnly bool, docker dockerx.Client, fake *dockerx.Fake) *snapFixture {
	t.Helper()
	return newSnapFixtureStore(t, readOnly, docker, fake, nil)
}

// newSnapFixtureStore: store nil opens the real one in the data directory.
func newSnapFixtureStore(t *testing.T, readOnly bool, docker dockerx.Client, fake *dockerx.Fake, store *snapshot.Store) *snapFixture {
	t.Helper()
	sf := &snapFixture{fixture: &fixture{docker: fake, fs: fsview.NewMemFS()}}
	sf.walker = &memWalker{MemFS: sf.fs}
	sf.fs.AddDir("/etc").AddFile("/etc/motd", []byte("bonjour")).AddFile("/etc/secret", []byte("s3cr3t-contenu")).AddLink("/etc/l", "motd")
	dir := t.TempDir()
	sf.data = dir
	tok, _, err := auth.EnsureToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	v, err := auth.LoadVerifier(dir)
	if err != nil {
		t.Fatal(err)
	}
	d, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	sf.store = store
	if store == nil {
		if sf.store, err = snapshot.Open(d); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { sf.store.Close() })
	ap := filepath.Join(dir, "audit.log")
	al, err := audit.Open(ap)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { al.Close() })
	views := fsview.NewViews(docker, fsview.DefaultOptions())
	sf.h = New(Deps{
		Docker: docker, Views: views, Verifier: v, Audit: al, Version: "test", ReadOnly: readOnly,
		OpenFS: func(ctx context.Context, id string) (fsview.FS, error) {
			if id == webID {
				return sf.walker, nil
			}
			return views.For(ctx, id)
		},
		Snapshots: sf.store,
		Sessions:  auth.NewSessions(auth.SessionTTL, time.Now),
		Limiter:   auth.NewLimiter(5, time.Minute, time.Now),
	})
	sf.token, sf.auditP, sf.auditLog = tok, ap, al
	sf.srv = lastServer.Load()
	return sf
}

func snapFake(t *testing.T) *dockerx.Fake {
	t.Helper()
	web := dockerx.ContainerInfo{ID: webID, Name: "web", Running: true, Pid: 42,
		Details: dockerx.Details{Image: "nginx:1.27", ImageID: "sha256:" + strings.Repeat("1", 64), Labels: map[string]string{"com.docker.compose.project": "shop"}}}
	db := dockerx.ContainerInfo{ID: dbID, Name: "db", FinishedAt: "2026-10-09T10:00:00Z"}
	return &dockerx.Fake{
		Containers: []dockerx.Container{{ID: webID, Name: "web", State: "running"}, {ID: dbID, Name: "db", State: "exited"}},
		Infos:      map[string]dockerx.ContainerInfo{"web": web, webID: web, "db": db, dbID: db},
		Archives: map[string][]byte{dbID + ":/": stTar(t,
			stEntry{name: "/", typ: tar.TypeDir}, stEntry{name: "/etc/", typ: tar.TypeDir},
			stEntry{name: "/etc/motd", body: "salut", typ: tar.TypeReg},
			stEntry{name: "/etc/secret", body: "s3cr3t-contenu", typ: tar.TypeReg})},
		NoWrites: true,
	}
}

func snapSetup(t *testing.T, readOnly bool) (*snapFixture, *http.Cookie) {
	t.Helper()
	fake := snapFake(t)
	sf := newSnapFixture(t, readOnly, fake, fake)
	return sf, sf.session(t)
}

type snapList struct {
	Snapshots []map[string]any `json:"snapshots"`
	Current   map[string]any   `json:"current"`
	Last      map[string]any   `json:"last"`
}

func (sf *snapFixture) list(t *testing.T, c *http.Cookie) (snapList, string) {
	t.Helper()
	rec := sf.do("GET", "/api/snapshots", "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/snapshots : %d %s", rec.Code, rec.Body)
	}
	var l snapList
	if err := json.Unmarshal(rec.Body.Bytes(), &l); err != nil {
		t.Fatalf("GET /api/snapshots : %v %s", err, rec.Body)
	}
	return l, rec.Body.String()
}

// idle waits for the end of the capture in progress, and returns the list.
func (sf *snapFixture) idle(t *testing.T, c *http.Cookie) (snapList, string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		l, body := sf.list(t, c)
		if l.Current == nil {
			return l, body
		}
		if time.Now().After(deadline) {
			t.Fatalf("prise jamais finie : %s", body)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (sf *snapFixture) files(t *testing.T) []string {
	t.Helper()
	es, err := os.ReadDir(filepath.Join(sf.data, "snapshots"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestSnapshotAccepted202(t *testing.T) {
	sf, c := snapSetup(t, false)
	rec := sf.do("POST", "/api/containers/web/snapshots", "", c)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("prise : %d %s", rec.Code, rec.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	keys := slices.Sorted(func(yield func(string) bool) {
		for k := range body {
			if !yield(k) {
				return
			}
		}
	})
	if !slices.Equal(keys, []string{"detail", "error", "progress", "snapshot"}) || body["error"] != "indexing" ||
		body["progress"] != 0.0 || body["detail"] != "snapshot in progress" {
		t.Fatalf("corps : %s", rec.Body)
	}
	id, _ := body["snapshot"].(string)
	if !uuidV4.MatchString(id) {
		t.Fatalf("snapshot %q : pas un UUID v4", id)
	}
	l, _ := sf.idle(t, c)
	if len(l.Snapshots) != 1 || l.Snapshots[0]["id"] != id || l.Snapshots[0]["containerId"] != webID ||
		l.Snapshots[0]["source"] != "procfs" || l.Snapshots[0]["state"] != "running" || l.Snapshots[0]["entries"] != 4.0 ||
		l.Snapshots[0]["project"] != "shop" || l.Snapshots[0]["containerGone"] != false {
		t.Fatalf("liste : %+v", l.Snapshots)
	}
	if l.Last == nil || l.Last["snapshot"] != id || l.Last["result"] != "ok" {
		t.Fatalf("dernier résultat : %+v", l.Last)
	}
	log, _ := os.ReadFile(sf.auditP)
	if n := strings.Count(string(log), `"action":"snapshot.create"`); n != 2 || !strings.Contains(string(log), `"path":"`+id+`"`) ||
		!strings.Contains(string(log), `"container":"`+webID+`"`) {
		t.Fatalf("journal : %d ligne(s) snapshot.create\n%s", n, log)
	}
}

func TestSnapshotStoppedArchive(t *testing.T) {
	sf, c := snapSetup(t, false)
	if rec := sf.do("POST", "/api/containers/db/snapshots", "", c); rec.Code != http.StatusAccepted {
		t.Fatalf("prise : %d %s", rec.Code, rec.Body)
	}
	l, _ := sf.idle(t, c)
	if len(l.Snapshots) != 1 || l.Snapshots[0]["source"] != "archive" || l.Snapshots[0]["state"] != "stopped" || l.Snapshots[0]["entries"] != 3.0 {
		t.Fatalf("liste : %+v (dernier : %+v)", l.Snapshots, l.Last)
	}
}

func TestSnapshotForbiddenBind(t *testing.T) {
	fake := snapFake(t)
	db := fake.Infos[dbID]
	db.Mounts = []dockerx.Mount{{Type: "bind", Source: "/", Destination: "/host"}}
	fake.Infos["db"], fake.Infos[dbID] = db, db
	sf := newSnapFixture(t, false, fake, fake)
	c := sf.session(t)
	rec := sf.do("POST", "/api/containers/db/snapshots", "", c)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"forbidden_path"`) {
		t.Fatalf("bind de / : %d %s", rec.Code, rec.Body)
	}
	if calls := fake.Calls(); len(calls) != 0 {
		t.Fatalf("GetArchive appelé : %v", calls)
	}
	if got := sf.files(t); len(got) != 0 {
		t.Fatalf("fichiers : %v", got)
	}
}

// Q57: read-only changes nothing for the snapshots, which write in the
// agent's directory, never in Docker.
func TestSnapshotAllowedReadOnly(t *testing.T) {
	for _, ro := range []bool{false, true} {
		sf, c := snapSetup(t, ro)
		for _, ref := range []string{"web", "db"} {
			if rec := sf.do("POST", "/api/containers/"+ref+"/snapshots", "", c); rec.Code != http.StatusAccepted {
				t.Fatalf("readOnly=%v, %s : %d %s", ro, ref, rec.Code, rec.Body)
			}
			sf.idle(t, c)
		}
		// The photo of the metadata needs the index already served.
		rec := sf.do("GET", "/api/containers/db/fs?path=/etc", "", c)
		for deadline := time.Now().Add(5 * time.Second); rec.Code == http.StatusAccepted && time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			rec = sf.do("GET", "/api/containers/db/fs?path=/etc", "", c)
		}
		if rec := sf.do("POST", "/api/containers/db/snapshots?source=index", "", c); rec.Code != http.StatusAccepted {
			t.Fatalf("readOnly=%v, index : %d %s", ro, rec.Code, rec.Body)
		}
		l, _ := sf.idle(t, c)
		var sources []string
		for _, s := range l.Snapshots {
			sources = append(sources, s["source"].(string))
		}
		slices.Sort(sources)
		if !slices.Equal(sources, []string{"archive", "index", "procfs"}) {
			t.Fatalf("readOnly=%v : sources %v (dernier : %+v)", ro, sources, l.Last)
		}
		id := l.Snapshots[0]["id"].(string)
		if rec := sf.do("DELETE", "/api/snapshots/"+id, "", c); rec.Code != http.StatusNoContent {
			t.Fatalf("readOnly=%v, suppression : %d %s", ro, rec.Code, rec.Body)
		}
		if l, _ := sf.list(t, c); len(l.Snapshots) != 2 {
			t.Fatalf("readOnly=%v : %d snapshots après la suppression", ro, len(l.Snapshots))
		}
		log, _ := os.ReadFile(sf.auditP)
		if n := strings.Count(string(log), `"action":"snapshot.delete"`); n != 2 {
			t.Fatalf("readOnly=%v : %d ligne(s) snapshot.delete\n%s", ro, n, log)
		}
	}
}

func TestSnapshotIndexNeedsBuiltIndex(t *testing.T) {
	sf, c := snapSetup(t, false)
	rec := sf.do("POST", "/api/containers/db/snapshots?source=index", "", c)
	if rec.Code != http.StatusConflict {
		t.Fatalf("sans index construit : %d %s", rec.Code, rec.Body)
	}
	if calls := sf.docker.Calls(); len(calls) != 0 {
		t.Fatalf("export lancé : %v", calls)
	}
	if rec := sf.do("POST", "/api/containers/web/snapshots?source=index", "", c); rec.Code != http.StatusConflict {
		t.Fatalf("conteneur actif : %d %s", rec.Code, rec.Body)
	}
	if rec := sf.do("POST", "/api/containers/web/snapshots?source=autre", "", c); rec.Code != http.StatusBadRequest {
		t.Fatalf("source inconnue : %d %s", rec.Code, rec.Body)
	}
}

func TestSnapshotAuditBeforeWrite(t *testing.T) {
	sf, c := snapSetup(t, false)
	sf.auditLog.Close()
	rec := sf.do("POST", "/api/containers/web/snapshots", "", c)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("journal fermé : %d %s", rec.Code, rec.Body)
	}
	time.Sleep(50 * time.Millisecond)
	if n := sf.walker.walks.Load(); n != 0 {
		t.Fatalf("%d parcours malgré l'échec du journal", n)
	}
	if got := sf.files(t); len(got) != 0 {
		t.Fatalf("fichiers : %v", got)
	}
}

func TestSnapshotDeleteAuditBeforeWrite(t *testing.T) {
	sf, c := snapSetup(t, false)
	sf.do("POST", "/api/containers/web/snapshots", "", c)
	l, _ := sf.idle(t, c)
	id := l.Snapshots[0]["id"].(string)
	sf.auditLog.Close()
	if rec := sf.do("DELETE", "/api/snapshots/"+id, "", c); rec.Code != http.StatusInternalServerError {
		t.Fatalf("journal fermé : %d %s", rec.Code, rec.Body)
	}
	if got := sf.files(t); len(got) != 1 {
		t.Fatalf("fichiers : %v, attendu le snapshot intact", got)
	}
}

func TestSnapshotOriginRequired(t *testing.T) {
	sf, c := snapSetup(t, false)
	for _, m := range []struct{ method, path string }{{"POST", "/api/containers/web/snapshots"}, {"DELETE", "/api/snapshots/" + snapshot.NewID()}} {
		r := httptestRequest(m.method, m.path, c)
		r.Header.Set("Origin", "http://evil.example")
		rec := serve(sf.h, r)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s depuis une autre origine : %d %s", m.method, m.path, rec.Code, rec.Body)
		}
		r = httptestRequest(m.method, m.path, c)
		if rec := serve(sf.h, r); rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s sans Origin : %d %s", m.method, m.path, rec.Code, rec.Body)
		}
	}
	if n := sf.walker.walks.Load(); n != 0 {
		t.Fatalf("%d parcours", n)
	}
}

func TestSnapshotSessionRequired(t *testing.T) {
	sf, _ := snapSetup(t, false)
	for _, m := range []struct{ method, path string }{{"POST", "/api/containers/web/snapshots"}, {"GET", "/api/snapshots"},
		{"DELETE", "/api/snapshots/" + snapshot.NewID()}, {"GET", "/api/containers/web/count"}} {
		if rec := sf.do(m.method, m.path, "", nil); rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s sans session : %d", m.method, m.path, rec.Code)
		}
	}
}

func TestSnapshotOneAtATime(t *testing.T) {
	sf, c := snapSetup(t, false)
	sf.walker.gate = make(chan struct{})
	if rec := sf.do("POST", "/api/containers/web/snapshots", "", c); rec.Code != http.StatusAccepted {
		t.Fatalf("première prise : %d %s", rec.Code, rec.Body)
	}
	for _, ref := range []string{"web", "db"} {
		rec := sf.do("POST", "/api/containers/"+ref+"/snapshots", "", c)
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"conflict"`) {
			t.Fatalf("seconde prise (%s) : %d %s", ref, rec.Code, rec.Body)
		}
	}
	if l, _ := sf.list(t, c); l.Current == nil || l.Current["containerId"] != webID {
		t.Fatalf("prise en cours absente de la liste : %+v", l.Current)
	}
	close(sf.walker.gate)
	sf.idle(t, c)
	if rec := sf.do("POST", "/api/containers/db/snapshots", "", c); rec.Code != http.StatusAccepted {
		t.Fatalf("prise après la fin de la première : %d %s", rec.Code, rec.Body)
	}
	sf.idle(t, c)
}

// e2e: a capture that ends while the list is read must show either in
// progress or in the list, never neither (the page would stop polling).
func TestSnapshotListNeverLosesTheEnd(t *testing.T) {
	sf, c := snapSetup(t, false)
	sf.walker.gate = make(chan struct{})
	if rec := sf.do("POST", "/api/containers/web/snapshots", "", c); rec.Code != http.StatusAccepted {
		t.Fatalf("prise : %d %s", rec.Code, rec.Body)
	}
	old := snapListBetween
	t.Cleanup(func() { snapListBetween = old })
	snapListBetween = func() {
		snapListBetween = func() {}
		close(sf.walker.gate)
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
			if j, _ := sf.snapJob(); j == nil {
				return
			}
		}
	}
	l, body := sf.list(t, c)
	if l.Current == nil && len(l.Snapshots) == 0 {
		t.Fatalf("prise ni en cours ni dans la liste : %s", body)
	}
	sf.idle(t, c)
}

func TestSnapshotRetention(t *testing.T) {
	sf, c := snapSetup(t, false)
	for range snapshot.MaxPerContainer {
		h := snapshot.Header{ID: snapshot.NewID(), ContainerID: webID, CreatedAt: time.Now().UTC(), Source: "procfs", State: "running"}
		w, err := sf.store.Create(h)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.Commit(h); err != nil {
			t.Fatal(err)
		}
	}
	rec := sf.do("POST", "/api/containers/web/snapshots", "", c)
	if rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), `"too_large"`) {
		t.Fatalf("21e prise : %d %s", rec.Code, rec.Body)
	}
	if got := sf.files(t); len(got) != snapshot.MaxPerContainer {
		t.Fatalf("%d fichiers, attendu %d (aucune suppression)", len(got), snapshot.MaxPerContainer)
	}
	if n := sf.walker.walks.Load(); n != 0 {
		t.Fatalf("%d parcours malgré le refus", n)
	}
}

// changingDocker answers the first Inspect of web as the fake, every later
// one with a stopped container: the final check must throw the walk away.
type changingDocker struct {
	*dockerx.Fake
	mu    sync.Mutex
	calls int
}

func (d *changingDocker) Inspect(ctx context.Context, id string) (dockerx.ContainerInfo, error) {
	info, err := d.Fake.Inspect(ctx, id)
	if err != nil || info.ID != webID {
		return info, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.calls++; d.calls > 1 {
		info.Running, info.Pid, info.FinishedAt = false, 0, "2026-10-24T10:00:00Z"
	}
	return info, nil
}

func TestSnapshotContainerStopsDuringWalk(t *testing.T) {
	fake := snapFake(t)
	sf := newSnapFixture(t, false, &changingDocker{Fake: fake}, fake)
	c := sf.session(t)
	if rec := sf.do("POST", "/api/containers/web/snapshots", "", c); rec.Code != http.StatusAccepted {
		t.Fatalf("prise : %d %s", rec.Code, rec.Body)
	}
	l, _ := sf.idle(t, c)
	if len(l.Snapshots) != 0 || l.Last == nil || l.Last["result"] != "container_changed" {
		t.Fatalf("liste : %+v, dernier : %+v", l.Snapshots, l.Last)
	}
	if got := sf.files(t); len(got) != 0 {
		t.Fatalf("fichiers : %v", got)
	}
	log, _ := os.ReadFile(sf.auditP)
	if !strings.Contains(string(log), `"result":"error"`) {
		t.Fatalf("journal sans le résultat de l'échec :\n%s", log)
	}
}

func TestSnapshotDeleteUnknown(t *testing.T) {
	sf, c := snapSetup(t, false)
	rec := sf.do("DELETE", "/api/snapshots/"+snapshot.NewID(), "", c)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"not_found"`) {
		t.Fatalf("suppression inconnue : %d %s", rec.Code, rec.Body)
	}
}

func TestSnapshotIDFormat(t *testing.T) {
	sf, c := snapSetup(t, false)
	sf.do("POST", "/api/containers/web/snapshots", "", c)
	l, _ := sf.idle(t, c)
	id := l.Snapshots[0]["id"].(string)
	for _, bad := range []string{"..%2Fx", "x", strings.ToUpper(id), id + ".jsonl.gz", "..%2F" + id} {
		if rec := sf.do("DELETE", "/api/snapshots/"+bad, "", c); rec.Code != http.StatusNotFound {
			t.Fatalf("DELETE %s : %d %s", bad, rec.Code, rec.Body)
		}
	}
	if got := sf.files(t); len(got) != 1 {
		t.Fatalf("fichiers : %v", got)
	}
}

func TestSnapshotKeyRefused(t *testing.T) {
	sf, c := snapSetup(t, false)
	p := filepath.Join(sf.data, "snapshot.key")
	if err := os.WriteFile(p, []byte(strings.Repeat("k", 32)), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Chmod(p, 0o644)
	rec := sf.do("POST", "/api/containers/web/snapshots", "", c)
	// Review of tranche 8: a fixed detail that says it (e2267be), never the
	// text of the error, which goes to the log, masked.
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), `"detail":"`+snapKeyDetail+`"`) ||
		strings.Contains(rec.Body.String(), "0644") || strings.Contains(rec.Body.String(), "mode") {
		t.Fatalf("clé ouverte au groupe : %d %s", rec.Code, rec.Body)
	}
	if n := sf.walker.walks.Load(); n != 0 {
		t.Fatalf("%d parcours", n)
	}
}

// A store refused at startup (or failing): fixed details, nothing of the
// error's text (a path of the host, say) in any answer.
func TestSnapshotStoreErrorsFixedDetail(t *testing.T) {
	fake := snapFake(t)
	bad := snapshot.Unavailable(errors.New("snapshots refused: /srv/hôte/secret-chemin"))
	sf := newSnapFixtureStore(t, false, fake, fake, bad)
	c := sf.session(t)
	for _, m := range []struct{ method, path string }{
		{"POST", "/api/containers/web/snapshots"}, {"GET", "/api/snapshots"}, {"DELETE", "/api/snapshots/" + snapshot.NewID()},
	} {
		rec := sf.do(m.method, m.path, "", c)
		if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "secret-chemin") {
			t.Fatalf("%s %s : %d %s", m.method, m.path, rec.Code, rec.Body)
		}
	}
}

func TestSnapshotContainerGone(t *testing.T) {
	sf, c := snapSetup(t, false)
	sf.do("POST", "/api/containers/web/snapshots", "", c)
	sf.idle(t, c)
	sf.docker.Containers = sf.docker.Containers[1:] // web removed
	l, _ := sf.list(t, c)
	if len(l.Snapshots) != 1 || l.Snapshots[0]["containerGone"] != true {
		t.Fatalf("liste : %+v", l.Snapshots)
	}
}

// httptestRequest is a request of the session c, without Origin.
func httptestRequest(method, path string, c *http.Cookie) *http.Request {
	r := httptest.NewRequest(method, "http://example.com"+path, nil)
	r.RemoteAddr = "192.0.2.1:40000"
	r.AddCookie(c)
	return r
}

func serve(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(deadlineRecorder{rec}, r)
	return rec
}

var hex64 = regexp.MustCompile(`[0-9a-f]{64}`)

func TestSnapshotResponsesNeverContainMAC(t *testing.T) {
	sf, c := snapSetup(t, false)
	var bodies []string
	keep := func(method, path string) {
		bodies = append(bodies, sf.do(method, path, "", c).Body.String())
	}
	keep("POST", "/api/containers/web/snapshots")
	_, body := sf.idle(t, c)
	bodies = append(bodies, body)
	keep("POST", "/api/containers/db/snapshots")
	_, body = sf.idle(t, c)
	bodies = append(bodies, body)
	keep("GET", "/api/containers/web/figures")
	keep("GET", "/api/containers/web/count")
	keep("GET", "/api/containers/db/figures")
	list, err := sf.store.List()
	if err != nil || len(list) != 2 {
		t.Fatalf("magasin : %d %v", len(list), err)
	}
	var macs []string
	for _, h := range list {
		_, seq, err := sf.store.Reader(h.ID)
		if err != nil {
			t.Fatal(err)
		}
		for e, err := range seq {
			if err != nil {
				t.Fatal(err)
			}
			if e.MAC != "" {
				macs = append(macs, e.MAC)
			}
		}
		if h.KeyID == "" {
			t.Fatalf("en-tête sans keyId : %+v", h)
		}
		for _, b := range bodies {
			if strings.Contains(b, h.KeyID) {
				t.Fatalf("keyId dans une réponse : %s", b)
			}
		}
	}
	if len(macs) != 4 {
		t.Fatalf("%d MAC dans les manifestes, attendu 4", len(macs))
	}
	for _, id := range []string{list[0].ID, list[1].ID} {
		keep("DELETE", "/api/snapshots/"+id)
	}
	for _, b := range bodies {
		if strings.Contains(b, `"mac"`) || strings.Contains(b, "keyId") || strings.Contains(b, "s3cr3t") {
			t.Fatalf("réponse : %s", b)
		}
		for _, h := range hex64.FindAllString(b, -1) {
			if slices.Contains(macs, h) {
				t.Fatalf("MAC dans une réponse : %s", b)
			}
		}
	}
}
