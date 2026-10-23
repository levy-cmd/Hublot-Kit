package secrets

// Tests of the masking of spec §7, written before the code (CLAUDE.md:
// security tests first). The rule is server-side: no secret leaves the
// agent unmasked unless revealed, one variable at a time, and audited.

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSecretName(t *testing.T) {
	for _, n := range []string{
		"PASSWORD", "db_password", "MYSQL_ROOT_PASSWORD", "PASSWD", "PGPASS", "Passphrase",
		"GITHUB_TOKEN", "api_key", "AWS_SECRET_ACCESS_KEY", "SECRET", "GOOGLE_APPLICATION_CREDENTIALS",
		"PRIVATE_KEY_PATH", "SENTRY_DSN", "dsn",
		"KEYBOARD", "COMPASS", // false positives the substring rule accepts (spec §7)
		// Fix round 1, Codex: format characters are dropped, then a name with a
		// non-ASCII letter is suspect (fail closed).
		"DB_PA\u200bSSWORD", "DB_P\u0410SSWORD", "\uff30\uff21\uff33\uff33",
	} {
		if !SecretName(n) {
			t.Errorf("%s : non masqué", n)
		}
	}
	for _, n := range []string{"PATH", "HOME", "LANG", "NODE_ENV", "PORT", "HOSTNAME", "TZ", "PS1", "USER", ""} {
		if SecretName(n) {
			t.Errorf("%s : masqué à tort", n)
		}
	}
}

// Q26: any user information in a URL ("://…@" before a blank), with or
// without a password: a token alone as user name is a secret too.
func TestURLCredentials(t *testing.T) {
	for _, v := range []string{
		"postgres://app:s3cr3t@db:5432/app", "redis://:s3cr3t@cache:6379/0", "https://ghp_abc@github.com/o/r.git",
		"amqp://u:p%40ss@mq", "x mysql://root:pw@db y", "HTTPS://U:P@H",
		"https://example.com/a@b", // a false positive the safe-side rule accepts
	} {
		if !URLCredentials(v) {
			t.Errorf("%q : non masqué", v)
		}
	}
	for _, v := range []string{"http://example.com:8080/path", "admin@example.com", "https:// @x", "plain", ""} {
		if URLCredentials(v) {
			t.Errorf("%q : masqué à tort", v)
		}
	}
}

func TestEnv(t *testing.T) {
	env := []string{"PATH=/usr/bin", "DB_PASSWORD=s3cr3t", "DATABASE_URL=postgres://u:pw@db/app", "EQ=a=b", "NOVAL", "Api_Key="}
	got := Env(env)
	str := func(s string) *string { return &s }
	want := []Var{
		{Name: "PATH", Value: str("/usr/bin")},
		{Name: "DB_PASSWORD", Masked: true},
		{Name: "DATABASE_URL", Masked: true},
		{Name: "EQ", Value: str("a=b")},
		{Name: "NOVAL", Value: str("")},
		{Name: "Api_Key", Masked: true}, // empty, masked all the same: the name decides
	}
	if len(got) != len(want) {
		t.Fatalf("%d variables, attendu %d", len(got), len(want))
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Name != w.Name || g.Masked != w.Masked || (g.Value == nil) != (w.Value == nil) || (g.Value != nil && *g.Value != *w.Value) {
			t.Errorf("%d : %+v, attendu %+v", i, g, w)
		}
	}
	b, _ := json.Marshal(got[1])
	if string(b) != `{"name":"DB_PASSWORD","value":null,"masked":true}` {
		t.Errorf("JSON d'une valeur masquée : %s (spec §7 : null avec masked: true)", b)
	}
}

