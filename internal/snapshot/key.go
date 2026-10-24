package snapshot

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// The key of the MACs (spec v0.2 §5.7, layer 1): 32 bytes of crypto/rand in
// <data-dir>/snapshot.key, 0600. Every access is relative to the descriptor
// of the data directory, never through a path built from it.

const (
	keyName = "snapshot.key"
	keyLen  = 32
)

// beforeKeyPublish runs between the fsync of a new key and its publication
// (linkat or renameat): a test makes it fail.
var beforeKeyPublish = func() error { return nil }

// KeyID names a key without revealing it: the first 8 bytes of
// HMAC(key, "hublot-snapshot-key-id"), in hex.
func KeyID(key []byte) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte("hublot-snapshot-key-id"))
	return hex.EncodeToString(m.Sum(nil)[:8])
}

// LoadOrCreateKey reads snapshot.key in dataDir, or creates it. Creation is
// atomic: a temporary file (O_CREAT|O_EXCL|O_NOFOLLOW, 0600), fsync, then
// linkat to snapshot.key, which fails if the name exists: of two concurrent
// creations one wins, the other reads the winner. No half-written key is
// ever visible.
func LoadOrCreateKey(dataDir *os.File) (key []byte, keyID string, err error) {
	dfd := int(dataDir.Fd())
	key, err = readKey(dfd)
	if !errors.Is(err, unix.ENOENT) {
		if err != nil {
			return nil, "", err
		}
		return key, KeyID(key), nil
	}
	tmp, key, err := writeTempKey(dfd)
	if err != nil {
		return nil, "", err
	}
	defer unix.Unlinkat(dfd, tmp, 0)
	if err := beforeKeyPublish(); err != nil {
		return nil, "", fmt.Errorf("snapshot key: %w", err)
	}
	switch err := unix.Linkat(dfd, tmp, dfd, keyName, 0); {
	case errors.Is(err, unix.EEXIST): // another creation won: its key is the one
		if key, err = readKey(dfd); err != nil {
			return nil, "", err
		}
	case err != nil:
		return nil, "", fmt.Errorf("snapshot key: linkat: %w", err)
	}
	unix.Fsync(dfd)
	return key, KeyID(key), nil
}

// RotateKey writes a new key the same way, then publishes it by renameat
// over snapshot.key (the agent is stopped, like reset-token). Snapshots of
// the old key keep its keyId: compared with the new ones, by their
// metadata only.
func RotateKey(dataDir *os.File) (keyID string, err error) {
	dfd := int(dataDir.Fd())
	tmp, key, err := writeTempKey(dfd)
	if err != nil {
		return "", err
	}
	if err := beforeKeyPublish(); err != nil {
		unix.Unlinkat(dfd, tmp, 0)
		return "", fmt.Errorf("snapshot key: %w", err)
	}
	if err := unix.Renameat(dfd, tmp, dfd, keyName); err != nil {
		unix.Unlinkat(dfd, tmp, 0)
		return "", fmt.Errorf("snapshot key: renameat: %w", err)
	}
	unix.Fsync(dfd)
	return KeyID(key), nil
}

// writeTempKey writes 32 random bytes in a new temporary file of dfd, 0600,
// synced; it returns its name.
func writeTempKey(dfd int) (string, []byte, error) {
	key := make([]byte, keyLen)
	rand.Read(key) // never fails (crypto/rand, Go 1.24+)
	var r [8]byte
	rand.Read(r[:])
	tmp := keyName + ".tmp-" + hex.EncodeToString(r[:])
	fd, err := unix.Openat(dfd, tmp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return "", nil, fmt.Errorf("snapshot key: %w", err)
	}
	f := os.NewFile(uintptr(fd), tmp)
	_, err = f.Write(key)
	if err == nil {
		err = f.Chmod(0o600) // the umask may have removed owner bits
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		unix.Unlinkat(dfd, tmp, 0)
		return "", nil, fmt.Errorf("snapshot key: %w", err)
	}
	return tmp, key, nil
}

// readKey opens snapshot.key without following a link and checks its
// descriptor: a regular file of the agent's user, no right for group or
// others, exactly 32 bytes (read to the end). unix.ENOENT: no key yet.
func readKey(dfd int) ([]byte, error) {
	fd, err := unix.Openat(dfd, keyName, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("%s refused: %w (a symbolic link?)", keyName, err)
	}
	f := os.NewFile(uintptr(fd), keyName)
	defer f.Close()
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return nil, fmt.Errorf("%s: %w", keyName, err)
	}
	switch {
	case st.Mode&unix.S_IFMT != unix.S_IFREG:
		return nil, fmt.Errorf("%s refused: not a regular file", keyName)
	case int(st.Uid) != os.Geteuid():
		return nil, fmt.Errorf("%s refused: owned by uid %d, not by the agent (uid %d)", keyName, st.Uid, os.Geteuid())
	case st.Mode&0o077 != 0:
		return nil, fmt.Errorf("%s refused: mode %#o, readable or writable by group or others", keyName, st.Mode&0o777)
	}
	b, err := io.ReadAll(io.LimitReader(f, keyLen+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", keyName, err)
	}
	if len(b) != keyLen {
		return nil, fmt.Errorf("%s refused: %d bytes, expected exactly %d", keyName, len(b), keyLen)
	}
	return b, nil
}
