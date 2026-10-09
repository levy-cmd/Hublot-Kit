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
	restore := SetCgroupReaderForTests(func(int) ([]byte, error) { return []byte("0::/docker/" + id + "\n"), nil })
	t.Cleanup(restore)
	docker := &dockerx.Fake{Infos: map[string]dockerx.ContainerInfo{"c": {
		ID: id, Name: "c", Running: true, Pid: cmd.Process.Pid,
		Mounts: []dockerx.Mount{{Type: "bind", Source: "/proc", Destination: "/host/proc"}},
	}}}
	fsys, err := For(context.Background(), docker, "c")
	must(t, err)
	defer fsys.Close()
	err = fsys.CheckArchive(ctx, "/etc")
	if !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), "bind /proc -> /host/proc") {
		t.Fatalf("les montages d'Inspect doivent atteindre la vue : %v", err)
	}
}
