package api

// Tests of the files of an image (spec v0.2 §5.5, §6, plan v0.2 tranche 6):
// GET /api/images/{id}/fs and /fs/file, POST /fs/search. Read only: no
// write route, no download, nothing written to Docker (the fake panics on
// PutArchive). The archives are `docker save` exports built in memory.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

type tarItem struct {
	name, body, link string
	typ              byte
}

func tarOf(t *testing.T, items ...tarItem) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, it := range items {
		h := &tar.Header{Name: it.name, Typeflag: it.typ, Linkname: it.link, Mode: 0o644, Size: int64(len(it.body)),
			ModTime: time.Unix(1_700_000_000, 0)}
		if h.Typeflag == 0 {
			h.Typeflag = tar.TypeReg
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(it.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// saveOf: the export of `docker save` of an image whose layers are given,
// encoded (gzip, or anything else to test a refusal).
func saveOf(t *testing.T, layers ...[]byte) []byte {
	t.Helper()
	config := []byte(`{"os":"linux"}`)
	files := []tarItem{}
	paths := []string{}
	for _, l := range layers {
		files = append(files, tarItem{name: "blobs/sha256/" + sha(l), body: string(l)})
		paths = append(paths, "blobs/sha256/"+sha(l))
	}
	m, _ := json.Marshal([]map[string]any{{"Config": "blobs/sha256/" + sha(config), "Layers": paths}})
	files = append(files, tarItem{name: "blobs/sha256/" + sha(config), body: string(config)},
		tarItem{name: "oci-layout", body: `{"imageLayoutVersion":"1.0.0"}`}, tarItem{name: "manifest.json", body: string(m)})
	return tarOf(t, files...)
}

func gzLayer(t *testing.T, items ...tarItem) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write(tarOf(t, items...))
	zw.Close()
	return buf.Bytes()
}

var fsImg = "sha256:" + imgHex(60)

// imageFSFixture: one image (fsImg) whose export is archive; Views with no
// first wait, so that the first request answers 202 at once.
func imageFSFixture(t *testing.T, readOnly bool, archive []byte) (*fixture, *dockerx.Fake, *http.Cookie) {
	t.Helper()
	fake := &dockerx.Fake{
		Images:     map[string]dockerx.ImageInfo{fsImg: {ID: fsImg, Size: int64(len(archive))}},
		ImageSaves: map[string][]byte{fsImg: archive},
		NoWrites:   true,
	}
	f := &fixture{docker: fake, views: fsview.NewViews(fake, fsview.Options{})}
	f.init(t, readOnly, fake, nil)
	_, c := f.login(t, f.token)
	return f, fake, c
}

func defaultImage(t *testing.T) []byte {
	return saveOf(t, gzLayer(t,
		tarItem{name: "etc/", typ: tar.TypeDir},
		tarItem{name: "etc/os-release", body: "NAME=hublot-test\n"},
		tarItem{name: "etc/fifo", typ: tar.TypeFifo},
		tarItem{name: "bin.dat", body: "\x00\x01binary"},
		tarItem{name: "big", body: strings.Repeat("b", 2<<20+1)},
		tarItem{name: "etc/nginx/", typ: tar.TypeDir},
		tarItem{name: "etc/nginx/nginx.conf", body: "worker_processes 1;"}))
}

// until polls method path while it answers 202 indexing.
func until(t *testing.T, f *fixture, method, path string, c *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(2 * time.Millisecond) {
		rec := f.do(method, path, "", c)
		if rec.Code != http.StatusAccepted {
			return rec
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s %s : toujours 202", method, path)
		}
	}
}

func imgFS(path string) string { return "/api/images/" + fsImg + "/fs" + path }

func TestImageFSList202Then200(t *testing.T) {
	f, _, c := imageFSFixture(t, false, defaultImage(t))
	rec := f.do("GET", imgFS("?path=/etc"), "", c)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("premier appel : %d %s", rec.Code, rec.Body)
	}
	var ix indexingJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &ix); err != nil || ix.Error != "indexing" {
		t.Errorf("202 : %s", rec.Body)
	}
	rec = until(t, f, "GET", imgFS("?path=/etc"), c)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	l := decodeListing(t, rec)
	names := []string{}
	for _, e := range l.Entries {
		names = append(names, e.Name)
	}
	if l.Type != "dir" || strings.Join(names, ",") != "fifo,nginx,os-release" {
		t.Errorf("listing : %+v", l)
	}
	// A file: its type, no entries (the UI previews it).
	rec = f.do("GET", imgFS("?path=/etc/os-release"), "", c)
	if l := decodeListing(t, rec); rec.Code != http.StatusOK || l.Type != "file" || len(l.Entries) != 0 {
		t.Errorf("listing d'un fichier : %d %+v", rec.Code, l)
	}
	// The ID with or without "sha256:".
	if rec := f.do("GET", "/api/images/"+imgHex(60)+"/fs?path=/", "", c); rec.Code != http.StatusOK {
		t.Errorf("ID sans sha256: : %d", rec.Code)
	}
}

