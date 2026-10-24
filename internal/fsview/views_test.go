package fsview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

var vID = strings.Repeat("ef", 32)

func stoppedInfo(id, name string, mounts ...dockerx.Mount) dockerx.ContainerInfo {
	return dockerx.ContainerInfo{ID: id, Name: name, FinishedAt: "2026-10-09T10:00:00Z", Mounts: mounts}
}

func stoppedDocker(t *testing.T, mounts ...dockerx.Mount) *dockerx.Fake {
	info := stoppedInfo(vID, "db", mounts...)
	return &dockerx.Fake{
		Infos: map[string]dockerx.ContainerInfo{"db": info, vID: info},
		Archives: map[string][]byte{
			vID + ":/":           mkTar(t, image()...),
			vID + ":/etc/passwd": fileTar(t, "/etc/passwd", "root:x:0:0"),
		},
		Sizes:   map[string]int64{vID: 1 << 20},
		Volumes: map[string]map[string]string{},
	}
}

func (v *Views) count() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.builds)
}

func waitCount(t *testing.T, v *Views, want int) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); v.count() != want; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("index en cache : %d, attendu %d", v.count(), want)
		}
	}
}

// ready polls For until the index is built.
func ready(t *testing.T, v *Views, id string) FS {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		fsys, err := v.For(tctx, id)
		var ie *IndexingError
		if !errors.As(err, &ie) {
			if err != nil {
				t.Fatalf("For(%s) : %v", id, err)
			}
			return fsys
		}
		if time.Now().After(deadline) {
			t.Fatalf("index jamais prêt : %v", err)
		}
	}
}

func archiveCalls(f *dockerx.Fake, id, p string) int {
	n := 0
	for _, c := range f.Calls() {
		if c == id+":"+p {
			n++
		}
	}
	return n
}

// gated holds GET /archive of "/" after free bytes until open is closed,
// and honours ctx like the real client.
type gated struct {
	*dockerx.Fake
	open chan struct{}
	free int
}

func (g *gated) GetArchive(ctx context.Context, id, p string) (io.ReadCloser, error) {
	rc, err := g.Fake.GetArchive(ctx, id, p)
	if err != nil || p != "/" {
		return rc, err
	}
	return &gateReader{ctx: ctx, open: g.open, rc: rc, free: g.free}, nil
}

type gateReader struct {
	ctx  context.Context
	open chan struct{}
	rc   io.ReadCloser
	free int
}

func (r *gateReader) Read(b []byte) (int, error) {
	if r.free > 0 {
		b = b[:min(len(b), r.free)]
		n, err := r.rc.Read(b)
		r.free -= n
		return n, err
	}
	select {
	case <-r.open:
		return r.rc.Read(b)
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	}
}

func (r *gateReader) Close() error { return r.rc.Close() }

// Spec §5.2 step 7 and Q9 (confirmed): no index, no tar of "/", for a
// container whose mounts may expose the host.
func TestViewsHostBindBuildsNothing(t *testing.T) {
	vol := func(driver string) dockerx.Mount {
		return dockerx.Mount{Type: "volume", Name: "v", Driver: driver, Destination: "/v"}
	}
	refused := []struct {
		m    dockerx.Mount
		opts map[string]string
		name string
	}{
		{dockerx.Mount{Type: "bind", Source: "/", Destination: "/host"}, nil, "bind / -> /host"},
		{dockerx.Mount{Type: "bind", Source: "/proc", Destination: "/host/proc"}, nil, "bind /proc -> /host/proc"},
		{dockerx.Mount{Type: "bind", Source: "/sys", Destination: "/host/sys"}, nil, "bind /sys -> /host/sys"},
		{dockerx.Mount{Type: "bind", Source: "/sys/fs/cgroup", Destination: "/cg"}, nil, "bind /sys/fs/cgroup -> /cg"},
		{dockerx.Mount{Type: "bind", Source: "/dev", Destination: "/dev"}, nil, "bind /dev -> /dev"},
		{vol("local"), map[string]string{"o": "bind", "device": "/proc"}, "bind /proc -> /v (volume v)"},
		{vol("local"), map[string]string{"type": "fuse.sshfs"}, "volume v (type=fuse.sshfs) -> /v"},
		{vol("rexray"), map[string]string{}, `unknown driver "rexray"`},
		{vol("local"), nil, "unreadable options"}, // no options for v: unreadable
	}
	for _, c := range refused {
		f := stoppedDocker(t, c.m)
		if c.opts != nil {
			f.Volumes["v"] = c.opts
		}
		v := NewViews(f, Options{FirstWait: time.Second})
		_, err := v.For(tctx, "db")
		if !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), c.name) {
			t.Errorf("%s : %v", c.name, err)
		}
		if n := v.count(); n != 0 || len(f.Calls()) != 0 {
			t.Errorf("ÉVASION : %s : %d index lancé(s), GetArchive %v", c.name, n, f.Calls())
		}
	}
	for _, m := range []dockerx.Mount{ // controls: indexed
		{Type: "bind", Source: "/srv/data", Destination: "/data"},
		{Type: "bind", Source: "/dev/log", Destination: "/dev/log"},
	} {
		f := stoppedDocker(t, m)
		ready(t, NewViews(f, Options{}), "db")
		if n := archiveCalls(f, vID, "/"); n != 1 {
			t.Errorf("%+v : %d GET /archive de /", m, n)
		}
	}
}

