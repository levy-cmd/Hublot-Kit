// Package fsview reads the files of a container (spec §4, §5): one interface,
// procfs for a running container, tarindex for a stopped one (plan
// Explorateur 2). It never writes: package files is the only writer.
package fsview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"time"
)

type EntryType string

const (
	TypeFile    EntryType = "file"
	TypeDir     EntryType = "dir"
	TypeSymlink EntryType = "symlink"
	TypeOther   EntryType = "other" // device, FIFO, socket
)

type Entry struct {
	Name       string
	Type       EntryType
	Size       int64
	Mode       fs.FileMode // permission bits + fs.ModeSetuid/Setgid/Sticky, never type bits
	UID, GID   int
	ModTime    time.Time
	LinkTarget string // raw, never resolved while listing
}

// MaxEntries caps a listing (spec §5.1 step 7).
const MaxEntries = 10_000

// OpenChecker is implemented by a view that can run every check of Open
// without reading: a view that would have to ask Docker implements it (spec
// §5.4, &check=1).
type OpenChecker interface {
	CheckOpen(ctx context.Context, path string) error
}

// MountPointChecker is implemented by a view that can tell whether a path
// is a mount point of the running container (procfs: STATX_MNT_ID of the
// path and of its parent, no link followed). Package files refuses to edit
// one: Docker writes in its own view of the mounts (plan Explorateur 3, Q21).
type MountPointChecker interface {
	IsMountPoint(ctx context.Context, path string) (bool, error)
}

// WriteRootChecker is implemented by a view that can prove its root is the
// container's root (procfs: the mount table of the pinned process shows "/").
// With a chrooted PID 1 the view shows another tree than the one Docker
// writes in: package files refuses every write then (review I1, plan 3).
// It also returns where the container's tmpfs mounts are (Inspect.Mounts
// of type tmpfs and --tmpfs), in the view's coordinates: a destination
// under a link is found where it is really mounted (spec §13; procfs by
// mount ID, tarindex through the index). Docker writes in none of them.
type WriteRootChecker interface {
	CheckWriteRoot(ctx context.Context) (tmpfs []string, err error)
}

// FS is a read view of a container's tree. Paths are container paths;
// List, Stat and Open follow symbolic links, never out of the container root.
type FS interface {
	List(ctx context.Context, dir string) (entries []Entry, truncated bool, err error)
	Stat(ctx context.Context, path string) (Entry, error)
	// Open returns a regular file only (ErrNotRegular otherwise).
	Open(ctx context.Context, path string) (io.ReadCloser, error)
	// StatNoLinks stats CleanPath(path) and refuses a symbolic link anywhere
	// in it, last component included (ErrLinkInPath): spec §5.3 step 2, §5.4.
	StatNoLinks(ctx context.Context, path string) (Entry, error)
	// CheckArchive refuses (ErrForbiddenPath) the tar of directory dir when
	// Docker would read the host through it: a bind of the host's /, /proc,
	// /sys or /dev (spec §5.2 step 7), or a pseudo filesystem or FUSE mount on,
	// under or above dir (spec §5.4). Called before every GetArchive of a directory.
	CheckArchive(ctx context.Context, dir string) error
	// Close releases the view (procfs: the root and pidfd descriptors).
	// Lifecycle contract: Close must only be called once every in-flight call
	// on the view has returned. procfs passes the raw root fd number to the
	// kernel; closing early lets that number be reused by an unrelated open.
	Close() error
}

// Errors of the views; package api maps each one to a code of spec §6.
// A missing path is reported with an error that matches fs.ErrNotExist.
var (
	ErrNotDir           = errors.New("pas un dossier")
	ErrNotRegular       = errors.New("pas un fichier régulier")
	ErrForbiddenPath    = errors.New("chemin interdit")
	ErrLinkInPath       = errors.New("lien symbolique dans le chemin")
	ErrContainerChanged = errors.New("le conteneur a changé")
	ErrTooLarge         = errors.New("trop volumineux")
	ErrIndexing         = errors.New("indexation en cours")
)

// CleanPath returns the absolute, lexically cleaned form of a container path.
// It only manipulates the string: it never touches a file.
func CleanPath(p string) string { return path.Clean("/" + p) }

// Unix special bits, identical in st_mode and in tar headers.
const (
	unixSetuid = 0o4000
	unixSetgid = 0o2000
	unixSticky = 0o1000
)

// ModeFromUnix converts st_mode (or a tar header mode) to Entry.Mode (spec §4):
// permission bits pass as is, setuid, setgid and sticky are mapped explicitly
// (their values differ in fs.FileMode), file type bits are dropped.
func ModeFromUnix(m uint32) fs.FileMode {
	mode := fs.FileMode(m & 0o777)
	if m&unixSetuid != 0 {
		mode |= fs.ModeSetuid
	}
	if m&unixSetgid != 0 {
		mode |= fs.ModeSetgid
	}
	if m&unixSticky != 0 {
		mode |= fs.ModeSticky
	}
	return mode
}

// UnixPerm is the reverse of ModeFromUnix: the 0o7777 bits shown by ls and stat.
func UnixPerm(m fs.FileMode) uint32 {
	p := uint32(m.Perm())
	if m&fs.ModeSetuid != 0 {
		p |= unixSetuid
	}
	if m&fs.ModeSetgid != 0 {
		p |= unixSetgid
	}
	if m&fs.ModeSticky != 0 {
		p |= unixSticky
	}
	return p
}

// IndexingError: the index of a stopped container is being built (spec §5.2
// step 2, §6 202 indexing). Progress goes from 0 to 0.99.
type IndexingError struct{ Progress float64 }

func (e *IndexingError) Error() string {
	return fmt.Sprintf("%v (%d %%)", ErrIndexing, int(e.Progress*100))
}

func (e *IndexingError) Unwrap() error { return ErrIndexing }