func TestImageFSFile202Then200(t *testing.T) {
	f, fake, c := imageFSFixture(t, false, defaultImage(t))
	until(t, f, "GET", imgFS("?path=/"), c)
	rec := f.do("GET", imgFS("/file?path=/etc/os-release"), "", c)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("extraction : %d %s", rec.Code, rec.Body)
	}
	rec = until(t, f, "GET", imgFS("/file?path=/etc/os-release"), c)
	if rec.Code != http.StatusOK || rec.Body.String() != "NAME=hublot-test\n" ||
		!strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Errorf("aperçu : %d %q %v", rec.Code, rec.Body, rec.Header())
	}
	// HEAD is answered as GET (Go's mux), without the body.
	head := f.do("HEAD", imgFS("/file?path=/etc/os-release"), "", c)
	if head.Code != http.StatusOK || head.Header().Get("Content-Type") != rec.Header().Get("Content-Type") {
		t.Errorf("HEAD : %d %v", head.Code, head.Header())
	}
	if n := fake.ImageSaveCalls.Load(); n != 2 {
		t.Errorf("%d exports, attendu 2 (l'index, une extraction gardée 60 s)", n)
	}
	// Binary: no text preview.
	rec = until(t, f, "GET", imgFS("/file?path=/bin.dat"), c)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != binaryType || rec.Body.Len() != 0 {
		t.Errorf("binaire : %d %v %d", rec.Code, rec.Header(), rec.Body.Len())
	}
}

func TestImageFSErrors(t *testing.T) {
	f, _, c := imageFSFixture(t, false, defaultImage(t))
	until(t, f, "GET", imgFS("?path=/"), c)
	for path, want := range map[string]struct {
		status int
		code   string
	}{
		"/file?path=/etc/fifo": {http.StatusUnprocessableEntity, "not_regular"},
		"/file?path=/etc":      {http.StatusUnprocessableEntity, "not_regular"},
		"/file?path=/big":      {http.StatusRequestEntityTooLarge, "too_large"},
		"/file?path=/absent":   {http.StatusNotFound, "not_found"},
		"?path=/absent":        {http.StatusNotFound, "not_found"},
		"?path=/a%00b":         {http.StatusNotFound, "not_found"},
	} {
		rec := until(t, f, "GET", imgFS(path), c)
		var e struct{ Error string }
		json.Unmarshal(rec.Body.Bytes(), &e)
		if rec.Code != want.status || e.Error != want.code {
			t.Errorf("%s : %d %s, attendu %d %s", path, rec.Code, rec.Body, want.status, want.code)
		}
	}
}

func TestImageFSUnsupported(t *testing.T) {
	zstd := append([]byte{0x28, 0xb5, 0x2f, 0xfd}, make([]byte, 600)...)
	f, fake, c := imageFSFixture(t, false, saveOf(t, zstd))
	rec := until(t, f, "GET", imgFS("?path=/"), c)
	var e struct{ Error string }
	json.Unmarshal(rec.Body.Bytes(), &e)
	if rec.Code != http.StatusUnprocessableEntity || e.Error != "unsupported_image" {
		t.Errorf("couche zstd : %d %s", rec.Code, rec.Body)
	}
	f.do("GET", imgFS("?path=/"), "", c)
	if n := fake.ImageSaveCalls.Load(); n != 1 {
		t.Errorf("%d exports, attendu 1 (refus gardé)", n)
	}
}

