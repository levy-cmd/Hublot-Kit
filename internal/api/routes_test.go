package api

import (
	"net/http"
	"testing"
)

// A module registered the way later tranches do (routes_<module>.go).
func init() {
	moduleRoutes = append(moduleRoutes, func(s *server, mux *http.ServeMux) {
		mux.Handle("GET /api/test-module", s.session(func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		}))
	})
}

func TestModuleRoutesRegistered(t *testing.T) {
	f := newFixture(t, false)
	if rec := f.do("GET", "/api/test-module", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("without a session: %d %s", rec.Code, rec.Body)
	}
	_, c := f.login(t, f.token)
	if rec := f.do("GET", "/api/test-module", "", c); rec.Code != http.StatusOK {
		t.Fatalf("with a session: %d %s", rec.Code, rec.Body)
	}
}
