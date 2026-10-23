package api

// Tests of GET /api/images and GET /api/images/{id} (spec v0.2 §5.4, plan
// v0.2 tranche 5), written before the code. The history and the
// configuration of an image leave the agent masked (secrets.History,
// secrets.Env, secrets.Args, secrets.Labels); an image is named by its full
// ID only, never by a name, which may change target between two requests;
// an image without a tag stays listed and opens by its ID; the cost is
// bounded (one walk at a time, a deadline, a fixed detail on errors).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/secrets"
)

// imgHex is a full image ID (64 hex characters) made of n.
func imgHex(n int) string { return fmt.Sprintf("%064x", 0xabc000+n) }

var imageWitnesses = []string{"s3cr3t", "admin:"}

// The history lines of the plan (TestHistoryBuildArgs), plus an ENV.
var witnessHistory = []string{
	"|2 GH_PAT=s3cr3t-a NPM_AUTH=s3cr3t-b /bin/sh -c npm ci",
	"RUN |2 GH_PAT=s3cr3t-a NPM_AUTH=s3cr3t-b /bin/sh -c npm ci",
	"|1 NOTE=a b c /bin/bash -c make",
	"|1 X=s3cr3t-c make",
	`/bin/sh -c curl -H "Authorization: Bearer s3cr3t-d" x`,
	"ENV DB_PASSWORD=s3cr3t-env",
	// BuildKit, measured (tranche 5): the value of --build-arg, and the labels.
	"ARG GH_PAT=s3cr3t-arg",
	"LABEL traefik.http.middlewares.a.basicauth.users=admin:s3cr3t-lbl-hist",
}

// imagesFake: image 1 (tagged, two containers, every secret), image 2
// (untagged, one container), image 3 (untagged as old daemons say it).
func imagesFake() *dockerx.Fake {
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	id1, id2, id3 := "sha256:"+imgHex(1), "sha256:"+imgHex(2), "sha256:"+imgHex(3)
	f := &dockerx.Fake{
		Containers: []dockerx.Container{
			{ID: hexID(21), Name: "web-b", State: "running", ImageID: id1, Ports: []dockerx.Port{}},
			{ID: hexID(22), Name: "web-a", State: "exited", ImageID: id1, Ports: []dockerx.Port{}},
			{ID: hexID(23), Name: "orphan", State: "exited", ImageID: id2, Ports: []dockerx.Port{}},
			{ID: hexID(24), Name: "other", State: "running", ImageID: "sha256:" + imgHex(9), Ports: []dockerx.Port{}},
		},
		ImageSummaries: []dockerx.ImageSummary{
			{ID: id1, Tags: []string{"shop/app:1.0", "shop/app:latest"}, Size: 1000, Created: at},
			{ID: id2, Size: 500, Created: at.Add(-time.Hour)},
			{ID: id3, Tags: []string{"<none>:<none>"}, Size: 10, Created: at.Add(-2 * time.Hour)},
		},
		Images: map[string]dockerx.ImageInfo{
			id1: {
				ID: id1, Tags: []string{"shop/app:1.0", "shop/app:latest"}, Size: 1000, Created: "2026-10-01T12:00:00Z",
				OS: "linux", Architecture: "arm64", Variant: "v8",
				Env:          []string{"API_KEY=s3cr3t", "PATH=/usr/bin", "URL=postgres://u:s3cr3t-url@db"},
				Entrypoint:   []string{"sh", "-c", "curl -H 'Authorization: Bearer s3cr3t-ep' x"},
				Cmd:          []string{"redis-server", "--requirepass", "s3cr3t-arg"},
				WorkingDir:   "/app",
				User:         "app",
				ExposedPorts: []string{"80/tcp"},
				Labels: map[string]string{
					"traefik.http.middlewares.auth.basicauth.users": "admin:s3cr3t-lbl",
					"org.opencontainers.image.title":                "app",
				},
			},
			id2: {ID: id2, Size: 500, Created: "2026-10-01T11:00:00Z", OS: "linux", Architecture: "amd64"},
		},
		Histories: map[string][]dockerx.ImageLayer{
			id1: historyOf(witnessHistory, at),
			id2: {{CreatedBy: "/bin/sh -c #(nop) ADD file:abc in / ", Created: at, Size: 500}},
		},
	}
	return f
}

