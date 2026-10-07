//go:build linux

package procroot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// mkRoot creates a fake container root and, outside it, a host marker file.
func mkRoot(t *testing.T) (root *os.File, rootPath, marker string) {
	t.Helper()
	marker = filepath.Join(t.TempDir(), "MARKER")
	must(t, os.WriteFile(marker, []byte("HOST-SECRET"), 0o644))
	rootPath = t.TempDir()
	root, err := os.Open(rootPath)
	must(t, err)
	t.Cleanup(func() { root.Close() })
	return root, rootPath, marker
}

func assertNoEscape(t *testing.T, b []byte, err error) {
	t.Helper()
	if err == nil || strings.Contains(string(b), "HOST-SECRET") {
		t.Fatalf("ESCAPE: read %q err=%v", b, err)
	}
}

func TestReadFileAbsoluteSymlinkStaysInRoot(t *testing.T) {
	root, rp, marker := mkRoot(t)
	must(t, os.Symlink(marker, filepath.Join(rp, "evil")))
	if b, err := os.ReadFile(filepath.Join(rp, "evil")); err != nil || string(b) != "HOST-SECRET" {
		t.Fatalf("control: the naive read should escape, got %q %v", b, err)
	}
	b, err := ReadFile(root, "evil")
	assertNoEscape(t, b, err)
}

func TestReadFileRelativeSymlinkStaysInRoot(t *testing.T) {
	root, rp, marker := mkRoot(t)
	target := strings.Repeat("../", 20) + strings.TrimPrefix(marker, "/")
	must(t, os.Symlink(target, filepath.Join(rp, "evil-rel")))
	if b, err := os.ReadFile(filepath.Join(rp, "evil-rel")); err != nil || string(b) != "HOST-SECRET" {
		t.Fatalf("control: the naive read should escape, got %q %v", b, err)
	}
	b, err := ReadFile(root, "evil-rel")
	assertNoEscape(t, b, err)
}

func TestReadFileDotDotPathStaysInRoot(t *testing.T) {
	root, _, marker := mkRoot(t)
	b, err := ReadFile(root, strings.Repeat("../", 20)+strings.TrimPrefix(marker, "/"))
	assertNoEscape(t, b, err)
}

func TestSymlinkLoopFails(t *testing.T) {
	root, rp, _ := mkRoot(t)
	must(t, os.Symlink("loop2", filepath.Join(rp, "loop1")))
	must(t, os.Symlink("loop1", filepath.Join(rp, "loop2")))
	if _, err := ReadFile(root, "loop1"); err == nil {
		t.Fatal("a symlink loop must fail")
	}
}

func TestListTypesAndLinkTarget(t *testing.T) {
	root, rp, _ := mkRoot(t)
	must(t, os.WriteFile(filepath.Join(rp, "a.txt"), []byte("hello"), 0o640))
	must(t, os.Mkdir(filepath.Join(rp, "dir"), 0o755))
	must(t, os.Symlink("/etc/passwd", filepath.Join(rp, "link")))

	entries, truncated, err := List(root, "/", 100)
	must(t, err)
	if truncated {
		t.Fatal("unexpected truncation")
	}
	got := map[string]Entry{}
	for _, e := range entries {
		got[e.Name] = e
	}
	if e := got["a.txt"]; e.Type != "file" || e.Size != 5 || e.Mode != 0o640 {
		t.Errorf("a.txt: %+v", e)
	}
	if e := got["dir"]; e.Type != "dir" {
		t.Errorf("dir: %+v", e)
	}
	if e := got["link"]; e.Type != "symlink" || e.LinkTarget != "/etc/passwd" {
		t.Errorf("link: %+v (the link must be shown, never followed)", e)
	}
}

func TestListOddNames(t *testing.T) {
	root, rp, _ := mkRoot(t)
	names := []string{"with space", "émoji-🎉", "new\nline"}
	for _, n := range names {
		must(t, os.WriteFile(filepath.Join(rp, n), nil, 0o644))
	}
	entries, _, err := List(root, "/", 100)
	must(t, err)
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.Name] = true
	}
	for _, n := range names {
		if !seen[n] {
			t.Errorf("missing %q", n)
		}
	}
}

func TestListTruncates(t *testing.T) {
	root, rp, _ := mkRoot(t)
	for i := 0; i < 15; i++ {
		must(t, os.WriteFile(filepath.Join(rp, "f"+strings.Repeat("x", i)), nil, 0o644))
	}
	entries, truncated, err := List(root, "/", 10)
	must(t, err)
	if len(entries) != 10 || !truncated {
		t.Fatalf("want 10 entries truncated, got %d truncated=%v", len(entries), truncated)
	}
	entries, truncated, err = List(root, "/", 15)
	must(t, err)
	if len(entries) != 15 || truncated {
		t.Fatalf("want 15 entries not truncated, got %d truncated=%v", len(entries), truncated)
	}
}
