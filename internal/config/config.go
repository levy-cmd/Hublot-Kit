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
	fs.StringVar(&c.Listen, "listen", or("HUBLOT_LISTEN", "127.0.0.1:7878"), "adresse d'écoute")
	fs.StringVar(&c.DataDir, "data-dir", or("HUBLOT_DATA_DIR", "/var/lib/hublot-kit"), "dossier de données")
	fs.BoolVar(&c.ReadOnly, "read-only", readOnly, "désactive toute écriture")
	fs.StringVar(&c.TLSCert, "tls-cert", or("HUBLOT_TLS_CERT", ""), "certificat TLS")
	fs.StringVar(&c.TLSKey, "tls-key", or("HUBLOT_TLS_KEY", ""), "clé TLS")
	fs.StringVar(&c.PublicOrigin, "public-origin", or("HUBLOT_PUBLIC_ORIGIN", ""), "origine publique derrière un reverse proxy")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	if fs.NArg() > 0 {
		return Config{}, fmt.Errorf("arguments positionnels non autorisés: %v", fs.Args())
	}

	if (c.TLSCert == "") != (c.TLSKey == "") {
		return Config{}, errors.New("--tls-cert/HUBLOT_TLS_CERT et --tls-key/HUBLOT_TLS_KEY vont ensemble")
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
		return "", errors.New("schéma http ou https attendu")
	}
	if u.Host == "" {
		return "", errors.New("hôte manquant")
	}
	if u.User != nil {
		return "", errors.New("authentification utilisateur non autorisée")
	}
	if strings.Contains(u.Host, "%") {
		return "", errors.New("zone ID IPv6 non autorisée dans Host")
	}
	if u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("une origine n'a ni chemin, ni requête, ni fragment")
	}
	if u.ForceQuery || (len(s) > 0 && s[len(s)-1] == '#') {
		return "", errors.New("une origine n'a ni chemin, ni requête, ni fragment")
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
