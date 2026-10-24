package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"iter"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
	"github.com/levy-cmd/hublot-kit/internal/secrets"
	"github.com/levy-cmd/hublot-kit/internal/snapshot"
)

// GET /api/snapshots/{sid}/compare?with=<sid2>|current and
// GET /api/snapshots/{sid}/image-match?path= (spec v0.2 §5.7, §6, Q59).
//
// Two snapshots: a merge of their manifests read in a stream (constant
// memory). Against the current container: a capture in memory, the same
// walk and caps as a snapshot, fed straight into the merge, nothing
// written; the agent's one heavy job, 202 indexing while it runs, its
// result kept compareTTL. No content is kept anywhere: the only content
// verdict is two MACs of the same key, and the only line diff the page may
// show is when image-match proves that the old content is the image's
// version (HMAC of that version, read by the extraction of v0.2 §5.5,
// which checks the digest of its blob). Neither a mac nor a keyId ever
// leaves the agent; image-match answers a bare boolean.

// maxCompareResults: the results of one comparison, then truncated.
const maxCompareResults = 10_000

// compareTTL: a comparison against the current container is served that
// long; maxKeptBytes caps the answers kept for the whole agent.
var compareTTL = 5 * time.Minute

const maxKeptBytes = 64 << 20

// comparison is the end of a comparison against the current container.
type comparison struct {
	state string // the container's state it describes (countKey)
	ans   []byte // the JSON answer, nil on error
	err   error
	at    time.Time
}

// compareStore keeps the comparisons against the current container, by
// snapshot ID (UUID v4: unique to a store, hence to a server).
type compareStore struct {
	mu sync.Mutex
	m  map[string]comparison
}

var compares = &compareStore{m: map[string]comparison{}}

// takeError returns the error kept for sid, once.
func (cs *compareStore) takeError(sid string) (error, bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	c, ok := cs.m[sid]
	if !ok || c.err == nil {
		return nil, false
	}
	delete(cs.m, sid)
	return c.err, true
}

// answer returns the answer kept for sid in state, if still fresh.
func (cs *compareStore) answer(sid, state string) ([]byte, bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	c, ok := cs.m[sid]
	if !ok || c.err != nil || c.state != state || time.Since(c.at) > compareTTL {
		return nil, false
	}
	return c.ans, true
}

func (cs *compareStore) keep(sid string, c comparison) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.m[sid] = c
	total := 0
	for k, old := range cs.m {
		if time.Since(old.at) > compareTTL {
			delete(cs.m, k)
			continue
		}
		total += len(old.ans)
	}
	for total > maxKeptBytes { // the oldest first
		var oldest string
		for k, old := range cs.m {
			if oldest == "" || old.at.Before(cs.m[oldest].at) {
				oldest = k
			}
		}
		total -= len(cs.m[oldest].ans)
		delete(cs.m, oldest)
	}
}

type compareJSON struct {
	A               snapshotJSON      `json:"a"`
	B               snapshotJSON      `json:"b"`
	ContentCompared bool              `json:"contentCompared"`
	Results         []snapshot.Result `json:"results"`
	Truncated       bool              `json:"truncated"`
	// The unknown apart (review of tranche 9): never in the cap of results.
	Unknown      []snapshot.Result `json:"unknown"`
	UnknownCount int               `json:"unknownCount"`
}

func withOutcome(c compareJSON, o snapshot.Outcome) compareJSON {
	c.Results, c.Truncated, c.Unknown, c.UnknownCount = o.Results, o.Truncated, o.Unknown, o.UnknownCount
	return c
}

// headerJSON: a header as the API shows it (no keyId, no limits).
func headerJSON(h snapshot.Header) snapshotJSON {
	return snapshotJSON{ID: h.ID, ContainerID: h.ContainerID, ContainerName: h.ContainerName, Project: h.Project,
		Image: h.Image, ImageID: h.ImageID, CreatedAt: h.CreatedAt, FinishedAt: h.FinishedAt, State: h.State,
		Source: h.Source, Entries: h.Entries, HashedBytes: h.HashedBytes, Partial: h.Partial}
}

