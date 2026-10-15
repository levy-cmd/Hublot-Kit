package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// logLimits of F6 (plan Observation, Q31 to Q33, Q37). Tests shrink them through
// defaultLogLimits, read by New.
type logLimits struct {
	MaxStreams  int           // logs streams open at once for the agent, downloads included
	Burst       int           // lines sent at once to one stream (the initial tail fits)
	Rate        float64       // lines per second refilled after the burst
	DefaultTail int           // lines of the past shown when the view opens
	MaxTail     int           // most lines of the past the view may ask for
	Heartbeat   time.Duration // a comment on a silent stream: proxies, gone clients
}

var defaultLogLimits = logLimits{
	MaxStreams: 8, Burst: 10_000, Rate: 1000, DefaultTail: 500, MaxTail: 10_000, Heartbeat: 15 * time.Second,
}

// maxDownloadLine caps one line of the downloaded file: a line longer than
// this ends with "[…]".
const maxDownloadLine = 1 << 20

// errTooManyStreams: every slot of logLimits.MaxStreams is taken by
// downloads, which are never evicted.
var errTooManyStreams = errors.New("too many open log streams")

// sseWriteTimeout bounds each write of a stream (review I4): a client
// that no longer reads cannot hold the goroutine.
const sseWriteTimeout = 30 * time.Second

// lineID is the SSE ID of a line: its timestamp, its stream, and its rank
// among the lines of that stream with that same timestamp. Within one
// stream Docker's timestamps rise; between stdout and stderr its journal is
// not in timestamp order (two goroutines stamp them), and two streams may
// share a timestamp. Docker's since returns every line, in journal order,
// from the first one that reaches it: resuming asks from maxStreamSkew
// before the timestamp and skips up to the cut line itself (see logsStream).
var lineID = regexp.MustCompile(`^(\d{4}-\d\d-\d\dT[0-9:.]+Z)/(stdout|stderr)/(\d{1,9})$`)

// maxStreamSkew bounds the skip of a resumption whose cut line is gone and
// whose stream stays silent (journal rotated, or an ID from another
// container): a line of the other stream this far past the cut stops it.
// The largest gap measured between stdout and stderr is 2.6 ms; too small a
// bound gives a duplicate, never a hole. A resumption also asks Docker from
// this far before the cut (see logsStream).
const maxStreamSkew = time.Second

// linePos is where a line stands in its stream: its timestamp and rank.
type linePos struct {
	ts   time.Time
	rank int
}

// cutPoint is the last line a client received.
type cutPoint struct {
	stream string
	linePos
}

// resumePoint reads a line ID (Last-Event-ID first: the browser's own
// reconnection, newer than the page's ?after=); ok false when none valid
// (an ID of the former "ts/rank" form is not valid: an ordinary start).
func resumePoint(r *http.Request) (cut cutPoint, ok bool) {
	for _, id := range []string{r.Header.Get("Last-Event-ID"), r.URL.Query().Get("after")} {
		m := lineID.FindStringSubmatch(id)
		if m == nil {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, m[1])
		n, err2 := strconv.Atoi(m[3])
		if err == nil && err2 == nil {
			return cutPoint{m[2], linePos{t, n}}, true
		}
	}
	return cutPoint{}, false
}

// compare orders two positions of one stream.
func (p linePos) compare(q linePos) int {
	if c := p.ts.Compare(q.ts); c != 0 {
		return c
	}
	return p.rank - q.rank
}

// tailOf: the tail parameter, DefaultTail when absent or invalid, at most
// MaxTail.
func (l logLimits) tailOf(r *http.Request) string {
	n, err := strconv.Atoi(r.URL.Query().Get("tail"))
	if err != nil || n < 0 {
		n = l.DefaultTail
	}
	return strconv.Itoa(min(n, l.MaxTail))
}

