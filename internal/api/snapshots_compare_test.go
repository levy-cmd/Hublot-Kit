package api

// Tests of the comparison routes (spec v0.2 §5.7, §6; plan v0.2 tranche 9),
// written before the code: two snapshots, a snapshot and the current
// container (nothing written, thrown away if the container changed, the
// agent's one heavy job), and the match with the image, a bare boolean,
// proved by the HMAC of the image's version read through the extraction
// that checks the blob's digest.

import (
	"archive/tar"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/snapshot"
)

var cmpImg = "sha256:" + imgHex(70)

func hmacHex(key []byte, s string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(s))
	return hex.EncodeToString(m.Sum(nil))
}

var cmpT0 = time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)

func cmpFile(p, mac string, size int64) snapshot.Entry {
	return snapshot.Entry{Path: p, Type: "file", Size: size, Mode: 0o644, MTime: cmpT0, MAC: mac}
}

func cmpDir(p string) snapshot.Entry {
	return snapshot.Entry{Path: p, Type: "dir", Mode: 0o755, MTime: cmpT0}
}

// putSnap writes a snapshot of container straight into the store; keyID ""
// is a photo of the metadata.
func (sf *snapFixture) putSnap(t *testing.T, container, imageID, keyID string, es ...snapshot.Entry) string {
	t.Helper()
	h := snapshot.Header{ID: snapshot.NewID(), ContainerID: container, ContainerName: "web", ImageID: imageID,
		CreatedAt: time.Now().UTC(), State: "running", Source: "procfs", KeyID: keyID}
	if keyID == "" {
		h.Source = "index"
	}
	w, err := sf.store.Create(h)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range es {
		if err := w.Add(e); err != nil {
			t.Fatal(err)
		}
	}
	h.FinishedAt = time.Now().UTC()
	if err := w.Commit(h); err != nil {
		t.Fatal(err)
	}
	return h.ID
}

func (sf *snapFixture) key(t *testing.T) ([]byte, string) {
	t.Helper()
	k, id, err := sf.store.Key()
	if err != nil {
		t.Fatal(err)
	}
	return k, id
}

// poll asks path until it answers anything but 202 (5 s at most), and
// returns every body it got.
func (sf *snapFixture) poll(t *testing.T, path string, c *http.Cookie) (*httptest.ResponseRecorder, []string) {
	t.Helper()
	var bodies []string
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		rec := sf.do("GET", path, "", c)
		bodies = append(bodies, rec.Body.String())
		if rec.Code != http.StatusAccepted {
			return rec, bodies
		}
		if !strings.Contains(rec.Body.String(), `"error":"indexing"`) {
			t.Fatalf("202 sans la forme indexing : %s", rec.Body)
		}
		if time.Now().After(deadline) {
			t.Fatalf("GET %s : toujours 202", path)
		}
	}
}

type compareBody struct {
	A               map[string]any    `json:"a"`
	B               map[string]any    `json:"b"`
	ContentCompared bool              `json:"contentCompared"`
	Results         []snapshot.Result `json:"results"`
	Truncated       bool              `json:"truncated"`
	Unknown         []snapshot.Result `json:"unknown"`
	UnknownCount    *int              `json:"unknownCount"`
}

func decodeCompare(t *testing.T, rec *httptest.ResponseRecorder) (compareBody, map[string]snapshot.Result) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("comparaison : %d %s", rec.Code, rec.Body)
	}
	var b compareBody
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("%v : %s", err, rec.Body)
	}
	by := map[string]snapshot.Result{}
	for _, r := range append(slices.Clone(b.Results), b.Unknown...) {
		by[r.Path] = r
	}
	return b, by
}