func TestViewsIndexingThenReady(t *testing.T) {
	g := &gated{Fake: stoppedDocker(t), open: make(chan struct{})}
	v := NewViews(g, Options{})
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := v.For(tctx, "db")
			var ie *IndexingError
			if !errors.As(err, &ie) || !errors.Is(err, ErrIndexing) || ie.Progress < 0 || ie.Progress > 0.99 {
				t.Errorf("pendant l'indexation : %v", err)
			}
		}()
	}
	wg.Wait()
	close(g.open)
	fsys := ready(t, v, "db")
	if _, _, err := fsys.List(tctx, "/etc"); err != nil {
		t.Fatal(err)
	}
	if n := archiveCalls(g.Fake, vID, "/"); n != 1 {
		t.Errorf("une seule construction par clé : %d GET /archive de /", n)
	}
}

func TestViewsProgressCapped(t *testing.T) {
	g := &gated{Fake: stoppedDocker(t), open: make(chan struct{}), free: 4096}
	g.Sizes[vID] = 1024 // volumes are in the archive, not in SizeRootFs
	v := NewViews(g, Options{})
	defer close(g.open)
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		_, err := v.For(tctx, "db")
		var ie *IndexingError
		if !errors.As(err, &ie) {
			t.Fatalf("attendu indexing : %v", err)
		}
		if ie.Progress > 0.99 {
			t.Fatalf("progression %v au-delà de 0,99", ie.Progress)
		}
		if ie.Progress == 0.99 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("progression bloquée à %v", ie.Progress)
		}
	}
}

func TestViewsCacheKey(t *testing.T) {
	f := stoppedDocker(t)
	v := NewViews(f, Options{})
	ready(t, v, "db")
	ready(t, v, "db")
	if n := archiveCalls(f, vID, "/"); n != 1 {
		t.Fatalf("en cache : %d constructions", n)
	}
	info := f.Infos["db"] // restarted then stopped: new FinishedAt
	info.FinishedAt = "2026-10-09T11:00:00Z"
	f.Infos["db"], f.Infos[vID] = info, info
	ready(t, v, "db")
	if n := archiveCalls(f, vID, "/"); n != 2 || v.count() != 1 {
		t.Errorf("nouvelle clé : %d constructions, %d index en cache (attendu 2 et 1)", n, v.count())
	}
}

// addStopped adds stopped containers named n, of ID strings.Repeat(n[1:], 64).
func addStopped(t *testing.T, f *dockerx.Fake, names ...string) {
	for _, n := range names {
		id := strings.Repeat(n[1:], 64)
		info := stoppedInfo(id, n)
		f.Infos[n], f.Infos[id] = info, info
		f.Archives[id+":/"] = mkTar(t, image()...)
	}
}

