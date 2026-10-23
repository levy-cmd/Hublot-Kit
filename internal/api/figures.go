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

// filesJSON: source "index" here; "snapshot" and "count" come with the
// snapshots (spec v0.2 §5.7, plan v0.2 tranche 8).
type filesJSON struct {
	Count  int    `json:"count"`
	Source string `json:"source"`
	At     string `json:"at"`
}

type sizeEntry struct {
	rw, rootfs *int64
	at         time.Time
}

// figureCache belongs to one server (routes_figures.go).
type figureCache struct {
	mu     sync.Mutex
	sizes  map[string]sizeEntry // full container ID
	images map[string]int64     // image ID
}

func newFigureCache() *figureCache {
	return &figureCache{sizes: map[string]sizeEntry{}, images: map[string]int64{}}
}

func (c *figureCache) size(ctx context.Context, d dockerx.Client, id string) (rw, rootfs *int64, err error) {
	now := figuresNow()
	c.mu.Lock()
	e, ok := c.sizes[id]
	c.mu.Unlock()
	if ok && now.Sub(e.at) < sizeTTL {
		return e.rw, e.rootfs, nil
	}
	rw, rootfs, err = d.ContainerSize(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.sizes) >= maxSizes {
		for k, old := range c.sizes {
			if now.Sub(old.at) >= sizeTTL {
				delete(c.sizes, k)
			}
		}
	}
	if len(c.sizes) < maxSizes {
		c.sizes[id] = sizeEntry{rw: rw, rootfs: rootfs, at: now}
	}
	return rw, rootfs, nil
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
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
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
		if n, at, ok := s.d.Views.CachedCount(info); ok {
			out.Files = &filesJSON{Count: n, Source: "index", At: at.UTC().Format(time.RFC3339Nano)}
		}
		writeJSON(w, http.StatusOK, out)
	}
}
