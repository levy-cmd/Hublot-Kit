package api

// Tests of GET /api/secrets (spec v0.2 §5.9, plan v0.2 tranche 4), written
// before the code: the inventory gives the NAMES of what the radiography
// masks, never a value, whatever its form (env, command, labels, URL with
// credentials), and its cost is bounded (4 Inspect in flight for the whole
// agent, 3 s each, a deadline for the whole request, a number of
// containers). The reveal stays POST /env/reveal, journaled.

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
)

// Witnesses: none of these may appear in an answer of /api/secrets.
const ghpWitness = "ghp_16C7e42F292c6912E7710c838347Ae178B4a"

var inventoryWitnesses = []string{"s3cr3t", "ghp_", "visible-plain", "/home/alice", "admin:"}

type inventoryRow struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Project     string   `json:"project"`
	Service     string   `json:"service"`
	State       string   `json:"state"`
	Env         []string `json:"env"`
	Args        []string `json:"args"`
	Labels      []string `json:"labels"`
	Unavailable bool     `json:"unavailable"`
}

// hexID is a full 64-character ID made of n.
func hexID(n int) string { return fmt.Sprintf("%064x", n) }

// inventoryFake: four containers, every form of secret of spec §7 and
// v0.2 §5.2, plus a plain value that is not a secret (never sent either).
func inventoryFake() *dockerx.Fake {
	f := &dockerx.Fake{Infos: map[string]dockerx.ContainerInfo{}}
	add := func(n int, name, state, project, service string, d dockerx.Details) {
		id := hexID(n)
		f.Containers = append(f.Containers, dockerx.Container{ID: id, Name: name, State: state, Project: project, Service: service, Ports: []dockerx.Port{}})
		f.Infos[id] = dockerx.ContainerInfo{ID: id, Name: name, Running: state == "running", Pid: 7, Details: d}
	}
	add(1, "app", "running", "shop", "api", dockerx.Details{
		Env: []string{"DB_PASSWORD=s3cr3t-env", "URL=postgres://u:s3cr3t-url@db", "PLAIN=visible-plain", "GITHUB_TOKEN=" + ghpWitness},
		Labels: map[string]string{
			"com.docker.compose.project":             "shop",
			"com.docker.compose.service":             "api",
			"com.docker.compose.project.working_dir": "/home/alice/shop",
			"backup.dsn":                             "postgres://u:s3cr3t-dsn@db",
		},
	})
	add(2, "cache", "exited", "shop", "cache", dockerx.Details{
		Cmd: []string{"redis-server", "--requirepass", "s3cr3t-arg"},
	})
	add(3, "proxy", "running", "", "", dockerx.Details{
		Entrypoint: []string{"sh", "-c", "curl -H 'Authorization: Bearer " + ghpWitness + "' http://x"},
		Cmd:        []string{"--api.insecure=false"},
		Labels:     map[string]string{"traefik.http.middlewares.auth.basicauth.users": "admin:s3cr3t-lbl"},
	})
	add(4, "plain", "running", "", "", dockerx.Details{Env: []string{"PATH=/usr/bin"}, Cmd: []string{"nginx", "-g", "daemon off;"}})
	return f
}

func inventoryFixture(t *testing.T, docker dockerx.Client, fake *dockerx.Fake, readOnly bool) (*fixture, *http.Cookie) {
	t.Helper()
	f := &fixture{docker: fake}
	f.init(t, readOnly, docker, nil)
	return f, f.session(t)
}

func decodeInventory(t *testing.T, body []byte) map[string]inventoryRow {
	t.Helper()
	var rows []inventoryRow
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("réponse illisible : %v %s", err, body)
	}
	out := map[string]inventoryRow{}
	for _, r := range rows {
		out[r.Name] = r
	}
	return out
}

func noWitness(t *testing.T, body string) {
	t.Helper()
	for _, w := range inventoryWitnesses {
		if strings.Contains(body, w) {
			t.Errorf("valeur témoin %q dans la réponse : %s", w, body)
		}
	}
}

