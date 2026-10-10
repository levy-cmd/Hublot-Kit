package api

// Tests of the logs routes (F6, spec §6), written before the code. The fake
// Docker serves raw logs streams (multiplexed frames, or raw with a TTY);
// LogsGate keeps a stream open like a running container. A real HTTP
// server reads the SSE stream as a browser would, event by event.

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

const (
	lts1 = "2026-10-10T07:14:22.769046552Z"
	lts2 = "2026-10-10T07:14:23.775474053Z"
	lts3 = "2026-10-10T07:14:24.000000001Z"
)

// mux is one frame of Docker's multiplexed logs stream.
func mux(stream byte, payload string) []byte {
	h := make([]byte, 8, 8+len(payload))
	h[0] = stream
	binary.BigEndian.PutUint32(h[4:], uint32(len(payload)))
	return append(h, payload...)
}

func muxAll(fs ...[]byte) []byte { return slices.Concat(fs...) }

// sseEvent is one event of the stream; a comment line is Comment.
type sseEvent struct {
	ID, Event, Data, Comment string
}

// logEvent is the data of a line event.
type logEvent struct {
	TS        string `json:"ts"`
	Stream    string `json:"stream"`
	Line      string `json:"line"`
	Truncated bool   `json:"truncated"`
}

// sseClient opens path on a real server with the session cookie; next
// reads one event (or comment), failing after 5 s.
type sseClient struct {
	res    *http.Response
	r      *bufio.Reader
	cancel context.CancelFunc
}

func openSSE(t *testing.T, f *fixture, path string, c *http.Cookie, headers map[string]string) *sseClient {
	t.Helper()
	srv := httptest.NewServer(f.h)
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+path, nil)
	if c != nil {
		req.AddCookie(c)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); res.Body.Close() })
	return &sseClient{res: res, r: bufio.NewReader(res.Body), cancel: cancel}
}

func (s *sseClient) next(t *testing.T) (sseEvent, bool) {
	t.Helper()
	type result struct {
		e  sseEvent
		ok bool
	}
	done := make(chan result, 1)
	go func() {
		var e sseEvent
		for {
			line, err := s.r.ReadString('\n')
			if err != nil {
				done <- result{e, false}
				return
			}
			line = strings.TrimSuffix(line, "\n")
			switch {
			case line == "":
				if e != (sseEvent{}) {
					done <- result{e, true}
					return
				}
			case strings.HasPrefix(line, ":"):
				done <- result{sseEvent{Comment: strings.TrimSpace(line[1:])}, true}
				return
			case strings.HasPrefix(line, "id: "):
				e.ID = line[4:]
			case strings.HasPrefix(line, "event: "):
				e.Event = line[7:]
			case strings.HasPrefix(line, "data: "):
				e.Data = line[6:]
			case strings.HasPrefix(line, "retry: "):
			default:
				done <- result{sseEvent{Event: "MALFORMED", Data: line}, true}
				return
			}
		}
	}()
	select {
	case r := <-done:
		return r.e, r.ok
	case <-time.After(5 * time.Second):
		t.Fatal("aucun événement en 5 s")
		return sseEvent{}, false
	}
}

// all reads the events up to the end of the stream, comments left out.
func (s *sseClient) all(t *testing.T) []sseEvent {
	t.Helper()
	var out []sseEvent
	for {
		e, ok := s.next(t)
		if !ok {
			return out
		}
		if e.Comment == "" {
			out = append(out, e)
		}
	}
}

func lineOf(t *testing.T, e sseEvent) logEvent {
	t.Helper()
	if e.Event != "" {
		t.Fatalf("événement %q, attendu une ligne : %+v", e.Event, e)
	}
	var l logEvent
	if err := json.Unmarshal([]byte(e.Data), &l); err != nil {
		t.Fatalf("data %q : %v", e.Data, err)
	}
	return l
}

