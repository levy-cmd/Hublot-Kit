//go:build integration

package api

import (
	"net/http"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// Plan v0.2 tranche 8, step 6: a real alpine:3, running then stopped; the
// walk of procfs and the export give the same paths, out of /proc, /sys,
// /dev and the mounts (noted, never walked, in both).
func TestSnapshotRealDocker(t *testing.T) {
	name := "hublot-it-snapshot"
	exec.Command("docker", "rm", "-f", name).Run()
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })
	if out, err := exec.Command("docker", "run", "-d", "--name", name, "alpine:3", "sh", "-c",
		"echo s3cr3t-it > /root/secret && sleep 300").CombinedOutput(); err != nil {
		t.Fatalf("docker run : %v %s", err, out)
	}
	time.Sleep(500 * time.Millisecond)
	dc, err := dockerx.New()
	if err != nil {
		t.Fatal(err)
	}
	sf := newSnapFixture(t, false, dc, nil)
	c := sf.session(t)
	take := func(state string) []string {
		t.Helper()
		rec := sf.do("POST", "/api/containers/"+name+"/snapshots", "", c)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("%s : prise %d %s", state, rec.Code, rec.Body)
		}
		l, body := sf.idle(t, c)
		if l.Last == nil || l.Last["result"] != "ok" {
			t.Fatalf("%s : %s", state, body)
		}
		id := l.Last["snapshot"].(string)
		h, seq, err := sf.store.Reader(id)
		if err != nil {
			t.Fatal(err)
		}
		if h.State != state || h.Partial {
			t.Fatalf("en-tête : %+v", h)
		}
		var paths []string
		secret := false
		for e, err := range seq {
			if err != nil {
				t.Fatal(err)
			}
			if e.Path == "/root/secret" {
				secret = e.MAC != "" && e.Size == int64(len("s3cr3t-it\n"))
			}
			if e.Type == "mount" || e.Type == "inaccessible" || under(e.Path, "/proc") || under(e.Path, "/sys") || under(e.Path, "/dev") {
				continue
			}
			paths = append(paths, e.Path)
		}
		if !secret {
			t.Errorf("%s : /root/secret sans empreinte", state)
		}
		t.Logf("%s : %d entrées (%d hors montages), source %s, %d octets hachés", state, h.Entries, len(paths), h.Source, h.HashedBytes)
		return paths
	}
	running := take("running")
	if out, err := exec.Command("docker", "stop", "-t", "0", name).CombinedOutput(); err != nil {
		t.Fatalf("docker stop : %v %s", err, out)
	}
	stopped := take("stopped")
	if !slices.Equal(running, stopped) {
		var only []string
		for _, p := range running {
			if !slices.Contains(stopped, p) {
				only = append(only, "actif seul : "+p)
			}
		}
		for _, p := range stopped {
			if !slices.Contains(running, p) {
				only = append(only, "arrêté seul : "+p)
			}
		}
		t.Fatalf("%d entrées actif, %d arrêté :\n%s", len(running), len(stopped), strings.Join(only, "\n"))
	}
}

func under(p, dir string) bool { return p == dir || strings.HasPrefix(p, dir+"/") }
