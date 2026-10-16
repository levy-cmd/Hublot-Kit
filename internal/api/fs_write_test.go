package api

// PUT /fs/file and POST /fs/upload (spec §6, F4) through the real wiring of
// New: package files on an in-memory view, a fake Docker.

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/files"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

// send is f.do with a raw body and extra headers.
func (f *fixture) send(method, path string, body []byte, cookie *http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://example.com"+path, bytes.NewReader(body))
	r.RemoteAddr = "192.0.2.1:40000"
	r.Header.Set("Origin", "http://example.com")
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, r)
	return rec
}

// multipartBody builds the body of an upload whose part field carries the
// filename given, quoted by %q (no base name).
func multipartBody(t *testing.T, field, filename string, content []byte) ([]byte, string) {
	t.Helper()
	return multipartRaw(t, fmt.Sprintf(`form-data; name=%q; filename=%q`, field, filename), content)
}

// multipartRaw builds the body of an upload with a raw Content-Disposition.
func multipartRaw(t *testing.T, disposition string, content []byte) ([]byte, string) {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", disposition)
	h.Set("Content-Type", "application/octet-stream")
	p, err := mw.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	p.Write(content)
	mw.Close()
	return b.Bytes(), mw.FormDataContentType()
}

func (f *fixture) upload(t *testing.T, c *http.Cookie, ctr, dir, filename string, content []byte, inm string) *httptest.ResponseRecorder {
	t.Helper()
	body, ct := multipartBody(t, "file", filename, content)
	h := map[string]string{"Content-Type": ct}
	if inm != "" {
		h["If-None-Match"] = inm
	}
	return f.send("POST", "/api/containers/"+ctr+"/fs/upload?dir="+dir, body, c, h)
}

func writeFixture(t *testing.T, readOnly bool) (*fixture, *http.Cookie) {
	t.Helper()
	f := newFixture(t, readOnly)
	f.fs.AddDir("/etc").AddFile("/etc/app.conf", []byte("port=80\n")).AddLink("/etc/alias", "app.conf")
	return f, f.session(t)
}

func TestEditRoute(t *testing.T) {
	f, c := writeFixture(t, false)
	// The If-Match is the ETag of the preview, as is.
	tag := f.do("GET", "/api/containers/web/fs/file?path=/etc/app.conf", "", c).Header().Get("ETag")
	rec := f.send("PUT", "/api/containers/web/fs/file?path=/etc/app.conf", []byte("10.0.0.1\n"), c, map[string]string{"If-Match": tag})
	var body struct{ ETag string }
	json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusOK || body.ETag != files.ETag([]byte("10.0.0.1\n")) || rec.Header().Get("ETag") != body.ETag {
		t.Fatalf("édition : %d %s", rec.Code, rec.Body)
	}
	puts := f.docker.PutCalls()
	if len(puts) != 1 || puts[0].ID != webID || puts[0].Dir != "/etc" {
		t.Fatalf("PutArchive : %d appel(s)", len(puts))
	}
	log, _ := os.ReadFile(f.auditP)
	if n := strings.Count(string(log), `"action":"file.write"`); n != 2 {
		t.Fatalf("journal : %d ligne(s) file.write, attendu 2 (intention, résultat)\n%s", n, log)
	}
}