func failureOf(t *testing.T, events []sseEvent) string {
	t.Helper()
	last := events[len(events)-1]
	if last.Event != "failure" {
		t.Fatalf("dernier événement %+v, attendu failure", last)
	}
	var body struct{ Error string }
	json.Unmarshal([]byte(last.Data), &body)
	return body.Error
}

// logsFixture: the fake "web" container with a stopped container's logs.
func logsFixture(t *testing.T) (*fixture, *http.Cookie) {
	t.Helper()
	f := newFixture(t, false)
	f.docker.LogStreams = map[string][]byte{webID: muxAll(
		mux(1, lts1+" out1\n"),
		mux(2, lts1+" err1\n"),
		mux(1, lts2+" out2\n"),
	)}
	return f, f.session(t)
}

// smallLimits shrinks the log limits for one test.
func smallLimits(t *testing.T, change func(*logLimits)) {
	t.Helper()
	old := defaultLogLimits
	t.Cleanup(func() { defaultLogLimits = old })
	change(&defaultLogLimits)
}

func TestLogsNeedSession(t *testing.T) {
	f, _ := logsFixture(t)
	for _, p := range []string{"/api/containers/web/logs", "/api/containers/web/logs/download"} {
		if rec := f.do("GET", p, "", nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s sans session : %d", p, rec.Code)
		}
	}
	if len(f.docker.LogsCalls()) != 0 {
		t.Fatal("Docker appelé sans session")
	}
}

// A stopped container: its past logs, then "end"; the stream closes and
// the browser must not reconnect (the UI closes it on "end").
func TestLogsStreamPastLines(t *testing.T) {
	f, c := logsFixture(t)
	s := openSSE(t, f, "/api/containers/web/logs", c, nil)
	if ct := s.res.Header.Get("Content-Type"); s.res.StatusCode != 200 || ct != "text/event-stream" {
		t.Fatalf("statut %d, Content-Type %q", s.res.StatusCode, ct)
	}
	if s.res.Header.Get("Cache-Control") != "no-store" || s.res.Header.Get("X-Accel-Buffering") != "no" {
		t.Errorf("en-têtes : %v", s.res.Header)
	}
	events := s.all(t)
	if len(events) != 4 || events[3].Event != "end" {
		t.Fatalf("événements : %+v", events)
	}
	want := []logEvent{{lts1, "stdout", "out1", false}, {lts1, "stderr", "err1", false}, {lts2, "stdout", "out2", false}}
	for i, w := range want {
		if got := lineOf(t, events[i]); got != w {
			t.Errorf("ligne %d : %+v, attendu %+v", i, got, w)
		}
	}
	// The ID of a line: its timestamp, and its rank among the lines of the
	// same timestamp (resumption, spec §3).
	if ids := []string{events[0].ID, events[1].ID, events[2].ID}; !slices.Equal(ids, []string{lts1 + "/0", lts1 + "/1", lts2 + "/0"}) {
		t.Errorf("identifiants : %q", ids)
	}
	if calls := f.docker.LogsCalls(); !slices.Equal(calls, []dockerx.LogsOptions{{Tail: "500", Follow: true}}) {
		t.Errorf("appels : %+v", calls)
	}
}

func TestLogsTTY(t *testing.T) {
	f, c := logsFixture(t)
	tty := dockerx.ContainerInfo{ID: strings.Repeat("7", 64), Name: "tty", Running: true, Pid: 7, Tty: true}
	f.docker.Infos["tty"] = tty
	f.docker.LogStreams[tty.ID] = []byte(lts1 + " un\r\n" + lts2 + " deux\r\n")
	events := openSSE(t, f, "/api/containers/tty/logs", c, nil).all(t)
	if len(events) != 3 || lineOf(t, events[0]).Line != "un" || lineOf(t, events[1]).Stream != "stdout" {
		t.Fatalf("TTY : %+v", events)
	}
}

