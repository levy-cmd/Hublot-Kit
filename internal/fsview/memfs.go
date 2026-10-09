package fsview

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"sync/atomic"
)

// MemFS is an in-memory FS for the tests of other packages (api). Keys are
// CleanPath forms; "/" is always a directory. Stat and Open do not follow
// links (tests that need real resolution use procfs); StatNoLinks refuses
// them like procfs. Build it with NewMemFS and the Add* methods.
type MemFS struct {
	Entries map[string]Entry
	Content map[string][]byte
	Err     error // when set, every call except Close returns it
	// ArchiveErr is CheckArchive's answer (nil: the tar is allowed).
	ArchiveErr error
	// OpenFile, when set, replaces the reader of every regular file (tests of
	// a reader that blocks, like a FUSE file that never answers).
	OpenFile func(p string) (io.ReadCloser, error)
	Closed   atomic.Int32 // number of Close calls
}

var _ FS = (*MemFS)(nil)

func NewMemFS() *MemFS {
	return &MemFS{Entries: map[string]Entry{}, Content: map[string][]byte{}}
}

func (m *MemFS) AddDir(p string) *MemFS {
	p = CleanPath(p)
	m.Entries[p] = Entry{Name: path.Base(p), Type: TypeDir, Mode: 0o755}
	return m
}

func (m *MemFS) AddFile(p string, content []byte) *MemFS {
	p = CleanPath(p)
	m.Entries[p] = Entry{Name: path.Base(p), Type: TypeFile, Mode: 0o644, Size: int64(len(content))}
	m.Content[p] = content
	return m
}

func (m *MemFS) AddLink(p, target string) *MemFS {
	p = CleanPath(p)
	m.Entries[p] = Entry{Name: path.Base(p), Type: TypeSymlink, Mode: 0o777, LinkTarget: target}
	return m
}

func (m *MemFS) get(p string) (Entry, error) {
	if m.Err != nil {
		return Entry{}, m.Err
	}
	p = CleanPath(p)
	if p == "/" {
		return Entry{Name: "/", Type: TypeDir, Mode: 0o755}, nil
	}
	if e, ok := m.Entries[p]; ok {
		return e, nil
	}
	return Entry{}, &fs.PathError{Op: "stat", Path: p, Err: fs.ErrNotExist}
}

func (m *MemFS) List(_ context.Context, dir string) ([]Entry, bool, error) {
	d, err := m.get(dir)
	if err != nil {
		return nil, false, err
	}
	if d.Type != TypeDir {
		return nil, false, fmt.Errorf("%w : %s", ErrNotDir, dir)
	}
	dir = CleanPath(dir)
	out := []Entry{}
	for p, e := range m.Entries {
		if path.Dir(p) == dir {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, false, nil
}

func (m *MemFS) Stat(_ context.Context, p string) (Entry, error) { return m.get(p) }

func (m *MemFS) Open(_ context.Context, p string) (io.ReadCloser, error) {
	e, err := m.get(p)
	if err != nil {
		return nil, err
	}
	if e.Type != TypeFile {
		return nil, fmt.Errorf("%w : %s", ErrNotRegular, p)
	}
	if m.OpenFile != nil {
		return m.OpenFile(CleanPath(p))
	}
	return io.NopCloser(bytes.NewReader(m.Content[CleanPath(p)])), nil
}

func (m *MemFS) StatNoLinks(_ context.Context, p string) (Entry, error) {
	p = CleanPath(p)
	for cur := p; cur != "/"; cur = path.Dir(cur) {
		if e, err := m.get(cur); err == nil && e.Type == TypeSymlink {
			return Entry{}, fmt.Errorf("%w : %s", ErrLinkInPath, p)
		}
	}
	return m.get(p)
}

func (m *MemFS) CheckArchive(_ context.Context, dir string) error {
	if _, err := m.get(dir); err != nil {
		return err
	}
	return m.ArchiveErr
}

func (m *MemFS) Close() error {
	m.Closed.Add(1)
	return nil
}
