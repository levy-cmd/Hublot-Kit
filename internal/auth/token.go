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
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const tokenFile = "token.sha256"

// linkFile is a package-level variable for os.Link, allowing tests to override it.
var linkFile = os.Link

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
// CreateTemp opens a fresh random name with O_CREATE|O_EXCL, which never
// follows a symbolic link.
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

// syncDir performs a best-effort fsync of the directory.
// Errors from Sync are ignored (not all filesystems support it).
func syncDir(dir string) {
	d, err := os.OpenFile(dir, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err == nil {
		d.Sync()
		d.Close()
	}
}

// EnsureToken creates the token on first launch using file-locking semantics.
// The plain token is returned only when it was just created; it is never stored.
// Exclusive creation is enforced via os.Link (or fallback exclusive open on unsupported filesystems).
// Concurrent launches will have at most one succeed.
// A token path that already exists, even as a symbolic link, is never followed
// nor replaced here: LoadVerifier then refuses the link.
func EnsureToken(dataDir string) (plain string, created bool, err error) {
	_, err = os.Lstat(filepath.Join(dataDir, tokenFile))
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
	err = linkFile(tmpPath, final)
	if errors.Is(err, fs.ErrExist) {
		return "", false, nil
	}
	if err != nil {
		// Link failed with non-ErrExist error (e.g., EPERM, EXDEV on filesystems without hard links).
		// Fall back to exclusive direct create.
		f, err := os.OpenFile(final, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
		if errors.Is(err, fs.ErrExist) {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		if _, err := f.WriteString(hex.EncodeToString(digest(tok)) + "\n"); err != nil {
			f.Close()
			os.Remove(final)
			return "", false, err
		}
		if err := f.Sync(); err != nil {
			f.Close()
			os.Remove(final)
			return "", false, err
		}
		if err := f.Close(); err != nil {
			os.Remove(final)
			return "", false, err
		}
		syncDir(dataDir)
		return tok, true, nil
	}
	syncDir(dataDir)
	return tok, true, nil
}

// ResetToken generates a new token and replaces the stored digest atomically.
// It performs a best-effort fsync of the dataDir for durability.
// rename(2) replaces a symbolic link at the token path without following it.
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
	syncDir(dataDir)
	return tok, nil
}

// Verifier holds the SHA-256 digest of the access token for constant-time verification.
type Verifier struct{ digest []byte }

// LoadVerifier reads the stored SHA-256 digest from dataDir/token.sha256.
// It returns an error if the file is missing, unreadable, corrupted, or a
// symbolic link (O_NOFOLLOW: a link could point to a digest someone else chose).
func LoadVerifier(dataDir string) (*Verifier, error) {
	f, err := os.OpenFile(filepath.Join(dataDir, tokenFile), os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, syscall.ELOOP) {
		return nil, fmt.Errorf("%s is a symbolic link: refused; remove it, then run `hublot reset-token` again", tokenFile)
	}
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		return nil, err
	}
	d, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(d) != sha256.Size {
		return nil, fmt.Errorf("%s is corrupt: run `hublot reset-token` again", tokenFile)
	}
	return &Verifier{digest: d}, nil
}

// Check compares digests in constant time.
func (v *Verifier) Check(candidate string) bool {
	return subtle.ConstantTimeCompare(digest(candidate), v.digest) == 1
}
