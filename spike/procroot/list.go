//go:build linux

package procroot

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	pathrs "github.com/cyphar/filepath-securejoin/pathrs-lite"
	"golang.org/x/sys/unix"
)

type Entry struct {
	Name       string
	Type       string // file, dir, symlink, other
	Size       int64
	Mode       fs.FileMode
	LinkTarget string // raw, never resolved
}

// openIn resolves path inside root (symlinks included) and reopens it with flags.
func openIn(root *os.File, path string, flags int) (*os.File, error) {
	h, err := pathrs.OpenatInRoot(root, path)
	if err != nil {
		return nil, err
	}
	defer h.Close()
	return pathrs.Reopen(h, flags|unix.O_CLOEXEC)
}

// List returns at most max entries of dir. Entries are stat'ed relative to the
// directory's descriptor, one path component at a time: no symlink is followed.
func List(root *os.File, dir string, max int) ([]Entry, bool, error) {
	if max < 0 {
		return nil, false, errors.New("max must be >= 0")
	}
	d, err := openIn(root, dir, unix.O_RDONLY|unix.O_DIRECTORY)
	if err != nil {
		return nil, false, err
	}
	defer d.Close()

	names, err := d.Readdirnames(max + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, err
	}
	truncated := len(names) > max
	if truncated {
		names = names[:max]
	}

	dfd := int(d.Fd())
	out := make([]Entry, 0, len(names))
	for _, n := range names {
		var st unix.Stat_t
		if err := unix.Fstatat(dfd, n, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			continue // removed while listing
		}
		e := Entry{Name: n, Size: st.Size, Mode: fs.FileMode(st.Mode & 0o777)}
		switch st.Mode & unix.S_IFMT {
		case unix.S_IFDIR:
			e.Type = "dir"
		case unix.S_IFREG:
			e.Type = "file"
		case unix.S_IFLNK:
			e.Type = "symlink"
			buf := make([]byte, 4096)
			if k, err := unix.Readlinkat(dfd, n, buf); err == nil {
				e.LinkTarget = string(buf[:k])
			}
		default:
			e.Type = "other"
		}
		out = append(out, e)
	}
	return out, truncated, nil
}

// ReadFile reads at most 1 MiB of path, resolved inside root.
func ReadFile(root *os.File, path string) ([]byte, error) {
	h, err := pathrs.OpenatInRoot(root, path)
	if err != nil {
		return nil, err
	}
	defer h.Close()
	var st unix.Stat_t
	if err := unix.Fstat(int(h.Fd()), &st); err != nil {
		return nil, err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG {
		return nil, fmt.Errorf("%w: %s", ErrNotRegular, path)
	}
	// O_NONBLOCK: never block on a file that was swapped for a FIFO after the check.
	f, err := pathrs.Reopen(h, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, 1<<20))
}
