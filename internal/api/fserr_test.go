package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
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

// v0.1.1: the detail of a 404 is a message for users, never the internal
// error (securejoin, openat2, the Docker response); the code stays not_found.
func TestNotFoundDetailForUsers(t *testing.T) {
	cases := []struct {
		err    error
		detail string
	}{
		{&fs.PathError{Op: "securejoin.OpenInRoot /etc/nope: openat2", Path: "/etc/nope", Err: syscall.ENOENT}, "no such file or directory"},
		{&fs.PathError{Op: "openat2", Path: "/m/etc", Err: syscall.ELOOP}, "no such file or directory"},
		{&fs.PathError{Op: "open", Path: "/etc/hosts/x", Err: syscall.ENOTDIR}, "no such file or directory"},
		{fmt.Errorf("%w : /etc/hostname", fsview.ErrNotDir), "no such file or directory"},
		{fmt.Errorf("%w : Error response from daemon: No such container: web", dockerx.ErrNotFound), "no such container"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		writeFSError(rec, c.err)
		var body struct{ Error, Detail string }
		json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != http.StatusNotFound || body.Error != "not_found" || body.Detail != c.detail {
			t.Errorf("%v : %d %s, attendu 404 not_found %q", c.err, rec.Code, rec.Body, c.detail)
		}
	}
}