// Spec v0.2 §5.9: names only. env: the names of the variables secrets.Masked
// masks; args: the arguments secrets.Args changed, in their masked form;
// labels: the keys of the labels secrets.Labels masks. Never a value, even
// a plain one, and arrays rather than null.
func TestSecretsInventoryNeverLeaksValues(t *testing.T) {
	fake := inventoryFake()
	f, c := inventoryFixture(t, fake, fake, false)
	if rec := f.do("GET", "/api/secrets", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("sans session : %d %s", rec.Code, rec.Body)
	}
	rec := f.do("GET", "/api/secrets", "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/secrets : %d %s", rec.Code, rec.Body)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, attendu no-store", cc)
	}
	noWitness(t, rec.Body.String())
	if strings.Contains(rec.Body.String(), `"value"`) {
		t.Errorf("un champ value dans la réponse : %s", rec.Body)
	}
	got := decodeInventory(t, rec.Body.Bytes())
	want := map[string]inventoryRow{
		"app": {ID: hexID(1), Name: "app", Project: "shop", Service: "api", State: "running",
			Env: []string{"DB_PASSWORD", "URL", "GITHUB_TOKEN"}, Args: []string{},
			Labels: []string{"backup.dsn", "com.docker.compose.project.working_dir"}},
		"cache": {ID: hexID(2), Name: "cache", Project: "shop", Service: "cache", State: "exited",
			Env: []string{}, Args: []string{"••••••"}, Labels: []string{}},
		"plain": {ID: hexID(4), Name: "plain", State: "running", Env: []string{}, Args: []string{}, Labels: []string{}},
	}
	for name, w := range want {
		if !reflect.DeepEqual(got[name], w) {
			t.Errorf("%s :\n obtenu  %+v\n attendu %+v", name, got[name], w)
		}
	}
	// proxy: the shell line of the entrypoint, masked by secrets.Args as in
	// the radiography; the clear argument of Cmd is not listed.
	p := got["proxy"]
	if len(p.Args) != 1 || !strings.Contains(p.Args[0], "••••••") || !reflect.DeepEqual(p.Labels, []string{"traefik.http.middlewares.auth.basicauth.users"}) || len(p.Env) != 0 {
		t.Errorf("proxy : %+v", p)
	}
	if len(got) != 4 {
		t.Errorf("%d conteneurs, attendu 4 : %s", len(got), rec.Body)
	}
}

// The reveal stays the route of the radiography (Q28): journaled, behind
// the Origin check; the inventory has no reveal of its own.
func TestSecretsInventoryRevealOnlyByEnvReveal(t *testing.T) {
	fake := inventoryFake()
	f, c := inventoryFixture(t, fake, fake, false)
	for _, m := range []string{"POST", "PUT", "DELETE"} {
		if rec := f.do(m, "/api/secrets", `{"id":"`+hexID(1)+`","name":"DB_PASSWORD"}`, c); rec.Code == http.StatusOK || strings.Contains(rec.Body.String(), "s3cr3t") {
			t.Errorf("%s /api/secrets : %d %s", m, rec.Code, rec.Body)
		}
	}
	for _, u := range []string{"/api/secrets?reveal=1", "/api/secrets?name=DB_PASSWORD", "/api/secrets/" + hexID(1)} {
		rec := f.do("GET", u, "", c)
		noWitness(t, rec.Body.String())
	}
	rec := f.do("POST", "/api/containers/"+hexID(1)+"/env/reveal", `{"name":"DB_PASSWORD"}`, c)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "s3cr3t-env") {
		t.Fatalf("révélation : %d %s", rec.Code, rec.Body)
	}
	if log := auditLines(t, f); !strings.Contains(log, `"action":"env.reveal","container":"`+hexID(1)+`","path":"DB_PASSWORD","result":"ok"`) || strings.Contains(log, "s3cr3t") {
		t.Errorf("journal : %s", log)
	}
	// Nothing of the inventory goes to the audit log: it is a read of names.
	if log := auditLines(t, f); strings.Count(log, "\n") != 1 {
		t.Errorf("le journal a d'autres lignes que la révélation : %s", log)
	}
}

