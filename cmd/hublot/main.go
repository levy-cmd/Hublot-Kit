// Command hublot is the Hublot-Kit agent.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
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
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
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

	tok, created, err := auth.EnsureToken(cfg.DataDir)
	if err != nil {
		fmt.Fprintln(stderr, "jeton :", err)
		return 1
	}
	if created {
		fmt.Fprintf(stdout, "Jeton d'accès (affiché une seule fois) : %s\n", tok)
	}
	verifier, err := auth.LoadVerifier(cfg.DataDir)
	if err != nil {
		fmt.Fprintln(stderr, "jeton :", err)
		return 1
	}
	auditLog, err := audit.Open(filepath.Join(cfg.DataDir, "audit.log"))
	if err != nil {
		fmt.Fprintln(stderr, "journal d'audit :", err)
		return 1
	}
	defer auditLog.Close()
	docker, err := dockerx.New()
	if err != nil {
		fmt.Fprintln(stderr, "client Docker :", err)
		return 1
	}

	if !isLoopback(cfg.Listen) && !cfg.TLS() {
		fmt.Fprintf(stderr, "ATTENTION : écoute sur %s sans TLS. À réserver à un conteneur publié sur 127.0.0.1 ou à un reverse proxy HTTPS.\n", cfg.Listen)
	}

	handler := api.New(api.Deps{
		Docker: docker, Verifier: verifier, Audit: auditLog, Version: version,
		Sessions: auth.NewSessions(auth.SessionTTL, time.Now),
		Limiter:  auth.NewLimiter(5, time.Minute, time.Now),
		ReadOnly: cfg.ReadOnly, TLS: cfg.TLS(), PublicOrigin: cfg.PublicOrigin,
		Static: web.Handler(),
	})
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		fmt.Fprintln(stderr, "écoute :", err)
		return 1
	}
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() {
		if cfg.TLS() {
			errc <- srv.ServeTLS(ln, cfg.TLSCert, cfg.TLSKey)
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
			return 1
		}
	}
	return 0
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
