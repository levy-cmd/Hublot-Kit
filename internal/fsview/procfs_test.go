//go:build linux

package fsview

// Evasion tests of procfs (spec §5.1, §10), written before the code. A
// temporary directory plays the container root; a host file outside it holds
// HOST-SECRET. Building the traps uses host paths (filepath, os.Symlink): that
// is the test fixture, never the code under test, which only sees the FS.

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

var ctx = context.Background()

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// openTestRoot opens dir the way OpenRoot opens /proc/<pid>/root.
func openTestRoot(t *testing.T, dir string) FS {
	t.Helper()
	fd, err := unix.Open(dir, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	must(t, err)
	fsys := NewProcFS(os.NewFile(uintptr(fd), "test-root"))
	t.Cleanup(func() { fsys.Close() })
	return fsys
}

// newRoot returns a fake container root and a host file outside it.
func newRoot(t *testing.T) (fsys FS, rootPath, marker string) {
	t.Helper()
	marker = filepath.Join(t.TempDir(), "MARKER")
	must(t, os.WriteFile(marker, []byte("HOST-SECRET"), 0o644))
	rootPath = t.TempDir()
	return openTestRoot(t, rootPath), rootPath, marker
}

func readAll(fsys FS, p string) ([]byte, error) {
	rc, err := fsys.Open(ctx, p)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// control proves the trap works: a naive read through the host path escapes.
func control(t *testing.T, hostPath string) {
	t.Helper()
	if b, err := os.ReadFile(hostPath); err != nil || string(b) != "HOST-SECRET" {
		t.Fatalf("contrôle : la lecture naïve devait sortir de la racine, lu %q err=%v", b, err)
	}
}

// assertNoEscape: the read never saw the secret (ÉVASION otherwise), and
// failed with "not found" (a wrong error is reported apart: it is a bug, not
// an escape).
func assertNoEscape(t *testing.T, b []byte, err error) {
	t.Helper()
	if bytes.Contains(b, []byte("HOST-SECRET")) {
		t.Fatalf("ÉVASION : le secret de l'hôte a été lu (%q)", b)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("mauvaise erreur : %v (attendu fs.ErrNotExist ; pas d'évasion)", err)
	}
}

func TestEvasionAbsoluteSymlink(t *testing.T) {
	fsys, rp, marker := newRoot(t)
	must(t, os.Symlink(marker, filepath.Join(rp, "evil")))
	control(t, filepath.Join(rp, "evil"))
	b, err := readAll(fsys, "/evil")
	assertNoEscape(t, b, err)
}

func TestEvasionRelativeSymlink(t *testing.T) {
	fsys, rp, marker := newRoot(t)
	must(t, os.Symlink(strings.Repeat("../", 20)+strings.TrimPrefix(marker, "/"), filepath.Join(rp, "evil-rel")))
	control(t, filepath.Join(rp, "evil-rel"))
	b, err := readAll(fsys, "/evil-rel")
	assertNoEscape(t, b, err)
}

func TestEvasionDotDotPath(t *testing.T) {
	fsys, rp, marker := newRoot(t)
	dotdot := strings.Repeat("../", 20) + strings.TrimPrefix(marker, "/")
	control(t, rp+"/"+dotdot)
	b, err := readAll(fsys, "/"+dotdot)
	assertNoEscape(t, b, err)
}

func TestEvasionLinkToLink(t *testing.T) {
	fsys, rp, marker := newRoot(t)
	must(t, os.Symlink("b", filepath.Join(rp, "a")))
	must(t, os.Symlink(marker, filepath.Join(rp, "b")))
	control(t, filepath.Join(rp, "a"))
	b, err := readAll(fsys, "/a")
	assertNoEscape(t, b, err)

	// A chain that stays inside the root still works.
	must(t, os.WriteFile(filepath.Join(rp, "in.txt"), []byte("inside"), 0o644))
	must(t, os.Symlink("d", filepath.Join(rp, "c")))
	must(t, os.Symlink("/in.txt", filepath.Join(rp, "d")))
	if b, err := readAll(fsys, "/c"); err != nil || string(b) != "inside" {
		t.Fatalf("chaîne interne : %q %v", b, err)
	}
}

func TestEvasionIntermediateSymlinkDir(t *testing.T) {
	fsys, rp, marker := newRoot(t)
	must(t, os.Symlink(filepath.Dir(marker), filepath.Join(rp, "evil-dir")))
	control(t, filepath.Join(rp, "evil-dir", "MARKER"))
	b, err := readAll(fsys, "/evil-dir/MARKER")
	assertNoEscape(t, b, err)
	if e, err := fsys.Stat(ctx, "/evil-dir/MARKER"); err == nil {
		t.Fatalf("ÉVASION : Stat a vu le fichier de l'hôte (%+v)", e)
	} else if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("mauvaise erreur pour Stat : %v", err)
	}
	entries, _, err := fsys.List(ctx, "/evil-dir")
	if len(entries) != 0 {
		t.Fatalf("ÉVASION : List a rendu le dossier de l'hôte %v", entries)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("mauvaise erreur pour List : %v", err)
	}
}

func TestSymlinkLoopFails(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	must(t, os.Symlink("loop2", filepath.Join(rp, "loop1")))
	must(t, os.Symlink("loop1", filepath.Join(rp, "loop2")))
	if _, err := readAll(fsys, "/loop1"); !errors.Is(err, unix.ELOOP) {
		t.Fatalf("une boucle de liens doit échouer avec ELOOP, reçu %v", err)
	}
	if _, _, err := fsys.List(ctx, "/loop1"); !errors.Is(err, unix.ELOOP) {
		t.Fatalf("une boucle de liens doit échouer avec ELOOP au listing, reçu %v", err)
	}
}

func TestListShowsLinksWithoutFollowing(t *testing.T) {
	fsys, rp, marker := newRoot(t)
	must(t, os.WriteFile(filepath.Join(rp, "a.txt"), []byte("hello"), 0o640))
	must(t, os.Chmod(filepath.Join(rp, "a.txt"), 0o640)) // the umask may have changed it
	must(t, os.Mkdir(filepath.Join(rp, "dir"), 0o755))
	must(t, os.Symlink("/etc/passwd", filepath.Join(rp, "link")))
	must(t, os.Symlink(marker, filepath.Join(rp, "hostlink")))
	entries, truncated, err := fsys.List(ctx, "/")
	must(t, err)
	if truncated {
		t.Fatal("troncature inattendue")
	}
	got := map[string]Entry{}
	for _, e := range entries {
		got[e.Name] = e
	}
	if e := got["a.txt"]; e.Type != TypeFile || e.Size != 5 || e.Mode != 0o640 || e.UID != os.Getuid() || e.ModTime.IsZero() {
		t.Errorf("a.txt : %+v", e)
	}
	if e := got["dir"]; e.Type != TypeDir {
		t.Errorf("dir : %+v", e)
	}
	if e := got["link"]; e.Type != TypeSymlink || e.LinkTarget != "/etc/passwd" || e.Size != int64(len("/etc/passwd")) {
		t.Errorf("link : %+v (le lien doit être montré, jamais suivi)", e)
	}
	if e := got["hostlink"]; e.Type != TypeSymlink || e.LinkTarget != marker || e.Size != int64(len(marker)) {
		t.Errorf("hostlink : %+v (lstat, pas la taille du fichier de l'hôte)", e)
	}
}

func TestListOddNames(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	names := []string{"with space", "émoji-🎉", "new\nline", "latin1-\xe9", "-dash"}
	for _, n := range names {
		must(t, os.WriteFile(filepath.Join(rp, n), nil, 0o644))
	}
	entries, _, err := fsys.List(ctx, "/")
	must(t, err)
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.Name] = true
	}
	for _, n := range names {
		if !seen[n] {
			t.Errorf("absent : %q", n)
		}
	}
}

