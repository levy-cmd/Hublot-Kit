// Package auth holds the access token, sessions and the login rate limiter.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const tokenFile = "token.sha256"

// GenerateToken returns 32 random bytes as unpadded base64url (43 characters).
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func digest(token string) []byte {
	s := sha256.Sum256([]byte(token))
	return s[:]
}

// writeDigestTemp creates a temporary file in dataDir with the hex digest.
// It returns the temporary file path; the caller must remove it.
// Returns an error if creation, chmod, write, or sync fails.
func writeDigestTemp(dataDir string, tok string) (string, error) {
	tmp, err := os.CreateTemp(dataDir, ".token-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", err
	}
	if _, err := tmp.WriteString(hex.EncodeToString(digest(tok)) + "\n"); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return "", err
	}
	return tmpPath, nil
}

// EnsureToken creates the token on first launch using file-locking semantics.
// The plain token is returned only when it was just created; it is never stored.
// Exclusive creation is enforced via os.Link; concurrent launches will have at most one succeed.
func EnsureToken(dataDir string) (plain string, created bool, err error) {
	_, err = os.Stat(filepath.Join(dataDir, tokenFile))
	if err == nil {
		return "", false, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", false, err
	}
	tok, err := GenerateToken()
	if err != nil {
		return "", false, err
	}
	tmpPath, err := writeDigestTemp(dataDir, tok)
	if err != nil {
		return "", false, err
	}
	defer os.Remove(tmpPath)
	final := filepath.Join(dataDir, tokenFile)
	err = os.Link(tmpPath, final)
	if errors.Is(err, fs.ErrExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return tok, true, nil
}

// ResetToken generates a new token and replaces the stored digest atomically.
// It fsync's the dataDir to ensure durability on filesystems that require it.
func ResetToken(dataDir string) (string, error) {
	tok, err := GenerateToken()
	if err != nil {
		return "", err
	}
	tmpPath, err := writeDigestTemp(dataDir, tok)
	if err != nil {
		return "", err
	}
	final := filepath.Join(dataDir, tokenFile)
	if err := os.Rename(tmpPath, final); err != nil {
		os.Remove(tmpPath)
		return "", err
	}
	// Best-effort fsync of dataDir for durability.
	d, err := os.Open(dataDir)
	if err == nil {
		d.Sync()
		d.Close()
	}
	return tok, nil
}

// Verifier holds the SHA-256 digest of the access token for constant-time verification.
type Verifier struct{ digest []byte }

// LoadVerifier reads the stored SHA-256 digest from dataDir/token.sha256.
// It returns an error if the file is missing, unreadable, or corrupted.
func LoadVerifier(dataDir string) (*Verifier, error) {
	raw, err := os.ReadFile(filepath.Join(dataDir, tokenFile))
	if err != nil {
		return nil, err
	}
	d, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(d) != sha256.Size {
		return nil, fmt.Errorf("%s corrompu : relancez `hublot reset-token`", tokenFile)
	}
	return &Verifier{digest: d}, nil
}

// Check compares digests in constant time.
func (v *Verifier) Check(candidate string) bool {
	return subtle.ConstantTimeCompare(digest(candidate), v.digest) == 1
}
