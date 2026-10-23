package files

import (
	"os"
	"testing"

	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

// TestMain: without root (CI job "go"), the host's root (/proc/1/root) is out
// of reach and fsview.HostBind would refuse every bind (v0.1.2); host paths
// are then judged as written.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "hublot-host")
	if err != nil {
		panic(err)
	}
	fsview.UseEmptyHostIfUnreachable(dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
