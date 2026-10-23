package api

// Tests of the radiography (F7, spec §6, §7), written before the code: no
// secret leaves the agent unmasked; a reveal needs the Origin check, is
// audited before the value is sent, and never writes the value in the log.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// The secrets of the fake "web" container: none may appear in a response
// that has not revealed it.
var radioSecrets = []string{"s3cr3t-env", "s3cr3t-url", "s3cr3t-cmd", "s3cr3t-ep", "s3cr3t-err", "s3cr3t-cs", "s3cr3t-label"}

func radioFixture(t *testing.T, readOnly bool) (*fixture, *http.Cookie) {
	t.Helper()
	f := newFixture(t, readOnly)
	web := f.docker.Infos["web"]
	web.Mounts = []dockerx.Mount{{Type: "bind", Source: "/srv/conf", Destination: "/etc/app", Mode: "ro"}}
	web.Tmpfs = []string{"/run"}
	web.LogDriver = "json-file"
	web.FinishedAt = "0001-01-01T00:00:00Z" // running: Docker's zero date
	web.Details = dockerx.Details{
		Image: "nginx:1.27", ImageID: "sha256:feed", Created: "2026-10-10T07:00:00Z", Status: "running",
		// Review Minor 8: Docker's message may quote the command.
		Error:        `OCI runtime create failed: exec: "--password=s3cr3t-err": unknown`,
		RestartCount: 2, RestartPolicy: "unless-stopped",
		Entrypoint: []string{"/entry.sh", "--token", "s3cr3t-ep"},
		Cmd:        []string{"redis-server", "--requirepass", "s3cr3t-cmd"},
		Env: []string{"PATH=/usr/bin", "DB_PASSWORD=s3cr3t-env", "CACHE_URL=redis://:s3cr3t-url@cache:6379",
			"PLAIN=visible", "DB=Server=db;Password=s3cr3t-cs;"},
		Ports:       []dockerx.Port{{Private: 80, Public: 8080, Type: "tcp", IP: "0.0.0.0"}},
		NetworkMode: "bridge",
		Networks:    []dockerx.Network{{Name: "front", IP: "172.18.0.2"}},
	}
	f.docker.Infos["web"], f.docker.Infos[webID] = web, web
	return f, f.session(t)
}

func noSecret(t *testing.T, body string) {
	t.Helper()
	for _, s := range radioSecrets {
		if strings.Contains(body, s) {
			t.Errorf("FUITE : %s dans la réponse", s)
		}
	}
}

func TestContainerNeedsSession(t *testing.T) {
	f, _ := radioFixture(t, false)
	if rec := f.do("GET", "/api/containers/web", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("statut %d", rec.Code)
	}
	if rec := f.do("POST", "/api/containers/web/env/reveal", `{"name":"DB_PASSWORD"}`, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("révélation sans session : %d", rec.Code)
	}
}

