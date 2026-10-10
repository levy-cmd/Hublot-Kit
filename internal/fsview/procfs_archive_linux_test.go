//go:build linux

package fsview

// CheckArchive on a real process of the test machine (no Docker): one of the
// test's own children plays the container. Its mount table is the test
// machine's, where /proc and /sys are pseudo filesystems.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

func startChild(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	must(t, cmd.Start())
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	return cmd
}

func childView(t *testing.T) (*procFS, *exec.Cmd) {
	t.Helper()
	cmd := startChild(t)
	pid := cmd.Process.Pid
	pidfd, err := unix.PidfdOpen(pid, 0)
	must(t, err)
	fd, err := unix.Open(fmt.Sprintf("/proc/%d/root", pid), unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	must(t, err)
	p := &procFS{root: os.NewFile(uintptr(fd), "root"), pid: pid, pidfd: pidfd, docker: &dockerx.Fake{}}
	t.Cleanup(func() { p.Close() })
	return p, cmd
}

func TestCheckArchiveMountTable(t *testing.T) {
	p, _ := childView(t)
	for _, dir := range []string{"/", "/proc", "/proc/1", "/sys", "/sys/fs"} {
		if err := p.CheckArchive(ctx, dir); !errors.Is(err, ErrForbiddenPath) {
			t.Errorf("%s : %v, attendu ErrForbiddenPath", dir, err)
		}
	}
	if err := p.CheckArchive(ctx, "/etc"); err != nil {
		t.Errorf("/etc : %v", err)
	}
}

func TestCheckArchiveHostBind(t *testing.T) {
	p, _ := childView(t)
	p.mounts = []dockerx.Mount{{Type: "bind", Source: "/proc", Destination: "/host/proc"}}
	err := p.CheckArchive(ctx, "/etc")
	if !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), "bind /proc -> /host/proc") {
		t.Fatalf("bind de /proc : %v", err)
	}
}

// Review R2: a pseudo filesystem under a Docker mount refuses every directory,
// not only those that overlap it. Here the test machine's /sys plays a mount
// found under the destination of a harmless Docker mount.
func TestCheckArchiveContainerWideRule(t *testing.T) {
	p, _ := childView(t)
	p.mounts = []dockerx.Mount{{Type: "tmpfs", Destination: "/sys"}}
	err := p.CheckArchive(ctx, "/etc")
	if !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), "sous le montage /sys") {
		t.Fatalf("montage dangereux sous un montage Docker : %v", err)
	}
}

// Review R1 (ii): the mount table read must belong to the pinned process. The
// child dies right after its table is read: CheckArchive must notice.
func TestCheckArchiveDeadProcess(t *testing.T) {
	p, cmd := childView(t)
	old := readMountinfo
	t.Cleanup(func() { readMountinfo = old })
	readPid := 0
	readMountinfo = func(pid int) ([]byte, error) {
		readPid = pid
		b, err := old(pid)
		cmd.Process.Kill()
		cmd.Wait() // reaped: its PID is free
		return b, err
	}
	if err := p.CheckArchive(ctx, "/etc"); !errors.Is(err, ErrContainerChanged) {
		t.Fatalf("process mort après la lecture de mountinfo : %v, attendu ErrContainerChanged", err)
	}
	if readPid != p.pid {
		t.Fatalf("mountinfo lu pour le pid %d, pas pour le process épinglé %d", readPid, p.pid)
	}
}

func TestCheckArchiveWithoutProcessFailsClosed(t *testing.T) {
	fsys, _, _ := newRoot(t)
	if err := fsys.CheckArchive(ctx, "/"); err == nil {
		t.Fatal("une vue sans process ne peut pas autoriser un tar")
	}
}

// Review R1 (i): the mounts of Inspect reach the view built by For.
func TestForWiresMountsIntoCheckArchive(t *testing.T) {
	cmd := startChild(t)
	id := strings.Repeat("ab", 32)
	docker := &dockerx.Fake{Infos: map[string]dockerx.ContainerInfo{"c": {
		ID: id, Name: "c", Running: true, Pid: cmd.Process.Pid,
		Mounts: []dockerx.Mount{{Type: "bind", Source: "/proc", Destination: "/host/proc"}},
	}}}
	fsys, err := NewViews(docker, Options{ReadCgroup: func(int) ([]byte, error) { return []byte("0::/docker/" + id + "\n"), nil }}).For(context.Background(), "c")
	must(t, err)
	defer fsys.Close()
	err = fsys.CheckArchive(ctx, "/etc")
	if !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), "bind /proc -> /host/proc") {
		t.Fatalf("les montages d'Inspect doivent atteindre la vue : %v", err)
	}
}

