package api

// Tests of the bounded count and of the "Files" figure (spec v0.2 §5.3,
// §5.7), written before the code.

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

type countJSON struct {
	Count  int    `json:"count"`
	Source string `json:"source"`
	At     string `json:"at"`
}

// count polls GET /count until it is not 202.
func (sf *snapFixture) count(t *testing.T, c *http.Cookie, ref string) (int, countJSON, string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		rec := sf.do("GET", "/api/containers/"+ref+"/count", "", c)
		if rec.Code != http.StatusAccepted || time.Now().After(deadline) {
			var out countJSON
			json.Unmarshal(rec.Body.Bytes(), &out)
			return rec.Code, out, rec.Body.String()
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCountRunning(t *testing.T) {
	sf, c := snapSetup(t, false)
	code, out, body := sf.count(t, c, "web")
	if code != http.StatusOK || out.Count != 4 || out.Source != "count" || out.At == "" {
		t.Fatalf("comptage : %d %s", code, body)
	}
	if n := sf.walker.read.Load(); n != 0 {
		t.Fatalf("%d octets lus pour un comptage", n)
	}
	// Kept 5 min: no second walk.
	walks := sf.walker.walks.Load()
	if code, out, _ := sf.count(t, c, "web"); code != http.StatusOK || out.Count != 4 || sf.walker.walks.Load() != walks {
		t.Fatalf("second comptage : %d, %d parcours (avant %d)", code, sf.walker.walks.Load(), walks)
	}
	if got := sf.files(t); len(got) != 0 {
		t.Fatalf("le comptage a écrit : %v", got)
	}
}

func TestCountStopped(t *testing.T) {
	sf, c := snapSetup(t, false)
	if code, out, body := sf.count(t, c, "db"); code != http.StatusOK || out.Count != 3 {
		t.Fatalf("comptage : %d %s", code, body)
	}
}

func TestCountAccepted202(t *testing.T) {
	sf, c := snapSetup(t, false)
	sf.walker.gate = make(chan struct{})
	rec := sf.do("GET", "/api/containers/web/count", "", c)
	var body indexingJSON
	if rec.Code != http.StatusAccepted || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.Error != "indexing" {
		t.Fatalf("comptage : %d %s", rec.Code, rec.Body)
	}
	// A count is the one heavy job of the agent: a snapshot waits.
	if rec := sf.do("POST", "/api/containers/db/snapshots", "", c); rec.Code != http.StatusConflict {
		t.Fatalf("prise pendant le comptage : %d %s", rec.Code, rec.Body)
	}
	close(sf.walker.gate)
	if code, out, body := sf.count(t, c, "web"); code != http.StatusOK || out.Count != 4 {
		t.Fatalf("comptage : %d %s", code, body)
	}
}

func TestCountForbiddenBind(t *testing.T) {
	fake := snapFake(t)
	db := fake.Infos[dbID]
	db.Mounts = []dockerx.Mount{{Type: "bind", Source: "/proc", Destination: "/host"}}
	fake.Infos["db"], fake.Infos[dbID] = db, db
	sf := newSnapFixture(t, false, fake, fake)
	c := sf.session(t)
	if code, _, body := sf.count(t, c, "db"); code != http.StatusForbidden {
		t.Fatalf("bind de /proc : %d %s", code, body)
	}
	if calls := fake.Calls(); len(calls) != 0 {
		t.Fatalf("GetArchive appelé : %v", calls)
	}
}

// Spec v0.2 §5.3: index already built, else the last snapshot, else the
// count, else null.
func TestFiguresFilesSources(t *testing.T) {
	sf, c := snapSetup(t, false)
	figures := func() *filesJSON {
		t.Helper()
		rec := sf.do("GET", "/api/containers/web/figures", "", c)
		var out figuresJSON
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &out) != nil {
			t.Fatalf("figures : %d %s", rec.Code, rec.Body)
		}
		return out.Files
	}
	if f := figures(); f != nil {
		t.Fatalf("avant tout : %+v", f)
	}
	sf.count(t, c, "web")
	if f := figures(); f == nil || f.Source != "count" || f.Count != 4 {
		t.Fatalf("après le comptage : %+v", f)
	}
	sf.walker.MemFS.AddFile("/etc/new", nil)
	sf.do("POST", "/api/containers/web/snapshots", "", c)
	sf.idle(t, c)
	if f := figures(); f == nil || f.Source != "snapshot" || f.Count != 5 || f.At == "" {
		t.Fatalf("après la prise : %+v", f)
	}
}
