//go:build linux

package files

// Evasion tests of the writer on a real procfs view (spec §5.3 step 2, §10):
// a temporary directory plays the container root, the checks are openat2
// RESOLVE_NO_SYMLINKS and fstatfs. Building the traps uses host paths (the
// fixture); the code under test only sees the view. The fake Docker writes
// nothing: a refusal is proven by the absence of any PutArchive call.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// procRig serves dir as the container root through fsview.NewProcFS.
func procRig(t *testing.T, dir string) (*Writer, *dockerx.Fake, *journal) {
	t.Helper()
	fake := &dockerx.Fake{}
	j := &journal{}
	w := &Writer{Docker: fake, Audit: j, Open: func(context.Context, string) (fsview.FS, error) {
		fd, err := unix.Open(dir, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
		if err != nil {
			return nil, err
		}
		return fsview.NewProcFS(os.NewFile(uintptr(fd), "test-root")), nil
	}}
	return w, fake, j
}

// trapRoot: /d/f.txt ("x"), /d/fifo, links /lf → d/f.txt, /ld → d,
// /d/dangling → /nope, /evil → a host directory outside the root.
func trapRoot(t *testing.T) (root, hostDir string) {
	t.Helper()
	hostDir = t.TempDir()
	must(t, os.WriteFile(filepath.Join(hostDir, "MARKER"), []byte("HOST-SECRET"), 0o644))
	root = t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, "d"), 0o755))
	must(t, os.WriteFile(filepath.Join(root, "d", "f.txt"), []byte("x"), 0o640))
	must(t, unix.Mkfifo(filepath.Join(root, "d", "fifo"), 0o644))
	must(t, os.Symlink("d/f.txt", filepath.Join(root, "lf")))
	must(t, os.Symlink("d", filepath.Join(root, "ld")))
	must(t, os.Symlink("/nope", filepath.Join(root, "d", "dangling")))
	must(t, os.Symlink(hostDir, filepath.Join(root, "evil")))
	return root, hostDir
}

// noAudit: a refusal leaves no audit line, even when an error is returned.
func noAudit(t *testing.T, j *journal) {
	t.Helper()
	if s := j.Steps(); len(s) != 0 {
		t.Fatalf("un refus ne laisse aucune ligne d'audit : %v", s)
	}
}

var xTag = ETag([]byte("x"))

// putDirs: the directories PutArchive received, without the tar bytes.
func putDirs(puts []dockerx.Put) []string {
	var dirs []string
	for _, p := range puts {
		dirs = append(dirs, p.Dir)
	}
	return dirs
}

func TestProcLinksRefused(t *testing.T) {
	root, _ := trapRoot(t)
	w, fake, j := procRig(t, root)
	// Control: a naive write path through /evil reaches the host.
	if _, err := os.Stat(filepath.Join(root, "evil", "MARKER")); err != nil {
		t.Fatalf("contrôle : le lien /evil devait mener à l'hôte : %v", err)
	}
	edits := []string{"/lf", "/ld/f.txt", "/evil/MARKER", "/d/dangling"}
	for _, p := range edits {
		if _, err := w.Edit(ctx, ctr, p, []byte("y"), xTag); !errors.Is(err, fsview.ErrLinkInPath) {
			t.Errorf("édition %s : %v, attendu ErrLinkInPath", p, err)
		}
	}
	uploads := [][2]string{{"/ld", "new"}, {"/evil", "new"}, {"/d", "dangling"}, {"/", "lf"}, {"/", "ld"}}
	for _, u := range uploads {
		if _, err := w.Upload(ctx, ctr, u[0], u[1], []byte("y"), "*"); !errors.Is(err, fsview.ErrLinkInPath) {
			t.Errorf("envoi %s/%s : %v, attendu ErrLinkInPath", u[0], u[1], err)
		}
	}
	// The fake Docker writes nothing: the proof is that it was never asked.
	if puts := fake.PutCalls(); len(puts) != 0 {
		t.Fatalf("ÉVASION : PutArchive appelé malgré un lien, dossiers %v", putDirs(puts))
	}
	noAudit(t, j)
}

