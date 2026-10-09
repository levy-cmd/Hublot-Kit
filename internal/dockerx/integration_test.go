//go:build integration

package dockerx

import (
	"archive/tar"
	"context"
	"errors"
	"os/exec"
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