func historyOf(lines []string, at time.Time) []dockerx.ImageLayer {
	out := make([]dockerx.ImageLayer, len(lines))
	for i, l := range lines {
		out[i] = dockerx.ImageLayer{CreatedBy: l, Created: at, Size: int64(i * 100), Empty: i == 0}
	}
	return out
}

func imagesFixture(t *testing.T, docker dockerx.Client, fake *dockerx.Fake) (*fixture, *http.Cookie) {
	t.Helper()
	f := &fixture{docker: fake}
	f.init(t, false, docker, nil)
	return f, f.session(t)
}

type imageCtrRow struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

type imageRow struct {
	ID         string        `json:"id"`
	Tags       []string      `json:"tags"`
	Size       int64         `json:"size"`
	Created    string        `json:"created"`
	Containers []imageCtrRow `json:"containers"`
}

type layerRow struct {
	CreatedBy string `json:"createdBy"`
	Created   string `json:"created"`
	Size      int64  `json:"size"`
	Empty     bool   `json:"empty"`
}

type imageDetailRow struct {
	ID           string   `json:"id"`
	Tags         []string `json:"tags"`
	Size         int64    `json:"size"`
	Created      string   `json:"created"`
	OS           string   `json:"os"`
	Architecture string   `json:"architecture"`
	Variant      string   `json:"variant"`
	Config       struct {
		Env           []secrets.Var   `json:"env"`
		Cmd           []string        `json:"cmd"`
		Entrypoint    []string        `json:"entrypoint"`
		CommandMasked bool            `json:"commandMasked"`
		WorkingDir    string          `json:"workingDir"`
		User          string          `json:"user"`
		ExposedPorts  []string        `json:"exposedPorts"`
		Labels        []secrets.Label `json:"labels"`
	} `json:"config"`
	Layers     []layerRow    `json:"layers"`
	Containers []imageCtrRow `json:"containers"`
}

func noImageWitness(t *testing.T, body string) {
	t.Helper()
	for _, w := range imageWitnesses {
		if strings.Contains(body, w) {
			t.Errorf("valeur témoin %q dans la réponse : %s", w, body)
		}
	}
}

func decodeImages(t *testing.T, body []byte) map[string]imageRow {
	t.Helper()
	var rows []imageRow
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("réponse illisible : %v %s", err, body)
	}
	out := map[string]imageRow{}
	for _, r := range rows {
		out[r.ID] = r
	}
	return out
}

func getImage(t *testing.T, f *fixture, c *http.Cookie, id string) imageDetailRow {
	t.Helper()
	rec := f.do("GET", "/api/images/"+id, "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/images/%s : %d %s", id, rec.Code, rec.Body)
	}
	noImageWitness(t, rec.Body.String())
	var d imageDetailRow
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("réponse illisible : %v %s", err, rec.Body)
	}
	return d
}

// Spec v0.2 §5.4: every history line goes through secrets.History; the
// build arguments after "|N" are masked whatever their name.
func TestImageHistoryMasked(t *testing.T) {
	fake := imagesFake()
	f, c := imagesFixture(t, fake, fake)
	if rec := f.do("GET", "/api/images/"+imgHex(1), "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("sans session : %d", rec.Code)
	}
	d := getImage(t, f, c, imgHex(1))
	if len(d.Layers) != len(witnessHistory) {
		t.Fatalf("%d couches, attendu %d : %+v", len(d.Layers), len(witnessHistory), d.Layers)
	}
	for i, l := range d.Layers {
		if want := secrets.History(witnessHistory[i]); l.CreatedBy != want {
			t.Errorf("couche %d : %q, attendu %q", i, l.CreatedBy, want)
		}
		if l.Size != int64(i*100) || l.Empty != (i == 0) || l.Created != "2026-10-01T12:00:00Z" {
			t.Errorf("couche %d : %+v", i, l)
		}
	}
	if d.Layers[0].CreatedBy != "|2 GH_PAT=•••••• NPM_AUTH=•••••• /bin/sh -c npm ci" {
		t.Errorf("arguments de construction : %q", d.Layers[0].CreatedBy)
	}
}

