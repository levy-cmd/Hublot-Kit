//go:build linux

package procroot

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// Open returns an O_PATH handle on the root filesystem of the container whose
// main process is pid. Sequence from spec §5.1: pidfd → cgroup → root → pidfd alive.
func Open(pid int, containerID string) (*os.File, error) {
	pidfd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		return nil, fmt.Errorf("%w: pidfd_open(%d): %v", ErrChanged, pid, err)
	}
	defer unix.Close(pidfd)

	cg, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid))
	if err != nil {
		return nil, fmt.Errorf("%w: read cgroup: %v", ErrChanged, err)
	}
	if !CgroupHasID(string(cg), containerID) {
		return nil, fmt.Errorf("%w: pid %d is not in the container's cgroup", ErrChanged, pid)
	}

	fd, err := unix.Open(fmt.Sprintf("/proc/%d/root", pid), unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open root of pid %d: %w", pid, err)
	}
	// Still alive => the PID was not recycled between pidfd_open and the open above.
	if err := unix.PidfdSendSignal(pidfd, 0, nil, 0); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("%w: process gone: %v", ErrChanged, err)
	}
	return os.NewFile(uintptr(fd), "container-root"), nil
}