func marshalCompare(c compareJSON) ([]byte, error) {
	if c.Results == nil {
		c.Results = []snapshot.Result{}
	}
	if c.Unknown == nil {
		c.Unknown = []snapshot.Result{}
	}
	return json.Marshal(c)
}

// writeAnswer writes a JSON answer by timed chunks (v0.1.2): up to 10 000
// paths, a client that stops reading is cut.
func writeAnswer(w http.ResponseWriter, b []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	writeTimed(w, b)
}

// writeStoreError: no such snapshot is 404; anything else is internal,
// with a fixed detail (the reason goes to the log, masked).
func writeStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, snapshot.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such snapshot")
		return
	}
	slog.Error("snapshot store", "err", secrets.Line(err.Error()))
	writeError(w, http.StatusInternalServerError, "internal", internalDetail)
}

// withCtx ends seq with ctx's error once ctx is done (a client gone).
func withCtx(ctx context.Context, seq iter.Seq2[snapshot.Entry, error]) iter.Seq2[snapshot.Entry, error] {
	return func(yield func(snapshot.Entry, error) bool) {
		n := 0
		for e, err := range seq {
			if n++; n%1024 == 0 && ctx.Err() != nil {
				yield(snapshot.Entry{}, ctx.Err())
				return
			}
			if !yield(e, err) {
				return
			}
		}
	}
}

func (s *server) snapshotCompare(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	if !snapshot.ValidID(sid) {
		writeError(w, http.StatusNotFound, "not_found", "no such snapshot")
		return
	}
	with := r.URL.Query().Get("with")
	if with == "" {
		writeReserved(w, "invalid_query", "with: the ID of a snapshot, or current")
		return
	}
	if with != "current" && !snapshot.ValidID(with) {
		writeError(w, http.StatusNotFound, "not_found", "no such snapshot")
		return
	}
	store := s.d.Snapshots
	if store == nil {
		writeError(w, http.StatusInternalServerError, "internal", "snapshots unavailable")
		return
	}
	if err := canTimeWrites(w); err != nil { // before any work (v0.1.2: fail closed)
		writeFSError(w, err)
		return
	}
	ha, seqA, err := store.Reader(sid)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if with == "current" {
		s.compareCurrent(w, r, store, ha, seqA)
		return
	}
	hb, seqB, err := store.Reader(with)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	release, ok := s.guard.take(ha.ContainerID)
	if !ok {
		writeFSError(w, errBusy)
		return
	}
	defer release()
	ctx := r.Context()
	same := snapshot.ContentComparable(ha, hb)
	o, err := snapshot.Compare(withCtx(ctx, seqA), withCtx(ctx, seqB), same, maxCompareResults)
	if err != nil {
		if ctx.Err() != nil {
			return // the client is gone
		}
		writeStoreError(w, err)
		return
	}
	b, err := marshalCompare(withOutcome(compareJSON{A: headerJSON(ha), B: headerJSON(hb), ContentCompared: same}, o))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	release() // the answer is in memory: a client slow to read holds no slot (as the previews)
	writeAnswer(w, b)
}

// currentSink feeds the merge with the capture of the current container,
// counting what makes its header.
type currentSink struct {
	m      *snapshot.Merger
	n      int
	hashed int64
}

func (c *currentSink) Add(e snapshot.Entry) error {
	c.n++
	if e.MAC != "" {
		c.hashed += e.Size
	}
	return c.m.Add(e)
}

func (c *currentSink) MarkPartial() { c.m.MarkPartial() }

// prepareCurrent: the capture of a comparison against info. Without a key
// (metadata only), a stopped container whose index is built is read from
// that index: no export, no lock, and the same verdicts.
func (s *server) prepareCurrent(ctx context.Context, info dockerx.ContainerInfo, metadataOnly bool) (capture, error) {
	if metadataOnly && !running(info) {
		if fsys, ok := s.d.Views.Cached(info); ok {
			return capture{src: "index", info: info, fs: fsys}, nil
		}
	}
	return s.prepare(ctx, info, false)
}

