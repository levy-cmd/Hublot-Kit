package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	if code != 0 || !strings.Contains(out.String(), "New access token") {
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
	if !strings.Contains(out.String(), "Access token") {
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
	if code != 1 || strings.Contains(out.String(), "Access token") {
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
		if strings.Contains(out.String(), "Access token") || strings.Contains(out.String(), "New access token") {
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
	runRefused(t, link, target, "symbolic link")
	if m := modeOf(t, target); m != 0o750 {
		t.Fatalf("link target mode changed to %v", m)
	}

	// A dangling link must not lead to creating its target either.
	dangling := filepath.Join(parent, "dangling")
	missing := filepath.Join(parent, "missing")
	if err := os.Symlink(missing, dangling); err != nil {
		t.Fatal(err)
	}
	runRefused(t, dangling, "", "symbolic link")
	if _, err := os.Lstat(missing); !os.IsNotExist(err) {
		t.Fatalf("dangling link target must not be created, lstat err: %v", err)
	}
}

func TestRunRefusesDataDirThatIsAFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "data")
	if err := os.WriteFile(f, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	runRefused(t, f, "", "not a directory")
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
	runRefused(t, dir, dir, "owned by uid")
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
	if code != 1 || strings.Contains(out.String(), "listening on") {
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

// v0.1.2 (audit, constat 2): the agent's server bounds the reading of a
// request and idle connections, never the writing (logs stream, downloads).
func TestNewServerTimeouts(t *testing.T) {
	srv := newServer()
	if srv.ReadHeaderTimeout <= 0 || srv.ReadTimeout <= 0 || srv.IdleTimeout <= 0 || srv.WriteTimeout != 0 {
		t.Fatalf("délais : en-têtes %s, lecture %s, inactivité %s, écriture %s", srv.ReadHeaderTimeout, srv.ReadTimeout, srv.IdleTimeout, srv.WriteTimeout)
	}
}

// closedSoon: the server closed conn (EOF or reset) within limit.
func closedSoon(conn net.Conn, limit time.Duration) bool {
	conn.SetReadDeadline(time.Now().Add(limit))
	_, err := io.Copy(io.Discard, conn)
	var ne net.Error
	return !(errors.As(err, &ne) && ne.Timeout())
}

// v0.1.2 (counter-audit, constat 2): the cap of 512 was global; 2 000 mute
// sockets of one address made three legitimate requests out of four time
// out. Each source address (RemoteAddr) holds at most PerIP connections:
// past that, a new one is closed at once.
func TestLimitListenerPerIP(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln := limitListener(inner, connLimits{Max: 10, PerIP: 2, FirstByte: time.Minute})
	defer ln.Close()
	accepted := make(chan net.Conn, 10)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			accepted <- c
		}
	}()
	dial := func(local string) net.Conn {
		d := net.Dialer{}
		if local != "" {
			d.LocalAddr = &net.TCPAddr{IP: net.ParseIP(local)}
		}
		c, err := d.Dial("tcp", inner.Addr().String())
		if err != nil {
			return nil
		}
		t.Cleanup(func() { c.Close() })
		return c
	}
	var got []net.Conn
	for i := 0; i < 2; i++ {
		dial("")
		select {
		case c := <-accepted:
			got = append(got, c)
		case <-time.After(2 * time.Second):
			t.Fatal("connexion sous la limite par adresse non acceptée")
		}
	}
	extra := dial("")
	if !closedSoon(extra, 2*time.Second) {
		t.Fatal("3e connexion de la même adresse gardée ouverte")
	}
	select {
	case <-accepted:
		t.Fatal("3e connexion de la même adresse remise au serveur")
	default:
	}
	if other := dial("127.0.0.2"); other != nil { // Linux: all of 127/8 is local
		select {
		case c := <-accepted:
			c.Close()
		case <-time.After(2 * time.Second):
			t.Fatal("une autre adresse est refusée")
		}
	}
	got[0].Close()
	dial("")
	select {
	case c := <-accepted:
		c.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("place rendue à l'adresse non reprise")
	}
	got[1].Close()
}

// fakeListener hands out one end of a pipe per remote address given.
type fakeListener struct{ conns chan net.Conn }

func (l fakeListener) Accept() (net.Conn, error) {
	c, ok := <-l.conns
	if !ok {
		return nil, net.ErrClosed
	}
	return c, nil
}
func (l fakeListener) Close() error   { return nil }
func (l fakeListener) Addr() net.Addr { return &net.TCPAddr{} }

type remoteConn struct {
	net.Conn
	remote net.Addr
}

func (c remoteConn) RemoteAddr() net.Addr { return c.remote }

// v0.1.2 (counter-audit): counted per address, one IPv6 host with its /64
// filled the 512 connections. The cap counts an IPv6 /64 as one address, and
// an IPv4 address mapped in IPv6 as that IPv4 address.
func TestLimitListenerGroupsAddresses(t *testing.T) {
	fl := fakeListener{conns: make(chan net.Conn, 16)}
	ln := limitListener(fl, connLimits{Max: 20, PerIP: 2, FirstByte: time.Minute})
	accepted := make(chan net.Conn, 16)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			accepted <- c
		}
	}()
	try := func(remote string) (ok bool, peer net.Conn) {
		srvEnd, cliEnd := net.Pipe()
		t.Cleanup(func() { srvEnd.Close(); cliEnd.Close() })
		ap, err := netip.ParseAddrPort(remote)
		if err != nil {
			t.Fatal(err)
		}
		fl.conns <- remoteConn{srvEnd, net.TCPAddrFromAddrPort(ap)}
		select {
		case c := <-accepted:
			return true, c
		case <-time.After(300 * time.Millisecond):
			return false, nil // closed at once; Accept waits for the next one
		}
	}
	feed := func(want bool, remotes ...string) {
		t.Helper()
		for _, r := range remotes {
			if ok, _ := try(r); ok != want {
				t.Errorf("%s : acceptée %v, attendu %v", r, ok, want)
			}
		}
	}
	feed(true, "[2001:db8:1:2::1]:1", "[2001:db8:1:2::2]:1")
	feed(false, "[2001:db8:1:2:ffff::3]:1") // same /64
	feed(true, "[2001:db8:1:3::1]:1")       // another /64
	feed(true, "192.0.2.1:1", "[::ffff:192.0.2.1]:2")
	feed(false, "[::ffff:192.0.2.1]:3", "192.0.2.1:4")
	feed(true, "192.0.2.2:1")
	close(fl.conns)
}

