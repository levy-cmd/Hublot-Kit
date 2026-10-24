package snapshot

import (
	"archive/tar"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

// Sink receives the entries of a capture, in the order of their
// components. A Sink that also has MarkPartial() is told when the capture is
// partial (hash budget reached, a directory not read entirely).
type Sink interface {
	Add(Entry) error
}

type partialMarker interface{ MarkPartial() }

func markPartial(out Sink) {
	if m, ok := out.(partialMarker); ok {
		m.MarkPartial()
	}
}

// Collector is the in-memory Sink (the archive, the comparison against the
// current container).
type Collector struct {
	Entries []Entry
	Partial bool
}

func (c *Collector) Add(e Entry) error {
	c.Entries = append(c.Entries, e)
	return nil
}

func (c *Collector) MarkPartial() { c.Partial = true }

var errTooManyEntries = errors.New("too many entries")

func tooManyEntries(lim Limits) error {
	return fmt.Errorf("%w: snapshot beyond %d entries (a partial manifest would show files as deleted)", fsview.ErrTooLarge, lim.MaxEntries)
}

// entryOf converts an entry of fsview. Only a regular file keeps a size:
// the size of a directory or a link depends on the filesystem (procfs) or
// is 0 (archive).
func entryOf(p string, e fsview.Entry) Entry {
	out := Entry{Path: p, Type: string(e.Type), Mode: fsview.UnixPerm(e.Mode), UID: e.UID, GID: e.GID,
		MTime: e.ModTime.UTC(), Link: e.LinkTarget}
	if e.Type == fsview.TypeFile {
		out.Size = e.Size
	}
	return out
}

// hasher computes the MACs under the caps of lim. key nil: no MAC at all
// (the count), nothing read.
type hasher struct {
	key     []byte
	lim     Limits
	hashed  int64
	partial bool
}

// skip is why a file of that size is not hashed, before reading it.
func (h *hasher) skip(size int64) string {
	switch {
	case size > h.lim.MaxFile:
		return SkipTooLarge
	case h.hashed+size > h.lim.MaxHashed:
		h.partial = true
		return SkipBudget
	}
	return ""
}

// sum hashes r into e: at most what the caps leave, plus one byte to see
// that the file goes past them (a file that grew since its stat).
func (h *hasher) sum(e *Entry, r io.Reader) {
	limit := min(h.lim.MaxFile, h.lim.MaxHashed-h.hashed)
	m := hmac.New(sha256.New, h.key)
	n, err := io.Copy(m, io.LimitReader(r, limit+1))
	h.hashed += min(n, limit)
	switch {
	case err != nil:
		e.MACSkipped = SkipUnreadable
	case n > limit && limit == h.lim.MaxFile:
		e.MACSkipped = SkipTooLarge
	case n > limit:
		e.MACSkipped, h.partial = SkipBudget, true
	default:
		e.MAC, e.Size = hex.EncodeToString(m.Sum(nil)), n
	}
}

func (h *hasher) file(e *Entry, open func() (io.ReadCloser, error)) {
	if why := h.skip(e.Size); why != "" {
		e.MACSkipped = why
		return
	}
	if open == nil {
		e.MACSkipped = SkipUnreadable
		return
	}
	rc, err := open()
	if err != nil {
		e.MACSkipped = SkipUnreadable // a link appeared in the path, a read refused…
		return
	}
	defer rc.Close()
	h.sum(e, rc)
}

// CaptureProcfs walks a running container (spec v0.2 §5.7) and writes each
// entry to out as it comes, the MAC of each regular file with it (key nil:
// none, nothing read). Caps of lim: MaxEntries (too_large), MaxHashed
// (budget, partial), MaxFile (too_large for that file), MaxDuration
// (too_large).
func CaptureProcfs(ctx context.Context, w fsview.Walker, key []byte, lim Limits, out Sink) error {
	ctx, cancel := context.WithTimeout(ctx, lim.MaxDuration)
	defer cancel()
	hs := &hasher{key: key, lim: lim}
	n := 0
	err := w.Walk(ctx, func(p string, fe fsview.Entry, open func() (io.ReadCloser, error)) error {
		if n++; n > lim.MaxEntries {
			return errTooManyEntries
		}
		e := entryOf(p, fe)
		if fe.Type == fsview.TypeFile && key != nil {
			hs.file(&e, open)
		}
		return out.Add(e)
	})
	if hs.partial {
		markPartial(out)
	}
	switch {
	case errors.Is(err, errTooManyEntries):
		return tooManyEntries(lim)
	case err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded):
		return fmt.Errorf("%w: snapshot beyond %s", fsview.ErrTooLarge, lim.MaxDuration)
	case errors.Is(err, fsview.ErrTruncated):
		markPartial(out)
		return nil
	}
	return err
}

