package fsview

// Index of the tar headers of a stopped container (spec §5.2): built once
// from GET /archive of "/", then every lookup is answered from memory.

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

type IndexLimits struct {
	MaxBytes    int64         // bytes read from GET /archive of "/"
	MaxDuration time.Duration // the whole build, GetArchive included
	MaxHeaders  int           // tar entries: bounds the agent's memory
	// MaxNameBytes caps the bytes of every name and link target read, which
	// the index keeps (v0.1.2, audit: 4 KiB paths and targets held ~5.4 GiB
	// at the byte cap). Zero or negative: DefaultIndexLimits'.
	MaxNameBytes int64
}

var DefaultIndexLimits = IndexLimits{MaxBytes: 4 << 30, MaxDuration: 5 * time.Minute, MaxHeaders: 1_000_000, MaxNameBytes: 256 << 20}

type index struct {
	entries   map[string]Entry    // clean container path → entry, "/" included
	children  map[string][]string // directory → names, in archive order
	hardlinks map[string]string   // hard link → its target, a regular file
}

type tarFS struct {
	docker dockerx.Client
	id     string
	mounts []dockerx.Mount
	tmpfs  []string // --tmpfs destinations of Inspect (not in mounts)
	idx    *index
	stale  func()
}

func newTarFS(docker dockerx.Client, info dockerx.ContainerInfo, idx *index, stale func()) *tarFS {
	return &tarFS{docker: docker, id: info.ID, mounts: info.Mounts, tmpfs: info.Tmpfs, idx: idx, stale: stale}
}

var _ FS = (*tarFS)(nil)

// maxHops: one single counter for a whole resolution (spec §5.2 step 5).
const maxHops = 40

var errTooManyBytes = errors.New("byte cap reached")

// errTooSlow marks a too_large of the time cap: it expires (Q11), the
// bytes and entries caps stay.
var errTooSlow = errors.New("time cap reached")

// countingReader counts the bytes of GET /archive (progress, MaxBytes) and
// stops at the end of ctx even if the reader below ignores it.
type countingReader struct {
	ctx context.Context
	r   io.Reader
	n   *atomic.Int64
	max int64
}

func (c *countingReader) Read(b []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := c.r.Read(b)
	if c.n.Add(int64(n)) > c.max {
		return n, errTooManyBytes
	}
	return n, err
}

// buildIndex reads the tar of "/" to io.EOF and keeps the headers only
// (spec §5.2 step 3). Any other error fails the whole build: never a
// partial index.
func buildIndex(ctx context.Context, r io.Reader, lim IndexLimits, read *atomic.Int64) (*index, error) {
	x := &index{
		entries:   map[string]Entry{"/": {Name: "/", Type: TypeDir, Mode: 0o755}},
		children:  map[string][]string{},
		hardlinks: map[string]string{},
	}
	if lim.MaxNameBytes <= 0 {
		lim.MaxNameBytes = DefaultIndexLimits.MaxNameBytes
	}
	tr := tar.NewReader(&countingReader{ctx: ctx, r: r, n: read, max: lim.MaxBytes})
	var names int64 // bytes of names and link targets read
	for n := 0; ; n++ {
		h, err := tr.Next()
		if err == io.EOF {
			return x, nil
		}
		switch {
		case errors.Is(err, errTooManyBytes):
			return nil, fmt.Errorf("%w: index beyond %d bytes read", ErrTooLarge, lim.MaxBytes)
		case err != nil && (errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded)):
			return nil, fmt.Errorf("%w: index beyond %s (%w)", ErrTooLarge, lim.MaxDuration, errTooSlow)
		case err != nil:
			return nil, fmt.Errorf("reading the archive of /: %w", err)
		case n >= lim.MaxHeaders:
			return nil, fmt.Errorf("%w: index beyond %d entries", ErrTooLarge, lim.MaxHeaders)
		}
		if names += int64(len(h.Name) + len(h.Linkname)); names > lim.MaxNameBytes {
			return nil, fmt.Errorf("%w: index beyond %d bytes of names and link targets", ErrTooLarge, lim.MaxNameBytes)
		}
		x.add(h)
	}
}

