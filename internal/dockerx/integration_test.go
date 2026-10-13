//go:build integration

package dockerx

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestListContainersRealDocker(t *testing.T) {
	name := "hublot-it-f1"
	exec.Command("docker", "rm", "-f", name).Run()
	if out, err := exec.Command("docker", "run", "-d", "--name", name, "alpine:3", "sleep", "300").CombinedOutput(); err != nil {
		t.Fatalf("docker run: %v %s", err, out)
	}
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })

	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	list, err := c.ListContainers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, ct := range list {
		if ct.Name == name {
			if ct.State != "running" || ct.Image != "alpine:3" {
				t.Fatalf("unexpected %+v", ct)
			}
			return
		}
	}
	t.Fatalf("%s not listed", name)
}

func TestInspectAndArchiveRealDocker(t *testing.T) {
	name := "hublot-it-inspect"
	exec.Command("docker", "rm", "-f", name).Run()
	if out, err := exec.Command("docker", "run", "-d", "--name", name, "alpine:3", "sleep", "300").CombinedOutput(); err != nil {
		t.Fatalf("docker run: %v %s", err, out)
	}
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })

	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	info, err := c.Inspect(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.ID) != 64 || info.Name != name || !info.Running || info.Pid == 0 {
		t.Fatalf("inspect: %+v", info)
	}
	rc, err := c.GetArchive(ctx, info.ID, "/etc/hostname")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	h, err := tar.NewReader(rc).Next()
	if err != nil || h.Name != "hostname" {
		t.Fatalf("archive: %+v %v", h, err)
	}
	if _, err := c.Inspect(ctx, "hublot-it-absent"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("absent container: %v", err)
	}
	if _, err := c.GetArchive(ctx, info.ID, "/absent"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("absent path: %v", err)
	}
}

func TestMountsAndVolumeOptionsRealDocker(t *testing.T) {
	vol, name := "hublot-it-vol", "hublot-it-mounts"
	exec.Command("docker", "rm", "-f", name).Run()
	exec.Command("docker", "volume", "rm", "-f", vol).Run()
	// A local volume that is a bind in disguise (spec §5.2 step 7). Creating it
	// mounts nothing; the container below mounts it on /data.
	if out, err := exec.Command("docker", "volume", "create", "--opt", "type=none", "--opt", "o=bind",
		"--opt", "device=/tmp", vol).CombinedOutput(); err != nil {
		t.Fatalf("docker volume create: %v %s", err, out)
	}
	t.Cleanup(func() { exec.Command("docker", "volume", "rm", "-f", vol).Run() })
	if out, err := exec.Command("docker", "run", "-d", "--name", name, "-v", vol+":/data", "alpine:3", "sleep", "300").CombinedOutput(); err != nil {
		t.Fatalf("docker run: %v %s", err, out)
	}
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })

	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	info, err := c.Inspect(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Mounts) != 1 || info.Mounts[0].Type != "volume" || info.Mounts[0].Name != vol ||
		info.Mounts[0].Driver != "local" || info.Mounts[0].Destination != "/data" {
		t.Fatalf("mounts: %+v", info.Mounts)
	}
	opts, err := c.VolumeOptions(ctx, vol)
	if err != nil || opts["o"] != "bind" || opts["device"] != "/tmp" {
		t.Fatalf("options: %v %v", opts, err)
	}
}

func TestMeasureSizeRootFs(t *testing.T) {
	name := "hublot-it-size"
	exec.Command("docker", "rm", "-f", name).Run()
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })
	if out, err := exec.Command("docker", "run", "--name", name, "alpine:3", "true").CombinedOutput(); err != nil {
		t.Fatalf("docker run : %v %s", err, out)
	}
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	size, err := c.SizeRootFs(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("MESURE : SizeRootFs = %d octets", size) // 0: the progress stays at 0 % (not an error)
}

// oneFileTar is the tar package files sends: one regular entry, base name.
func oneFileTar(t *testing.T, name, body string, mode int64, uid, gid int) *bytes.Buffer {
	t.Helper()
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: mode, Uid: uid, Gid: gid,
		Size: int64(len(body)), ModTime: time.Now()}); err != nil {
		t.Fatal(err)
	}
	io.WriteString(tw, body)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return &b
}

