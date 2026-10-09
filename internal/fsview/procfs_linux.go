//go:build linux

package fsview

import (
	"context"
	"errors"
	"io"
	"os"
)

// Skeleton: the evasion tests of procfs_test.go are written and reviewed
// before the code they protect (CLAUDE.md). Task 5 replaces this file.
var errNotImplemented = errors.New("procfs : pas encore implémenté (tâche 5)")

type procFS struct{ root *os.File }

// NewProcFS serves the tree under root, a descriptor on the container's root
// directory (O_PATH is enough). It takes ownership of root: Close closes it.
func NewProcFS(root *os.File) FS { return &procFS{root: root} }

func (p *procFS) Close() error { return p.root.Close() }

func (p *procFS) List(context.Context, string) ([]Entry, bool, error) {
	return nil, false, errNotImplemented
}

func (p *procFS) Stat(context.Context, string) (Entry, error) { return Entry{}, errNotImplemented }

func (p *procFS) Open(context.Context, string) (io.ReadCloser, error) { return nil, errNotImplemented }

func (p *procFS) StatNoLinks(context.Context, string) (Entry, error) {
	return Entry{}, errNotImplemented
}

func (p *procFS) CheckArchive(context.Context, string) error { return errNotImplemented }