// add records one header. GET /archive of "/" spells names "/bin/"; "./bin"
// and "bin" are accepted too.
func (x *index) add(h *tar.Header) {
	p := CleanPath(h.Name)
	if strings.HasPrefix(p, "/proc/") || strings.HasPrefix(p, "/sys/") {
		return // spec §5.2 step 7: never indexed, so never opened
	}
	e := Entry{Name: path.Base(p), Size: h.Size, Mode: ModeFromUnix(uint32(h.Mode)), UID: h.Uid, GID: h.Gid, ModTime: h.ModTime}
	hardTarget := ""
	switch h.Typeflag {
	case tar.TypeReg, tar.TypeCont, tar.TypeGNUSparse:
		e.Type = TypeFile
	case tar.TypeDir:
		e.Type, e.Size = TypeDir, 0
	case tar.TypeSymlink:
		e.Type, e.Size, e.LinkTarget = TypeSymlink, 0, h.Linkname
	case tar.TypeLink: // spec §5.2 step 6: a file with the size and mode of its target
		target := CleanPath(h.Linkname)
		if t, ok := x.entries[target]; ok && t.Type == TypeFile && target != p {
			if first, ok := x.hardlinks[target]; ok {
				target = first
			}
			e.Type, e.Size, e.Mode = TypeFile, t.Size, t.Mode
			hardTarget = target
		} else {
			e.Type, e.Size = TypeOther, 0 // unknown target: Open refuses it
		}
	default: // device, FIFO…
		e.Type, e.Size = TypeOther, 0
	}
	if p == "/" {
		if e.Type == TypeDir {
			e.Name = "/"
			x.entries["/"] = e
		}
		return
	}
	parent := path.Dir(p)
	if !x.ensureDir(parent) {
		return // under a non-directory: no resolution reaches it
	}
	if _, seen := x.entries[p]; !seen {
		x.children[parent] = append(x.children[parent], e.Name)
	}
	delete(x.hardlinks, p)
	if hardTarget != "" {
		x.hardlinks[p] = hardTarget
	}
	x.entries[p] = e
}

// ensureDir makes d a directory of the index, creating the parents the
// archive did not declare. false: something on the way is not a directory.
func (x *index) ensureDir(d string) bool {
	if e, ok := x.entries[d]; ok {
		return e.Type == TypeDir
	}
	if !x.ensureDir(path.Dir(d)) {
		return false
	}
	x.entries[d] = Entry{Name: path.Base(d), Type: TypeDir, Mode: 0o755}
	x.children[path.Dir(d)] = append(x.children[path.Dir(d)], path.Base(d))
	return true
}

func progress(read, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return min(0.99, float64(read)/float64(total)) // volumes are read but not in SizeRootFs
}

// resolve walks p component by component in the index (spec §5.2 step 5)
// and returns the path it designates, every link resolved: the only kind of
// path that ever goes to GetArchive. cur never contains a link, so ".." is
// the parent of the resolved target, and stays at "/" on the root.
func (x *index) resolve(p string) (string, error) {
	cur, rest, err := x.walk(p)
	if err != nil {
		return "", err
	}
	if rest != nil {
		return "", notExist(p)
	}
	return cur, nil
}

// walk is resolve that stops at the first missing component: it returns
// the resolved path so far and the components left (nil when p exists).
func (x *index) walk(p string) (cur string, rest []string, err error) {
	todo := components(p)
	mustDir := strings.HasSuffix(p, "/")
	cur, hops := "/", 0
	for len(todo) > 0 {
		c := todo[0]
		todo = todo[1:]
		if c == ".." {
			cur = path.Dir(cur)
			continue
		}
		next := childPath(cur, c)
		e, ok := x.entries[next]
		if !ok {
			return cur, append([]string{c}, todo...), nil
		}
		if e.Type == TypeSymlink {
			if hops++; hops > maxHops {
				return "", nil, &fs.PathError{Op: "resolve", Path: p, Err: syscall.ELOOP}
			}
			if e.LinkTarget == "" {
				return "", nil, notExist(p)
			}
			if strings.HasPrefix(e.LinkTarget, "/") {
				cur = "/" // absolute: from the container root, never the host's
			} // relative: from cur, the link's directory
			if strings.HasSuffix(e.LinkTarget, "/") && len(todo) == 0 {
				mustDir = true
			}
			todo = append(components(e.LinkTarget), todo...)
			continue
		}
		if len(todo) > 0 && e.Type != TypeDir {
			return "", nil, fmt.Errorf("%w: %s", ErrNotDir, p)
		}
		cur = next
	}
	if mustDir && x.entries[cur].Type != TypeDir {
		return "", nil, fmt.Errorf("%w: %s", ErrNotDir, p)
	}
	return cur, nil, nil
}

func components(p string) []string {
	var out []string
	for _, c := range strings.Split(p, "/") {
		if c != "" && c != "." {
			out = append(out, c)
		}
	}
	return out
}

func childPath(dir, name string) string {
	if dir == "/" {
		return "/" + name
	}
	return dir + "/" + name
}

func notExist(p string) error { return &fs.PathError{Op: "resolve", Path: p, Err: fs.ErrNotExist} }

// tarFS is the view of a stopped container (spec §5.2): every lookup in the
// index; only Open asks Docker, for the resolved path of a regular file.
func (t *tarFS) List(_ context.Context, dir string) ([]Entry, bool, error) {
	r, err := t.idx.resolve(dir)
	if err != nil {
		return nil, false, err
	}
	if t.idx.entries[r].Type != TypeDir {
		return nil, false, fmt.Errorf("%w: %s", ErrNotDir, dir)
	}
	names := t.idx.children[r]
	truncated := len(names) > MaxEntries
	if truncated {
		names = names[:MaxEntries]
	}
	out := make([]Entry, 0, len(names))
	for _, n := range names {
		out = append(out, t.idx.entries[childPath(r, n)])
	}
	return out, truncated, nil
}