func TestViewsLRU(t *testing.T) {
	f := stoppedDocker(t)
	names := []string{"db", "c2", "c3", "c4"}
	addStopped(t, f, names[1:]...)
	v := NewViews(f, Options{MaxCached: 3})
	for _, n := range names {
		ready(t, v, n)
	}
	if v.count() != 3 {
		t.Fatalf("%d index en cache, attendu 3", v.count())
	}
	ready(t, v, "db") // the least recently used was evicted: built again
	if n := archiveCalls(f, vID, "/"); n != 2 {
		t.Errorf("db : %d constructions, attendu 2", n)
	}
	ready(t, v, "c4")
	if n := archiveCalls(f, strings.Repeat("4", 64), "/"); n != 1 {
		t.Errorf("c4 : %d constructions, attendu 1", n)
	}
}

// Review I1: a build in progress is never evicted; four containers polled
// at once all end up ready.
func TestViewsFourPolledAtOnce(t *testing.T) {
	f := stoppedDocker(t)
	addStopped(t, f, "c2", "c3", "c4")
	g := &gated{Fake: f, open: make(chan struct{})}
	v := NewViews(g, Options{MaxCached: 3})
	names := []string{"db", "c2", "c3", "c4"}
	for _, n := range names {
		if _, err := v.For(tctx, n); !errors.Is(err, ErrIndexing) {
			t.Fatalf("%s : %v", n, err)
		}
	}
	if n := archiveCalls(f, strings.Repeat("4", 64), "/"); n != 0 || v.count() != 3 {
		t.Fatalf("c4 : %d construction(s), %d index ; attendu 0 et 3 (aucune construction évincée)", n, v.count())
	}
	close(g.open)
	seen := map[string]bool{}
	for deadline := time.Now().Add(5 * time.Second); len(seen) < len(names); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("jamais prêts : %v", seen)
		}
		for _, n := range names {
			_, err := v.For(tctx, n)
			if err == nil {
				seen[n] = true
			} else if !errors.Is(err, ErrIndexing) {
				t.Fatalf("%s : %v", n, err)
			}
		}
	}
}

// R20: three builds finish but their requests went away. The fourth
// container gets a slot once Grace has passed, not after IdleTTL.
func TestViewsUnseenIndexesExpire(t *testing.T) {
	f := stoppedDocker(t)
	addStopped(t, f, "c2", "c3", "c4")
	// The gate keeps the three builds running while their first request
	// returns: no request ever sees them (without it, a build finished
	// before For returned was served, hence evictable at once). Grace is
	// long: the test ages the indexes itself instead of waiting.
	g := &gated{Fake: f, open: make(chan struct{})}
	v := NewViews(g, Options{MaxCached: 3, Grace: time.Hour})
	for _, n := range []string{"db", "c2", "c3"} {
		if _, err := v.For(tctx, n); !errors.Is(err, ErrIndexing) { // 202, then never asked again
			t.Fatalf("%s : %v, attendu 202", n, err)
		}
	}
	close(g.open)
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		v.mu.Lock()
		done := 0
		for _, b := range v.builds {
			if finished(b) && !b.served {
				done++
			}
		}
		v.mu.Unlock()
		if done == 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("constructions jamais finies")
		}
	}
	c4 := strings.Repeat("4", 64)
	_, err := v.For(tctx, "c4")
	v.mu.Lock() // an eviction happens under the lock, before For returns
	var ids []string
	for _, b := range v.builds {
		ids = append(ids, b.id)
	}
	v.mu.Unlock()
	slices.Sort(ids)
	if want := []string{strings.Repeat("2", 64), strings.Repeat("3", 64), vID}; !errors.Is(err, ErrIndexing) ||
		!slices.Equal(ids, want) || archiveCalls(f, c4, "/") != 0 {
		t.Fatalf("pendant Grace : %v, index %v, %d construction(s) de c4 (attendu 202, aucun évincé, 0)",
			err, ids, archiveCalls(f, c4, "/"))
	}
	v.mu.Lock() // Grace has passed for the three unseen indexes
	for _, b := range v.builds {
		b.touched = b.touched.Add(-2 * time.Hour)
	}
	v.mu.Unlock()
	ready(t, v, "c4")
}

// Review I5: an index unused for IdleTTL is dropped.
func TestViewsIdleTTL(t *testing.T) {
	f := stoppedDocker(t)
	v := NewViews(f, Options{IdleTTL: 50 * time.Millisecond})
	ready(t, v, "db")
	waitCount(t, v, 0)
	ready(t, v, "db")
	if n := archiveCalls(f, vID, "/"); n != 2 {
		t.Errorf("%d constructions, attendu 2", n)
	}
}

