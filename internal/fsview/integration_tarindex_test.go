//go:build integration

package fsview

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

func dockerCmd(t *testing.T, args ...string) string {
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

// indexed polls For until the index is built (60 s for a cold image pull).
func indexed(t *testing.T, v *Views, name string) FS {
	t.Helper()
	for deadline := time.Now().Add(60 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		fsys, err := v.For(context.Background(), name)
		var ie *IndexingError
		if !errors.As(err, &ie) {
			if err != nil {
				t.Fatalf("For(%s) : %v", name, err)
			}
			return fsys
		}
		if time.Now().After(deadline) {
			t.Fatalf("index jamais prêt : %v", err)
		}
	}
}

func readFS(t *testing.T, fsys FS, p string) string {
	t.Helper()
	rc, err := fsys.Open(context.Background(), p)
	if err != nil {
		t.Fatalf("Open(%s) : %v", p, err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("lecture de %s : %v", p, err)
	}
	return string(b)
}

func TestIntegrationTarIndexStopped(t *testing.T) {
	name, vol := "hublot-it-tar", "hublot-it-tar-vol"
	exec.Command("docker", "rm", "-f", name).Run()
	exec.Command("docker", "volume", "rm", "-f", vol).Run()
	t.Cleanup(func() {
		exec.Command("docker", "rm", "-f", name).Run()
		exec.Command("docker", "volume", "rm", "-f", vol).Run()
	})
	dockerCmd(t, "run", "--name", name, "-v", vol+":/data", "alpine:3", "sh", "-c",
		"echo bonjour > /root/note.txt && chmod 4755 /root/note.txt && ln /root/note.txt /root/hard && "+
			"ln -s ../root/note.txt /etc/rel && ln -s /root/note.txt /abs && mkfifo /root/fifo && echo vol > /data/v.txt")
	dc, err := dockerx.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	fsys := indexed(t, NewViews(dc, Options{}), name)
	for _, p := range []string{"/root/note.txt", "/root/hard"} { // one of them is the tar's hard link
		if e, err := fsys.Stat(ctx, p); err != nil || e.Type != TypeFile || e.Size != 8 || e.Mode&0o7777 == 0 {
			t.Errorf("Stat(%s) : %+v %v", p, e, err)
		}
		if got := readFS(t, fsys, p); got != "bonjour\n" {
			t.Errorf("Open(%s) : %q", p, got)
		}
	}
	for _, p := range []string{"/etc/rel", "/abs"} {
		if got := readFS(t, fsys, p); got != "bonjour\n" {
			t.Errorf("Open(%s) : %q", p, got)
		}
	}
	if _, err := fsys.Open(ctx, "/root/fifo"); !errors.Is(err, ErrNotRegular) {
		t.Errorf("FIFO : %v", err)
	}
	if got := readFS(t, fsys, "/data/v.txt"); got != "vol\n" { // the volume is in the archive (spike)
		t.Errorf("volume : %q", got)
	}
	if entries, _, err := fsys.List(ctx, "/proc"); err != nil || len(entries) != 0 {
		t.Errorf("/proc : %v %v", entries, err)
	}
	// The hard link is the same file: same type, size and mode as the original.
	orig, err1 := fsys.Stat(ctx, "/root/note.txt")
	hard, err2 := fsys.Stat(ctx, "/root/hard")
	if err1 != nil || err2 != nil || orig.Type != TypeFile || hard.Type != TypeFile ||
		orig.Size != hard.Size || orig.Mode != hard.Mode {
		t.Errorf("lien physique : %+v %v / %+v %v", orig, err1, hard, err2)
	}
	if _, err := fsys.StatNoLinks(ctx, "/etc/rel"); !errors.Is(err, ErrLinkInPath) {
		t.Errorf("StatNoLinks : %v", err)
	}
	if err := fsys.CheckArchive(ctx, "/root"); err != nil {
		t.Errorf("CheckArchive : %v", err)
	}
}

func TestIntegrationTarIndexCreated(t *testing.T) {
	name := "hublot-it-created"
	exec.Command("docker", "rm", "-f", name).Run()
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })
	dockerCmd(t, "create", "--name", name, "alpine:3", "true") // never started: FinishedAt is zero
	dc, err := dockerx.New()
	if err != nil {
		t.Fatal(err)
	}
	fsys := indexed(t, NewViews(dc, Options{}), name)
	entries, _, err := fsys.List(context.Background(), "/etc")
	if err != nil {
		t.Fatal(err)
	}
	hasPasswd := false
	for _, e := range entries {
		hasPasswd = hasPasswd || (e.Name == "passwd" && e.Type == TypeFile)
	}
	if !hasPasswd {
		t.Errorf("passwd absent du listing de /etc : %v", entries)
	}
	if got := readFS(t, fsys, "/etc/passwd"); !strings.Contains(got, "root:") {
		t.Errorf("/etc/passwd : %q", got)
	}
}
