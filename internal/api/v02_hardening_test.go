package api

// The rules of v0.1.2 (security audit) applied to the routes of v0.2:
// Docker's text never reaches a response nor the agent's log, a long write
// has a sliding deadline (fail closed without one), and the 15 s read delay
// of a request cuts none of the v0.2 routes.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

const dockerSecret = "s3cr3t-pw"

var dockerMsg = "Cannot connect to the Docker daemon at tcp://alice:" + dockerSecret + "@10.0.0.1:2375. Is the docker daemon running?"

// captureLogs sends the agent's log, debug level included, to a buffer.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &logs
}

// v02Fixture: the image fsImg (defaultImage) and the container web.
func v02Fixture(t *testing.T, docker func(*dockerx.Fake) dockerx.Client) (*fixture, *dockerx.Fake, *http.Cookie) {
	t.Helper()
	archive := defaultImage(t)
	web := dockerx.ContainerInfo{ID: webID, Name: "web", Running: true, Pid: 42}
	fake := &dockerx.Fake{
		Containers:     []dockerx.Container{{ID: webID, Name: "web", Image: "nginx", State: "running", Ports: []dockerx.Port{}}},
		Infos:          map[string]dockerx.ContainerInfo{"web": web, webID: web},
		Images:         map[string]dockerx.ImageInfo{fsImg: {ID: fsImg, Size: int64(len(archive))}},
		ImageSummaries: []dockerx.ImageSummary{{ID: fsImg}},
		ImageSaves:     map[string][]byte{fsImg: archive},
		NoWrites:       true,
	}
	var client dockerx.Client = fake
	if docker != nil {
		client = docker(fake)
	}
	f := &fixture{docker: fake, views: fsview.NewViews(client, fsview.Options{})}
	f.init(t, false, client, nil)
	return f, fake, f.session(t)
}

// v02Requests: every route of v0.2 that asks Docker, directly or not.
var v02Requests = []struct{ method, path, body string }{
	{"GET", "/api/meta", ""},
	{"GET", "/api/images", ""},
	{"GET", "/api/images/" + fsImg, ""},
	{"GET", imgFS("?path=/etc"), ""},
	{"GET", imgFS("/file?path=/etc/os-release"), ""},
	{"POST", imgFS("/search"), `{"q":"os"}`},
	{"GET", "/api/containers/web/figures", ""},
	{"POST", "/api/containers/web/fs/search", `{"q":"os"}`},
	{"GET", "/api/secrets", ""},
}

func noDockerSecret(t *testing.T, what string, rec *httptest.ResponseRecorder) {
	t.Helper()
	if b := rec.Body.String(); strings.Contains(b, dockerSecret) || strings.Contains(b, "tcp://") || strings.Contains(b, "Cannot connect") {
		t.Errorf("%s : %d %s", what, rec.Code, b)
	}
}

// v0.1.2 (audit, constat 4) on the routes of v0.2: with
// DOCKER_HOST=tcp://user:password@…, the daemon's error names the password.
// Whatever its class, or none (an error the engine did not classify), it
// reaches neither a response nor the agent's log.
func TestV02DockerErrorsGiveFixedDetail(t *testing.T) {
	logs := captureLogs(t)
	f, fake, c := v02Fixture(t, nil)
	for _, err := range []error{
		errors.New(dockerMsg), // as the client gives it
		fmt.Errorf("%w: %s", dockerx.ErrUnavailable, dockerMsg),
		fmt.Errorf("%w: %s", dockerx.ErrNotFound, dockerMsg),
		fmt.Errorf("%w: %s", dockerx.ErrRefused, dockerMsg),
		fmt.Errorf("%w: %s", dockerx.ErrPutInvalid, dockerMsg),
		fmt.Errorf("%w: %s", dockerx.ErrNoLogs, dockerMsg),
	} {
		fake.Err = err
		for _, r := range v02Requests {
			noDockerSecret(t, r.method+" "+r.path, f.do(r.method, r.path, r.body, c))
		}
	}
	if strings.Contains(logs.String(), dockerSecret) {
		t.Errorf("journal de l'agent : %s", logs.String())
	}
}