func TestListTruncatesAtMaxEntries(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	big := filepath.Join(rp, "big")
	must(t, os.Mkdir(big, 0o755))
	for i := 0; i <= MaxEntries; i++ {
		f, err := os.Create(filepath.Join(big, fmt.Sprintf("f%05d", i)))
		must(t, err)
		f.Close()
	}
	entries, truncated, err := fsys.List(ctx, "/big")
	must(t, err)
	if len(entries) != MaxEntries || !truncated {
		t.Fatalf("%d entrées, truncated=%v ; attendu %d, true", len(entries), truncated, MaxEntries)
	}
	must(t, os.Remove(filepath.Join(big, "f00000")))
	entries, truncated, err = fsys.List(ctx, "/big")
	must(t, err)
	if len(entries) != MaxEntries || truncated {
		t.Fatalf("%d entrées, truncated=%v ; attendu %d, false", len(entries), truncated, MaxEntries)
	}
}

func TestOpenAndStatInRoot(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	must(t, os.MkdirAll(filepath.Join(rp, "d"), 0o755))
	must(t, os.WriteFile(filepath.Join(rp, "d", "f.txt"), []byte("in-root"), 0o644))
	must(t, os.Symlink("d/f.txt", filepath.Join(rp, "l")))
	if b, err := readAll(fsys, "/d/f.txt"); err != nil || string(b) != "in-root" {
		t.Fatalf("lecture : %q %v", b, err)
	}
	if b, err := readAll(fsys, "/l"); err != nil || string(b) != "in-root" {
		t.Fatalf("lecture par un lien interne : %q %v", b, err)
	}
	e, err := fsys.Stat(ctx, "/l")
	if err != nil || e.Type != TypeFile || e.Size != 7 || e.Name != "l" {
		t.Fatalf("Stat suit les liens : %+v %v", e, err)
	}
	if _, _, err := fsys.List(ctx, "/d/f.txt"); !errors.Is(err, ErrNotDir) {
		t.Fatalf("lister un fichier : %v, attendu ErrNotDir", err)
	}
	if _, err := fsys.Stat(ctx, "/absent"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("absent : %v", err)
	}
}

