package api

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

// escape encodes a query value (spaces, newlines, '/' and all).
func escape(s string) string { return url.QueryEscape(s) }

func filename(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	disp, params, err := mime.ParseMediaType(rec.Header().Get("Content-Disposition"))
	if err != nil || disp != "attachment" {
		t.Fatalf("Content-Disposition %q : %v", rec.Header().Get("Content-Disposition"), err)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Fatalf("Content-Type %q", ct)
	}
	return params["filename"]
}

func TestDownloadFile(t *testing.T) {
	f := newFixture(t, false)
	f.fs.AddDir("/etc").AddFile("/etc/hosts", []byte("127.0.0.1"))
	rec := f.do("GET", "/api/containers/web/fs/download?path=/etc/hosts", "", f.session(t))
	if rec.Code != http.StatusOK || rec.Body.String() != "127.0.0.1" || filename(t, rec) != "hosts" {
		t.Fatalf("%d %v %q", rec.Code, rec.Header(), rec.Body)
	}
	if f.fs.Closed.Load() != 1 || len(f.docker.ArchiveCalls) != 0 {
		t.Fatalf("fermetures %d, appels archive %v", f.fs.Closed.Load(), f.docker.ArchiveCalls)
	}
}

func TestDownloadOddFileName(t *testing.T) {
	f := newFixture(t, false)
	c := f.session(t)
	for _, name := range []string{"rapport final.txt", "été-🎉.log", "a\"b;c.txt", "new\nline"} {
		f.fs.AddFile("/"+name, []byte("x"))
		rec := f.do("GET", "/api/containers/web/fs/download?path="+escape(name), "", c)
		if rec.Code != http.StatusOK || filename(t, rec) != name {
			t.Errorf("%q : %d %q", name, rec.Code, rec.Header().Get("Content-Disposition"))
		}
	}
}

func TestDownloadDirectoryAsTar(t *testing.T) {
	f := newFixture(t, false)
	f.fs.AddDir("/etc")
	f.docker.Archives[webID+":/etc"] = []byte("TAR")
	rec := f.do("GET", "/api/containers/web/fs/download?path=/etc/", "", f.session(t))
	if rec.Code != http.StatusOK || rec.Body.String() != "TAR" || filename(t, rec) != "etc.tar" {
		t.Fatalf("%d %v %q", rec.Code, rec.Header(), rec.Body)
	}
	// Exactly the cleaned path, with the full ID (never the name "web").
	if len(f.docker.ArchiveCalls) != 1 || f.docker.ArchiveCalls[0] != webID+":/etc" {
		t.Fatalf("appels archive : %v", f.docker.ArchiveCalls)
	}
	f.docker.Archives[webID+":/"] = []byte("ROOT")
	if rec := f.do("GET", "/api/containers/web/fs/download?path=/", "", f.session(t)); filename(t, rec) != "rootfs.tar" {
		t.Fatalf("racine : %q", rec.Header().Get("Content-Disposition"))
	}
}

func TestDownloadDirectoryThroughLink(t *testing.T) {
	f := newFixture(t, false)
	f.fs.AddLink("/link", "/etc").AddDir("/link/sub")
	rec := f.do("GET", "/api/containers/web/fs/download?path=/link/sub", "", f.session(t))
	if rec.Code != http.StatusUnprocessableEntity || errCode(t, rec) != "link_in_path" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if len(f.docker.ArchiveCalls) != 0 {
		t.Fatalf("GetArchive ne doit pas être appelé : %v", f.docker.ArchiveCalls)
	}
}

