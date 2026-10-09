//go:build integration && linux

package api

// File routes end to end (spec §10): real Docker, real procfs, real HTTP
// handler. Needs root and the host PID namespace: scripts/test-linux.sh or the
// CI integration job.

import (
	"archive/tar"
	"bytes"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

func dockerCLI(t *testing.T, args ...string) string {
	t.Helper()
	var stderr bytes.Buffer
	cmd := exec.Command("docker", args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("docker %s : %v\n%s", strings.Join(args, " "), err, stderr.Bytes())
	}
	return strings.TrimSpace(string(out))
}

func TestIntegrationFilesEndToEnd(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root requis pour /proc/<pid>/root des conteneurs : scripts/test-linux.sh ou job CI privilégié")
	}
	name := "hublot-it-api-fs"
	exec.Command("docker", "rm", "-f", name).Run()
	dockerCLI(t, "run", "-d", "--name", name, "alpine:3", "sleep", "300")
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })
	dockerCLI(t, "exec", name, "ln", "-s", "/etc", "/linkdir")

	dc, err := dockerx.New()
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{}
	f.init(t, false, dc, nil) // nil: the real fsview.For
	c := f.session(t)
	base := "/api/containers/" + name + "/fs"

	l := decodeListing(t, f.do("GET", base+"?path=/etc", "", c))
	found := false
	for _, e := range l.Entries {
		if e.Name == "os-release" && e.Type == "symlink" && e.LinkTarget == "../usr/lib/os-release" {
			found = true
		}
	}
	if !found {
		t.Fatalf("/etc/os-release absent ou mal décrit : %+v", l.Entries)
	}

	rec := f.do("GET", base+"/file?path=/etc/os-release", "", c)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Alpine") || rec.Header().Get("ETag") == "" {
		t.Fatalf("aperçu : %d %s", rec.Code, rec.Body)
	}

	rec = f.do("GET", base+"/download?path=/etc", "", c)
	if rec.Code != http.StatusOK || filename(t, rec) != "etc.tar" {
		t.Fatalf("téléchargement de /etc : %d %s", rec.Code, rec.Body)
	}
	tr := tar.NewReader(rec.Body)
	hasHostname := false
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		hasHostname = hasHostname || h.Name == "etc/hostname"
	}
	if !hasHostname {
		t.Fatal("etc/hostname absent du tar")
	}

	if rec := f.do("GET", base+"/download?check=1&path=/etc", "", c); rec.Code != http.StatusNoContent {
		t.Errorf("contrôle de /etc : %d %s", rec.Code, rec.Body)
	}
	for path, want := range map[string]string{
		"/download?path=/":            "forbidden_path", // its own /proc and /sys are under /
		"/download?path=/sys":         "forbidden_path",
		"/download?check=1&path=/":    "forbidden_path",
		"/download?check=1&path=/sys": "forbidden_path",
		"/download?path=/linkdir":     "link_in_path",
		"/download?path=/linkdir/apk": "link_in_path",
		"/file?path=/dev/null":        "not_regular",
		"?path=/proc":                 "forbidden_path",
		"/file?path=/proc/kallsyms":   "forbidden_path",
		"?path=/absent":               "not_found",
	} {
		if rec := f.do("GET", base+path, "", c); errCode(t, rec) != want {
			t.Errorf("%s : %d %s, attendu %s", path, rec.Code, rec.Body, want)
		}
	}
}
