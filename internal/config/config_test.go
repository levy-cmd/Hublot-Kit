package config

import "testing"

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDefaults(t *testing.T) {
	c, err := Load(nil, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != "127.0.0.1:7878" || c.DataDir != "/var/lib/hublot-kit" || c.ReadOnly || c.TLS() || c.PublicOrigin != "" {
		t.Fatalf("bad defaults: %+v", c)
	}
}

func TestEnvThenFlagPrecedence(t *testing.T) {
	e := env(map[string]string{"HUBLOT_LISTEN": "0.0.0.0:9000", "HUBLOT_READ_ONLY": "true", "HUBLOT_DATA_DIR": "/env"})
	c, err := Load(nil, e)
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != "0.0.0.0:9000" || !c.ReadOnly || c.DataDir != "/env" {
		t.Fatalf("env not applied: %+v", c)
	}
	c, err = Load([]string{"--listen", "127.0.0.1:1234", "--data-dir", "/flag"}, e)
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != "127.0.0.1:1234" || c.DataDir != "/flag" {
		t.Fatalf("flag must win over env: %+v", c)
	}
}

// v0.1.2 (counter-audit, constat 2): connections at once per source address.
// Behind a reverse proxy every connection comes from the proxy: settable.
func TestMaxConnsPerIP(t *testing.T) {
	if c, err := Load(nil, env(nil)); err != nil || c.MaxConnsPerIP != 64 {
		t.Fatalf("défaut : %+v %v", c, err)
	}
	if c, err := Load(nil, env(map[string]string{"HUBLOT_MAX_CONNS_PER_IP": "200"})); err != nil || c.MaxConnsPerIP != 200 {
		t.Fatalf("variable : %+v %v", c, err)
	}
	if c, err := Load([]string{"--max-conns-per-ip", "300"}, env(map[string]string{"HUBLOT_MAX_CONNS_PER_IP": "200"})); err != nil || c.MaxConnsPerIP != 300 {
		t.Fatalf("option : %+v %v", c, err)
	}
	for _, bad := range []string{"0", "-1", "x", "513"} {
		if _, err := Load([]string{"--max-conns-per-ip", bad}, env(nil)); err == nil {
			t.Errorf("--max-conns-per-ip %s accepté", bad)
		}
		if _, err := Load(nil, env(map[string]string{"HUBLOT_MAX_CONNS_PER_IP": bad})); err == nil {
			t.Errorf("HUBLOT_MAX_CONNS_PER_IP=%s accepté", bad)
		}
	}
}

func TestInvalidBoolEnv(t *testing.T) {
	if _, err := Load(nil, env(map[string]string{"HUBLOT_READ_ONLY": "peut-être"})); err == nil {
		t.Fatal("want error for invalid HUBLOT_READ_ONLY")
	}
}

func TestTLSPair(t *testing.T) {
	if _, err := Load([]string{"--tls-cert", "c.pem"}, env(nil)); err == nil {
		t.Fatal("cert without key must fail")
	}
	c, err := Load([]string{"--tls-cert", "c.pem", "--tls-key", "k.pem"}, env(nil))
	if err != nil || !c.TLS() {
		t.Fatalf("pair must be accepted: %+v %v", c, err)
	}
}

func TestPublicOrigin(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"https://hublot.example.fr", "https://hublot.example.fr", true},
		{"https://Hublot.Example.fr/", "https://hublot.example.fr", true},
		{"http://localhost:7878", "http://localhost:7878", true},
		{"https://h.example:443", "https://h.example", true},
		{"http://h.example:80", "http://h.example", true},
		{"https://h.example:8443", "https://h.example:8443", true},
		{"http://[::1]:7878", "http://[::1]:7878", true},
		{"HTTPS://H.EXAMPLE/", "https://h.example", true},
		{"https://hublot.example.fr/app", "", false},
		{"ftp://hublot.example.fr", "", false},
		{"hublot.example.fr", "", false},
		{"https://u@h.example", "", false},
		{"http://[fe80::1%25eth0]:80", "", false},
		{"https://", "", false},
		{"https://h.example?", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			c, err := Load([]string{"--public-origin", tc.in}, env(nil))
			if tc.ok && (err != nil || c.PublicOrigin != tc.want) {
				t.Errorf("got %q, %v; want %q", c.PublicOrigin, err, tc.want)
			}
			if !tc.ok && err == nil {
				t.Errorf("want error")
			}
		})
	}
}

func TestUnknownFlag(t *testing.T) {
	if _, err := Load([]string{"--nope"}, env(nil)); err == nil {
		t.Fatal("unknown flag must fail")
	}
}

func TestFlagPrecedenceReadOnly(t *testing.T) {
	e := env(map[string]string{"HUBLOT_READ_ONLY": "true"})
	c, err := Load([]string{"--read-only=false"}, e)
	if err != nil {
		t.Fatal(err)
	}
	if c.ReadOnly {
		t.Fatalf("flag must override env: got ReadOnly=%v", c.ReadOnly)
	}
}

func TestEnvPublicOrigin(t *testing.T) {
	e := env(map[string]string{"HUBLOT_PUBLIC_ORIGIN": "https://hublot.example.fr"})
	c, err := Load(nil, e)
	if err != nil {
		t.Fatal(err)
	}
	if c.PublicOrigin != "https://hublot.example.fr" {
		t.Fatalf("env not applied: got %q", c.PublicOrigin)
	}
}

func TestPositionalArgs(t *testing.T) {
	if _, err := Load([]string{"foo"}, env(nil)); err == nil {
		t.Fatal("positional args must fail")
	}
}
