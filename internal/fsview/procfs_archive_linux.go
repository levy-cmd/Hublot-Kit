//go:build linux

package fsview

import (
	"context"
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// readMountinfo reads a host file: /proc/<pid>/mountinfo of the process pinned
// by the view's pidfd (its mount points are relative to the container root).
// Tests replace it.
var readMountinfo = func(pid int) ([]byte, error) {
	return os.ReadFile(fmt.Sprintf("/proc/%d/mountinfo", pid))
}

// CheckArchive (spec §5.2 step 7, §5.4), in this order: the container's Docker
// mounts (HostBind); its mount table, read for the very process the pidfd pins
// and still alive afterwards; no dangerous mount under any Docker mount (the
// whole container); none on, under or above dir.
func (p *procFS) CheckArchive(ctx context.Context, dir string) error {
	if p.pidfd < 0 {
		return errors.New("contrôle des montages impossible : vue sans process")
	}
	if err := HostBind(ctx, p.docker, p.mounts); err != nil {
		return err
	}
	mi, err := readMountinfo(p.pid)
	if err != nil {
		return changed("lecture de /proc/%d/mountinfo : %v", p.pid, err)
	}
	if err := unix.PidfdSendSignal(p.pidfd, 0, nil, 0); err != nil {
		return changed("le process %d a disparu : %v", p.pid, err)
	}
	if err := CheckVisibleMounts(string(mi), p.mounts); err != nil {
		return err
	}
	if mp, typ, dst, bad := PseudoMountInMounts(string(mi), p.mounts); bad {
		return fmt.Errorf("%w : %s monté sur %s, sous le montage %s du conteneur : aucun tar", ErrForbiddenPath, typ, mp, dst)
	}
	if mp, typ, bad := PseudoMountUnder(string(mi), dir); bad {
		return fmt.Errorf("%w : %s monté sur %s, le tar de %s l'inclurait", ErrForbiddenPath, typ, mp, CleanPath(dir))
	}
	return nil
}
