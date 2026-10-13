// Package config reads the agent's options from flags and HUBLOT_* variables.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

type Config struct {
	Listen       string
	DataDir      string
	ReadOnly     bool
	TLSCert      string
	TLSKey       string
	PublicOrigin string
}

// TLS reports whether the agent serves HTTPS itself.
func (c Config) TLS() bool { return c.TLSCert != "" }

// Load parses args. Precedence: flag, then HUBLOT_* variable, then default.
func Load(args []string, getenv func(string) string) (Config, error) {
	or := func(name, def string) string {
		if v := getenv(name); v != "" {
			return v
		}
		return def
	}
	readOnly, err := strconv.ParseBool(or("HUBLOT_READ_ONLY", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("HUBLOT_READ_ONLY: %w", err)
	}

	var c Config
	fs := flag.NewFlagSet("hublot", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&c.Listen, "listen", or("HUBLOT_LISTEN", "127.0.0.1:7878"), "listen address")
	fs.StringVar(&c.DataDir, "data-dir", or("HUBLOT_DATA_DIR", "/var/lib/hublot-kit"), "data directory")
	fs.BoolVar(&c.ReadOnly, "read-only", readOnly, "disable every write")
	fs.StringVar(&c.TLSCert, "tls-cert", or("HUBLOT_TLS_CERT", ""), "TLS certificate")
	fs.StringVar(&c.TLSKey, "tls-key", or("HUBLOT_TLS_KEY", ""), "TLS key")
	fs.StringVar(&c.PublicOrigin, "public-origin", or("HUBLOT_PUBLIC_ORIGIN", ""), "public origin behind a reverse proxy")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	if fs.NArg() > 0 {
		return Config{}, fmt.Errorf("positional arguments not allowed: %v", fs.Args())
	}

	if (c.TLSCert == "") != (c.TLSKey == "") {
		return Config{}, errors.New("--tls-cert/HUBLOT_TLS_CERT and --tls-key/HUBLOT_TLS_KEY go together")
	}
	if c.PublicOrigin != "" {
		o, err := normalizeOrigin(c.PublicOrigin)
		if err != nil {
			return Config{}, fmt.Errorf("--public-origin: %w", err)
		}
		c.PublicOrigin = o
	}
	return c, nil
}

// normalizeOrigin returns scheme://host[:port] in lower case, or an error.
func normalizeOrigin(s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", err
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", errors.New("http or https scheme expected")
	}
	if u.Host == "" {
		return "", errors.New("missing host")
	}
	if u.User != nil {
		return "", errors.New("user credentials not allowed")
	}
	if strings.Contains(u.Host, "%") {
		return "", errors.New("IPv6 zone ID not allowed in Host")
	}
	if u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("an origin has no path, query or fragment")
	}
	if u.ForceQuery || (len(s) > 0 && s[len(s)-1] == '#') {
		return "", errors.New("an origin has no path, query or fragment")
	}
	host := strings.ToLower(u.Host)
	// Strip default ports
	if scheme == "https" && strings.HasSuffix(host, ":443") {
		host = host[:len(host)-4]
	} else if scheme == "http" && strings.HasSuffix(host, ":80") {
		host = host[:len(host)-3]
	}
	return scheme + "://" + host, nil
}
