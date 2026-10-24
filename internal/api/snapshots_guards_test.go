package api

// Three guards of the snapshot routes that no test held (review of the
// whole v0.2 branch): the timed write of a comparison and its container
// slot, the detail of a failed capture, the project of a header.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/snapshot"
)

// stuckClient: a client that stopped reading. Its first write waits for
// the write deadline, then fails; entered is closed when it starts waiting.
type stuckClient struct {
	*httptest.ResponseRecorder
	deadline time.Time
	entered  chan struct{}
}

func (s *stuckClient) SetWriteDeadline(t time.Time) error { s.deadline = t; return nil }

func (s *stuckClient) Write([]byte) (int, error) {
	select {
	case <-s.entered:
	default:
		close(s.entered)
	}
	time.Sleep(time.Until(s.deadline))
	return 0, os.ErrDeadlineExceeded
}

// bigComparison: two snapshots of web whose comparison lists n additions.
func bigComparison(t *testing.T, sf *snapFixture, n int) string {
	t.Helper()
	key, keyID := sf.key(t)
	s1 := sf.putSnap(t, webID, "", keyID, cmpDir("/srv"))
	es := []snapshot.Entry{cmpDir("/srv")}
	for i := range n {
		es = append(es, cmpFile(fmt.Sprintf("/srv/file-%05d.txt", i), hmacHex(key, "x"), 1))
	}
	s2 := sf.putSnap(t, webID, "", keyID, es...)
	return "/api/snapshots/" + s1 + "/compare?with=" + s2
}

// A client that stops reading a comparison does not hold the container's
// slot (spec §13: 4 operations at most) while its write waits: the slot is
// given back before the answer is written, as by the previews.
func TestCompareStalledClientFreesSlot(t *testing.T) {
	stallWrites(t, 2*time.Second)
	sf, c := snapSetup(t, false)
	path := bigComparison(t, sf, 50)
	r := httptest.NewRequest("GET", "http://example.com"+path, nil)
	r.AddCookie(c)
	w := &stuckClient{ResponseRecorder: httptest.NewRecorder(), entered: make(chan struct{})}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { recover() }() // http.ErrAbortHandler, as net/http would catch it
		sf.h.ServeHTTP(w, r)
	}()
	select {
	case <-w.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("la réponse ne s'écrit jamais")
	}
	var held []func()
	for range fsMaxPerContainer {
		release, ok := sf.srv.guard.take(webID)
		if !ok {
			t.Errorf("créneau de %s tenu pendant l'écriture bloquée", webID[:12])
			break
		}
		held = append(held, release)
	}
	for _, release := range held {
		release()
	}
	<-done
}

// A client that reads slowly gets the whole comparison: the write deadline
// slides with each chunk (writeTimed), the one set before the work would cut
// it. 10 000 results at 1 MiB/s take well over writeStallTimeout.
func TestCompareSlowClientGetsAll(t *testing.T) {
	stallWrites(t, 300*time.Millisecond)
	sf, c := snapSetup(t, false)
	path := bigComparison(t, sf, maxCompareResults)
	r := httptest.NewRequest("GET", "http://example.com"+path, nil)
	r.AddCookie(c)
	w := &slowClient{ResponseRecorder: httptest.NewRecorder(), rate: time.Second / (1 << 20)}
	func() {
		defer func() {
			if v := recover(); v != nil {
				t.Errorf("client lent coupé après %d octets : %v", w.Body.Len(), v)
			}
		}()
		sf.h.ServeHTTP(w, r)
	}()
	var out compareJSON
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || len(out.Results) != maxCompareResults {
		t.Fatalf("comparaison incomplète (%d octets) : %v, %d résultats", w.Body.Len(), err, len(out.Results))
	}
}

// failingArchive: an export that fails with an error of the daemon whose
// text names a credential (DOCKER_HOST or a proxy with a password).
type failingArchive struct{ *dockerx.Fake }

func (d failingArchive) GetArchive(context.Context, string, string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("%w: error during connect: Get \"http://docker/archive\": proxyconnect tcp://admin:s3cr3t-pw@proxy:3128 refused, password=s3cr3t-pw",
		dockerx.ErrUnavailable)
}

// A capture that fails in the background never leaves the text of Docker's
// error in the list (last.detail) nor in the audit log.
func TestSnapshotFailureDetailMasked(t *testing.T) {
	fake := snapFake(t)
	sf := newSnapFixture(t, false, failingArchive{fake}, fake)
	c := sf.session(t)
	if rec := sf.do("POST", "/api/containers/db/snapshots", "", c); rec.Code != http.StatusAccepted {
		t.Fatalf("prise : %d %s", rec.Code, rec.Body)
	}
	l, body := sf.idle(t, c)
	if l.Last == nil || l.Last["result"] != "docker_unavailable" {
		t.Fatalf("dernier résultat : %s", body)
	}
	if strings.Contains(body, "s3cr3t-pw") {
		t.Errorf("liste : %s", body)
	}
	log, _ := os.ReadFile(sf.auditP)
	if !strings.Contains(string(log), `"action":"snapshot.create"`) || strings.Contains(string(log), "s3cr3t-pw") {
		t.Errorf("journal : %s", log)
	}
}

// The project of a header follows SafeLabel: a com.docker.compose.project
// label set by hand with a secret is not kept in clear.
func TestSnapshotProjectMasked(t *testing.T) {
	fake := snapFake(t)
	web := fake.Infos[webID]
	web.Details.Labels = map[string]string{"com.docker.compose.project": "https://admin:s3cr3t-proj@registry.example"}
	fake.Infos["web"], fake.Infos[webID] = web, web
	sf := newSnapFixture(t, false, fake, fake)
	c := sf.session(t)
	sid := sf.takeWeb(t, c)
	_, list := sf.list(t, c)
	rec, _ := sf.poll(t, "/api/snapshots/"+sid+"/compare?with=current", c)
	for name, body := range map[string]string{"liste": list, "comparaison": rec.Body.String()} {
		if strings.Contains(body, "s3cr3t-proj") || strings.Contains(body, `"project"`) {
			t.Errorf("%s : %s", name, body)
		}
	}
}
