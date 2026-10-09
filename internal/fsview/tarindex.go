package fsview

// Index of the tar headers of a stopped container (spec §5.2). Stub: the
// bodies arrive with task 3 of plan Explorateur 2; the tests come first.

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

var errNotYet = errors.New("tarindex : pas encore implémenté (tâche 3)")

type IndexLimits struct {
	MaxBytes    int64         // bytes read from GET /archive of "/"
	MaxDuration time.Duration // the whole build, GetArchive included
	MaxHeaders  int           // tar entries: bounds the agent's memory
}

var DefaultIndexLimits = IndexLimits{MaxBytes: 4 << 30, MaxDuration: 5 * time.Minute, MaxHeaders: 1_000_000}

type index struct {
	entries   map[string]Entry    // clean container path → entry, "/" included
	children  map[string][]string // directory → names, in archive order
	hardlinks map[string]string   // hard link → its target, a regular file
}

func buildIndex(ctx context.Context, r io.Reader, lim IndexLimits, read *atomic.Int64) (*index, error) {
	return nil, errNotYet
}

// progress is read/total, at most 0.99; 0 when total <= 0.
func progress(read, total int64) float64 { return -1 }

type tarFS struct {
	docker dockerx.Client
	id     string
	mounts []dockerx.Mount
	idx    *index
	stale  func()
}

func newTarFS(docker dockerx.Client, id string, mounts []dockerx.Mount, idx *index, stale func()) *tarFS {
	return &tarFS{docker: docker, id: id, mounts: mounts, idx: idx, stale: stale}
}

var _ FS = (*tarFS)(nil)

func (t *tarFS) List(ctx context.Context, dir string) ([]Entry, bool, error) {
	return nil, false, errNotYet
}
func (t *tarFS) Stat(ctx context.Context, p string) (Entry, error) { return Entry{}, errNotYet }
func (t *tarFS) Open(ctx context.Context, p string) (io.ReadCloser, error) {
	return nil, errNotYet
}
func (t *tarFS) StatNoLinks(ctx context.Context, p string) (Entry, error) {
	return Entry{}, errNotYet
}
func (t *tarFS) CheckArchive(ctx context.Context, dir string) error { return errNotYet }

// CheckOpen runs the checks of Open from the index only (review I2).
func (t *tarFS) CheckOpen(ctx context.Context, p string) error { return errNotYet }
func (t *tarFS) Close() error                                  { return nil }