// Stat follows links, like any resolved path; List shows the links themselves.
func (t *tarFS) Stat(_ context.Context, p string) (Entry, error) {
	r, err := t.idx.resolve(p)
	if err != nil {
		return Entry{}, err
	}
	return t.idx.entries[r], nil
}

// regularPath is the path Open sends to GetArchive for p: resolved, a
// regular file. For a hard link, its first occurrence, which must itself
// resolve to itself (review I3: a later entry may have turned one of its
// directories into a link, which Docker would follow).
func (t *tarFS) regularPath(p string) (string, error) {
	r, err := t.idx.resolve(p)
	if err != nil {
		return "", err
	}
	if t.idx.entries[r].Type != TypeFile {
		return "", fmt.Errorf("%w: %s", ErrNotRegular, p)
	}
	if first, ok := t.idx.hardlinks[r]; ok {
		_, chained := t.idx.hardlinks[first]
		if got, err := t.idx.resolve(first); err != nil || got != first || chained || t.idx.entries[first].Type != TypeFile {
			return "", fmt.Errorf("%w: %s (hard link to %s)", ErrNotRegular, p, first)
		}
		r = first
	}
	return r, nil
}

// CheckOpen: every check of Open, without asking Docker (&check=1, HEAD).
func (t *tarFS) CheckOpen(_ context.Context, p string) error {
	_, err := t.regularPath(p)
	return err
}

func (t *tarFS) Open(ctx context.Context, p string) (io.ReadCloser, error) {
	r, err := t.regularPath(p)
	if err != nil {
		return nil, err
	}
	rc, err := t.docker.GetArchive(ctx, t.id, r)
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(rc)
	h, err := tr.Next()
	if err != nil {
		rc.Close()
		return nil, fmt.Errorf("archive of %s: %w", r, err)
	}
	if h.Typeflag != tar.TypeReg || path.Base(CleanPath(h.Name)) != path.Base(r) {
		rc.Close()
		t.stale()
		return nil, changed("%s changed since indexing", r)
	}
	return &entryReader{tr: tr, rc: rc}, nil
}

// entryReader reads the single entry of a file's archive, then requires the
// end of the tar.
type entryReader struct {
	tr *tar.Reader
	rc io.ReadCloser
}

func (e *entryReader) Read(b []byte) (int, error) {
	n, err := e.tr.Read(b)
	if err == io.EOF {
		if _, next := e.tr.Next(); next != io.EOF {
			if next == nil {
				next = errors.New("archive of a file: more than one entry")
			}
			return n, next
		}
	}
	return n, err
}

func (e *entryReader) Close() error { return e.rc.Close() }

// StatNoLinks: CleanPath(p), with no symbolic link anywhere on it (spec §5.4).
func (t *tarFS) StatNoLinks(_ context.Context, p string) (Entry, error) {
	cur := "/"
	for _, c := range components(CleanPath(p)) {
		if t.idx.entries[cur].Type != TypeDir {
			return Entry{}, fmt.Errorf("%w: %s", ErrNotDir, cur)
		}
		cur = childPath(cur, c)
		e, ok := t.idx.entries[cur]
		if !ok {
			return Entry{}, notExist(p)
		}
		if e.Type == TypeSymlink {
			return Entry{}, fmt.Errorf("%w: %s", ErrLinkInPath, cur)
		}
	}
	return t.idx.entries[cur], nil
}

// CheckArchive replays the host-bind rule (spec §5.2 step 7): Views.For
// already refused such a container, this guards the view on its own.
func (t *tarFS) CheckArchive(ctx context.Context, _ string) error {
	return HostBind(ctx, t.docker, t.mounts)
}

// CheckWriteRoot (stopped container): no process, the root is the one
// Docker writes in. At start, runc mounts each tmpfs at its destination
// resolved in the root filesystem, links followed (spec §13: /var/run ->
// ../run puts it at /run): the index resolves it the same way, never out
// of the root, as far as the path exists; runc creates the rest there
// (review Minor 1: /dangling -> ../real, then x, gives /real/x). A path
// the index refuses (loop, not a directory) stays as declared. Limit
// (review M1): runc mounts volumes and binds first, and the index shows
// them as they were when it was built; a link planted there afterwards (by
// the host or another container, the cache key unchanged) is not followed
// (spec §13).
func (t *tarFS) CheckWriteRoot(context.Context) ([]string, error) {
	points := tmpfsPoints(withTmpfs(t.mounts, t.tmpfs))
	for i, d := range points {
		if cur, rest, err := t.idx.walk(d); err == nil {
			points[i] = CleanPath(cur + "/" + strings.Join(rest, "/"))
		}
	}
	return points, nil
}

func (t *tarFS) Close() error { return nil }
