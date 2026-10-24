package api

// Read only on Docker's side (plan v0.2, tranche 10, step 1): every route
// of the agent, v0.1 and v0.2, is called against a Docker that records
// each method; only reads may come out. PUT fs/file and POST fs/upload,
// the two writes of spec §5.3, are the one exception, kept out of this test.
// The routes come from the router's own source (server.go and the
// routes_<module>.go of moduleRoutes), never from a list written here: a
// route added later is called without touching this test.

import (
	"archive/tar"
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// roWrites: the methods of dockerx.Client that write in Docker. Every
// other method must be listed in roReads: a method added to the interface
// fails this test until it is sorted.
var (
	roWrites = map[string]bool{"PutArchive": true}
	roReads  = map[string]bool{
		"ListContainers": true, "Ping": true, "Inspect": true, "GetArchive": true, "VolumeOptions": true,
		"SizeRootFs": true, "Changes": true, "Logs": true, "Info": true, "ContainerSize": true,
		"ImageInspect": true, "ImageList": true, "ImageHistory": true, "ImageSave": true,
	}
	// roAllowedWrites: the routes that may call a write, not called here.
	roAllowedWrites = []string{"PUT /api/containers/{id}/fs/file", "POST /api/containers/{id}/fs/upload"}
)

// roDocker records the name of every method called, then answers by the
// fake; a write is recorded and refused, never passed on.
type roDocker struct {
	inner *dockerx.Fake
	mu    sync.Mutex
	calls map[string]int
	by    map[string][]string // method → routes that called it (route set by the test)
	route string
}

var _ dockerx.Client = (*roDocker)(nil)

func (d *roDocker) rec(m string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls[m]++
	if !slices.Contains(d.by[m], d.route) {
		d.by[m] = append(d.by[m], d.route)
	}
}

func (d *roDocker) on(route string) {
	d.mu.Lock()
	d.route = route
	d.mu.Unlock()
}

func (d *roDocker) ListContainers(ctx context.Context) ([]dockerx.Container, error) {
	d.rec("ListContainers")
	return d.inner.ListContainers(ctx)
}
func (d *roDocker) Ping(ctx context.Context) error { d.rec("Ping"); return d.inner.Ping(ctx) }
func (d *roDocker) Inspect(ctx context.Context, id string) (dockerx.ContainerInfo, error) {
	d.rec("Inspect")
	return d.inner.Inspect(ctx, id)
}
func (d *roDocker) GetArchive(ctx context.Context, id, path string) (io.ReadCloser, error) {
	d.rec("GetArchive")
	return d.inner.GetArchive(ctx, id, path)
}
func (d *roDocker) VolumeOptions(ctx context.Context, name string) (map[string]string, error) {
	d.rec("VolumeOptions")
	return d.inner.VolumeOptions(ctx, name)
}
func (d *roDocker) SizeRootFs(ctx context.Context, id string) (int64, error) {
	d.rec("SizeRootFs")
	return d.inner.SizeRootFs(ctx, id)
}
func (d *roDocker) Changes(ctx context.Context, id string) ([]dockerx.Change, error) {
	d.rec("Changes")
	return d.inner.Changes(ctx, id)
}
func (d *roDocker) PutArchive(context.Context, string, string, io.Reader) error {
	d.rec("PutArchive")
	return dockerx.ErrUnavailable
}
func (d *roDocker) Logs(ctx context.Context, id string, opt dockerx.LogsOptions) (io.ReadCloser, error) {
	d.rec("Logs")
	return d.inner.Logs(ctx, id, opt)
}
func (d *roDocker) Info(ctx context.Context) (dockerx.DaemonInfo, error) {
	d.rec("Info")
	return d.inner.Info(ctx)
}
func (d *roDocker) ContainerSize(ctx context.Context, id string) (rw, rootfs *int64, err error) {
	d.rec("ContainerSize")
	return d.inner.ContainerSize(ctx, id)
}
func (d *roDocker) ImageInspect(ctx context.Context, id string) (dockerx.ImageInfo, error) {
	d.rec("ImageInspect")
	return d.inner.ImageInspect(ctx, id)
}
func (d *roDocker) ImageList(ctx context.Context) ([]dockerx.ImageSummary, error) {
	d.rec("ImageList")
	return d.inner.ImageList(ctx)
}
func (d *roDocker) ImageHistory(ctx context.Context, id string) ([]dockerx.ImageLayer, error) {
	d.rec("ImageHistory")
	return d.inner.ImageHistory(ctx, id)
}
func (d *roDocker) ImageSave(ctx context.Context, id string) (io.ReadCloser, error) {
	d.rec("ImageSave")
	return d.inner.ImageSave(ctx, id)
}

// roRoute is a pattern of the router and the file that registers it.
type roRoute struct{ pattern, file string }

// routerRoutes reads every Handle and HandleFunc call of the package's
// source (tests aside): its pattern must be a string literal, so that no
// route escapes the reading.
func routerRoutes(t *testing.T) []roRoute {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var out []roRoute
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "Handle" && sel.Sel.Name != "HandleFunc") || len(call.Args) != 2 {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				t.Fatalf("%s : motif de route qui n'est pas un littéral, illisible pour ce test", fset.Position(call.Pos()))
			}
			p, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, roRoute{pattern: p, file: name})
			return true
		})
	}
	return out
}

