//go:build linux

package fsview

// Evasion tests of the snapshot walk (spec v0.2 §5.7, §8), written before the
// code. A temporary directory plays the container root, opened like
// /proc/<pid>/root; a host file outside it holds HOST-SECRET. The traps are
// built with host paths (the fixture), the code under test only sees the
// root descriptor.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

type visit struct {
	p    string
	e    Entry
	open func() (io.ReadCloser, error)
}

// walkAll walks fsys and records every visit; fn, when set, decides what
// the walk does next.
func walkAll(t *testing.T, fsys FS, fn func(v visit) error) ([]visit, error) {
	t.Helper()
	w, ok := fsys.(Walker)
	if !ok {
		t.Fatal("procFS n'implémente pas Walker")
	}
	var out []visit
	err := w.Walk(ctx, func(p string, e Entry, open func() (io.ReadCloser, error)) error {
		v := visit{p, e, open}
		out = append(out, v)
		if fn != nil {
			return fn(v)
		}
		return nil
	})
	return out, err
}

func find(vs []visit, p string) (visit, bool) {
	i := slices.IndexFunc(vs, func(v visit) bool { return v.p == p })
	if i < 0 {
		return visit{}, false
	}
	return vs[i], true
}

func paths(vs []visit) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = v.p
	}
	return out
}

func TestWalkSymlinksNeverFollowed(t *testing.T) {
	fsys, rp, marker := newRoot(t)
	must(t, os.Symlink(marker, filepath.Join(rp, "a")))
	must(t, os.Symlink("../../..", filepath.Join(rp, "b")))
	must(t, os.Symlink("/c", filepath.Join(rp, "c")))
	must(t, os.Symlink("/a", filepath.Join(rp, "e")))
	vs, err := walkAll(t, fsys, nil)
	if err != nil {
		t.Fatalf("Walk : %v", err)
	}
	want := map[string]string{"/a": marker, "/b": "../../..", "/c": "/c", "/e": "/a"}
	for p, target := range want {
		v, ok := find(vs, p)
		if !ok {
			t.Errorf("%s absent du parcours", p)
			continue
		}
		if v.e.Type != TypeSymlink || v.e.LinkTarget != target {
			t.Errorf("%s : %+v, attendu un lien vers %q", p, v.e, target)
		}
		if v.open != nil {
			t.Errorf("%s : open fourni pour un lien", p)
		}
	}
	if len(vs) != len(want) {
		t.Errorf("parcours : %v, attendu les 4 liens seuls (rien de suivi)", paths(vs))
	}
}

func TestWalkFifoNotOpened(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	must(t, unix.Mkfifo(filepath.Join(rp, "f"), 0o644))
	done := make(chan []visit, 1)
	go func() {
		vs, _ := walkAll(t, fsys, nil)
		done <- vs
	}()
	select {
	case vs := <-done:
		v, ok := find(vs, "/f")
		if !ok || v.e.Type != TypeOther || v.open != nil {
			t.Fatalf("/f : %+v (open fourni : %v), attendu other sans open", v.e, v.open != nil)
		}
	case <-time.After(time.Second):
		t.Fatal("parcours bloqué : la FIFO a été ouverte")
	}
}

func TestWalkDeviceNotOpened(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	if err := unix.Mknod(filepath.Join(rp, "zero"), unix.S_IFCHR|0o666, int(unix.Mkdev(1, 5))); errors.Is(err, unix.EPERM) {
		t.Skipf("mknod refusé (%v) : ce test demande CAP_MKNOD (scripts/test-linux.sh)", err)
	} else if err != nil {
		t.Fatalf("mknod : %v", err)
	}
	vs, err := walkAll(t, fsys, nil)
	if err != nil {
		t.Fatalf("Walk : %v", err)
	}
	if v, ok := find(vs, "/zero"); !ok || v.e.Type != TypeOther || v.open != nil {
		t.Fatalf("/zero : %+v (open fourni : %v), attendu other sans open", v.e, v.open != nil)
	}
}

