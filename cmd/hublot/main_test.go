package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func noenv(string) string { return "" }

func TestIsLoopback(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:7878": true, "[::1]:7878": true, "localhost:7878": true,
		"0.0.0.0:7878": false, ":7878": false, "192.168.1.10:7878": false, "garbage": false,
	}
	for addr, want := range cases {
		if got := isLoopback(addr); got != want {
			t.Errorf("isLoopback(%q) = %v", addr, got)
		}
	}
}

func TestResetTokenSubcommand(t *testing.T) {
	dir := t.TempDir()
	var out, errb bytes.Buffer
	code := run(context.Background(), []string{"reset-token", "--data-dir", dir}, noenv, &out, &errb)
	if code != 0 || !strings.Contains(out.String(), "jeton") {
		t.Fatalf("code %d, out %q, err %q", code, out.String(), errb.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "token.sha256")); err != nil {
		t.Fatal(err)
	}
}

func TestRunBadFlag(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"--nope"}, noenv, &out, &errb); code != 2 {
		t.Fatalf("want exit 2, got %d", code)
	}
}

func TestRunHelp(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		var out, errb bytes.Buffer
		if code := run(context.Background(), []string{flag}, noenv, &out, &errb); code != 0 {
			t.Fatalf("%s: want exit 0, got %d (stderr %q)", flag, code, errb.String())
		}
		for _, want := range []string{"--listen", "--data-dir", "--read-only", "--tls-cert", "--tls-key", "--public-origin", "reset-token"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("%s: usage lacks %q: %q", flag, want, out.String())
			}
		}
	}
}

func TestRunDataDirUnwritable(t *testing.T) {
	parent := t.TempDir()
	blocker := filepath.Join(parent, "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(blocker, "data") // a file stands where a directory is needed
	var out, errb bytes.Buffer
	code := run(context.Background(), []string{"--data-dir", dataDir}, noenv, &out, &errb)
	if code != 1 || !strings.Contains(errb.String(), dataDir) {
		t.Fatalf("code %d, stderr %q", code, errb.String())
	}
}

func TestRunStartsAndStops(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	var out, errb bytes.Buffer
	done := make(chan int)
	go func() { done <- run(ctx, []string{"--data-dir", dir, "--listen", "127.0.0.1:0"}, noenv, &out, &errb) }()
	cancel()
	if code := <-done; code != 0 {
		t.Fatalf("code %d, stderr %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "Jeton d'accès") {
		t.Fatalf("first launch must print the token once: %q", out.String())
	}
}
