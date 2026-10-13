package api

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"

	"github.com/levy-cmd/hublot-kit/internal/files"
)

// uploadOverhead: room for the multipart boundaries and headers around the
// file of an upload.
const uploadOverhead = 1 << 20

// fsEdit is PUT /fs/file (F4, spec §5.3): the raw body replaces the file if
// If-Match is its current ETag. The body is read before taking a guard slot:
// a slow client must not hold one.
func (s *server) fsEdit(w http.ResponseWriter, r *http.Request) {
	if err := s.files.Allowed(); err != nil {
		writeFSError(w, err)
		return
	}
	info, p, ok := s.fsRequest(w, r, "path")
	if !ok {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, files.MaxEditBytes+1)) // files reports too_large
	if err != nil {
		writeFSError(w, err)
		return
	}
	ctx := r.Context()
	tag, release, err := guarded(s.guard, ctx, info.ID, func() (string, error) {
		return s.files.Edit(ctx, info, p, body, r.Header.Get("If-Match"))
	}, nil)
	release()
	if err != nil {
		writeGuardError(w, err)
		return
	}
	w.Header().Set("ETag", tag)
	writeJSON(w, http.StatusOK, map[string]string{"etag": tag})
}

// fsUpload is POST /fs/upload?dir= (F4): one multipart part "file", whose
// filename is the name created in dir. It never replaces anything.
func (s *server) fsUpload(w http.ResponseWriter, r *http.Request) {
	if err := s.files.Allowed(); err != nil {
		writeFSError(w, err)
		return
	}
	info, dir, ok := s.fsRequest(w, r, "dir")
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, files.MaxUploadBytes+uploadOverhead)
	name, content, err := readUpload(r)
	if err != nil {
		writeFSError(w, err)
		return
	}
	ctx := r.Context()
	written, release, err := guarded(s.guard, ctx, info.ID, func() (string, error) {
		return s.files.Upload(ctx, info, dir, name, content, r.Header.Get("If-None-Match"))
	}, nil)
	release()
	if err != nil {
		writeGuardError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"path": written, "etag": files.ETag(content)})
}

// readUpload returns the filename and the content of the part "file". The
// filename is the value mime.ParseMediaType decodes (RFC 2231 filename*,
// quoted pairs): package files validates that very value, the one written
// in the tar. Not Part.FileName(): it keeps only the base name, and "a/b"
// would become "b" instead of being refused (spec §5.3: invalid_name). No
// part "file": an empty name, refused the same way. The content is read up
// to one byte past the cap: package files reports too_large.
func readUpload(r *http.Request) (string, []byte, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return "", nil, fmt.Errorf("%w: multipart body expected (%v)", files.ErrInvalidName, err)
	}
	var name string
	var content []byte
	found := false
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", nil, err
		}
		if part.FormName() != "file" {
			part.Close()
			continue
		}
		if found { // a second "file": the name would be ambiguous
			part.Close()
			return "", nil, fmt.Errorf("%w: several file fields", files.ErrInvalidName)
		}
		_, params, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		if err != nil {
			part.Close()
			return "", nil, fmt.Errorf("%w: %v", files.ErrInvalidName, err)
		}
		b, err := io.ReadAll(io.LimitReader(part, files.MaxUploadBytes+1))
		part.Close()
		if err != nil {
			return "", nil, err
		}
		name, content, found = params["filename"], b, true
	}
	if !found {
		return "", nil, fmt.Errorf("%w: no file field", files.ErrInvalidName)
	}
	return name, content, nil
}

// writeGuardError answers a failed guarded write. On a timeout or an
// abandoned request the write goes on in the background: say so.
func writeGuardError(w http.ResponseWriter, err error) {
	if errors.Is(err, errTimeout) {
		err = fmt.Errorf("%w; the write may still complete: reload the file to check", err)
	}
	writeFSError(w, err)
}

// isTooLarge: the body went past http.MaxBytesReader.
func isTooLarge(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}