// The whole JSON of Env never carries a secret value.
func TestEnvNeverLeaks(t *testing.T) {
	secrets := []string{"s3cr3t-1", "s3cr3t-2", "s3cr3t-3", "s3cr3t-4", "s3cr3t-5", "s3cr3t-6", "s3cr3t-7", "s3cr3t-8", "s3cr3t-9", "s3cr3t-A", "s3cr3t-B"}
	env := []string{"MYSQL_PASSWORD=" + secrets[0], "GH_TOKEN=" + secrets[1], "URL=https://bot:" + secrets[2] + "@x.io", "ssh_private_key=" + secrets[3],
		// Q36: a secret assignment inside the value of a neutral name.
		"ConnectionStrings__Default=Server=db;User Id=sa;Password=" + secrets[4] + ";",
		"SPRING_DATASOURCE_URL=jdbc:postgresql://db/x?user=u&password=" + secrets[5],
		"JAVA_OPTS=-Xmx512m -Djavax.net.ssl.keyStorePassword=" + secrets[6],
		// Review 2: the three rules of the command (redis-stack's REDIS_ARGS),
		// ODBC's Pwd=, an escaped "\=", a percent-encoded name.
		"REDIS_ARGS=--requirepass " + secrets[7],
		"ODBC=Driver={x};Server=db;Uid=u;Pwd=" + secrets[8] + ";",
		`JAVA_TOOL_OPTIONS=-DDB_PASSWORD\=` + secrets[9],
		"CALLBACK=https://x.io/cb?pass%77ord=" + secrets[10],
	}
	b, _ := json.Marshal(Env(env))
	for _, s := range secrets {
		if strings.Contains(string(b), s) {
			t.Errorf("FUITE : %s dans %s", s, b)
		}
	}
	// Controls: neutral values stay shown.
	for _, kv := range []string{"JAVA_OPTS=-Xmx512m -Dfile.encoding=UTF-8", "NODE_OPTIONS=--max-old-space-size=4096", "PATH=/usr/local/keycloak/bin:/usr/bin", "PWD=/srv/app"} {
		if Env([]string{kv})[0].Masked {
			t.Errorf("%s : masqué à tort", kv)
		}
	}
}

func TestLookup(t *testing.T) {
	env := []string{"A=1", "DB_PASSWORD=s3cr3t", "A=2", "NOVAL"}
	if v, ok := Lookup(env, "DB_PASSWORD"); !ok || v != "s3cr3t" {
		t.Errorf("DB_PASSWORD : %q %v", v, ok)
	}
	if v, ok := Lookup(env, "A"); !ok || v != "2" {
		t.Errorf("doublon : %q %v, attendu la dernière valeur", v, ok)
	}
	if v, ok := Lookup(env, "NOVAL"); !ok || v != "" {
		t.Errorf("sans '=' : %q %v", v, ok)
	}
	if _, ok := Lookup(env, "db_password"); ok {
		t.Error("le nom est exact, casse comprise")
	}
	if _, ok := Lookup(env, ""); ok {
		t.Error("nom vide")
	}
}