// A connection that sends no byte is closed after FirstByte, well before
// ReadHeaderTimeout; once a byte came, the server's own delays apply.
func TestMuteConnectionClosedFast(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln := limitListener(inner, connLimits{Max: 10, PerIP: 10, FirstByte: 200 * time.Millisecond})
	srv := newServer()
	srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") })
	go srv.Serve(ln)
	defer srv.Close()
	mute, err := net.Dial("tcp", inner.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer mute.Close()
	start := time.Now()
	if !closedSoon(mute, 3*time.Second) || time.Since(start) > 2*time.Second {
		t.Fatalf("connexion muette encore ouverte après %s", time.Since(start))
	}
	slow, err := net.Dial("tcp", inner.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer slow.Close()
	slow.Write([]byte("G"))
	time.Sleep(400 * time.Millisecond) // past FirstByte, within ReadHeaderTimeout
	slow.Write([]byte("ET / HTTP/1.1\r\nHost: x\r\n\r\n"))
	slow.SetReadDeadline(time.Now().Add(3 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(slow), nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("requête lente mais commencée : %v %v", resp, err)
	}
}

// At most n connections at once: the next one waits until one closes.
func TestLimitListener(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln := limitListener(inner, connLimits{Max: 2, PerIP: 10, FirstByte: time.Minute})
	defer ln.Close()
	accepted := make(chan net.Conn, 3)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			accepted <- c
		}
	}()
	for i := 0; i < 3; i++ {
		c, err := net.Dial("tcp", inner.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
	}
	var got []net.Conn
	for len(got) < 2 {
		select {
		case c := <-accepted:
			got = append(got, c)
		case <-time.After(2 * time.Second):
			t.Fatalf("%d connexions acceptées, attendu 2", len(got))
		}
	}
	select {
	case <-accepted:
		t.Fatal("3e connexion acceptée au-delà de la limite")
	case <-time.After(300 * time.Millisecond):
	}
	got[0].Close()
	got[0].Close() // a second Close frees nothing more
	select {
	case c := <-accepted:
		c.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("3e connexion jamais acceptée après une fermeture")
	}
	// Every slot taken, Accept waits: Close ends it (Shutdown waits for Serve).
	ln.Close()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Accept toujours bloqué après Close")
	}
	got[1].Close()
}
