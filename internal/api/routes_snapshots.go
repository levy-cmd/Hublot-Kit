package api

import "net/http"

// Snapshots and the bounded count (spec v0.2 §5.7, §6; snapshots.go,
// count.go). Allowed read-only (Q57): they write in the agent's directory,
// never in Docker. The unsafe methods pass the Origin check (§7).
func init() {
	moduleRoutes = append(moduleRoutes, func(s *server, mux *http.ServeMux) {
		mux.Handle("POST /api/containers/{id}/snapshots", s.session(s.snapshotCreate))
		mux.Handle("GET /api/snapshots", s.session(s.snapshotList))
		mux.Handle("DELETE /api/snapshots/{sid}", s.session(s.snapshotDelete))
		mux.Handle("GET /api/containers/{id}/count", s.session(s.count))
	})
}