// streamSlots: at most max logs streams at once (Q33, Q37). When every slot
// is taken, the oldest live stream gives its slot to the newcomer: a client
// gone without FIN keeps its connection for ~15 min (TCP retransmissions),
// and its EventSource reconnects on a new one; the newest stream is almost
// always the living one. A download is never evicted.
type streamSlots struct {
	mu   sync.Mutex
	max  int
	used int
	live []*slot // oldest first
}

type slot struct {
	evicted chan struct{} // closed on eviction
	gone    bool          // evicted: its slot is no longer counted
}

func newStreamSlots(max int) *streamSlots { return &streamSlots{max: max} }

// take reserves a slot (live: an SSE stream, evictable). release frees it,
// once; evicted is closed if a newer stream takes the slot.
func (s *streamSlots) take(live bool) (release func(), evicted <-chan struct{}, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.used >= s.max {
		if len(s.live) == 0 {
			return nil, nil, fmt.Errorf("%w (%d at most, all downloads)", errTooManyStreams, s.max)
		}
		old := s.live[0]
		s.live, s.used, old.gone = s.live[1:], s.used-1, true
		close(old.evicted)
	}
	s.used++
	sl := &slot{evicted: make(chan struct{})}
	if live {
		s.live = append(s.live, sl)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if sl.gone {
				return
			}
			s.used--
			if i := slices.Index(s.live, sl); i >= 0 {
				s.live = slices.Delete(s.live, i, i+1)
			}
		})
	}, sl.evicted, nil
}

// sessionValid: the session that opened the stream still exists (review
// I3: logout and expiry end the streams at the next heartbeat).
func (s *server) sessionValid(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	return err == nil && s.d.Sessions.Valid(c.Value)
}

// inspectByRef checks {id} and resolves the container by Inspect, bounded
// like the file routes. Every later step uses info.ID, the full ID.
func (s *server) inspectByRef(r *http.Request) (dockerx.ContainerInfo, error) {
	id := r.PathValue("id")
	if !containerRef.MatchString(id) {
		return dockerx.ContainerInfo{}, fmt.Errorf("%w: invalid container ID", dockerx.ErrNotFound)
	}
	ctx, cancel := context.WithTimeout(r.Context(), fsTimeout)
	defer cancel()
	return s.d.Docker.Inspect(ctx, id)
}

// sse writes the events of one stream.
type sse struct {
	w http.ResponseWriter
	f *http.ResponseController
}

// deadline: every write of a stream ends within sseWriteTimeout (a writer
// that cannot set one, in tests, writes without).
func (e sse) deadline() { e.f.SetWriteDeadline(time.Now().Add(sseWriteTimeout)) }

func (e sse) event(name, id string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var sb strings.Builder
	if id != "" {
		sb.WriteString("id: " + id + "\n")
	}
	if name != "" {
		sb.WriteString("event: " + name + "\n")
	}
	sb.WriteString("data: ")
	sb.Write(b)
	sb.WriteString("\n\n")
	e.deadline()
	if _, err := e.w.Write([]byte(sb.String())); err != nil {
		return err
	}
	return e.f.Flush()
}

func (e sse) comment(text string) error {
	e.deadline()
	if _, err := e.w.Write([]byte(": " + text + "\n\n")); err != nil {
		return err
	}
	return e.f.Flush()
}

// failure reports err in the stream with the code of spec §6.
func (e sse) failure(err error) {
	_, code := fsErrorStatus(err)
	if code == "internal" || code == "docker_unavailable" {
		slog.Warn("logs stream error", "code", code, "err", err)
	}
	e.event("failure", "", apiError{Error: code, Detail: err.Error()})
}

type logJSON struct {
	TS        string `json:"ts,omitempty"`
	Stream    string `json:"stream"`
	Line      string `json:"line"`
	Truncated bool   `json:"truncated,omitempty"`
}

// bucket: Burst lines at once, then Rate per second.
type bucket struct {
	tokens, max, rate float64
	last              time.Time
}