// Q28: the inventory and the reveal are reads; --read-only allows both.
func TestSecretsInventoryReadOnly(t *testing.T) {
	fake := inventoryFake()
	f, c := inventoryFixture(t, fake, fake, true)
	rec := f.do("GET", "/api/secrets", "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("lecture seule : %d %s", rec.Code, rec.Body)
	}
	noWitness(t, rec.Body.String())
	if r := f.do("POST", "/api/containers/"+hexID(1)+"/env/reveal", `{"name":"DB_PASSWORD"}`, c); r.Code != http.StatusOK {
		t.Errorf("révélation en lecture seule : %d %s", r.Code, r.Body)
	}
}

// Docker down: 503 docker_unavailable, as the list. A container removed
// between the list and its Inspect is left out, not unavailable.
func TestSecretsInventoryDockerErrors(t *testing.T) {
	fake := inventoryFake()
	delete(fake.Infos, hexID(4))
	f, c := inventoryFixture(t, fake, fake, false)
	got := decodeInventory(t, f.do("GET", "/api/secrets", "", c).Body.Bytes())
	if _, ok := got["plain"]; ok || len(got) != 3 {
		t.Errorf("conteneur disparu : %+v", got)
	}
	fake.Err = dockerx.ErrUnavailable
	if rec := f.do("GET", "/api/secrets", "", c); rec.Code != http.StatusServiceUnavailable || errCode(t, rec) != "docker_unavailable" {
		t.Errorf("Docker absent : %d %s", rec.Code, rec.Body)
	}
}

// slowDocker counts the Inspect calls in flight, waits delay in each, and
// holds those of the IDs in locked for 10 s (dockerd's lock during an
// export) or until their context ends.
type slowDocker struct {
	*dockerx.Fake
	delay    time.Duration
	locked   map[string]bool
	inFlight atomic.Int32
	max      atomic.Int32
	calls    atomic.Int32
}

func (d *slowDocker) Inspect(ctx context.Context, id string) (dockerx.ContainerInfo, error) {
	d.calls.Add(1)
	n := d.inFlight.Add(1)
	defer d.inFlight.Add(-1)
	for {
		m := d.max.Load()
		if n <= m || d.max.CompareAndSwap(m, n) {
			break
		}
	}
	wait := d.delay
	if d.locked[id] {
		wait = 10 * time.Second
	}
	select {
	case <-time.After(wait):
	case <-ctx.Done():
		return dockerx.ContainerInfo{}, fmt.Errorf("%w: %v", dockerx.ErrUnavailable, ctx.Err())
	}
	return d.Fake.Inspect(ctx, id)
}