// The machine's own "/" as the root: its /proc is a real procfs. Every other
// directory is skipped (fs.SkipDir), so the walk stays short.
func TestWalkPseudoFsInaccessible(t *testing.T) {
	host := openTestRoot(t, "/")
	vs, err := walkAll(t, host, func(v visit) error {
		if v.e.Type == TypeDir {
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Walk : %v", err)
	}
	v, ok := find(vs, "/proc")
	if !ok || v.e.Type != TypeInaccessible || v.open != nil {
		t.Fatalf("/proc : %+v (présent : %v), attendu inaccessible", v.e, ok)
	}
	for _, v := range vs {
		if strings.HasPrefix(v.p, "/proc/") || strings.HasPrefix(v.p, "/sys/") {
			t.Errorf("%s visité sous un pseudo-système", v.p)
		}
	}
}

func requireRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("ce test demande root (scripts/test-linux.sh)")
	}
}

func mountTmpfs(t *testing.T, dir string) {
	t.Helper()
	if err := unix.Mount("tmpfs", dir, "tmpfs", 0, "size=4m"); err != nil {
		t.Skipf("mount tmpfs refusé (%v) : ce test demande CAP_SYS_ADMIN", err)
	}
	t.Cleanup(func() { unix.Unmount(dir, unix.MNT_DETACH) })
}

