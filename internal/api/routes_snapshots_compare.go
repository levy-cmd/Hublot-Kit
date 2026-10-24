package api

import "net/http"

// The comparison of snapshots and the match with the image (spec v0.2
// §5.7, §6; snapshots_compare.go). Reads only: nothing is written, neither
// in Docker nor in the agent's directory.
func init() {
	moduleRoutes = append(moduleRoutes, func(s *server, mux *http.ServeMux) {
		mux.Handle("GET /api/snapshots/{sid}/compare", s.session(s.snapshotCompare))
		mux.Handle("GET /api/snapshots/{sid}/image-match", s.session(s.snapshotImageMatch))
	})
}
