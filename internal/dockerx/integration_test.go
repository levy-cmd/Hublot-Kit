//go:build integration

package dockerx

import (
	"context"
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
