//go:build linux

package fsview

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"

	pathrs "github.com/cyphar/filepath-securejoin/pathrs-lite"
	"golang.org/x/sys/unix"
)

// readMountinfo reads a host file: /proc/<pid>/mountinfo of the process pinned
// by the view's pidfd (its mount points are relative to the container root).
// Tests replace it.
var readMountinfo = func(pid int) ([]byte, error) {
	return os.ReadFile(fmt.Sprintf("/proc/%d/mountinfo", pid))
}

// errNoMountID: statx did not return STATX_MNT_ID (kernel before 5.8).
var errNoMountID = errors.New("statx without STATX_MNT_ID (kernel older than 5.8)")

// statxMountID is statx on the descriptor itself, asking for the mount ID
// (and extra mask bits); errNoMountID when the kernel did not give it.
func statxMountID(fd, flags int, extra int) (unix.Statx_t, error) {
	var st unix.Statx_t
	if err := unix.Statx(fd, "", unix.AT_EMPTY_PATH|flags, unix.STATX_MNT_ID|extra, &st); err != nil {
		return st, err
	}
	if st.Mask&unix.STATX_MNT_ID == 0 {
		return st, errNoMountID
	}
	return st, nil
}

// mountIdent resolves name in the container root (pathrs: links followed,
// never out of it) and returns its mount ID, device and inode.
func (p *procFS) mountIdent(name string) (MountIdent, error) {
	h, err := pathrs.OpenatInRoot(p.root, name)
	if err != nil {
		return MountIdent{}, err
	}
	defer h.Close()
	st, err := statxMountID(int(h.Fd()), unix.AT_SYMLINK_NOFOLLOW, unix.STATX_INO)
	if err != nil {
		return MountIdent{}, err
	}
	return MountIdent{MountID: st.Mnt_id, Dev: unix.Mkdev(st.Dev_major, st.Dev_minor), Ino: st.Ino}, nil
}

// CheckWriteRoot proves the view shows the tree Docker writes in, from the
// mount table of the pinned process (review I1 and its re-review):
//   - a "/" mount exists (CheckRootMount), and it is the mount of the root
//     descriptor pinned when the view was opened (else the process changed
//     root after the open);
//   - every destination declared by Inspect, tmpfs included, appears as a
//     mount point (CheckVisibleMounts), verbatim or found by mount ID
//     (LocateMounts: a --tmpfs under a link, spec §13): a PID 1 chrooted
//     into a volume or a tmpfs of the container shows a "/" line, but its
//     destination is gone.
//
// It returns the mount points of the tmpfs mounts, as located.
// A jail the container mounts itself (CAP_SYS_ADMIN) is not seen: spec §13.
// A view without process (NewProcFS, tests only) has nothing to read; every
// real view comes from For with a pid.
func (p *procFS) CheckWriteRoot(context.Context) ([]string, error) {
	if p.pidfd < 0 {
		return nil, nil
	}
	raw, err := readMountinfo(p.pid)
	if err != nil {
		return nil, changed("reading /proc/%d/mountinfo: %v", p.pid, err)
	}
	if err := alive(p.pidfd, p.pid); err != nil {
		return nil, err
	}
	mi := string(raw)
	if err := CheckRootMount(mi); err != nil {
		return nil, err
	}
	st, err := statxMountID(int(p.root.Fd()), 0, 0)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(rootMountIDs(mi), st.Mnt_id) {
		return nil, fmt.Errorf("%w: the \"/\" mount of the process is not the one of the root pinned when the view was opened (chroot of the process): write refused", ErrForbiddenPath)
	}
	mounts := LocateMounts(mi, withTmpfs(p.mounts, p.tmpfs), p.mountIdent)
	if err := CheckVisibleMounts(mi, mounts); err != nil {
		return nil, fmt.Errorf("%w (write refused: the process may not be in the container's root, chroot of PID 1)", err)
	}
	return tmpfsPoints(mounts), nil
}

// CheckArchive (spec §5.2 step 7, §5.4). The checks that read nothing in the
// container come first; ident (pathrs + statx) may meet a FUSE mount there.
func (p *procFS) CheckArchive(ctx context.Context, dir string) error {
	if p.pidfd < 0 {
		return errors.New("mount check impossible: view without a process")
	}
	if err := HostBind(ctx, p.docker, p.mounts); err != nil {
		return err
	}
	raw, err := readMountinfo(p.pid)
	if err != nil {
		return changed("reading /proc/%d/mountinfo: %v", p.pid, err)
	}
	if err := alive(p.pidfd, p.pid); err != nil {
		return err
	}
	mi := string(raw)
	if mp, typ, dst, bad := PseudoMountInMounts(mi, p.mounts); bad {
		return fmt.Errorf("%w: %s mounted on %s, under the container mount %s: no tar", ErrForbiddenPath, typ, mp, dst)
	}
	if mp, typ, bad := PseudoMountUnder(mi, dir); bad {
		return fmt.Errorf("%w: %s mounted on %s, the tar of %s would include it", ErrForbiddenPath, typ, mp, CleanPath(dir))
	}
	mounts := LocateMounts(mi, p.mounts, p.mountIdent)
	if err := CheckVisibleMounts(mi, mounts); err != nil {
		return err
	}
	if mp, typ, dst, bad := PseudoMountInMounts(mi, mounts); bad {
		return fmt.Errorf("%w: %s mounted on %s, under the container mount %s: no tar", ErrForbiddenPath, typ, mp, dst)
	}
	return nil
}