func TestModeSpecialBitsFromDisk(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	must(t, os.WriteFile(filepath.Join(rp, "suid"), nil, 0o755))
	must(t, os.Chmod(filepath.Join(rp, "suid"), 0o755|os.ModeSetuid))
	must(t, os.Mkdir(filepath.Join(rp, "tmp"), 0o777))
	must(t, os.Chmod(filepath.Join(rp, "tmp"), 0o777|os.ModeSticky))
	if e, err := fsys.Stat(ctx, "/suid"); err != nil || e.Mode != fs.ModeSetuid|0o755 {
		t.Fatalf("setuid : %v %v", e.Mode, err)
	}
	if e, err := fsys.Stat(ctx, "/tmp"); err != nil || e.Mode != fs.ModeSticky|0o777 {
		t.Fatalf("sticky : %v %v", e.Mode, err)
	}
}

// openWithTimeout fails the test if Open blocks (a FIFO opened for reading
// without O_NONBLOCK waits for a writer forever).
func openWithTimeout(t *testing.T, fsys FS, p string) error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		rc, err := fsys.Open(ctx, p)
		if err == nil {
			rc.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("Open a bloqué sur un fichier non régulier")
		return nil
	}
}

func TestOpenRefusesFIFO(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	must(t, unix.Mkfifo(filepath.Join(rp, "fifo"), 0o644))
	if err := openWithTimeout(t, fsys, "/fifo"); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("attendu ErrNotRegular, reçu %v", err)
	}
}

func TestOpenRefusesSocket(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	l, err := net.Listen("unix", filepath.Join(rp, "sock"))
	must(t, err)
	defer l.Close()
	if err := openWithTimeout(t, fsys, "/sock"); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("attendu ErrNotRegular, reçu %v", err)
	}
}

func TestOpenRefusesDirectory(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	must(t, os.Mkdir(filepath.Join(rp, "d"), 0o755))
	if err := openWithTimeout(t, fsys, "/d"); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("attendu ErrNotRegular, reçu %v", err)
	}
}

func TestOpenRefusesDevice(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	// /dev/null's numbers: reading it would read the host device.
	if err := unix.Mknod(filepath.Join(rp, "dev"), unix.S_IFCHR|0o666, int(unix.Mkdev(1, 3))); errors.Is(err, unix.EPERM) {
		t.Skipf("mknod refusé (%v) : ce test demande CAP_MKNOD (scripts/test-linux.sh, job CI privilégié)", err)
	} else if err != nil {
		t.Fatalf("mknod : %v", err)
	}
	if err := openWithTimeout(t, fsys, "/dev"); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("attendu ErrNotRegular, reçu %v", err)
	}
}

