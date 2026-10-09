package fsview

import (
	"context"
	"fmt"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// For returns the view of a container's files (spec §4): procfs while it runs.
// id may be a name or a short ID. Errors of docker.Inspect pass through
// (dockerx.ErrNotFound, dockerx.ErrUnavailable).
func For(ctx context.Context, docker dockerx.Client, id string) (FS, error) {
	info, err := docker.Inspect(ctx, id)
	if err != nil {
		return nil, err
	}
	if !info.Running || info.Pid == 0 {
		return nil, fmt.Errorf("%w : %s", ErrStopped, info.Name)
	}
	return openProcFS(docker, info)
}