// Plan v0.2 tranche 4: a container locked by an export is unavailable in
// 3 s at most (3.5 s with the test's margin), and the others are complete.
func TestSecretsInventoryTimeout(t *testing.T) {
	fake := inventoryFake()
	slow := &slowDocker{Fake: fake, locked: map[string]bool{hexID(2): true}}
	f, c := inventoryFixture(t, slow, fake, false)
	start := time.Now()
	rec := f.do("GET", "/api/secrets", "", c)
	if d := time.Since(start); d > 3500*time.Millisecond {
		t.Errorf("réponse en %v, attendu moins de 3,5 s", d)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	noWitness(t, rec.Body.String())
	got := decodeInventory(t, rec.Body.Bytes())
	cache := got["cache"]
	want := inventoryRow{ID: hexID(2), Name: "cache", Project: "shop", Service: "cache", State: "exited",
		Env: []string{}, Args: []string{}, Labels: []string{}, Unavailable: true}
	if !reflect.DeepEqual(cache, want) {
		t.Errorf("verrouillé :\n obtenu  %+v\n attendu %+v", cache, want)
	}
	if a := got["app"]; a.Unavailable || len(a.Env) != 3 {
		t.Errorf("app incomplet : %+v", a)
	}
	if p := got["proxy"]; p.Unavailable || len(p.Labels) != 1 {
		t.Errorf("proxy incomplet : %+v", p)
	}
}

// Spec v0.2 §5.9: never more than 4 Inspect in flight, for the whole agent:
// two inventories at once share the same 4 places.
func TestSecretsInventoryConcurrency(t *testing.T) {
	fake := &dockerx.Fake{Infos: map[string]dockerx.ContainerInfo{}}
	for i := range 24 {
		id := hexID(100 + i)
		fake.Containers = append(fake.Containers, dockerx.Container{ID: id, Name: fmt.Sprintf("c%d", i), State: "running"})
		fake.Infos[id] = dockerx.ContainerInfo{ID: id, Name: fmt.Sprintf("c%d", i), Running: true, Pid: 9,
			Details: dockerx.Details{Env: []string{"API_TOKEN=s3cr3t-" + fmt.Sprint(i)}}}
	}
	slow := &slowDocker{Fake: fake, delay: 30 * time.Millisecond}
	f, c := inventoryFixture(t, slow, fake, false)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			rec := f.do("GET", "/api/secrets", "", c)
			if rec.Code != http.StatusOK {
				t.Errorf("%d %s", rec.Code, rec.Body)
				return
			}
			noWitness(t, rec.Body.String())
			for name, r := range decodeInventory(t, rec.Body.Bytes()) {
				if r.Unavailable || !reflect.DeepEqual(r.Env, []string{"API_TOKEN"}) {
					t.Errorf("%s : %+v", name, r)
				}
			}
		})
	}
	wg.Wait()
	if m := slow.max.Load(); m > 4 || m < 2 {
		t.Errorf("%d Inspect en vol au plus, attendu de 2 à 4", m)
	}
	if n := slow.calls.Load(); n != 48 {
		t.Errorf("%d Inspect, attendu 48", n)
	}
}

// The cost of one inventory is bounded whatever the number of containers:
// past inventoryTimeout, the containers not yet inspected are unavailable
// without an Inspect, and past inventoryMax containers, the rest too.
func TestSecretsInventoryBounded(t *testing.T) {
	fake := &dockerx.Fake{Infos: map[string]dockerx.ContainerInfo{}}
	locked := map[string]bool{}
	for i := range 12 {
		id := hexID(200 + i)
		fake.Containers = append(fake.Containers, dockerx.Container{ID: id, Name: fmt.Sprintf("l%d", i), State: "exited"})
		fake.Infos[id] = dockerx.ContainerInfo{ID: id, Name: fmt.Sprintf("l%d", i)}
		locked[id] = true
	}
	slow := &slowDocker{Fake: fake, locked: locked}
	f, c := inventoryFixture(t, slow, fake, false)

	old := inventoryTimeout
	inventoryTimeout = 500 * time.Millisecond
	t.Cleanup(func() { inventoryTimeout = old })
	start := time.Now()
	rec := f.do("GET", "/api/secrets", "", c)
	if d := time.Since(start); d > 1500*time.Millisecond {
		t.Errorf("douze conteneurs verrouillés : réponse en %v, attendu le délai de la requête", d)
	}
	got := decodeInventory(t, rec.Body.Bytes())
	if len(got) != 12 {
		t.Fatalf("%d lignes, attendu 12 : %s", len(got), rec.Body)
	}
	for name, r := range got {
		if !r.Unavailable {
			t.Errorf("%s disponible : %+v", name, r)
		}
	}
	if n := slow.calls.Load(); n > 4 {
		t.Errorf("%d Inspect lancés en 0,5 s, attendu 4 au plus", n)
	}

	inventoryTimeout = old
	oldMax := inventoryMax
	inventoryMax = 3
	t.Cleanup(func() { inventoryMax = oldMax })
	slow.locked = nil
	slow.calls.Store(0)
	got = decodeInventory(t, f.do("GET", "/api/secrets", "", c).Body.Bytes())
	avail := 0
	for _, r := range got {
		if !r.Unavailable {
			avail++
		}
	}
	if len(got) != 12 || avail != 3 || slow.calls.Load() != 3 {
		t.Errorf("plafond de 3 : %d lignes, %d complètes, %d Inspect", len(got), avail, slow.calls.Load())
	}
}