func TestLogsTail(t *testing.T) {
	for q, want := range map[string]string{
		"": "500", "?tail=20": "20", "?tail=0": "0", "?tail=99999": "10000", "?tail=abc": "500", "?tail=-1": "500",
	} {
		f, c := logsFixture(t)
		openSSE(t, f, "/api/containers/web/logs"+q, c, nil).all(t)
		if calls := f.docker.LogsCalls(); len(calls) != 1 || calls[0].Tail != want || calls[0].Since != "" {
			t.Errorf("%q : %+v, attendu Tail %s", q, calls, want)
		}
	}
}

// Resumption after a cut (browser: Last-Event-ID; UI after a pause:
// ?after=): Docker's since is inclusive, so the lines of the same timestamp
// already sent are skipped by their rank.
func TestLogsResume(t *testing.T) {
	stream := muxAll(mux(1, lts2+" a\n"), mux(1, lts2+" b\n"), mux(2, lts3+" c\n"))
	for name, h := range map[string]map[string]string{
		"Last-Event-ID": {"Last-Event-ID": lts2 + "/0"},
		"after":         nil,
		"les deux":      {"Last-Event-ID": lts2 + "/0"},
	} {
		f, c := logsFixture(t)
		f.docker.LogStreams[webID] = stream
		path := "/api/containers/web/logs"
		switch name {
		case "after":
			path += "?after=" + lts2 + "/0"
		case "les deux":
			path += "?after=" + lts1 + "/0" // the header, newer, wins
		}
		events := openSSE(t, f, path, c, h).all(t)
		if len(events) != 3 || lineOf(t, events[0]).Line != "b" || events[0].ID != lts2+"/1" ||
			lineOf(t, events[1]).Line != "c" || events[1].ID != lts3+"/0" {
			t.Errorf("%s : %+v", name, events)
		}
		if calls := f.docker.LogsCalls(); len(calls) != 1 || calls[0].Since != lts2 || calls[0].Tail != "all" {
			t.Errorf("%s : appels %+v", name, calls)
		}
	}
	// A malformed ID is ignored: an ordinary start.
	f, c := logsFixture(t)
	openSSE(t, f, "/api/containers/web/logs?after=hier/0", c, nil).all(t)
	if calls := f.docker.LogsCalls(); calls[0].Since != "" || calls[0].Tail != "500" {
		t.Errorf("identifiant invalide : %+v", calls)
	}
}

// Errors come inside the stream (EventSource cannot read an HTTP error),
// as a "failure" event with the code of spec §6, then the stream closes.
func TestLogsFailures(t *testing.T) {
	cases := []struct {
		name, path string
		set        func(*dockerx.Fake)
		code       string
	}{
		{"conteneur absent", "/api/containers/absent/logs", nil, "not_found"},
		{"identifiant invalide", "/api/containers/..x/logs", nil, "not_found"},
		{"pilote sans lecture", "/api/containers/web/logs", func(d *dockerx.Fake) { d.LogsErr = dockerx.ErrNoLogs }, "logs_unavailable"},
		{"Docker indisponible", "/api/containers/web/logs", func(d *dockerx.Fake) { d.Err = dockerx.ErrUnavailable }, "docker_unavailable"},
		{"erreur du démon en cours de flux", "/api/containers/web/logs", func(d *dockerx.Fake) {
			d.LogStreams[webID] = muxAll(mux(1, lts1+" avant\n"), mux(3, "log file rotated"))
		}, "docker_unavailable"},
	}
	for _, k := range cases {
		f, c := logsFixture(t)
		if k.set != nil {
			k.set(f.docker)
		}
		s := openSSE(t, f, k.path, c, nil)
		events := s.all(t)
		if s.res.StatusCode != 200 || len(events) == 0 {
			t.Errorf("%s : statut %d, %+v", k.name, s.res.StatusCode, events)
			continue
		}
		if code := failureOf(t, events); code != k.code {
			t.Errorf("%s : %s, attendu %s", k.name, code, k.code)
		}
	}
}

