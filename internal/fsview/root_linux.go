//go:build linux

package fsview

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// Test seams of spec §5.1 steps 3 and 5: tests give a fake cgroup content to
// a process of their own, and kill it between the open of its root and the
// liveness check.
var (
	readCgroup = func(pid int) ([]byte, error) {
		return os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid)) // a host file, not a container path
	}
	afterRootOpen = func() {}
)

// openRoot runs spec §5.1 steps 2 to 5 and returns an O_PATH descriptor on the
// root directory of the container whose main process is pid, and the pidfd
// that pins this process. The caller closes both.
func openRoot(pid int, containerID string, read func(int) ([]byte, error)) (*os.File, int, error) {
	if read == nil {
		read = readCgroup
	}
	pidfd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		return nil, -1, changed("pidfd_open(%d) : %v", pid, err)
	}
	fail := func(err error) (*os.File, int, error) {
		unix.Close(pidfd)
		return nil, -1, err
	}
	cg, err := read(pid)
	if err != nil {
		return fail(changed("lecture du cgroup du pid %d : %v", pid, err))
	}
	if !CgroupHasID(string(cg), containerID) {
		return fail(changed("le pid %d n'est pas dans le cgroup du conteneur %.12s", pid, containerID))
	}
	fd, err := unix.Open(fmt.Sprintf("/proc/%d/root", pid), unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	switch {
	case errors.Is(err, unix.ENOENT), errors.Is(err, unix.ESRCH):
		return fail(changed("ouverture de /proc/%d/root : %v", pid, err))
	case errors.Is(err, unix.EACCES):
		return fail(fmt.Errorf("ouverture de /proc/%d/root : %w ; causes mesurées (spec §8) : "+
			"--cap-add SYS_PTRACE absent, ou cible non confinée (--privileged, apparmor=unconfined) "+
			"sans --security-opt apparmor=unconfined sur l'agent", pid, err))
	case err != nil:
		return fail(fmt.Errorf("ouverture de /proc/%d/root : %w", pid, err))
	}
	afterRootOpen()
	// Still alive: the PID was not reused between pidfd_open and the open above,
	// so fd is the root of the process checked at step 3.
	if err := unix.PidfdSendSignal(pidfd, 0, nil, 0); err != nil {
		unix.Close(fd)
		return fail(changed("le process %d a disparu : %v", pid, err))
	}
	return os.NewFile(uintptr(fd), "container-root"), pidfd, nil
}

// OpenRoot is openRoot without the pidfd.
func OpenRoot(pid int, containerID string) (*os.File, error) {
	root, pidfd, err := openRoot(pid, containerID, nil)
	if err != nil {
		return nil, err
	}
	unix.Close(pidfd)
	return root, nil
}

func openProcFS(docker dockerx.Client, info dockerx.ContainerInfo, read func(int) ([]byte, error)) (FS, error) {
	root, pidfd, err := openRoot(info.Pid, info.ID, read)
	if err != nil {
		return nil, err
	}
	return &procFS{root: root, pid: info.Pid, pidfd: pidfd, docker: docker, mounts: info.Mounts}, nil
}
