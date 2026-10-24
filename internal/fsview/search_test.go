package fsview

// Tests of SearchCached (spec v0.2 §5.8, Q61), written before the code: the
// file search reads only an index already served to the explorer, never
// builds one, and returns only names the explorer shows.

import (
	"archive/tar"
	"context"
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// servedViews: the stopped container "db" whose archive of "/" holds hs,
// its index built and served once, as the explorer does.
func servedViews(t *testing.T, hs ...hdr) (*Views, *dockerx.Fake, dockerx.ContainerInfo) {
	t.Helper()
	f := stoppedDocker(t)
	f.Archives[vID+":/"] = mkTar(t, hs...)
	v := NewViews(f, Options{})
	ready(t, v, "db")
	return v, f, f.Infos["db"]
}

func hitPaths(hits []SearchHit) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.Path)
	}
	return out
}

// Spec v0.2 §5.8: the search never starts a build, nor a walk; only an
// index already served answers, under the key For uses (ID and FinishedAt).
func TestSearchCachedNeverBuilds(t *testing.T) {
	g := &gated{Fake: stoppedDocker(t), open: make(chan struct{})}
	v := NewViews(g, Options{})
	info := g.Infos["db"]
	none := func(what string, info dockerx.ContainerInfo) {
		t.Helper()
		hits, truncated, indexed := v.SearchCached(tctx, info, "etc", 10)
		if indexed || truncated || len(hits) != 0 {
			t.Errorf("%s : indexed=%v truncated=%v hits=%v", what, indexed, truncated, hits)
		}
	}
	none("aucun index", info)
	if n := archiveCalls(g.Fake, vID, "/"); n != 0 || v.count() != 0 {
		t.Fatalf("la recherche a lancé une construction : %d GET /archive de /, %d index", n, v.count())
	}
	v.For(tctx, "db") // the explorer starts the build, held by the gate
	none("construction en cours", info)
	close(g.open)
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		v.mu.Lock()
		b := v.builds[vID+"\x00"+info.FinishedAt]
		done := b != nil && finished(b)
		v.mu.Unlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("construction jamais finie")
		}
	}
	none("index fini, jamais servi", info)
	ready(t, v, "db")
	if hits, _, indexed := v.SearchCached(tctx, info, "passwd", 10); !indexed || !slices.Equal(hitPaths(hits), []string{"/etc/passwd"}) {
		t.Errorf("index servi : indexed=%v hits=%v", indexed, hits)
	}
	later := info
	later.FinishedAt = "2026-10-09T11:00:00Z"
	none("nouveau FinishedAt", later)
	running := info
	running.Running, running.Pid = true, 42
	none("conteneur actif", running)
	if n := archiveCalls(g.Fake, vID, "/"); n != 1 {
		t.Errorf("%d GET /archive de /, attendu le seul de l'explorateur", n)
	}
}

// Spec v0.2 §5.8: substring of the full path, case ignored; limit, then
// truncated.
func TestSearchCachedSubstring(t *testing.T) {
	v, _, info := servedViews(t,
		tdir("/"), tdir("/etc/"), tdir("/etc/nginx/"), tfile("/etc/nginx/nginx.conf", "x"),
		tdir("/usr/"), tdir("/usr/bin/"), tfile("/usr/bin/env", "x"), tlink("/usr/bin/Nginx-Link", "/etc/nginx"))
	hits, truncated, indexed := v.SearchCached(tctx, info, "NGINX.conf", 200)
	if !indexed || truncated || len(hits) != 1 || hits[0] != (SearchHit{Path: "/etc/nginx/nginx.conf", Type: TypeFile}) {
		t.Errorf("NGINX.conf : indexed=%v truncated=%v %v", indexed, truncated, hits)
	}
	hits, truncated, _ = v.SearchCached(tctx, info, "nginx", 200)
	want := []string{"/etc/nginx", "/etc/nginx/nginx.conf", "/usr/bin/Nginx-Link"}
	if truncated || !slices.Equal(hitPaths(hits), want) {
		t.Errorf("nginx : %v (truncated=%v), attendu %v", hitPaths(hits), truncated, want)
	}
	for _, h := range hits {
		if h.Path == "/usr/bin/Nginx-Link" && h.Type != TypeSymlink {
			t.Errorf("le lien est montré comme lien, sans être suivi : %+v", h)
		}
	}
	if hits, truncated, _ := v.SearchCached(tctx, info, "nginx", 1); len(hits) != 1 || !truncated {
		t.Errorf("limit=1 : %v truncated=%v", hits, truncated)
	}
	if hits, truncated, _ := v.SearchCached(tctx, info, "absent", 200); len(hits) != 0 || truncated {
		t.Errorf("absent : %v truncated=%v", hits, truncated)
	}
}

// A name whose own entry matches comes before a path that matches only by
// a parent directory: within the limit, the names searched for first.
func TestSearchCachedNamesFirst(t *testing.T) {
	hs := []hdr{tdir("/"), tdir("/srv/"), tdir("/srv/app/")}
	for i := range 5 {
		hs = append(hs, tfile(fmt.Sprintf("/srv/app/f%d", i), "x"))
	}
	hs = append(hs, tdir("/opt/"), tfile("/opt/app.yml", "x"))
	v, _, info := servedViews(t, hs...)
	hits, truncated, _ := v.SearchCached(tctx, info, "app", 2)
	if !truncated || !slices.Equal(hitPaths(hits), []string{"/opt/app.yml", "/srv/app"}) {
		t.Errorf("%v truncated=%v", hitPaths(hits), truncated)
	}
}