func TestViewsTooLargeSticky(t *testing.T) {
	f := stoppedDocker(t)
	v := NewViews(f, Options{Limits: IndexLimits{MaxBytes: 1024}, FirstWait: 5 * time.Second})
	for range 2 {
		if _, err := v.For(tctx, "db"); !errors.Is(err, ErrTooLarge) {
			t.Fatalf("attendu too_large : %v", err)
		}
	}
	if n := archiveCalls(f, vID, "/"); n != 1 {
		t.Errorf("too_large reste en cache : %d constructions", n)
	}
}

func TestViewsDurationCap(t *testing.T) {
	g := &gated{Fake: stoppedDocker(t), open: make(chan struct{})} // never opened
	v := NewViews(g, Options{Limits: IndexLimits{MaxDuration: 50 * time.Millisecond}, FirstWait: 5 * time.Second,
		SlowRetry: 200 * time.Millisecond})
	for range 2 {
		if _, err := v.For(tctx, "db"); !errors.Is(err, ErrTooLarge) {
			t.Fatalf("attendu too_large : %v", err)
		}
	}
	if n := archiveCalls(g.Fake, vID, "/"); n != 1 {
		t.Fatalf("gardé pendant SlowRetry : %d constructions", n)
	}
	time.Sleep(250 * time.Millisecond)
	v.For(tctx, "db") // Q11: the time cap expires, the bytes cap does not (TestViewsTooLargeSticky)
	if n := archiveCalls(g.Fake, vID, "/"); n != 2 {
		t.Errorf("après SlowRetry : %d constructions, attendu 2", n)
	}
}

func TestViewsErrorsAreRetried(t *testing.T) {
	f := stoppedDocker(t)
	tarOfRoot := f.Archives[vID+":/"]
	delete(f.Archives, vID+":/")
	v := NewViews(f, Options{FirstWait: 5 * time.Second})
	if _, err := v.For(tctx, "db"); !errors.Is(err, dockerx.ErrNotFound) {
		t.Fatalf("attendu not_found : %v", err)
	}
	waitCount(t, v, 0) // not kept
	f.Archives[vID+":/"] = tarOfRoot
	ready(t, v, "db")
	if n := archiveCalls(f, vID, "/"); n != 2 {
		t.Errorf("%d constructions, attendu 2", n)
	}
}

// docker start while dockerd builds the tar: the index would mix two states.
type startsDuringBuild struct {
	*dockerx.Fake
	started atomic.Bool
}

func (s *startsDuringBuild) GetArchive(ctx context.Context, id, p string) (io.ReadCloser, error) {
	s.started.Store(true)
	return s.Fake.GetArchive(ctx, id, p)
}

func (s *startsDuringBuild) Inspect(ctx context.Context, id string) (dockerx.ContainerInfo, error) {
	info, err := s.Fake.Inspect(ctx, id)
	if s.started.Load() {
		info.Running, info.Pid = true, 4242
	}
	return info, err
}

func TestViewsStartedDuringBuild(t *testing.T) {
	d := &startsDuringBuild{Fake: stoppedDocker(t)}
	v := NewViews(d, Options{FirstWait: 5 * time.Second})
	if _, err := v.For(tctx, "db"); !errors.Is(err, ErrContainerChanged) {
		t.Fatalf("attendu container_changed : %v", err)
	}
	waitCount(t, v, 0)
}

func TestViewsForget(t *testing.T) {
	f := stoppedDocker(t)
	v := NewViews(f, Options{})
	ready(t, v, "db")
	v.Forget(vID)
	if v.count() != 0 {
		t.Fatal("Forget n'a rien retiré")
	}
	ready(t, v, "db")
	if n := archiveCalls(f, vID, "/"); n != 2 {
		t.Errorf("%d constructions, attendu 2", n)
	}
}

