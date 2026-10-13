// Package files is the agent's only writer (spec §4, §5.3, §7): read-only
// mode, link checks, If-Match, audit, then PutArchive, in one function per
// write. It never opens a container path itself: every check goes through
// the fsview view, every write through Docker.
package files

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"path"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/levy-cmd/hublot-kit/internal/audit"
	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

// Size caps (plan Explorateur 3, Q14). An edit never exceeds the preview
// (2 MiB, spec §6): the editor only gets what the preview served.
const (
	MaxEditBytes   = 2 << 20
	MaxUploadBytes = 32 << 20
)

// PutTimeout bounds a PutArchive once its intent is in the audit log. The
// write no longer depends on the request: Docker removes the existing file
// before creating it again (measured), a cut write would lose it.
const PutTimeout = time.Minute

var (
	ErrReadOnly    = errors.New("agent is read-only")
	ErrConflict    = errors.New("the file changed or already exists")
	ErrInvalidName = errors.New("invalid file name")
)

// Auditor is the part of *audit.Log the writer uses.
type Auditor interface {
	Write(audit.Event) error
}

type Writer struct {
	ReadOnly bool
	// Docker: PutArchive, and VolumeOptions for the host-bind rule (Q22).
	Docker dockerx.Client
	Audit  Auditor
	// Open opens the view of a container by full ID: fsview.Views.For.
	Open func(ctx context.Context, id string) (fsview.FS, error)
	// Forget drops the index of a stopped container: fsview.Views.Forget.
	// Called after every PutArchive, failed ones included (Docker may have
	// removed the file already). nil: nothing to drop.
	Forget func(id string)

	mu    sync.Mutex
	locks map[string]chan struct{} // one per container: steps 2 to 6 never interleave (Q23)
}