func TestContainerRadiography(t *testing.T) {
	f, c := radioFixture(t, false)
	rec := f.do("GET", "/api/containers/web", "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("statut %d : %s", rec.Code, rec.Body)
	}
	noSecret(t, rec.Body.String())
	if strings.Contains(rec.Body.String(), "0001-01-01") || strings.Contains(rec.Body.String(), "finishedAt") {
		t.Errorf("date zéro de Docker affichée : %s", rec.Body)
	}
	var got struct {
		ID, Name, Image, ImageID, LogDriver string
		State                               struct {
			Status  string
			Running bool
		}
		RestartCount    int
		RestartPolicy   struct{ Name string }
		Cmd, Entrypoint []string
		CommandMasked   bool
		Env             []struct {
			Name   string
			Value  *string
			Masked bool
		}
		Ports    []dockerx.Port
		Networks []struct{ Name, IP string }
		Mounts   []struct {
			Type, Source, Destination, Mode string
			RW                              bool
		}
		Tmpfs []string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != webID || got.Name != "web" || got.Image != "nginx:1.27" || got.ImageID != "sha256:feed" ||
		got.State.Status != "running" || !got.State.Running || got.RestartCount != 2 ||
		got.RestartPolicy.Name != "unless-stopped" || got.LogDriver != "json-file" {
		t.Errorf("identité, état : %+v", got)
	}
	if !got.CommandMasked || strings.Join(got.Cmd, " ") != "redis-server --requirepass ••••••" ||
		strings.Join(got.Entrypoint, " ") != "/entry.sh --token ••••••" {
		t.Errorf("commande : %q %q %v", got.Entrypoint, got.Cmd, got.CommandMasked)
	}
	masked := map[string]bool{}
	for _, v := range got.Env {
		masked[v.Name] = v.Masked
		if v.Masked != (v.Value == nil) {
			t.Errorf("%s : masked %v mais value %v (spec §7 : null avec masked: true)", v.Name, v.Masked, v.Value)
		}
	}
	if !masked["DB_PASSWORD"] || !masked["CACHE_URL"] || !masked["DB"] || masked["PATH"] || masked["PLAIN"] || len(got.Env) != 5 {
		t.Errorf("masquage : %v", masked)
	}
	if len(got.Ports) != 1 || got.Ports[0].Public != 8080 || len(got.Networks) != 1 || got.Networks[0].IP != "172.18.0.2" {
		t.Errorf("ports, réseaux : %+v %+v", got.Ports, got.Networks)
	}
	if len(got.Mounts) != 1 || got.Mounts[0].Mode != "ro" || got.Mounts[0].Destination != "/etc/app" || len(got.Tmpfs) != 1 {
		t.Errorf("montages : %+v %v", got.Mounts, got.Tmpfs)
	}
}

// Review of tranche 5 (Codex): Docker runs the entrypoint and the command as
// one argv, so a secret flag that ends the entrypoint names the first
// argument of the command. Both are masked together, then split again.
func TestContainerCommandBoundary(t *testing.T) {
	for _, tc := range []struct{ ep, cmd, wantEp, wantCmd []string }{
		{[]string{"redis-server", "--requirepass"}, []string{"s3cr3t-clair", "--port", "6379"},
			[]string{"redis-server", "--requirepass"}, []string{"••••••", "--port", "6379"}},
		{[]string{"app", "--password="}, []string{"s3cr3t-clair"}, []string{"app", "--password="}, []string{"••••••"}},
		{nil, []string{"redis-server", "--requirepass", "s3cr3t-clair"}, []string{}, []string{"redis-server", "--requirepass", "••••••"}},
		{[]string{"/entry.sh"}, nil, []string{"/entry.sh"}, []string{}},
	} {
		f, c := radioFixture(t, false)
		web := f.docker.Infos["web"]
		web.Details.Entrypoint, web.Details.Cmd = tc.ep, tc.cmd
		f.docker.Infos["web"], f.docker.Infos[webID] = web, web
		rec := f.do("GET", "/api/containers/web", "", c)
		if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "s3cr3t-clair") {
			t.Errorf("%q + %q : %d %s", tc.ep, tc.cmd, rec.Code, rec.Body)
			continue
		}
		var got struct {
			Entrypoint, Cmd []string
			CommandMasked   bool
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if strings.Join(got.Entrypoint, "|") != strings.Join(tc.wantEp, "|") || strings.Join(got.Cmd, "|") != strings.Join(tc.wantCmd, "|") ||
			got.Entrypoint == nil || got.Cmd == nil || got.CommandMasked != (len(tc.cmd) > 0) {
			t.Errorf("%q + %q : %q %q %v", tc.ep, tc.cmd, got.Entrypoint, got.Cmd, got.CommandMasked)
		}
	}
}

// Spec v0.2 §5.2 (Q60): labels leave the agent through secrets.Labels only;
// outside com.docker.compose. and org.opencontainers.image., the key alone,
// and no reveal for them.
func TestContainerLabelsNeverLeak(t *testing.T) {
	f, c := radioFixture(t, false)
	web := f.docker.Infos["web"]
	web.Details.Labels = map[string]string{
		"traefik.http.middlewares.auth.basicauth.users": "admin:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/",
		"com.docker.compose.project":                    "shop",
		"com.docker.compose.x":                          "postgres://u:s3cr3t@db/app",
		"org.opencontainers.image.source":               "https://github.com/o/r",
		"backup.dsn":                                    "s3cr3t",
	}
	f.docker.Infos["web"], f.docker.Infos[webID] = web, web
	rec := f.do("GET", "/api/containers/web", "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("statut %d : %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, w := range []string{"s3cr3t", "$apr1$", "IgXLP6ew"} {
		if strings.Contains(body, w) {
			t.Errorf("FUITE : %s dans la réponse", w)
		}
	}
	for _, want := range []string{
		`{"key":"com.docker.compose.project","value":"shop","masked":false}`,
		`{"key":"traefik.http.middlewares.auth.basicauth.users","value":null,"masked":true}`,
		`{"key":"com.docker.compose.x","value":null,"masked":true}`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("%s absent de %s", want, body)
		}
	}
	// No reveal of a label: env/reveal knows only Config.Env.
	r := f.do("POST", "/api/containers/web/env/reveal", `{"name":"backup.dsn"}`, c)
	if r.Code != http.StatusNotFound || strings.Contains(r.Body.String(), "s3cr3t") {
		t.Errorf("label révélé : %d %s", r.Code, r.Body)
	}
}

func TestContainerErrors(t *testing.T) {
	f, c := radioFixture(t, false)
	if rec := f.do("GET", "/api/containers/absent", "", c); rec.Code != http.StatusNotFound || errCode(t, rec) != "not_found" {
		t.Errorf("absent : %d %s", rec.Code, rec.Body)
	}
	if rec := f.do("GET", "/api/containers/..x", "", c); rec.Code != http.StatusNotFound {
		t.Errorf("identifiant invalide : %d", rec.Code)
	}
	f.docker.Err = dockerx.ErrUnavailable
	if rec := f.do("GET", "/api/containers/web", "", c); rec.Code != http.StatusServiceUnavailable || errCode(t, rec) != "docker_unavailable" {
		t.Errorf("Docker indisponible : %d %s", rec.Code, rec.Body)
	}
}

func auditLines(t *testing.T, f *fixture) string {
	t.Helper()
	b, _ := os.ReadFile(f.auditP)
	return string(b)
}

// Spec §6, §7: the value of one variable, journaled (env.reveal) before it
// is sent; the value itself never goes to the log. Read-only mode does not
// forbid it: it is a read.
func TestRevealEnv(t *testing.T) {
	f, c := radioFixture(t, true)
	rec := f.do("POST", "/api/containers/web/env/reveal", `{"name":"DB_PASSWORD"}`, c)
	if rec.Code != http.StatusOK {
		t.Fatalf("statut %d : %s", rec.Code, rec.Body)
	}
	var got struct{ Name, Value string }
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Name != "DB_PASSWORD" || got.Value != "s3cr3t-env" {
		t.Fatalf("révélé : %+v", got)
	}
	log := auditLines(t, f)
	if !strings.Contains(log, `"action":"env.reveal","container":"`+webID+`","path":"DB_PASSWORD","result":"ok"`) {
		t.Errorf("journal : %s", log)
	}
	noSecret(t, log)
	// A variable shown anyway can be asked too, and is journaled the same.
	if rec := f.do("POST", "/api/containers/web/env/reveal", `{"name":"PLAIN"}`, c); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "visible") {
		t.Errorf("variable non masquée : %d %s", rec.Code, rec.Body)
	}
}

