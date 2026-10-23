package api

import (
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"syscall"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/files"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

// indexingJSON is the 202 of spec §6: the error body plus the progress.
type indexingJSON struct {
	Error    string  `json:"error"`
	Detail   string  `json:"detail,omitempty"`
	Progress float64 `json:"progress"`
}

func writeFSError(w http.ResponseWriter, err error) {
	var ie *fsview.IndexingError
	if errors.As(err, &ie) {
		writeJSON(w, http.StatusAccepted, indexingJSON{Error: "indexing", Detail: err.Error(), Progress: ie.Progress})
		return
	}
	status, code := fsErrorStatus(err)
	if status >= 500 {
		slog.Warn("file route error", "code", code, "status", status, "err", err)
	}
	detail := err.Error()
	if status == http.StatusNotFound {
		detail = notFoundDetail(err)
	}
	writeError(w, status, code, detail)
}

// reservedStatus: codes of spec v0.2 §6 reserved by tranche 1, so that the
// tranches that use them (3: invalid_query, 6: unsupported_image) never
// write this file.
var reservedStatus = map[string]int{
	"invalid_query":     http.StatusBadRequest,
	"unsupported_image": http.StatusUnprocessableEntity,
}

// apiCoder is an error that names its reserved code. A package that cannot
// import api (fsview) gives its error an APICode method; writeFSError maps
// it. Any code outside reservedStatus is ignored (internal).
type apiCoder interface{ APICode() string }

// writeReserved writes a reserved code with its status; any other code is
// a programming error, answered as internal.
func writeReserved(w http.ResponseWriter, code, detail string) {
	status, ok := reservedStatus[code]
	if !ok {
		slog.Error("unreserved error code", "code", code)
		writeError(w, http.StatusInternalServerError, "internal", "")
		return
	}
	writeError(w, status, code, detail)
}

// notFoundDetail is the detail of a 404, for users (v0.1.1): never the
// internal error (securejoin.OpenInRoot, openat2, the daemon's response).
func notFoundDetail(err error) string {
	switch {
	case errors.Is(err, errNoVariable):
		return "unknown variable"
	case errors.Is(err, dockerx.ErrNotFound): // a container, or an archive path
		return "no such container or path"
	}
	return "no such file or directory"
}

// fsErrorStatus maps the errors of file routes to the codes of spec §6. The
// spec fixes 202 indexing, 400 invalid_name and 429; the other statuses are
// the choice of plan Explorateur 1 ("Décisions"). Order matters: Docker
// errors first (a missing socket is ENOENT too, it is not a missing file).
func fsErrorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, errBusy), errors.Is(err, errTooManyStreams):
		return http.StatusServiceUnavailable, "internal"
	case errors.Is(err, errTimeout):
		return http.StatusGatewayTimeout, "internal"
	case errors.Is(err, dockerx.ErrNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, dockerx.ErrNoLogs): // F6: --log-driver none, syslog… (Q34)
		return http.StatusUnprocessableEntity, "logs_unavailable"
	case errors.Is(err, files.ErrReadOnly), errors.Is(err, dockerx.ErrRefused): // Q15: Docker's read-only rootfs or volume
		return http.StatusForbidden, "read_only"
	case errors.Is(err, files.ErrConflict), errors.Is(err, dockerx.ErrPutInvalid): // the path changed after the checks
		return http.StatusConflict, "conflict"
	case errors.Is(err, files.ErrInvalidName):
		return http.StatusBadRequest, "invalid_name"
	case isTooLarge(err):
		return http.StatusRequestEntityTooLarge, "too_large"
	case errors.Is(err, dockerx.ErrUnavailable):
		return http.StatusServiceUnavailable, "docker_unavailable"
	case errors.Is(err, fsview.ErrContainerChanged):
		return http.StatusConflict, "container_changed"
	case errors.Is(err, fsview.ErrIndexing):
		return http.StatusAccepted, "indexing"
	case errors.Is(err, fsview.ErrForbiddenPath):
		return http.StatusForbidden, "forbidden_path"
	case errors.Is(err, fsview.ErrLinkInPath):
		return http.StatusUnprocessableEntity, "link_in_path"
	case errors.Is(err, fsview.ErrNotRegular):
		return http.StatusUnprocessableEntity, "not_regular"
	case errors.Is(err, fsview.ErrTooLarge):
		return http.StatusRequestEntityTooLarge, "too_large"
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, fsview.ErrNotDir), errors.Is(err, syscall.ENOTDIR),
		errors.Is(err, syscall.ELOOP): // a link loop, or a magic link refused by pathrs
		return http.StatusNotFound, "not_found"
	}
	var coder apiCoder
	if errors.As(err, &coder) {
		if status, ok := reservedStatus[coder.APICode()]; ok {
			return status, coder.APICode()
		}
	}
	return http.StatusInternalServerError, "internal"
}
