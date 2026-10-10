//go:build integration && linux

package api

// Writes end to end on a running container (spec §5.3, §10): real Docker,
// real procfs (openat2, fstatfs). Needs root: scripts/test-linux.sh or the
// CI privileged job.

import (
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

func TestIntegrationWriteRunning(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root requis pour /proc/<pid>/root des conteneurs : scripts/test-linux.sh ou job CI privilégié")
	}
	name := "hublot-it-write-run"
	exec.Command("docker", "rm", "-f", name).Run()
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })
	// /etc/os-release exists on every daemon host (Docker Desktop, Ubuntu,
	// Fedora); :ro, so a regression could never write into the host.
	dockerCLI(t, "run", "-d", "--name", name, "--tmpfs", "/scratch", "--init",
		"-v", "/etc/os-release:/app/conf.yml:ro", "alpine:3", "sleep", "300")
	dockerCLI(t, "exec", name, "sh", "-c", "echo v1 > /etc/app.conf && chown 1000:1001 /etc/app.conf && chmod 640 /etc/app.conf && ln -s /etc /linkdir && ln -s /nope /tmp/dangling")
	dc, err := dockerx.New()
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{}
	f.init(t, false, dc, nil)
	c := f.session(t)
	base := "/api/containers/" + name
	// State before the refusals, read inside the container.
	snap := "cat /etc/hosts; echo ==; cat /app/conf.yml; echo ==; sha256sum /sbin/docker-init; echo ==; cat /etc/app.conf"
	rec := f.do("GET", base+"/fs/file?path=/etc/app.conf", "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("aperçu : %d %s", rec.Code, rec.Body)
	}
	tag := rec.Header().Get("ETag")
	if rec := f.send("PUT", base+"/fs/file?path=/etc/app.conf", []byte("v2\n"), c, map[string]string{"If-Match": tag}); rec.Code != http.StatusOK {
		t.Fatalf("édition : %d %s", rec.Code, rec.Body)
	}
	if got := dockerCLI(t, "exec", name, "sh", "-c", "cat /etc/app.conf; stat -c '%a %u %g' /etc/app.conf"); got != "v2\n640 1000 1001" {
		t.Fatalf("contenu, mode et propriétaire gardés : %q", got)
	}
	before := dockerCLI(t, "exec", name, "sh", "-c", snap)
	for _, k := range []struct {
		label  string
		rec    func() (int, string)
		status int
		code   string
	}{
		{"lien parent", func() (int, string) {
			r := f.send("PUT", base+"/fs/file?path=/linkdir/app.conf", []byte("x"), c, map[string]string{"If-Match": tag})
			return r.Code, errCode(t, r)
		}, 422, "link_in_path"},
		{"envoi sur un lien pendant", func() (int, string) {
			r := f.upload(t, c, name, "/tmp", "dangling", []byte("x"), "*")
			return r.Code, errCode(t, r)
		}, 422, "link_in_path"},
		{"envoi dans /proc", func() (int, string) {
			r := f.upload(t, c, name, "/proc", "x", []byte("x"), "*")
			return r.Code, errCode(t, r)
		}, 403, "forbidden_path"},
		// Q21: Docker would answer "device or resource busy" (500), or take
		// the write where the container never sees it.
		{"édition de /etc/hosts", func() (int, string) {
			r := f.send("PUT", base+"/fs/file?path=/etc/hosts", []byte("x"), c, map[string]string{"If-Match": tag})
			return r.Code, errCode(t, r)
		}, 403, "forbidden_path"},
		{"envoi dans un --tmpfs", func() (int, string) {
			r := f.upload(t, c, name, "/scratch", "x", []byte("x"), "*")
			return r.Code, errCode(t, r)
		}, 403, "forbidden_path"},
		{"fichier monté (Destination d'Inspect)", func() (int, string) {
			r := f.send("PUT", base+"/fs/file?path=/app/conf.yml", []byte("x"), c, map[string]string{"If-Match": tag})
			return r.Code, errCode(t, r)
		}, 403, "forbidden_path"},
		// --init mounts /sbin/docker-init without showing it in Inspect.Mounts:
		// only the STATX_MNT_ID check sees it (Q21).
		{"montage absent d'Inspect (--init)", func() (int, string) {
			r := f.send("PUT", base+"/fs/file?path=/sbin/docker-init", []byte("x"), c, map[string]string{"If-Match": tag})
			if !strings.Contains(r.Body.String(), "point de montage") {
				t.Errorf("docker-init : refus attendu par le point de montage : %s", r.Body)
			}
			return r.Code, errCode(t, r)
		}, 403, "forbidden_path"},
	} {
		if status, code := k.rec(); status != k.status || code != k.code {
			t.Errorf("%s : %d %s, attendu %d %s", k.label, status, code, k.status, k.code)
		}
	}
	// Every refusal left the container untouched: read from inside, not
	// through the agent.
	if after := dockerCLI(t, "exec", name, "sh", "-c", snap); after != before || !strings.HasSuffix(after, "v2") {
		t.Errorf("un refus a modifié le conteneur :\navant %q\naprès %q", before, after)
	}
	if got := dockerCLI(t, "exec", name, "sh", "-c", "test ! -e /proc/x && test ! -e /scratch/x && echo absent"); got != "absent" {
		t.Errorf("fichier créé par un envoi refusé (/proc/x ou /scratch/x) : %q", got)
	}
	if rec := f.upload(t, c, name, "/tmp", "new.txt", []byte("n\n"), "*"); rec.Code != http.StatusCreated {
		t.Fatalf("envoi : %d %s", rec.Code, rec.Body)
	}
	if got := dockerCLI(t, "exec", name, "sh", "-c", "cat /tmp/new.txt; stat -c '%a %u %g' /tmp/new.txt; readlink /tmp/dangling"); got != "n\n644 0 0\n/nope" {
		t.Fatalf("fichier envoyé, lien pendant intact : %q", got)
	}
}