func TestRevealEnvRefusals(t *testing.T) {
	f, c := radioFixture(t, false)
	const same = "http://example.com"
	cases := []struct {
		name, body, origin string
		status             int
		code               string
	}{
		{"Origin absente", `{"name":"DB_PASSWORD"}`, "", http.StatusForbidden, "forbidden_origin"},
		{"Origin étrangère", `{"name":"DB_PASSWORD"}`, "http://evil.example", http.StatusForbidden, "forbidden_origin"},
		{"variable absente", `{"name":"NOPE"}`, same, http.StatusNotFound, "not_found"},
		{"casse différente", `{"name":"db_password"}`, same, http.StatusNotFound, "not_found"},
		{"corps invalide", `{"name":`, same, http.StatusNotFound, "not_found"},
		{"données en trop", `{"name":"DB_PASSWORD"}{}`, same, http.StatusNotFound, "not_found"},
	}
	for _, k := range cases {
		r := httptest.NewRequest("POST", "http://example.com/api/containers/web/env/reveal", strings.NewReader(k.body))
		if k.origin != "" {
			r.Header.Set("Origin", k.origin)
		}
		r.AddCookie(c)
		rec := httptest.NewRecorder()
		f.h.ServeHTTP(rec, r)
		if r := rec; r.Code != k.status || errCode(t, r) != k.code {
			t.Errorf("%s : %d %s, attendu %d %s", k.name, r.Code, r.Body, k.status, k.code)
		}
		noSecret(t, rec.Body.String())
	}
	if rec := f.do("POST", "/api/containers/absent/env/reveal", `{"name":"DB_PASSWORD"}`, c); rec.Code != http.StatusNotFound || errCode(t, rec) != "not_found" {
		t.Errorf("conteneur absent : %d %s", rec.Code, rec.Body)
	}
	if log := auditLines(t, f); strings.Contains(log, "env.reveal") {
		t.Errorf("un refus a été journalisé comme une révélation : %s", log)
	}
	// The audit log cannot be written: nothing is revealed.
	f.auditLog.Close()
	if r := f.do("POST", "/api/containers/web/env/reveal", `{"name":"DB_PASSWORD"}`, c); r.Code != http.StatusInternalServerError || strings.Contains(r.Body.String(), "s3cr3t") {
		t.Errorf("journal en échec : %d %s", r.Code, r.Body)
	}
}
