package main

import (
	"bytes"
	"context"
	"net"
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

func TestRunListenFailureKeepsNoOrphanToken(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	dir := t.TempDir()
	var out, errb bytes.Buffer
	code := run(context.Background(), []string{"--data-dir", dir, "--listen", busy.Addr().String()}, noenv, &out, &errb)
	if code != 1 || strings.Contains(out.String(), "Jeton d'accès") {
		t.Fatalf("code %d, out %q, err %q", code, out.String(), errb.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "token.sha256")); !os.IsNotExist(err) {
		t.Fatalf("token must not be created, stat err: %v", err)
	}
}

func modeOf(t *testing.T, dir string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

func TestRunRefusesWritableDataDir(t *testing.T) {
	for _, sub := range [][]string{nil, {"reset-token"}} {
		dir := t.TempDir()
		if err := os.Chmod(dir, 0o777); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var out, errb bytes.Buffer
		args := append(sub, "--data-dir", dir, "--listen", "127.0.0.1:0")
		if code := run(ctx, args, noenv, &out, &errb); code != 1 || !strings.Contains(errb.String(), dir) {
			t.Fatalf("%v: code %d, stderr %q", sub, code, errb.String())
		}
		if m := modeOf(t, dir); m != 0o777 {
			t.Fatalf("%v: mode changed to %v", sub, m)
		}
		if _, err := os.Stat(filepath.Join(dir, "token.sha256")); !os.IsNotExist(err) {
			t.Fatalf("%v: no token may be written, stat err: %v", sub, err)
		}
	}
}

func TestRunKeepsExistingDataDirMode(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, errb bytes.Buffer
	if code := run(ctx, []string{"--data-dir", dir, "--listen", "127.0.0.1:0"}, noenv, &out, &errb); code != 0 {
		t.Fatalf("code %d, stderr %q", code, errb.String())
	}
	if m := modeOf(t, dir); m != 0o755 {
		t.Fatalf("mode changed to %v", m)
	}
}

func TestRunCreatesDataDir0700(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new", "data")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, errb bytes.Buffer
	if code := run(ctx, []string{"--data-dir", dir, "--listen", "127.0.0.1:0"}, noenv, &out, &errb); code != 0 {
		t.Fatalf("code %d, stderr %q", code, errb.String())
	}
	if m := modeOf(t, dir); m != 0o700 {
		t.Fatalf("mode %v, want 0700", m)
	}
}

func TestRunBadTLSCert(t *testing.T) {
	dir := t.TempDir()
	var out, errb bytes.Buffer
	code := run(context.Background(), []string{"--data-dir", dir, "--listen", "127.0.0.1:0",
		"--tls-cert", filepath.Join(dir, "no.crt"), "--tls-key", filepath.Join(dir, "no.key")}, noenv, &out, &errb)
	if code != 1 || strings.Contains(out.String(), "écoute sur") {
		t.Fatalf("code %d, out %q, err %q", code, out.String(), errb.String())
	}
}
