//go:build integration && linux

package fsview

// procfs on real containers (spec §10, privileged job). Needs root, the host
// PID namespace and the Docker socket: scripts/test-linux.sh or the CI
// integration job.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

func needRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("root requis pour /proc/<pid>/root des conteneurs : scripts/test-linux.sh ou job CI privilégié")
	}
}

// docker runs the CLI and returns its stdout (a pull writes its progress on
// stderr, which must not end up in the returned ID).
func docker(t *testing.T, args ...string) string {
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

// runTarget starts a container and returns its full ID; removed at the end.
func runTarget(t *testing.T, name string, args ...string) string {
	t.Helper()
	exec.Command("docker", "rm", "-f", name).Run()
	// Registered before the run: a run that creates the container then fails must not leak it.
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })
	id := docker(t, append([]string{"run", "-d", "--name", name}, args...)...)
	return id
}

func openFor(t *testing.T, id string) FS {
	t.Helper()
	dc, err := dockerx.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fsys, err := NewViews(dc, Options{}).For(ctx, id)
	if err != nil {
		t.Fatalf("For(%s) : %v", id, err)
	}
	t.Cleanup(func() { fsys.Close() })
	return fsys
}

func hasEntry(entries []Entry, name string, typ EntryType) bool {
	for _, e := range entries {
		if e.Name == name && e.Type == typ {
			return true
		}
	}
	return false
}

func TestIntegrationProcFSAlpine(t *testing.T) {
	needRoot(t)
	id := runTarget(t, "hublot-it-fs-alpine", "alpine:3", "sleep", "300")
	fsys := openFor(t, id)
	ctx := context.Background()

	entries, _, err := fsys.List(ctx, "/")
	if err != nil || !hasEntry(entries, "etc", TypeDir) || !hasEntry(entries, "proc", TypeDir) {
		t.Fatalf("List / : %v %v", entries, err)
	}
	rc, err := fsys.Open(ctx, "/etc/os-release") // a link to ../usr/lib/os-release, followed in the root
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if !strings.Contains(string(b), "Alpine") {
		t.Fatalf("os-release : %q", b)
	}
	if _, err := fsys.StatNoLinks(ctx, "/etc/os-release"); !errors.Is(err, ErrLinkInPath) {
		t.Fatalf("StatNoLinks d'un lien : %v", err)
	}
	if _, err := fsys.Open(ctx, "/dev/null"); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("/dev/null : %v", err)
	}
	if _, _, err := fsys.List(ctx, "/proc"); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("/proc : %v", err)
	}
	if _, err := fsys.Open(ctx, "/proc/kallsyms"); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("/proc/kallsyms : %v", err)
	}
	if _, err := fsys.Stat(ctx, "/sys"); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("/sys : %v", err)
	}
}

func TestIntegrationProcFSDistrolessNonRoot(t *testing.T) {
	needRoot(t)
	id := runTarget(t, "hublot-it-fs-distroless", "--user", "65532", "--read-only",
		"--entrypoint", "/busybox/sleep", "gcr.io/distroless/static-debian12:debug", "300")
	fsys := openFor(t, id)
	rc, err := fsys.Open(context.Background(), "/etc/passwd")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if !strings.Contains(string(b), "nonroot") {
		t.Fatalf("/etc/passwd : %q", b)
	}
}

func TestIntegrationProcFSPrivilegedTarget(t *testing.T) {
	needRoot(t)
	// Unconfined target: needs an agent with apparmor=unconfined on an AppArmor
	// host (spec §8, spike 3b). scripts/test-linux.sh gives it.
	id := runTarget(t, "hublot-it-fs-priv", "--privileged", "alpine:3", "sleep", "300")
	if _, _, err := openFor(t, id).List(context.Background(), "/etc"); err != nil {
		t.Fatal(err)
	}
}

func TestIntegrationProcFSAfterRestart(t *testing.T) {
	needRoot(t)
	id := runTarget(t, "hublot-it-fs-restart", "alpine:3", "sleep", "300")
	before := docker(t, "inspect", "-f", "{{.State.Pid}}", id)
	docker(t, "restart", "-t", "0", id)
	if after := docker(t, "inspect", "-f", "{{.State.Pid}}", id); after == before {
		t.Fatalf("le pid n'a pas changé (%s)", after)
	}
	if _, _, err := openFor(t, id).List(context.Background(), "/etc"); err != nil {
		t.Fatal(err)
	}
}