// registered says whether pattern is already in mux: a second
// registration of the same pattern panics.
func registered(mux *http.ServeMux, pattern string) (yes bool) {
	defer func() { yes = recover() != nil }()
	mux.Handle(pattern, http.NotFoundHandler())
	return false
}

// roHint is the request a route gets: query and body. A route without a
// hint is called bare; the route list never comes from here.
var roHint = map[string]struct{ query, body string }{
	"GET /api/containers/{id}/fs":            {query: "path=/etc"},
	"GET /api/containers/{id}/fs/file":       {query: "path=/etc/motd"},
	"GET /api/containers/{id}/fs/download":   {query: "path=/etc/motd"},
	"POST /api/containers/{id}/env/reveal":   {body: `{"name":"API_KEY"}`},
	"POST /api/containers/{id}/fs/search":    {body: `{"q":"motd"}`},
	"GET /api/images/{id}/fs":                {query: "path=/etc"},
	"GET /api/images/{id}/fs/file":           {query: "path=/etc/os-release"},
	"POST /api/images/{id}/fs/search":        {body: `{"q":"os"}`},
	"GET /api/snapshots/{sid}/compare":       {query: "with=current"},
	"GET /api/snapshots/{sid}/image-match":   {query: "path=/etc/motd"},
	"GET /api/containers/{id}/logs/download": {query: "tail=10"},
	"GET /api/containers/{id}/logs":          {query: "tail=10"},
	"POST /api/containers/{id}/snapshots":    {},
	"GET /api/containers/{id}/changes":       {},
	"GET /api/containers/{id}/count":         {},
	"GET /api/containers/{id}/figures":       {},
	"DELETE /api/snapshots/{sid}":            {},
	"GET /api/snapshots":                     {},
	"GET /api/secrets":                       {},
	"GET /api/images":                        {},
	"GET /api/images/{id}":                   {},
	"GET /api/meta":                          {},
	"GET /api/containers":                    {},
	"GET /api/containers/{id}":               {},
	"POST /api/login":                        {},
	"POST /api/logout":                       {},
}

var roWildcard = regexp.MustCompile(`\{[^}]*\}`)