// Without any privilege (CI job go, scripts/test-linux.sh), on a mount that
// is already there: /dev of the machine's "/" is a tmpfs (container) or a
// devtmpfs (host), never the root's mount, and no pseudo filesystem.
func TestWalkExistingMountNotTraversed(t *testing.T) {
	host := openTestRoot(t, "/")
	vs, err := walkAll(t, host, func(v visit) error {
		if v.e.Type == TypeDir {
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Walk : %v", err)
	}
	if v, ok := find(vs, "/dev"); !ok || v.e.Type != TypeMount || v.open != nil {
		t.Fatalf("/dev : %+v (présent : %v), attendu mount", v.e, ok)
	}
	for _, v := range vs {
		if strings.HasPrefix(v.p, "/dev/") {
			t.Errorf("%s visité sous un montage", v.p)
		}
	}
}

// Review of tranche 8: the listings of the directories on the way down are
// all held at once (a deep tree whose every level has thousands of long
// links); their names and targets are capped in all, like the tar index.
func TestWalkHeldNamesCap(t *testing.T) {
	old := walkMaxHeld
	t.Cleanup(func() { walkMaxHeld = old })
	if walkMaxHeld != DefaultIndexLimits.MaxNameBytes {
		t.Fatalf("plafond par défaut : %d, attendu celui de l'index tar (%d)", walkMaxHeld, DefaultIndexLimits.MaxNameBytes)
	}
	fsys, rp, _ := newRoot(t)
	target := strings.Repeat("t", 1000)
	for i := range 20 {
		must(t, os.Symlink(target, filepath.Join(rp, fmt.Sprintf("l%02d", i))))
	}
	walkMaxHeld = 64 << 10
	if _, err := walkAll(t, fsys, nil); err != nil {
		t.Fatalf("sous le plafond : %v", err)
	}
	walkMaxHeld = 8 << 10 // 20 × ~1 Kio
	if _, err := walkAll(t, fsys, nil); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("au-delà du plafond : %v, attendu ErrTooLarge", err)
	}
}

func TestWalkMountNotTraversed(t *testing.T) {
	requireRoot(t)
	fsys, rp, _ := newRoot(t)
	m := filepath.Join(rp, "m")
	must(t, os.Mkdir(m, 0o755))
	mountTmpfs(t, m)
	must(t, os.WriteFile(filepath.Join(m, "x"), []byte("dans le montage"), 0o644))
	must(t, os.WriteFile(filepath.Join(rp, "y"), []byte("hors montage"), 0o644))
	vs, err := walkAll(t, fsys, nil)
	if err != nil {
		t.Fatalf("Walk : %v", err)
	}
	if v, ok := find(vs, "/m"); !ok || v.e.Type != TypeMount || v.open != nil {
		t.Fatalf("/m : %+v (présent : %v), attendu mount", v.e, ok)
	}
	if _, ok := find(vs, "/m/x"); ok {
		t.Error("/m/x visité : le montage a été parcouru")
	}
	if _, ok := find(vs, "/y"); !ok {
		t.Error("/y absent")
	}
}

// A bind-mounted file (Docker's /etc/hosts) is a mount too, never read.
func TestWalkFileMountNotRead(t *testing.T) {
	requireRoot(t)
	fsys, rp, marker := newRoot(t)
	target := filepath.Join(rp, "hosts")
	must(t, os.WriteFile(target, nil, 0o644))
	if err := unix.Mount(marker, target, "", unix.MS_BIND, ""); err != nil {
		t.Skipf("bind refusé : %v", err)
	}
	t.Cleanup(func() { unix.Unmount(target, unix.MNT_DETACH) })
	vs, err := walkAll(t, fsys, nil)
	if err != nil {
		t.Fatalf("Walk : %v", err)
	}
	if v, ok := find(vs, "/hosts"); !ok || v.e.Type != TypeMount || v.open != nil {
		t.Fatalf("/hosts : %+v (présent : %v), attendu mount", v.e, ok)
	}
}

func TestWalkStaysInRoot(t *testing.T) {
	fsys, rp, marker := newRoot(t)
	created := map[string]string{"/f": "un", "/d/g": "deux", "/d/e/h": "trois"}
	must(t, os.MkdirAll(filepath.Join(rp, "d", "e"), 0o755))
	for p, c := range created {
		must(t, os.WriteFile(filepath.Join(rp, p), []byte(c), 0o644))
	}
	must(t, os.Symlink(marker, filepath.Join(rp, "d", "secret")))
	must(t, os.Symlink(filepath.Dir(marker), filepath.Join(rp, "d", "e", "hostdir")))
	must(t, os.Symlink("../../../..", filepath.Join(rp, "up")))
	read := map[string]string{}
	vs, err := walkAll(t, fsys, func(v visit) error {
		if v.open == nil {
			return nil
		}
		rc, err := v.open()
		if err != nil {
			t.Errorf("open(%s) : %v", v.p, err)
			return nil
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		read[v.p] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk : %v", err)
	}
	for _, v := range vs {
		if v.p != CleanPath(v.p) || v.p == "/" {
			t.Errorf("chemin %q : pas un chemin propre sous la racine", v.p)
		}
	}
	for p, c := range read {
		if c == "HOST-SECRET" || created[p] != c {
			t.Errorf("%s : lu %q, attendu %q", p, c, created[p])
		}
	}
	if len(read) != len(created) {
		t.Errorf("fichiers lus : %v, attendu %v", read, created)
	}
}

// Between the listing of /d and the open of /d/x, /d becomes a link to a
// directory of the host that holds x too: the walk must not read it.
func TestWalkLinkRace(t *testing.T) {
	fsys, rp, marker := newRoot(t)
	must(t, os.Mkdir(filepath.Join(rp, "d"), 0o755))
	must(t, os.WriteFile(filepath.Join(rp, "d", "x"), []byte("dans la racine"), 0o644))
	hostDir := filepath.Dir(marker)
	must(t, os.WriteFile(filepath.Join(hostDir, "x"), []byte("HOST-SECRET"), 0o644))
	old := walkBeforeOpen
	t.Cleanup(func() { walkBeforeOpen = old })
	swapped := false
	walkBeforeOpen = func(p string) {
		if p == "/d/x" && !swapped {
			swapped = true
			must(t, os.Rename(filepath.Join(rp, "d"), filepath.Join(rp, "d.old")))
			must(t, os.Symlink(hostDir, filepath.Join(rp, "d")))
		}
	}
	var got string
	vs, err := walkAll(t, fsys, func(v visit) error {
		if v.p == "/d/x" && v.open != nil {
			rc, err := v.open()
			if err == nil {
				b, _ := io.ReadAll(rc)
				rc.Close()
				got = string(b)
			} else if !errors.Is(err, ErrLinkInPath) {
				t.Errorf("open(/d/x) : %v, attendu ErrLinkInPath", err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Walk : %v", err)
	}
	if !swapped {
		t.Fatal("crochet jamais appelé pour /d/x")
	}
	if got != "" {
		t.Fatalf("/d/x lu après le remplacement : %q", got)
	}
	v, ok := find(vs, "/d/x")
	if !ok || v.e.Type != TypeFile || v.open == nil {
		t.Fatalf("/d/x : %+v (présent : %v), attendu un fichier dont open échoue", v.e, ok)
	}
}

var atime2001 = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

// ageAtime sets the atime of f to 2001, its mtime to now, and returns it.
func ageAtime(t *testing.T, f string) {
	t.Helper()
	must(t, unix.UtimesNanoAt(unix.AT_FDCWD, f, []unix.Timespec{unix.NsecToTimespec(atime2001.UnixNano()), unix.NsecToTimespec(time.Now().UnixNano())}, 0))
}

func atimeOf(t *testing.T, f string) time.Time {
	t.Helper()
	var st unix.Stat_t
	must(t, unix.Stat(f, &st))
	return time.Unix(st.Atim.Unix()).UTC()
}

// relatime updates an atime older than the mtime on any read: the atime is
// aged first, else the test would pass without O_NOATIME. A plain read of
// a sibling must change it (else the mount is noatime: no conclusion).
func TestWalkNoAtime(t *testing.T) {
	requireRoot(t)
	rp := t.TempDir()
	if err := unix.Mount("tmpfs", rp, "tmpfs", 0, "size=4m"); err == nil {
		t.Cleanup(func() { unix.Unmount(rp, unix.MNT_DETACH) })
	}
	fsys := openTestRoot(t, rp)
	f := filepath.Join(rp, "f")
	must(t, os.WriteFile(f, []byte("contenu"), 0o644))
	ageAtime(t, f)
	probe := filepath.Join(rp, "probe")
	must(t, os.WriteFile(probe, []byte("témoin"), 0o644))
	ageAtime(t, probe)
	os.ReadFile(probe)
	if atimeOf(t, probe).Equal(atime2001) {
		t.Skip("une lecture sans O_NOATIME ne change pas l'atime ici (montage noatime) : test non concluant")
	}
	_, err := walkAll(t, fsys, func(v visit) error {
		if v.p == "/f" {
			rc, err := v.open()
			if err != nil {
				t.Fatalf("open : %v", err)
			}
			io.ReadAll(rc)
			rc.Close()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Walk : %v", err)
	}
	if at := atimeOf(t, f); !at.Equal(atime2001) {
		t.Fatalf("atime %v après la lecture, attendu %v (O_NOATIME absent)", at, atime2001)
	}
}

func TestWalkComponentOrder(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	must(t, os.MkdirAll(filepath.Join(rp, "a", "b"), 0o755))
	must(t, os.WriteFile(filepath.Join(rp, "a-b"), nil, 0o644))
	must(t, os.WriteFile(filepath.Join(rp, "a", "b", "c"), nil, 0o644))
	vs, err := walkAll(t, fsys, nil)
	if err != nil {
		t.Fatalf("Walk : %v", err)
	}
	if got, want := paths(vs), []string{"/a", "/a/b", "/a/b/c", "/a-b"}; !slices.Equal(got, want) {
		t.Fatalf("ordre : %v, attendu %v", got, want)
	}
	// Many names: the order of readdir (a hash on ext4, overlay) is not
	// the order of the names.
	d := filepath.Join(rp, "many")
	must(t, os.Mkdir(d, 0o755))
	var want []string
	for i := range 200 {
		n := strconv.Itoa(i*7919%1000) + "-x"
		must(t, os.WriteFile(filepath.Join(d, n), nil, 0o644))
		want = append(want, "/many/"+n)
	}
	slices.Sort(want)
	vs, err = walkAll(t, fsys, nil)
	if err != nil {
		t.Fatalf("Walk : %v", err)
	}
	var got []string
	for _, v := range vs {
		if strings.HasPrefix(v.p, "/many/") {
			got = append(got, v.p)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("ordre de /many : %v", got[:10])
	}
}

// Past MaxEntries names in a directory, the walk goes on and says it is
// partial (ErrTruncated), as List does with truncated.
func TestWalkTruncatedDirectory(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	d := filepath.Join(rp, "big")
	must(t, os.Mkdir(d, 0o755))
	for i := range MaxEntries + 5 {
		must(t, os.WriteFile(filepath.Join(d, strconv.Itoa(i)), nil, 0o644))
	}
	must(t, os.WriteFile(filepath.Join(rp, "z"), nil, 0o644))
	vs, err := walkAll(t, fsys, nil)
	if !errors.Is(err, ErrTruncated) {
		t.Fatalf("Walk : %v, attendu ErrTruncated", err)
	}
	n := 0
	for _, v := range vs {
		if path.Dir(v.p) == "/big" {
			n++
		}
	}
	if n != MaxEntries {
		t.Errorf("%d entrées de /big, attendu %d", n, MaxEntries)
	}
	if _, ok := find(vs, "/z"); !ok {
		t.Error("/z absent : le parcours s'est arrêté au dossier tronqué")
	}
}

// The pinned process ends during the walk: its result is thrown away
// (container_changed), never served as a whole tree.
func TestWalkProcessExited(t *testing.T) {
	p, cmd := childView(t)
	killed := false
	err := p.Walk(ctx, func(path string, e Entry, _ func() (io.ReadCloser, error)) error {
		if !killed {
			killed = true
			cmd.Process.Kill()
			cmd.Wait()
		}
		if e.Type == TypeDir {
			return fs.SkipDir
		}
		return nil
	})
	if !killed || !errors.Is(err, ErrContainerChanged) {
		t.Fatalf("Walk : %v, attendu ErrContainerChanged", err)
	}
}

// A read that never answers (a FUSE server, spec §13) is given up after
// walkReadTimeout; the caller's buffer is never written afterwards.
func TestWalkReadTimeout(t *testing.T) {
	old := walkReadTimeout
	t.Cleanup(func() { walkReadTimeout = old })
	walkReadTimeout = 50 * time.Millisecond
	r, wp, err := os.Pipe()
	must(t, err)
	t.Cleanup(func() { r.Close(); wp.Close() })
	tf := &timedFile{f: r, w: &walk{ctx: ctx}, buf: make([]byte, 16)}
	buf := make([]byte, 8)
	start := time.Now()
	done := make(chan error, 1)
	go func() {
		_, err := tf.Read(buf)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, errReadTimeout) {
			t.Fatalf("Read : %v, attendu errReadTimeout", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("lecture jamais abandonnée")
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("abandon après %v", d)
	}
	wp.Write([]byte("tardif!!"))
	time.Sleep(20 * time.Millisecond)
	if string(buf) == "tardif!!" {
		t.Fatal("tampon de l'appelant écrit après l'abandon")
	}
	if _, err := tf.Read(buf); !errors.Is(err, errReadTimeout) {
		t.Fatalf("Read après l'abandon : %v", err)
	}
}

// fs.SkipAll ends the walk without error; the context too, with its error.
func TestWalkStops(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	for _, n := range []string{"a", "b", "c"} {
		must(t, os.WriteFile(filepath.Join(rp, n), nil, 0o644))
	}
	vs, err := walkAll(t, fsys, func(visit) error { return fs.SkipAll })
	if err != nil || len(vs) != 1 {
		t.Fatalf("SkipAll : %d visites, %v", len(vs), err)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := fsys.(Walker).Walk(cctx, func(string, Entry, func() (io.ReadCloser, error)) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("contexte annulé : %v", err)
	}
}
