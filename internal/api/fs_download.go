package api

import (
	"io"
	"mime"
	"net/http"
	"path"

	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

// fsDownload streams a regular file, or a directory as a tar (spec §5.4).
// With &check=1 it runs every check, stops before reading anything (no
// GetArchive) and answers 204: the UI asks first, then lets the browser
// download.
func (s *server) fsDownload(w http.ResponseWriter, r *http.Request) {
	info, p, ok := s.fsRequest(w, r, "path")
	if !ok {
		return
	}
	// HEAD matches the GET pattern: like check=1, it reads nothing.
	check := r.URL.Query().Get("check") == "1" || r.Method == http.MethodHead
	if !check {
		if err := canTimeWrites(w); err != nil { // before any archive (v0.1.2, Codex)
			writeFSError(w, err)
			return
		}
	}
	ctx := r.Context()
	type opened struct {
		rc  io.ReadCloser // nil in check mode
		dir bool
	}
	o, release, err := guarded(s.guard, ctx, info.ID, func() (opened, error) {
		fsys, err := s.d.OpenFS(ctx, info.ID)
		if err != nil {
			return opened{}, err
		}
		defer fsys.Close() // an opened file stays readable without the view
		e, err := fsys.Stat(ctx, p)
		if err != nil {
			return opened{}, err
		}
		if e.Type != fsview.TypeDir {
			if oc, ok := fsys.(fsview.OpenChecker); check && ok {
				return opened{}, oc.CheckOpen(ctx, p)
			}
			rc, err := fsys.Open(ctx, p)
			if err != nil {
				return opened{}, err
			}
			if check {
				rc.Close()
				return opened{}, nil
			}
			return opened{rc: rc}, nil
		}
		// The cleaned request path, without any link in it, is exactly what
		// goes to GetArchive; and the tar must not cross a host bind, a pseudo
		// filesystem or a FUSE mount (CheckArchive).
		if _, err := fsys.StatNoLinks(ctx, p); err != nil {
			return opened{}, err
		}
		if err := fsys.CheckArchive(ctx, p); err != nil {
			return opened{}, err
		}
		if check {
			return opened{dir: true}, nil
		}
		rc, err := s.d.Docker.GetArchive(ctx, info.ID, p)
		if err != nil {
			return opened{}, err
		}
		return opened{rc: rc, dir: true}, nil
	}, func(o opened) {
		if o.rc != nil {
			o.rc.Close()
		}
	})
	defer release() // after the copy: a reader stuck on FUSE keeps its slot
	if err != nil {
		writeFSError(w, err)
		return
	}
	if check {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	defer o.rc.Close()
	name := path.Base(p)
	if o.dir {
		if p == "/" {
			name = "rootfs"
		}
		name += ".tar"
	}
	setAttachment(w, name)
	// Streamed, never held in memory. A failure midway (dockerd cut, EIO on a
	// FUSE file) aborts the response: a clean final chunk would let the browser
	// save a truncated file as complete. The defers above still run. Each
	// write ends within writeStallTimeout (v0.1.2, counter-audit): a client
	// that stopped reading (download paused) frees the slot and, on a
	// stopped container, dockerd's archive.
	written := 0
	if _, err := io.Copy(timedWriter{w, http.NewResponseController(w), &written}, o.rc); err != nil {
		panic(http.ErrAbortHandler)
	}
}

// setAttachment: always saved, never rendered (spec §5.4, §7).
func setAttachment(w http.ResponseWriter, name string) {
	cd := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	if cd == "" {
		cd = "attachment"
	}
	w.Header().Set("Content-Disposition", cd)
	w.Header().Set("Content-Type", "application/octet-stream")
}
