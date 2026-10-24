package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/audit"
	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
	"github.com/levy-cmd/hublot-kit/internal/secrets"
	"github.com/levy-cmd/hublot-kit/internal/snapshot"
)

// Snapshots (spec v0.2 §5.7, §6): POST /api/containers/{id}/snapshots,
// GET /api/snapshots, DELETE /api/snapshots/{sid}. A snapshot writes in the
// agent's data directory, never in Docker: allowed read-only (Q57), after
// a confirmation of its cost in the interface. One heavy job at a time for
// the whole agent, a snapshot or a count (count.go): a second one is 409.
// Neither a mac nor a keyId ever leaves the agent.

// snapLimits: the caps of a capture (a variable: tests lower them).
var snapLimits = snapshot.DefaultLimits

// snapInspectTimeout bounds the final Inspect, after an export that held
// dockerd's lock on the container.
const snapInspectTimeout = 30 * time.Second

// snapService is the state of the snapshot routes of one server.
type snapService struct {
	mu     sync.Mutex
	job    *snapJob     // the capture or count in progress, nil if none
	last   *snapOutcome // the end of the last capture
	counts map[string]countResult
}

func newSnapService() *snapService { return &snapService{counts: map[string]countResult{}} }

// snapJob is the one heavy job of the agent.
type snapJob struct {
	kind     string // "snapshot" or "count"
	snapshot string // ID of the snapshot taken ("" for a count)
	refs     []string
	key      string // a count: the key of its result
	done     chan struct{}
	total    atomic.Int64 // SizeRootFs, 0 while unknown: progress
	read     atomic.Int64 // bytes of the archive read, or hashed
	entries  atomic.Int64
	// set before done is closed (a count)
	count int
	err   error
}

func (j *snapJob) progress() float64 {
	return progress(j.read.Load(), j.total.Load())
}

// progress goes from 0 to 0.99, as the 202 of an index.
func progress(done, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return min(0.99, float64(done)/float64(total))
}

type snapOutcome struct {
	Snapshot    string    `json:"snapshot"`
	ContainerID string    `json:"containerId"`
	Result      string    `json:"result"` // "ok" or the code of the error
	Detail      string    `json:"detail,omitempty"`
	At          time.Time `json:"at"`
}

// claim takes the slot of the agent's heavy job; false if taken.
func (sv *snapService) claim(j *snapJob) bool {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	if sv.job != nil {
		return false
	}
	j.done = make(chan struct{})
	sv.job = j
	return true
}

// named adds the full ID and the name of the container to the refs a poll
// of the job may use.
func (sv *snapService) named(j *snapJob, info dockerx.ContainerInfo) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	j.refs = append(j.refs, info.ID, info.Name)
}

// finish frees the slot; outcome, when not nil, becomes the last result.
func (sv *snapService) finish(j *snapJob, outcome *snapOutcome) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	if sv.job == j {
		sv.job = nil
	}
	if outcome != nil {
		sv.last = outcome
	}
	close(j.done)
}

// current returns the job in progress and whether ref names its container.
func (sv *snapService) current(ref string) (*snapJob, bool) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	if sv.job == nil {
		return nil, false
	}
	return sv.job, slices.Contains(sv.job.refs, ref)
}

// jobSink counts what a capture gives (progress) and passes it on to out
// (nil: the count keeps nothing).
type jobSink struct {
	out snapshot.Sink
	job *snapJob
	src string
}

func (s jobSink) Add(e snapshot.Entry) error {
	s.job.entries.Add(1)
	if s.src == "procfs" && e.MAC != "" {
		s.job.read.Add(e.Size)
	}
	if s.out == nil {
		return nil
	}
	return s.out.Add(e)
}

func (s jobSink) MarkPartial() {
	if m, ok := s.out.(interface{ MarkPartial() }); ok {
		m.MarkPartial()
	}
}

// countingBody counts the bytes of the archive (progress).
type countingBody struct {
	r io.Reader
	n *atomic.Int64
}

func (c countingBody) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	c.n.Add(int64(n))
	return n, err
}

// snapshotMounts: the destinations a capture of a stopped container marks
// as mounts, never noted below them (Q58): the container's mounts and
// --tmpfs, plus what the runtime mounts in every container (/dev, and the
// three files Docker binds), as procfs sees them.
func snapshotMounts(info dockerx.ContainerInfo) []string {
	ds := []string{"/dev", "/etc/hostname", "/etc/hosts", "/etc/resolv.conf"}
	for _, m := range info.Mounts {
		ds = append(ds, m.Destination)
	}
	return append(ds, info.Tmpfs...)
}

