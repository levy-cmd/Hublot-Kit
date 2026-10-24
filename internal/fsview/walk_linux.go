//go:build linux

package fsview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"sync/atomic"
	"time"

	pathrs "github.com/cyphar/filepath-securejoin/pathrs-lite"
	"golang.org/x/sys/unix"
)

// Test seams: walkBeforeOpen runs before the open of every path (a test
// plants a link there); walkReadTimeout bounds each read of a file.
var (
	walkBeforeOpen  = func(string) {}
	walkReadTimeout = 10 * time.Second
)

// maxStuckReads: past that many reads given up (a FUSE server that never
// answers, spec §13), the walk ends: each one keeps a goroutine.
const maxStuckReads = 4

var errReadTimeout = errors.New("read of the file timed out (stuck FUSE mount?)")

// Walk is the snapshot walk of spec v0.2 §5.7, on the root pinned by §5.1.
// Every path, directory or file, is opened again from the root by
// openNoLinks (openat2, RESOLVE_IN_ROOT|RESOLVE_NO_SYMLINKS, O_PATH): never
// OpenatInRoot, which would follow a link planted during the walk. Then, on
// that descriptor: fstatfs (pseudo filesystem: inaccessible), the mount ID
// (another than the root's: mount), fstat (anything but a directory or a
// regular file: other); a directory is read by Reopen, its names stated
// with fstatat(AT_SYMLINK_NOFOLLOW) and readlinkat as List does; a regular
// file is reopened only when fn calls open. Links are never followed.
func (p *procFS) Walk(ctx context.Context, fn func(string, Entry, func() (io.ReadCloser, error)) error) error {
	st, err := statxMountID(int(p.root.Fd()), 0, 0)
	if err != nil {
		return fmt.Errorf("mount ID of the root: %w", err) // no STATX_MNT_ID: mounts cannot be told, fail closed
	}
	w := &walk{p: p, ctx: ctx, fn: fn, rootMnt: st.Mnt_id}
	err = w.dir("/", p.root)
	if errors.Is(err, fs.SkipAll) {
		err = nil
	}
	if err == nil && p.pidfd >= 0 {
		err = alive(p.pidfd, p.pid) // the process (so the root) ended during the walk
	}
	if err == nil && w.truncated {
		err = ErrTruncated
	}
	return err
}

type walk struct {
	p         *procFS
	ctx       context.Context
	fn        func(string, Entry, func() (io.ReadCloser, error)) error
	rootMnt   uint64
	truncated bool
	stuck     atomic.Int32
}

// dir walks the children of dir, whose handle h was checked by the caller.
// Its names are read and stated first, then the directory is closed: one
// directory open per level at most.
func (w *walk) dir(dir string, h *os.File) error {
	children, ok := w.list(h)
	if !ok {
		w.truncated = true // unreadable directory: its children are missing
	}
	for _, e := range children {
		if err := w.ctx.Err(); err != nil {
			return err
		}
		if w.stuck.Load() >= maxStuckReads {
			return errors.New("snapshot walk: too many reads stuck (FUSE?)")
		}
		p := childPath(dir, e.Name)
		if e.Type == TypeSymlink {
			if err := w.call(w.fn(p, e, nil)); err != nil {
				return err
			}
			continue
		}
		if err := w.child(p, e); err != nil {
			return err
		}
	}
	return nil
}

// call maps what fn returned: fs.SkipDir is only meaningful for a directory.
func (w *walk) call(err error) error {
	if errors.Is(err, fs.SkipDir) {
		return nil
	}
	return err
}