// ETag is the quoted SHA-256 of content: the ETag of the preview (spec §6)
// and the If-Match of an edit.
func ETag(content []byte) string {
	sum := sha256.Sum256(content)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

// Allowed is step 1 alone (spec §5.3): the API calls it before reading a
// request body, then Edit and Upload check it again.
func (w *Writer) Allowed() error {
	if w.ReadOnly {
		return ErrReadOnly
	}
	return nil
}

// Edit replaces the regular file p of container c (its Inspect) by content,
// if the file's current ETag is ifMatch. It returns the new ETag.
func (w *Writer) Edit(ctx context.Context, c dockerx.ContainerInfo, p string, content []byte, ifMatch string) (string, error) {
	// 1. Read-only mode.
	if err := w.Allowed(); err != nil {
		return "", err
	}
	if len(content) > MaxEditBytes {
		return "", fmt.Errorf("%w: edits limited to %d bytes", fsview.ErrTooLarge, MaxEditBytes)
	}
	// 2. The write path is the cleaned request path, nothing else; a place
	// Docker really writes to (Inspect); no link anywhere on it; fstatfs
	// (procfs) or the host-bind rule (tarindex) inside the view; a regular file.
	target := fsview.CleanPath(p)
	if err := w.writable(ctx, c, target); err != nil {
		return "", err
	}
	unlock, err := w.lock(ctx, c.ID)
	if err != nil {
		return "", err
	}
	defer unlock()
	fsys, err := w.Open(ctx, c.ID)
	if err != nil {
		return "", err
	}
	defer fsys.Close()
	if err := checkRoot(ctx, fsys, target); err != nil {
		return "", err
	}
	e, err := fsys.StatNoLinks(ctx, target)
	if err != nil {
		return "", err
	}
	if e.Type != fsview.TypeFile {
		return "", fmt.Errorf("%w: %s", fsview.ErrNotRegular, target)
	}
	// A mount point of the running container that Inspect may not show (a
	// file bind, a Swarm config, docker-init, a CDI file): Docker cannot
	// replace it, or writes in its own view (Q21). Fail closed.
	if mp, ok := fsys.(fsview.MountPointChecker); ok {
		point, err := mp.IsMountPoint(ctx, target)
		if err != nil {
			return "", err
		}
		if point {
			return "", fmt.Errorf("%w: %s is a mount point of the container: edit its source", fsview.ErrForbiddenPath, target)
		}
	}
	// 3. Read the file again, compare its SHA-256 to If-Match.
	current, err := readCapped(ctx, fsys, target)
	if err != nil {
		return "", err
	}
	if ifMatch == "" || ETag(current) != ifMatch {
		return "", fmt.Errorf("%w: %s (If-Match)", ErrConflict, target)
	}
	tarball, err := oneFile(path.Base(target), content, e.Mode, e.UID, e.GID)
	if err != nil {
		return "", err
	}
	// 4 to 6.
	if err := w.put(ctx, "file.write", c.ID, target, tarball); err != nil {
		return "", err
	}
	return ETag(content), nil
}

// Upload creates the file name in directory dir of container c (its
// Inspect); it never replaces anything (If-None-Match: *). It returns the
// path written.
func (w *Writer) Upload(ctx context.Context, c dockerx.ContainerInfo, dir, name string, content []byte, ifNoneMatch string) (string, error) {
	// 1. Read-only mode.
	if err := w.Allowed(); err != nil {
		return "", err
	}
	if len(content) > MaxUploadBytes {
		return "", fmt.Errorf("%w: uploads limited to %d bytes", fsview.ErrTooLarge, MaxUploadBytes)
	}
	// 2. A single component; a place Docker really writes to (Inspect); then
	// the parent, without any link, not on a pseudo filesystem (inside
	// StatNoLinks), a directory.
	if !validName(name) {
		return "", fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	parent := fsview.CleanPath(dir)
	target := fsview.CleanPath(parent + "/" + name)
	if err := w.writable(ctx, c, target); err != nil {
		return "", err
	}
	unlock, err := w.lock(ctx, c.ID)
	if err != nil {
		return "", err
	}
	defer unlock()
	fsys, err := w.Open(ctx, c.ID)
	if err != nil {
		return "", err
	}
	defer fsys.Close()
	if err := checkRoot(ctx, fsys, target); err != nil {
		return "", err
	}
	d, err := fsys.StatNoLinks(ctx, parent)
	if err != nil {
		return "", err
	}
	if d.Type != fsview.TypeDir {
		return "", fmt.Errorf("%w: %s", fsview.ErrNotDir, parent)
	}
	// 3. If-None-Match: *, then the last component, without following a
	// link: absent → go on; a link, even dangling → link_in_path (from
	// StatNoLinks); anything else → conflict.
	if ifNoneMatch != "*" {
		return "", fmt.Errorf("%w: an upload requires If-None-Match: *", ErrConflict)
	}
	switch _, err := fsys.StatNoLinks(ctx, target); {
	case err == nil:
		return "", fmt.Errorf("%w: %s exists", ErrConflict, target)
	case !errors.Is(err, fs.ErrNotExist):
		return "", err
	}
	tarball, err := oneFile(name, content, 0o644, d.UID, d.GID)
	if err != nil {
		return "", err
	}
	// 4 to 6.
	if err := w.put(ctx, "file.upload", c.ID, target, tarball); err != nil {
		return "", err
	}
	return target, nil
}

// put is steps 4 to 6: intent, PutArchive on the parent of the checked path
// (exactly that string), result. A failed intent writes nothing, and so
// does a request that has already ended (the guard answered 504).
func (w *Writer) put(ctx context.Context, action, id, target string, tarball []byte) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("request ended before the write, nothing written: %w", err)
	}
	ev := audit.Event{Action: action, Container: id, Path: target, Result: "intent"}
	if err := w.Audit.Write(ev); err != nil {
		return fmt.Errorf("audit log, nothing written: %w", err)
	}
	pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), PutTimeout)
	defer cancel()
	err := w.Docker.PutArchive(pctx, id, path.Dir(target), bytes.NewReader(tarball))
	if w.Forget != nil {
		w.Forget(id)
	}
	ev.Result = "ok"
	if err != nil {
		ev.Result, ev.Error = "error", err.Error()
	}
	if aerr := w.Audit.Write(ev); aerr != nil {
		// The write is done (or failed) already: report it as it is.
		slog.Error("audit write", "err", aerr, "action", action, "path", target)
	}
	return err
}

// checkRoot: the view must show the container's own root, else it checks a
// tree Docker does not write in (a chrooted PID 1; review I1). target must
// not be under a tmpfs where the view finds it (spec §13: a --tmpfs under a
// link, /var/run -> ../run, is at /run): Docker would write unseen.
func checkRoot(ctx context.Context, fsys fsview.FS, target string) error {
	rc, ok := fsys.(fsview.WriteRootChecker)
	if !ok {
		return nil
	}
	points, err := rc.CheckWriteRoot(ctx)
	if err != nil {
		return err
	}
	return underTmpfs(target, points)
}