// Q27: the command and entrypoint carry secrets too (redis-server
// --requirepass, mysqld --password=, sh -c "PGPASSWORD=… psql URL").
func TestArgs(t *testing.T) {
	cases := []struct {
		in, want []string
		masked   bool
	}{
		{[]string{"redis-server", "--requirepass", "s3cr3t"}, []string{"redis-server", "--requirepass", Mask}, true},
		{[]string{"mysqld", "--password=s3cr3t", "--port=3306"}, []string{"mysqld", "--password=" + Mask, "--port=3306"}, true},
		{[]string{"sh", "-c", "PGPASSWORD=s3cr3t psql postgres://u:pw@db/x && echo ok"},
			[]string{"sh", "-c", "PGPASSWORD=" + Mask + " psql postgres://" + Mask + "@db/x && echo ok"}, true},
		{[]string{"sh", "-c", "redis-server --requirepass s3cr3t --port 6379 && x"},
			[]string{"sh", "-c", "redis-server --requirepass " + Mask + " --port 6379 && x"}, true},
		// Review I1 and Codex: quotes, a value with blanks or a quote inside, an
		// assignment inside a value, "--x=" with its value in the next argument.
		{[]string{"sh", "-c", `PGPASSWORD="s3cr3t" psql`}, []string{"sh", "-c", "PGPASSWORD=" + Mask + " psql"}, true},
		{[]string{"sh", "-c", `PGPASSWORD='s3cr3t' psql`}, []string{"sh", "-c", "PGPASSWORD=" + Mask + " psql"}, true},
		{[]string{"sh", "-c", `redis-server --requirepass "s3cr3t" --port 6379`},
			[]string{"sh", "-c", "redis-server --requirepass " + Mask + " --port 6379"}, true},
		{[]string{"sh", "-c", `x --password=s3"cr3t y`}, []string{"sh", "-c", "x --password=" + Mask}, true},
		{[]string{"app", "--env=DB_PASSWORD=s3cr3t"}, []string{"app", "--env=DB_PASSWORD=" + Mask}, true},
		{[]string{"app", "--password=s3 cr3t"}, []string{"app", "--password=" + Mask}, true},
		{[]string{"app", `--password=s3"cr3t`}, []string{"app", "--password=" + Mask}, true},
		{[]string{"app", "--password=", "s3cr3t"}, []string{"app", "--password=", Mask}, true},
		// Fail closed: a word of §7 left with nothing masked after it in the
		// argument masks the whole argument (false positives accepted).
		{[]string{"sh", "-c", "mysql -ps3cr3t --password"}, []string{"sh", "-c", Mask}, true},
		{[]string{"sh", "-c", "x --api-key -v"}, []string{"sh", "-c", Mask}, true},
		// Review 2, Codex: the link is local. The value of this very word must be
		// masked; a mask elsewhere in the line (the URL) does not cover it.
		{[]string{"sh", "-c", "echo --password=$SECRET postgres://u:p@db"},
			[]string{"sh", "-c", "echo --password=" + Mask + " postgres://" + Mask + "@db"}, true},
		{[]string{"sh", "-c", "echo --password=$(cat /f) postgres://u:p@db"}, []string{"sh", "-c", Mask}, true},
		// Review 2, Minor 1: "-c" is a shell line only after a shell.
		{[]string{"postgres", "-c", "ssl_passphrase_command=echo s3cr3t"}, []string{"postgres", "-c", "ssl_passphrase_command=" + Mask}, true},
		{[]string{"/bin/bash", "-lc", "redis-server --requirepass s3cr3t"}, []string{"/bin/bash", "-lc", "redis-server --requirepass " + Mask}, true},
		// Review 2, Minor 2 and 5, Codex Minor: a value after "--x=" that starts
		// with "-"; left boundaries ":" "{" "/"; an escaped "\=".
		{[]string{"app", "--api-key=", "-s3cr3t"}, []string{"app", "--api-key=", Mask}, true},
		{[]string{"sh", "-c", "x:password=s3cr3t {token=s3cr3t} /x/secret=s3cr3t"},
			[]string{"sh", "-c", "x:password=" + Mask + " {token=" + Mask + " /x/secret=" + Mask}, true},
		{[]string{"java", `-DDB_PASSWORD\=s3cr3t`, "-jar", "app.jar"}, []string{"java", `-DDB_PASSWORD\=` + Mask, "-jar", "app.jar"}, true},
		// Review 2, I1: a word of §7 in a path, or in a plain word, is not a
		// secret; these stay readable (controls of the fail-closed rule).
		{[]string{"/opt/keycloak/bin/kc.sh", "start"}, []string{"/opt/keycloak/bin/kc.sh", "start"}, false},
		{[]string{"sh", "-c", "cat /run/secrets/db_password > /tmp/p && exec app"}, []string{"sh", "-c", "cat /run/secrets/db_password > /tmp/p && exec app"}, false},
		{[]string{"sh", "-c", "ssh-keyscan github.com >> ~/.ssh/known_hosts && exec app"}, []string{"sh", "-c", "ssh-keyscan github.com >> ~/.ssh/known_hosts && exec app"}, false},
		{[]string{"sh", "-c", "echo 'reset your password' && exec app"}, []string{"sh", "-c", "echo 'reset your password' && exec app"}, false},
		{[]string{"keydb-server", "/etc/keydb.conf"}, []string{"keydb-server", "/etc/keydb.conf"}, false},
		{[]string{"app", "--cert=/etc/ssl/private/x.pem"}, []string{"app", "--cert=/etc/ssl/private/x.pem"}, false},
		{[]string{"passwd", "root"}, []string{"passwd", "root"}, false},
		{[]string{"app", "-api-key", "-v"}, []string{"app", "-api-key", "-v"}, false}, // a flag follows: no value
		{[]string{"app", "--token"}, []string{"app", "--token"}, false},
		{[]string{"nginx", "-g", "daemon off;"}, []string{"nginx", "-g", "daemon off;"}, false},
		{[]string{"env", "API_TOKEN=abc", "run"}, []string{"env", "API_TOKEN=" + Mask, "run"}, true},
		{nil, nil, false},
		// Fix round 1, I2: the mask must follow THE word's own separator; the mask
		// of another key in the same word does not cover it.
		{[]string{"app", "--opts=password:s3cr3t,token=abc"}, []string{"app", Mask}, true},
		{[]string{"sh", "-c", "app password:s3cr3t,token=abc"}, []string{"sh", "-c", Mask}, true},
		{[]string{"app", "-Dauth=password:s3cr3t;key=abc"}, []string{"app", Mask}, true},
		// Fix round 1, I3 (Q27): Pwd is a secret key in an assignment too.
		{[]string{"app", "--pwd=s3cr3t"}, []string{"app", "--pwd=" + Mask}, true},
		{[]string{"app", "Pwd=s3cr3t"}, []string{"app", "Pwd=" + Mask}, true},
		// Fix round 1, Minor: JSON, an escaped blank, a bearer token.
		{[]string{"curl", "-d", `{"password":"s3cr3t"}`, "http://x"}, []string{"curl", "-d", Mask, "http://x"}, true},
		{[]string{"sh", "-c", `curl -d '{"user":"a","password":"s3cr3t"}' http://x`}, []string{"sh", "-c", Mask}, true},
		{[]string{"sh", "-c", `PGPASSWORD=s3\ cr3t psql`}, []string{"sh", "-c", "PGPASSWORD=" + Mask + " psql"}, true},
		{[]string{"sh", "-c", `app --password s3\ cr3t --x`}, []string{"sh", "-c", "app --password " + Mask + " --x"}, true},
		{[]string{"curl", "-H", "Authorization: Bearer s3cr3t", "http://x"}, []string{"curl", "-H", "Authorization: Bearer " + Mask, "http://x"}, true},
		{[]string{"sh", "-c", "curl -H 'Authorization: Basic s3cr3t' http://x"}, []string{"sh", "-c", "curl -H 'Authorization: Basic " + Mask + "' http://x"}, true},
		// Fix round 2, Minor: JSON escaped inside a shell line (\"password\":\"x\").
		{[]string{"sh", "-c", `curl -d "{\"password\":\"s3cr3t\"}" http://x`}, []string{"sh", "-c", Mask}, true},
		{[]string{"curl", "-d", `{\"password\":\"s3cr3t\"}`, "http://x"}, []string{"curl", "-d", Mask, "http://x"}, true},
		{[]string{"sh", "-c", `curl -d "{\"password\": \"s3cr3t\", \"a\": 1}" http://x`}, []string{"sh", "-c", Mask}, true},
		// Over the cap: masked whole (fail closed).
		{[]string{"app", strings.Repeat("a", maxLen+1)}, []string{"app", Mask}, true},
	}
	for _, k := range cases {
		got, masked := Args(k.in)
		if !slices.Equal(got, k.want) || masked != k.masked {
			t.Errorf("%q : %q %v, attendu %q %v", k.in, got, masked, k.want, k.masked)
		}
		for _, a := range got {
			if strings.Contains(a, "cr3t") || strings.Contains(a, ":pw@") || strings.Contains(a, "=abc") || strings.Contains(a, "(cat") {
				t.Errorf("FUITE : %q", a)
			}
		}
	}
	in := []string{"x", "--password=s3cr3t"}
	Args(in)
	if in[1] != "--password=s3cr3t" {
		t.Error("Args ne doit pas modifier son entrée")
	}
}

