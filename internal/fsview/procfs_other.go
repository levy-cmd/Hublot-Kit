//go:build !linux

package fsview

import (
	"errors"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// The agent reads /proc/<pid>/root: Linux only. On macOS, run it in a
// container (scripts/dev-agent.sh); the tests that need Linux run through
// scripts/test-linux.sh.
func openProcFS(dockerx.Client, dockerx.ContainerInfo, func(int) ([]byte, error)) (FS, error) {
	return nil, errors.New("explorer: Linux only (on macOS, run the agent in a container)")
}
