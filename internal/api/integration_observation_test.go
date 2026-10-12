//go:build integration

package api

// F6 and F7 end to end (spec §10): real Docker, real HTTP handler, a real
// HTTP client for the SSE stream. Portable: logs and Inspect need no
// /proc access, Docker Desktop answers from macOS.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

func dockerRun(t *testing.T, args ...string) {
	t.Helper()
	if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
		t.Fatalf("docker %v : %v %s", args, err, out)
	}
}

// countingDocker is the real client; it counts the logs streams Docker
// sent, those the agent closed, and the reads in progress on them, to see on
// a real Docker that a client leaving frees everything (plan T3, for T6).
type countingDocker struct {
	dockerx.Client
	opened, closed, reading atomic.Int32
}

type countingStream struct {
	io.ReadCloser
	d *countingDocker
}

func (d *countingDocker) Logs(ctx context.Context, id string, opt dockerx.LogsOptions) (io.ReadCloser, error) {
	rc, err := d.Client.Logs(ctx, id, opt)
	if err != nil {
		return nil, err
	}
	d.opened.Add(1)
	return &countingStream{ReadCloser: rc, d: d}, nil
}

func (s *countingStream) Read(p []byte) (int, error) {
	s.d.reading.Add(1)
	defer s.d.reading.Add(-1)
	return s.ReadCloser.Read(p)
}

func (s *countingStream) Close() error {
	s.d.closed.Add(1)
	return s.ReadCloser.Close()
}

