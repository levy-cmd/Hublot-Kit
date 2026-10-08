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

// EnsureToken creates the token on first launch. The plain token is returned
// only when it was just created; it is never stored.
func EnsureToken(dataDir string) (plain string, created bool, err error) {
	_, err = os.Stat(filepath.Join(dataDir, tokenFile))
	if err == nil {
		return "", false, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", false, err
	}
	plain, err = ResetToken(dataDir)
	return plain, err == nil, err
}

// ResetToken generates a new token and replaces the stored digest atomically.
func ResetToken(dataDir string) (string, error) {
	tok, err := GenerateToken()
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dataDir, ".token-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return "", err
	}
	if _, err := tmp.WriteString(hex.EncodeToString(digest(tok)) + "\n"); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dataDir, tokenFile)); err != nil {
		return "", err
	}
	return tok, nil
}

type Verifier struct{ digest []byte }

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