func TestViewsInspectErrors(t *testing.T) {
	v := NewViews(&dockerx.Fake{}, Options{})
	if _, err := v.For(tctx, "nope"); !errors.Is(err, dockerx.ErrNotFound) {
		t.Errorf("inconnu : %v", err)
	}
	down := NewViews(&dockerx.Fake{Err: fmt.Errorf("%w : dial", dockerx.ErrUnavailable)}, Options{})
	if _, err := down.For(tctx, "web"); !errors.Is(err, dockerx.ErrUnavailable) {
		t.Errorf("Docker arrêté : %v", err)
	}
	// Running without a pid: the stopped path, never procfs.
	f := stoppedDocker(t)
	info := f.Infos["db"]
	info.Running = true
	f.Infos["db"], f.Infos[vID] = info, info
	ready(t, NewViews(f, Options{}), "db")
}

// failsLate holds GET /archive of "/" until open is closed, then fails.
type failsLate struct {
	*dockerx.Fake
	open chan struct{}
}

func (f *failsLate) GetArchive(ctx context.Context, id, p string) (io.ReadCloser, error) {
	if p == "/" {
		f.Fake.GetArchive(ctx, id, p) // recorded
		<-f.open
		return nil, errors.New("docker : boom")
	}
	return f.Fake.GetArchive(ctx, id, p)
}

// Review: an error after FirstWait is delivered to the next request, once;
// polls never rebuild before seeing it.
func TestViewsLateErrorDeliveredOnce(t *testing.T) {
	d := &failsLate{Fake: stoppedDocker(t), open: make(chan struct{})}
	v := NewViews(d, Options{FirstWait: 20 * time.Millisecond})
	if _, err := v.For(tctx, "db"); !errors.Is(err, ErrIndexing) {
		t.Fatalf("attendu indexing : %v", err)
	}
	close(d.open)
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		v.mu.Lock()
		done := false
		for _, b := range v.builds {
			done = finished(b)
		}
		v.mu.Unlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("construction jamais finie")
		}
	}
	if _, err := v.For(tctx, "db"); err == nil || errors.Is(err, ErrIndexing) {
		t.Fatalf("l'erreur doit être livrée : %v", err)
	}
	if n := archiveCalls(d.Fake, vID, "/"); n != 1 {
		t.Fatalf("%d GET /archive avant la livraison, attendu 1", n)
	}
	v.For(tctx, "db") // delivered: builds again
	if n := archiveCalls(d.Fake, vID, "/"); n != 2 {
		t.Errorf("%d GET /archive après la livraison, attendu 2", n)
	}
}

// lockedExport models dockerd (moby daemon/archive.go): GET /archive holds
// the container lock until the whole tar is sent, and an Inspect of that
// container, with or without size, waits for it. Measured on Docker Desktop
// 29.7.2 (v0.1.1): docker inspect of a stopped golang:1 took 10.6 s during
// its 11.6 s export, 0.02 s at rest. waitLock gives up after 2 s, like a
// client deadline, so that a red run stays short.
type lockedExport struct {
	*gated
	exporting atomic.Bool
}

func (l *lockedExport) GetArchive(ctx context.Context, id, p string) (io.ReadCloser, error) {
	if p == "/" {
		l.exporting.Store(true)
	}
	return l.gated.GetArchive(ctx, id, p)
}

func (l *lockedExport) waitLock(ctx context.Context) error {
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

func (l *lockedExport) Inspect(ctx context.Context, id string) (dockerx.ContainerInfo, error) {
	if err := l.waitLock(ctx); err != nil {
		return dockerx.ContainerInfo{}, err
	}
	return l.gated.Inspect(ctx, id)
}

func (l *lockedExport) SizeRootFs(ctx context.Context, id string) (int64, error) {
	if err := l.waitLock(ctx); err != nil {
		return 0, err
	}
	return l.gated.SizeRootFs(ctx, id)
}

func newLockedExport(t *testing.T) *lockedExport {
	return &lockedExport{gated: &gated{Fake: stoppedDocker(t), open: make(chan struct{}), free: 4096}}
}

// v0.1.1, spec §5.2 step 2: while the export holds the container, polls
// answer 202 at once, by full ID or by name, with a progress that moves
// (SizeRootFs is read before the export takes the lock).
func TestViewsPollsDuringLockedExport(t *testing.T) {
	d := newLockedExport(t)
	v := NewViews(d, Options{FirstWait: 50 * time.Millisecond})
	if _, err := v.For(tctx, "db"); !errors.Is(err, ErrIndexing) {
		t.Fatalf("première requête : %v", err)
	}
	for _, ref := range []string{"db", vID, "db", vID} {
		start := time.Now()
		_, err := v.For(tctx, ref)
		var ie *IndexingError
		if dt := time.Since(start); !errors.As(err, &ie) || dt > 200*time.Millisecond {
			t.Fatalf("relance %s pendant l'export : %v en %s, attendu 202 immédiat", ref, err, dt)
		}
	}
	for deadline := time.Now().Add(3 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		_, err := v.For(tctx, "db")
		var ie *IndexingError
		if !errors.As(err, &ie) {
			t.Fatalf("attendu indexing : %v", err)
		}
		if ie.Progress > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("progression bloquée à 0 : SizeRootFs attend la fin de l'export")
		}
	}
	close(d.open)
	fsys := ready(t, v, "db")
	if _, _, err := fsys.List(tctx, "/etc"); err != nil {
		t.Fatal(err)
	}
	if n := archiveCalls(d.Fake, vID, "/"); n != 1 {
		t.Errorf("%d GET /archive de /, attendu 1", n)
	}
}

