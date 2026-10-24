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
	vs, err := walkAll(t, fsys, nil)
	if err != nil {
		t.Fatalf("Walk : %v", err)
	}
	read := map[string]string{}
	for _, v := range vs {
		if v.p != CleanPath(v.p) || v.p == "/" {
			t.Errorf("chemin %q : pas un chemin propre sous la racine", v.p)
		}
		if v.open == nil {
			continue
		}
		rc, err := v.open()
		if err != nil {
			t.Errorf("open(%s) : %v", v.p, err)
			continue
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		read[v.p] = string(b)
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

// relatime updates an atime older than the mtime on any read: the atime is
// aged first, else the test would pass without O_NOATIME.
func TestWalkNoAtime(t *testing.T) {
	requireRoot(t)
	rp := t.TempDir()
	mountTmpfs(t, rp)
	fsys := openTestRoot(t, rp)
	f := filepath.Join(rp, "f")
	must(t, os.WriteFile(f, []byte("contenu"), 0o644))
	old := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	must(t, unix.UtimesNanoAt(unix.AT_FDCWD, f, []unix.Timespec{unix.NsecToTimespec(old.UnixNano()), unix.NsecToTimespec(time.Now().UnixNano())}, 0))
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
	var st unix.Stat_t
	must(t, unix.Stat(f, &st))
	if at := time.Unix(st.Atim.Unix()).UTC(); !at.Equal(old) {
		t.Fatalf("atime %v après la lecture, attendu %v (O_NOATIME absent)", at, old)
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
