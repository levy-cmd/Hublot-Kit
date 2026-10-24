package api

import "net/http"

// Files of an image, read only (spec v0.2 §5.5, images_fs.go). No write
// route and no download (Q63).
func init() {
	moduleRoutes = append(moduleRoutes, func(s *server, mux *http.ServeMux) {
		mux.Handle("GET /api/images/{id}/fs", s.session(s.imageFSList))
		mux.Handle("GET /api/images/{id}/fs/file", s.session(s.imageFSFile))
		mux.Handle("POST /api/images/{id}/fs/search", s.session(s.imageFSSearch))
	})
}