func TestNoDockerWritesOutsideFiles(t *testing.T) {
	routes := routerRoutes(t)
	var patterns []string
	for _, r := range routes {
		patterns = append(patterns, r.pattern)
	}
	for _, p := range roAllowedWrites {
		if !slices.Contains(patterns, p) {
			t.Fatalf("route d'écriture %q introuvable dans le routeur : la liste des exceptions est périmée", p)
		}
	}
	if len(patterns) < 20 {
		t.Fatalf("%d routes lues seulement : %v", len(patterns), patterns)
	}
	// Every method of the Client is sorted, read or write.
	ct := reflect.TypeFor[dockerx.Client]()
	for i := range ct.NumMethod() {
		if m := ct.Method(i).Name; roReads[m] == roWrites[m] {
			t.Fatalf("méthode %s de dockerx.Client ni lecture ni écriture : à classer ici", m)
		}
	}

	fake := snapFake(t)
	fake.NoWrites = true
	web := fake.Infos[webID]
	web.Details.Env = []string{"API_KEY=s3cr3t", "PATH=/usr/bin"}
	web.Details.ImageID = fsImg
	fake.Infos["web"], fake.Infos[webID] = web, web
	db := fake.Infos[dbID]
	db.Details.ImageID = fsImg
	fake.Infos["db"], fake.Infos[dbID] = db, db
	save := saveOf(t, gzLayer(t,
		tarItem{name: "etc/", typ: tar.TypeDir},
		tarItem{name: "etc/motd", body: "salut"},
		tarItem{name: "etc/os-release", body: "NAME=hublot-test\n"}))
	fake.Images = map[string]dockerx.ImageInfo{fsImg: {ID: fsImg, Size: int64(len(save))}}
	fake.ImageSummaries = []dockerx.ImageSummary{{ID: fsImg, Tags: []string{"demo:1"}, Size: 10, Created: time.Unix(1_700_000_000, 0)}}
	fake.Histories = map[string][]dockerx.ImageLayer{fsImg: {{CreatedBy: "/bin/sh -c #(nop) ADD file:abc in / ", Size: 10}}}
	fake.ImageSaves = map[string][]byte{fsImg: save}
	fake.Diffs = map[string][]dockerx.Change{webID: {{Path: "/etc/motd", Kind: "modified"}}, dbID: {}}
	fake.LogStreams = map[string][]byte{webID: mux(1, lts1+" out1\n"), dbID: mux(1, lts1+" out1\n")}
	n := int64(100)
	fake.ContainerSizes = map[string]dockerx.SizePair{webID: {RW: &n, RootFs: &n}, dbID: {RW: &n, RootFs: &n}}
	fake.Daemon = dockerx.DaemonInfo{Host: "unix:///var/run/docker.sock", ServerVersion: "29.8.2", APIVersion: "1.44", Connected: true}

	rd := &roDocker{inner: fake, calls: map[string]int{}, by: map[string][]string{}}
	sf := newSnapFixture(t, false, rd, fake)
	c := sf.session(t)

	// The patterns of moduleRoutes read from the source are those the
	// server registers: registered again, each one panics.
	again := http.NewServeMux()
	for _, register := range moduleRoutes {
		register(sf.srv, again)
	}
	for _, r := range routes {
		if r.file != "server.go" && !registered(again, r.pattern) {
			t.Fatalf("%s (%s) : lu dans la source, absent de moduleRoutes", r.pattern, r.file)
		}
	}

	// Order: the routes without a snapshot, the captures, the routes of a
	// snapshot, then its deletion; login and logout last (logout ends the
	// session).
	rank := func(p string) int {
		switch {
		case strings.HasPrefix(p, "POST /api/login"), strings.HasPrefix(p, "POST /api/logout"):
			return 5
		case strings.HasPrefix(p, "DELETE "):
			return 4
		case strings.Contains(p, "{sid}"):
			return 3
		case strings.HasSuffix(p, "/snapshots") && strings.HasPrefix(p, "POST "):
			return 2
		}
		return 1
	}
	slices.SortStableFunc(routes, func(a, b roRoute) int { return rank(a.pattern) - rank(b.pattern) })

	var sid string
	called := 0
	for _, r := range routes {
		if slices.Contains(roAllowedWrites, r.pattern) {
			continue
		}
		method, path, ok := strings.Cut(r.pattern, " ")
		if !ok { // the catch-all "/api/" and the UI "/"
			method, path = "GET", r.pattern
		}
		hint := roHint[r.pattern]
		if r.pattern == "POST /api/login" {
			hint.body = `{"token":"` + sf.token + `"}`
		}
		var targets []string
		switch {
		case strings.Contains(path, "{sid}"):
			if sid == "" {
				t.Fatalf("%s : aucune photo prise avant", r.pattern)
			}
			targets = []string{strings.ReplaceAll(path, "{sid}", sid)}
		case strings.HasPrefix(path, "/api/images/{id}"):
			targets = []string{strings.ReplaceAll(path, "{id}", fsImg)}
		case strings.Contains(path, "{id}"):
			targets = []string{strings.ReplaceAll(path, "{id}", webID), strings.ReplaceAll(path, "{id}", dbID)}
		default:
			targets = []string{path}
		}
		for _, target := range targets {
			if w := roWildcard.FindString(target); w != "" {
				t.Fatalf("%s : joker %s sans valeur dans ce test", r.pattern, w)
			}
			if hint.query != "" {
				target += "?" + hint.query
			}
			rd.on(r.pattern)
			called++
			rec := roDo(t, sf, method, target, hint.body, c)
			if rank(r.pattern) == 2 {
				sf.idle(t, c)
				if l, _ := sf.list(t, c); len(l.Snapshots) > 0 {
					sid, _ = l.Snapshots[0]["id"].(string)
				}
				// The photo of the metadata too, from the index built above.
				if strings.Contains(target, dbID) {
					roDo(t, sf, method, target+"?source=index", "", c)
					sf.idle(t, c)
				}
			}
			t.Logf("%s %s → %d", method, target, rec.Code)
		}
	}
	if called == 0 {
		t.Fatal("aucune route appelée")
	}

	rd.mu.Lock()
	defer rd.mu.Unlock()
	for m, routes := range rd.by {
		if !roReads[m] {
			t.Errorf("écriture dans Docker : %s appelé par %v", m, routes)
		}
	}
	// The test reaches Docker for real: the reads of each feature are seen.
	for _, m := range []string{"ListContainers", "Inspect", "GetArchive", "Changes", "Logs", "Info", "ContainerSize",
		"ImageInspect", "ImageList", "ImageHistory", "ImageSave"} {
		if rd.calls[m] == 0 {
			t.Errorf("%s jamais appelé : le test n'atteint pas Docker par cette voie", m)
		}
	}
	if t.Failed() {
		t.Logf("appels : %v", rd.calls)
	}
}

// roDo sends one request, under a delay (a logs stream), and polls while
// it answers 202 (an index, a count, a comparison in progress); a capture
// is waited for by the caller.
func roDo(t *testing.T, sf *snapFixture, method, target, body string, c *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		r := httptest.NewRequestWithContext(ctx, method, "http://example.com"+target, strings.NewReader(body))
		r.RemoteAddr = "192.0.2.1:40000"
		if method != "GET" {
			r.Header.Set("Origin", "http://example.com")
		}
		r.AddCookie(c)
		rec := httptest.NewRecorder()
		sf.h.ServeHTTP(deadlineRecorder{rec}, r)
		cancel()
		if rec.Code != http.StatusAccepted || strings.HasSuffix(strings.Split(target, "?")[0], "/snapshots") {
			return rec
		}
		var b map[string]any
		if json.Unmarshal(rec.Body.Bytes(), &b) != nil || time.Now().After(deadline) {
			t.Fatalf("%s %s : %d %s", method, target, rec.Code, rec.Body)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
