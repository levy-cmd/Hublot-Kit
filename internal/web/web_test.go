package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

var site = fstest.MapFS{
	"index.html":          {Data: []byte("<!doctype html><title>Hublot</title>")},
	"_app/immutable/a.js": {Data: []byte("console.log(1)")},
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

func TestServesIndexAtRoot(t *testing.T) {
	rec := get(NewHandler(site), "/")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<title>Hublot</title>") {
		t.Fatalf("got %d %q", rec.Code, rec.Body)
	}
}

func TestServesAsset(t *testing.T) {
	rec := get(NewHandler(site), "/_app/immutable/a.js")
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Content-Type"), "javascript") {
		t.Fatalf("got %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestSPAFallback(t *testing.T) {
	for _, p := range []string{"/login", "/c/abc123", "/servers"} {
		rec := get(NewHandler(site), p)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<title>Hublot</title>") || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
			t.Errorf("%s: got %d", p, rec.Code)
		}
	}
}

func TestNotBuiltYet(t *testing.T) {
	rec := get(NewHandler(fstest.MapFS{}), "/")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "make web") {
		t.Fatalf("got %d %q", rec.Code, rec.Body)
	}
}

func TestMissingAssetWithDot(t *testing.T) {
	rec := get(NewHandler(site), "/_app/immutable/missing.js")
	if rec.Code != 404 {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}

func TestMissingFaviconWithDot(t *testing.T) {
	rec := get(NewHandler(site), "/favicon.png")
	if rec.Code != 404 {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}

func TestPathWithDotSegment(t *testing.T) {
	rec := get(NewHandler(site), "/.gitkeep")
	if rec.Code != 404 {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}

func TestSPAFallbackCacheControl(t *testing.T) {
	rec := get(NewHandler(site), "/login")
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("got %d, Cache-Control: %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
}

func TestAssetCacheControl(t *testing.T) {
	rec := get(NewHandler(site), "/_app/immutable/a.js")
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("got %d, Cache-Control: %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
}

func TestHeadRequest(t *testing.T) {
	rec := httptest.NewRecorder()
	NewHandler(site).ServeHTTP(rec, httptest.NewRequest("HEAD", "/login", nil))
	if rec.Code != 200 {
		t.Fatalf("got %d, want 200", rec.Code)
	}
}
