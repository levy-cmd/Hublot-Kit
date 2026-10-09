//go:build linux

package fsview

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
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