// Review 2, I1: Docker's messages stay readable when a word of §7 is only a
// word or a path; a secret value in them is masked.
func TestLine(t *testing.T) {
	for _, m := range []string{`exec: "/opt/keycloak/bin/kc.sh": permission denied`, "unable to read key file", ""} {
		if got := Line(m); got != m {
			t.Errorf("%q : %q, attendu inchangé", m, got)
		}
	}
	if got := Line(`exec: "--password=s3cr3t": unknown`); strings.Contains(got, "cr3t") {
		t.Errorf("FUITE : %q", got)
	}
	// Fix round 1: JSON, and the cap.
	if got := Line(`{"password":"s3cr3t"}`); got != Mask {
		t.Errorf("JSON : %q", got)
	}
	// Fix round 2, Minor: escaped JSON.
	for _, m := range []string{`{\"password\":\"s3cr3t\"}`, `{\"token\" : \"s3cr3t\"}`, `{\"key\":\"s3cr3t\"}`} {
		if got := Line(m); got != Mask {
			t.Errorf("JSON échappé %q : %q", m, got)
		}
	}
	if got := Line(strings.Repeat("a", maxLen+1)); got != Mask {
		t.Errorf("au-dessus du plafond : %.20q", got)
	}
}

func TestEnvUnicodeAndBearer(t *testing.T) {
	for _, kv := range []string{"DB_PA\u200bSSWORD=s3cr3t", "DB_P\u0410SSWORD=s3cr3t", "HEALTH=curl -H 'Authorization: Bearer s3cr3t'"} {
		if v := Env([]string{kv})[0]; !v.Masked || v.Value != nil {
			t.Errorf("%q : non masqué", kv)
		}
	}
}

