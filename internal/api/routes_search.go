package api

import "net/http"

// File search of the global search (spec v0.2 §5.8, fs_search.go).
func init() {
	moduleRoutes = append(moduleRoutes, func(s *server, mux *http.ServeMux) {
		mux.Handle("GET /api/containers/{id}/fs/search", s.session(s.fsSearch))
	})
}
