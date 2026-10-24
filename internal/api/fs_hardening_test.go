package api

// Two holes of v0.1 found while applying v0.1.2 to v0.2: the preview of a
// container's file had no write deadline, and writeFSError sent the text of
// an unclassified error (paths of the host, internal errors) to the client.

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// An error that nothing classifies: its text (here a path of the host and
// a DOCKER_HOST password) never reaches the client, on any route that goes
// through writeFSError; the agent's log keeps it, masked.
func TestUnclassifiedErrorGivesFixedDetail(t *testing.T) {
	logs := captureLogs(t)
	f := newFixture(t, false)
	c := f.session(t)
	f.openErr = errors.New("open /proc/4242/root: operation not permitted (tcp://alice:" + dockerSecret + "@10.0.0.1:2375)")
	for _, p := range []string{"/api/containers/web/fs?path=/", "/api/containers/web/fs/file?path=/x",
		"/api/containers/web/fs/download?path=/x"} {
		rec := f.do("GET", p, "", c)
		if b := rec.Body.String(); rec.Code != http.StatusInternalServerError || strings.Contains(b, "/proc/4242") || strings.Contains(b, dockerSecret) {
			t.Errorf("%s : %d %s", p, rec.Code, b)
		}
	}
	if l := logs.String(); !strings.Contains(l, "/proc/4242") || strings.Contains(l, dockerSecret) {
		t.Errorf("journal de l'agent : %s", l)
	}
}

// The details of classified errors stay: they are the agent's own text.
func TestClassifiedErrorKeepsDetail(t *testing.T) {
	f := newFixture(t, false)
	c := f.session(t)
	f.fs.AddFile("/big", bytes.Repeat([]byte("a"), previewMax+1))
	if rec := f.do("GET", "/api/containers/web/fs/file?path=/big", "", c); rec.Code != http.StatusRequestEntityTooLarge ||
		!strings.Contains(rec.Body.String(), fmt.Sprint(previewMax)) {
		t.Errorf("413 : %d %s", rec.Code, rec.Body)
	}
}

// v0.1.2 (counter-audit, constat 1) on the preview of a container's file:
// a client that stopped reading is cut within writeStallTimeout.
func TestStalledPreviewIsCut(t *testing.T) {
	stallWrites(t, 300*time.Millisecond)
	f := newFixture(t, false)
	c := f.session(t)
	f.fs.AddFile("/big.txt", bytes.Repeat([]byte("t"), previewMax))
	addr := f.serve(t, 5*time.Second)
	conn := stalledGet(t, addr, "/api/containers/web/fs/file?path=/big.txt", c)
	time.Sleep(1500 * time.Millisecond)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, _ := bytes.NewBuffer(nil).ReadFrom(conn)
	if n+512 >= previewMax {
		t.Errorf("aperçu envoyé en entier (%d octets) à un client qui ne lisait plus", n+512)
	}
}

// A slow client that reads goes on (slowClient, v02_hardening_test.go).
func TestSlowPreviewGoesOn(t *testing.T) {
	stallWrites(t, 300*time.Millisecond)
	f := newFixture(t, false)
	c := f.session(t)
	f.fs.AddFile("/big.txt", bytes.Repeat([]byte("t"), previewMax))
	r := httptest.NewRequest("GET", "http://example.com/api/containers/web/fs/file?path=/big.txt", nil)
	r.AddCookie(c)
	w := &slowClient{ResponseRecorder: httptest.NewRecorder(), rate: time.Second / (1 << 20)}
	func() {
		defer func() {
			if v := recover(); v != nil {
				t.Errorf("client lent coupé après %d octets : %v", w.Body.Len(), v)
			}
		}()
		f.h.ServeHTTP(w, r)
	}()
	if w.Body.Len() != previewMax {
		t.Errorf("client lent : %d octets sur %d", w.Body.Len(), previewMax)
	}
}

// Without a write deadline, the preview sends nothing (fail closed).
func TestPreviewNoWriteDeadline(t *testing.T) {
	f := newFixture(t, false)
	c := f.session(t)
	f.fs.AddFile("/note.txt", []byte("contenu-secret"))
	r := httptest.NewRequest("GET", "http://example.com/api/containers/web/fs/file?path=/note.txt", nil)
	r.AddCookie(c)
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, r)
	if strings.Contains(rec.Body.String(), "contenu-secret") {
		t.Errorf("aperçu envoyé sans délai d'écriture : %d %q", rec.Code, rec.Body)
	}
}
