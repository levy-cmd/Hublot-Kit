package api

import "net/http"

// Images, list and detail (spec v0.2 §5.4, images.go).
func init() {
	moduleRoutes = append(moduleRoutes, func(s *server, mux *http.ServeMux) {
		mux.Handle("GET /api/images", s.session(s.images))
		mux.Handle("GET /api/images/{id}", s.session(s.image))
	})
}
