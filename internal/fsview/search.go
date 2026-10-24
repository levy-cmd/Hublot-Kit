package fsview

// File search of the global search (spec v0.2 §5.8, Q61): only in the index
// of a stopped container already served to the explorer, never built for
// it, and only among the names the explorer shows.

import (
	"context"
	"slices"
	"strings"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// SearchHit is one path found, with the type List gives it (a link is a
// link, never followed).
type SearchHit struct {
	Path string
	Type EntryType
}

// searchCheckEvery: the walk looks at its context every that many entries.
const searchCheckEvery = 1024

// SearchCached looks for q (substring of the full path, case ignored) in
// the index Cached would serve for info, which the caller has just
// inspected: same key (full ID and FinishedAt), never a build, never a
// request to Docker, not a visit (LRU, IdleTTL). indexed is false when there
// is no such index (running container, no index served yet, build in
// progress or failed, older state). At most limit hits: the paths whose own
// name matches first, then those that match by a parent directory, each
// group sorted; truncated when more matched, or when ctx ended the walk.
func (v *Views) SearchCached(ctx context.Context, info dockerx.ContainerInfo, q string, limit int) (hits []SearchHit, truncated, indexed bool) {
	_, b := v.cached(info)
	if b == nil {
		return nil, false, false
	}
	hits, truncated = b.idx.search(ctx, q, max(limit, 1))
	return hits, truncated, true
}

// SearchImage looks for q in the index of image id (full ID) already served
// to the explorer, as SearchCached does for a container: never an export,
// never a request to Docker, not a visit. indexed is false when there is no
// such index (never built, in progress or failed).
func (v *Views) SearchImage(ctx context.Context, id string, q string, limit int) (hits []SearchHit, truncated, indexed bool) {
	v.mu.Lock()
	b, ok := v.builds[imageKey(id)]
	if !ok || !finished(b) || b.err != nil || b.img == nil || !b.served {
		v.mu.Unlock()
		return nil, false, false
	}
	v.mu.Unlock()
	hits, truncated = b.img.idx.search(ctx, q, max(limit, 1))
	return hits, truncated, true
}

// Indexing reports whether a build of ref (full ID or name, as Pending
// matches them: never a prefix) is in progress, without asking Docker and
// without making a visit of it: unlike Pending, neither its last use (Grace,
// IdleTTL) nor its place in the LRU change. For the search, which is not a
// visit (review of tranche 3).
func (v *Views) Indexing(ref string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, b := range v.builds {
		if (b.id == ref || b.name == ref) && !finished(b) {
			return true
		}
	}
	return false
}

// search walks the index from "/" as the explorer lists it: through
// directories only, the first MaxEntries names of each (List). An entry
// left in the map but out of reach (a directory replaced by a file later
// in the archive) is never seen.
func (x *index) search(ctx context.Context, q string, limit int) (hits []SearchHit, truncated bool) {
	lq := strings.ToLower(q)
	span := strings.Contains(lq, "/") // a match across a "/" needs the full path
	var byName, byParent []SearchHit
	matched := 0
	type dir struct {
		path    string
		matched bool // the path of the directory itself contains q
	}
	stack := []dir{{"/", false}}
	seen := 0
	for len(stack) > 0 {
		d := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		names := x.children[d.path]
		if len(names) > MaxEntries {
			names = names[:MaxEntries]
		}
		for _, n := range names {
			if seen%searchCheckEvery == 0 && ctx.Err() != nil {
				return finish(byName, byParent, limit), true
			}
			seen++
			p := childPath(d.path, n)
			e := x.entries[p]
			own := strings.Contains(strings.ToLower(n), lq)
			m := own || d.matched || (span && strings.Contains(strings.ToLower(p), lq))
			if e.Type == TypeDir {
				stack = append(stack, dir{p, m})
			}
			if !m {
				continue
			}
			if matched++; matched > limit && len(byName) >= limit {
				return finish(byName, byParent, limit), true // nothing found later can enter the result
			}
			h := SearchHit{Path: p, Type: e.Type}
			switch {
			case own && len(byName) < limit:
				byName = append(byName, h)
			case !own && len(byParent) < limit:
				byParent = append(byParent, h)
			}
		}
	}
	return finish(byName, byParent, limit), matched > limit
}

func finish(byName, byParent []SearchHit, limit int) []SearchHit {
	cmp := func(a, b SearchHit) int { return strings.Compare(a.Path, b.Path) }
	slices.SortFunc(byName, cmp)
	slices.SortFunc(byParent, cmp)
	out := append(byName, byParent...)
	return out[:min(len(out), limit)]
}
