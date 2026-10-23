package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/audit"
	"github.com/levy-cmd/hublot-kit/internal/auth"
	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/files"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
	"github.com/levy-cmd/hublot-kit/internal/secrets"
)

type Deps struct {
	Docker dockerx.Client
	// Views is the cache of the indexes of stopped containers; nil:
	// fsview.NewViews(Docker, fsview.DefaultOptions()).
	Views *fsview.Views
	// OpenFS opens the file view of a container by full ID; nil means
	// Views.For. API tests give an in-memory view.
	OpenFS       func(ctx context.Context, id string) (fsview.FS, error)
	Verifier     *auth.Verifier
	Sessions     *auth.Sessions
	Limiter      *auth.Limiter
	Audit        *audit.Log
	Version      string
	ReadOnly     bool
	TLS          bool
	PublicOrigin string       // normalized, or ""
	Static       http.Handler // embedded UI; nil in API tests
	ScriptHashes []string     // CSP sources of the UI's inline scripts (web.ScriptHashes)
}

// logClock is the clock used by throttleLog; tests may replace it before New.
var logClock = time.Now

// throttleSweepThreshold is the map size above which throttleLog drops stale IPs.
var throttleSweepThreshold = 1024

// throttleLog limits a repeated audit line to once per IP per minute.
type throttleLog struct {
	mu        sync.Mutex
	last      map[string]time.Time
	now       func() time.Time
	lastSweep time.Time
}

func (t *throttleLog) shouldLog(ip string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	if at, ok := t.last[ip]; ok && now.Sub(at) < time.Minute {
		return false
	}
	if len(t.last) > throttleSweepThreshold && now.Sub(t.lastSweep) >= time.Minute {
		t.lastSweep = now
		for k, at := range t.last {
			if now.Sub(at) >= time.Minute {
				delete(t.last, k)
			}
		}
	}
	t.last[ip] = now
	return true
}

type server struct {
	d        Deps
	throttle *throttleLog
	loginLog *loginBudget
	guard    *fsGuard
	files    *files.Writer // the only writer (spec §7: read-only applied there)
	// F6: limits of the logs streams, and one slot per stream open.
	logLimits logLimits
	streams   *streamSlots
}

// Spec §13 (FUSE): at most 4 file operations at once per container, each one
// given up after 10 s.
const (
	fsMaxPerContainer = 4
	fsTimeout         = 10 * time.Second
)

func New(d Deps) http.Handler {
	if d.Views == nil {
		d.Views = fsview.NewViews(d.Docker, fsview.DefaultOptions())
	}
	if d.OpenFS == nil {
		d.OpenFS = d.Views.For
	}
	s := &server{
		d:        d,
		throttle: &throttleLog{last: map[string]time.Time{}, now: logClock},
		loginLog: &loginBudget{now: logClock},
		guard:    newFSGuard(fsMaxPerContainer, fsTimeout),
		files: &files.Writer{ReadOnly: d.ReadOnly, Docker: d.Docker, Audit: d.Audit,
			Open: d.OpenFS, Forget: d.Views.Forget},
		logLimits: defaultLogLimits,
		streams:   newStreamSlots(defaultLogLimits.MaxStreams),
	}
	session := func(h http.HandlerFunc) http.Handler { return requireSession(d.Sessions, h) }

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.logout)
	mux.Handle("GET /api/meta", session(s.meta))
	mux.Handle("GET /api/containers", session(s.containers))
	mux.Handle("GET /api/containers/{id}", session(s.container))
	mux.Handle("POST /api/containers/{id}/env/reveal", session(s.revealEnv))
	mux.Handle("GET /api/containers/{id}/fs", session(s.fsList))
	mux.Handle("GET /api/containers/{id}/fs/file", session(s.fsFile))
	mux.Handle("GET /api/containers/{id}/fs/download", session(s.fsDownload))
	mux.Handle("PUT /api/containers/{id}/fs/file", session(longBody(s.fsEdit)))
	mux.Handle("POST /api/containers/{id}/fs/upload", session(longBody(s.fsUpload)))
	mux.Handle("GET /api/containers/{id}/changes", session(s.changes))
	mux.Handle("GET /api/containers/{id}/logs", session(s.logsStream))
	mux.Handle("GET /api/containers/{id}/logs/download", session(s.logsDownload))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "")
	})
	if d.Static != nil {
		mux.Handle("/", d.Static)
	}
	return securityHeaders(originChecker{public: d.PublicOrigin, tls: d.TLS}.wrap(mux), d.ScriptHashes)
}

// Delays of the agent's HTTP server (v0.1.2, audit: only ReadHeaderTimeout
// was set, and 1 000 connections held a login body that never came).
// Variables: tests shorten them.
var (
	readHeaderTimeout = 10 * time.Second
	// readTimeout bounds the reading of a whole request, body included;
	// longBody extends it for a logged-in edit or upload. Once nothing is
	// left to read, net/http lifts it (its background read has no
	// deadline): a logs stream or a long download is not cut.
	readTimeout = 15 * time.Second
	idleTimeout = time.Minute
	// uploadReadTimeout: reading the body of an edit or an upload, 32 MiB
	// at most (about 55 KiB/s).
	uploadReadTimeout = 10 * time.Minute
)

