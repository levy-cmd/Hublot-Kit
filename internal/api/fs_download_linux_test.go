//go:build linux

package api

// The mount check through the real wiring (review R1 iii, R4): fsRequest,
// the default OpenFS (fsview.For), procfs and CheckArchive, with a fake Docker
// whose "container" is one of the test's own processes. No real container,
// no real bind: GetArchive must never be called.

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

func TestDownloadMountCheckThroughTheRealView(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	t.Cleanup(fsview.SetCgroupReaderForTests(func(int) ([]byte, error) {
		return []byte("0::/docker/" + webID + "\n"), nil
	}))
	fake := &dockerx.Fake{Infos: map[string]dockerx.ContainerInfo{}, Archives: map[string][]byte{},
		Volumes: map[string]map[string]string{}}
	setMounts := func(mounts ...dockerx.Mount) {
		info := dockerx.ContainerInfo{ID: webID, Name: "web", Running: true, Pid: cmd.Process.Pid, Mounts: mounts}
		fake.Infos["web"], fake.Infos[webID] = info, info
	}
	f := &fixture{docker: fake}
	f.init(t, false, fake, nil) // nil: the real fsview.For
	c := f.session(t)

	refused := []struct {
		mount dockerx.Mount
		names []string
	}{
		{dockerx.Mount{Type: "bind", Source: "/proc", Destination: "/host/proc"}, []string{"bind /proc -> /host/proc"}},
		{dockerx.Mount{Type: "volume", Name: "gone", Destination: "/data", Driver: "local"}, []string{"gone", "/data"}},
		{dockerx.Mount{Type: "volume", Name: "ceph", Destination: "/c", Driver: "rexray/rbd"}, []string{"ceph", "/c"}},
	}
	for _, r := range refused {
		setMounts(r.mount)
		for _, q := range []string{"", "&check=1"} {
			rec := f.do("GET", "/api/containers/web/fs/download?path=/etc"+q, "", c)
			var body apiError
			json.Unmarshal(rec.Body.Bytes(), &body)
			if rec.Code != http.StatusForbidden || body.Error != "forbidden_path" {
				t.Errorf("%+v %q : %d %s", r.mount, q, rec.Code, rec.Body)
				continue
			}
			for _, n := range r.names {
				if !strings.Contains(body.Detail, n) {
					t.Errorf("%+v : le détail %q ne nomme pas %q", r.mount, body.Detail, n)
				}
			}
		}
	}
	if len(fake.ArchiveCalls) != 0 {
		t.Fatalf("GetArchive ne doit jamais être appelé : %v", fake.ArchiveCalls)
	}
	// Control: the same process without dangerous mounts passes the check. No
	// Inspect.Mounts entry: since the hardening of CheckArchive, a destination
	// must appear verbatim in the process's mount table, and "/data" does not
	// exist as a mount point of the test's own process.
	setMounts()
	if rec := f.do("GET", "/api/containers/web/fs/download?check=1&path=/etc", "", c); rec.Code != http.StatusNoContent {
		t.Fatalf("contrôle : %d %s", rec.Code, rec.Body)
	}
}