func (s *server) compareCurrent(w http.ResponseWriter, r *http.Request, store *snapshot.Store, ha snapshot.Header, seqA iter.Seq2[snapshot.Entry, error]) {
	// A poll during the job: 202 at once, without Inspect (dockerd holds
	// the container's lock during an export, v0.1.1).
	if j, _ := s.snap.current(""); j != nil {
		if j.kind == "compare" && j.snapshot == ha.ID {
			writeJSON(w, http.StatusAccepted, indexingJSON{Error: "indexing", Detail: "comparison in progress", Progress: j.progress()})
			return
		}
		writeError(w, http.StatusConflict, "conflict", "a snapshot, a count or a comparison is in progress")
		return
	}
	if err, ok := compares.takeError(ha.ID); ok {
		writeSnapError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), fsTimeout)
	defer cancel()
	info, err := s.d.Docker.Inspect(ctx, ha.ContainerID)
	if err != nil {
		writeFSError(w, err)
		return
	}
	state := countKey(info.ID, info.Details.StartedAt, info.FinishedAt)
	if b, ok := compares.answer(ha.ID, state); ok {
		writeAnswer(w, b)
		return
	}
	j := &snapJob{kind: "compare", snapshot: ha.ID, refs: []string{ha.ID, info.ID, info.Name}}
	if !s.snap.claim(j) {
		writeError(w, http.StatusConflict, "conflict", "a snapshot, a count or a comparison is in progress")
		return
	}
	release, ok := s.guard.take(info.ID)
	if !ok {
		s.snap.finish(j, nil)
		writeFSError(w, errBusy)
		return
	}
	// The key is read, never created (nothing written). Another key, or
	// none: the metadata only, and no content read at all.
	var key []byte
	k, keyID, kerr := store.ExistingKey()
	switch {
	case errors.Is(kerr, snapshot.ErrNoKey):
	case kerr != nil:
		release()
		s.snap.finish(j, nil)
		slog.Error("snapshot key", "err", secrets.Line(kerr.Error()))
		writeError(w, http.StatusInternalServerError, "internal", snapKeyDetail)
		return
	case ha.KeyID != "" && keyID == ha.KeyID:
		key = k
	}
	c, err := s.prepareCurrent(ctx, info, key == nil)
	if err != nil {
		release()
		s.snap.finish(j, nil)
		writeSnapError(w, err)
		return
	}
	go func() {
		defer release()
		started := time.Now().UTC()
		m := snapshot.NewMerger(seqA, key != nil, maxCompareResults)
		sink := &currentSink{m: m}
		err := s.run(c, j, key, sink)
		if errors.Is(err, snapshot.ErrResultsFull) {
			err = nil // truncated: the rest is not needed
		}
		if err == nil {
			err = s.unchanged(info)
		}
		var ans []byte
		if err == nil {
			var o snapshot.Outcome
			if o, err = m.Finish(); err == nil {
				b := snapshotJSON{ID: "current", ContainerID: info.ID, ContainerName: info.Name, Project: project(info),
					Image: info.Details.Image, ImageID: info.Details.ImageID, CreatedAt: started, FinishedAt: time.Now().UTC(),
					State: "stopped", Source: c.src, Entries: sink.n, HashedBytes: sink.hashed, Partial: m.Partial()}
				if running(info) {
					b.State = "running"
				}
				ans, err = marshalCompare(withOutcome(compareJSON{A: headerJSON(ha), B: b, ContentCompared: key != nil}, o))
			}
		} else {
			m.Close()
		}
		if err != nil {
			slog.Warn("snapshot comparison", "snapshot", ha.ID, "container", info.ID, "err", secrets.Line(err.Error()))
		}
		compares.keep(ha.ID, comparison{state: state, ans: ans, err: err, at: time.Now()})
		s.snap.finish(j, nil)
	}()
	timer := time.NewTimer(countFirstWait)
	defer timer.Stop()
	select {
	case <-j.done:
		if err, ok := compares.takeError(ha.ID); ok {
			writeSnapError(w, err)
			return
		}
		if b, ok := compares.answer(ha.ID, state); ok {
			writeAnswer(w, b)
			return
		}
	case <-timer.C:
	case <-r.Context().Done():
	}
	writeJSON(w, http.StatusAccepted, indexingJSON{Error: "indexing", Detail: "comparison in progress", Progress: j.progress()})
}