func TestDownloadNotRegular(t *testing.T) {
	f := newFixture(t, false)
	f.fs.AddLink("/l", "/etc/hosts") // MemFS.Stat does not follow: a "symlink" entry is not regular
	rec := f.do("GET", "/api/containers/web/fs/download?path=/l", "", f.session(t))
	if rec.Code != http.StatusUnprocessableEntity || errCode(t, rec) != "not_regular" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

// Review ruling C1: a tar that would cross a host bind, a pseudo filesystem
// or a FUSE mount is refused before any GetArchive (the real check, through
// the real view: fs_download_linux_test.go).
func TestDownloadDirectoryRefusedByMountCheck(t *testing.T) {
	f := newFixture(t, false)
	f.fs.AddDir("/host")
	f.fs.ArchiveErr = fmt.Errorf("%w : bind /proc -> /host/proc", fsview.ErrForbiddenPath)
	c := f.session(t)
	for _, q := range []string{"", "&check=1"} {
		rec := f.do("GET", "/api/containers/web/fs/download?path=/host"+q, "", c)
		var body apiError
		json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != http.StatusForbidden || body.Error != "forbidden_path" ||
			!strings.Contains(body.Detail, "bind /proc -> /host/proc") {
			t.Errorf("%q : %d %s", q, rec.Code, rec.Body)
		}
	}
	if len(f.docker.ArchiveCalls) != 0 {
		t.Fatalf("GetArchive ne doit jamais être appelé : %v", f.docker.ArchiveCalls)
	}
}

// Review ruling M2: check=1 runs every check and stops before GetArchive.
func TestDownloadCheckMode(t *testing.T) {
	f := newFixture(t, false)
	f.fs.AddDir("/etc").AddFile("/etc/hosts", []byte("x")).AddLink("/l", "/etc").AddDir("/l/sub")
	c := f.session(t)
	for _, p := range []string{"/etc", "/etc/hosts"} {
		if rec := f.do("GET", "/api/containers/web/fs/download?check=1&path="+p, "", c); rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
			t.Errorf("%s : %d %q", p, rec.Code, rec.Body)
		}
	}
	if rec := f.do("GET", "/api/containers/web/fs/download?check=1&path=/l/sub", "", c); errCode(t, rec) != "link_in_path" {
		t.Errorf("lien : %d %s", rec.Code, rec.Body)
	}
	if len(f.docker.ArchiveCalls) != 0 {
		t.Fatalf("le contrôle ne doit pas lancer GetArchive : %v", f.docker.ArchiveCalls)
	}
}

// stuckReader is a file of a FUSE mount that never answers: its first Read
// says so on started, then waits for unblock.
type stuckReader struct {
	once             sync.Once
	started, unblock chan struct{}
}

func (r *stuckReader) Read([]byte) (int, error) {
	r.once.Do(func() { r.started <- struct{}{} })
	<-r.unblock
	return 0, io.EOF
}

func (r *stuckReader) Close() error { return nil }

// Review rulings (Codex 4, I3, M3): a download keeps its guard slot during the
// whole copy; the budget is per full ID (name and ID share it); the view is
// closed as soon as the file is open.
func TestDownloadKeepsItsSlotDuringTheCopy(t *testing.T) {
	f := newFixture(t, false)
	started, unblock := make(chan struct{}, fsMaxPerContainer), make(chan struct{})
	f.fs.AddFile("/stuck", nil)
	f.fs.OpenFile = func(string) (io.ReadCloser, error) {
		return &stuckReader{started: started, unblock: unblock}, nil
	}
	c := f.session(t)
	var wg sync.WaitGroup
	for range fsMaxPerContainer {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f.do("GET", "/api/containers/web/fs/download?path=/stuck", "", c)
		}()
	}
	for range fsMaxPerContainer {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("les téléchargements n'ont pas commencé")
		}
	}
	if n := f.fs.Closed.Load(); n != fsMaxPerContainer {
		t.Errorf("vues fermées pendant la copie : %d, attendu %d", n, fsMaxPerContainer)
	}
	rec := f.do("GET", "/api/containers/"+webID+"/fs?path=/", "", c)
	if rec.Code != http.StatusServiceUnavailable || errCode(t, rec) != "internal" {
		t.Errorf("budget plein (par l'ID complet) : %d %s", rec.Code, rec.Body)
	}
	close(unblock)
	wg.Wait()
	if rec := f.do("GET", "/api/containers/web/fs?path=/", "", c); rec.Code != http.StatusOK {
		t.Fatalf("créneaux non rendus après la copie : %d %s", rec.Code, rec.Body)
	}
}