func running(info dockerx.ContainerInfo) bool { return info.Running && info.Pid != 0 }

// unchanged is the final Inspect: the container is in the state it was in
// when the capture started, else the capture is thrown away
// (container_changed).
func (s *server) unchanged(info dockerx.ContainerInfo) error {
	ctx, cancel := context.WithTimeout(context.Background(), snapInspectTimeout)
	defer cancel()
	now, err := s.d.Docker.Inspect(ctx, info.ID)
	if err != nil {
		return err
	}
	if running(now) != running(info) || now.Pid != info.Pid || now.FinishedAt != info.FinishedAt {
		return fmt.Errorf("%w: container %.12s started or stopped during the capture", fsview.ErrContainerChanged, info.ID)
	}
	return nil
}

func project(info dockerx.ContainerInfo) string {
	const k = "com.docker.compose.project"
	if v := info.Details.Labels[k]; secrets.SafeLabel(k, v) {
		return v
	}
	return ""
}

// capture is what the background of a snapshot or a count runs.
type capture struct {
	src  string // procfs, archive or index
	info dockerx.ContainerInfo
	fs   fsview.FS // procfs (a Walker) or the index; nil for the archive
}

// prepare runs the checks of a capture of info before any answer: the view
// of a running container, the bind rule of a stopped one (spec §5.2 step 7),
// the index for a photo of the metadata. On error, nothing is open.
func (s *server) prepare(ctx context.Context, info dockerx.ContainerInfo, wantIndex bool) (capture, error) {
	c := capture{info: info}
	switch {
	case running(info) && wantIndex:
		return c, fmt.Errorf("%w: the photo of the metadata is for a stopped container", errSnapConflict)
	case running(info):
		fsys, err := s.d.OpenFS(ctx, info.ID)
		if err != nil {
			return c, err
		}
		if _, ok := fsys.(fsview.Walker); !ok {
			fsys.Close()
			return c, fmt.Errorf("%w: container %.12s stopped meanwhile", fsview.ErrContainerChanged, info.ID)
		}
		c.src, c.fs = "procfs", fsys
	case wantIndex:
		fsys, ok := s.d.Views.Cached(info)
		if !ok {
			return c, fmt.Errorf("%w: the index of this container is not built (open its files first)", errSnapConflict)
		}
		c.src, c.fs = "index", fsys
	default:
		if err := fsview.HostBind(ctx, s.d.Docker, info.Mounts); err != nil {
			return c, err
		}
		c.src = "archive"
	}
	return c, nil
}

// errSnapConflict: 409 conflict, with the reason.
var errSnapConflict = errors.New("conflict")

func writeSnapError(w http.ResponseWriter, err error) {
	if errors.Is(err, errSnapConflict) {
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return
	}
	writeFSError(w, err)
}

// run captures c into out, key nil for a count. It closes c.fs.
func (s *server) run(c capture, j *snapJob, key []byte, out snapshot.Sink) error {
	ctx := context.Background()
	sctx, scancel := context.WithTimeout(ctx, 2*time.Second)
	if size, err := s.d.Docker.SizeRootFs(sctx, c.info.ID); err == nil {
		if c.src == "procfs" {
			size = min(size, snapLimits.MaxHashed)
		}
		j.total.Store(size)
	}
	scancel()
	sink := jobSink{out: out, job: j, src: c.src}
	switch c.src {
	case "procfs":
		defer c.fs.Close()
		return snapshot.CaptureProcfs(ctx, c.fs.(fsview.Walker), key, snapLimits, sink)
	case "index":
		defer c.fs.Close()
		return snapshot.CaptureIndex(snapshot.MaskMounts(c.fs, snapshotMounts(c.info)), sink)
	}
	actx, cancel := context.WithTimeout(ctx, snapLimits.MaxArchiveDuration)
	defer cancel()
	rc, err := s.d.Docker.GetArchive(actx, c.info.ID, "/")
	if err != nil {
		if errors.Is(actx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%w: snapshot beyond %s", fsview.ErrTooLarge, snapLimits.MaxArchiveDuration)
		}
		return err
	}
	defer rc.Close()
	return snapshot.CaptureArchive(actx, countingBody{rc, &j.read}, snapshotMounts(c.info), key, snapLimits, sink)
}