// Review fix 1: a Docker destination that the process's mount table does not
// show (moved root, symlink in the path) refuses the tar.
func TestCheckArchiveDestinationNotVisible(t *testing.T) {
	p, _ := childView(t)
	p.mounts = []dockerx.Mount{{Type: "tmpfs", Destination: "/link/m"}}
	err := p.CheckArchive(ctx, "/etc")
	if !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), "ne montre pas la destination /link/m") {
		t.Fatalf("destination invisible : %v", err)
	}
}

// Deferred issue (a) of plan 1.
func TestCheckArchiveDestinationUnderSymlink(t *testing.T) {
	p, cmd := childView(t)
	// Control first: under a symlink but not a mount, still refused.
	dir := t.TempDir()
	must(t, os.Mkdir(filepath.Join(dir, "real"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "real", "f"), nil, 0o644))
	must(t, os.Symlink("real", filepath.Join(dir, "link")))
	p.mounts = []dockerx.Mount{{Type: "bind", Source: "/x/f", Destination: filepath.Join(dir, "link", "f")}}
	if err := p.CheckArchive(ctx, "/etc"); !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), "ne montre pas la destination") {
		t.Fatalf("lien sans montage : %v", err)
	}
	// The real case: scripts/test-linux.sh mounts the Docker socket at
	// /var/run/docker.sock, and /var/run -> /run in the golang image. Its
	// Source is the mount's root as mountinfo shows it (host layouts differ).
	mi, err := os.ReadFile(fmt.Sprintf("/proc/%d/mountinfo", cmd.Process.Pid))
	must(t, err)
	root := ""
	for _, line := range strings.Split(string(mi), "\n") {
		if f := strings.Fields(line); len(f) > 4 && f[4] == "/run/docker.sock" {
			root = f[3]
		}
	}
	target, err := os.Readlink("/var/run")
	if err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, unix.EINVAL) {
		t.Fatalf("readlink /var/run : %v", err) // not the exact precondition: fail
	}
	if err == nil && !path.IsAbs(target) {
		target = path.Join("/var", target)
	}
	if err != nil || path.Clean(target) != "/run" {
		t.Skipf("précondition absente : /var/run n'est pas un lien vers /run (%q, %v) ; lancer par scripts/test-linux.sh", target, err)
	}
	if root == "" || root == "/" {
		t.Skipf("précondition absente : /run/docker.sock absent de mountinfo ou racine de montage %q ; lancer par scripts/test-linux.sh", root)
	}
	p.mounts = []dockerx.Mount{{Type: "bind", Source: root, Destination: "/var/run/docker.sock"}}
	if err := p.CheckArchive(ctx, "/etc"); err != nil {
		t.Fatalf("socket Docker sous /var/run -> /run : %v", err)
	}
}

// Review I1 (plan 3): a write needs the view's root to be the container's
// root. A mount table without "/" (process chrooted) refuses it. The mount
// table is faked: a real chrooted child needs a binary inside the jail.
func TestCheckWriteRoot(t *testing.T) {
	p, _ := childView(t)
	if err := p.CheckWriteRoot(ctx); err != nil {
		t.Fatalf("process non chrooté : %v", err)
	}
	old := readMountinfo
	t.Cleanup(func() { readMountinfo = old })
	readMountinfo = func(int) ([]byte, error) {
		return []byte("36 35 8:1 / /etc rw - ext4 /dev/sda1 rw\n"), nil
	}
	err := p.CheckWriteRoot(ctx)
	if !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), "racine") {
		t.Fatalf("racine déplacée : %v, attendu ErrForbiddenPath", err)
	}
	readMountinfo = func(int) ([]byte, error) { return nil, errors.New("illisible") }
	if err := p.CheckWriteRoot(ctx); err == nil {
		t.Fatal("mountinfo illisible : doit échouer fermé")
	}
}