func (b *bucket) take(now time.Time) bool {
	b.tokens = min(b.max, b.tokens+now.Sub(b.last).Seconds()*b.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// logsStream is GET /logs (F6, spec §6): an SSE stream of the container's
// logs, {ts, stream, line} per event. It answers 200 as soon as the session
// passed; every error comes as a "failure" event (EventSource cannot read
// an HTTP error), the end of the container's stream as "end".
func (s *server) logsStream(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("X-Accel-Buffering", "no") // nginx: no buffering of the stream
	w.WriteHeader(http.StatusOK)
	out := sse{w: w, f: http.NewResponseController(w)}
	out.deadline()
	out.f.Flush()
	lim := s.logLimits

	info, err := s.inspectByRef(r)
	if err != nil {
		out.failure(err)
		return
	}
	release, evicted, err := s.streams.take(true)
	if err != nil {
		out.failure(err)
		return
	}
	defer release()
	opt := dockerx.LogsOptions{Tail: lim.tailOf(r), Follow: true}
	cut, skipping := resumePoint(r)
	// since: what Docker is asked from when resuming, maxStreamSkew before
	// the cut (zero otherwise). Docker cuts a long line into parts, each
	// behind the timestamp of the first, and its since drops every entry
	// before the first one that reaches it: a part sent after that entry
	// with an older timestamp is the end of a line whose start was dropped.
	// Starting a second early brings the start of a line begun within that
	// second back; a line older still comes marked truncated. A whole line
	// stamped before since and written after the first entry kept would
	// need a gap between the streams beyond maxStreamSkew (the cut line,
	// written after it, is a second later); a line of several parts whose
	// start came back is longer than MaxLogLine, so already truncated.
	var since time.Time
	if skipping {
		since = cut.ts.Add(-maxStreamSkew)
		opt = dockerx.LogsOptions{Tail: "all", Since: since.Format(time.RFC3339Nano), Follow: true}
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	rc, err := s.d.Docker.Logs(ctx, info.ID, opt)
	if err != nil {
		out.failure(err)
		return
	}
	lines := make(chan dockerx.LogLine, 64)
	readErr := make(chan error, 1)
	go func() {
		defer close(lines)
		readErr <- dockerx.ReadLogs(rc, info.Tty, dockerx.MaxLogLine, func(l dockerx.LogLine) error {
			select {
			case lines <- l:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	// Leaving (client gone, end, error): the reader stops, Docker's
	// stream is closed, and only then the slot is free.
	defer func() {
		cancel()
		rc.Close()
		for range lines {
		}
	}()

	tick := time.NewTicker(lim.Heartbeat)
	defer tick.Stop()
	b := &bucket{tokens: float64(lim.Burst), max: float64(lim.Burst), rate: lim.Rate, last: time.Now()}
	pos := map[string]linePos{} // per stream: the last line read
	dropped := 0
	flushDropped := func() error {
		if dropped == 0 {
			return nil
		}
		n := dropped
		dropped = 0
		return out.event("dropped", "", map[string]int{"count": n})
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-evicted:
			flushDropped()
			out.event("evicted", "", apiError{Error: "internal", Detail: fmt.Sprintf("stream replaced by a newer one (%d log streams at most)", lim.MaxStreams)})
			return
		case <-tick.C:
			if !s.sessionValid(r) {
				out.event("failure", "", apiError{Error: "unauthorized", Detail: "session ended"})
				return
			}
			if flushDropped() != nil || out.comment("ping") != nil {
				return
			}
		case l, ok := <-lines:
			if !ok {
				if r.Context().Err() != nil {
					return // agent shutting down or client gone: stay silent, the UI reconnects
				}
				if err := <-readErr; err != nil {
					flushDropped()
					out.failure(err)
					return
				}
				if flushDropped() == nil {
					out.event("end", "", struct{}{})
				}
				return
			}
			p, seen := pos[l.Stream]
			if seen && l.TS.Equal(p.ts) {
				p.rank++
			} else {
				p = linePos{l.TS, 0}
			}
			pos[l.Stream] = p
			if skipping {
				// Every line before the cut line, in journal order, was sent
				// (or announced as dropped). A line of the cut's stream past
				// the cut, or of the other stream past it by more than
				// maxStreamSkew, means the cut line is gone (journal rotated):
				// stop skipping there rather than skip the rest.
				if l.Stream != cut.stream {
					if l.TS.Sub(cut.ts) <= maxStreamSkew {
						continue
					}
				} else if c := p.compare(cut.linePos); c <= 0 {
					skipping = c < 0
					continue
				}
				skipping = false
			}
			if !b.take(time.Now()) {
				dropped++
				continue
			}
			if flushDropped() != nil {
				return
			}
			id, ts := "", ""
			if !l.TS.IsZero() {
				ts = l.TS.Format(time.RFC3339Nano)
				id = ts + "/" + l.Stream + "/" + strconv.Itoa(p.rank)
			}
			headLost := !l.TS.IsZero() && l.TS.Before(since) // see since
			if out.event("", id, logJSON{TS: ts, Stream: l.Stream, Line: l.Text, Truncated: l.Truncated || headLost}) != nil {
				return
			}
		}
	}
}

// timedWriter puts a write deadline before each write of a download
// (review 2, Minor 3): a download is never evicted, a client gone without
// FIN must not keep its slot for ~15 min.
type timedWriter struct {
	w http.ResponseWriter
	c *http.ResponseController
	n *int // bytes that reached the client
}

func (t timedWriter) Write(p []byte) (int, error) {
	t.c.SetWriteDeadline(time.Now().Add(sseWriteTimeout))
	n, err := t.w.Write(p)
	*t.n += n
	return n, err
}

// downloadIdle: a download gives up when Docker sends no line for this long
// (same order as files.PutTimeout). It is an idle limit, not a total one: a
// huge journal may take long, a silent Docker may not hold the request and
// its slot.
var downloadIdle = time.Minute

// safeName keeps [A-Za-z0-9_.-] of a container name, for a file name.
var unsafeName = regexp.MustCompile(`[^A-Za-z0-9_.-]`)

// logsDownload is GET /logs/download (F6): every line Docker keeps, as text,
// "<ts> <stream> <line>" per line, as an attachment (spec §7). Errors before
// the first byte are API errors; after it, the file is cut short.
func (s *server) logsDownload(w http.ResponseWriter, r *http.Request) {
	info, err := s.inspectByRef(r)
	if err != nil {
		writeFSError(w, err)
		return
	}
	release, _, err := s.streams.take(false)
	if err != nil {
		writeFSError(w, err)
		return
	}
	defer release()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	idle := time.AfterFunc(downloadIdle, cancel)
	defer idle.Stop()
	rc, err := s.d.Docker.Logs(ctx, info.ID, dockerx.LogsOptions{Tail: "all"})
	if err != nil {
		writeFSError(w, err)
		return
	}
	defer rc.Close()
	name := unsafeName.ReplaceAllString(info.Name, "_") + "-" + time.Now().UTC().Format("20060102-150405") + ".log"
	h := w.Header()
	h.Set("Content-Type", "application/octet-stream")
	h.Set("Content-Disposition", `attachment; filename="`+name+`"`)
	written := 0
	bw := bufio.NewWriterSize(timedWriter{w, http.NewResponseController(w), &written}, 64<<10)
	err = dockerx.ReadLogs(rc, info.Tty, maxDownloadLine, func(l dockerx.LogLine) error {
		idle.Reset(downloadIdle)
		if !l.TS.IsZero() {
			bw.WriteString(l.TS.Format(time.RFC3339Nano) + " ")
		}
		bw.WriteString(l.Stream + " " + l.Text)
		if l.Truncated {
			bw.WriteString(" […]")
		}
		return bw.WriteByte('\n')
	})
	if err == nil {
		err = bw.Flush()
	}
	if err != nil && r.Context().Err() == nil {
		slog.Warn("logs download cut", "container", info.ID, "err", err)
		if written == 0 {
			h.Del("Content-Disposition")
			writeFSError(w, err)
			return
		}
		panic(http.ErrAbortHandler) // cut the connection: the browser shows a failed download
	}
}