// The pseudo-filesystem tests use the test machine's own "/" as the root: its
// /proc and /sys are real procfs and sysfs.
func TestPseudoFSRefused(t *testing.T) {
	host := openTestRoot(t, "/")
	if _, err := readAll(host, "/proc/kallsyms"); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("/proc/kallsyms : %v, attendu ErrForbiddenPath", err)
	}
	if _, err := host.Stat(ctx, "/proc"); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("Stat /proc : %v", err)
	}
	if _, _, err := host.List(ctx, "/proc"); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("List /proc : %v", err)
	}
	if _, _, err := host.List(ctx, "/sys"); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("List /sys : %v", err)
	}
	// A link that leads there is refused too: the check is on the descriptor.
	dir := t.TempDir()
	must(t, os.Symlink("/proc", filepath.Join(dir, "p")))
	if _, err := readAll(host, dir+"/p/kallsyms"); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("lien vers /proc : %v", err)
	}
	if _, _, err := host.List(ctx, dir+"/p"); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("List du lien vers /proc : %v", err)
	}
}

func TestRootListingShowsPseudoFSAsDirs(t *testing.T) {
	host := openTestRoot(t, "/")
	entries, _, err := host.List(ctx, "/")
	must(t, err)
	for _, e := range entries {
		if e.Name == "proc" {
			if e.Type != TypeDir {
				t.Fatalf("proc : %+v", e)
			}
			return
		}
	}
	t.Fatal("proc absent du listing de /")
}

func TestDevIsListableButNotReadable(t *testing.T) {
	host := openTestRoot(t, "/")
	if _, _, err := host.List(ctx, "/dev"); err != nil {
		t.Fatalf("List /dev : %v", err)
	}
	if err := openWithTimeout(t, host, "/dev/null"); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("/dev/null : %v, attendu ErrNotRegular", err)
	}
}

func TestStatNoLinks(t *testing.T) {
	fsys, rp, marker := newRoot(t)
	must(t, os.MkdirAll(filepath.Join(rp, "d"), 0o755))
	must(t, os.WriteFile(filepath.Join(rp, "d", "f.txt"), []byte("x"), 0o644))
	must(t, os.Symlink("d", filepath.Join(rp, "ld")))
	must(t, os.Symlink("d/f.txt", filepath.Join(rp, "lf")))
	must(t, os.Symlink("/nope", filepath.Join(rp, "dangling")))
	must(t, os.Symlink(filepath.Dir(marker), filepath.Join(rp, "evil")))

	for _, p := range []string{"/d/f.txt", "/d", "/", "/../../d/f.txt", "d//f.txt"} {
		if _, err := fsys.StatNoLinks(ctx, p); err != nil {
			t.Errorf("%q : %v", p, err)
		}
	}
	if e, err := fsys.StatNoLinks(ctx, "/d/f.txt"); err != nil || e.Type != TypeFile || e.Name != "f.txt" {
		t.Errorf("entrée : %+v %v", e, err)
	}
	for _, p := range []string{"/lf", "/ld/f.txt", "/ld", "/dangling", "/evil/MARKER"} {
		if _, err := fsys.StatNoLinks(ctx, p); !errors.Is(err, ErrLinkInPath) {
			t.Errorf("%q : %v, attendu ErrLinkInPath", p, err)
		}
	}
	if _, err := fsys.StatNoLinks(ctx, "/absent"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("absent : %v", err)
	}
	host := openTestRoot(t, "/")
	if _, err := host.StatNoLinks(ctx, "/proc/kallsyms"); !errors.Is(err, ErrForbiddenPath) {
		t.Errorf("/proc/kallsyms : %v, attendu ErrForbiddenPath", err)
	}
}

// countOpens returns the number of IN_OPEN events on hostPath while fn runs.
// An O_PATH open (pathrs.OpenatInRoot) generates none; a Reopen for reading does.
func countOpens(t *testing.T, hostPath string, fn func()) int {
	t.Helper()
	in, err := unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC)
	must(t, err)
	defer unix.Close(in)
	_, err = unix.InotifyAddWatch(in, hostPath, unix.IN_OPEN)
	must(t, err)
	fn()
	buf := make([]byte, 64*unix.SizeofInotifyEvent)
	n, err := unix.Read(in, buf)
	if err != nil || n <= 0 {
		return 0 // EAGAIN: no event
	}
	count := 0
	for off := 0; off+unix.SizeofInotifyEvent <= n; count++ {
		nameLen := binary.NativeEndian.Uint32(buf[off+12:]) // struct inotify_event.len
		off += unix.SizeofInotifyEvent + int(nameLen)
	}
	return count
}