// The configuration: env by secrets.Env (masked values null), entrypoint
// and cmd by secrets.Args, labels by secrets.Labels (whitelist).
func TestImageConfigMasked(t *testing.T) {
	fake := imagesFake()
	f, c := imagesFixture(t, fake, fake)
	d := getImage(t, f, c, imgHex(1))
	cfg := d.Config
	if !reflect.DeepEqual(cfg.Env, secrets.Env(fake.Images["sha256:"+imgHex(1)].Env)) || cfg.Env[0].Value != nil || cfg.Env[1].Value == nil {
		t.Errorf("env : %+v", cfg.Env)
	}
	ep, _ := secrets.Args(fake.Images["sha256:"+imgHex(1)].Entrypoint)
	cmd, _ := secrets.Args(fake.Images["sha256:"+imgHex(1)].Cmd)
	if !reflect.DeepEqual(cfg.Entrypoint, ep) || !reflect.DeepEqual(cfg.Cmd, cmd) || !cfg.CommandMasked {
		t.Errorf("commande : %q %q %v", cfg.Entrypoint, cfg.Cmd, cfg.CommandMasked)
	}
	if cfg.WorkingDir != "/app" || cfg.User != "app" || !reflect.DeepEqual(cfg.ExposedPorts, []string{"80/tcp"}) {
		t.Errorf("config : %+v", cfg)
	}
	title := "app"
	want := []secrets.Label{
		{Key: "org.opencontainers.image.title", Value: &title},
		{Key: "traefik.http.middlewares.auth.basicauth.users", Masked: true},
	}
	if !reflect.DeepEqual(cfg.Labels, want) {
		t.Errorf("labels : %+v", cfg.Labels)
	}
	if d.ID != "sha256:"+imgHex(1) || d.OS != "linux" || d.Architecture != "arm64" || d.Variant != "v8" || d.Size != 1000 ||
		d.Created != "2026-10-01T12:00:00Z" || !reflect.DeepEqual(d.Tags, []string{"shop/app:1.0", "shop/app:latest"}) {
		t.Errorf("détail : %+v", d)
	}
	// An image without configuration: arrays, never null.
	d2 := getImage(t, f, c, imgHex(2))
	if d2.Config.Env == nil || d2.Config.Cmd == nil || d2.Config.Entrypoint == nil || d2.Config.ExposedPorts == nil ||
		d2.Config.Labels == nil || d2.Config.CommandMasked || d2.Tags == nil {
		t.Errorf("image sans configuration : %+v", d2)
	}
}

// {id}: the full ID only, 64 lowercase hex characters, "sha256:" accepted.
// A name, a short ID, anything else: 404, and Docker is never asked.
func TestImageIDParam(t *testing.T) {
	fake := imagesFake()
	f, c := imagesFixture(t, fake, fake)
	for _, id := range []string{
		"nginx:latest", "nginx", "shop%2Fapp:1.0", imgHex(1)[:63], imgHex(1)[:12], imgHex(1) + "0",
		strings.ToUpper(imgHex(1)), "SHA256:" + imgHex(1), "sha512:" + imgHex(1), "sha256:" + imgHex(1)[:63],
		"sha256:sha256:" + imgHex(1), "sha256%3A" + imgHex(1)[:63], strings.Repeat("g", 64), "%20" + imgHex(1),
	} {
		rec := f.do("GET", "/api/images/"+id, "", c)
		if rec.Code != http.StatusNotFound || errCode(t, rec) != "not_found" {
			t.Errorf("%q : %d %s", id, rec.Code, rec.Body)
		}
	}
	if n := fake.ImageCalls.Load(); n != 0 {
		t.Errorf("%d appels à Docker pour des ID refusés", n)
	}
	for _, id := range []string{imgHex(1), "sha256:" + imgHex(1)} {
		if d := getImage(t, f, c, id); d.ID != "sha256:"+imgHex(1) {
			t.Errorf("%q : %+v", id, d)
		}
	}
	// A full ID Docker does not know: 404 with a fixed detail.
	rec := f.do("GET", "/api/images/"+imgHex(7), "", c)
	if rec.Code != http.StatusNotFound || errCode(t, rec) != "not_found" || strings.Contains(rec.Body.String(), imgHex(7)) {
		t.Errorf("image inconnue : %d %s", rec.Code, rec.Body)
	}
	// Docker answering another image than the one asked (a reference it
	// resolved otherwise): never shown.
	fake.Images["sha256:"+imgHex(8)] = dockerx.ImageInfo{ID: "sha256:" + imgHex(1)}
	if rec := f.do("GET", "/api/images/"+imgHex(8), "", c); rec.Code != http.StatusNotFound {
		t.Errorf("autre image : %d %s", rec.Code, rec.Body)
	}
}