// PutArchive on a running and on a stopped container: content, mode and
// owner come from the tar header; the existing file is replaced by a new
// inode (Docker removes, then creates); Docker's refusals are classified.
func TestPutArchiveRealDocker(t *testing.T) {
	run, stopped, ro, rov := "hublot-it-put", "hublot-it-put-stopped", "hublot-it-put-ro", "hublot-it-put-rov"
	exec.Command("docker", "rm", "-f", run, stopped, ro, rov).Run()
	for _, args := range [][]string{
		{"run", "-d", "--name", run, "--tmpfs", "/scratch", "alpine:3", "sleep", "300"},
		{"create", "--name", stopped, "alpine:3", "true"},
		{"run", "-d", "--name", ro, "--read-only", "alpine:3", "sleep", "300"},
		{"run", "-d", "--name", rov, "-v", "hublot-it-put-rov:/data:ro", "alpine:3", "sleep", "300"},
	} {
		if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
			t.Fatalf("docker %v : %v %s", args, err, out)
		}
	}
	t.Cleanup(func() {
		exec.Command("docker", "rm", "-f", run, stopped, ro, rov).Run()
		exec.Command("docker", "volume", "rm", "hublot-it-put-rov").Run()
	})
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	inode := func() string {
		out, _ := exec.Command("docker", "exec", run, "stat", "-c", "%i", "/etc/motd").Output()
		return strings.TrimSpace(string(out))
	}
	before := inode()
	if err := c.PutArchive(ctx, run, "/etc", oneFileTar(t, "motd", "bonjour\n", 0o640, 1000, 1001)); err != nil {
		t.Fatalf("conteneur actif : %v", err)
	}
	out, err := exec.Command("docker", "exec", run, "stat", "-c", "%a %u %g %s", "/etc/motd").Output()
	if err != nil || strings.TrimSpace(string(out)) != "640 1000 1001 8" {
		t.Fatalf("mode et propriétaire : %q %v", out, err)
	}
	t.Logf("MESURE inode de /etc/motd : %s avant, %s après", before, inode())

	if err := c.PutArchive(ctx, stopped, "/etc", oneFileTar(t, "hublot.txt", "arrêté\n", 0o644, 0, 0)); err != nil {
		t.Fatalf("conteneur arrêté : %v", err)
	}
	rc, err := c.GetArchive(ctx, stopped, "/etc/hublot.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	tr := tar.NewReader(rc)
	if _, err := tr.Next(); err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(tr); string(b) != "arrêté\n" {
		t.Fatalf("relu : %q", b)
	}

	if err := c.PutArchive(ctx, ro, "/etc", oneFileTar(t, "x", "x", 0o644, 0, 0)); !errors.Is(err, ErrRefused) {
		t.Fatalf("rootfs en lecture seule : %v, attendu ErrRefused", err)
	}
	if err := c.PutArchive(ctx, rov, "/data", oneFileTar(t, "x", "x", 0o644, 0, 0)); !errors.Is(err, ErrRefused) {
		t.Fatalf("volume :ro : %v, attendu ErrRefused", err)
	}
	// Q21: the --tmpfs destinations come from Inspect (they are not in Mounts).
	if info, err := c.Inspect(ctx, run); err != nil || !slices.Equal(info.Tmpfs, []string{"/scratch"}) {
		t.Fatalf("Tmpfs : %v %v", info.Tmpfs, err)
	}
	// A file where a directory was checked (the path changed under us).
	if err := c.PutArchive(ctx, run, "/etc/motd", oneFileTar(t, "x", "x", 0o644, 0, 0)); !errors.Is(err, ErrPutInvalid) {
		t.Fatalf("point d'extraction qui n'est pas un dossier : %v, attendu ErrPutInvalid", err)
	}
	if err := c.PutArchive(ctx, run, "/absent", oneFileTar(t, "x", "x", 0o644, 0, 0)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("dossier absent : %v, attendu ErrNotFound", err)
	}
}

