package api

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

// containerRef accepts Docker names and IDs only. The value goes into Docker
// API URLs: it must never carry "/" nor start with ".".
var containerRef = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)

// fsRequest reads {id} and the query parameter param (path, or dir for an
// upload), then resolves the container by Inspect
// (bounded by fsTimeout: a stuck daemon must not hang the request). Every
// later step uses info.ID, the full ID: the guard counts per container, and a
// container recreated under the same name cannot step in mid-request. On any
// failure it answers itself.
func (s *server) fsRequest(w http.ResponseWriter, r *http.Request, param string) (info dockerx.ContainerInfo, p string, ok bool) {
	id := r.PathValue("id")
	if !containerRef.MatchString(id) {
		writeError(w, http.StatusNotFound, "not_found", "identifiant de conteneur invalide")
		return info, "", false
	}
	raw := r.URL.Query().Get(param)
	if strings.ContainsRune(raw, 0) {
		writeError(w, http.StatusNotFound, "not_found", "chemin invalide")
		return info, "", false
	}
	ctx, cancel := context.WithTimeout(r.Context(), fsTimeout)
	defer cancel()
	info, err := s.d.Docker.Inspect(ctx, id)
	if err != nil {
		writeFSError(w, err)
		return info, "", false
	}
	return info, fsview.CleanPath(raw), true
}

type entryJSON struct {
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	Size       int64     `json:"size"`
	Mode       uint32    `json:"mode"` // Unix bits 0o7777: permissions + setuid/setgid/sticky
	UID        int       `json:"uid"`
	GID        int       `json:"gid"`
	ModTime    time.Time `json:"modTime"`
	LinkTarget string    `json:"linkTarget,omitempty"`
}

func toEntryJSON(e fsview.Entry) entryJSON {
	return entryJSON{
		Name: e.Name, Type: string(e.Type), Size: e.Size, Mode: fsview.UnixPerm(e.Mode),
		UID: e.UID, GID: e.GID, ModTime: e.ModTime, LinkTarget: e.LinkTarget,
	}
}

// listingJSON: Type is "dir" for a directory. For any other path, Type gives
// the kind of the resolved path and Entries is empty: the UI then previews it.
type listingJSON struct {
	Path      string      `json:"path"`
	Type      string      `json:"type"`
	Entries   []entryJSON `json:"entries"`
	Truncated bool        `json:"truncated"`
}

func (s *server) fsList(w http.ResponseWriter, r *http.Request) {
	info, p, ok := s.fsRequest(w, r, "path")
	if !ok {
		return
	}
	ctx := r.Context()
	res, release, err := guarded(s.guard, ctx, info.ID, func() (listingJSON, error) {
		fsys, err := s.d.OpenFS(ctx, info.ID)
		if err != nil {
			return listingJSON{}, err
		}
		defer fsys.Close()
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
		writeFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