// within fails the test if f takes longer than d (f runs in a goroutine so
// that a quadratic cost fails the test instead of hanging it).
func within(t *testing.T, name string, d time.Duration, f func()) {
	t.Helper()
	done := make(chan struct{})
	start := time.Now()
	go func() { f(); close(done) }()
	select {
	case <-done:
		if el := time.Since(start); el > d {
			t.Errorf("%s : %v, plafond %v", name, el, d)
		}
	case <-time.After(5 * time.Second):
		t.Errorf("%s : plus de 5 s, coût non linéaire", name)
	}
}

// Fix round 1, I1: the cost of the masking is linear, and a value over the
// cap is masked whole. The costs are relative (review of tranche 6 of v0.2:
// absolute ceilings of 100 and 250 ms failed under load): each call is
// compared with Line on a banal input of maxLen bytes, measured in the same
// conditions. A quadratic cost on such an input is hundreds of times that;
// linear work stays within a small factor, whatever the machine.
func TestCost(t *testing.T) {
	pad := func(s, fill string) string { return s + strings.Repeat(fill, (maxLen-len(s))/len(fill)) }
	plain := pad("", "abc ")
	big := strings.Repeat("a=", 1<<16) // 128 KiB
	if !Masked("X", big) {
		t.Error("au-dessus du plafond : non masqué")
	}
	if _, m := Args([]string{"app", "--x=" + big}); !m {
		t.Error("au-dessus du plafond : non masqué")
	}
	for _, c := range []struct {
		name string
		f    func()
	}{
		{"Masked 128 Kio", func() { Masked("X", big) }},
		{"Line 128 Kio", func() { Line(strings.Repeat("key", 43690)) }},
		{"Args 128 Kio", func() { Args([]string{"app", "--x=" + big}) }},
		// Just under the cap: the work itself must be linear.
		{"Masked a= sous le plafond", func() { Masked("X", strings.Repeat("a=", maxLen/2-1)) }},
		{"Masked k=v, sous le plafond", func() { Masked("X", strings.Repeat("k=v,", maxLen/4-1)) }},
		{"Line key sous le plafond", func() { Line(strings.Repeat("key", maxLen/3-1)) }},
		{"Args sous le plafond", func() { Args([]string{"app", "--attrs=" + strings.Repeat("service.name=x,", 500)}) }},
	} {
		within(t, c.name, 5*time.Second, c.f) // a cost far from linear fails here instead of hanging
		r := relCost(c.f, func() { Line(plain) })
		t.Logf("%s : rapport %.1f", c.name, r)
		if r > 20 {
			t.Errorf("%s : %.1f fois le coût de Line sur une entrée banale de %d octets, plafond 20", c.name, r, maxLen)
		}
	}

	// Fix round 2, I: many words of §7 that share one separator, then a long
	// tail; and a flag made of many words, then many blanks before a mask.
	// Each word must not rescan the tail. The cost is compared with a plain
	// input of the same size (ratio, not a duration: CI machines differ).
	sep := pad(":"+strings.Repeat("key", 700)+"=x;", ";")
	blanks := pad("-"+strings.Repeat("key", 700), " ")
	blanks = blanks[:maxLen-len(Mask)] + Mask
	if Line(sep) == sep || Line(blanks) != blanks || Line(plain) != plain {
		t.Error("entrées du test de coût : masquage inattendu")
	}
	for _, k := range []struct{ name, in string }{{"séparateur partagé", sep}, {"blancs avant le masque", blanks}} {
		r := costRatio(k.in, plain)
		t.Logf("Line %s : rapport %.1f", k.name, r)
		if r > 8 {
			t.Errorf("Line %s : %.1f fois le coût d'une entrée banale de même taille, plafond 8", k.name, r)
		}
	}
}

// costRatio: relCost of Line on s over Line on ref.
func costRatio(s, ref string) float64 {
	return relCost(func() { Line(s) }, func() { Line(ref) })
}

// relCost: the best time of f over the best time of ref, over a few runs of
// 16 calls each (the best time is the least noisy; both run alike under
// load or -race).
func relCost(f, ref func()) float64 {
	best := func(g func()) time.Duration {
		b := time.Duration(1 << 62)
		for range 5 {
			st := time.Now()
			for range 16 {
				g()
			}
			b = min(b, time.Since(st))
		}
		return max(b, time.Nanosecond)
	}
	return float64(best(f)) / float64(best(ref))
}