// collect reads a logs stream of container id to its end (or 20 s).
func collect(t *testing.T, c Client, id string, opt LogsOptions, tty bool) []LogLine {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	rc, err := c.Logs(ctx, id, opt)
	if err != nil {
		t.Fatalf("Logs(%s) : %v", id, err)
	}
	defer rc.Close()
	var out []LogLine
	if err := ReadLogs(rc, tty, MaxLogLine, func(l LogLine) error { out = append(out, l); return nil }); err != nil {
		t.Fatalf("ReadLogs(%s) : %v", id, err)
	}
	return out
}

// texts: "stream:text" per line, a long text shortened to its length (the
// test output stays readable). The first two lines are sorted: stdout and
// stderr written at the same instant reach the log file in either order.
func texts(ls []LogLine) []string {
	var out []string
	for _, l := range ls {
		s := l.Stream + ":" + l.Text
		if len(l.Text) > 64 {
			s = fmt.Sprintf("%s:%d×%c tronquée=%v", l.Stream, len(l.Text), l.Text[0], l.Truncated)
		}
		out = append(out, s)
	}
	if len(out) >= 2 {
		slices.Sort(out[:2])
	}
	return out
}

// F6 on a real daemon: stdout and stderr apart (multiplexed), one stream
// with a TTY, a 40 000-byte line joined back, a stopped container's past
// logs (Follow ends by itself), Since inclusive, a driver without reading.
func TestLogsRealDocker(t *testing.T) {
	plain, tty, none := "hublot-it-logs", "hublot-it-logs-tty", "hublot-it-logs-none"
	exec.Command("docker", "rm", "-f", plain, tty, none).Run()
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", plain, tty, none).Run() })
	script := `echo out1; echo err1 >&2; head -c 40000 /dev/zero | tr "\0" a; echo; echo out2`
	for _, args := range [][]string{
		{"run", "-d", "--name", plain, "alpine:3", "sh", "-c", script},
		{"run", "-d", "-t", "--name", tty, "alpine:3", "sh", "-c", script},
		{"run", "-d", "--name", none, "--log-driver", "none", "alpine:3", "true"},
	} {
		if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
			t.Fatalf("docker %v : %v %s", args, err, out)
		}
	}
	exec.Command("docker", "wait", plain, tty).Run() // stopped: past logs only
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	info, err := c.Inspect(ctx, tty)
	if err != nil || !info.Tty || info.LogDriver != "json-file" {
		t.Fatalf("Inspect TTY : %+v %v", info, err)
	}

	long := fmt.Sprintf("stdout:%d×a tronquée=true", MaxLogLine)
	got := collect(t, c, plain, LogsOptions{Tail: "all", Follow: true}, false)
	if want := []string{"stderr:err1", "stdout:out1", long, "stdout:out2"}; !slices.Equal(texts(got), want) || got[0].TS.IsZero() {
		t.Fatalf("sans TTY : %q, attendu %q", texts(got), want)
	}
	// Since is inclusive: from the timestamp of the long line, it comes again.
	again := collect(t, c, plain, LogsOptions{Tail: "all", Since: got[2].TS.Format(time.RFC3339Nano), Follow: true}, false)
	if want := []string{long, "stdout:out2"}; !slices.Equal(texts(again), want) {
		t.Fatalf("Since inclusif : %q, attendu %q", texts(again), want)
	}
	if tail := collect(t, c, plain, LogsOptions{Tail: "1"}, false); !slices.Equal(texts(tail), []string{"stdout:out2"}) {
		t.Fatalf("Tail 1 : %q", texts(tail))
	}

	got = collect(t, c, tty, LogsOptions{Tail: "all", Follow: true}, true)
	if want := []string{"stdout:err1", "stdout:out1", long, "stdout:out2"}; !slices.Equal(texts(got), want) {
		t.Fatalf("TTY : %q, attendu %q", texts(got), want)
	}

	if _, err := c.Logs(ctx, none, LogsOptions{Tail: "all"}); !errors.Is(err, ErrNoLogs) {
		t.Fatalf("--log-driver none : %v, attendu ErrNoLogs", err)
	}
}
