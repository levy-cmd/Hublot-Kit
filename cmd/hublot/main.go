// Command hublot is the Hublot-Kit agent.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/api"
	"github.com/levy-cmd/hublot-kit/internal/audit"
	"github.com/levy-cmd/hublot-kit/internal/auth"
	"github.com/levy-cmd/hublot-kit/internal/config"
	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
	"github.com/levy-cmd/hublot-kit/internal/web"
)

var version = "dev" // set by -ldflags "-X main.version=…"

// selfCgroup reads the agent's own cgroup for the startup check of spec §8;
// tests replace it. Outside Linux the read fails and the check is skipped.
var selfCgroup = func() ([]byte, error) { return os.ReadFile("/proc/self/cgroup") }

const usage = `Usage: hublot [options]
       hublot reset-token [--data-dir DIR]
       hublot --version

Options (each has a HUBLOT_* equivalent):
  --listen ADDR          listen address (default 127.0.0.1:7878)
  --data-dir DIR         data directory (default /var/lib/hublot-kit)
  --read-only            disable every write
  --tls-cert FILE        TLS certificate (with --tls-key)
  --tls-key FILE         TLS key (with --tls-cert)
  --public-origin URL    public origin behind a reverse proxy

Subcommand:
  reset-token            create a new access token and invalidate the old one
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() { <-ctx.Done(); stop() }() // a second Ctrl-C then uses the default behaviour
	os.Exit(run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	// Before the configuration: --version never reads HUBLOT_* nor touches the data directory.
	if len(args) == 1 && (args[0] == "--version" || args[0] == "-version") {
		fmt.Fprintf(stdout, "hublot %s\n", version)
		return 0
	}
	resetToken := len(args) > 0 && args[0] == "reset-token"
	if resetToken {
		args = args[1:]
	}
	cfg, err := config.Load(args, getenv)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if err != nil {
		fmt.Fprintln(stderr, "configuration:", err)
		return 2
	}
	if err := prepareDataDir(cfg.DataDir); err != nil {
		fmt.Fprintf(stderr, "data directory %s: %v\n", cfg.DataDir, err)
		return 1
	}

	if resetToken {
		tok, err := auth.ResetToken(cfg.DataDir)
		if err != nil {
			fmt.Fprintln(stderr, "reset-token:", err)
			return 1
		}
		fmt.Fprintf(stdout, "New access token (shown only once): %s\n", tok)
		return 0
	}

	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second}
	// A logs stream (SSE, F6) never ends by itself: Shutdown cancels the
	// context of every request, so the streams return, then waits for them.
	base, cancelBase := context.WithCancel(context.Background())
	defer cancelBase()
	srv.BaseContext = func(net.Listener) context.Context { return base }
	srv.RegisterOnShutdown(cancelBase)
	if cfg.TLS() {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			fmt.Fprintln(stderr, "TLS certificate:", err)
			return 1
		}
		srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
	}

	if !isLoopback(cfg.Listen) && !cfg.TLS() {
		fmt.Fprintf(stderr, "WARNING: listening on %s without TLS. Only for a container published on 127.0.0.1 or behind an HTTPS reverse proxy.\n", cfg.Listen)
	}

	// Listen before creating the token, so a port conflict never burns it.
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		fmt.Fprintln(stderr, "listen:", err)
		return 1
	}
	defer ln.Close()

	tok, created, err := auth.EnsureToken(cfg.DataDir)
	if err != nil {
		fmt.Fprintln(stderr, "token:", err)
		return 1
	}
	if created {
		fmt.Fprintf(stdout, "Access token (shown only once): %s\n", tok)
	}
	fail := func(what string, err error) int {
		fmt.Fprintf(stderr, "%s: %v\n", what, err)
		if created {
			fmt.Fprintln(stderr, "The token printed above stays valid; if in doubt, run `hublot reset-token`.")
		}
		return 1
	}
	verifier, err := auth.LoadVerifier(cfg.DataDir)
	if err != nil {
		return fail("token", err)
	}
	auditLog, err := audit.Open(filepath.Join(cfg.DataDir, "audit.log"))
	if err != nil {
		return fail("audit log", err)
	}
	defer auditLog.Close()
	docker, err := dockerx.New()
	if err != nil {
		return fail("Docker client", err)
	}
	if b, err := selfCgroup(); err == nil && fsview.NoteCgroupNamespace(string(b)) {
		fmt.Fprintln(stderr, "WARNING: private cgroup namespace (/proc/self/cgroup = 0::/): the explorer will answer container_changed. Start the agent with --cgroupns=host.")
	}

	srv.Handler = api.New(api.Deps{
		Docker: docker, Verifier: verifier, Audit: auditLog, Version: version,
		Sessions: auth.NewSessions(auth.SessionTTL, time.Now),
		Limiter:  auth.NewLimiter(5, time.Minute, time.Now),
		ReadOnly: cfg.ReadOnly, TLS: cfg.TLS(), PublicOrigin: cfg.PublicOrigin,
		Static: web.Handler(), ScriptHashes: web.ScriptHashes(),
	})
	errc := make(chan error, 1)
	go func() {
		if cfg.TLS() {
			errc <- srv.ServeTLS(ln, "", "")
		} else {
			errc <- srv.Serve(ln)
		}
	}()
	fmt.Fprintf(stdout, "Hublot-Kit %s listening on %s\n", version, ln.Addr())

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(stderr, "server:", err)
			return 1
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintln(stderr, "shutdown:", err)
			srv.Close()
			return 1
		}
	}
	return 0
}

// prepareDataDir creates the data directory with mode 0700 if it does not
// exist (only its parents are created with MkdirAll). An existing directory
// keeps its mode (never chmod /tmp, ., $HOME…). Every check is made on a
// descriptor opened with O_NOFOLLOW, never on the path again: the directory
// must not be a symbolic link, must belong to the effective user (the token
// lives there: a directory pre-created by someone else, e.g. /tmp/hublot,
// would let its owner plant their own token) and must not be writable by
// group or others.
func prepareDataDir(dir string) error {
	dir = filepath.Clean(dir)
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return err
	}
	created := true
	if err := os.Mkdir(dir, 0o700); errors.Is(err, fs.ErrExist) {
		created = false
	} else if err != nil {
		return err
	}
	d, err := os.OpenFile(dir, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, syscall.ELOOP) || errors.Is(err, syscall.ENOTDIR) {
		// The open already refused; Lstat only picks the message (Linux
		// answers ELOOP for a link, macOS ENOTDIR).
		if li, lerr := os.Lstat(dir); lerr == nil && li.Mode()&fs.ModeSymlink != 0 {
			return errors.New("it is a symbolic link: refused; give a dedicated directory directly, for example /var/lib/hublot-kit")
		}
		return errors.New("not a directory")
	}
	if err != nil {
		return err
	}
	defer d.Close()
	fi, err := d.Stat()
	if err != nil {
		return err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !fi.IsDir() || !ok {
		return errors.New("not a directory")
	}
	if euid := os.Geteuid(); int(st.Uid) != euid {
		return fmt.Errorf("owned by uid %d, not by the current user (uid %d); use a dedicated directory, for example /var/lib/hublot-kit", st.Uid, euid)
	}
	if created {
		return d.Chmod(0o700) // the umask may have removed owner bits
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("writable by group or others (%v); use a dedicated directory, for example /var/lib/hublot-kit", fi.Mode().Perm())
	}
	return nil
}

func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