// Q33: the client leaves (tab closed, navigation): Docker's stream is closed,
// even while the container keeps running. The slot itself: TestStreamSlots.
func TestLogsClientGone(t *testing.T) {
	f, c := logsFixture(t)
	f.docker.LogsGate = make(chan struct{}) // running: the stream never ends
	s := openSSE(t, f, "/api/containers/web/logs", c, nil)
	if e, _ := s.next(t); lineOf(t, e).Line != "out1" {
		t.Fatalf("premier événement : %+v", e)
	}
	s.cancel()
	deadline := time.Now().Add(2 * time.Second)
	for f.docker.LogsCloses.Load() != 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if f.docker.LogsCloses.Load() != 1 {
		t.Fatal("le flux de Docker n'est pas fermé après le départ du client")
	}
}

// Q33, Q37: slots are counted per stream; when they are all taken, the
// oldest live stream gives its slot (a client gone without FIN keeps one
// for ~15 min, and its EventSource reconnects on a new connection). A
// download is never evicted; with only downloads, the newcomer is refused.
func TestStreamSlots(t *testing.T) {
	s := newStreamSlots(2)
	relA, evA, err := s.take(true)
	if err != nil {
		t.Fatal(err)
	}
	relD, evD, _ := s.take(false)
	relB, evB, err := s.take(true) // full: A, the oldest live stream, goes
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-evA:
	default:
		t.Fatal("A doit être évincé")
	}
	for name, ev := range map[string]<-chan struct{}{"téléchargement": evD, "B": evB} {
		select {
		case <-ev:
			t.Errorf("%s évincé à tort", name)
		default:
		}
	}
	relA() // the evicted stream returns: its slot is not counted twice
	relA()
	if _, _, err := s.take(false); err != nil { // B is evicted for it
		t.Fatalf("téléchargement : %v", err)
	}
	if _, _, err := s.take(true); !errors.Is(err, errTooManyStreams) { // two downloads: refused
		t.Fatalf("plein de téléchargements : %v", err)
	}
	relB()
	relD()
	if _, _, err := s.take(true); err != nil {
		t.Fatalf("après départ du téléchargement : %v", err)
	}
}

// Q33, Q37: through the routes, the newest stream wins; the evicted one
// is told ("evicted", so the UI does not reconnect) and closed.
func TestLogsEvictsOldest(t *testing.T) {
	smallLimits(t, func(l *logLimits) { l.MaxStreams = 2 })
	f, c := logsFixture(t)
	f.docker.LogsGate = make(chan struct{})
	a := openSSE(t, f, "/api/containers/web/logs", c, nil)
	a.next(t)
	b := openSSE(t, f, "/api/containers/web/logs", c, nil)
	b.next(t)
	third := openSSE(t, f, "/api/containers/web/logs", c, nil)
	if e, _ := third.next(t); lineOf(t, e).Line != "out1" {
		t.Fatalf("troisième flux : %+v", e)
	}
	var last sseEvent
	for {
		e, ok := a.next(t)
		if !ok {
			break
		}
		last = e
	}
	if last.Event != "evicted" {
		t.Fatalf("le plus ancien doit être évincé : dernier événement %+v", last)
	}
	close(f.docker.LogsGate)
	if events := b.all(t); events[len(events)-1].Event != "end" {
		t.Fatalf("B ne doit pas être évincé : %+v", events)
	}
}

// Downloads count in the slots: one evicts a live stream; with every slot
// held by downloads, a new stream is refused, and says why.
func TestLogsDownloadSlots(t *testing.T) {
	smallLimits(t, func(l *logLimits) { l.MaxStreams = 1 })
	f, c := logsFixture(t)
	f.docker.LogsGate = make(chan struct{})
	srv := httptest.NewServer(f.h)
	t.Cleanup(srv.Close)
	download := func() chan int {
		done := make(chan int, 1)
		go func() {
			req, _ := http.NewRequest("GET", srv.URL+"/api/containers/web/logs/download", nil)
			req.AddCookie(c)
			res, err := srv.Client().Do(req)
			if err != nil {
				done <- 0
				return
			}
			io.Copy(io.Discard, res.Body)
			res.Body.Close()
			done <- res.StatusCode
		}()
		return done
	}
	waitCalls := func(n int) {
		deadline := time.Now().Add(2 * time.Second)
		for len(f.docker.LogsCalls()) < n && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
	}
	a := openSSE(t, f, "/api/containers/web/logs", c, nil)
	a.next(t)
	d := download() // takes A's slot
	waitCalls(2)
	if events := a.all(t); events[len(events)-1].Event != "evicted" {
		t.Fatalf("flux évincé par un téléchargement : %+v", events)
	}
	refused := openSSE(t, f, "/api/containers/web/logs", c, nil).all(t)
	if failureOf(t, refused) != "internal" || !strings.Contains(refused[len(refused)-1].Data, "flux de logs") {
		t.Fatalf("créneaux pris par des téléchargements : %+v", refused)
	}
	close(f.docker.LogsGate)
	if code := <-d; code != http.StatusOK {
		t.Fatalf("téléchargement : %d", code)
	}
}

