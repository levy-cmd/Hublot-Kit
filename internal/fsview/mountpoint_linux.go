//go:build linux

package fsview

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"golang.org/x/sys/unix"
)

// IsMountPoint: the mount ID of CleanPath(name) differs from its parent's.
// Both are opened like StatNoLinks (openat2, no link anywhere). Without
// STATX_MNT_ID (kernel before 5.8) the answer is unknown: an error, so the
// edit is refused (fail closed).
func (p *procFS) IsMountPoint(_ context.Context, name string) (bool, error) {
	clean := CleanPath(name)
	if clean == "/" {
		return true, nil
	}
	id, err := p.mountIDNoLinks(clean)
	if err != nil {
		return false, err
	}
	parent, err := p.mountIDNoLinks(path.Dir(clean))
	if err != nil {
		return false, err
	}
	return id != parent, nil
}

func (p *procFS) mountIDNoLinks(clean string) (uint64, error) {
	fd, err := unix.Openat2(int(p.root.Fd()), clean, &unix.OpenHow{
		Flags:   unix.O_PATH | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_IN_ROOT | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if errors.Is(err, unix.ELOOP) {
		return 0, fmt.Errorf("%w : %s", ErrLinkInPath, clean)
	}
	if err != nil {
		return 0, &fs.PathError{Op: "openat2", Path: clean, Err: err}
	}
	defer unix.Close(fd)
	var st unix.Statx_t
	if err := unix.Statx(fd, "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &st); err != nil {
		return 0, &fs.PathError{Op: "statx", Path: clean, Err: err}
	}
	if st.Mask&unix.STATX_MNT_ID == 0 {
		return 0, errors.New("statx sans STATX_MNT_ID (noyau antérieur à 5.8) : point de montage indécidable")
	}
	return st.Mnt_id, nil
}