// Spec v0.2 §5.8: only the names the explorer shows. A directory replaced
// by a file later in the archive keeps its old children in the map of
// entries, unreachable by List; a directory lists MaxEntries names at most.
func TestSearchCachedOnlyVisible(t *testing.T) {
	hs := []hdr{
		tdir("/"), tdir("/a/"), tfile("/a/hidden-stripe_live_key", "x"), tfile("/a", "now a file"),
		tdir("/big/"),
	}
	for i := range MaxEntries {
		hs = append(hs, tfile(fmt.Sprintf("/big/f%05d", i), ""))
	}
	hs = append(hs, tfile("/big/zz-hidden-backup.sql", ""), tdir("/proc/"), tfile("/proc/hidden-1", ""))
	v, _, info := servedViews(t, hs...)
	if hits, _, indexed := v.SearchCached(tctx, info, "hidden", 200); !indexed || len(hits) != 0 {
		t.Errorf("noms que l'explorateur ne montre pas : %v", hitPaths(hits))
	}
	fsys, ok := v.Cached(info)
	if !ok {
		t.Fatal("Cached")
	}
	es, truncated, err := fsys.List(tctx, "/big")
	if err != nil || !truncated || len(es) != MaxEntries || slices.ContainsFunc(es, func(e Entry) bool { return e.Name == "zz-hidden-backup.sql" }) {
		t.Fatalf("témoin : List(/big) = %d entrées, truncated=%v, %v", len(es), truncated, err)
	}
	if hits, _, _ := v.SearchCached(tctx, info, "f09999", 200); !slices.Equal(hitPaths(hits), []string{"/big/f09999"}) {
		t.Errorf("dernier nom listé : %v", hitPaths(hits))
	}
}

// Spec v0.2 §5.8: bounded in time; a context that ends stops the walk.
func TestSearchCachedContext(t *testing.T) {
	hs := []hdr{tdir("/"), tdir("/d/")}
	for i := range 5000 {
		hs = append(hs, tfile(fmt.Sprintf("/d/f%d", i), ""))
	}
	v, _, info := servedViews(t, hs...)
	ctx, cancel := context.WithCancel(tctx)
	cancel()
	hits, truncated, indexed := v.SearchCached(ctx, info, "f", 200)
	if !indexed || !truncated || len(hits) == 200 {
		t.Errorf("contexte fini : indexed=%v truncated=%v, %d résultats", indexed, truncated, len(hits))
	}
}

// Nothing of a device, a FIFO or a hard link is opened: the hits carry the
// type the explorer lists.
func TestSearchCachedTypes(t *testing.T) {
	v, _, info := servedViews(t, tdir("/"), tdir("/x/"), tfile("/x/k-file", "x"), thard("/x/k-hard", "/x/k-file"),
		tnode("/x/k-fifo", tar.TypeFifo))
	hits, _, _ := v.SearchCached(tctx, info, "k-", 200)
	want := []SearchHit{{"/x/k-fifo", TypeOther}, {"/x/k-file", TypeFile}, {"/x/k-hard", TypeFile}}
	if !slices.Equal(hits, want) {
		t.Errorf("%v, attendu %v", hits, want)
	}
}

// Review of tranche 3: the search asks whether a build is in progress
// (Indexing) without making a visit of it: neither its last use (Grace,
// IdleTTL) nor its place in the LRU change. Same matching as Pending: full
// ID or name, never a prefix.
func TestViewsIndexingNotAVisit(t *testing.T) {
	g := &gated{Fake: stoppedDocker(t), open: make(chan struct{})}
	defer close(g.open)
	addStopped(t, g.Fake, "x2")
	v := NewViews(g, Options{})
	if v.Indexing("db") {
		t.Fatal("aucune construction")
	}
	v.For(tctx, "db")
	v.For(tctx, "x2")
	snapshot := func() ([]string, map[string]time.Time) {
		v.mu.Lock()
		defer v.mu.Unlock()
		touched := map[string]time.Time{}
		for k, b := range v.builds {
			touched[k] = b.touched
		}
		return slices.Clone(v.order), touched
	}
	order, touched := snapshot()
	time.Sleep(5 * time.Millisecond)
	for _, ref := range []string{"db", vID} {
		if !v.Indexing(ref) {
			t.Errorf("Indexing(%s) pendant la construction : false", ref)
		}
	}
	for _, ref := range []string{vID[:12], "d", "web"} {
		if v.Indexing(ref) {
			t.Errorf("Indexing(%s) d'un autre conteneur : true", ref)
		}
	}
	if o, tc := snapshot(); !slices.Equal(o, order) || !maps.Equal(tc, touched) {
		t.Errorf("Indexing a compté une visite : ordre %v → %v", order, o)
	}
}

// BenchmarkSearchWalk: the cost of a whole walk (nothing found) of an index
// of 37 224 entries (golang:1.27, spec v0.2 §5.5) and of 1 000 000
// (DefaultIndexLimits.MaxHeaders).
func BenchmarkSearchWalk(b *testing.B) {
	for _, n := range []int{37_224, 1_000_000} {
		x := &index{entries: map[string]Entry{"/": {Name: "/", Type: TypeDir}}, children: map[string][]string{}, hardlinks: map[string]string{}}
		for i := range n {
			x.add(&tar.Header{Name: fmt.Sprintf("/usr/lib/d%03d/s%02d/File-%d.so", i%500, i%40, i), Typeflag: tar.TypeReg})
		}
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			for b.Loop() {
				if hits, _ := x.search(tctx, "absent-name", 200); len(hits) != 0 {
					b.Fatal(hits)
				}
			}
		})
	}
}
