//go:build linux

// lsproc lists a directory of a running container through /proc/<pid>/root.
// Usage: lsproc <container> <dir> [file...]
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/levy-cmd/hublot-kit/spike/dockerapi"
	"github.com/levy-cmd/hublot-kit/spike/procroot"
)

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "FATAL "+format+"\n", a...)
	os.Exit(1)
}

func short(b []byte) string {
	s := string(b)
	if len(s) > 80 {
		s = s[:80] + "…"
	}
	return s
}

func main() {
	if len(os.Args) < 3 {
		fatal("usage: lsproc <container> <dir> [file...]")
	}
	ctx := context.Background()
	ct, err := dockerapi.New("/var/run/docker.sock").Inspect(ctx, os.Args[1], false)
	if err != nil {
		fatal("inspect: %v", err)
	}
	if !ct.State.Running {
		fatal("container is not running")
	}
	cg, cgErr := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", ct.State.Pid))
	if cgErr != nil {
		fmt.Printf("cgroup read error: %v\n", cgErr)
	}
	shortID := ct.ID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	fmt.Printf("container %s pid=%d cgroup=%q\n", shortID, ct.State.Pid, strings.TrimSpace(string(cg)))

	start := time.Now()
	root, err := procroot.Open(ct.State.Pid, ct.ID)
	if err != nil {
		fatal("open root: %v", err)
	}
	defer root.Close()
	entries, truncated, err := procroot.List(root, os.Args[2], 10000)
	if err != nil {
		fatal("list %s: %v", os.Args[2], err)
	}
	fmt.Printf("RESULT list %s: %d entries (truncated=%v) in %s\n", os.Args[2], len(entries), truncated, time.Since(start))
	for _, e := range entries[:min(5, len(entries))] {
		fmt.Printf("  %-7s %s %6d %q -> %q\n", e.Type, e.Mode, e.Size, e.Name, e.LinkTarget)
	}

	for _, f := range os.Args[3:] {
		data, err := procroot.ReadFile(root, f)
		fmt.Printf("RESULT securejoin read %s: %q err=%v\n", f, short(data), err)
		naive, err := os.ReadFile(fmt.Sprintf("/proc/%d/root/%s", ct.State.Pid, strings.TrimPrefix(f, "/")))
		fmt.Printf("CONTROL naive read %s: %q err=%v\n", f, short(naive), err)
	}
}
