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
	"path"
	"slices"
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

func TestIntegrationDockerSocketUnderVarRun(t *testing.T) {
	needRoot(t)
	name := "hublot-it-sock"
	id := runTarget(t, name, "-v", "/var/run/docker.sock:/var/run/docker.sock", "alpine:3", "sleep", "300")
	// "|| true": an empty output means "not a link" (the exact precondition
	// we skip on); any other failure (stopped container, daemon error) fails.
	l := docker(t, "exec", name, "sh", "-c", "readlink /var/run || true")
	if l == "" {
		t.Skip("précondition absente : /var/run n'est pas un lien dans alpine:3 (readlink vide)")
	}
	if !path.IsAbs(l) {
		l = path.Join("/var", l)
	}
	if path.Clean(l) != "/run" {
		t.Skipf("précondition absente : /var/run n'est pas un lien vers /run dans alpine:3 (%q)", l)
	}
	// Docker Desktop shows a proxy socket whose mount root (mountinfo field 4)
	// does not end the Source: spec §5.4 (b) refuses it on purpose. Skip on
	// exactly that, naming both values; anywhere else the tar must pass.
	const source = "/var/run/docker.sock"
	root := ""
	for _, line := range strings.Split(docker(t, "exec", name, "cat", "/proc/1/mountinfo"), "\n") {
		if f := strings.Fields(line); len(f) > 4 && f[4] == "/run/docker.sock" {
			root = f[3]
		}
	}
	if root == "" {
		t.Fatal("/run/docker.sock absent de mountinfo dans le conteneur")
	}
	if root == "/" || !strings.HasSuffix(source, root) {
		if !strings.HasPrefix(root, "/host-services/") {
			t.Fatalf("racine du montage %q : ne termine pas la Source %q et n'est pas un socket mandataire de Docker Desktop (/host-services/)", root, source)
		}
		t.Skipf("précondition absente : la racine du montage (%q) ne termine pas la Source (%q), socket mandataire de Docker Desktop (spec §5.4 b : refus voulu)", root, source)
	}
	if err := openFor(t, id).CheckArchive(context.Background(), "/etc"); err != nil {
		t.Fatalf("tar de /etc refusé à cause du socket Docker : %v", err)
	}
}

// Review re-I1: an ordinary running container, with a volume and a tmpfs,
// is not refused for a write; the same container whose PID 1 chroots into its
// own volume or tmpfs (CAP_SYS_CHROOT, default) is.
func TestIntegrationCheckWriteRoot(t *testing.T) {
	needRoot(t)
	ctx := context.Background()
	id := runTarget(t, "hublot-it-fs-writeroot", "-v", "/vol", "--tmpfs", "/scratch", "alpine:3", "sleep", "300")
	w, ok := openFor(t, id).(WriteRootChecker)
	if !ok {
		t.Fatal("la vue procfs doit implémenter WriteRootChecker")
	}
	if points, err := w.CheckWriteRoot(ctx); err != nil || !slices.Equal(points, []string{"/scratch"}) {
		t.Fatalf("conteneur ordinaire avec volume et tmpfs : %v %v", points, err)
	}
	for name, args := range map[string][]string{
		"volume": {"-v", "/jail"},
		"tmpfs":  {"--tmpfs", "/jail:exec"},
	} {
		id := runTarget(t, "hublot-it-fs-jail-"+name, append(args, "alpine:3", "sh", "-c",
			"mkdir /jail/lib && cp /lib/ld-musl-* /jail/lib/ && cp /bin/busybox /jail/ && exec chroot /jail /busybox sleep 300")...)
		var err error
		for i := 0; i < 25; i++ { // PID 1 chroots a moment after the start
			time.Sleep(200 * time.Millisecond)
			_, err = openFor(t, id).(WriteRootChecker).CheckWriteRoot(ctx)
			if err != nil {
				break
			}
		}
		if !errors.Is(err, ErrForbiddenPath) {
			t.Errorf("prison dans un %s : %v, attendu ErrForbiddenPath", name, err)
		}
	}
}

// Spec §13: alpine has /var/run -> ../run; a --tmpfs /var/run (compose
// "tmpfs: [/var/run]") is mounted at /run. CheckWriteRoot finds it by mount
// ID and returns /run, where package files refuses to write. A PID 1
// chrooted into that tmpfs, with a link var/run -> / planted in the jail,
// leads the destination to the jail's root: refused.
func TestIntegrationCheckWriteRootTmpfsUnderLink(t *testing.T) {
	needRoot(t)
	ctx := context.Background()
	id := runTarget(t, "hublot-it-fs-tmpfs-link", "--tmpfs", "/var/run", "alpine:3", "sleep", "300")
	if got := docker(t, "exec", id, "readlink", "/var/run"); got != "../run" {
		t.Skipf("précondition absente : /var/run -> %q dans alpine:3, attendu ../run", got)
	}
	points, err := openFor(t, id).(WriteRootChecker).CheckWriteRoot(ctx)
	if err != nil || !slices.Equal(points, []string{"/run"}) {
		t.Fatalf("tmpfs sous un lien : %v %v, attendu [/run]", points, err)
	}
	jail := runTarget(t, "hublot-it-fs-tmpfs-jail", "--tmpfs", "/var/run:exec", "alpine:3", "sh", "-c",
		"mkdir /run/lib /run/var && ln -s / /run/var/run && cp /lib/ld-musl-* /run/lib/ && cp /bin/busybox /run/ && exec chroot /run /busybox sleep 300")
	for i := 0; i < 25; i++ { // PID 1 chroots a moment after the start
		time.Sleep(200 * time.Millisecond)
		if _, err = openFor(t, jail).(WriteRootChecker).CheckWriteRoot(ctx); err != nil {
			break
		}
	}
	if !errors.Is(err, ErrForbiddenPath) {
		t.Errorf("ÉVASION : prison dans le tmpfs sous un lien acceptée (%v)", err)
	}
}

// Review I5: a --mount type=tmpfs under a link is in Inspect.Mounts; the tar
// of a directory (spec §5.4 b) locates it on /run like a write does, instead
// of refusing every tar of the container.
func TestIntegrationCheckArchiveTmpfsUnderLink(t *testing.T) {
	needRoot(t)
	id := runTarget(t, "hublot-it-fs-mount-tmpfs", "--mount", "type=tmpfs,dst=/var/run", "alpine:3", "sleep", "300")
	if got := docker(t, "exec", id, "readlink", "/var/run"); got != "../run" {
		t.Skipf("précondition absente : /var/run -> %q dans alpine:3, attendu ../run", got)
	}
	if err := openFor(t, id).CheckArchive(context.Background(), "/etc"); err != nil {
		t.Fatalf("tar de /etc refusé à cause du tmpfs /var/run : %v", err)
	}
}
