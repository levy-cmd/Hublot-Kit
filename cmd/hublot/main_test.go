package main

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levy-cmd/hublot-kit/internal/fsview"
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
		for _, want := range []string{"--listen", "--data-dir", "--read-only", "--tls-cert", "--tls-key", "--public-origin", "--version", "reset-token"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("%s: usage lacks %q: %q", flag, want, out.String())
			}
		}
	}
}

func TestRunVersion(t *testing.T) {
	old := version
	version = "1.2.3"
	t.Cleanup(func() { version = old })
	for _, flag := range []string{"--version", "-version"} {
		var out, errb bytes.Buffer
		// The data directory is never touched: --version answers before
		// reading the configuration (a file stands where it would go).
		blocker := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(blocker, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		env := func(k string) string {
			if k == "HUBLOT_DATA_DIR" {
				return filepath.Join(blocker, "data")
			}
			return ""
		}
		if code := run(context.Background(), []string{flag}, env, &out, &errb); code != 0 {
			t.Fatalf("%s: code %d, stderr %q", flag, code, errb.String())
		}
		if out.String() != "hublot 1.2.3\n" || errb.Len() != 0 {
			t.Errorf("%s: out %q, stderr %q", flag, out.String(), errb.String())
		}
	}
}

// --version answers only when alone: with another argument it stays a
// configuration error, and the data directory is not created.
func TestRunVersionWithOtherArgs(t *testing.T) {
	for _, args := range [][]string{{"--version", "--nope"}, {"--version", "extra"}} {
		dataDir := filepath.Join(t.TempDir(), "data")
		env := func(k string) string {
			if k == "HUBLOT_DATA_DIR" {
				return dataDir
			}
			return ""
		}
		var out, errb bytes.Buffer
		if code := run(context.Background(), args, env, &out, &errb); code != 2 {
			t.Errorf("%q: want exit 2, got %d (out %q)", args, code, out.String())
		}
		if _, err := os.Lstat(dataDir); !os.IsNotExist(err) {
			t.Errorf("%q: data directory touched (%v)", args, err)
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
	for _, mode := range []os.FileMode{0o700, 0o750, 0o755} {
		dir := t.TempDir()
		if err := os.Chmod(dir, mode); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var out, errb bytes.Buffer
		if code := run(ctx, []string{"--data-dir", dir, "--listen", "127.0.0.1:0"}, noenv, &out, &errb); code != 0 {
			t.Fatalf("%v: code %d, stderr %q", mode, code, errb.String())
		}
		if m := modeOf(t, dir); m != mode {
			t.Fatalf("%v: mode changed to %v", mode, m)
		}
	}
}

// runRefused runs the agent (and reset-token) on dataDir and expects a refusal
// mentioning dataDir and want, with no token written in checkDir.
func runRefused(t *testing.T, dataDir, checkDir, want string) {
	t.Helper()
	for _, sub := range [][]string{nil, {"reset-token"}} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var out, errb bytes.Buffer
		args := append(sub, "--data-dir", dataDir, "--listen", "127.0.0.1:0")
		code := run(ctx, args, noenv, &out, &errb)
		if code != 1 || !strings.Contains(errb.String(), dataDir) || !strings.Contains(errb.String(), want) {
			t.Fatalf("%v: code %d, stderr %q (want %q)", sub, code, errb.String(), want)
		}
		if strings.Contains(out.String(), "Jeton d'accès") || strings.Contains(out.String(), "Nouveau jeton") {
			t.Fatalf("%v: no token may be shown: %q", sub, out.String())
		}
		if checkDir == "" {
			continue
		}
		if _, err := os.Lstat(filepath.Join(checkDir, "token.sha256")); !os.IsNotExist(err) {
			t.Fatalf("%v: no token may be written, lstat err: %v", sub, err)
		}
		if _, err := os.Lstat(filepath.Join(checkDir, "audit.log")); !os.IsNotExist(err) {
			t.Fatalf("%v: no audit log may be written, lstat err: %v", sub, err)
		}
	}
}

func TestRunRefusesSymlinkDataDir(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "real")
	if err := os.Mkdir(target, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o750); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "data")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	runRefused(t, link, target, "lien symbolique")
	if m := modeOf(t, target); m != 0o750 {
		t.Fatalf("link target mode changed to %v", m)
	}

	// A dangling link must not lead to creating its target either.
	dangling := filepath.Join(parent, "dangling")
	missing := filepath.Join(parent, "missing")
	if err := os.Symlink(missing, dangling); err != nil {
		t.Fatal(err)
	}
	runRefused(t, dangling, "", "lien symbolique")
	if _, err := os.Lstat(missing); !os.IsNotExist(err) {
		t.Fatalf("dangling link target must not be created, lstat err: %v", err)
	}
}

func TestRunRefusesDataDirThatIsAFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "data")
	if err := os.WriteFile(f, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	runRefused(t, f, "", "pas un dossier")
}

// A directory pre-created by another local user (e.g. /tmp/hublot) must be
// refused: its owner could plant their own token.sha256. Needs root to chown.
func TestRunRefusesDataDirOwnedByAnotherUser(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("chown vers un autre uid : nécessite root (lancer sous Linux en root)")
	}
	dir := filepath.Join(t.TempDir(), "hublot")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(dir, 65534, 65534); err != nil {
		t.Fatal(err)
	}
	runRefused(t, dir, dir, "appartient")
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

func TestRunWarnsPrivateCgroupNamespace(t *testing.T) {
	old := selfCgroup
	selfCgroup = func() ([]byte, error) { return []byte("0::/\n"), nil }
	t.Cleanup(func() { selfCgroup = old; fsview.NoteCgroupNamespace("") })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, errb bytes.Buffer
	if code := run(ctx, []string{"--data-dir", t.TempDir(), "--listen", "127.0.0.1:0"}, noenv, &out, &errb); code != 0 {
		t.Fatalf("code %d, stderr %q", code, errb.String())
	}
	if !strings.Contains(errb.String(), "--cgroupns=host") {
		t.Fatalf("avertissement absent : %q", errb.String())
	}
}

func TestRunNoWarningOnHostCgroup(t *testing.T) {
	old := selfCgroup
	selfCgroup = func() ([]byte, error) { return []byte("0::/system.slice/hublot.service\n"), nil }
	t.Cleanup(func() { selfCgroup = old; fsview.NoteCgroupNamespace("") })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, errb bytes.Buffer
	if code := run(ctx, []string{"--data-dir", t.TempDir(), "--listen", "127.0.0.1:0"}, noenv, &out, &errb); code != 0 {
		t.Fatalf("code %d, stderr %q", code, errb.String())
	}
	if strings.Contains(errb.String(), "cgroupns") {
		t.Fatalf("avertissement inattendu : %q", errb.String())
	}
}
