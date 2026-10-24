package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// GET /api/containers/{id}/figures (spec v0.2 §5.3): the key figures of the
// detail, loaded apart from it. Each figure has its source and its cost:
//   - sizeRw, sizeRootFs: Inspect with size=true (dockerd walks the writable
//     layer: 11 to 16 ms measured, against 2 to 9 ms without), cached
//     sizeTTL per container; never asked for the list;
//   - imageSize: GET /images/{id}/json, cached by image ID (immutable);
//   - files: the index of a stopped container already served
//     (Views.CachedCount), never built for this route; null otherwise.
//
// The last exit is in GET /api/containers/{id} (state.finishedAt).

// figuresTimeout: dockerd holds the lock of a container during its export,
// and an Inspect of it waits (v0.1.1); past this, 504.
var figuresTimeout = 30 * time.Second

// figuresCallTimeout bounds the shared Inspect with size=true (sizeCall),
// which outlives the request that started it: the same 30 s, apart only so
// that a test can make either deadline end first.
var figuresCallTimeout = 30 * time.Second

// errSizeTimeout: the shared call reached figuresCallTimeout. Every request
// that waited on it answers 504, the one that started it as well as those
// that joined it, whatever their own deadline (review of tranche 3).
var errSizeTimeout = errors.New("size call timed out")

// figuresNow is the clock of the size cache; tests replace it.
var figuresNow = time.Now

const (
	sizeTTL = time.Minute
	// Bounds of the caches (memory): past them, expired sizes are dropped,
	// and the image sizes forgotten all at once.
	maxSizes      = 256
	maxImageSizes = 1024
)

type figuresJSON struct {
	SizeRw     *int64     `json:"sizeRw"`
	SizeRootFs *int64     `json:"sizeRootFs"`
	ImageSize  *int64     `json:"imageSize"`
	Files      *filesJSON `json:"files"`
}

// filesFigure is the "Files" figure, at no cost (spec v0.2 §5.3): the index
// of a stopped container already served, else the last snapshot of the
// container, with its date, else a count still kept (count.go), else nil
// (the interface offers the count).
func (s *server) filesFigure(info dockerx.ContainerInfo) *filesJSON {
	if n, at, ok := s.d.Views.CachedCount(info); ok {
		return &filesJSON{Count: n, Source: "index", At: at.UTC().Format(time.RFC3339Nano)}
	}
	if h, ok := s.latestSnapshot(info.ID); ok {
		return &filesJSON{Count: h.Entries, Source: "snapshot", At: h.CreatedAt.UTC().Format(time.RFC3339Nano)}
	}
	if r, ok := s.snap.countByID(info.ID); ok {
		return &filesJSON{Count: r.n, Source: "count", At: r.at.UTC().Format(time.RFC3339Nano)}
	}
	return nil
}

// filesJSON: source "index", "snapshot" or "count".
type filesJSON struct {
	Count  int    `json:"count"`
	Source string `json:"source"`
	At     string `json:"at"`
}

type sizeEntry struct {
	rw, rootfs *int64
	at         time.Time
}

// sizeCall is one Inspect with size=true in flight, shared by every request
// for that container meanwhile (review of tranche 2, Codex).
type sizeCall struct {
	done       chan struct{} // closed once rw, rootfs and err are set
	rw, rootfs *int64
	err        error
}

// figureCache belongs to one server (routes_figures.go).
type figureCache struct {
	mu       sync.Mutex
	sizes    map[string]sizeEntry // full container ID
	inflight map[string]*sizeCall // full container ID
	images   map[string]int64     // image ID
}

func newFigureCache() *figureCache {
	return &figureCache{sizes: map[string]sizeEntry{}, inflight: map[string]*sizeCall{}, images: map[string]int64{}}
}

// size answers from the cache, else joins the call in flight for id, else
// starts it. The call runs on its own, bounded by figuresCallTimeout: a request
// that goes away leaves it to the others, never fails them.
func (c *figureCache) size(ctx context.Context, d dockerx.Client, id string) (rw, rootfs *int64, err error) {
	now, timeout := figuresNow(), figuresCallTimeout
	c.mu.Lock()
	if e, ok := c.sizes[id]; ok && now.Sub(e.at) < sizeTTL {
		c.mu.Unlock()
		return e.rw, e.rootfs, nil
	}
	call, ok := c.inflight[id]
	if !ok {
		call = &sizeCall{done: make(chan struct{})}
		c.inflight[id] = call
		go c.fetch(d, id, call, now, timeout)
	}
	c.mu.Unlock()
	select {
	case <-call.done:
		return call.rw, call.rootfs, call.err
	case <-ctx.Done():
		return nil, nil, fmt.Errorf("%w: %v", dockerx.ErrUnavailable, ctx.Err())
	}
}

func (c *figureCache) fetch(d dockerx.Client, id string, call *sizeCall, now time.Time, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	rw, rootfs, err := d.ContainerSize(ctx, id)
	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		err = fmt.Errorf("%w: %w", errSizeTimeout, err)
	}
	c.mu.Lock()
	delete(c.inflight, id)
	if err == nil {
		c.storeLocked(id, sizeEntry{rw: rw, rootfs: rootfs, at: now})
	}
	call.rw, call.rootfs, call.err = rw, rootfs, err
	c.mu.Unlock()
	close(call.done)
}

func (c *figureCache) storeLocked(id string, e sizeEntry) {
	now := e.at
	if len(c.sizes) >= maxSizes {
		for k, old := range c.sizes {
			if now.Sub(old.at) >= sizeTTL {
				delete(c.sizes, k)
			}
		}
	}
	if len(c.sizes) < maxSizes {
		c.sizes[id] = e
	}
}

// image: nil for an image Docker no longer has (docker rmi -f of an image
// in use) or no image ID.
func (c *figureCache) image(ctx context.Context, d dockerx.Client, id string) (*int64, error) {
	if id == "" {
		return nil, nil
	}
	c.mu.Lock()
	n, ok := c.images[id]
	c.mu.Unlock()
	if ok {
		return &n, nil
	}
	img, err := d.ImageInspect(ctx, id)
	if errors.Is(err, dockerx.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.images) >= maxImageSizes {
		clear(c.images)
	}
	c.images[id] = img.Size
	return &img.Size, nil
}

func (s *server) figures(c *figureCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ref := r.PathValue("id")
		if !containerRef.MatchString(ref) {
			writeFSError(w, fmt.Errorf("%w: invalid container ID", dockerx.ErrNotFound))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), figuresTimeout)
		defer cancel()
		fail := func(err error) {
			if errors.Is(err, errSizeTimeout) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				writeError(w, http.StatusGatewayTimeout, "internal",
					fmt.Sprintf("Docker did not answer within %s (export of the container in progress?)", figuresTimeout))
				return
			}
			writeFSError(w, err)
		}
		info, err := s.d.Docker.Inspect(ctx, ref)
		if err != nil {
			fail(err)
			return
		}
		var out figuresJSON
		if out.SizeRw, out.SizeRootFs, err = c.size(ctx, s.d.Docker, info.ID); err != nil {
			fail(err)
			return
		}
		if out.ImageSize, err = c.image(ctx, s.d.Docker, info.Details.ImageID); err != nil {
			fail(err)
			return
		}
		out.Files = s.filesFigure(info)
		writeJSON(w, http.StatusOK, out)
	}
}