// Spec v0.2 §5.4, plan Review Focus 5: an image without a tag stays listed
// (tags: []) and opens by its ID.
func TestImagesUntaggedKeptByID(t *testing.T) {
	fake := imagesFake()
	f, c := imagesFixture(t, fake, fake)
	if rec := f.do("GET", "/api/images", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("sans session : %d", rec.Code)
	}
	rec := f.do("GET", "/api/images", "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/images : %d %s", rec.Code, rec.Body)
	}
	got := decodeImages(t, rec.Body.Bytes())
	if len(got) != 3 {
		t.Fatalf("%d images, attendu 3 : %s", len(got), rec.Body)
	}
	for _, n := range []int{2, 3} {
		r := got["sha256:"+imgHex(n)]
		if r.Tags == nil || len(r.Tags) != 0 {
			t.Errorf("image %d sans étiquette : %+v", n, r)
		}
	}
	if r := got["sha256:"+imgHex(1)]; r.Size != 1000 || r.Created != "2026-10-01T12:00:00Z" ||
		!reflect.DeepEqual(r.Tags, []string{"shop/app:1.0", "shop/app:latest"}) {
		t.Errorf("image 1 : %+v", r)
	}
	if d := getImage(t, f, c, imgHex(2)); d.Tags == nil || len(d.Tags) != 0 || len(d.Layers) != 1 {
		t.Errorf("détail par ID : %+v", d)
	}
}

// The containers of an image come from the container list, by ImageID.
func TestImagesContainers(t *testing.T) {
	fake := imagesFake()
	f, c := imagesFixture(t, fake, fake)
	got := decodeImages(t, f.do("GET", "/api/images", "", c).Body.Bytes())
	want := []imageCtrRow{{ID: hexID(22), Name: "web-a", State: "exited"}, {ID: hexID(21), Name: "web-b", State: "running"}}
	if r := got["sha256:"+imgHex(1)]; !reflect.DeepEqual(r.Containers, want) {
		t.Errorf("liste : %+v", r.Containers)
	}
	if r := got["sha256:"+imgHex(2)]; !reflect.DeepEqual(r.Containers, []imageCtrRow{{ID: hexID(23), Name: "orphan", State: "exited"}}) {
		t.Errorf("image 2 : %+v", r.Containers)
	}
	if r := got["sha256:"+imgHex(3)]; r.Containers == nil || len(r.Containers) != 0 {
		t.Errorf("image 3 : %+v", r.Containers)
	}
	if d := getImage(t, f, c, imgHex(1)); !reflect.DeepEqual(d.Containers, want) {
		t.Errorf("détail : %+v", d.Containers)
	}
}