// findEntry reads seq up to p (the order of the components: past it, p is
// not there).
func findEntry(ctx context.Context, seq iter.Seq2[snapshot.Entry, error], p string) (snapshot.Entry, bool, error) {
	for e, err := range withCtx(ctx, seq) {
		if err != nil {
			return snapshot.Entry{}, false, err
		}
		switch c := snapshot.ComparePaths(e.Path, p); {
		case c == 0:
			return e, true, nil
		case c > 0:
			return snapshot.Entry{}, false, nil
		}
	}
	return snapshot.Entry{}, false, nil
}

// absent: the path is not a file of the image (absent, a dead link…).
func absent(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, fsview.ErrNotDir) || errors.Is(err, syscall.ENOTDIR) ||
		errors.Is(err, syscall.ELOOP) || errors.Is(err, fsview.ErrNotRegular)
}

type matchJSON struct {
	Match bool `json:"match"`
}

// snapshotImageMatch: whether the MAC of path in snapshot sid equals the
// HMAC of the version of path in the snapshot's image. Only a boolean:
// false whenever it cannot be proved (no MAC, another key, no such file in
// the image, another size); errors of the image (202 while extracting,
// unsupported_image for a blob whose digest is wrong) as the image routes.
func (s *server) snapshotImageMatch(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	raw := r.URL.Query().Get("path")
	if !snapshot.ValidID(sid) || strings.ContainsRune(raw, 0) {
		writeError(w, http.StatusNotFound, "not_found", "no such snapshot")
		return
	}
	p := fsview.CleanPath(raw)
	store := s.d.Snapshots
	if store == nil {
		writeError(w, http.StatusInternalServerError, "internal", "snapshots unavailable")
		return
	}
	h, seq, err := store.Reader(sid)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	e, found, err := findEntry(r.Context(), seq, p)
	if err != nil {
		if r.Context().Err() == nil {
			writeStoreError(w, err)
		}
		return
	}
	no := func() { writeJSON(w, http.StatusOK, matchJSON{}) }
	want, herr := hex.DecodeString(e.MAC)
	id, ok := imageID(h.ImageID)
	if !found || e.Type != "file" || e.MAC == "" || herr != nil || e.Size > fsview.MaxExtract || !ok || h.KeyID == "" {
		no()
		return
	}
	key, keyID, err := store.ExistingKey()
	switch {
	case errors.Is(err, snapshot.ErrNoKey):
		no()
		return
	case err != nil:
		slog.Error("snapshot key", "err", secrets.Line(err.Error()))
		writeError(w, http.StatusInternalServerError, "internal", snapKeyDetail)
		return
	case keyID != h.KeyID:
		no()
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), fsTimeout)
	defer cancel()
	match, release, err := guarded(s.guard, ctx, imageGuardKey(id), func() (bool, error) {
		fsys, err := s.d.Views.ForImage(ctx, id)
		if err != nil {
			return false, err
		}
		st, err := fsys.Stat(ctx, p)
		switch {
		case absent(err):
			return false, nil
		case err != nil:
			return false, err
		case st.Type != fsview.TypeFile || st.Size != e.Size:
			return false, nil // nothing to extract
		}
		rc, err := fsys.Open(ctx, p)
		if absent(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		defer rc.Close()
		m := hmac.New(sha256.New, key)
		if _, err := io.Copy(m, rc); err != nil {
			return false, err
		}
		return hmac.Equal(m.Sum(nil), want), nil
	}, nil)
	release()
	if err != nil {
		writeImageFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, matchJSON{Match: match})
}
