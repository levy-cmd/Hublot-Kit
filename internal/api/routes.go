package api

import "net/http"

// moduleRoutes is where the routes of later tranches register (plan v0.2,
// "Ordre, branches et parallélisme"): each module adds its own
// routes_<module>.go with
//
//	func init() { moduleRoutes = append(moduleRoutes, func(s *server, mux *http.ServeMux) { … }) }
//
// and never edits server.go. New calls each function once, after the
// routes of v0.1. Wrap every handler in s.session.
var moduleRoutes []func(*server, *http.ServeMux)

// session lets a handler through only with a valid session cookie (§6).
func (s *server) session(h http.HandlerFunc) http.Handler { return requireSession(s.d.Sessions, h) }