func TestProcNotRegular(t *testing.T) {
	root, _ := trapRoot(t)
	w, fake, j := procRig(t, root)
	done := make(chan error, 1)
	go func() {
		_, err := w.Edit(ctx, ctr, "/d/fifo", []byte("y"), xTag)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, fsview.ErrNotRegular) {
			t.Errorf("FIFO : %v, attendu ErrNotRegular", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("la FIFO a bloqué l'édition")
	}
	if _, err := w.Edit(ctx, ctr, "/d", []byte("y"), xTag); !errors.Is(err, fsview.ErrNotRegular) {
		t.Errorf("dossier : %v", err)
	}
	if len(fake.PutCalls()) != 0 {
		t.Fatal("PutArchive appelé")
	}
	noAudit(t, j)
}

// The parent of an upload, or the target of an edit, on a pseudo filesystem
// → forbidden_path (spec §5.3 step 2, fstatfs). The host's /proc is the
// root: "/" and "/version" do not start with /proc, so the prefix rule of
// Q21 lets them through and only fstatfs on the descriptor refuses them.
func TestProcPseudoFSRefused(t *testing.T) {
	w, fake, j := procRig(t, "/proc")
	if _, err := w.Upload(ctx, ctr, "/", "x", []byte("y"), "*"); !errors.Is(err, fsview.ErrForbiddenPath) {
		t.Errorf("envoi dans un dossier procfs : %v, attendu ErrForbiddenPath", err)
	}
	if _, err := w.Edit(ctx, ctr, "/version", []byte("y"), xTag); !errors.Is(err, fsview.ErrForbiddenPath) {
		t.Errorf("édition d'un fichier procfs : %v, attendu ErrForbiddenPath", err)
	}
	if len(fake.PutCalls()) != 0 {
		t.Fatal("PutArchive appelé")
	}
	noAudit(t, j)
}

func TestProcWriteSendsExactPath(t *testing.T) {
	root, _ := trapRoot(t)
	uid, gid := os.Getuid(), os.Getgid()
	if os.Geteuid() == 0 {
		uid, gid = 1234, 5678
		must(t, os.Chown(filepath.Join(root, "d", "f.txt"), uid, gid))
		must(t, os.Chown(filepath.Join(root, "d"), uid+1, gid+1))
	}
	w, fake, _ := procRig(t, root)
	if _, err := w.Edit(ctx, ctr, "/d/./f.txt", []byte("y"), xTag); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Upload(ctx, ctr, "/d", "new.txt", []byte("z"), "*"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Upload(ctx, ctr, "/d", "f.txt", []byte("z"), "*"); !errors.Is(err, ErrConflict) {
		t.Errorf("fichier existant : %v, attendu ErrConflict", err)
	}
	puts := fake.PutCalls()
	if len(puts) != 2 || puts[0].Dir != "/d" || puts[1].Dir != "/d" {
		t.Fatalf("PutArchive doit recevoir le dossier parent nettoyé /d : %v", putDirs(puts))
	}
	h, body := entryOf(t, puts[0].Tar)
	if h.Name != "f.txt" || body != "y" || h.Mode != 0o640 || h.Uid != uid || h.Gid != gid {
		t.Errorf("édition : %+v %q", h, body)
	}
	h, _ = entryOf(t, puts[1].Tar)
	wantUID, wantGID := os.Getuid(), os.Getgid()
	if os.Geteuid() == 0 {
		wantUID, wantGID = uid+1, gid+1
	}
	if h.Name != "new.txt" || h.Mode != 0o644 || h.Uid != wantUID || h.Gid != wantGID {
		t.Errorf("envoi : %+v", h)
	}
}