// ConfigureServer sets the delays of the agent's server. No WriteTimeout: it
// would cut the logs stream and long downloads, whose writes have their own
// deadlines.
func ConfigureServer(srv *http.Server) {
	srv.ReadHeaderTimeout = readHeaderTimeout
	srv.ReadTimeout = readTimeout
	srv.IdleTimeout = idleTimeout
}

// longBody extends the read delay of an edit or an upload. Registered
// behind the session check: without a session, the short delay stays.
func longBody(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.NewResponseController(w).SetReadDeadline(time.Now().Add(uploadReadTimeout))
		h(w, r)
	}
}

// login takes the attempt before reading the body, in one step (v0.1.2,
// audit: checked before, counted after, a burst got 40 tokens checked
// instead of 5), and gives it back on success.
func (s *server) login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	at, ok := s.d.Limiter.Take(ip)
	if !ok {
		if s.throttle.shouldLog(ip) {
			s.auditLogin(audit.Event{Action: "login.fail", Result: "rate_limited", Remote: ip})
		}
		writeError(w, http.StatusTooManyRequests, "rate_limited", "")
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
	err := dec.Decode(&body)
	if err == nil && dec.Decode(&struct{}{}) != io.EOF {
		err = errors.New("trailing data")
	}
	if err != nil || !s.d.Verifier.Check(strings.TrimSpace(body.Token)) {
		s.auditLogin(audit.Event{Action: "login.fail", Result: "refused", Remote: ip})
		writeError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}
	s.d.Limiter.Refund(ip, at)
	id, err := s.d.Sessions.Create()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: id, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.secureCookies(),
		MaxAge: int(auth.SessionTTL.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// loginLogPerBlock: at most this many login.fail lines a minute per address
// block, an IPv4 address or an IPv6 /64 (v0.1.2, audit: 1 040 lines in 2
// minutes; counter-audit: a budget for every IP together let one attacker
// hide another). The next one is a single "suppressed" line for the block;
// the first line of a later minute is preceded by one line that counts what
// was not written.
var loginLogPerBlock = 10

// loginBudget counts the login.fail lines of the current minute.
type loginBudget struct {
	mu      sync.Mutex
	now     func() time.Time
	start   time.Time
	n       map[string]int // lines of the minute, per block
	dropped int            // lines of the minute not written
}

// next: how many lines the previous minute did not write (to report
// first), then write the line, or write the "suppressed" line instead, or
// neither.
func (b *loginBudget) next(block string) (dropped int, write, suppressed bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if now := b.now(); now.Sub(b.start) >= time.Minute || b.n == nil {
		dropped = b.dropped
		b.start, b.n, b.dropped = now, map[string]int{}, 0
	}
	b.n[block]++
	k := b.n[block]
	if k > loginLogPerBlock {
		b.dropped++
	}
	return dropped, k <= loginLogPerBlock, k == loginLogPerBlock+1
}

// addressBlock: the IPv4 address, or the /64 of an IPv6 address (one host
// gets a whole /64).
func addressBlock(ip string) string {
	a, err := netip.ParseAddr(ip)
	if err != nil || a.Unmap().Is4() {
		return ip
	}
	p, _ := a.Prefix(64)
	return p.String()
}

func (s *server) auditLogin(e audit.Event) {
	block := addressBlock(e.Remote)
	dropped, write, suppressed := s.loginLog.next(block)
	if dropped > 0 {
		s.writeAudit(audit.Event{Action: "login.fail", Result: "suppressed",
			Error: fmt.Sprintf("%d failed logins not logged in the previous minute (over %d per address block)", dropped, loginLogPerBlock)})
	}
	switch {
	case suppressed:
		s.writeAudit(audit.Event{Action: "login.fail", Result: "suppressed", Remote: block,
			Error: fmt.Sprintf("over %d failed logins this minute from %s: the next ones are counted, not logged", loginLogPerBlock, block)})
	case write:
		s.writeAudit(e)
	}
}

func (s *server) writeAudit(e audit.Event) {
	if err := s.d.Audit.Write(e); err != nil {
		slog.Error("audit write", "err", err)
	}
}

func (s *server) secureCookies() bool {
	return s.d.TLS || strings.HasPrefix(s.d.PublicOrigin, "https://")
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.d.Sessions.Delete(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.secureCookies(), MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) meta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"version": s.d.Version, "readOnly": s.d.ReadOnly})
}

func (s *server) containers(w http.ResponseWriter, r *http.Request) {
	list, err := s.d.Docker.ListContainers(r.Context())
	if err != nil {
		slog.Warn("list containers", "err", secrets.Line(err.Error()))
		writeError(w, http.StatusServiceUnavailable, "docker_unavailable", dockerDetail(dockerx.ErrUnavailable))
		return
	}
	if list == nil {
		list = []dockerx.Container{}
	}
	writeJSON(w, http.StatusOK, list)
}
