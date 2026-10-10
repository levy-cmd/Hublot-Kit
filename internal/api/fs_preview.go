package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/levy-cmd/hublot-kit/internal/files"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

// previewMax caps the preview (spec §6 "2 Mo", taken as 2 MiB).
const previewMax = 2 << 20

const binaryType = "application/octet-stream"

// fsFile serves the preview of a regular file (spec §6, §7): text/plain or
// one of four raster image types, never an active type. The ETag is the
// SHA-256 of the content: the If-Match of a later edit (plan Explorateur 3).
func (s *server) fsFile(w http.ResponseWriter, r *http.Request) {
	info, p, ok := s.fsRequest(w, r, "path")
	if !ok {
		return
	}
	ctx := r.Context()
	b, release, err := guarded(s.guard, ctx, info.ID, func() ([]byte, error) {
		fsys, err := s.d.OpenFS(ctx, info.ID)
		if err != nil {
			return nil, err
		}
		defer fsys.Close()
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
			return nil, fmt.Errorf("%w : aperçu limité à %d octets", fsview.ErrTooLarge, previewMax)
		}
		return b, nil
	}, nil)
	release() // the content is in memory: nothing left to read
	if err != nil {
		writeFSError(w, err)
		return
	}
	ct := previewType(b)
	w.Header().Set("ETag", files.ETag(b)) // the If-Match of an edit
	w.Header().Set("Content-Type", ct)
	if ct == binaryType {
		return // no text preview for a binary file: the UI offers the download
	}
	w.Write(b)
}

// previewType: PNG, JPEG, GIF or WebP as detected by http.DetectContentType;
// otherwise binary when a NUL byte appears in the first 8 KiB; otherwise
// text/plain, SVG and HTML included (spec §6, §7).
func previewType(b []byte) string {
	switch ct := http.DetectContentType(b); ct {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return ct
	}
	if bytes.IndexByte(b[:min(len(b), 8<<10)], 0) >= 0 {
		return binaryType
	}
	return "text/plain; charset=utf-8"
}