// noMAC: no body names a MAC of the store, nor a keyId, nor a content.
func (sf *snapFixture) noMAC(t *testing.T, bodies ...string) {
	t.Helper()
	list, err := sf.store.List()
	if err != nil {
		t.Fatal(err)
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
		for _, b := range bodies {
			if h.KeyID != "" && strings.Contains(b, h.KeyID) {
				t.Fatalf("keyId dans une réponse : %s", b)
			}
		}
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

func TestCompareTwoSnapshots(t *testing.T) {
	sf, c := snapSetup(t, false)
	key, keyID := sf.key(t)
	s1 := sf.putSnap(t, webID, "", keyID, cmpDir("/etc"), cmpFile("/etc/motd", hmacHex(key, "bonjour"), 7),
		cmpFile("/etc/old", hmacHex(key, "x"), 1), cmpFile("/etc/same", hmacHex(key, "s"), 1))
	s2 := sf.putSnap(t, webID, "", keyID, cmpDir("/etc"), cmpFile("/etc/motd", hmacHex(key, "bonsoir"), 7),
		cmpFile("/etc/new", hmacHex(key, "y"), 1), cmpFile("/etc/same", hmacHex(key, "s"), 1))
	rec := sf.do("GET", "/api/snapshots/"+s1+"/compare?with="+s2, "", c)
	b, by := decodeCompare(t, rec)
	if !b.ContentCompared || b.Truncated || b.A["id"] != s1 || b.B["id"] != s2 {
		t.Fatalf("en-tête : %s", rec.Body)
	}
	want := map[string]string{"/etc/motd": "modified", "/etc/new": "added", "/etc/old": "deleted"}
	if len(by) != len(want) {
		t.Fatalf("résultats : %s", rec.Body)
	}
	for p, k := range want {
		if by[p].Kind != k {
			t.Errorf("%s : %+v, attendu %s", p, by[p], k)
		}
	}
	sf.noMAC(t, rec.Body.String())
}

func TestCompareKeyMismatchContentCompared(t *testing.T) {
	sf, c := snapSetup(t, false)
	key, keyID := sf.key(t)
	s1 := sf.putSnap(t, webID, "", keyID, cmpFile("/etc/motd", hmacHex(key, "bonjour"), 7))
	s2 := sf.putSnap(t, webID, "", "ffffffffffffffff", cmpFile("/etc/motd", hmacHex([]byte("autre"), "bonsoir"), 7))
	b, by := decodeCompare(t, sf.do("GET", "/api/snapshots/"+s1+"/compare?with="+s2, "", c))
	if b.ContentCompared || by["/etc/motd"].Kind != "unknown" {
		t.Fatalf("clés différentes : %+v", b)
	}
	// A photo of the metadata has no key at all.
	s3 := sf.putSnap(t, webID, "", "", cmpFile("/etc/motd", "", 7))
	if b, _ := decodeCompare(t, sf.do("GET", "/api/snapshots/"+s1+"/compare?with="+s3, "", c)); b.ContentCompared {
		t.Fatalf("photo des métadonnées : %+v", b)
	}
}

func TestCompareBadIDs(t *testing.T) {
	sf, c := snapSetup(t, false)
	_, keyID := sf.key(t)
	s1 := sf.putSnap(t, webID, "", keyID, cmpDir("/etc"))
	for path, code := range map[string]int{
		"/api/snapshots/..%2Fx/compare?with=current":                                http.StatusNotFound,
		"/api/snapshots/" + strings.ToUpper(s1) + "/compare?with=current":           http.StatusNotFound,
		"/api/snapshots/" + snapshot.NewID() + "/compare?with=" + s1:                http.StatusNotFound,
		"/api/snapshots/" + s1 + "/compare?with=" + snapshot.NewID():                http.StatusNotFound,
		"/api/snapshots/" + s1 + "/compare?with=..%2Fx":                             http.StatusNotFound,
		"/api/snapshots/" + s1 + "/compare":                                         http.StatusBadRequest,
		"/api/snapshots/" + snapshot.NewID() + "/compare?with=current":              http.StatusNotFound,
		"/api/snapshots/" + snapshot.NewID() + "/image-match?path=%2Fetc%2Fmotd":    http.StatusNotFound,
		"/api/snapshots/" + strings.ToUpper(s1) + "/image-match?path=%2Fetc%2Fmotd": http.StatusNotFound,
	} {
		if rec := sf.do("GET", path, "", c); rec.Code != code {
			t.Errorf("GET %s : %d %s, attendu %d", path, rec.Code, rec.Body, code)
		}
	}
}

func TestCompareSessionRequired(t *testing.T) {
	sf, _ := snapSetup(t, false)
	_, keyID := sf.key(t)
	s1 := sf.putSnap(t, webID, "", keyID, cmpDir("/etc"))
	for _, p := range []string{"/compare?with=current", "/compare?with=" + s1, "/image-match?path=%2Fetc"} {
		if rec := sf.do("GET", "/api/snapshots/"+s1+p, "", nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s sans session : %d", p, rec.Code)
		}
	}
	if j, _ := sf.snapJob(); j != nil {
		t.Fatalf("un travail a démarré sans session : %+v", j)
	}
}

// takeWeb takes a snapshot of web (running: the walk of sf.walker) through
// the API, and returns its ID.
func (sf *snapFixture) takeWeb(t *testing.T, c *http.Cookie) string {
	t.Helper()
	rec := sf.do("POST", "/api/containers/web/snapshots", "", c)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("prise : %d %s", rec.Code, rec.Body)
	}
	var b struct{ Snapshot string }
	json.Unmarshal(rec.Body.Bytes(), &b)
	if l, body := sf.idle(t, c); len(l.Snapshots) == 0 || l.Last["result"] != "ok" {
		t.Fatalf("prise : %s", body)
	}
	return b.Snapshot
}

func TestCompareCurrent(t *testing.T) {
	sf, c := snapSetup(t, false)
	sid := sf.takeWeb(t, c)
	sf.fs.AddFile("/etc/motd", []byte("bonsoir")).AddFile("/etc/new", []byte("neuf"))
	rec, bodies := sf.poll(t, "/api/snapshots/"+sid+"/compare?with=current", c)
	b, by := decodeCompare(t, rec)
	if !b.ContentCompared || b.B["id"] != "current" || b.A["id"] != sid {
		t.Fatalf("en-tête : %s", rec.Body)
	}
	if by["/etc/motd"].Kind != "modified" || by["/etc/new"].Kind != "added" || len(by) != 2 {
		t.Fatalf("résultats : %s", rec.Body)
	}
	// Kept: asked again, nothing is walked again.
	walks := sf.walker.walks.Load()
	if rec := sf.do("GET", "/api/snapshots/"+sid+"/compare?with=current", "", c); rec.Code != http.StatusOK {
		t.Fatalf("relance : %d %s", rec.Code, rec.Body)
	}
	if n := sf.walker.walks.Load(); n != walks {
		t.Fatalf("parcours refait : %d → %d", walks, n)
	}
	sf.noMAC(t, append(bodies, rec.Body.String())...)
}

// A snapshot of another key (rotated since): the current container is
// compared by its metadata, without reading any content.
func TestCompareCurrentOtherKey(t *testing.T) {
	sf, c := snapSetup(t, false)
	key, _ := sf.key(t)
	sid := sf.putSnap(t, webID, "", "ffffffffffffffff", cmpDir("/etc"), cmpFile("/etc/motd", hmacHex(key, "autre"), 7))
	rec, _ := sf.poll(t, "/api/snapshots/"+sid+"/compare?with=current", c)
	b, by := decodeCompare(t, rec)
	if b.ContentCompared || by["/etc/motd"].Kind == "modified" {
		t.Fatalf("contenu comparé avec une autre clé : %s", rec.Body)
	}
	if n := sf.walker.read.Load(); n != 0 {
		t.Fatalf("%d octets lus pour une comparaison sans empreinte", n)
	}
}

// Without a write deadline, nothing is compared nor sent (v0.1.2: fail
// closed).
func TestCompareNoWriteDeadline(t *testing.T) {
	sf, c := snapSetup(t, false)
	key, keyID := sf.key(t)
	s1 := sf.putSnap(t, webID, "", keyID, cmpFile("/etc/motd", hmacHex(key, "a"), 1))
	s2 := sf.putSnap(t, webID, "", keyID, cmpFile("/etc/new", hmacHex(key, "b"), 1))
	for _, with := range []string{s2, "current"} {
		r := httptest.NewRequest("GET", "http://example.com/api/snapshots/"+s1+"/compare?with="+with, nil)
		r.AddCookie(c)
		rec := httptest.NewRecorder()
		sf.h.ServeHTTP(rec, r)
		if rec.Code == http.StatusOK || rec.Code == http.StatusAccepted || strings.Contains(rec.Body.String(), "/etc/") {
			t.Errorf("with=%s sans délai d'écriture : %d %q", with, rec.Code, rec.Body)
		}
	}
	if j, _ := sf.snapJob(); j != nil {
		t.Fatalf("un travail a démarré : %+v", j)
	}
}

// A result describes one state of the container: restarted since, it is
// computed again.
func TestCompareCurrentKeptPerState(t *testing.T) {
	sf, c := snapSetup(t, false)
	sid := sf.takeWeb(t, c)
	path := "/api/snapshots/" + sid + "/compare?with=current"
	rec, _ := sf.poll(t, path, c)
	decodeCompare(t, rec)
	walks := sf.walker.walks.Load()
	web := sf.docker.Infos[webID]
	web.Details.StartedAt = "2026-10-24T09:00:00Z"
	sf.docker.Infos[webID], sf.docker.Infos["web"] = web, web
	rec, _ = sf.poll(t, path, c)
	decodeCompare(t, rec)
	if n := sf.walker.walks.Load(); n != walks+1 {
		t.Fatalf("après un redémarrage : %d parcours, attendu %d", n, walks+1)
	}
}

// tree is the state of a directory: names, sizes, modes, modification times.
func tree(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		out = append(out, p+" "+fi.Mode().String()+" "+fi.ModTime().String()+" "+fmt.Sprint(fi.Size()))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCompareCurrentWritesNothing(t *testing.T) {
	sf, c := snapSetup(t, false)
	sid := sf.takeWeb(t, c)
	sf.fs.AddFile("/etc/motd", []byte("bonsoir"))
	auditLog := filepath.Join(sf.data, "audit.log")
	before := tree(t, filepath.Join(sf.data, "snapshots"))
	auditBefore, _ := os.ReadFile(auditLog)
	keyBefore := tree(t, sf.data)
	rec, _ := sf.poll(t, "/api/snapshots/"+sid+"/compare?with=current", c)
	decodeCompare(t, rec)
	if after := tree(t, filepath.Join(sf.data, "snapshots")); !slices.Equal(before, after) {
		t.Fatalf("snapshots/ a changé :\n%v\n%v", before, after)
	}
	if after := tree(t, sf.data); !slices.Equal(keyBefore, after) {
		t.Fatalf("dossier de données changé :\n%v\n%v", keyBefore, after)
	}
	if after, _ := os.ReadFile(auditLog); string(after) != string(auditBefore) {
		t.Fatalf("journal écrit par une lecture :\n%s", after[len(auditBefore):])
	}
}

// Without the key (deleted since), the comparison against the current
// container never creates one: metadata only, and nothing written.
func TestCompareCurrentNoKeyCreatesNone(t *testing.T) {
	sf, c := snapSetup(t, false)
	sid := sf.putSnap(t, webID, "", "0123456789abcdef", cmpDir("/etc"), cmpFile("/etc/motd", "00", 7))
	if err := os.Remove(filepath.Join(sf.data, "snapshot.key")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	rec, _ := sf.poll(t, "/api/snapshots/"+sid+"/compare?with=current", c)
	b, _ := decodeCompare(t, rec)
	if b.ContentCompared {
		t.Fatalf("contenu comparé sans clé : %s", rec.Body)
	}
	if _, err := os.Lstat(filepath.Join(sf.data, "snapshot.key")); !os.IsNotExist(err) {
		t.Fatalf("clé créée par une comparaison : %v", err)
	}
	if n := sf.walker.read.Load(); n != 0 {
		t.Fatalf("%d octets lus pour une comparaison sans empreinte", n)
	}
}

func TestCompareCurrentContainerChanged(t *testing.T) {
	fake := snapFake(t)
	sf := newSnapFixture(t, false, &changingDocker{Fake: fake}, fake)
	c := sf.session(t)
	_, keyID := sf.key(t)
	sid := sf.putSnap(t, webID, "", keyID, cmpDir("/etc"))
	// The walk ends after the first answer (202): the error reaches a poll,
	// which must get it without asking Docker again (whose answer changed).
	sf.walker.gate = make(chan struct{})
	path := "/api/snapshots/" + sid + "/compare?with=current"
	first := sf.do("GET", path, "", c)
	close(sf.walker.gate)
	if first.Code != http.StatusAccepted {
		t.Fatalf("première réponse : %d %s", first.Code, first.Body)
	}
	rec, bodies := sf.poll(t, path, c)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"container_changed"`) {
		t.Fatalf("conteneur arrêté pendant le parcours : %d %s", rec.Code, rec.Body)
	}
	for _, b := range bodies {
		if strings.Contains(b, `"results"`) {
			t.Fatalf("résultat servi d'un parcours jeté : %s", b)
		}
	}
	if got := sf.files(t); len(got) != 1 {
		t.Fatalf("fichiers : %v", got)
	}
}

// Review of tranche 9 (auditor, proved): the unknown of a photo of the
// metadata are counted apart; 20 000 of them never hide the file added.
func TestCompareUnknownApart(t *testing.T) {
	sf, c := snapSetup(t, false)
	var es []snapshot.Entry
	for i := range 20_000 {
		es = append(es, cmpFile(fmt.Sprintf("/f%05d", i), "", 1))
	}
	s1 := sf.putSnap(t, webID, "", "", es...)
	s2 := sf.putSnap(t, webID, "", "", append(es, cmpFile("/zz-added", "", 1))...)
	rec := sf.do("GET", "/api/snapshots/"+s1+"/compare?with="+s2, "", c)
	b, by := decodeCompare(t, rec)
	if b.Truncated || len(b.Results) != 1 || by["/zz-added"].Kind != "added" {
		t.Fatalf("changements : %+v, tronqué %v", b.Results, b.Truncated)
	}
	if b.UnknownCount == nil || *b.UnknownCount != 20_000 || len(b.Unknown) != snapshot.MaxUnknownPaths {
		t.Fatalf("inconnus : %v comptés, %d listés", b.UnknownCount, len(b.Unknown))
	}
	for _, r := range b.Unknown {
		if r.Kind != "unknown" {
			t.Fatalf("liste des inconnus : %+v", r)
		}
	}
	// Nothing unknown: the fields are there all the same.
	key, keyID := sf.key(t)
	s3 := sf.putSnap(t, webID, "", keyID, cmpFile("/f", hmacHex(key, "a"), 1))
	b, _ = decodeCompare(t, sf.do("GET", "/api/snapshots/"+s3+"/compare?with="+s3, "", c))
	if b.UnknownCount == nil || *b.UnknownCount != 0 || b.Unknown == nil {
		t.Fatalf("sans inconnu : %+v", b)
	}
}

// gatedInspect holds the first Inspect of web until open is closed.
type gatedInspect struct {
	*dockerx.Fake
	open    chan struct{}
	entered chan struct{}
	n       atomic.Int32
}

func (d *gatedInspect) Inspect(ctx context.Context, id string) (dockerx.ContainerInfo, error) {
	if d.n.Add(1) == 1 {
		close(d.entered)
		<-d.open
	}
	return d.Fake.Inspect(ctx, id)
}

// Review of tranche 9 (auditor, proved): a snapshot that takes the agent's
// slot while the comparison waits for its Inspect. The comparison must be
// refused (409), never run without the slot: its job would have no done
// channel, and its end would close a nil channel (a panic of the agent).
func TestCompareCurrentClaimRace(t *testing.T) {
	fake := snapFake(t)
	g := &gatedInspect{Fake: fake, open: make(chan struct{}), entered: make(chan struct{})}
	sf := newSnapFixture(t, false, g, fake)
	c := sf.session(t)
	_, keyID := sf.key(t)
	sid := sf.putSnap(t, webID, "", keyID, cmpDir("/etc"))
	got := make(chan *httptest.ResponseRecorder, 1)
	go func() { got <- sf.do("GET", "/api/snapshots/"+sid+"/compare?with=current", "", c) }()
	<-g.entered // past the check of the job in progress, in its Inspect
	sf.walker.gate = make(chan struct{})
	if rec := sf.do("POST", "/api/containers/web/snapshots", "", c); rec.Code != http.StatusAccepted {
		t.Fatalf("prise : %d %s", rec.Code, rec.Body)
	}
	close(g.open)
	rec := <-got
	if rec.Code != http.StatusConflict {
		t.Fatalf("comparaison sans la place de l'agent : %d %s", rec.Code, rec.Body)
	}
	close(sf.walker.gate)
	if l, body := sf.idle(t, c); l.Last["result"] != "ok" {
		t.Fatalf("prise : %s", body)
	}
}

// The comparison against the current container is the agent's one heavy
// job: no snapshot nor count meanwhile, and a poll answers 202 with the
// progress without asking Docker.
type cmpInspects struct {
	*dockerx.Fake
	n atomic.Int32
}

func (d *cmpInspects) Inspect(ctx context.Context, id string) (dockerx.ContainerInfo, error) {
	d.n.Add(1)
	return d.Fake.Inspect(ctx, id)
}

func TestCompareCurrentOneAtATime(t *testing.T) {
	fake := snapFake(t)
	counter := &cmpInspects{Fake: fake}
	sf := newSnapFixture(t, false, counter, fake)
	c := sf.session(t)
	sid := sf.takeWeb(t, c)
	sf.walker.gate = make(chan struct{})
	path := "/api/snapshots/" + sid + "/compare?with=current"
	if rec := sf.do("GET", path, "", c); rec.Code != http.StatusAccepted {
		t.Fatalf("comparaison : %d %s", rec.Code, rec.Body)
	}
	if j, _ := sf.snapJob(); j == nil {
		t.Fatal("aucun travail en cours")
	}
	if rec := sf.do("POST", "/api/containers/web/snapshots", "", c); rec.Code != http.StatusConflict {
		t.Errorf("prise pendant la comparaison : %d %s", rec.Code, rec.Body)
	}
	if rec := sf.do("GET", "/api/containers/db/count", "", c); rec.Code != http.StatusConflict {
		t.Errorf("comptage pendant la comparaison : %d %s", rec.Code, rec.Body)
	}
	_, keyID := sf.key(t)
	other := sf.putSnap(t, webID, "", keyID, cmpDir("/etc"))
	if rec := sf.do("GET", "/api/snapshots/"+other+"/compare?with=current", "", c); rec.Code != http.StatusConflict {
		t.Errorf("seconde comparaison à l'actuel : %d %s", rec.Code, rec.Body)
	}
	calls := counter.n.Load()
	if rec := sf.do("GET", path, "", c); rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"progress"`) {
		t.Errorf("relance : %d %s", rec.Code, rec.Body)
	}
	if n := counter.n.Load(); n != calls {
		t.Errorf("une relance a interrogé Docker : %d Inspect", n-calls)
	}
	l, _ := sf.list(t, c)
	if l.Current == nil || l.Current["kind"] != "compare" || l.Current["containerName"] != "web" {
		t.Errorf("travail en cours dans la liste : %+v", l.Current)
	}
	close(sf.walker.gate)
	rec, _ := sf.poll(t, path, c)
	decodeCompare(t, rec)
}

// imageMatchFixture: web's image is cmpImg, whose export is archive; a
// snapshot of web whose MACs are those of the contents given.
func imageMatchFixture(t *testing.T, docker func(*dockerx.Fake) dockerx.Client, archive []byte, contents map[string]string) (*snapFixture, *http.Cookie, string) {
	t.Helper()
	fake := snapFake(t)
	fake.Images = map[string]dockerx.ImageInfo{cmpImg: {ID: cmpImg, Size: int64(len(archive))}}
	fake.ImageSaves = map[string][]byte{cmpImg: archive}
	var d dockerx.Client = fake
	if docker != nil {
		d = docker(fake)
	}
	sf := newSnapFixture(t, false, d, fake)
	c := sf.session(t)
	key, keyID := sf.key(t)
	es := []snapshot.Entry{cmpDir("/etc")}
	for _, p := range []string{"/etc/motd", "/etc/os-release"} {
		if s, ok := contents[p]; ok {
			es = append(es, cmpFile(p, hmacHex(key, s), int64(len(s))))
		}
	}
	return sf, c, sf.putSnap(t, webID, cmpImg, keyID, es...)
}

func matchImage(t *testing.T) []byte {
	return saveOf(t, gzLayer(t,
		tarItem{name: "etc/", typ: tar.TypeDir},
		tarItem{name: "etc/motd", body: "bonjour"},
		tarItem{name: "etc/os-release", body: "NAME=hublot-test\n"}))
}

func matchPath(sid, p string) string { return "/api/snapshots/" + sid + "/image-match?path=" + p }

func TestImageMatch(t *testing.T) {
	// motd: same size as the image's, another content.
	sf, c, sid := imageMatchFixture(t, nil, matchImage(t), map[string]string{"/etc/os-release": "NAME=hublot-test\n", "/etc/motd": "bonsoir"})
	for p, want := range map[string]bool{"%2Fetc%2Fos-release": true, "%2Fetc%2Fmotd": false, "%2Fetc": false, "%2Fabsent": false} {
		rec, _ := sf.poll(t, matchPath(sid, p), c)
		var b struct{ Match *bool }
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &b) != nil || b.Match == nil || *b.Match != want {
			t.Errorf("%s : %d %s, attendu %v", p, rec.Code, rec.Body, want)
		}
	}
	// A snapshot that names another key never matches, whatever its MAC
	// (here, the one the agent's key would give).
	key, _ := sf.key(t)
	other := sf.putSnap(t, webID, cmpImg, "ffffffffffffffff", cmpFile("/etc/os-release", hmacHex(key, "NAME=hublot-test\n"), 17))
	if rec, _ := sf.poll(t, matchPath(other, "%2Fetc%2Fos-release"), c); strings.TrimSpace(rec.Body.String()) != `{"match":false}` {
		t.Errorf("autre clé : %d %s", rec.Code, rec.Body)
	}
	// A photo of the metadata has no MAC: false, without asking Docker.
	saves := sf.docker.ImageSaveCalls.Load()
	meta := sf.putSnap(t, webID, cmpImg, "", cmpFile("/etc/os-release", "", 17))
	if rec := sf.do("GET", matchPath(meta, "%2Fetc%2Fos-release"), "", c); strings.TrimSpace(rec.Body.String()) != `{"match":false}` {
		t.Errorf("photo des métadonnées : %d %s", rec.Code, rec.Body)
	}
	if n := sf.docker.ImageSaveCalls.Load(); n != saves {
		t.Errorf("export demandé pour une entrée sans MAC")
	}
}

// A file noted without MAC (budget, too large) proves nothing: false,
// without asking Docker anything.
func TestImageMatchSkippedMAC(t *testing.T) {
	sf, c, _ := imageMatchFixture(t, nil, matchImage(t), nil)
	_, keyID := sf.key(t)
	skipped := cmpFile("/etc/os-release", "", 17)
	skipped.MACSkipped = snapshot.SkipBudget
	sid := sf.putSnap(t, webID, cmpImg, keyID, skipped)
	rec := sf.do("GET", matchPath(sid, "%2Fetc%2Fos-release"), "", c)
	if strings.TrimSpace(rec.Body.String()) != `{"match":false}` {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if n, m := sf.docker.ImageSaveCalls.Load(), sf.docker.ImageCalls.Load(); n+m != 0 {
		t.Fatalf("Docker interrogé : %d exports, %d inspect", n, m)
	}
}

// A size other than the image's says false from the index: no extraction
// (one export only, the index's).
func TestImageMatchSizeFromIndex(t *testing.T) {
	sf, c, sid := imageMatchFixture(t, nil, matchImage(t), map[string]string{"/etc/motd": "bonjour!"})
	rec, _ := sf.poll(t, matchPath(sid, "%2Fetc%2Fmotd"), c)
	if strings.TrimSpace(rec.Body.String()) != `{"match":false}` {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if n := sf.docker.ImageSaveCalls.Load(); n != 1 {
		t.Fatalf("%d exports, attendu 1 (l'index seul)", n)
	}
}

func TestImageMatchOnlyBool(t *testing.T) {
	sf, c, sid := imageMatchFixture(t, nil, matchImage(t), map[string]string{"/etc/os-release": "NAME=hublot-test\n", "/etc/motd": "bonsoir"})
	for p, want := range map[string]string{"%2Fetc%2Fos-release": `{"match":true}`, "%2Fetc%2Fmotd": `{"match":false}`} {
		rec, bodies := sf.poll(t, matchPath(sid, p), c)
		if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != want {
			t.Errorf("%s : %d %q, attendu exactement %s", p, rec.Code, rec.Body, want)
		}
		sf.noMAC(t, bodies...)
	}
}

// alteredSaves answers the first export as the fake, every later one with
// altered: the layer is changed between the index and the extraction.
type alteredSaves struct {
	*dockerx.Fake
	altered []byte
	n       atomic.Int32
}

func (a *alteredSaves) ImageSave(ctx context.Context, id string) (io.ReadCloser, error) {
	if a.n.Add(1) == 1 {
		return a.Fake.ImageSave(ctx, id)
	}
	a.Fake.ImageSaveCalls.Add(1)
	return io.NopCloser(strings.NewReader(string(a.altered))), nil
}

func TestImageMatchVerifiesBlob(t *testing.T) {
	good := gzLayer(t, tarItem{name: "etc/", typ: tar.TypeDir}, tarItem{name: "etc/os-release", body: "NAME=hublot-test\n"})
	evil := gzLayer(t, tarItem{name: "etc/", typ: tar.TypeDir}, tarItem{name: "etc/os-release", body: "NAME=hublot-evil\n"})
	// The altered export: the evil layer under the good layer's name.
	config := []byte(`{"os":"linux"}`)
	m, _ := json.Marshal([]map[string]any{{"Config": "blobs/sha256/" + sha(config), "Layers": []string{"blobs/sha256/" + sha(good)}}})
	altered := tarOf(t, tarItem{name: "blobs/sha256/" + sha(good), body: string(evil)},
		tarItem{name: "blobs/sha256/" + sha(config), body: string(config)},
		tarItem{name: "oci-layout", body: `{"imageLayoutVersion":"1.0.0"}`}, tarItem{name: "manifest.json", body: string(m)})
	// The snapshot's MAC is the evil content's: without the check of the
	// digest, the match would be true.
	sf, c, sid := imageMatchFixture(t, func(f *dockerx.Fake) dockerx.Client { return &alteredSaves{Fake: f, altered: altered} },
		saveOf(t, good), map[string]string{"/etc/os-release": "NAME=hublot-evil\n"})
	rec, bodies := sf.poll(t, matchPath(sid, "%2Fetc%2Fos-release"), c)
	for _, b := range bodies {
		if strings.Contains(b, `"match":true`) {
			t.Fatalf("correspondance avec une couche altérée : %s", b)
		}
	}
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"unsupported_image"`) {
		t.Fatalf("couche altérée : %d %s, attendu 422 unsupported_image", rec.Code, rec.Body)
	}
}