// Pending answers for a build in progress only: never for another
// container (no prefix match), never once the build is over (serving goes
// through Inspect).
func TestViewsPendingOnlyInProgress(t *testing.T) {
	d := newLockedExport(t)
	v := NewViews(d, Options{})
	if err := v.Pending("db"); err != nil {
		t.Fatalf("aucune construction : %v", err)
	}
	v.For(tctx, "db")
	for _, ref := range []string{"db", vID} {
		if err := v.Pending(ref); !errors.Is(err, ErrIndexing) {
			t.Errorf("Pending(%s) pendant la construction : %v", ref, err)
		}
	}
	for _, ref := range []string{"web", vID[:12], "d", "e", strings.Repeat("ab", 32)} {
		if err := v.Pending(ref); err != nil {
			t.Errorf("Pending(%s) d'un autre conteneur : %v", ref, err)
		}
	}
	close(d.open)
	ready(t, v, "db")
	if err := v.Pending("db"); err != nil {
		t.Errorf("Pending après la construction : %v", err)
	}
}

// The identity is checked again when an index is served: restarted and
// stopped once the build is over, the container gets a new index, never the
// old one.
func TestViewsServeReinspects(t *testing.T) {
	d := newLockedExport(t)
	v := NewViews(d, Options{})
	v.For(tctx, "db")
	close(d.open)
	ready(t, v, "db")
	info := d.Infos["db"]
	info.FinishedAt = "2026-10-09T11:00:00Z"
	d.Infos["db"], d.Infos[vID] = info, info
	ready(t, v, "db")
	if n := archiveCalls(d.Fake, vID, "/"); n != 2 {
		t.Errorf("ÉVASION : %d GET /archive de /, attendu 2 (ancien index servi)", n)
	}
}

// Spec v0.2 §5.3 (figures) and §5.7 (photo of the metadata): Cached and
// CachedCount give the index already served to a request, under the key For
// would use (full ID and FinishedAt), and never start a build.
func TestViewsCachedNeverBuilds(t *testing.T) {
	g := &gated{Fake: stoppedDocker(t), open: make(chan struct{})}
	v := NewViews(g, Options{})
	info := g.Infos["db"]
	none := func(what string, info dockerx.ContainerInfo) {
		t.Helper()
		if fsys, ok := v.Cached(info); ok || fsys != nil {
			t.Errorf("%s : Cached a servi un index", what)
		}
		if n, at, ok := v.CachedCount(info); ok || n != 0 || !at.IsZero() {
			t.Errorf("%s : CachedCount = %d, %v, %v", what, n, at, ok)
		}
	}
	none("aucun index", info)
	if n := archiveCalls(g.Fake, vID, "/"); n != 0 || v.count() != 0 {
		t.Fatalf("une construction lancée : %d GET /archive de /, %d index", n, v.count())
	}
	v.For(tctx, "db") // starts the build, held by the gate
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
	start := time.Now()
	fsys, ok := v.Cached(info)
	if !ok {
		t.Fatal("index servi, Cached refuse")
	}
	if _, _, err := fsys.List(tctx, "/etc"); err != nil {
		t.Errorf("List : %v", err)
	}
	// image() of tarindex_test.go: 29 entries besides "/", /proc and /sys
	// themselves kept, nothing under them.
	n, at, ok := v.CachedCount(info)
	if !ok || n != 29 || at.IsZero() || at.After(start) {
		t.Errorf("CachedCount = %d, %v, %v ; attendu 29, l'heure de la construction", n, at, ok)
	}
	// Restarted and stopped since (new FinishedAt), or running now: the old
	// index is not this container's state.
	later := info
	later.FinishedAt = "2026-10-09T11:00:00Z"
	none("nouveau FinishedAt", later)
	running := info
	running.Running, running.Pid = true, 42
	none("conteneur actif", running)
	if n := archiveCalls(g.Fake, vID, "/"); n != 1 {
		t.Errorf("%d GET /archive de /, attendu 1", n)
	}
}

