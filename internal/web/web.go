// Package web serves the compiled SvelteKit UI embedded in the binary.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // "dist" is always embedded
	}
	return NewHandler(sub)
}

func NewHandler(files fs.FS) http.Handler {
	fileServer := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")

		// Check if any path segment starts with "."
		if p != "" {
			for _, segment := range strings.Split(p, "/") {
				if strings.HasPrefix(segment, ".") {
					http.NotFound(w, r)
					return
				}
			}
		}

		if p != "" {
			if st, err := fs.Stat(files, p); err == nil && !st.IsDir() {
				// File exists, serve it
				w.Header().Set("Content-Type", contentType(p))
				if strings.HasPrefix(p, "_app/immutable/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				fileServer.ServeHTTP(w, r)
				return
			}

			// File doesn't exist: return 404 if it looks like an asset
			if isAssetPath(p) {
				http.NotFound(w, r)
				return
			}
		}

		// SPA fallback: serve index.html for extension-less paths
		index, err := fs.ReadFile(files, "index.html")
		if err != nil {
			http.Error(w, "interface non compilée : lancez `make web`", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(index)
	})
}

func isAssetPath(p string) bool {
	// Paths starting with _app/ are assets
	if strings.HasPrefix(p, "_app/") {
		return true
	}
	// Paths with a dot in the last segment (file extensions) are assets
	lastSegment := p
	if idx := strings.LastIndex(p, "/"); idx >= 0 {
		lastSegment = p[idx+1:]
	}
	return strings.Contains(lastSegment, ".")
}

func contentType(p string) string {
	if strings.HasSuffix(p, ".js") {
		return "application/javascript"
	}
	if strings.HasSuffix(p, ".css") {
		return "text/css"
	}
	if strings.HasSuffix(p, ".html") {
		return "text/html; charset=utf-8"
	}
	return "application/octet-stream"
}