// list returns the entries of the directory h, sorted by name, at most
// MaxEntries of them; ok is false when they are not all there.
func (w *walk) list(h *os.File) ([]Entry, bool) {
	d, err := pathrs.Reopen(h, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC)
	if err != nil {
		return nil, false
	}
	defer d.Close()
	names, err := d.Readdirnames(MaxEntries + 1)
	ok := err == nil || errors.Is(err, io.EOF)
	if len(names) > MaxEntries {
		names, ok = names[:MaxEntries], false
	}
	sort.Strings(names)
	dfd := int(d.Fd())
	out := make([]Entry, 0, len(names))
	for _, n := range names {
		var st unix.Stat_t
		if err := unix.Fstatat(dfd, n, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			continue // removed since
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
	return out, ok
}

// child opens p again from the root, without any link, and visits it.
// listed is its entry from the listing of its parent.
func (w *walk) child(p string, listed Entry) error {
	walkBeforeOpen(p)
	fd, err := w.p.openNoLinks(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil // removed since the listing
	}
	if err != nil {
		// A link in the chain since the listing (ErrLinkInPath), or any other
		// failure: nothing is followed, nothing is read.
		e := listed
		var open func() (io.ReadCloser, error)
		switch e.Type {
		case TypeDir:
			e.Type = TypeInaccessible
		case TypeFile:
			open = func() (io.ReadCloser, error) { return nil, err }
		}
		return w.call(w.fn(p, e, open))
	}
	h := os.NewFile(uintptr(fd), p)
	defer h.Close()
	return w.visit(p, h, listed.Name)
}

func (w *walk) visit(p string, h *os.File, name string) error {
	fd := int(h.Fd())
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return w.call(w.fn(p, Entry{Name: name, Type: TypeInaccessible}, nil))
	}
	e := entryFromStat(name, &st)
	// Spec §5.1 step 9 first: a pseudo filesystem is never read.
	if err := checkNotPseudo(fd, p); err != nil {
		e.Type, e.Size = TypeInaccessible, 0
		return w.call(w.fn(p, e, nil))
	}
	stx, err := statxMountID(fd, 0, 0)
	if err != nil || stx.Mnt_id != w.rootMnt {
		e.Type, e.Size = TypeMount, 0 // a mount point (or unknown): never walked nor read (Q58)
		return w.call(w.fn(p, e, nil))
	}
	switch e.Type {
	case TypeDir:
		err := w.fn(p, e, nil)
		if errors.Is(err, fs.SkipDir) {
			return nil
		}
		if err != nil {
			return err
		}
		return w.dir(p, h)
	case TypeFile:
		return w.call(w.fn(p, e, func() (io.ReadCloser, error) { return w.open(h) }))
	}
	return w.call(w.fn(p, e, nil)) // device, FIFO, socket: never opened
}

// open reopens the regular file h for reading. O_NOATIME: a snapshot does
// not change the access times of the production disk; the kernel refuses
// it (EPERM) to a non-owner without CAP_FOWNER, then the read goes without.
func (w *walk) open(h *os.File) (io.ReadCloser, error) {
	const flags = unix.O_RDONLY | unix.O_NONBLOCK | unix.O_CLOEXEC
	f, err := pathrs.Reopen(h, flags|unix.O_NOATIME)
	if errors.Is(err, unix.EPERM) {
		f, err = pathrs.Reopen(h, flags)
	}
	if err != nil {
		return nil, err
	}
	return &timedFile{f: f, w: w, buf: make([]byte, 256<<10)}, nil
}

// timedFile gives up a read after walkReadTimeout. The read goes on in its
// goroutine, into the file's own buffer (never the caller's), and the file
// stays open until it returns (os.File closes the descriptor after the last
// operation).
type timedFile struct {
	f    *os.File
	w    *walk
	buf  []byte
	dead bool
}

type readResult struct {
	n   int
	err error
}

func (t *timedFile) Read(b []byte) (int, error) {
	if t.dead {
		return 0, errReadTimeout
	}
	if err := t.w.ctx.Err(); err != nil {
		return 0, err
	}
	buf := t.buf[:min(len(b), len(t.buf))]
	done := make(chan readResult, 1)
	go func() {
		n, err := t.f.Read(buf)
		done <- readResult{n, err}
	}()
	timer := time.NewTimer(walkReadTimeout)
	defer timer.Stop()
	select {
	case r := <-done:
		return copy(b, buf[:r.n]), r.err
	case <-timer.C:
	case <-t.w.ctx.Done():
	}
	t.dead = true
	t.w.stuck.Add(1)
	go func() { <-done; t.w.stuck.Add(-1) }()
	return 0, errReadTimeout
}

func (t *timedFile) Close() error { return t.f.Close() }
