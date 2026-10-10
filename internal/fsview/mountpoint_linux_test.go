//go:build linux

package fsview

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// IsMountPoint (plan Explorateur 3, Q21): the STATX_MNT_ID of a path and of
// its parent differ. /proc is a mount point on any Linux host or container;
// /etc/passwd is not. No root needed.
func TestIsMountPoint(t *testing.T) {
	host, ok := openTestRoot(t, "/").(MountPointChecker)
	if !ok {
		t.Fatal("procfs doit implémenter MountPointChecker")
	}
	for p, want := range map[string]bool{"/proc": true, "/": true, "/etc/passwd": false, "/etc": false} {
		if got, err := host.IsMountPoint(ctx, p); err != nil || got != want {
			t.Errorf("%s : %v %v, attendu %v", p, got, err, want)
		}
	}
	// A link anywhere is refused, as by StatNoLinks: never followed.
	dir := t.TempDir()
	must(t, os.Symlink("/proc", filepath.Join(dir, "p")))
	if _, err := host.IsMountPoint(ctx, dir+"/p"); !errors.Is(err, ErrLinkInPath) {
		t.Errorf("lien : %v, attendu ErrLinkInPath", err)
	}
}
