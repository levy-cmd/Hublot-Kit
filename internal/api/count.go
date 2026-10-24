package api

import (
	"context"
	"net/http"
	"slices"
	"time"
)

// GET /api/containers/{id}/count (spec v0.2 §5.3, §5.7): the bounded count
// behind the "Files" figure. The walk of a snapshot (or the export of a
// stopped container), without any MAC nor any write; 202 indexing while it
// runs, then the result kept countTTL. It is the agent's one heavy job,
// like a snapshot.

// countTTL: a result is served that long; countFirstWait: the request that
// starts a count waits that long before 202.
var (
	countTTL       = 5 * time.Minute
	countFirstWait = time.Second
)

const maxCounts = 256

type countResult struct {
	n   int
	at  time.Time
	err error
	key string // the countKey of the state counted
}

// countKey: a count describes one state of the container.
func countKey(id, startedAt, finishedAt string) string {
	return id + "\x00" + startedAt + "\x00" + finishedAt
}

// countErrKey: a failed count is also kept under each ref of its job
// (review of tranche 8). After a cap, dockerd may hold the container's lock
// a while after the export was cut: the next poll gets the result without
// an Inspect, which would wait for that lock and answer 503.
func countErrKey(ref string) string { return "\x01" + ref }

// countFor returns the result kept for key; an error is delivered once.
func (sv *snapService) countFor(key string) (countResult, bool) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	return sv.takeCount(key)
}

// countErrFor returns the failed count of ref, once (countErrKey).
func (sv *snapService) countErrFor(ref string) (countResult, bool) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	return sv.takeCount(countErrKey(ref))
}

// takeCount: under sv.mu. An error goes away with every entry of its count.
func (sv *snapService) takeCount(k string) (countResult, bool) {
	r, ok := sv.counts[k]
	if !ok || time.Since(r.at) > countTTL {
		delete(sv.counts, k)
		return countResult{}, false
	}
	if r.err != nil {
		for k2, o := range sv.counts {
			if o.key == r.key {
				delete(sv.counts, k2)
			}
		}
	}
	return r, true
}

// countByID: the latest count of container id still kept, for figures.
func (sv *snapService) countByID(id string) (countResult, bool) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	var best countResult
	found := false
	for k, r := range sv.counts {
		if r.err == nil && len(k) > len(id) && k[:len(id)+1] == id+"\x00" && time.Since(r.at) <= countTTL && r.at.After(best.at) {
			best, found = r, true
		}
	}
	return best, found
}

func (sv *snapService) keepCount(key string, r countResult) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	if len(sv.counts) >= maxCounts {
		for k, old := range sv.counts {
			if time.Since(old.at) > countTTL {
				delete(sv.counts, k)
			}
		}
	}
	if len(sv.counts) < maxCounts {
		sv.counts[key] = r
	}
}

func writeCount(w http.ResponseWriter, r countResult) {
	if r.err != nil {
		writeFSError(w, r.err)
		return
	}
	writeJSON(w, http.StatusOK, filesJSON{Count: r.n, Source: "count", At: r.at.UTC().Format(time.RFC3339Nano)})
}

func (s *server) count(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	if !containerRef.MatchString(ref) {
		writeError(w, http.StatusNotFound, "not_found", "invalid container ID")
		return
	}
	// A poll during the count: 202 at once, without Inspect (dockerd holds
	// the container's lock during an export, v0.1.1).
	if j, mine := s.snap.current(ref); j != nil {
		if mine && j.kind == "count" {
			writeJSON(w, http.StatusAccepted, indexingJSON{Error: "indexing", Detail: "count in progress", Progress: j.progress()})
			return
		}
		writeError(w, http.StatusConflict, "conflict", "a snapshot or a count is in progress")
		return
	}
	if res, ok := s.snap.countErrFor(ref); ok {
		writeCount(w, res)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), fsTimeout)
	defer cancel()
	info, err := s.d.Docker.Inspect(ctx, ref)
	if err != nil {
		writeFSError(w, err)
		return
	}
	key := countKey(info.ID, info.Details.StartedAt, info.FinishedAt)
	if res, ok := s.snap.countFor(key); ok {
		writeCount(w, res)
		return
	}
	j := &snapJob{kind: "count", key: key, refs: []string{ref, info.ID, info.Name}}
	if !s.snap.claim(j) {
		writeError(w, http.StatusConflict, "conflict", "a snapshot or a count is in progress")
		return
	}
	release, ok := s.guard.take(info.ID)
	if !ok {
		s.snap.finish(j, nil)
		writeFSError(w, errBusy)
		return
	}
	c, err := s.prepare(ctx, info, false)
	if err != nil {
		release()
		s.snap.finish(j, nil)
		writeSnapError(w, err)
		return
	}
	refs := slices.Clone(j.refs)
	go func() {
		defer release()
		err := s.run(c, j, nil, nil)
		if err == nil {
			err = s.unchanged(info)
		}
		res := countResult{n: int(j.entries.Load()), at: time.Now(), err: err, key: key}
		s.snap.keepCount(key, res)
		if err != nil {
			for _, ref := range refs {
				s.snap.keepCount(countErrKey(ref), res)
			}
		}
		s.snap.finish(j, nil)
	}()
	timer := time.NewTimer(countFirstWait)
	defer timer.Stop()
	select {
	case <-j.done:
		if res, ok := s.snap.countFor(key); ok {
			writeCount(w, res)
			return
		}
	case <-timer.C:
	case <-r.Context().Done():
	}
	writeJSON(w, http.StatusAccepted, indexingJSON{Error: "indexing", Detail: "count in progress", Progress: j.progress()})
}
