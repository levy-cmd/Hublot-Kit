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
	staticExts := map[string]bool{
		".js": true, ".mjs": true, ".css": true, ".map": true,
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
		".webp": true, ".svg": true, ".ico": true,
		".woff": true, ".woff2": true, ".ttf": true,
		".json": true, ".txt": true, ".webmanifest": true,
	}

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
				if strings.HasPrefix(p, "_app/immutable/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				fileServer.ServeHTTP(w, r)
				return
			}

			// File doesn't exist: return 404 only for known static asset extensions or _app/ paths
			// Never 404 for c/ paths (container names may have dots)
			if !strings.HasPrefix(p, "c/") && (strings.HasPrefix(p, "_app/") || staticExts[strings.ToLower(path.Ext(p))]) {
				http.NotFound(w, r)
				return
			}
		}

		// SPA fallback: serve index.html
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