// Review I3: logging out, or the session expiring, ends the streams it
// opened: the next heartbeat sees it, says "unauthorized" and closes.
func TestLogsSessionEnds(t *testing.T) {
	smallLimits(t, func(l *logLimits) { l.Heartbeat = 20 * time.Millisecond })
	f, c := logsFixture(t)
	f.docker.LogsGate = make(chan struct{})
	s := openSSE(t, f, "/api/containers/web/logs", c, nil)
	s.next(t)
	if rec := f.do("POST", "/api/logout", "", c); rec.Code != http.StatusNoContent {
		t.Fatalf("déconnexion : %d", rec.Code)
	}
	var last sseEvent
	for end := time.Now().Add(2 * time.Second); time.Now().Before(end); {
		e, ok := s.next(t) // a ping every 20 ms: the loop stays bounded
		if !ok {
			break
		}
		if e.Comment == "" {
			last = e
		}
	}
	if last.Event != "failure" || failureOf(t, []sseEvent{last}) != "unauthorized" {
		t.Fatalf("flux encore ouvert 2 s après la déconnexion, dernier événement %+v", last)
	}
	deadline := time.Now().Add(2 * time.Second)
	for f.docker.LogsCloses.Load() != 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if f.docker.LogsCloses.Load() != 1 {
		t.Fatal("flux de Docker resté ouvert après la déconnexion")
	}
}

// deadlineWriter counts the write deadlines the handler sets (through
// http.ResponseController), then passes them on.
type deadlineWriter struct {
	http.ResponseWriter
	n *atomic.Int32
}

func (d deadlineWriter) SetWriteDeadline(t time.Time) error {
	d.n.Add(1)
	return http.NewResponseController(d.ResponseWriter).SetWriteDeadline(t)
}

func (d deadlineWriter) Unwrap() http.ResponseWriter { return d.ResponseWriter }

// Review I4: every write of a stream, and of a download, has a deadline: a
// client that no longer reads cannot hold the goroutine nor the slot.
func TestLogsWriteDeadline(t *testing.T) {
	f, c := logsFixture(t)
	n := new(atomic.Int32)
	h := f.h
	f.h = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.ServeHTTP(deadlineWriter{w, n}, r) })
	events := openSSE(t, f, "/api/containers/web/logs", c, nil).all(t)
	if len(events) != 4 || n.Load() < int32(len(events)) {
		t.Fatalf("%d délais pour %d événements", n.Load(), len(events))
	}
	// Review 2, Minor 3: the download too (it is never evicted).
	n.Store(0)
	if rec := f.do("GET", "/api/containers/web/logs/download", "", c); rec.Code != http.StatusOK || n.Load() == 0 {
		t.Fatalf("téléchargement : %d, %d délais", rec.Code, n.Load())
	}
}