// An image is named by its full ID only: anything else is 404 without
// asking Docker.
func TestImageFSBadID(t *testing.T) {
	f, fake, c := imageFSFixture(t, false, defaultImage(t))
	for _, id := range []string{"nginx:latest", "nginx", imgHex(60)[:63], imgHex(60)[:12], "sha256:" + strings.ToUpper(imgHex(60)), "..%2f..%2fx"} {
		for _, p := range []string{"/fs?path=/", "/fs/file?path=/etc/os-release"} {
			rec := f.do("GET", "/api/images/"+id+p, "", c)
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s%s : %d", id, p, rec.Code)
			}
		}
		if rec := f.do("POST", "/api/images/"+id+"/fs/search", `{"q":"os"}`, c); rec.Code != http.StatusNotFound {
			t.Errorf("recherche %s : %d", id, rec.Code)
		}
	}
	if fake.ImageCalls.Load() != 0 || fake.ImageSaveCalls.Load() != 0 {
		t.Errorf("Docker interrogé : %d inspect, %d exports", fake.ImageCalls.Load(), fake.ImageSaveCalls.Load())
	}
	// A full ID unknown to Docker: 404, a fixed detail.
	rec := f.do("GET", "/api/images/sha256:"+imgHex(61)+"/fs?path=/", "", c)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "no such image") {
		t.Errorf("image inconnue : %d %s", rec.Code, rec.Body)
	}
}

func TestImageFSNeedsSession(t *testing.T) {
	f, _, _ := imageFSFixture(t, false, defaultImage(t))
	for _, p := range []string{imgFS("?path=/"), imgFS("/file?path=/etc/os-release")} {
		if rec := f.do("GET", p, "", nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s sans session : %d", p, rec.Code)
		}
	}
	if rec := f.do("POST", imgFS("/search"), `{"q":"os"}`, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("recherche sans session : %d", rec.Code)
	}
}

// Read only (Q63): no write route, no download; allowed in --read-only.
func TestImageFSReadOnlyRoutes(t *testing.T) {
	f, fake, c := imageFSFixture(t, true, defaultImage(t))
	if rec := until(t, f, "GET", imgFS("?path=/etc"), c); rec.Code != http.StatusOK {
		t.Errorf("listing en --read-only : %d", rec.Code)
	}
	for _, r := range []struct{ method, path string }{
		{"PUT", imgFS("/file?path=/etc/os-release")}, {"POST", imgFS("/upload?dir=/etc")},
		{"GET", imgFS("/download?path=/etc")}, {"DELETE", imgFS("/file?path=/etc/os-release")},
	} {
		if rec := f.do(r.method, r.path, "x", c); rec.Code < 400 {
			t.Errorf("%s %s : %d", r.method, r.path, rec.Code)
		}
	}
	if len(fake.PutCalls()) != 0 {
		t.Error("PutArchive appelé")
	}
}

func imageSearch(f *fixture, body string, c *http.Cookie) (*httptest.ResponseRecorder, searchJSON) {
	rec := f.do("POST", imgFS("/search"), body, c)
	var s searchJSON
	json.Unmarshal(rec.Body.Bytes(), &s)
	return rec, s
}

func TestImageFSSearch(t *testing.T) {
	f, fake, c := imageFSFixture(t, false, defaultImage(t))
	rec, s := imageSearch(f, `{"q":"nginx"}`, c)
	if rec.Code != http.StatusOK || s.Indexed || len(s.Results) != 0 {
		t.Errorf("avant l'index : %d %s", rec.Code, rec.Body)
	}
	if n := fake.ImageSaveCalls.Load(); n != 0 {
		t.Fatalf("la recherche a lancé %d export(s)", n)
	}
	until(t, f, "GET", imgFS("?path=/"), c)
	rec, s = imageSearch(f, `{"q":"nginx.c"}`, c)
	if rec.Code != http.StatusOK || !s.Indexed || len(s.Results) != 1 || s.Results[0].Path != "/etc/nginx/nginx.conf" || s.Results[0].Type != "file" {
		t.Errorf("après l'index : %d %s", rec.Code, rec.Body)
	}
	for _, body := range []string{`{"q":"x"}`, `{"q":"nginx"}{"q":"x"}`, `nginx`, `{"q":"` + strings.Repeat("a", 201) + `"}`} {
		rec, _ := imageSearch(f, body, c)
		var e struct{ Error string }
		json.Unmarshal(rec.Body.Bytes(), &e)
		if rec.Code != http.StatusBadRequest || e.Error != "invalid_query" {
			t.Errorf("corps %.30q : %d %s", body, rec.Code, rec.Body)
		}
	}
	if strings.Contains(fmt.Sprint(rec.Header()), "nginx") {
		t.Error("la requête est reprise dans un en-tête")
	}
}
