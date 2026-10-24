package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"
)

// POST /api/containers/{id}/fs/search, body {"q": "…"} (spec v0.2 §5.8,
// Q61): the files
// of the global search, only in the index of a stopped container already
// served to the explorer (Views.SearchCached). It never builds an index,
// never exports, and never answers 202: without such an index, indexed is
// false and the palette offers to open the explorer.
//
// The query is in the body, never in the URL that a reverse proxy logs
// (review of tranche 3); a POST goes through the Origin check of spec §7. It
// is never logged, audited, or repeated in an answer: errors below carry
// fixed details only.
//
// {id} is matched as given: Indexing knows a build by the full ID or the
// name, never a prefix (v0.1.1). A short ID during an export waits for
// dockerd's lock in Inspect, up to fsTimeout; the UI sends the full ID.

const (
	minQuery      = 2   // characters
	maxQuery      = 200 // characters
	maxSearchHits = 200
	maxSearchBody = 4096 // bytes: 200 characters of 4 bytes, escaped, fit
)

type searchHit struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

type searchAnswer struct {
	Indexed   bool        `json:"indexed"`
	Results   []searchHit `json:"results"`
	Truncated bool        `json:"truncated"`
}

var notIndexed = searchAnswer{Results: []searchHit{}}

// errBody: a request body that is not exactly one JSON value of max bytes at
// most.
var errBody = errors.New("invalid body")

// decodeOne decodes ONE JSON value of max bytes at most into v, with nothing
// after it but blanks (review of tranche 3, Codex): a second value, a stray
// "}" or "]", or a body longer than max is refused, wherever it starts.
func decodeOne(body io.Reader, max int64, v any) error {
	b, err := io.ReadAll(io.LimitReader(body, max+1))
	if err != nil || int64(len(b)) > max {
		return errBody
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	if err := dec.Decode(v); err != nil {
		return errBody
	}
	if err := dec.Decode(new(json.RawMessage)); err != io.EOF {
		return errBody
	}
	return nil
}

// validQuery: 2 to 200 characters of valid UTF-8, without NUL.
func validQuery(q string) bool {
	n := utf8.RuneCountInString(q)
	return n >= minQuery && n <= maxQuery && utf8.ValidString(q) && !strings.ContainsRune(q, 0)
}

func (s *server) fsSearch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !containerRef.MatchString(id) {
		writeError(w, http.StatusNotFound, "not_found", "invalid container ID")
		return
	}
	var body struct {
		Q string `json:"q"`
	}
	if err := decodeOne(r.Body, maxSearchBody, &body); err != nil {
		writeReserved(w, "invalid_query", `the body must be {"q": "…"}, of 2 to 200 characters`)
		return
	}
	q := body.Q
	if !validQuery(q) {
		writeReserved(w, "invalid_query", "the query must have 2 to 200 characters")
		return
	}
	// An index being built is not served: answer at once, without the
	// Inspect that would wait for dockerd's lock during the export (v0.1.1).
	// Indexing, not Pending: a search is not a visit (LRU, Grace).
	if s.d.Views.Indexing(id) {
		writeJSON(w, http.StatusOK, notIndexed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), fsTimeout)
	defer cancel()
	info, err := s.d.Docker.Inspect(ctx, id)
	if err != nil {
		writeFSError(w, err)
		return
	}
	if info.Running && info.Pid != 0 {
		writeJSON(w, http.StatusOK, notIndexed) // procfs: no index (spec v0.2 §5.8)
		return
	}
	// Under the guard of the file routes (spec v0.2 §5.8), by full ID; its
	// timeout ends the walk through ctx.
	res, release, err := guarded(s.guard, ctx, info.ID, func() (searchAnswer, error) {
		hits, truncated, indexed := s.d.Views.SearchCached(ctx, info, q, maxSearchHits)
		if !indexed {
			return notIndexed, nil
		}
		out := searchAnswer{Indexed: true, Truncated: truncated, Results: make([]searchHit, 0, len(hits))}
		for _, h := range hits {
			out.Results = append(out.Results, searchHit{Path: h.Path, Type: string(h.Type)})
		}
		return out, nil
	}, nil)
	release()
	if err != nil {
		writeFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
