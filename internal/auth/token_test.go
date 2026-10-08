package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
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
	if err != nil {
		t.Fatal(err)
	}
	if !created || plain == "" {
		t.Fatalf("first call must create: %q %v", plain, created)
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
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", st.Mode().Perm())
	}
	// Check no .token-* temp files remain
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".token-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
	plain2, created2, err := EnsureToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	if created2 || plain2 != "" {
		t.Fatalf("second call must not recreate nor reveal: %q %v", plain2, created2)
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
	old, _, err := EnsureToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := ResetToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Check no .token-* temp files remain
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".token-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
	v, err := LoadVerifier(dir)
	if err != nil {
		t.Fatal(err)
	}
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

func TestLoadVerifierRejectsWrongLength(t *testing.T) {
	dir := t.TempDir()
	// Valid hex but wrong length (not 32 bytes = 64 hex chars)
	if err := os.WriteFile(filepath.Join(dir, "token.sha256"), []byte("abcd"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadVerifier(dir); err == nil {
		t.Fatal("wrong-length digest must fail")
	}
}

func TestEnsureTokenLinkUnsupported(t *testing.T) {
	dir := t.TempDir()
	// Swap linkFile to simulate a filesystem that doesn't support hard links.
	oldLinkFile := linkFile
	linkFile = func(oldname, newname string) error {
		return syscall.EPERM
	}
	t.Cleanup(func() {
		linkFile = oldLinkFile
	})
	plain, created, err := EnsureToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !created || plain == "" {
		t.Fatalf("fallback create should succeed: %q %v", plain, created)
	}
	// Verify the token
	v, err := LoadVerifier(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Check(plain) {
		t.Fatal("verifier must accept the fallback-created token")
	}
	// Check no .token-* temp files remain
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".token-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestEnsureTokenConcurrent(t *testing.T) {
	dir := t.TempDir()
	const n = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]struct {
		plain   string
		created bool
		err     error
	}, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start // Wait for all goroutines to be ready
			p, c, e := EnsureToken(dir)
			results[idx].plain = p
			results[idx].created = c
			results[idx].err = e
		}(i)
	}
	close(start) // Signal all goroutines to proceed
	wg.Wait()
	createdCount := 0
	var token string
	for i, r := range results {
		if r.err != nil {
			t.Fatalf("goroutine %d: %v", i, r.err)
		}
		if r.created {
			createdCount++
			token = r.plain
			if token == "" {
				t.Fatal("created=true but token empty")
			}
		} else if r.plain != "" {
			t.Fatal("created=false but token non-empty")
		}
	}
	if createdCount != 1 {
		t.Fatalf("exactly one goroutine should have created; got %d", createdCount)
	}
	// Check no .token-* temp files remain
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".token-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
	v, err := LoadVerifier(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Check(token) {
		t.Fatal("verifier must accept the created token")
	}
}

// symlinkToken makes dir/token.sha256 a symbolic link to target.
func symlinkToken(t *testing.T, dir, target string) string {
	t.Helper()
	link := filepath.Join(dir, "token.sha256")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	return link
}

func TestLoadVerifierRefusesSymlink(t *testing.T) {
	// The link points to a valid digest the attacker knows the token of.
	elsewhere := t.TempDir()
	plain, _, err := EnsureToken(elsewhere)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	symlinkToken(t, dir, filepath.Join(elsewhere, "token.sha256"))
	if v, err := LoadVerifier(dir); err == nil {
		t.Fatalf("a symlinked token file must be refused (accepts the planted token: %v)", v.Check(plain))
	}
}

func TestEnsureTokenDoesNotFollowSymlink(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		if fallback {
			old := linkFile
			linkFile = func(string, string) error { return syscall.EPERM }
			t.Cleanup(func() { linkFile = old })
		}
		dir := t.TempDir()
		target := filepath.Join(t.TempDir(), "victim")
		symlinkToken(t, dir, target)
		plain, created, _ := EnsureToken(dir)
		if created || plain != "" {
			t.Fatalf("fallback=%v: no token may be created through a link", fallback)
		}
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Fatalf("fallback=%v: link target must not be created, lstat err: %v", fallback, err)
		}
	}
}

func TestResetTokenReplacesSymlinkWithoutFollowing(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(target, []byte("intact\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o644); err != nil {
		t.Fatal(err)
	}
	link := symlinkToken(t, dir, target)
	fresh, err := ResetToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(target)
	if err != nil || string(raw) != "intact\n" {
		t.Fatalf("link target modified: %q %v", raw, err)
	}
	if st, err := os.Stat(target); err != nil || st.Mode().Perm() != 0o644 {
		t.Fatalf("link target mode changed: %v %v", st, err)
	}
	st, err := os.Lstat(link)
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm() != 0o600 {
		t.Fatalf("token file must now be a regular 0600 file: %v %v", st, err)
	}
	v, err := LoadVerifier(dir)
	if err != nil || !v.Check(fresh) {
		t.Fatalf("reset token must verify: %v", err)
	}
}