// failingSave: the export of the image fails after its first bytes with a
// raw error of the connection (an error of the body, which the engine
// cannot classify), from the save number from on.
type failingSave struct {
	*dockerx.Fake
	from  int32
	saves atomic.Int32
}

type cutReader struct{ r io.Reader }

func (b cutReader) Read(p []byte) (int, error) {
	if n, _ := b.r.Read(p); n > 0 {
		return n, nil
	}
	return 0, errors.New(dockerMsg)
}

func (d *failingSave) ImageSave(ctx context.Context, id string) (io.ReadCloser, error) {
	rc, err := d.Fake.ImageSave(ctx, id)
	if err != nil || d.saves.Add(1) < d.from {
		return rc, err
	}
	return io.NopCloser(cutReader{io.LimitReader(rc, 1024)}), nil
}

// The export cut midway, in the index's build and in an extraction: its
// text reaches neither the answer nor the log.
func TestV02ImageExportCutGivesFixedDetail(t *testing.T) {
	for _, from := range []int32{1, 2} { // 1: the index; 2: the extraction
		logs := captureLogs(t)
		f, _, c := v02Fixture(t, func(fake *dockerx.Fake) dockerx.Client { return &failingSave{Fake: fake, from: from} })
		if from == 2 {
			if rec := until(t, f, "GET", imgFS("?path=/etc"), c); rec.Code != http.StatusOK {
				t.Fatalf("index : %d %s", rec.Code, rec.Body)
			}
		}
		for _, p := range []string{imgFS("?path=/etc"), imgFS("/file?path=/etc/os-release")} {
			rec := until(t, f, "GET", p, c)
			noDockerSecret(t, fmt.Sprintf("export coupé (%d) %s", from, p), rec)
		}
		if strings.Contains(logs.String(), dockerSecret) {
			t.Errorf("export coupé (%d), journal de l'agent : %s", from, logs.String())
		}
	}
}

// bigImage: an image with a text file of previewMax bytes, at /big.txt.
func bigImage(t *testing.T) []byte {
	return saveOf(t, gzLayer(t, tarItem{name: "big.txt", body: strings.Repeat("t", previewMax)}))
}

// v0.1.2 (counter-audit, constat 1) on the preview of a file of an image: a
// client that stopped reading is cut within writeStallTimeout, renewed by
// each write; the preview does not hang on it.
func TestV02StalledImagePreviewIsCut(t *testing.T) {
	stallWrites(t, 300*time.Millisecond)
	f, _, c := imageFSFixture(t, false, bigImage(t))
	path := imgFS("/file?path=/big.txt")
	if rec := until(t, f, "GET", path, c); rec.Code != http.StatusOK || rec.Body.Len() != previewMax {
		t.Fatalf("aperçu : %d, %d octets", rec.Code, rec.Body.Len())
	}
	addr := f.serve(t, 5*time.Second)
	conn := stalledGet(t, addr, path, c)
	time.Sleep(1500 * time.Millisecond) // past writeStallTimeout, the client reading nothing
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, _ := io.Copy(io.Discard, conn)
	if n+512 >= previewMax {
		t.Errorf("aperçu envoyé en entier (%d octets) à un client qui ne lisait plus", n+512)
	}
}