// Spec v0.2 §5.5 step 6: the indexes of images share the 3 slots of the
// containers', under the same rules (spec §5.2 step 4): the least recently
// used is evicted, a build in progress never is.
func TestViewsImageAndContainerShareThreeSlots(t *testing.T) {
	f := stoppedDocker(t)
	addStopped(t, f, "c2")
	imgs, ids := imagesDocker(t, 2)
	f.Images, f.ImageSaves = imgs.Images, imgs.ImageSaves
	v := NewViews(f, Options{MaxCached: 3})
	ready(t, v, "db")
	readyImage(t, v, ids[0])
	ready(t, v, "c2")
	readyImage(t, v, ids[1]) // evicts db, the least recently used
	if v.count() != 3 {
		t.Fatalf("%d index en cache, attendu 3", v.count())
	}
	readyImage(t, v, ids[0]) // still there: no export
	if n := f.ImageSaveCalls.Load(); n != 2 {
		t.Errorf("%d exports, attendu 2", n)
	}
	ready(t, v, "db") // evicted: built again, evicting c2
	if n := archiveCalls(f, vID, "/"); n != 2 {
		t.Errorf("db : %d constructions, attendu 2", n)
	}
	if _, _, indexed := v.SearchImage(tctx, ids[1], "f", 10); !indexed {
		t.Error("image 2 évincée à la place de c2")
	}

	// Three builds in progress, two containers and an image: a fourth
	// (an image) waits, nothing is evicted, nothing is exported.
	f2 := stoppedDocker(t)
	addStopped(t, f2, "c2")
	imgs2, ids2 := imagesDocker(t, 2)
	f2.Images, f2.ImageSaves = imgs2.Images, imgs2.ImageSaves
	open := make(chan struct{})
	var closes atomic.Int32
	g := &gated{Fake: f2, open: open}
	h := &heldIndex{gated: g, closes: &closes}
	v2 := NewViews(h, Options{MaxCached: 3})
	for _, n := range []string{"db", "c2"} {
		if _, err := v2.For(tctx, n); !errors.Is(err, ErrIndexing) {
			t.Fatalf("%s : %v", n, err)
		}
	}
	if _, err := v2.ForImage(tctx, ids2[0]); !errors.Is(err, ErrIndexing) {
		t.Fatalf("image 1 : %v", err)
	}
	for deadline := time.Now().Add(2 * time.Second); f2.ImageSaveCalls.Load() < 1; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("export de l'image 1 jamais lancé")
		}
	}
	if _, err := v2.ForImage(tctx, ids2[1]); !errors.Is(err, ErrIndexing) {
		t.Fatalf("image 2 : %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if n := f2.ImageSaveCalls.Load(); n != 1 || v2.count() != 3 {
		t.Errorf("%d exports, %d index ; attendu 1 et 3 (aucune construction évincée)", n, v2.count())
	}
	close(open)
	ready(t, v2, "db")
	ready(t, v2, "c2")
	readyImage(t, v2, ids2[0])
	readyImage(t, v2, ids2[1]) // the others served: evictable now
}

// heldIndex holds the exports of images like gated holds GET /archive.
type heldIndex struct {
	*gated
	closes *atomic.Int32
}

func (h *heldIndex) ImageSave(ctx context.Context, id string) (io.ReadCloser, error) {
	rc, err := h.Fake.ImageSave(ctx, id)
	if err != nil {
		return nil, err
	}
	return &held{ctx: ctx, open: h.open, rc: rc, closes: h.closes}, nil
}
