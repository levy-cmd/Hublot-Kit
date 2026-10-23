//go:build integration

package api

// v0.1.1, found on Docker Desktop 29.7.2 with a real stack: dockerd holds the
// container lock for the whole GET /archive of "/", and an Inspect of that
// container waits for it. Polls during the build must not Inspect: 202 at
// once, never 503 docker_unavailable after 10 s (spec §5.2 step 2).

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// pausedExport is the real client, except that the agent stops reading the
// tar of "/" after free bytes until open is closed. dockerd then blocks on
// its write, the container lock held: the state of a long export, made
// deterministic.
type pausedExport struct {
	dockerx.Client
	open chan struct{}
	once sync.Once
	free int
}

func (p *pausedExport) release() { p.once.Do(func() { close(p.open) }) }

func (p *pausedExport) GetArchive(ctx context.Context, id, path string) (io.ReadCloser, error) {
	rc, err := p.Client.GetArchive(ctx, id, path)
	if err != nil || path != "/" {
		return rc, err
	}
	return &pausedReader{ctx: ctx, p: p, rc: rc, free: p.free}, nil
}

type pausedReader struct {
	ctx  context.Context
	p    *pausedExport
	rc   io.ReadCloser
	free int
}

func (r *pausedReader) Read(b []byte) (int, error) {
	if r.free > 0 {
		n, err := r.rc.Read(b[:min(len(b), r.free)])
		r.free -= n
		return n, err
	}
	select {
	case <-r.p.open:
		return r.rc.Read(b)
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	}
}

func (r *pausedReader) Close() error { return r.rc.Close() }

func TestIntegrationStoppedPollsDuringExport(t *testing.T) {
	name := "hublot-it-api-export-lock"
	exec.Command("docker", "rm", "-f", name).Run()
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })
	// 64 Mio: far more than the socket buffers, so dockerd stays mid-write.
	if out, err := exec.Command("docker", "run", "--name", name, "alpine:3",
		"dd", "if=/dev/zero", "of=/big", "bs=1M", "count=64").CombinedOutput(); err != nil {
		t.Fatalf("docker run : %v %s", err, out)
	}
	dc, err := dockerx.New()
	if err != nil {
		t.Fatal(err)
	}
	p := &pausedExport{Client: dc, open: make(chan struct{}), free: 1 << 20}
	t.Cleanup(p.release)
	f := &fixture{}
	f.init(t, false, p, nil) // real Views, DefaultOptions (FirstWait 1 s)
	c := f.session(t)
	base := "/api/containers/" + name

	if rec := f.do("GET", base+"/fs?path=/", "", c); rec.Code != http.StatusAccepted {
		t.Fatalf("première requête : %d %s, attendu 202 (export en pause)", rec.Code, rec.Body)
	}
	// Control: the export really holds the container.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	start := time.Now()
	_, err = dc.Inspect(ctx, name)
	cancel()
	if err == nil {
		t.Fatalf("témoin : Inspect a répondu en %s pendant l'export, le verrou de dockerd n'est pas reproduit", time.Since(start))
	}

	var progress float64
	for _, u := range []string{"/fs?path=/", "/fs/file?path=/etc/hostname", "/fs/download?path=/etc&check=1"} {
		start := time.Now()
		rec := f.do("GET", base+u, "", c)
		var body struct {
			Error    string  `json:"error"`
			Progress float64 `json:"progress"`
		}
		json.Unmarshal(rec.Body.Bytes(), &body)
		if dt := time.Since(start); rec.Code != http.StatusAccepted || body.Error != "indexing" || dt > time.Second {
			t.Errorf("relance %s pendant l'export : %d en %s %s, attendu 202 en moins d'1 s", u, rec.Code, dt, rec.Body)
		}
		progress = body.Progress
	}
	if progress <= 0 || progress > 0.99 {
		t.Errorf("progression %v pendant l'export : SizeRootFs doit précéder l'export", progress)
	}

	p.release()
	rec := f.do("GET", base+"/fs?path=/", "", c)
	for deadline := time.Now().Add(60 * time.Second); rec.Code == http.StatusAccepted && time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		rec = f.do("GET", base+"/fs?path=/", "", c)
	}
	l := decodeListing(t, rec)
	found := false
	for _, e := range l.Entries {
		found = found || (e.Name == "big" && e.Size == 64<<20)
	}
	if !found {
		t.Errorf("/big absent du listing : %+v", l.Entries)
	}
	if rec := f.do("GET", base+"/fs?path=/absent", "", c); rec.Code != http.StatusNotFound || errCode(t, rec) != "not_found" {
		t.Errorf("chemin absent : %d %s", rec.Code, rec.Body)
	}
}