func TestWriteRoutesErrors(t *testing.T) {
	f, c := writeFixture(t, false)
	tag := files.ETag([]byte("port=80\n"))
	put := func(p string, body []byte, ifMatch string) *httptest.ResponseRecorder {
		return f.send("PUT", "/api/containers/web/fs/file?path="+p, body, c, map[string]string{"If-Match": ifMatch})
	}
	cases := []struct {
		name   string
		rec    *httptest.ResponseRecorder
		status int
		code   string
	}{
		{"If-Match faux", put("/etc/app.conf", []byte("x"), files.ETag([]byte("autre"))), 409, "conflict"},
		{"If-Match absent", put("/etc/app.conf", []byte("x"), ""), 409, "conflict"},
		{"lien final", put("/etc/alias", []byte("x"), tag), 422, "link_in_path"},
		{"dossier", put("/etc", []byte("x"), tag), 422, "not_regular"},
		{"absent", put("/etc/nope", []byte("x"), tag), 404, "not_found"},
		{"trop gros", put("/etc/app.conf", make([]byte, files.MaxEditBytes+1), tag), 413, "too_large"},
		{"nom a/b brut", f.upload(t, c, "web", "/etc", "a/b", []byte("x"), "*"), 400, "invalid_name"},
		{"nom ..", f.upload(t, c, "web", "/etc", "..", []byte("x"), "*"), 400, "invalid_name"},
		{"nom vide", f.upload(t, c, "web", "/etc", "", []byte("x"), "*"), 400, "invalid_name"},
		{"sans If-None-Match", f.upload(t, c, "web", "/etc", "n", []byte("x"), ""), 409, "conflict"},
		{"nom existant", f.upload(t, c, "web", "/etc", "app.conf", []byte("x"), "*"), 409, "conflict"},
		{"nom ../x sans If-None-Match", f.upload(t, c, "web", "/etc", "../x", []byte("x"), ""), 400, "invalid_name"},
		{"nom avec \\", f.upload(t, c, "web", "/etc", `a\b`, []byte("x"), "*"), 400, "invalid_name"},
		{"/etc/hosts (Q21)", f.upload(t, c, "web", "/etc", "hosts", []byte("x"), "*"), 403, "forbidden_path"},
		{"nom a/b vers /proc : le nom d'abord", f.upload(t, c, "web", "/proc", "a/b", []byte("x"), "*"), 400, "invalid_name"},
		{"envoi sur un lien", f.upload(t, c, "web", "/etc", "alias", []byte("x"), "*"), 422, "link_in_path"},
		{"envoi trop gros", f.upload(t, c, "web", "/etc", "big", make([]byte, files.MaxUploadBytes+1), "*"), 413, "too_large"},
	}
	for _, k := range cases {
		if k.rec.Code != k.status || errCode(t, k.rec) != k.code {
			t.Errorf("%s : %d %s, attendu %d %s", k.name, k.rec.Code, k.rec.Body, k.status, k.code)
		}
	}
	// A part other than "file" only: no file, an empty name.
	body, ct := multipartBody(t, "autre", "x", []byte("x"))
	if rec := f.send("POST", "/api/containers/web/fs/upload?dir=/etc", body, c, map[string]string{"Content-Type": ct, "If-None-Match": "*"}); rec.Code != 400 || errCode(t, rec) != "invalid_name" {
		t.Errorf("sans champ file : %d %s", rec.Code, rec.Body)
	}
	if n := len(f.docker.PutCalls()); n != 0 {
		t.Fatalf("PutArchive appelé %d fois malgré les refus", n)
	}
}

func TestUploadRoute(t *testing.T) {
	f, c := writeFixture(t, false)
	rec := f.upload(t, c, "web", "/etc", "new.conf", []byte("port=80\n"), "*")
	var body struct{ Path, ETag string }
	json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusCreated || body.Path != "/etc/new.conf" || body.ETag != files.ETag([]byte("port=80\n")) {
		t.Fatalf("envoi : %d %s", rec.Code, rec.Body)
	}
	if puts := f.docker.PutCalls(); len(puts) != 1 || puts[0].Dir != "/etc" {
		t.Fatalf("PutArchive : %d appel(s)", len(puts))
	}
}

func TestWriteRoutesReadOnly(t *testing.T) {
	f, c := writeFixture(t, true)
	// Refused first: before the container is even looked up.
	for _, rec := range []*httptest.ResponseRecorder{
		f.send("PUT", "/api/containers/absent/fs/file?path=/etc/app.conf", []byte("x"), c, map[string]string{"If-Match": "x"}),
		f.upload(t, c, "web", "/etc", "n", []byte("x"), "*"),
	} {
		if rec.Code != http.StatusForbidden || errCode(t, rec) != "read_only" {
			t.Errorf("lecture seule : %d %s", rec.Code, rec.Body)
		}
	}
	if n := len(f.docker.PutCalls()); n != 0 {
		t.Fatal("PutArchive appelé en lecture seule")
	}
}

func TestWriteRoutesNeedSessionAndOrigin(t *testing.T) {
	f, c := writeFixture(t, false)
	if rec := f.send("PUT", "/api/containers/web/fs/file?path=/etc/app.conf", []byte("x"), nil, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("sans session : %d", rec.Code)
	}
	if rec := f.send("PUT", "/api/containers/web/fs/file?path=/etc/app.conf", []byte("x"), c, map[string]string{"Origin": "http://evil.example"}); rec.Code != http.StatusForbidden || errCode(t, rec) != "forbidden_origin" {
		t.Errorf("Origin étrangère : %d %s", rec.Code, rec.Body)
	}
}