func (s *server) snapshotCreate(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	if !containerRef.MatchString(ref) {
		writeError(w, http.StatusNotFound, "not_found", "invalid container ID")
		return
	}
	var wantIndex bool
	switch r.URL.Query().Get("source") {
	case "":
	case "index":
		wantIndex = true
	default:
		writeReserved(w, "invalid_query", "source: index, or nothing")
		return
	}
	store := s.d.Snapshots
	if store == nil {
		writeError(w, http.StatusInternalServerError, "internal", "snapshots unavailable")
		return
	}
	// Before Inspect: during the export of a stopped container, dockerd holds
	// its lock, and an Inspect would wait (v0.1.1).
	j := &snapJob{kind: "snapshot", snapshot: snapshot.NewID(), refs: []string{ref}}
	if !s.snap.claim(j) {
		writeError(w, http.StatusConflict, "conflict", "a snapshot or a count is in progress")
		return
	}
	started := false
	defer func() {
		if !started {
			s.snap.finish(j, nil)
		}
	}()
	ctx, cancel := context.WithTimeout(r.Context(), fsTimeout)
	defer cancel()
	info, err := s.d.Docker.Inspect(ctx, ref)
	if err != nil {
		writeFSError(w, err)
		return
	}
	s.snap.named(j, info)
	key, keyID, err := store.Key()
	if err != nil {
		slog.Error("snapshot key", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "snapshot key refused: "+err.Error())
		return
	}
	release, ok := s.guard.take(info.ID)
	if !ok {
		writeFSError(w, errBusy)
		return
	}
	defer func() {
		if !started {
			release()
		}
	}()
	c, err := s.prepare(ctx, info, wantIndex)
	if err != nil {
		writeSnapError(w, err)
		return
	}
	defer func() {
		if !started && c.fs != nil {
			c.fs.Close()
		}
	}()
	h := snapshot.Header{ID: j.snapshot, ContainerID: info.ID, ContainerName: info.Name, Project: project(info),
		Image: info.Details.Image, ImageID: info.Details.ImageID, CreatedAt: time.Now().UTC(),
		State: "stopped", Source: c.src, Limits: snapLimits}
	if running(info) {
		h.State = "running"
	}
	if c.src != "index" {
		h.KeyID = keyID
	} else {
		key = nil
	}
	ev := audit.Event{Action: "snapshot.create", Container: info.ID, Path: h.ID, Result: "intent", Remote: clientIP(r)}
	if err := s.d.Audit.Write(ev); err != nil {
		slog.Error("audit write", "err", err, "action", ev.Action)
		writeError(w, http.StatusInternalServerError, "internal", "audit log: nothing done")
		return
	}
	wr, err := store.Create(h)
	if err != nil {
		ev.Result, ev.Error = "error", secrets.Line(err.Error())
		s.writeAudit(ev)
		writeFSError(w, err)
		return
	}
	started = true
	writeJSON(w, http.StatusAccepted, struct {
		indexingJSON
		Snapshot string `json:"snapshot"`
	}{indexingJSON{Error: "indexing", Detail: "snapshot in progress"}, h.ID})
	go func() {
		defer release()
		err := s.run(c, j, key, wr)
		if err == nil {
			err = s.unchanged(info)
		}
		if err == nil {
			h.FinishedAt = time.Now().UTC()
			err = wr.Commit(h)
		} else {
			wr.Abort()
		}
		out := &snapOutcome{Snapshot: h.ID, ContainerID: info.ID, Result: "ok", At: time.Now().UTC()}
		ev.Result, ev.TS = "ok", time.Time{}
		if err != nil {
			status, code := fsErrorStatus(err)
			out.Result, out.Detail = code, errorDetail(status, err)
			ev.Result, ev.Error = "error", secrets.Line(err.Error())
			slog.Warn("snapshot", "container", info.ID, "err", secrets.Line(err.Error()))
		}
		s.writeAudit(ev)
		s.snap.finish(j, out)
	}()
}

