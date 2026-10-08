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
		{"https://hublot.example.fr/app", "", false},
		{"ftp://hublot.example.fr", "", false},
		{"hublot.example.fr", "", false},
	}
	for _, tc := range cases {
		c, err := Load([]string{"--public-origin", tc.in}, env(nil))
		if tc.ok && (err != nil || c.PublicOrigin != tc.want) {
			t.Errorf("%q: got %q, %v; want %q", tc.in, c.PublicOrigin, err, tc.want)
		}
		if !tc.ok && err == nil {
			t.Errorf("%q: want error", tc.in)
		}
	}
}

func TestUnknownFlag(t *testing.T) {
	if _, err := Load([]string{"--nope"}, env(nil)); err == nil {
		t.Fatal("unknown flag must fail")
	}
}
