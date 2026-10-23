package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
	"github.com/levy-cmd/hublot-kit/internal/secrets"
)

// GET /api/images/{id}/fs?path=, GET /api/images/{id}/fs/file?path= and
// POST /api/images/{id}/fs/search (spec v0.2 §5.5, §6): the files of an
// image, read from the index of its layers (fsview.ForImage), never by a
// container (no ContainerCreate: a read only, allowed in --read-only).
// Same answers as the routes of a container (listingJSON, the preview of
// fs_preview.go, searchAnswer), and the same 202 indexing while the index is
// built or a file extracted. No write route, no download (Q63).
//
// {id} is the full ID only (imageID): never a name, nor a short ID; anything
// else is 404 without asking Docker. Every route runs under the guard of
// the file routes, with "image:" + ID for key (spec v0.2 §6).

func imageGuardKey(id string) string { return "image:" + id }

// imageFSRequest reads {id} and ?path=; on any failure it answers itself.
func imageFSRequest(w http.ResponseWriter, r *http.Request) (id, p string, ok bool) {
	id, ok = imageID(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "no such image")
		return "", "", false
	}
	raw := r.URL.Query().Get("path")
	if strings.ContainsRune(raw, 0) {
		writeError(w, http.StatusNotFound, "not_found", "invalid path")
		return "", "", false
	}
	return id, fsview.CleanPath(raw), true
}

// writeImageFSError: the codes of the file routes (writeFSError), except
// Docker's own errors, answered with a fixed detail (images.go): never the
// text of the daemon's error.
func writeImageFSError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, dockerx.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "no such image")
	case errors.Is(err, dockerx.ErrUnavailable):
		slog.Warn("image files: Docker", "err", secrets.Line(err.Error()))
		writeError(w, http.StatusServiceUnavailable, "docker_unavailable", "Docker unavailable")
	default:
		writeFSError(w, err)
	}
}

func (s *server) imageFSList(w http.ResponseWriter, r *http.Request) {
	id, p, ok := imageFSRequest(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), fsTimeout)
	defer cancel()
	res, release, err := guarded(s.guard, ctx, imageGuardKey(id), func() (listingJSON, error) {
		fsys, err := s.d.Views.ForImage(ctx, id)
		if err != nil {
			return listingJSON{}, err
		}
		entries, truncated, err := fsys.List(ctx, p)
		if errors.Is(err, fsview.ErrNotDir) {
			e, err := fsys.Stat(ctx, p)
			if err != nil {
				return listingJSON{}, err
			}
			return listingJSON{Path: p, Type: string(e.Type), Entries: []entryJSON{}}, nil
		}
		if err != nil {
			return listingJSON{}, err
		}
		out := listingJSON{Path: p, Type: string(fsview.TypeDir), Truncated: truncated, Entries: make([]entryJSON, 0, len(entries))}
		for _, e := range entries {
			out.Entries = append(out.Entries, toEntryJSON(e))
		}
		sort.Slice(out.Entries, func(i, j int) bool { return out.Entries[i].Name < out.Entries[j].Name })
		return out, nil
	}, nil)
	release()
	if err != nil {
		writeImageFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// imageFSFile: the preview of a regular file of the image (spec §6, §7),
// as fsFile, without ETag (nothing to edit). The first request of a file
// starts its extraction and waits 1 s at most, then 202 indexing.
func (s *server) imageFSFile(w http.ResponseWriter, r *http.Request) {
	id, p, ok := imageFSRequest(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), fsTimeout)
	defer cancel()
	b, release, err := guarded(s.guard, ctx, imageGuardKey(id), func() ([]byte, error) {
		fsys, err := s.d.Views.ForImage(ctx, id)
		if err != nil {
			return nil, err
		}
		rc, err := fsys.Open(ctx, p)
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		b, err := io.ReadAll(io.LimitReader(rc, previewMax+1))
		if err != nil {
			return nil, err
		}
		if len(b) > previewMax {
			return nil, fmt.Errorf("%w: preview limited to %d bytes", fsview.ErrTooLarge, previewMax)
		}
		return b, nil
	}, nil)
	release()
	if err != nil {
		writeImageFSError(w, err)
		return
	}
	ct := previewType(b)
	w.Header().Set("Content-Type", ct)
	if ct == binaryType {
		return // no text preview for a binary file, and no download (Q63)
	}
	w.Write(b)
}

// imageFSSearch: the file search of the global search (spec v0.2 §5.8) in
// the index of an image already served to the explorer (Views.SearchImage):
// never an export, never a request to Docker, never 202. The query is in
// the body, never logged nor repeated (fs_search.go).
func (s *server) imageFSSearch(w http.ResponseWriter, r *http.Request) {
	id, ok := imageID(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "no such image")
		return
	}
	var body struct {
		Q string `json:"q"`
	}
	if err := decodeOne(r.Body, maxSearchBody, &body); err != nil {
		writeReserved(w, "invalid_query", `the body must be {"q": "…"}, of 2 to 200 characters`)
		return
	}
	if !validQuery(body.Q) {
		writeReserved(w, "invalid_query", "the query must have 2 to 200 characters")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), fsTimeout)
	defer cancel()
	res, release, err := guarded(s.guard, ctx, imageGuardKey(id), func() (searchAnswer, error) {
		hits, truncated, indexed := s.d.Views.SearchImage(ctx, id, body.Q, maxSearchHits)
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