// Docker's own refusals: read-only rootfs or :ro volume → read_only, with
// Docker's message as detail; any other invalid extraction (the path
// changed after the checks) → conflict.
func TestWriteDockerRefused(t *testing.T) {
	f, c := writeFixture(t, false)
	f.docker.PutErr = fmt.Errorf("%w : container rootfs is marked read-only", dockerx.ErrRefused)
	rec := f.upload(t, c, "web", "/etc", "n", []byte("x"), "*")
	if rec.Code != http.StatusForbidden || errCode(t, rec) != "read_only" || !strings.Contains(rec.Body.String(), "rootfs") {
		t.Fatalf("refus de Docker : %d %s", rec.Code, rec.Body)
	}
	f.docker.PutErr = fmt.Errorf("%w : extraction point is not a directory", dockerx.ErrPutInvalid)
	rec = f.upload(t, c, "web", "/etc", "n2", []byte("x"), "*")
	if rec.Code != http.StatusConflict || errCode(t, rec) != "conflict" {
		t.Fatalf("extraction refusée : %d %s", rec.Code, rec.Body)
	}
}

// Stopped container: the write waits for the index like every file route.
func TestWriteStoppedIndexing(t *testing.T) {
	fake := stoppedFake(t)
	g := &gatedAPI{Fake: fake, open: make(chan struct{})}
	f, c := stoppedFixture(t, g, fake, fsview.Options{})
	rec := f.send("PUT", "/api/containers/db/fs/file?path=/etc/motd", []byte("x"), c, map[string]string{"If-Match": files.ETag([]byte("db\n"))})
	if rec.Code != http.StatusAccepted || errCode(t, rec) != "indexing" {
		t.Fatalf("pendant l'indexation : %d %s", rec.Code, rec.Body)
	}
	close(g.open)
}

