package api

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"syscall"
	"testing"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

func TestFSErrorStatus(t *testing.T) {
	wrapped := func(e error) error { return fmt.Errorf("%w : détail", e) }
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{wrapped(errBusy), http.StatusServiceUnavailable, "internal"},
		{wrapped(errTimeout), http.StatusGatewayTimeout, "internal"},
		{wrapped(dockerx.ErrNotFound), http.StatusNotFound, "not_found"},
		{wrapped(dockerx.ErrUnavailable), http.StatusServiceUnavailable, "docker_unavailable"},
		// A missing Docker socket is ENOENT: it must not read as a missing file.
		{fmt.Errorf("%w : %w", dockerx.ErrUnavailable, syscall.ENOENT), http.StatusServiceUnavailable, "docker_unavailable"},
		{wrapped(fsview.ErrContainerChanged), http.StatusConflict, "container_changed"},
		{wrapped(fsview.ErrForbiddenPath), http.StatusForbidden, "forbidden_path"},
		{wrapped(fsview.ErrLinkInPath), http.StatusUnprocessableEntity, "link_in_path"},
		{wrapped(fsview.ErrNotRegular), http.StatusUnprocessableEntity, "not_regular"},
		{wrapped(fsview.ErrTooLarge), http.StatusRequestEntityTooLarge, "too_large"},
		{&fs.PathError{Op: "open", Path: "/x", Err: syscall.ENOENT}, http.StatusNotFound, "not_found"},
		{&fs.PathError{Op: "open", Path: "/etc/hosts/x", Err: syscall.ENOTDIR}, http.StatusNotFound, "not_found"},
		{wrapped(fsview.ErrNotDir), http.StatusNotFound, "not_found"},
		{&fs.PathError{Op: "openat2", Path: "/m/etc", Err: syscall.ELOOP}, http.StatusNotFound, "not_found"},
		{fmt.Errorf("%w : /lien", fsview.ErrLinkInPath), http.StatusUnprocessableEntity, "link_in_path"},
		{&fsview.IndexingError{Progress: 0.5}, http.StatusAccepted, "indexing"},
		{errors.New("autre"), http.StatusInternalServerError, "internal"},
	}
	for _, c := range cases {
		if status, code := fsErrorStatus(c.err); status != c.status || code != c.code {
			t.Errorf("%v : %d %s, attendu %d %s", c.err, status, code, c.status, c.code)
		}
	}
}
