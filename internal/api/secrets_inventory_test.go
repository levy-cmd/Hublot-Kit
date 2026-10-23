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
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/secrets"
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
	Args        []argRow `json:"args"`
	Labels      []string `json:"labels"`
	Unavailable bool     `json:"unavailable"`
}

// argRow: a masked argument by its position and the name detected, never
// its text (review of tranche 4, Codex: a partial mask lets the rest of the
// value out).
type argRow struct {
	Part  string `json:"part"`
	Index int    `json:"index"`
	Kind  string `json:"kind"`
	Name  string `json:"name,omitempty"`
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
// masks; args: the position and detected name of the arguments secrets.Args
// changed, never their text;
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
			Env: []string{"DB_PASSWORD", "URL", "GITHUB_TOKEN"}, Args: []argRow{},
			Labels: []string{"backup.dsn", "com.docker.compose.project.working_dir"}},
		"cache": {ID: hexID(2), Name: "cache", Project: "shop", Service: "cache", State: "exited",
			Env: []string{}, Args: []argRow{{Part: "cmd", Index: 2, Kind: "flag", Name: "--requirepass"}}, Labels: []string{}},
		"plain": {ID: hexID(4), Name: "plain", State: "running", Env: []string{}, Args: []argRow{}, Labels: []string{}},
		"proxy": {ID: hexID(3), Name: "proxy", State: "running", Env: []string{},
			Args:   []argRow{{Part: "entrypoint", Index: 2, Kind: "shell", Name: "sh -c"}},
			Labels: []string{"traefik.http.middlewares.auth.basicauth.users"}},
	}
	for name, w := range want {
		if !reflect.DeepEqual(got[name], w) {
			t.Errorf("%s :\n obtenu  %+v\n attendu %+v", name, got[name], w)
		}
	}
	// No masked text either: a position and a name, never "••••••" around
	// what a partial mask would leave.
	if strings.Contains(rec.Body.String(), "••••••") {
		t.Errorf("texte masqué dans la réponse : %s", rec.Body)
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
		Env: []string{}, Args: []argRow{}, Labels: []string{}, Unavailable: true}
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