// The filename is decoded by mime.ParseMediaType (RFC 2231 filename*,
// quoted pairs) before it is validated: the value checked is the value
// written in the tar, never another one (Q19).
func TestUploadNameIsTheDecodedValue(t *testing.T) {
	f, c := writeFixture(t, false)
	send := func(disposition string) *httptest.ResponseRecorder {
		body, ct := multipartRaw(t, disposition, []byte("x"))
		return f.send("POST", "/api/containers/web/fs/upload?dir=/etc", body, c, map[string]string{"Content-Type": ct, "If-None-Match": "*"})
	}
	for _, d := range []string{
		`form-data; name="file"; filename*=UTF-8''..%2Fx`, // decodes to ../x
		`form-data; name="file"; filename="a\/b"`,         // quoted pair: a/b
		`form-data; name="file"; filename*=UTF-8''a%0Ab`,  // a, LF, b
	} {
		if rec := send(d); rec.Code != http.StatusBadRequest || errCode(t, rec) != "invalid_name" {
			t.Errorf("%s : %d %s", d, rec.Code, rec.Body)
		}
	}
	if n := len(f.docker.PutCalls()); n != 0 {
		t.Fatalf("PutArchive appelé %d fois", n)
	}
	rec := send(`form-data; name="file"; filename*=UTF-8''%C3%A9t%C3%A9.txt`)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"/etc/été.txt"`) {
		t.Fatalf("nom encodé : %d %s", rec.Code, rec.Body)
	}
	if h, _ := tarEntry(t, f.docker.PutCalls()[0].Tar); h != "été.txt" {
		t.Fatalf("nom écrit dans le tar : %q", h)
	}
}

// tarEntry returns the name and body of the single entry of a tar.
func tarEntry(t *testing.T, b []byte) (string, string) {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(b))
	h, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(tr)
	return h.Name, string(body)
}

// auditWrites counts the file.write lines of the audit log.
func (f *fixture) auditWrites(t *testing.T) int {
	t.Helper()
	log, _ := os.ReadFile(f.auditP)
	return strings.Count(string(log), `"action":"file.write"`)
}

// A refusal is proved by no PutArchive AND no audit line (global constraint).
func TestWriteRefusalsLeaveNoTrace(t *testing.T) {
	for _, ro := range []bool{false, true} {
		f, c := writeFixture(t, ro)
		f.send("PUT", "/api/containers/web/fs/file?path=/etc/app.conf", []byte("x"), c, map[string]string{"If-Match": files.ETag([]byte("autre"))})
		f.send("PUT", "/api/containers/web/fs/file?path=/etc/alias", []byte("x"), c, map[string]string{"If-Match": files.ETag([]byte("port=80\n"))})
		f.upload(t, c, "web", "/etc", "a/b", []byte("x"), "*")
		f.upload(t, c, "web", "/etc", "n", []byte("x"), "")
		f.upload(t, c, "web", "/etc", "hosts", []byte("x"), "*")
		if ro {
			f.upload(t, c, "web", "/etc", "n", []byte("x"), "*")
		}
		if n := len(f.docker.PutCalls()); n != 0 {
			t.Errorf("readOnly=%v : PutArchive appelé %d fois", ro, n)
		}
		if n := f.auditWrites(t); n != 0 {
			t.Errorf("readOnly=%v : %d ligne(s) file.write après des refus", ro, n)
		}
	}
}

// The MaxBytesError path: an upload body past the cap of the reader (32 MiB
// plus the multipart overhead) is too_large, with no write and no audit.
func TestWriteTooLargeBodies(t *testing.T) {
	f, c := writeFixture(t, false)
	rec := f.upload(t, c, "web", "/etc", "huge", make([]byte, files.MaxUploadBytes+uploadOverhead+1024), "*")
	if rec.Code != http.StatusRequestEntityTooLarge || errCode(t, rec) != "too_large" {
		t.Errorf("envoi au-delà du plafond du lecteur : %d %s", rec.Code, rec.Body)
	}
	tag := files.ETag([]byte("port=80\n"))
	rec = f.send("PUT", "/api/containers/web/fs/file?path=/etc/app.conf", make([]byte, 3<<20), c, map[string]string{"If-Match": tag})
	if rec.Code != http.StatusRequestEntityTooLarge || errCode(t, rec) != "too_large" {
		t.Errorf("édition de 3 Mio : %d %s", rec.Code, rec.Body)
	}
	if n := len(f.docker.PutCalls()); n != 0 {
		t.Errorf("PutArchive appelé %d fois", n)
	}
	if n := f.auditWrites(t); n != 0 {
		t.Errorf("%d ligne(s) file.write", n)
	}
}

// Two parts "file": refused (ambiguous name), nothing written or audited.
func TestUploadSecondFilePartRefused(t *testing.T) {
	f, c := writeFixture(t, false)
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	for _, n := range []string{"a.txt", "b.txt"} {
		p, _ := mw.CreateFormFile("file", n)
		p.Write([]byte("x"))
	}
	mw.Close()
	rec := f.send("POST", "/api/containers/web/fs/upload?dir=/etc", b.Bytes(), c, map[string]string{"Content-Type": mw.FormDataContentType(), "If-None-Match": "*"})
	if rec.Code != http.StatusBadRequest || errCode(t, rec) != "invalid_name" {
		t.Fatalf("deux champs file : %d %s", rec.Code, rec.Body)
	}
	if n := len(f.docker.PutCalls()); n != 0 {
		t.Fatalf("PutArchive appelé %d fois", n)
	}
	if n := f.auditWrites(t); n != 0 {
		t.Fatalf("%d ligne(s) file.write", n)
	}
}

// A write that outlives the guard's timeout says it may still succeed.
func TestWriteTimeoutDetail(t *testing.T) {
	f, c := writeFixture(t, false)
	block := make(chan struct{})
	defer close(block)
	s := &server{
		d:     Deps{Docker: f.docker},
		guard: newFSGuard(1, 20*time.Millisecond),
		files: &files.Writer{Docker: f.docker, Open: func(ctx context.Context, id string) (fsview.FS, error) {
			<-block
			return nil, errors.New("unreachable")
		}},
	}
	_ = c
	r := httptest.NewRequest("PUT", "/api/containers/web/fs/file?path=/etc/app.conf", strings.NewReader("x"))
	r.SetPathValue("id", "web")
	r.Header.Set("If-Match", files.ETag([]byte("port=80\n")))
	rec := httptest.NewRecorder()
	s.fsEdit(rec, r)
	if rec.Code != http.StatusGatewayTimeout || !strings.Contains(rec.Body.String(), "may still complete") {
		t.Fatalf("délai de la garde : %d %s", rec.Code, rec.Body)
	}
}
