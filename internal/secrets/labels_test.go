package secrets

// Tests of the label rule (spec v0.2 §5.2, Q60), written before the code:
// a value leaves the agent only under com.docker.compose. or
// org.opencontainers.image., and only if §7 finds nothing to mask in it.
// Every other label shows its key alone, without any reveal.

import (
	"encoding/json"
	"strings"
	"testing"
)

// Witnesses: none may appear in what Labels returns.
var labelWitnesses = []string{"s3cr3t", "$apr1$", "IgXLP6ew"}

func TestLabels(t *testing.T) {
	in := map[string]string{
		// §7 alone would let it through: neither "users" nor "basicauth" is a
		// word of §7, and the value has no "://", "=" nor "Bearer".
		"traefik.http.middlewares.auth.basicauth.users": "admin:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/",
		"com.docker.compose.project":                    "shop",
		"com.docker.compose.x":                          "postgres://u:s3cr3t@db/app", // whitelisted key, §7 still applies
		"com.docker.compose.password":                   "s3cr3t",                     // a secret name under the whitelist
		"org.opencontainers.image.source":               "https://github.com/o/r",
		"backup.dsn":                                    "s3cr3t",
		"com.example.webhook":                           "https://hooks.example/s3cr3t",
		// Look-alikes of the whitelist: masked.
		"com.docker.composer.x": "s3cr3t",
		"xcom.docker.compose.y": "s3cr3t",
		"com.docker.compose":    "s3cr3t",
	}
	got := Labels(in)
	if len(got) != len(in) {
		t.Fatalf("%d labels, %d attendus : %+v", len(got), len(in), got)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Key >= got[i].Key {
			t.Errorf("ordre par clé rompu : %q puis %q", got[i-1].Key, got[i].Key)
		}
	}
	clear := map[string]string{
		"com.docker.compose.project":      "shop",
		"org.opencontainers.image.source": "https://github.com/o/r",
	}
	for _, l := range got {
		if want, ok := clear[l.Key]; ok {
			if l.Masked || l.Value == nil || *l.Value != want {
				t.Errorf("%s : %+v, en clair %q attendu", l.Key, l, want)
			}
			continue
		}
		if !l.Masked || l.Value != nil {
			t.Errorf("FUITE : %s non masqué : %+v", l.Key, l)
		}
	}
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range labelWitnesses {
		if strings.Contains(string(b), w) {
			t.Errorf("FUITE : %s dans %s", w, b)
		}
	}
	// JSON form of spec v0.2 §6: [{key, value, masked}], value null when masked.
	if !strings.Contains(string(b), `{"key":"backup.dsn","value":null,"masked":true}`) {
		t.Errorf("forme JSON : %s", b)
	}
}

func TestLabelsEmpty(t *testing.T) {
	if got := Labels(nil); got == nil || len(got) != 0 {
		t.Errorf("Labels(nil) = %#v, liste vide attendue (JSON [])", got)
	}
}

// A long value under the whitelist is masked whole, as in §7 (maxLen).
func TestLabelsLongValueMasked(t *testing.T) {
	got := Labels(map[string]string{"org.opencontainers.image.description": strings.Repeat("a", maxLen+1)})
	if len(got) != 1 || !got[0].Masked || got[0].Value != nil {
		t.Errorf("valeur trop longue montrée : %+v", got)
	}
}
