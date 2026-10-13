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
	fd, err := p.openNoLinks(clean)
	if err != nil {
		return 0, err
	}
	defer unix.Close(fd)
	st, err := statxMountID(fd, 0, 0)
	if errors.Is(err, errNoMountID) {
		return 0, fmt.Errorf("%w: cannot tell whether this is a mount point", err)
	}
	if err != nil {
		return 0, &fs.PathError{Op: "statx", Path: clean, Err: err}
	}
	return st.Mnt_id, nil
}
