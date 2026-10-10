//go:build linux

package fsview

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestOpenRootRejectsForeignCgroup(t *testing.T) {
	_, err := OpenRoot(os.Getpid(), strings.Repeat("a", 64))
	if !errors.Is(err, ErrContainerChanged) {
		t.Fatalf("attendu ErrContainerChanged, reçu %v", err)
	}
}

func TestOpenRootRejectsMissingPid(t *testing.T) {
	_, err := OpenRoot(0x7ffffffe, strings.Repeat("a", 64))
	if !errors.Is(err, ErrContainerChanged) {
		t.Fatalf("attendu ErrContainerChanged, reçu %v", err)
	}
}

func TestOpenRootRejectsShortID(t *testing.T) {
	_, err := OpenRoot(os.Getpid(), "abc")
	if !errors.Is(err, ErrContainerChanged) {
		t.Fatalf("attendu ErrContainerChanged, reçu %v", err)
	}
}

// Spec §5.1 step 5: a process that dies between the open of its root and the
// liveness check must be detected (its PID may already belong to another
// process). The test pins one of its own children, gives it the container's
// cgroup through the readCgroup seam, and kills it at the afterRootOpen seam.
func TestOpenRootDetectsDeadProcess(t *testing.T) {
	id := strings.Repeat("ab", 32)
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	oldCgroup, oldHook := readCgroup, afterRootOpen
	t.Cleanup(func() { readCgroup, afterRootOpen = oldCgroup, oldHook })
	readCgroup = func(int) ([]byte, error) { return []byte("0::/docker/" + id + "\n"), nil }

	root, err := OpenRoot(cmd.Process.Pid, id) // control: alive, the sequence succeeds
	if err != nil {
		t.Fatalf("contrôle : %v", err)
	}
	root.Close()

	afterRootOpen = func() { cmd.Process.Kill(); cmd.Wait() } // dead and reaped: its PID is free
	if _, err := OpenRoot(cmd.Process.Pid, id); !errors.Is(err, ErrContainerChanged) {
		t.Fatalf("process mort après l'ouverture de sa racine : %v, attendu ErrContainerChanged", err)
	}
}

func TestAliveSeam(t *testing.T) {
	old := pidfdExited
	t.Cleanup(func() { pidfdExited = old })
	for _, c := range []struct {
		exited      bool
		err         error
		ok, changed bool
	}{
		{false, nil, true, false},
		{true, nil, false, true},
		{false, unix.EBADF, false, false}, // poll failed: internal, never container_changed
	} {
		pidfdExited = func(int) (bool, error) { return c.exited, c.err }
		if err := alive(3, 42); (err == nil) != c.ok || errors.Is(err, ErrContainerChanged) != c.changed {
			t.Errorf("%+v : %v", c, err)
		}
	}
}

// Real processes. PID 1 cannot be signalled without root (the old EPERM):
// alive. A zombie still received signal 0: it must now count as exited.
func TestAliveReal(t *testing.T) {
	pidfd, err := unix.PidfdOpen(1, 0)
	if err != nil {
		t.Fatalf("pidfd_open(1) : %v", err)
	}
	defer unix.Close(pidfd)
	if err := alive(pidfd, 1); err != nil {
		t.Errorf("PID 1 : %v", err)
	}
	cmd := exec.Command("sleep", "30")
	must(t, cmd.Start())
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	cfd, err := unix.PidfdOpen(cmd.Process.Pid, 0)
	must(t, err)
	defer unix.Close(cfd)
	if err := alive(cfd, cmd.Process.Pid); err != nil {
		t.Fatalf("vivant : %v", err)
	}
	// Not reaped: a zombie. A pidfd polls readable once its process exits,
	// before the reap (Linux >= 5.3, already required by spec §5.1).
	must(t, cmd.Process.Kill())
	for deadline := time.Now().Add(5 * time.Second); !errors.Is(alive(cfd, cmd.Process.Pid), ErrContainerChanged); time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("un zombie compte encore comme vivant")
		}
	}
}