// snapshotJSON is a header as the API shows it: no keyId, no limits.
type snapshotJSON struct {
	ID            string    `json:"id"`
	ContainerID   string    `json:"containerId"`
	ContainerName string    `json:"containerName"`
	Project       string    `json:"project,omitempty"`
	Image         string    `json:"image,omitempty"`
	ImageID       string    `json:"imageId,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	FinishedAt    time.Time `json:"finishedAt"`
	State         string    `json:"state"`
	Source        string    `json:"source"`
	Entries       int       `json:"entries"`
	HashedBytes   int64     `json:"hashedBytes"`
	Partial       bool      `json:"partial"`
	// ContainerGone: the container no longer exists (Q66: kept, marked);
	// absent when Docker did not answer.
	ContainerGone *bool `json:"containerGone,omitempty"`
}

type currentJSON struct {
	Kind          string  `json:"kind"`
	Snapshot      string  `json:"snapshot,omitempty"`
	ContainerID   string  `json:"containerId,omitempty"`
	ContainerName string  `json:"containerName,omitempty"`
	Progress      float64 `json:"progress"`
	Entries       int64   `json:"entries"`
}

func (s *server) snapshotList(w http.ResponseWriter, r *http.Request) {
	store := s.d.Snapshots
	if store == nil {
		writeError(w, http.StatusInternalServerError, "internal", "snapshots unavailable")
		return
	}
	list, err := store.List()
	if err != nil {
		slog.Error("snapshot list", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "snapshots: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var alive map[string]bool
	if cs, err := s.d.Docker.ListContainers(ctx); err == nil {
		alive = map[string]bool{}
		for _, c := range cs {
			alive[c.ID] = true
		}
	}
	out := struct {
		Snapshots []snapshotJSON `json:"snapshots"`
		Current   *currentJSON   `json:"current"`
		Last      *snapOutcome   `json:"last"`
	}{Snapshots: make([]snapshotJSON, 0, len(list))}
	for _, h := range list {
		sj := snapshotJSON{ID: h.ID, ContainerID: h.ContainerID, ContainerName: h.ContainerName, Project: h.Project,
			Image: h.Image, ImageID: h.ImageID, CreatedAt: h.CreatedAt, FinishedAt: h.FinishedAt, State: h.State,
			Source: h.Source, Entries: h.Entries, HashedBytes: h.HashedBytes, Partial: h.Partial}
		if alive != nil {
			gone := !alive[h.ContainerID]
			sj.ContainerGone = &gone
		}
		out.Snapshots = append(out.Snapshots, sj)
	}
	s.snap.mu.Lock()
	if j := s.snap.job; j != nil {
		cj := &currentJSON{Kind: j.kind, Snapshot: j.snapshot, Progress: j.progress(), Entries: j.entries.Load()}
		if len(j.refs) >= 3 {
			cj.ContainerID, cj.ContainerName = j.refs[1], j.refs[2]
		}
		out.Current = cj
	}
	out.Last = s.snap.last
	s.snap.mu.Unlock()
	writeJSON(w, http.StatusOK, out)
}

func (s *server) snapshotDelete(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	store := s.d.Snapshots
	if !snapshot.ValidID(sid) {
		writeError(w, http.StatusNotFound, "not_found", "no such snapshot")
		return
	}
	if store == nil {
		writeError(w, http.StatusInternalServerError, "internal", "snapshots unavailable")
		return
	}
	h, _, err := store.Reader(sid)
	if errors.Is(err, snapshot.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such snapshot")
		return
	}
	if err != nil {
		writeFSError(w, err)
		return
	}
	ev := audit.Event{Action: "snapshot.delete", Container: h.ContainerID, Path: sid, Result: "intent", Remote: clientIP(r)}
	if err := s.d.Audit.Write(ev); err != nil {
		slog.Error("audit write", "err", err, "action", ev.Action)
		writeError(w, http.StatusInternalServerError, "internal", "audit log: nothing done")
		return
	}
	err = store.Delete(sid)
	ev.Result, ev.TS = "ok", time.Time{}
	if err != nil {
		ev.Result, ev.Error = "error", secrets.Line(err.Error())
	}
	s.writeAudit(ev)
	if errors.Is(err, snapshot.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such snapshot")
		return
	}
	if err != nil {
		writeFSError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// latestSnapshot: the most recent snapshot of the container id, for the
// "Files" figure (spec v0.2 §5.3).
func (s *server) latestSnapshot(id string) (snapshot.Header, bool) {
	if s.d.Snapshots == nil {
		return snapshot.Header{}, false
	}
	list, err := s.d.Snapshots.List()
	if err != nil {
		return snapshot.Header{}, false
	}
	for _, h := range list { // most recent first
		if h.ContainerID == id {
			return h, true
		}
	}
	return snapshot.Header{}, false
}