// A slow client that reads goes on: the deadline slides with each write.
func TestV02SlowImagePreviewGoesOn(t *testing.T) {
	stallWrites(t, 300*time.Millisecond)
	f, _, c := imageFSFixture(t, false, bigImage(t))
	path := imgFS("/file?path=/big.txt")
	until(t, f, "GET", path, c)
	addr := f.serve(t, 5*time.Second)
	conn := stalledGet(t, addr, path, c)
	got := 512
	buf := make([]byte, 64<<10)
	start := time.Now()
	for {
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, err := conn.Read(buf)
		got += n
		if err != nil {
			break
		}
		time.Sleep(10 * time.Millisecond) // about 2 MiB in more than writeStallTimeout
	}
	if got < previewMax || time.Since(start) < 300*time.Millisecond {
		t.Errorf("client lent : %d octets en %s", got, time.Since(start))
	}
}

// Without a write deadline, the preview of an image sends nothing (fail
// closed, v0.1.2 Codex). A plain recorder has none.
func TestV02ImagePreviewNoWriteDeadline(t *testing.T) {
	f, _, c := imageFSFixture(t, false, defaultImage(t))
	path := imgFS("/file?path=/etc/os-release")
	if rec := until(t, f, "GET", path, c); rec.Code != http.StatusOK {
		t.Fatalf("aperçu : %d %s", rec.Code, rec.Body)
	}
	r := httptest.NewRequest("GET", "http://example.com"+path, nil)
	r.AddCookie(c)
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, r)
	if strings.Contains(rec.Body.String(), "hublot-test") {
		t.Errorf("aperçu envoyé sans délai d'écriture : %d %q", rec.Code, rec.Body)
	}
}

// slowDockerV02 holds the Docker calls of the v0.2 routes for delay.
type slowDockerV02 struct {
	*dockerx.Fake
	delay time.Duration
}

func (d *slowDockerV02) sleep(ctx context.Context) error {
	select {
	case <-time.After(d.delay):
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%w: %v", dockerx.ErrUnavailable, ctx.Err())
	}
}

func (d *slowDockerV02) ImageList(ctx context.Context) ([]dockerx.ImageSummary, error) {
	if err := d.sleep(ctx); err != nil {
		return nil, err
	}
	return d.Fake.ImageList(ctx)
}

func (d *slowDockerV02) ImageInspect(ctx context.Context, id string) (dockerx.ImageInfo, error) {
	if err := d.sleep(ctx); err != nil {
		return dockerx.ImageInfo{}, err
	}
	return d.Fake.ImageInspect(ctx, id)
}

func (d *slowDockerV02) Inspect(ctx context.Context, id string) (dockerx.ContainerInfo, error) {
	if err := d.sleep(ctx); err != nil {
		return dockerx.ContainerInfo{}, err
	}
	return d.Fake.Inspect(ctx, id)
}

func (d *slowDockerV02) ContainerSize(ctx context.Context, id string) (rw, rootfs *int64, err error) {
	if err := d.sleep(ctx); err != nil {
		return nil, nil, err
	}
	return d.Fake.ContainerSize(ctx, id)
}

// v0.1.2 (audit, constat 2): the read delay of a request (15 s, here
// 200 ms) bounds its reading, never the wait of a v0.2 route for Docker
// (the figures wait up to 30 s, the inventory up to 20 s), with or without
// a body.
func TestV02RoutesOutliveReadTimeout(t *testing.T) {
	f, _, c := v02Fixture(t, func(fake *dockerx.Fake) dockerx.Client {
		return &slowDockerV02{Fake: fake, delay: 700 * time.Millisecond}
	})
	addr := f.serve(t, 200*time.Millisecond)
	for _, r := range v02Requests {
		if r.path == "/api/meta" {
			continue
		}
		path := r.path
		for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(50 * time.Millisecond) {
			req, _ := http.NewRequest(r.method, "http://"+addr+path, strings.NewReader(r.body))
			req.Header.Set("Origin", "http://"+addr)
			req.AddCookie(c)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("%s %s : %v", r.method, path, err)
				break
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusAccepted && time.Now().Before(deadline) {
				continue // the index of the image, or an extraction, in progress
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("%s %s : %d %s", r.method, path, resp.StatusCode, b)
			}
			break
		}
	}
}
