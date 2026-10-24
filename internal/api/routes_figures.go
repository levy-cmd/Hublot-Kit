package api

import "net/http"

// Key figures of the detail (spec v0.2 §5.3, figures.go).
func init() {
	moduleRoutes = append(moduleRoutes, func(s *server, mux *http.ServeMux) {
		mux.Handle("GET /api/containers/{id}/figures", s.session(s.figures(newFigureCache())))
	})
}