// Q31: beyond the burst, at most Rate lines per second; the lines left out
// are counted and announced ("dropped"), never silently lost.
func TestLogsRateLimit(t *testing.T) {
	smallLimits(t, func(l *logLimits) {
		l.Burst, l.Rate = 3, 0
	})
	f, c := logsFixture(t)
	var frames [][]byte
	for i := 0; i < 5; i++ {
		frames = append(frames, mux(1, lts1+" x\n"))
	}
	f.docker.LogStreams[webID] = muxAll(frames...)
	events := openSSE(t, f, "/api/containers/web/logs", c, nil).all(t)
	if len(events) != 5 || events[3].Event != "dropped" || events[3].Data != `{"count":2}` || events[4].Event != "end" {
		t.Fatalf("événements : %+v", events)
	}
	// The ranks go on through the dropped lines: a resumption skips them.
	if events[2].ID != lts1+"/2" {
		t.Errorf("identifiant : %q", events[2].ID)
	}
}

// Codex: resuming after lines the limiter dropped, with shared timestamps:
// the lines sent are never sent again; the dropped ones come back (they
// were only announced, never shown).
func TestLogsResumeAfterDrops(t *testing.T) {
	smallLimits(t, func(l *logLimits) { l.Burst, l.Rate = 2, 0 })
	f, c := logsFixture(t)
	f.docker.LogStreams[webID] = muxAll(mux(1, lts1+" a\n"), mux(2, lts1+" b\n"), mux(1, lts1+" c\n"), mux(1, lts2+" d\n"))
	events := openSSE(t, f, "/api/containers/web/logs", c, nil).all(t)
	if len(events) != 4 || events[1].ID != lts1+"/1" || events[2].Data != `{"count":2}` {
		t.Fatalf("premier flux : %+v", events)
	}
	events = openSSE(t, f, "/api/containers/web/logs", c, map[string]string{"Last-Event-ID": events[1].ID}).all(t)
	if len(events) != 3 || lineOf(t, events[0]).Line != "c" || events[0].ID != lts1+"/2" ||
		lineOf(t, events[1]).Line != "d" || events[1].ID != lts2+"/0" || events[2].Event != "end" {
		t.Fatalf("reprise : %+v", events)
	}
}

// A silent stream gets a comment line now and then: proxies keep it open,
// and a gone client is noticed.
func TestLogsHeartbeat(t *testing.T) {
	smallLimits(t, func(l *logLimits) { l.Heartbeat = 20 * time.Millisecond })
	f, c := logsFixture(t)
	f.docker.LogStreams[webID] = nil
	f.docker.LogsGate = make(chan struct{})
	s := openSSE(t, f, "/api/containers/web/logs", c, nil)
	if e, _ := s.next(t); e.Comment != "ping" {
		t.Fatalf("attendu un commentaire ping : %+v", e)
	}
}

// A line is JSON in one data field: no raw CR or LF that would cut the
// event, no markup that would matter (the UI escapes anyway).
func TestLogsEventEscaping(t *testing.T) {
	f, c := logsFixture(t)
	f.docker.LogStreams[webID] = mux(1, lts1+" a\rb </script>\xff\n")
	events := openSSE(t, f, "/api/containers/web/logs", c, nil).all(t)
	if len(events) != 2 || events[0].Event == "MALFORMED" {
		t.Fatalf("événements : %+v", events)
	}
	if l := lineOf(t, events[0]); l.Line != "a\rb </script>�" {
		t.Fatalf("ligne : %q", l.Line)
	}
}

func TestLogsDownload(t *testing.T) {
	f, c := logsFixture(t)
	rec := f.do("GET", "/api/containers/web/logs/download", "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("statut %d : %s", rec.Code, rec.Body)
	}
	h := rec.Header()
	if h.Get("Content-Type") != "application/octet-stream" ||
		!strings.HasPrefix(h.Get("Content-Disposition"), `attachment; filename="web-`) ||
		!strings.HasSuffix(h.Get("Content-Disposition"), `.log"`) {
		t.Errorf("en-têtes : %v", h)
	}
	want := lts1 + " stdout out1\n" + lts1 + " stderr err1\n" + lts2 + " stdout out2\n"
	if rec.Body.String() != want {
		t.Errorf("corps %q, attendu %q", rec.Body, want)
	}
	if calls := f.docker.LogsCalls(); !slices.Equal(calls, []dockerx.LogsOptions{{Tail: "all"}}) {
		t.Errorf("appels : %+v", calls)
	}
	// Errors before the first byte are ordinary API errors.
	f.docker.LogsErr = dockerx.ErrNoLogs
	if rec := f.do("GET", "/api/containers/web/logs/download", "", c); rec.Code != http.StatusUnprocessableEntity || errCode(t, rec) != "logs_unavailable" {
		t.Errorf("pilote sans lecture : %d %s", rec.Code, rec.Body)
	}
	if rec := f.do("GET", "/api/containers/absent/logs/download", "", c); rec.Code != http.StatusNotFound {
		t.Errorf("absent : %d", rec.Code)
	}
}