// The list is sorted, newest first.
func TestImagesOrder(t *testing.T) {
	fake := imagesFake()
	fake.ImageSummaries[0], fake.ImageSummaries[2] = fake.ImageSummaries[2], fake.ImageSummaries[0]
	f, c := imagesFixture(t, fake, fake)
	var rows []imageRow
	if err := json.Unmarshal(f.do("GET", "/api/images", "", c).Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	if want := []string{"sha256:" + imgHex(1), "sha256:" + imgHex(2), "sha256:" + imgHex(3)}; !reflect.DeepEqual(ids, want) {
		t.Errorf("ordre : %v", ids)
	}
}

// An error of Docker: 503 docker_unavailable with a fixed detail, never the
// daemon's text (it may hold the address of Docker, tcp://user:pass@host).
func TestImagesDockerUnavailable(t *testing.T) {
	fake := imagesFake()
	f, c := imagesFixture(t, fake, fake)
	fake.Err = fmt.Errorf("%w: dial tcp://u:s3cr3t-host@docker:2375", dockerx.ErrUnavailable)
	for _, u := range []string{"/api/images", "/api/images/" + imgHex(1)} {
		rec := f.do("GET", u, "", c)
		if rec.Code != http.StatusServiceUnavailable || errCode(t, rec) != "docker_unavailable" {
			t.Errorf("%s : %d %s", u, rec.Code, rec.Body)
		}
		if !strings.Contains(rec.Body.String(), `"detail":"Docker unavailable"`) {
			t.Errorf("%s : détail %s", u, rec.Body)
		}
		noImageWitness(t, rec.Body.String())
	}
}

// slowImages holds the image calls of Docker until release is closed or
// their context ends, and counts the ImageList calls.
type slowImages struct {
	*dockerx.Fake
	release chan struct{}
	lists   atomic.Int32
}

func (d *slowImages) wait(ctx context.Context) error {
	select {
	case <-d.release:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%w: %v", dockerx.ErrUnavailable, ctx.Err())
	}
}

func (d *slowImages) ImageList(ctx context.Context) ([]dockerx.ImageSummary, error) {
	d.lists.Add(1)
	if err := d.wait(ctx); err != nil {
		return nil, err
	}
	return d.Fake.ImageList(ctx)
}

func (d *slowImages) ImageInspect(ctx context.Context, id string) (dockerx.ImageInfo, error) {
	if err := d.wait(ctx); err != nil {
		return dockerx.ImageInfo{}, err
	}
	return d.Fake.ImageInspect(ctx, id)
}

// setVar replaces a variable for one test.
func setVar[T any](t *testing.T, p *T, v T) {
	t.Helper()
	old := *p
	*p = v
	t.Cleanup(func() { *p = old })
}

// A daemon that does not answer: 504 within the deadline, for the list and
// the detail.
func TestImagesTimeout(t *testing.T) {
	setVar(t, &imagesTimeout, 200*time.Millisecond)
	fake := imagesFake()
	slow := &slowImages{Fake: fake, release: make(chan struct{})}
	f, c := imagesFixture(t, slow, fake)
	for _, u := range []string{"/api/images", "/api/images/" + imgHex(1)} {
		start := time.Now()
		rec := f.do("GET", u, "", c)
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("%s : réponse en %v", u, d)
		}
		if rec.Code != http.StatusGatewayTimeout {
			t.Errorf("%s : %d %s", u, rec.Code, rec.Body)
		}
	}
}

// Simultaneous lists share one walk of Docker (the same question as the
// Secrets page): hundreds of images are listed once, not once per tab.
func TestImagesListCoalesced(t *testing.T) {
	fake := imagesFake()
	slow := &slowImages{Fake: fake, release: make(chan struct{})}
	f, c := imagesFixture(t, slow, fake)
	var wg sync.WaitGroup
	codes := make([]int, 6)
	for i := range codes {
		wg.Go(func() { codes[i] = f.do("GET", "/api/images", "", c).Code })
	}
	deadline := time.Now().Add(2 * time.Second)
	for slow.lists.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond) // the others join the walk in flight
	close(slow.release)
	wg.Wait()
	for i, code := range codes {
		if code != http.StatusOK {
			t.Errorf("requête %d : %d", i, code)
		}
	}
	if n := slow.lists.Load(); n != 1 {
		t.Errorf("%d ImageList, attendu 1", n)
	}
	// Once done, a new request walks again (nothing is cached).
	if rec := f.do("GET", "/api/images", "", c); rec.Code != http.StatusOK || slow.lists.Load() != 2 {
		t.Errorf("après : %d, %d ImageList", rec.Code, slow.lists.Load())
	}
}

// Read routes: allowed in --read-only, and no other method.
func TestImagesReadOnlyAndMethods(t *testing.T) {
	fake := imagesFake()
	f := &fixture{docker: fake}
	f.init(t, true, fake, nil)
	c := f.session(t)
	for _, u := range []string{"/api/images", "/api/images/" + imgHex(1)} {
		if rec := f.do("GET", u, "", c); rec.Code != http.StatusOK {
			t.Errorf("lecture seule %s : %d %s", u, rec.Code, rec.Body)
		}
		for _, m := range []string{"POST", "PUT", "DELETE"} {
			if rec := f.do(m, u, "{}", c); rec.Code == http.StatusOK {
				t.Errorf("%s %s : %d", m, u, rec.Code)
			}
		}
	}
}