var errTooManyBytes = errors.New("byte cap reached")

// countingReader stops at the byte cap of the archive and at the end of
// ctx, even when the reader below ignores it.
type countingReader struct {
	ctx context.Context
	r   io.Reader
	n   int64
	max int64
}

func (c *countingReader) Read(b []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := c.r.Read(b)
	if c.n += int64(n); c.n > c.max {
		return n, errTooManyBytes
	}
	return n, err
}

func isSparse(h *tar.Header) bool {
	if h.Typeflag == tar.TypeGNUSparse {
		return true
	}
	for k := range h.PAXRecords {
		if strings.HasPrefix(k, "GNU.sparse.") {
			return true
		}
	}
	return false
}

// under: p is dir or below it.
func under(p, dir string) bool {
	return p == dir || dir == "/" || strings.HasPrefix(p, dir+"/")
}

// pseudoDirs: never walked; noted as inaccessible, as procfs sees them.
var pseudoDirs = []string{"/proc", "/sys"}

// CaptureArchive reads GET /archive of "/" of a stopped container (spec v0.2
// §5.7): one entry per header; the content of a regular file goes into the
// MAC only if its declared size fits the caps, read through
// io.LimitReader(tr, size); a sparse entry is never hashed; everything else
// is skipped by Next(). Entries under /proc, /sys and under a mount
// destination are skipped without being noted, the destination itself is
// a mount entry. The entries are sorted in memory, then written to out;
// on any error nothing is written.
func CaptureArchive(ctx context.Context, r io.Reader, mounts []string, key []byte, lim Limits, out Sink) error {
	ctx, cancel := context.WithTimeout(ctx, lim.MaxArchiveDuration)
	defer cancel()
	ms := make([]string, len(mounts))
	for i, m := range mounts {
		ms[i] = fsview.CleanPath(m)
	}
	tr := tar.NewReader(&countingReader{ctx: ctx, r: r, max: lim.MaxArchiveBytes})
	hs := &hasher{key: key, lim: lim}
	var entries []Entry
	at := map[string]int{} // path → index in entries (duplicates, hard links)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if errors.Is(err, tar.ErrInsecurePath) && h != nil {
			err = nil // names are cleaned (CleanPath), never used on the disk
		}
		switch {
		case errors.Is(err, errTooManyBytes):
			return fmt.Errorf("%w: snapshot beyond %d bytes read from the archive", fsview.ErrTooLarge, lim.MaxArchiveBytes)
		case err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded):
			return fmt.Errorf("%w: snapshot beyond %s", fsview.ErrTooLarge, lim.MaxArchiveDuration)
		case err != nil:
			return fmt.Errorf("reading the archive of /: %w", err)
		}
		e, keep := archiveEntry(h, ms)
		if !keep {
			continue
		}
		if e.Type == "file" && key != nil {
			switch {
			case h.Typeflag == tar.TypeLink:
				if i, ok := at[fsview.CleanPath(h.Linkname)]; ok && entries[i].Type == "file" {
					e.Size, e.MAC, e.MACSkipped = entries[i].Size, entries[i].MAC, entries[i].MACSkipped
				} else {
					e.MACSkipped = SkipUnreadable
				}
			case isSparse(h):
				e.MACSkipped = SkipSparse
			default:
				if why := hs.skip(e.Size); why != "" {
					e.MACSkipped = why
				} else {
					hs.sum(&e, io.LimitReader(tr, h.Size))
				}
			}
		}
		if i, ok := at[e.Path]; ok {
			entries[i] = e // the last one wins
			continue
		}
		if len(entries) >= lim.MaxEntries {
			return tooManyEntries(lim)
		}
		at[e.Path] = len(entries)
		entries = append(entries, e)
	}
	slices.SortFunc(entries, func(a, b Entry) int { return ComparePaths(a.Path, b.Path) })
	for _, e := range entries {
		if err := out.Add(e); err != nil {
			return err
		}
	}
	if hs.partial {
		markPartial(out)
	}
	return nil
}