// readCapped reads the current file through the view (regular files only),
// at most MaxEditBytes: its hash never needs more.
func readCapped(ctx context.Context, fsys fsview.FS, p string) ([]byte, error) {
	rc, err := fsys.Open(ctx, p)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, MaxEditBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxEditBytes {
		return nil, fmt.Errorf("%w: %s exceeds %d bytes", fsview.ErrTooLarge, p, MaxEditBytes)
	}
	return b, nil
}

// lock takes the lock of container id, or gives up with ctx.
func (w *Writer) lock(ctx context.Context, id string) (func(), error) {
	w.mu.Lock()
	if w.locks == nil {
		w.locks = map[string]chan struct{}{}
	}
	l, ok := w.locks[id]
	if !ok {
		l = make(chan struct{}, 1)
		w.locks[id] = l
	}
	w.mu.Unlock()
	select {
	case l <- struct{}{}:
		return func() { <-l }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// writable refuses, from the Inspect alone and before any intent, a write
// Docker would not make where the container sees it (Q21), and any write to
// a container whose mounts fall under the host-bind rule of spec §5.2 step 7
// (a bind of /, /proc, /sys, /dev…; Q22: a bind of /etc stays writable). Docker
// extracts in its own view: the root filesystem plus the configured mounts.
func (w *Writer) writable(ctx context.Context, c dockerx.ContainerInfo, target string) error {
	if err := fsview.HostBind(ctx, w.Docker, c.Mounts); err != nil {
		return fmt.Errorf("write refused for the whole container: %w", err)
	}
	for _, d := range []string{"/proc", "/sys", "/dev"} {
		if under(target, d) {
			return fmt.Errorf("%w: %s: %s is mounted when the container starts, Docker does not write there", fsview.ErrForbiddenPath, target, d)
		}
	}
	switch target {
	case "/etc/hosts", "/etc/hostname", "/etc/resolv.conf":
		return fmt.Errorf("%w: %s is managed and mounted by Docker", fsview.ErrForbiddenPath, target)
	}
	tmpfs := slices.Clone(c.Tmpfs)
	for _, m := range c.Mounts {
		d := fsview.CleanPath(m.Destination)
		if d == target {
			return fmt.Errorf("%w: %s is mounted (%s %s): edit its source", fsview.ErrForbiddenPath, target, m.Type, m.Source+m.Name)
		}
		if m.Type == "tmpfs" {
			tmpfs = append(tmpfs, d)
		}
	}
	return underTmpfs(target, tmpfs)
}

// underTmpfs refuses target on or under one of the tmpfs mount points.
func underTmpfs(target string, points []string) error {
	for _, d := range points {
		if d := fsview.CleanPath(d); under(target, d) {
			return fmt.Errorf("%w: %s is under the tmpfs %s, Docker does not write there", fsview.ErrForbiddenPath, target, d)
		}
	}
	return nil
}

// under: p is dir or below it (both clean, absolute).
func under(p, dir string) bool {
	return p == dir || dir == "/" || strings.HasPrefix(p, dir+"/")
}

// validName: a single path component Linux accepts (NAME_MAX 255 bytes),
// valid UTF-8 (the UI could not open it otherwise, spec §13), without
// control characters (C0, DEL, C1), format characters (bidi overrides, zero
// width) or line and paragraph separators: a misleading confirmation; nor
// backslash.
func validName(n string) bool {
	if n == "" || n == "." || n == ".." || len(n) > 255 || !utf8.ValidString(n) || strings.ContainsAny(n, `/\`) {
		return false
	}
	for _, r := range n {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			return false
		}
	}
	return true
}

// oneFile is the tar of a single regular file: base name, mode, owner.
func oneFile(name string, content []byte, mode fs.FileMode, uid, gid int) ([]byte, error) {
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	h := &tar.Header{Typeflag: tar.TypeReg, Name: name, Size: int64(len(content)),
		Mode: int64(fsview.UnixPerm(mode)), Uid: uid, Gid: gid, ModTime: time.Now()}
	if err := tw.WriteHeader(h); err != nil {
		return nil, err
	}
	if _, err := tw.Write(content); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
