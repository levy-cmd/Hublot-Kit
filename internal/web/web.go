// Package web serves the compiled SvelteKit UI embedded in the binary.
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"slices"
	"strings"
)

//go:embed all:dist
var dist embed.FS

func Handler() http.Handler { return NewHandler(embedded()) }

func embedded() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // "dist" is always embedded
	}
	return sub
}

// ScriptHashes returns the CSP sources ('sha256-…') of the inline scripts of
// the embedded index.html (SvelteKit's bootstrap); empty if the UI is not built.
func ScriptHashes() []string { return InlineScriptHashes(embedded()) }

var inlineScript = regexp.MustCompile(`(?is)<script(\s[^>]*)?>(.*?)</script\s*>`)
var srcAttr = regexp.MustCompile(`(?i)(^|\s)src\s*=`)

// InlineScriptHashes hashes the body of every <script> without a src attribute
// in files/index.html, in document order, without duplicates.
func InlineScriptHashes(files fs.FS) []string {
	index, err := fs.ReadFile(files, "index.html")
	if err != nil {
		return nil
	}
	var out []string
	for _, m := range inlineScript.FindAllSubmatch(index, -1) {
		if srcAttr.Match(m[1]) {
			continue
		}
		sum := sha256.Sum256(m[2])
		h := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
		if !slices.Contains(out, h) {
			out = append(out, h)
		}
	}
	return out
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
