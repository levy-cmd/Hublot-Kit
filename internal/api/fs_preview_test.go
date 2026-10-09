package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
)

func preview(t *testing.T, content []byte) (*fixture, *http.Response, []byte) {
	t.Helper()
	f := newFixture(t, false)
	f.fs.AddDir("/d").AddFile("/d/f", content)
	rec := f.do("GET", "/api/containers/web/fs/file?path=/d/f", "", f.session(t))
	return f, rec.Result(), rec.Body.Bytes()
}

func TestPreviewText(t *testing.T) {
	_, res, body := preview(t, []byte("hello\n"))
	sum := sha256.Sum256([]byte("hello\n"))
	if res.StatusCode != http.StatusOK || string(body) != "hello\n" ||
		res.Header.Get("Content-Type") != "text/plain; charset=utf-8" ||
		res.Header.Get("ETag") != `"`+hex.EncodeToString(sum[:])+`"` ||
		res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("%d %v %q", res.StatusCode, res.Header, body)
	}
}

func TestPreviewActiveContentIsText(t *testing.T) {
	for _, doc := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`,
		`<!DOCTYPE html><html><script>alert(1)</script></html>`,
		`<?xml version="1.0"?><x/>`,
	} {
		if _, res, _ := preview(t, []byte(doc)); res.Header.Get("Content-Type") != "text/plain; charset=utf-8" {
			t.Errorf("%.20q servi en %q", doc, res.Header.Get("Content-Type"))
		}
	}
}

func TestPreviewImage(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), 0, 0, 0, 13)
	_, res, body := preview(t, png)
	if res.Header.Get("Content-Type") != "image/png" || !bytes.Equal(body, png) {
		t.Fatalf("%q %q", res.Header.Get("Content-Type"), body)
	}
}

func TestPreviewBinaryHasNoBody(t *testing.T) {
	_, res, body := preview(t, []byte("ELF\x00\x01\x02"))
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "application/octet-stream" || len(body) != 0 {
		t.Fatalf("%d %q %q", res.StatusCode, res.Header.Get("Content-Type"), body)
	}
	// A NUL byte after the first 8 KiB does not make a binary.
	_, res, _ = preview(t, append([]byte(strings.Repeat("a", 8<<10)), 0))
	if res.Header.Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("NUL après 8 Kio : %q", res.Header.Get("Content-Type"))
	}
}

func TestPreviewSizeLimit(t *testing.T) {
	if _, res, body := preview(t, nil); res.StatusCode != http.StatusOK || len(body) != 0 || res.Header.Get("ETag") == "" {
		t.Fatalf("fichier vide : %d %v", res.StatusCode, res.Header)
	}
	if _, res, body := preview(t, bytes.Repeat([]byte("a"), previewMax)); res.StatusCode != http.StatusOK || len(body) != previewMax {
		t.Fatalf("exactement 2 Mio : %d (%d octets)", res.StatusCode, len(body))
	}
	f, _, _ := preview(t, nil)
	f.fs.AddFile("/d/big", bytes.Repeat([]byte("a"), previewMax+1))
	rec := f.do("GET", "/api/containers/web/fs/file?path=/d/big", "", f.session(t))
	if rec.Code != http.StatusRequestEntityTooLarge || errCode(t, rec) != "too_large" {
		t.Fatalf("2 Mio + 1 : %d %s", rec.Code, rec.Body)
	}
}

func TestPreviewNotRegular(t *testing.T) {
	f, _, _ := preview(t, nil)
	rec := f.do("GET", "/api/containers/web/fs/file?path=/d", "", f.session(t))
	if rec.Code != http.StatusUnprocessableEntity || errCode(t, rec) != "not_regular" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}
