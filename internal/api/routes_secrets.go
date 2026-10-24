package api

import "net/http"

// Inventory of the secrets, names only (spec v0.2 §5.9, secrets_inventory.go).
func init() {
	moduleRoutes = append(moduleRoutes, func(s *server, mux *http.ServeMux) {
		mux.Handle("GET /api/secrets", s.session(s.secretsInventory))
	})
}
