//go:build linux

package fsview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"time"

	pathrs "github.com/cyphar/filepath-securejoin/pathrs-lite"
	"golang.org/x/sys/unix"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// procFS reads a running container through a descriptor on its root
// (spec §5.1). Every path goes through pathrs.OpenatInRoot or openat2
// RESOLVE_IN_ROOT: never filepath.Join, never os.Open on a container path.
type procFS struct {
	root *os.File
	// Set by openProcFS for a real container (CheckArchive needs them);
	// pidfd is -1 for a view built by NewProcFS.
	pid    int
	pidfd  int
	docker dockerx.Client
	mounts []dockerx.Mount
	tmpfs  []string // --tmpfs destinations of Inspect (not in mounts)
}

// NewProcFS serves the tree under root, a descriptor on a root directory
// (O_PATH is enough), without any process behind it: tests use it with a
// temporary directory. It takes ownership of root: Close closes it.
func NewProcFS(root *os.File) FS { return &procFS{root: root, pidfd: -1} }

func (p *procFS) Close() error {
	if p.pidfd >= 0 {
		unix.Close(p.pidfd)
		p.pidfd = -1
	}
	return p.root.Close()
}

// resolve opens name inside the root, links followed without ever leaving it,
// as an O_PATH handle, then refuses pseudo filesystems before any Reopen
// (spec §5.1 steps 6 and 9). It also returns the handle's fstat.
func (p *procFS) resolve(name string) (*os.File, *unix.Stat_t, error) {
	h, err := pathrs.OpenatInRoot(p.root, name)
	if err != nil {
		return nil, nil, err
	}
	if err := checkNotPseudo(int(h.Fd()), name); err != nil {
		h.Close()
		return nil, nil, err
	}
	var st unix.Stat_t
	if err := unix.Fstat(int(h.Fd()), &st); err != nil {
		h.Close()
		return nil, nil, err
	}
	return h, &st, nil
}

func (p *procFS) List(_ context.Context, dir string) ([]Entry, bool, error) {
	h, st, err := p.resolve(dir)
	if err != nil {
		return nil, false, err
	}
	defer h.Close()
	if st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return nil, false, fmt.Errorf("%w: %s", ErrNotDir, dir)
	}
	d, err := pathrs.Reopen(h, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC)
	if err != nil {
		return nil, false, err
	}
	defer d.Close()
	names, err := d.Readdirnames(MaxEntries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, err
	}
	truncated := len(names) > MaxEntries
	if truncated {
		names = names[:MaxEntries]
	}
	// One path component at a time, relative to the directory descriptor,
	// without following anything (spec §5.1 step 7): nothing can leave the root.
	dfd := int(d.Fd())
	out := make([]Entry, 0, len(names))
	for _, n := range names {
		var st unix.Stat_t
		if err := unix.Fstatat(dfd, n, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			continue // removed while listing
		}
		e := entryFromStat(n, &st)
		if e.Type == TypeSymlink {
			buf := make([]byte, 4096)
			if k, err := unix.Readlinkat(dfd, n, buf); err == nil {
				e.LinkTarget = string(buf[:k])
			}
		}
		out = append(out, e)
	}
	return out, truncated, nil
}

// Stat works on the O_PATH handle only: it never reopens (spec §5.1 step 6).
func (p *procFS) Stat(_ context.Context, name string) (Entry, error) {
	h, st, err := p.resolve(name)
	if err != nil {
		return Entry{}, err
	}
	h.Close()
	return entryFromStat(path.Base(CleanPath(name)), st), nil
}

func (p *procFS) Open(_ context.Context, name string) (io.ReadCloser, error) {
	h, st, err := p.resolve(name)
	if err != nil {
		return nil, err
	}
	defer h.Close()
	// Regular files only, checked before Reopen: a device node created by
	// mknod in the container would read the host device (spec §5.1 step 6).
	if st.Mode&unix.S_IFMT != unix.S_IFREG {
		return nil, fmt.Errorf("%w: %s", ErrNotRegular, name)
	}
	// O_NONBLOCK: defence in depth; Reopen reopens the very inode checked above, so it is a regular file.
	f, err := pathrs.Reopen(h, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC)
	if err != nil {
		return nil, err // never a typed nil *os.File inside the interface
	}
	return f, nil
}

// openNoLinks opens the clean path with O_PATH, no symbolic link followed
// anywhere (ErrLinkInPath), never out of the root. The caller closes fd.
func (p *procFS) openNoLinks(clean string) (int, error) {
	fd, err := unix.Openat2(int(p.root.Fd()), clean, &unix.OpenHow{
		Flags:   unix.O_PATH | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_IN_ROOT | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if errors.Is(err, unix.ELOOP) {
		return -1, fmt.Errorf("%w: %s", ErrLinkInPath, clean)
	}
	if err != nil {
		return -1, &fs.PathError{Op: "openat2", Path: clean, Err: err}
	}
	return fd, nil
}

func (p *procFS) StatNoLinks(_ context.Context, name string) (Entry, error) {
	clean := CleanPath(name)
	fd, err := p.openNoLinks(clean)
	if err != nil {
		return Entry{}, err
	}
	defer unix.Close(fd)
	if err := checkNotPseudo(fd, clean); err != nil {
		return Entry{}, err
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return Entry{}, err
	}
	return entryFromStat(path.Base(clean), &st), nil
}

func entryFromStat(name string, st *unix.Stat_t) Entry {
	e := Entry{
		Name:    name,
		Size:    st.Size,
		Mode:    ModeFromUnix(st.Mode),
		UID:     int(st.Uid),
		GID:     int(st.Gid),
		ModTime: time.Unix(int64(st.Mtim.Sec), int64(st.Mtim.Nsec)).UTC(),
	}
	switch st.Mode & unix.S_IFMT {
	case unix.S_IFREG:
		e.Type = TypeFile
	case unix.S_IFDIR:
		e.Type = TypeDir
	case unix.S_IFLNK:
		e.Type = TypeSymlink
	default:
		e.Type = TypeOther
	}
	return e
}
