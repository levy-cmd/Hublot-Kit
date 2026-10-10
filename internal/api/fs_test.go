package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

// session logs in and returns the cookie for the file routes.
func (f *fixture) session(t *testing.T) *http.Cookie {
	t.Helper()
	_, c := f.login(t, f.token)
	if c == nil {
		t.Fatal("connexion impossible")
	}
	return c
}

func decodeListing(t *testing.T, rec *httptest.ResponseRecorder) listingJSON {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("statut %d : %s", rec.Code, rec.Body)
	}
	var l listingJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &l); err != nil {
		t.Fatal(err)
	}
	return l
}

func TestFSListNeedsSession(t *testing.T) {
	f := newFixture(t, false)
	if rec := f.do("GET", "/api/containers/web/fs?path=/", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("statut %d", rec.Code)
	}
}

func TestFSList(t *testing.T) {
	f := newFixture(t, false)
	f.fs.AddDir("/etc").AddFile("/etc/hosts", []byte("127.0.0.1")).AddLink("/etc/os-release", "../usr/lib/os-release")
	f.fs.Entries["/etc/hosts"] = fsview.Entry{Name: "hosts", Type: fsview.TypeFile, Size: 9, Mode: fs.ModeSetuid | 0o644, UID: 0, GID: 101}
	l := decodeListing(t, f.do("GET", "/api/containers/web/fs?path=/etc", "", f.session(t)))
	if l.Path != "/etc" || l.Type != "dir" || l.Truncated || len(l.Entries) != 2 {
		t.Fatalf("listing : %+v", l)
	}
	if e := l.Entries[0]; e.Name != "hosts" || e.Type != "file" || e.Size != 9 || e.Mode != 0o4644 || e.GID != 101 {
		t.Fatalf("hosts : %+v", e)
	}
	if e := l.Entries[1]; e.Name != "os-release" || e.Type != "symlink" || e.LinkTarget != "../usr/lib/os-release" {
		t.Fatalf("lien : %+v", e)
	}
	if f.opened() != webID {
		t.Fatalf("la vue doit s'ouvrir sur l'ID complet, pas sur le nom : %q", f.opened())
	}
	if f.fs.Closed.Load() != 1 {
		t.Fatalf("la vue doit être fermée une fois, fermée %d fois", f.fs.Closed.Load())
	}
}

func TestFSListCleansPath(t *testing.T) {
	f := newFixture(t, false)
	f.fs.AddDir("/etc")
	c := f.session(t)
	for _, raw := range []string{"etc", "/etc/", "//etc", "/etc/../etc", "../../etc", "%2Fetc"} {
		if l := decodeListing(t, f.do("GET", "/api/containers/web/fs?path="+raw, "", c)); l.Path != "/etc" {
			t.Errorf("%q → %q", raw, l.Path)
		}
	}
	if l := decodeListing(t, f.do("GET", "/api/containers/web/fs", "", c)); l.Path != "/" || l.Entries == nil {
		t.Errorf("sans path → %+v", l)
	}
}

func TestFSListOfAFile(t *testing.T) {
	f := newFixture(t, false)
	f.fs.AddDir("/etc").AddFile("/etc/hosts", []byte("x"))
	l := decodeListing(t, f.do("GET", "/api/containers/web/fs?path=/etc/hosts", "", f.session(t)))
	if l.Type != "file" || len(l.Entries) != 0 || l.Path != "/etc/hosts" {
		t.Fatalf("listing d'un fichier : %+v", l)
	}
}

func TestFSListErrors(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{fsview.ErrForbiddenPath, http.StatusForbidden, "forbidden_path"},
		{fmt.Errorf("%w : pid 7", fsview.ErrContainerChanged), http.StatusConflict, "container_changed"},
		{fs.ErrNotExist, http.StatusNotFound, "not_found"},
	}
	for _, c := range cases {
		f := newFixture(t, false)
		f.fs.Err = c.err
		rec := f.do("GET", "/api/containers/web/fs?path=/x", "", f.session(t))
		if rec.Code != c.status || errCode(t, rec) != c.code {
			t.Errorf("%v : %d %s", c.err, rec.Code, rec.Body)
		}
	}
	f := newFixture(t, false)
	if rec := f.do("GET", "/api/containers/nope/fs?path=/", "", f.session(t)); rec.Code != http.StatusNotFound || errCode(t, rec) != "not_found" {
		t.Errorf("conteneur inconnu : %d %s", rec.Code, rec.Body)
	}
	f = newFixture(t, false)
	c := f.session(t)
	f.docker.Err = fmt.Errorf("%w : dial", dockerx.ErrUnavailable)
	if rec := f.do("GET", "/api/containers/web/fs?path=/", "", c); rec.Code != http.StatusServiceUnavailable || errCode(t, rec) != "docker_unavailable" {
		t.Errorf("Docker arrêté : %d %s", rec.Code, rec.Body)
	}
}

func TestFSBadContainerRef(t *testing.T) {
	f := newFixture(t, false)
	c := f.session(t)
	for _, id := range []string{".hidden", "a%2Fb", "%2E%2E", "-x", "a%20b"} {
		rec := f.do("GET", "/api/containers/"+id+"/fs?path=/", "", c)
		if rec.Code != http.StatusNotFound || errCode(t, rec) != "not_found" {
			t.Errorf("%q : %d %s", id, rec.Code, rec.Body)
		}
	}
	// A literal ".." segment is cleaned by ServeMux itself (redirect).
	if rec := f.do("GET", "/api/containers/../fs?path=/", "", c); rec.Code == http.StatusOK {
		t.Errorf(".. : %d", rec.Code)
	}
	if f.opened() != "" {
		t.Fatalf("aucune vue ne doit s'ouvrir, ouverte pour %q", f.opened())
	}
}

func TestFSNULInPath(t *testing.T) {
	f := newFixture(t, false)
	rec := f.do("GET", "/api/containers/web/fs?path=/etc%00/passwd", "", f.session(t))
	if rec.Code != http.StatusNotFound || errCode(t, rec) != "not_found" || f.opened() != "" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}
