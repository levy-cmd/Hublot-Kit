package api

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// changesTimeout bounds GET /changes (F5): Docker computes the diff of the
// container layer, which is slow on a large one. No FUSE guard: the diff does
// not walk the mounts (volumes are excluded).
const changesTimeout = 30 * time.Second

// changes answers the diff of the container against its image, sorted by
// path, as a complete array (`[]` when empty).
func (s *server) changes(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !containerRef.MatchString(id) {
		writeError(w, http.StatusNotFound, "not_found", "invalid container ID")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), changesTimeout)
	defer cancel()
	cs, err := s.d.Docker.Changes(ctx, id)
	if err != nil {
		writeFSError(w, err)
		return
	}
	if cs == nil {
		cs = []dockerx.Change{}
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].Path < cs[j].Path })
	writeJSON(w, http.StatusOK, cs)
}
