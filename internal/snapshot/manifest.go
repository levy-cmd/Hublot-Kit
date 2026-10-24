// Package snapshot takes, keeps and lists the snapshots of spec v0.2 §5.7:
// a manifest of the files of a container at one moment, with a keyed HMAC of
// each regular file, never its content. The key (key.go), the manifests on
// the disk (store.go) and the capture (capture.go) live here; the walk of a
// running container is fsview's (Walker).
package snapshot

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"time"
)

// Entry is one line of a manifest, in the order of the path components.
type Entry struct {
	Path string `json:"path"`
	// Type: file, dir, symlink, other, mount or inaccessible.
	Type string `json:"type"`
	Size int64  `json:"size"`
	// Mode: the 0o7777 bits (fsview.UnixPerm).
	Mode  uint32    `json:"mode"`
	UID   int       `json:"uid"`
	GID   int       `json:"gid"`
	MTime time.Time `json:"mtime"`
	// Link: the raw target of a symbolic link.
	Link string `json:"link,omitempty"`
	// MAC: HMAC-SHA256(key of the agent, content), hex, regular files only.
	// It never leaves the agent (spec v0.2 §5.7, layer 2).
	MAC string `json:"mac,omitempty"`
	// MACSkipped: too_large, budget, sparse or unreadable.
	MACSkipped string `json:"macSkipped,omitempty"`
}

// MACSkipped values.
const (
	SkipTooLarge   = "too_large"
	SkipBudget     = "budget"
	SkipSparse     = "sparse"
	SkipUnreadable = "unreadable"
)

// Header is the first line of a manifest. CreatedAt is when the capture
// started, FinishedAt when it ended (a photo of a running container is not
// atomic: a file may be seen before or after a change made meanwhile).
type Header struct {
	ID            string    `json:"id"`
	ContainerID   string    `json:"containerId"`
	ContainerName string    `json:"containerName"`
	Project       string    `json:"project,omitempty"`
	Image         string    `json:"image,omitempty"`
	ImageID       string    `json:"imageId,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	FinishedAt    time.Time `json:"finishedAt"`
	// State: running or stopped.
	State string `json:"state"`
	// Source: procfs, archive or index.
	Source string `json:"source"`
	// KeyID names the key of the MACs ("" for a photo of the metadata).
	KeyID       string `json:"keyId,omitempty"`
	Entries     int    `json:"entries"`
	HashedBytes int64  `json:"hashedBytes"`
	Partial     bool   `json:"partial"`
	Limits      Limits `json:"limits"`
}

// ContentComparable: the MACs of a and b were made with the same key, so
// equal MACs mean equal contents (spec v0.2 §5.7, rotation).
func ContentComparable(a, b Header) bool { return a.KeyID != "" && a.KeyID == b.KeyID }

// Limits of a capture (spec v0.2 §5.7, constants of the spec).
type Limits struct {
	MaxEntries         int           `json:"maxEntries"`
	MaxHashed          int64         `json:"maxHashed"`
	MaxFile            int64         `json:"maxFile"`
	MaxArchiveBytes    int64         `json:"maxArchiveBytes"`
	MaxDuration        time.Duration `json:"maxDuration"`
	MaxArchiveDuration time.Duration `json:"maxArchiveDuration"`
}

var DefaultLimits = Limits{
	MaxEntries:         500_000,
	MaxHashed:          1 << 30,
	MaxFile:            256 << 20,
	MaxArchiveBytes:    4 << 30,
	MaxDuration:        10 * time.Minute,
	MaxArchiveDuration: 5 * time.Minute,
}

// ComparePaths orders clean container paths by their components: "/a",
// "/a/b", "/a-b" ("/" sorts before every other byte). The walk of procfs
// gives this order; the archive is sorted to it; the comparison relies on it.
func ComparePaths(a, b string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		x, y := a[i], b[i]
		if x == y {
			continue
		}
		if x == '/' {
			return -1
		}
		if y == '/' {
			return 1
		}
		if x < y {
			return -1
		}
		return 1
	}
	return len(a) - len(b)
}

var idPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// ValidID: a UUID v4 in lower case, the only form of a snapshot ID (spec
// v0.2 §6). Anything else names no snapshot.
func ValidID(id string) bool { return idPattern.MatchString(id) }

// NewID returns a random UUID v4, in lower case.
func NewID() string {
	var b [16]byte
	rand.Read(b[:]) // never fails (crypto/rand, Go 1.24+)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