func TestIntegrationForAbsentContainer(t *testing.T) {
	dc, err := dockerx.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := NewViews(dc, Options{}).For(ctx, "hublot-it-absent"); !errors.Is(err, dockerx.ErrNotFound) {
		t.Fatalf("conteneur absent : %v", err)
	}
}

// A target sharing the host's PID namespace sees the host's processes:
// /proc/1/root is the host's root. Never followed, directly or by a link
// (spec §5.1 steps 6 and 9; pathrs refuses magic links).
func TestIntegrationPidHostTarget(t *testing.T) {
	needRoot(t)
	id := runTarget(t, "hublot-it-fs-pidhost", "--pid=host", "alpine:3", "sh", "-c", "ln -s /proc/1/root /m && sleep 300")
	fsys := openFor(t, id)
	ctx := context.Background()
	// Wait for the shell to create /m (seen in the listing of /, never followed).
	for deadline := time.Now().Add(10 * time.Second); ; {
		entries, _, err := fsys.List(ctx, "/")
		if err == nil && hasEntry(entries, "m", TypeSymlink) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("/m n'apparaît pas : %v %v", entries, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	refused := func(err error) bool {
		return errors.Is(err, ErrLinkInPath) || errors.Is(err, ErrForbiddenPath) ||
			errors.Is(err, fs.ErrNotExist) || errors.Is(err, unix.ELOOP)
	}
	for _, p := range []string{"/proc/1/root/etc/hostname", "/m/etc/hostname", "/m/etc/shadow"} {
		rc, err := fsys.Open(ctx, p)
		if rc != nil {
			b, _ := io.ReadAll(rc)
			rc.Close()
			t.Errorf("ÉVASION : %s lu (%q), err=%v", p, b, err)
		}
		if err == nil {
			t.Errorf("ÉVASION : Open %s sans erreur", p)
		} else if !refused(err) {
			t.Errorf("Open %s : refus d'un genre inattendu : %v", p, err)
		}
		t.Logf("Open %s : %v", p, err)
	}
	for _, p := range []string{"/proc/1/root", "/m", "/m/etc"} {
		entries, _, err := fsys.List(ctx, p)
		if len(entries) > 0 || err == nil {
			t.Errorf("ÉVASION : List %s a rendu %d entrées, err=%v", p, len(entries), err)
		} else if !refused(err) {
			t.Errorf("List %s : refus d'un genre inattendu : %v", p, err)
		}
		t.Logf("List %s : %v", p, err)
	}
	if _, err := fsys.StatNoLinks(ctx, "/m/etc"); !errors.Is(err, ErrLinkInPath) {
		t.Errorf("StatNoLinks /m/etc : %v", err)
	}
}

// The mount check on a real container without binds: its own /proc and /sys
// are under "/", /etc has none.
func TestIntegrationCheckArchive(t *testing.T) {
	needRoot(t)
	id := runTarget(t, "hublot-it-fs-archive", "alpine:3", "sleep", "300")
	fsys := openFor(t, id)
	ctx := context.Background()
	for _, dir := range []string{"/", "/proc", "/sys"} {
		if err := fsys.CheckArchive(ctx, dir); !errors.Is(err, ErrForbiddenPath) {
			t.Errorf("%s : %v, attendu ErrForbiddenPath", dir, err)
		}
	}
	if err := fsys.CheckArchive(ctx, "/etc"); err != nil {
		t.Errorf("/etc : %v", err)
	}
}

// Measurement, not an assertion (review ruling C1): does GET /archive of a
// running container see the container's own procfs? One small pseudo-file,
// a short delay: never a directory of /proc (a tar of /proc can lock dockerd).
func TestIntegrationMeasureOwnProcInArchive(t *testing.T) {
	needRoot(t)
	id := runTarget(t, "hublot-it-fs-ownproc", "alpine:3", "sleep", "300")
	dc, err := dockerx.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rc, err := dc.GetArchive(ctx, id, "/proc/sys/kernel/hostname")
	switch {
	case err == nil:
		rc.Close()
		t.Logf("MESURE : GET /archive VOIT le /proc propre du conteneur")
	case errors.Is(err, dockerx.ErrNotFound):
		t.Logf("MESURE : GET /archive ne voit pas le /proc propre du conteneur (%v)", err)
	default:
		t.Logf("MESURE : non concluante (%v)", err)
	}
}
