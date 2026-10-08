package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateToken(t *testing.T) {
	a, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := GenerateToken()
	if len(a) != 43 || a == b || strings.ContainsAny(a, "+/=") {
		t.Fatalf("want 43-char unpadded base64url, distinct: %q %q", a, b)
	}
}

func TestEnsureTokenCreatesOnce(t *testing.T) {
	dir := t.TempDir()
	plain, created, err := EnsureToken(dir)
	if err != nil || !created || plain == "" {
		t.Fatalf("first call must create: %q %v %v", plain, created, err)
	}
	p := filepath.Join(dir, "token.sha256")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(plain))
	if strings.TrimSpace(string(raw)) != hex.EncodeToString(sum[:]) {
		t.Fatal("file must hold the hex SHA-256 of the token, never the token")
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", st.Mode().Perm())
	}
	plain2, created2, err := EnsureToken(dir)
	if err != nil || created2 || plain2 != "" {
		t.Fatalf("second call must not recreate nor reveal: %q %v %v", plain2, created2, err)
	}
}

func TestVerifier(t *testing.T) {
	dir := t.TempDir()
	plain, _, err := EnsureToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	v, err := LoadVerifier(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Check(plain) {
		t.Fatal("the right token must pass")
	}
	for _, bad := range []string{"", "x", plain + "x", strings.ToUpper(plain)} {
		if v.Check(bad) {
			t.Fatalf("%q must fail", bad)
		}
	}
}

func TestResetInvalidatesOldToken(t *testing.T) {
	dir := t.TempDir()
	old, _, _ := EnsureToken(dir)
	fresh, err := ResetToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := LoadVerifier(dir)
	if v.Check(old) || !v.Check(fresh) {
		t.Fatal("reset must replace the token")
	}
}

func TestLoadVerifierRejectsCorruptFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "token.sha256"), []byte("pas-hex"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadVerifier(dir); err == nil {
		t.Fatal("corrupt file must fail")
	}
}