// Spec v0.2 §5.9: never more than 4 Inspect in flight (simultaneous
// inventories: TestSecretsInventoryCoalesced).
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
	rec := f.do("GET", "/api/secrets", "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	noWitness(t, rec.Body.String())
	for name, r := range decodeInventory(t, rec.Body.Bytes()) {
		if r.Unavailable || !reflect.DeepEqual(r.Env, []string{"API_TOKEN"}) {
			t.Errorf("%s : %+v", name, r)
		}
	}
	if m := slow.max.Load(); m > 4 || m < 2 {
		t.Errorf("%d Inspect en vol au plus, attendu de 2 à 4", m)
	}
	if n := slow.calls.Load(); n != 24 {
		t.Errorf("%d Inspect, attendu 24", n)
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

// Review of tranche 4 (Codex, Critical): the partial masks of spec §13 leave
// a rest of the value in the masked text. The inventory gives the position
// and the detected name of a masked argument, never its text: not one
// character of a value goes out, whatever the form.
func TestSecretsInventoryPartialMasks(t *testing.T) {
	fake := &dockerx.Fake{Infos: map[string]dockerx.ContainerInfo{}}
	add := func(n int, name string, ep, cmd []string) {
		id := hexID(n)
		fake.Containers = append(fake.Containers, dockerx.Container{ID: id, Name: name, State: "running"})
		fake.Infos[id] = dockerx.ContainerInfo{ID: id, Name: name, Running: true, Pid: 3, Details: dockerx.Details{Entrypoint: ep, Cmd: cmd}}
	}
	long := "x" + strings.Repeat("s3cr3t-long", 1000)
	add(1, "rest", nil, []string{"sh", "-c", "app Password=ab(üü)"})
	add(2, "json", []string{"bash", "-lc", `app --token {"password": "s3cr3t-json"}`}, nil)
	add(3, "forms", []string{"app", "postgres://u:s3cr3t-url@db", "Authorization: Bearer s3cr3t-bearer"},
		[]string{"PGPASSWORD=s3cr3t-pg", "--password=", "s3cr3t-flag", "--db", "Server=x;Pwd=s3cr3t-pwd;", long})
	f, c := inventoryFixture(t, fake, fake, false)
	rec := f.do("GET", "/api/secrets", "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, w := range []string{"s3cr3t", "üü", "ab(", "(", `\"`, "••••••", "Server", "u:", "app"} {
		if strings.Contains(body, w) {
			t.Errorf("%q dans la réponse : %s", w, body)
		}
	}
	got := decodeInventory(t, rec.Body.Bytes())
	want := map[string][]argRow{
		"rest": {{Part: "cmd", Index: 2, Kind: "shell", Name: "sh -c"}},
		"json": {{Part: "entrypoint", Index: 2, Kind: "shell", Name: "bash -lc"}},
		"forms": {
			{Part: "entrypoint", Index: 1, Kind: "url"},
			{Part: "entrypoint", Index: 2, Kind: "authorization"},
			{Part: "cmd", Index: 0, Kind: "assignment", Name: "PGPASSWORD"},
			{Part: "cmd", Index: 2, Kind: "flag", Name: "--password"},
			{Part: "cmd", Index: 4, Kind: "other"},
			{Part: "cmd", Index: 5, Kind: "other"},
		},
	}
	for name, w := range want {
		if !reflect.DeepEqual(got[name].Args, w) {
			t.Errorf("%s :\n obtenu  %+v\n attendu %+v", name, got[name].Args, w)
		}
	}
}

// Second review of tranche 4 (relecteur, Important): a name is taken only
// from text the masking left in clear. "--password=" masks the argument
// after it whole, even "--Tok3n_SECRET_xyz", which must then not name the
// next one; a masked shell flag must not name the shell line.
func TestSecretsInventoryNameFromClearTextOnly(t *testing.T) {
	cases := [][]string{
		{"app", "--password=", "--Tok3n_SECRET_xyz", "x"},
		{"sh", "--password=", "-zc", "app --token s3cr3t"},
		{"app", "--password=", "PGPASSWORD=s3cr3t"},
		{"app", "--password=", "--api-key", "s3cr3t"},
		{"app", "--token", "--secret=s3cr3t", "--key", "s3cr3t"},
		{"bash", "-c", "--password=s3cr3t", "x"},
		{"app", "--password", "-c", "Authorization: Bearer s3cr3t"},
		{"app", "--password=", "postgres://u:s3cr3t@db", "--password", "y"},
		{"sh", "-c", "PASS=s3cr3t app", "--secret", "z"},
		// "--token" is the masked value of "--password=", so the next argument
		// is masked whole as its value: its key "MY_PASSWORD_s3cr3t" is value.
		{"app", "--password=", "--token", "MY_PASSWORD_s3cr3t=x"},
		// The shell is read from args[0], here a masked value: path.Base
		// would take "sh" out of it.
		{"API_TOKEN=hunter/sh", "-c", "echo TOKEN=abc"},
	}
	fake := &dockerx.Fake{Infos: map[string]dockerx.ContainerInfo{}}
	for n, cmd := range cases {
		id := hexID(500 + n)
		name := fmt.Sprintf("n%d", n)
		fake.Containers = append(fake.Containers, dockerx.Container{ID: id, Name: name, State: "running"})
		fake.Infos[id] = dockerx.ContainerInfo{ID: id, Name: name, Running: true, Pid: 3, Details: dockerx.Details{Cmd: cmd}}
	}
	f, c := inventoryFixture(t, fake, fake, false)
	rec := f.do("GET", "/api/secrets", "", c)
	body := rec.Body.String()
	for _, w := range []string{"Tok3n", "-zc", "s3cr3t"} {
		if strings.Contains(body, w) {
			t.Errorf("%q dans la réponse : %s", w, body)
		}
	}
	got := decodeInventory(t, rec.Body.Bytes())
	if a := got["n0"].Args; !reflect.DeepEqual(a, []argRow{
		{Part: "cmd", Index: 2, Kind: "flag", Name: "--password"},
		{Part: "cmd", Index: 3, Kind: "other"},
	}) {
		t.Errorf("n0 : %+v", a)
	}
	if a := got["n1"].Args; len(a) != 2 || a[1] != (argRow{Part: "cmd", Index: 3, Kind: "other"}) {
		t.Errorf("n1 : %+v", a)
	}
	// Every name, whatever the form, is text secrets.Args left in clear: the
	// flag before (or the shell and its flag), or the key kept before "=".
	for n, cmd := range cases {
		masked, _ := secrets.Args(cmd)
		for _, a := range got[fmt.Sprintf("n%d", n)].Args {
			i := a.Index
			ok := true
			switch a.Kind {
			case "flag":
				ok = masked[i-1] == cmd[i-1] && strings.TrimSuffix(cmd[i-1], "=") == a.Name
			case "shell":
				ok = masked[0] == cmd[0] && masked[i-1] == cmd[i-1] && a.Name == cmd[0]+" "+cmd[i-1]
			case "assignment":
				ok = strings.HasPrefix(masked[i], a.Name+"=") && strings.HasPrefix(cmd[i], a.Name+"=")
			default:
				ok = a.Name == ""
			}
			if !ok {
				t.Errorf("n%d %q (masqué %q) : nom %+v tiré d'un texte masqué", n, cmd, masked, a)
			}
		}
	}
}

// Second review of tranche 5: Docker runs the entrypoint and the command as
// one argv; a secret flag that ends the entrypoint names the first argument
// of the command. part and index name the field of origin, after the split.
func TestSecretsInventoryCommandBoundary(t *testing.T) {
	cases := []struct {
		ep, cmd []string
		want    []argRow
	}{
		{[]string{"redis-server", "--requirepass"}, []string{"s3cr3t-v", "--port", "6379"},
			[]argRow{{Part: "cmd", Index: 0, Kind: "flag", Name: "--requirepass"}}},
		{[]string{"app", "--password="}, []string{"s3cr3t-w"}, []argRow{{Part: "cmd", Index: 0, Kind: "flag", Name: "--password"}}},
		{[]string{"sh", "-c"}, []string{"curl -H 'Authorization: Bearer s3cr3t-x' x"},
			[]argRow{{Part: "cmd", Index: 0, Kind: "shell", Name: "sh -c"}}},
		{[]string{"/entry.sh", "--token", "s3cr3t-y"}, []string{"run", "--secret", "s3cr3t-z"},
			[]argRow{{Part: "entrypoint", Index: 2, Kind: "flag", Name: "--token"}, {Part: "cmd", Index: 2, Kind: "flag", Name: "--secret"}}},
		// The name comes from clear text only, across the boundary too.
		{[]string{"app", "--password="}, []string{"--token", "s3cr3t-u"},
			[]argRow{{Part: "cmd", Index: 0, Kind: "flag", Name: "--password"}, {Part: "cmd", Index: 1, Kind: "other"}}},
	}
	fake := &dockerx.Fake{Infos: map[string]dockerx.ContainerInfo{}}
	for n, tc := range cases {
		id := hexID(700 + n)
		name := fmt.Sprintf("b%d", n)
		fake.Containers = append(fake.Containers, dockerx.Container{ID: id, Name: name, State: "running"})
		fake.Infos[id] = dockerx.ContainerInfo{ID: id, Name: name, Running: true, Pid: 3,
			Details: dockerx.Details{Entrypoint: tc.ep, Cmd: tc.cmd}}
	}
	f, c := inventoryFixture(t, fake, fake, false)
	rec := f.do("GET", "/api/secrets", "", c)
	if strings.Contains(rec.Body.String(), "s3cr3t") {
		t.Errorf("valeur dans la réponse : %s", rec.Body)
	}
	got := decodeInventory(t, rec.Body.Bytes())
	for n, tc := range cases {
		if a := got[fmt.Sprintf("b%d", n)].Args; !reflect.DeepEqual(a, tc.want) {
			t.Errorf("%q + %q :\n obtenu  %+v\n attendu %+v", tc.ep, tc.cmd, a, tc.want)
		}
	}
}

// Review of tranche 4 (relecteur, Important): Docker keeps two variables of
// the same name (API_TOKEN=1, API_TOKEN=2); the inventory lists a name once.
func TestSecretsInventoryDuplicateNames(t *testing.T) {
	fake := &dockerx.Fake{Infos: map[string]dockerx.ContainerInfo{}}
	id := hexID(9)
	fake.Containers = []dockerx.Container{{ID: id, Name: "dup", State: "running"}}
	fake.Infos[id] = dockerx.ContainerInfo{ID: id, Name: "dup", Running: true, Pid: 3,
		Details: dockerx.Details{Env: []string{"API_TOKEN=s3cr3t-1", "DB_PASSWORD=s3cr3t-2", "API_TOKEN=s3cr3t-3"}}}
	f, c := inventoryFixture(t, fake, fake, false)
	got := decodeInventory(t, f.do("GET", "/api/secrets", "", c).Body.Bytes())
	if !reflect.DeepEqual(got["dup"].Env, []string{"API_TOKEN", "DB_PASSWORD"}) {
		t.Errorf("env : %v", got["dup"].Env)
	}
}

// Review of tranche 4 (Codex): the error of the list may hold the address of
// Docker with credentials; the answer is docker_unavailable with a fixed
// detail.
func TestSecretsInventoryListErrorMasked(t *testing.T) {
	fake := inventoryFake()
	f, c := inventoryFixture(t, fake, fake, false)
	fake.Err = fmt.Errorf("%w: dial tcp://u:pw-s3cr3t@h:2376: connection refused", dockerx.ErrUnavailable)
	rec := f.do("GET", "/api/secrets", "", c)
	if rec.Code != http.StatusServiceUnavailable || errCode(t, rec) != "docker_unavailable" {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	for _, w := range []string{"pw-s3cr3t", "u:", "tcp://", "2376"} {
		if strings.Contains(rec.Body.String(), w) {
			t.Errorf("%q dans la réponse : %s", w, rec.Body)
		}
	}
}

// gatedList holds ListContainers until open is closed, and counts the calls.
type gatedList struct {
	*slowDocker
	open  chan struct{}
	lists atomic.Int32
}

func (g *gatedList) ListContainers(ctx context.Context) ([]dockerx.Container, error) {
	g.lists.Add(1)
	select {
	case <-g.open:
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: %v", dockerx.ErrUnavailable, ctx.Err())
	}
	return g.slowDocker.ListContainers(ctx)
}

// Review of tranche 4 (Codex): one caller must not take every place by
// opening many inventories. Simultaneous inventories share one walk (one
// list, one Inspect per container, 4 in flight at most); each waits with its
// own deadline.
func TestSecretsInventoryCoalesced(t *testing.T) {
	fake := &dockerx.Fake{Infos: map[string]dockerx.ContainerInfo{}}
	for i := range 12 {
		id := hexID(300 + i)
		fake.Containers = append(fake.Containers, dockerx.Container{ID: id, Name: fmt.Sprintf("k%d", i), State: "running"})
		fake.Infos[id] = dockerx.ContainerInfo{ID: id, Name: fmt.Sprintf("k%d", i), Running: true, Pid: 9,
			Details: dockerx.Details{Env: []string{"API_TOKEN=s3cr3t"}}}
	}
	g := &gatedList{slowDocker: &slowDocker{Fake: fake, delay: 20 * time.Millisecond}, open: make(chan struct{})}
	f, c := inventoryFixture(t, g, fake, false)

	// A caller that leaves: its own deadline, not the walk's.
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	early := httptest.NewRecorder()
	var wg sync.WaitGroup
	wg.Go(func() {
		r := httptest.NewRequestWithContext(ctx, "GET", "http://example.com/api/secrets", nil)
		r.RemoteAddr = "192.0.2.1:40000"
		r.AddCookie(c)
		start := time.Now()
		f.h.ServeHTTP(early, r)
		if d := time.Since(start); d > time.Second {
			t.Errorf("appelant parti servi en %v", d)
		}
	})
	const n = 8
	recs := make([]*httptest.ResponseRecorder, n)
	for i := range n {
		wg.Go(func() { recs[i] = f.do("GET", "/api/secrets", "", c) })
	}
	time.Sleep(300 * time.Millisecond)
	close(g.open)
	wg.Wait()
	if early.Code == http.StatusOK {
		t.Errorf("appelant parti : 200 %s", early.Body)
	}
	for i, rec := range recs {
		if rec.Code != http.StatusOK || len(decodeInventory(t, rec.Body.Bytes())) != 12 {
			t.Errorf("requête %d : %d %s", i, rec.Code, rec.Body)
		}
	}
	if l := g.lists.Load(); l != 1 {
		t.Errorf("%d listes, attendu une seule", l)
	}
	if k := g.calls.Load(); k != 12 {
		t.Errorf("%d Inspect, attendu 12 (un seul parcours)", k)
	}
	if m := g.max.Load(); m > 4 {
		t.Errorf("%d Inspect en vol", m)
	}
}

// Review of tranche 4: the reveal reads one JSON object of 4 KiB at most,
// nothing after it but blanks, as the search (decodeOne).
func TestRevealEnvTrailingData(t *testing.T) {
	fake := inventoryFake()
	f, c := inventoryFixture(t, fake, fake, false)
	u := "/api/containers/" + hexID(1) + "/env/reveal"
	for _, body := range []string{
		`{"name":"DB_PASSWORD"}` + strings.Repeat(" ", 5000) + "garbage",
		`{"name":"DB_PASSWORD"}` + strings.Repeat(" ", 5000),
		`{"name":"DB_PASSWORD"}}`,
	} {
		rec := f.do("POST", u, body, c)
		if rec.Code != http.StatusNotFound || errCode(t, rec) != "not_found" || strings.Contains(rec.Body.String(), "s3cr3t") {
			t.Errorf("corps de %d octets : %d %s", len(body), rec.Code, rec.Body)
		}
	}
	if log := auditLines(t, f); strings.Contains(log, "env.reveal") {
		t.Errorf("refus journalisé : %s", log)
	}
	if rec := f.do("POST", u, "{\"name\":\"DB_PASSWORD\"}\n", c); rec.Code != http.StatusOK {
		t.Errorf("corps valide : %d %s", rec.Code, rec.Body)
	}
}