// The download keeps a long line whole up to maxDownloadLine, the live
// view cuts it at dockerx.MaxLogLine.
func TestLogsLongLine(t *testing.T) {
	f, c := logsFixture(t)
	long := strings.Repeat("a", dockerx.MaxLogLine+10)
	f.docker.LogStreams[webID] = mux(1, lts1+" "+long+"\n")
	events := openSSE(t, f, "/api/containers/web/logs", c, nil).all(t)
	if l := lineOf(t, events[0]); len(l.Line) != dockerx.MaxLogLine || !l.Truncated {
		t.Errorf("vue en direct : %d octets, tronquée %v", len(l.Line), l.Truncated)
	}
	rec := f.do("GET", "/api/containers/web/logs/download", "", c)
	if !strings.Contains(rec.Body.String(), " stdout "+long+"\n") {
		t.Errorf("téléchargement : %d octets", rec.Body.Len())
	}
}

// Review I1: a download cut by a Docker error before the first byte is an
// API error (code of spec §6), never a 200 with an empty or partial file.
func TestLogsDownloadErrorBeforeFirstByte(t *testing.T) {
	f, c := logsFixture(t)
	f.docker.LogStreams[webID] = muxAll(mux(1, lts1+" avant\n"), mux(3, "log file rotated"))
	rec := f.do("GET", "/api/containers/web/logs/download", "", c)
	if rec.Code != http.StatusServiceUnavailable || errCode(t, rec) != "docker_unavailable" {
		t.Fatalf("erreur avant le premier octet : %d %s", rec.Code, rec.Body)
	}
}

// Review I1: once bytes are out, the connection is cut (no final chunk), so
// the browser shows a failed download instead of a complete-looking file.
func TestLogsDownloadErrorAfterFirstByte(t *testing.T) {
	f, c := logsFixture(t)
	big := strings.Repeat("x", 1000)
	var frames [][]byte
	for i := 0; i < 200; i++ { // 200 KB: beyond the 64 KiB buffer
		frames = append(frames, mux(1, lts1+" "+big+"\n"))
	}
	frames = append(frames, mux(3, "log file rotated"))
	f.docker.LogStreams[webID] = muxAll(frames...)
	srv := httptest.NewServer(f.h)
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest("GET", srv.URL+"/api/containers/web/logs/download", nil)
	req.AddCookie(c)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if _, err := io.ReadAll(res.Body); err == nil {
		t.Fatal("fichier tronqué présenté comme complet")
	}
}

// Codex: Docker silent before any byte: the download gives up after
// downloadIdle, answers an API error, and frees its slot.
func TestLogsDownloadIdle(t *testing.T) {
	old := downloadIdle
	downloadIdle = 100 * time.Millisecond
	t.Cleanup(func() { downloadIdle = old })
	f, c := logsFixture(t)
	f.docker.LogStreams[webID] = nil
	f.docker.LogsGate = make(chan struct{})
	start := time.Now()
	rec := f.do("GET", "/api/containers/web/logs/download", "", c)
	if rec.Code == http.StatusOK || time.Since(start) > 3*time.Second {
		t.Fatalf("%d après %v : %s", rec.Code, time.Since(start), rec.Body)
	}
	if f.docker.LogsCloses.Load() != 1 {
		t.Fatal("flux de Docker resté ouvert")
	}
}