func TestIntegrationObservation(t *testing.T) {
	// A unique suffix: two runs (or a run and a leftover) never share a name.
	sfx := strconv.FormatInt(time.Now().UnixNano(), 36)
	live, past, none := "hublot-it-obs-live-"+sfx, "hublot-it-obs-past-"+sfx, "hublot-it-obs-none-"+sfx
	exec.Command("docker", "rm", "-f", live, past, none).Run()
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", live, past, none).Run() })
	// 81 without an IP: Docker binds 0.0.0.0 and ::, one mapping for the agent.
	// Bursts of 50 lines per stream: the journal is often out of timestamp
	// order between stdout and stderr, as for a busy service.
	dockerRun(t, "run", "-d", "--name", live, "-e", "DB_PASSWORD=s3cr3t-env", "-e", "PLAIN=visible",
		"-p", "127.0.0.1::80", "-p", "81", "alpine:3", "sh", "-c",
		`echo "start s3cr3t-in-logs"; i=0; while true; do i=$((i+1)); echo "tick $i"; echo "warn $i" >&2; [ $((i % 50)) -ne 0 ] || sleep 0.1; done`)
	// Stopped, with a binding configured without IP nor host port (HostIp "").
	dockerRun(t, "run", "--name", past, "-p", "80", "alpine:3", "sh", "-c", "echo un; echo deux >&2")
	// Host network: Docker keeps the -p in PortBindings but ignores it.
	dockerRun(t, "run", "-d", "--name", none, "--log-driver", "none", "--network", "host",
		"-p", "127.0.0.1::80", "alpine:3", "sleep", "300")
	real, err := dockerx.New()
	if err != nil {
		t.Fatal(err)
	}
	dc := &countingDocker{Client: real}
	f := &fixture{}
	f.init(t, false, dc, nil)
	c := f.session(t)
	radioOf := func(name string) radioJSON {
		t.Helper()
		rec := f.do("GET", "/api/containers/"+name, "", c)
		if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "s3cr3t-env") {
			t.Fatalf("radiographie de %s : %d %s", name, rec.Code, rec.Body)
		}
		var radio radioJSON
		if err := json.Unmarshal(rec.Body.Bytes(), &radio); err != nil {
			t.Fatal(err)
		}
		return radio
	}

	// F7: masked by default, revealed on demand, journaled.
	radio := radioOf(live)
	if radio.Image != "alpine:3" || !radio.State.Running || radio.State.StartedAt == "" || radio.State.FinishedAt != "" ||
		radio.LogDriver != "json-file" || len(radio.Networks) != 1 || radio.Networks[0].IP == "" {
		t.Errorf("radiographie : %+v", radio)
	}
	if p := radio.Ports; len(p) != 2 ||
		p[0].Private != 80 || p[0].Public == 0 || p[0].Type != "tcp" || p[0].IP != "127.0.0.1" ||
		p[1].Private != 81 || p[1].Public == 0 || p[1].Type != "tcp" || p[1].IP != "0.0.0.0" {
		t.Errorf("ports en marche : %+v", p)
	}
	plain := false
	for _, v := range radio.Env {
		if v.Name == "DB_PASSWORD" && (!v.Masked || v.Value != nil) {
			t.Errorf("DB_PASSWORD non masqué : %+v", v)
		}
		if v.Name == "PLAIN" && !v.Masked && v.Value != nil && *v.Value == "visible" {
			plain = true
		}
	}
	if !plain {
		t.Errorf("PLAIN absente ou masquée : %+v", radio.Env)
	}
	rec := f.do("POST", "/api/containers/"+live+"/env/reveal", `{"name":"DB_PASSWORD"}`, c)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "s3cr3t-env") {
		t.Fatalf("révélation : %d %s", rec.Code, rec.Body)
	}
	if log := auditLines(t, f); !strings.Contains(log, `"action":"env.reveal"`) || strings.Contains(log, "s3cr3t-env") {
		t.Errorf("journal : %s", log)
	}
	// Stopped: the configured binding, HostIp "" read as every interface,
	// no host port yet; dates of a finished run.
	radio = radioOf(past)
	if radio.State.Running || radio.State.StartedAt == "" || radio.State.FinishedAt == "" || radio.State.ExitCode != 0 {
		t.Errorf("radiographie arrêtée : %+v", radio.State)
	}
	if p := radio.Ports; len(p) != 1 || p[0] != (dockerx.Port{Private: 80, Type: "tcp", IP: "0.0.0.0"}) {
		t.Errorf("ports à l'arrêt : %+v", p)
	}
	// Host network: Docker keeps the -p in PortBindings but ignores it; the
	// port stays as merely exposed (-p also exposes it): no IP, no host port.
	radio = radioOf(none)
	if radio.NetworkMode != "host" || len(radio.Ports) != 1 || radio.Ports[0] != (dockerx.Port{Private: 80, Type: "tcp"}) ||
		radio.LogDriver != "none" {
		t.Errorf("réseau host : mode %q, ports %+v, pilote %q", radio.NetworkMode, radio.Ports, radio.LogDriver)
	}

	// F6, running container: past lines, then new ones as they come, both
	// streams; then resumption from the last ID.
	s := openSSE(t, f, "/api/containers/"+live+"/logs?tail=5", c, nil)
	// counter: the number of a "tick N" (stdout) or "warn N" (stderr) line.
	counter := func(l logEvent) int {
		_, n, _ := strings.Cut(l.Line, " ")
		v, err := strconv.Atoi(n)
		if err != nil {
			return -1 // "start …"
		}
		return v
	}
	seen := map[string]int{} // stream → highest counter received
	var lastID, lastLine string
	// At least 12 lines, and both streams (a burst comes stream by stream).
	for n := 0; n < 12 || seen["stdout"] <= 0 || seen["stderr"] <= 0; {
		e, ok := s.next(t)
		if !ok {
			t.Fatal("flux fermé trop tôt")
		}
		if e.Comment != "" {
			continue // only lines count
		}
		n++
		l := lineOf(t, e)
		seen[l.Stream] = max(seen[l.Stream], counter(l))
		lastID, lastLine = e.ID, l.Stream+":"+l.Line
	}
	if seen["stdout"] <= 0 || seen["stderr"] <= 0 || lastID == "" {
		t.Fatalf("flux en direct : %v, dernier ID %q", seen, lastID)
	}
	// The client leaves while the container keeps logging: Docker's stream
	// is closed and no read stays blocked on it (the slot is freed after).
	s.cancel()
	deadline := time.Now().Add(5 * time.Second)
	for (dc.closed.Load() != 1 || dc.reading.Load() != 0) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if dc.opened.Load() != 1 || dc.closed.Load() != 1 || dc.reading.Load() != 0 {
		t.Fatalf("départ du client : %d flux ouverts, %d fermés, %d lectures en cours",
			dc.opened.Load(), dc.closed.Load(), dc.reading.Load())
	}
	r := openSSE(t, f, "/api/containers/"+live+"/logs", c, map[string]string{"Last-Event-ID": lastID})
	// Resumption on real lines: neither duplicate nor hole. Docker stamps
	// stdout and stderr in two goroutines, so its journal is not always in
	// timestamp order between streams (a few % of neighbours), and since
	// returns everything from the first line that reaches it: the agent
	// skips up to the cut line itself.
	first := map[string]int{}
	for len(first) < 2 {
		e, ok := r.next(t)
		if !ok {
			t.Fatal("reprise : flux fermé")
		}
		if e.Comment != "" {
			continue
		}
		l := lineOf(t, e)
		if e.ID == "" || e.ID == lastID || l.Stream+":"+l.Line == lastLine {
			t.Fatalf("reprise : %q (%s) déjà reçu en dernier", e.ID, l.Line)
		}
		if _, ok := first[l.Stream]; !ok {
			first[l.Stream] = counter(l)
		}
	}
	for _, st := range []string{"stdout", "stderr"} {
		if n := first[st]; n != seen[st]+1 {
			t.Errorf("reprise de %s : première ligne n° %d, dernière reçue n° %d", st, n, seen[st])
		}
	}
	// The container stops: the stream ends with "end".
	go exec.Command("docker", "stop", "-t", "1", live).Run()
	deadline = time.Now().Add(15 * time.Second)
	ended := false
	for !ended && time.Now().Before(deadline) {
		e, ok := r.next(t)
		if !ok {
			t.Fatal("flux fermé sans événement end")
		}
		switch e.Event {
		case "end":
			ended = true
		case "failure":
			t.Fatalf("échec à l'arrêt : %+v", e)
		}
	}
	if !ended {
		t.Fatal("pas d'événement end 15 s après l'arrêt")
	}

	// F6, stopped container: its past logs, then end.
	events := openSSE(t, f, "/api/containers/"+past+"/logs", c, nil).all(t)
	if len(events) != 3 || events[2].Event != "end" {
		t.Fatalf("conteneur arrêté : %+v", events)
	}
	got := []string{lineOf(t, events[0]).Stream + ":" + lineOf(t, events[0]).Line, lineOf(t, events[1]).Stream + ":" + lineOf(t, events[1]).Line}
	if !(got[0] == "stdout:un" && got[1] == "stderr:deux" || got[0] == "stderr:deux" && got[1] == "stdout:un") {
		t.Errorf("lignes : %q", got)
	}
	dl := f.do("GET", "/api/containers/"+past+"/logs/download", "", c)
	if dl.Code != http.StatusOK || !strings.Contains(dl.Body.String(), " stdout un\n") || !strings.Contains(dl.Body.String(), " stderr deux\n") {
		t.Errorf("téléchargement : %d %q", dl.Code, dl.Body)
	}

	// A driver Docker cannot read: a clear code, in the stream and on the download.
	if code := failureOf(t, openSSE(t, f, "/api/containers/"+none+"/logs", c, nil).all(t)); code != "logs_unavailable" {
		t.Errorf("--log-driver none : %s", code)
	}
	if rec := f.do("GET", "/api/containers/"+none+"/logs/download", "", c); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("--log-driver none, téléchargement : %d %s", rec.Code, rec.Body)
	}
}