// archiveEntry converts a header; keep is false for what is not noted.
func archiveEntry(h *tar.Header, mounts []string) (Entry, bool) {
	p := fsview.CleanPath(h.Name)
	if p == "/" {
		return Entry{}, false
	}
	e := Entry{Path: p, Mode: uint32(h.Mode) & 0o7777, UID: h.Uid, GID: h.Gid, MTime: h.ModTime.UTC()}
	for _, d := range pseudoDirs {
		if under(p, d) {
			e.Type = "inaccessible"
			return e, p == d
		}
	}
	for _, d := range mounts {
		if under(p, d) {
			e.Type = "mount"
			return e, p == d
		}
	}
	switch h.Typeflag {
	case tar.TypeReg, tar.TypeLink, tar.TypeGNUSparse, tar.TypeCont:
		e.Type, e.Size = "file", h.Size
		if h.Typeflag == tar.TypeLink {
			e.Size = 0
		}
	case tar.TypeDir:
		e.Type = "dir"
	case tar.TypeSymlink:
		e.Type, e.Link = "symlink", h.Linkname
	default:
		e.Type = "other"
	}
	return e, true
}

// CaptureIndex takes the photo of the metadata from an index already built
// (spec v0.2 §5.7): no export, no lock, no MAC. /proc and /sys are
// inaccessible; MaskMounts marks the mounts.
func CaptureIndex(fsys fsview.FS, out Sink) error {
	n := 0
	err := indexDir(fsys, "/", out, &n)
	if errors.Is(err, errTooManyEntries) {
		return tooManyEntries(DefaultLimits)
	}
	return err
}

func indexDir(fsys fsview.FS, dir string, out Sink, n *int) error {
	es, truncated, err := fsys.List(context.Background(), dir)
	if err != nil {
		return err
	}
	if truncated {
		markPartial(out)
	}
	slices.SortFunc(es, func(a, b fsview.Entry) int { return strings.Compare(a.Name, b.Name) })
	for _, fe := range es {
		if *n++; *n > DefaultLimits.MaxEntries {
			return errTooManyEntries
		}
		p := fsview.CleanPath(dir + "/" + fe.Name)
		e := entryOf(p, fe)
		if slices.Contains(pseudoDirs, p) {
			e.Type, e.Size = "inaccessible", 0
		}
		if err := out.Add(e); err != nil {
			return err
		}
		if e.Type == "dir" {
			if err := indexDir(fsys, p, out, n); err != nil {
				return err
			}
		}
	}
	return nil
}

// MaskMounts shows each destination of mounts as a mount entry, never
// walked (Q58: volumes and binds are not the container's own files).
func MaskMounts(fsys fsview.FS, mounts []string) fsview.FS {
	ms := make([]string, len(mounts))
	for i, m := range mounts {
		ms[i] = fsview.CleanPath(m)
	}
	return masked{fsys, ms}
}

type masked struct {
	fsview.FS
	mounts []string
}

func (m masked) List(ctx context.Context, dir string) ([]fsview.Entry, bool, error) {
	es, truncated, err := m.FS.List(ctx, dir)
	for i, e := range es {
		if slices.Contains(m.mounts, fsview.CleanPath(dir+"/"+e.Name)) {
			es[i].Type, es[i].Size = fsview.TypeMount, 0
		}
	}
	return es, truncated, err
}