// The refusal of a non-regular file must happen on the O_PATH descriptor,
// before any Reopen: the FIFO and the device node are never opened for
// reading (zero IN_OPEN), not just closed right after.
func TestRefusalHappensBeforeReopen(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	reg := filepath.Join(rp, "reg")
	must(t, os.WriteFile(reg, []byte("x"), 0o644))
	// Calibration with the test's own O_PATH open, not with the code under test.
	if n := countOpens(t, reg, func() {
		if fd, err := unix.Open(reg, unix.O_PATH|unix.O_CLOEXEC, 0); err == nil {
			unix.Close(fd)
		}
	}); n != 0 {
		t.Skipf("ce noyau signale IN_OPEN pour un O_PATH (%d événements) : test non concluant", n)
	}
	if n := countOpens(t, reg, func() { fsys.Stat(ctx, "/reg") }); n != 0 {
		t.Fatalf("Stat a ouvert le fichier %d fois : Stat ne doit jamais faire Reopen", n)
	}
	if n := countOpens(t, reg, func() { readAll(fsys, "/reg") }); n == 0 {
		t.Fatal("contrôle : la lecture d'un fichier régulier doit produire un IN_OPEN")
	}
	t.Run("fifo", func(t *testing.T) {
		fifo := filepath.Join(rp, "fifo")
		must(t, unix.Mkfifo(fifo, 0o644))
		if n := countOpens(t, fifo, func() { fsys.Stat(ctx, "/fifo") }); n != 0 {
			t.Fatalf("Stat a ouvert la FIFO %d fois : pas de Reopen avant le contrôle", n)
		}
		if n := countOpens(t, fifo, func() { openWithTimeout(t, fsys, "/fifo") }); n != 0 {
			t.Fatalf("FIFO ouverte %d fois : le refus doit précéder Reopen", n)
		}
	})
	t.Run("device", func(t *testing.T) {
		dev := filepath.Join(rp, "dev")
		if err := unix.Mknod(dev, unix.S_IFCHR|0o666, int(unix.Mkdev(1, 3))); errors.Is(err, unix.EPERM) {
			t.Skipf("mknod refusé (%v) : ce test demande CAP_MKNOD (scripts/test-linux.sh, job CI privilégié)", err)
		} else if err != nil {
			t.Fatalf("mknod : %v", err)
		}
		if n := countOpens(t, dev, func() { openWithTimeout(t, fsys, "/dev") }); n != 0 {
			t.Fatalf("périphérique ouvert %d fois : le refus doit précéder Reopen", n)
		}
	})
}

// A magic link (/proc/<pid>/root) leads out of any root: never followed,
// directly or through an ordinary link. The test machine's "/" is the root:
// following it would land on a readable file. /proc/self/root works without
// root (CI job "go"); /proc/1/root needs it.
func TestMagicLinksNeverFollowed(t *testing.T) {
	host := openTestRoot(t, "/")
	dir := t.TempDir()
	must(t, os.Symlink("/proc/1/root", filepath.Join(dir, "m")))
	must(t, os.Symlink("/proc/self/root", filepath.Join(dir, "s")))
	if _, err := os.ReadFile(dir + "/s/etc/passwd"); err != nil {
		t.Fatalf("contrôle : la lecture naïve par /proc/self/root doit réussir : %v", err)
	}
	for _, p := range []string{"/proc/1/root/etc/passwd", "/proc/self/root/etc/passwd",
		dir + "/m/etc/passwd", dir + "/s/etc/passwd"} {
		if rc, err := host.Open(ctx, p); err == nil {
			rc.Close()
			t.Errorf("%s ouvert : un lien magique ne doit jamais être suivi", p)
		}
	}
	for _, l := range []string{"/m", "/s"} {
		if entries, _, err := host.List(ctx, dir+l); len(entries) > 0 || err == nil {
			t.Errorf("List par le lien magique %s : %d entrées, err=%v", l, len(entries), err)
		}
		if _, err := host.StatNoLinks(ctx, dir+l+"/etc"); !errors.Is(err, ErrLinkInPath) {
			t.Errorf("StatNoLinks par le lien magique %s : %v", l, err)
		}
	}
	if _, err := host.StatNoLinks(ctx, "/proc/self/root/etc"); !errors.Is(err, ErrLinkInPath) && !errors.Is(err, ErrForbiddenPath) {
		t.Errorf("StatNoLinks /proc/self/root/etc : %v, attendu ErrLinkInPath ou ErrForbiddenPath", err)
	}
}
