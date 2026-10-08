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
	"github.com/levy-cmd/hublot-kit/internal/web"
)

var version = "dev" // set by -ldflags "-X main.version=…"

const usage = `Usage : hublot [options]
        hublot reset-token [--data-dir DIR]

Options (chacune a un équivalent HUBLOT_*) :
  --listen ADDR          adresse d'écoute (défaut 127.0.0.1:7878)
  --data-dir DIR         dossier de données (défaut /var/lib/hublot-kit)
  --read-only            désactive toute écriture
  --tls-cert FILE        certificat TLS (avec --tls-key)
  --tls-key FILE         clé TLS (avec --tls-cert)
  --public-origin URL    origine publique derrière un reverse proxy

Sous-commande :
  reset-token            génère un nouveau jeton d'accès et invalide l'ancien
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() { <-ctx.Done(); stop() }() // a second Ctrl-C then uses the default behaviour
	os.Exit(run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
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
		fmt.Fprintln(stderr, "configuration :", err)
		return 2
	}
	if err := prepareDataDir(cfg.DataDir); err != nil {
		fmt.Fprintf(stderr, "dossier de données %s : %v\n", cfg.DataDir, err)
		return 1
	}

	if resetToken {
		tok, err := auth.ResetToken(cfg.DataDir)
		if err != nil {
			fmt.Fprintln(stderr, "reset-token :", err)
			return 1
		}
		fmt.Fprintf(stdout, "Nouveau jeton d'accès (affiché une seule fois) : %s\n", tok)
		return 0
	}

	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second}
	if cfg.TLS() {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			fmt.Fprintln(stderr, "certificat TLS :", err)
			return 1
		}
		srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
	}

	if !isLoopback(cfg.Listen) && !cfg.TLS() {
		fmt.Fprintf(stderr, "ATTENTION : écoute sur %s sans TLS. À réserver à un conteneur publié sur 127.0.0.1 ou à un reverse proxy HTTPS.\n", cfg.Listen)
	}

	// Listen before creating the token, so a port conflict never burns it.
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		fmt.Fprintln(stderr, "écoute :", err)
		return 1
	}
	defer ln.Close()

	tok, created, err := auth.EnsureToken(cfg.DataDir)
	if err != nil {
		fmt.Fprintln(stderr, "jeton :", err)
		return 1
	}
	if created {
		fmt.Fprintf(stdout, "Jeton d'accès (affiché une seule fois) : %s\n", tok)
	}
	fail := func(what string, err error) int {
		fmt.Fprintln(stderr, what, ":", err)
		if created {
			fmt.Fprintln(stderr, "Le jeton affiché plus haut reste valide ; en cas de doute, lancez `hublot reset-token`.")
		}
		return 1
	}
	verifier, err := auth.LoadVerifier(cfg.DataDir)
	if err != nil {
		return fail("jeton", err)
	}
	auditLog, err := audit.Open(filepath.Join(cfg.DataDir, "audit.log"))
	if err != nil {
		return fail("journal d'audit", err)
	}
	defer auditLog.Close()
	docker, err := dockerx.New()
	if err != nil {
		return fail("client Docker", err)
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
	fmt.Fprintf(stdout, "Hublot-Kit %s écoute sur %s\n", version, ln.Addr())

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(stderr, "serveur :", err)
			return 1
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintln(stderr, "arrêt :", err)
			srv.Close()
			return 1
		}
	}
	return 0
}

// prepareDataDir creates the data directory with mode 0700 if it does not
// exist. An existing directory keeps its mode (never chmod /tmp, ., $HOME…),
// but one writable by group or others is refused: the token lives there.
func prepareDataDir(dir string) error {
	fi, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		return os.Chmod(dir, 0o700)
	}
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return errors.New("ce n'est pas un dossier")
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("accessible en écriture au groupe ou à tous (%v) ; utilisez un dossier dédié, par exemple /var/lib/hublot-kit", fi.Mode().Perm())
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
