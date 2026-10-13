package fsview

// Tests of tarindex (spec §5.2, §10), written before the code. The index is
// built from tars generated in memory; Open goes through a fake Docker whose
// archives are keyed by the exact path asked. At every path a naive
// implementation would ask for (the link itself, a lexical "..", a raw
// target), the fake holds an archive of HOST-SECRET: TestTarTrapsAreArmed
// proves the fake serves it, the other tests prove it is never asked for.

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

var tctx = context.Background()

var tarID = strings.Repeat("cd", 32)

type hdr struct {
	name, link, body string
	typ              byte
	mode             int64
}

func tdir(name string) hdr        { return hdr{name: name, typ: tar.TypeDir, mode: 0o755} }
func tfile(name, body string) hdr { return hdr{name: name, typ: tar.TypeReg, mode: 0o644, body: body} }
func tlink(name, target string) hdr {
	return hdr{name: name, typ: tar.TypeSymlink, mode: 0o777, link: target}
}
func thard(name, target string) hdr {
	return hdr{name: name, typ: tar.TypeLink, mode: 0o644, link: target}
}
func tnode(name string, typ byte) hdr { return hdr{name: name, typ: typ, mode: 0o644} }

func mkTar(t *testing.T, hs ...hdr) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, h := range hs {
		th := &tar.Header{Name: h.name, Typeflag: h.typ, Linkname: h.link, Mode: h.mode,
			Size: int64(len(h.body)), ModTime: time.Unix(1_700_000_000, 0), Uid: 1000, Gid: 1000}
		if err := tw.WriteHeader(th); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tw, h.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// image is the fake container, named as GET /archive of "/" names it.
func image() []hdr {
	return []hdr{
		tdir("/"), tdir("/etc/"), tfile("/etc/passwd", "root:x:0:0"),
		tdir("/usr/"), tdir("/usr/lib/"), tfile("/usr/lib/os-release", "NAME=Alpine"),
		tdir("/usr/share/"), tfile("/usr/share/x", "SHARE-X"),
		tlink("/etc/os-release", "../usr/lib/os-release"), // relative: from the link's directory
		tlink("/abs", "/usr/lib/os-release"),              // absolute: from the container root
		tlink("/up", "../../../../usr/lib/os-release"),    // ".." blocked at the root
		tlink("/lib", "usr/lib"),                          // link on an intermediate directory
		tlink("/d", "/usr/lib"),                           // ".." after a linked directory
		tlink("/e", "d/../share/x"),                       // ".." inside a target, after a linked directory
		tlink("/c1", "c2"), tlink("/c2", "c3"), tlink("/c3", "/etc/passwd"),
		tlink("/loop1", "loop2"), tlink("/loop2", "loop1"),
		tlink("/dangling", "/nope"), tlink("/empty", ""),
		tdir("/bin/"), {name: "/bin/busybox", typ: tar.TypeReg, mode: 0o4755, body: "BUSYBOX"},
		thard("/bin/sh", "/bin/busybox"), // hard link: linkname = first occurrence
		tdir("/dev/"), tnode("/dev/null", tar.TypeChar), tnode("/dev/sda", tar.TypeBlock),
		tnode("/run.fifo", tar.TypeFifo),
		// Spec §5.2 step 7: nothing under /proc and /sys is indexed.
		tdir("/proc/"), tfile("/proc/kallsyms", "HOST-SECRET"), tdir("/sys/"), tfile("/sys/kernel/x", "HOST-SECRET"),
	}
}

// fileTar is what GET /archive of one file returns: one entry, its base name.
func fileTar(t *testing.T, p, body string) []byte { return mkTar(t, tfile(path.Base(p), body)) }

var realFiles = map[string]string{
	"/etc/passwd": "root:x:0:0", "/usr/lib/os-release": "NAME=Alpine",
	"/usr/share/x": "SHARE-X", "/bin/busybox": "BUSYBOX",
}

// traps: what a naive implementation would send to GetArchive.
var traps = []string{
	"/etc/os-release", "/abs", "/up", "/lib/os-release", "/lib", "/d/../share/x", "/share/x",
	"/c1", "/c2", "/c3", "/e", "/bin/sh", "/a/x", "/../../../../usr/lib/os-release",
	"/usr/lib/../../../../usr/lib/os-release", "../usr/lib/os-release", "usr/lib/os-release",
	"/proc/kallsyms", "/sys/kernel/x",
}

func trapDocker(t *testing.T) *dockerx.Fake {
	f := &dockerx.Fake{Archives: map[string][]byte{}}
	for p, body := range realFiles {
		f.Archives[tarID+":"+p] = fileTar(t, p, body)
	}
	for _, p := range traps {
		f.Archives[tarID+":"+p] = fileTar(t, p, "HOST-SECRET")
	}
	return f
}

type tarView struct {
	*tarFS
	docker *dockerx.Fake
	stale  *atomic.Int32
}

func newTarView(t *testing.T, mounts []dockerx.Mount, hs ...hdr) tarView {
	t.Helper()
	idx, err := buildIndex(tctx, bytes.NewReader(mkTar(t, hs...)), DefaultIndexLimits, new(atomic.Int64))
	if err != nil {
		t.Fatalf("buildIndex : %v", err)
	}
	f := trapDocker(t)
	stale := new(atomic.Int32)
	return tarView{newTarFS(f, dockerx.ContainerInfo{ID: tarID, Mounts: mounts}, idx, func() { stale.Add(1) }), f, stale}
}

func readTar(fsys FS, p string) ([]byte, error) {
	rc, err := fsys.Open(tctx, p)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// noLeak: no HOST-SECRET in what was read, error or not, and GetArchive was
// called exactly for want (none when want is empty).
func noLeak(t *testing.T, v tarView, b []byte, want ...string) {
	t.Helper()
	if bytes.Contains(b, []byte("HOST-SECRET")) {
		t.Errorf("ÉVASION : contenu piégé lu : %q", b)
	}
	exp := make([]string, len(want))
	for i, w := range want {
		exp[i] = tarID + ":" + w
	}
	if calls := v.docker.Calls(); !slices.Equal(calls, exp) {
		t.Errorf("ÉVASION : GetArchive %v, attendu %v", calls, exp)
	}
}

func TestTarTrapsAreArmed(t *testing.T) {
	f := trapDocker(t)
	for _, p := range traps {
		rc, err := f.GetArchive(tctx, tarID, p)
		if err != nil {
			t.Fatalf("piège %s : %v", p, err)
		}
		tr := tar.NewReader(rc)
		if _, err := tr.Next(); err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tr.Next(); err != io.EOF { // to the end of the tar
			t.Fatalf("piège %s : %v, attendu io.EOF", p, err)
		}
		rc.Close()
		if string(b) != "HOST-SECRET" {
			t.Fatalf("piège %s désarmé : %q", p, b)
		}
	}
}

func TestTarOpenResolves(t *testing.T) {
	cases := []struct{ path, resolved, body string }{
		{"/etc/os-release", "/usr/lib/os-release", "NAME=Alpine"},
		{"/abs", "/usr/lib/os-release", "NAME=Alpine"},
		{"/up", "/usr/lib/os-release", "NAME=Alpine"},
		{"/lib/os-release", "/usr/lib/os-release", "NAME=Alpine"},
		{"/d/../share/x", "/usr/share/x", "SHARE-X"}, // parent of the resolved target, never /share/x
		{"/e", "/usr/share/x", "SHARE-X"},            // the same "..", inside a link target
		{"/c1", "/etc/passwd", "root:x:0:0"},
		{"/bin/sh", "/bin/busybox", "BUSYBOX"},
		{"/../../etc/passwd", "/etc/passwd", "root:x:0:0"},
		{"/etc/./passwd", "/etc/passwd", "root:x:0:0"},
		{"//etc//passwd", "/etc/passwd", "root:x:0:0"},
		{"/usr/lib/../../../../usr/lib/os-release", "/usr/lib/os-release", "NAME=Alpine"},
	}
	for _, c := range cases {
		v := newTarView(t, nil, image()...)
		b, err := readTar(v, c.path)
		noLeak(t, v, b, c.resolved)
		if err != nil || string(b) != c.body {
			t.Errorf("mauvaise erreur : Open(%s) = %q, %v ; attendu %q", c.path, b, err, c.body)
		}
	}
}

func TestTarStat(t *testing.T) {
	v := newTarView(t, nil, image()...)
	if e, err := v.Stat(tctx, "/etc/os-release"); err != nil || e.Type != TypeFile || e.Size != int64(len("NAME=Alpine")) {
		t.Errorf("lien suivi : %+v %v", e, err)
	}
	// Hard link (spec §5.2 step 6): a file, with the size and mode of its target.
	if e, err := v.Stat(tctx, "/bin/sh"); err != nil || e.Type != TypeFile || e.Size != 7 || e.Mode != fs.ModeSetuid|0o755 {
		t.Errorf("lien physique : %+v %v", e, err)
	}
	for _, p := range []string{"/", "/d", "/etc/", "/lib/", "/d/"} {
		if e, err := v.Stat(tctx, p); err != nil || e.Type != TypeDir {
			t.Errorf("Stat(%s) : %+v %v, attendu un dossier", p, e, err)
		}
	}
	for _, p := range []string{"/proc/kallsyms", "/sys/kernel/x", "/nope", "/dangling", "/empty"} {
		if _, err := v.Stat(tctx, p); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Stat(%s) : %v, attendu fs.ErrNotExist", p, err)
		}
	}
	for _, p := range []string{"/etc/passwd/", "/c3/", "/etc/passwd/x", "/etc/passwd/.."} {
		if _, err := v.Stat(tctx, p); !errors.Is(err, ErrNotDir) {
			t.Errorf("Stat(%s) : %v, attendu ErrNotDir", p, err)
		}
	}
	noLeak(t, v, nil) // Stat never asks Docker
}

func TestTarList(t *testing.T) {
	v := newTarView(t, nil, image()...)
	root, truncated, err := v.List(tctx, "/")
	if err != nil || truncated {
		t.Fatalf("List(/) : %v %v", truncated, err)
	}
	byName := map[string]Entry{}
	for _, e := range root {
		byName[e.Name] = e
	}
	if e := byName["up"]; e.Type != TypeSymlink || e.LinkTarget != "../../../../usr/lib/os-release" {
		t.Errorf("cible brute attendue : %+v", e)
	}
	if e := byName["proc"]; e.Type != TypeDir {
		t.Errorf("/proc doit être montré comme un dossier : %+v", e)
	}
	if e := byName["run.fifo"]; e.Type != TypeOther {
		t.Errorf("FIFO : %+v", e)
	}
	for _, d := range []string{"/proc", "/sys"} {
		if entries, _, err := v.List(tctx, d); err != nil || len(entries) != 0 {
			t.Errorf("ÉVASION : List(%s) = %v, %v ; attendu vide", d, entries, err)
		}
	}
	if lib, _, err := v.List(tctx, "/lib"); err != nil || len(lib) != 1 || lib[0].Name != "os-release" {
		t.Errorf("List(/lib) : %+v %v", lib, err)
	}
	if _, _, err := v.List(tctx, "/etc/passwd"); !errors.Is(err, ErrNotDir) {
		t.Errorf("List d'un fichier : %v", err)
	}
	if _, _, err := v.List(tctx, "/nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("List absent : %v", err)
	}
	noLeak(t, v, nil)
}

func TestTarListTruncates(t *testing.T) {
	hs := []hdr{tdir("/"), tdir("/big/")}
	for i := 0; i <= MaxEntries; i++ {
		hs = append(hs, tfile(fmt.Sprintf("/big/f%05d", i), ""))
	}
	v := newTarView(t, nil, hs...)
	if entries, truncated, err := v.List(tctx, "/big"); err != nil || !truncated || len(entries) != MaxEntries {
		t.Fatalf("%d entrées, tronqué %v, %v", len(entries), truncated, err)
	}
}

// One single counter of 40 hops for the whole resolution (spec §5.2 step 5).
func TestTarHopBudget(t *testing.T) {
	hs := []hdr{tdir("/"), tdir("/real/"), tfile("/real/file", "OK"), tlink("/real/g1", "file"), tlink("/s1", "/real"),
		tlink("/loop1", "loop2"), tlink("/loop2", "loop1")}
	for i := 2; i <= 21; i++ {
		hs = append(hs, tlink(fmt.Sprintf("/real/g%d", i), fmt.Sprintf("g%d", i-1)))
	}
	for i := 2; i <= 20; i++ {
		hs = append(hs, tlink(fmt.Sprintf("/s%d", i), fmt.Sprintf("/s%d", i-1)))
	}
	v := newTarView(t, nil, hs...)
	if e, err := v.Stat(tctx, "/s20/g20"); err != nil || e.Type != TypeFile { // 20 + 20 = 40
		t.Errorf("40 sauts : %+v %v", e, err)
	}
	if _, err := v.Stat(tctx, "/real/g21"); err != nil { // control: 21 hops alone resolve
		t.Errorf("21 sauts : %v", err)
	}
	if _, err := v.Stat(tctx, "/s20/g21"); !errors.Is(err, syscall.ELOOP) { // 20 + 21: one counter
		t.Errorf("41 sauts : %v, attendu ELOOP", err)
	}
	if _, err := v.Stat(tctx, "/loop1"); !errors.Is(err, syscall.ELOOP) {
		t.Errorf("boucle : %v", err)
	}
	b, err := readTar(v, "/loop1")
	noLeak(t, v, b)
	if !errors.Is(err, syscall.ELOOP) {
		t.Errorf("Open d'une boucle : %v", err)
	}
}

// Review I1: a path the index does not know (absent, excluded, behind a file)
// is refused from the index, never looked up in Docker: the /proc and /sys
// traps are armed and must not be asked for.
func TestTarOpenAbsent(t *testing.T) {
	cases := []struct {
		path string
		want error
	}{
		{"/proc/kallsyms", fs.ErrNotExist}, {"/sys/kernel/x", fs.ErrNotExist},
		{"/nope", fs.ErrNotExist}, {"/dangling", fs.ErrNotExist}, {"/empty", fs.ErrNotExist},
		{"/k", fs.ErrNotExist}, // link to an excluded path
		{"/etc/passwd/x", ErrNotDir}, {"/etc/passwd/", ErrNotDir},
	}
	for _, c := range cases {
		v := newTarView(t, nil, append(image(), tlink("/k", "/proc/kallsyms"))...)
		b, err := readTar(v, c.path)
		noLeak(t, v, b)
		if !errors.Is(err, c.want) {
			t.Errorf("Open(%s) : %v, attendu %v", c.path, err, c.want)
		}
		if err := v.CheckOpen(tctx, c.path); !errors.Is(err, c.want) {
			t.Errorf("CheckOpen(%s) : %v, attendu %v", c.path, err, c.want)
		}
	}
}

func TestTarOpenRefusesNonRegular(t *testing.T) {
	for _, p := range []string{"/dev/null", "/dev/sda", "/run.fifo", "/etc", "/lib", "/"} {
		v := newTarView(t, nil, image()...)
		b, err := readTar(v, p)
		noLeak(t, v, b) // refused from the index, before any GetArchive
		if !errors.Is(err, ErrNotRegular) {
			t.Errorf("Open(%s) : %v, attendu ErrNotRegular", p, err)
		}
	}
}

// The entry Docker sends must be the regular file the index announced; a
// docker cp since the build changes it.
func TestTarOpenChecksTheArchive(t *testing.T) {
	cases := map[string]func(t *testing.T) []byte{
		"lien à la place": func(t *testing.T) []byte { return mkTar(t, tlink("passwd", "/proc/self/root/etc/shadow")) },
		"autre nom":       func(t *testing.T) []byte { return mkTar(t, tfile("shadow", "HOST-SECRET")) },
		"dossier":         func(t *testing.T) []byte { return mkTar(t, tdir("passwd/"), tfile("passwd/x", "HOST-SECRET")) },
	}
	for name, archive := range cases {
		v := newTarView(t, nil, image()...)
		v.docker.Archives[tarID+":/etc/passwd"] = archive(t)
		b, err := readTar(v, "/etc/passwd")
		noLeak(t, v, b, "/etc/passwd")
		if !errors.Is(err, ErrContainerChanged) || v.stale.Load() != 1 {
			t.Errorf("%s : %v, index retiré %d fois", name, err, v.stale.Load())
		}
		if n := v.docker.ArchiveCloses.Load(); n != 1 {
			t.Errorf("%s : archive fermée %d fois", name, n)
		}
	}
}

// Read to io.EOF of the tar: a second entry or a cut archive is an error,
// never a silent success (a download would save a truncated file as whole).
func TestTarOpenReadsToEOF(t *testing.T) {
	full := mkTar(t, tfile("passwd", "root:x:0:0"))
	cases := map[string][]byte{
		"deux entrées": mkTar(t, tfile("passwd", "root:x:0:0"), tfile("extra", "HOST-SECRET")),
		"tronquée":     full[:512+5],
	}
	for name, archive := range cases {
		v := newTarView(t, nil, image()...)
		v.docker.Archives[tarID+":/etc/passwd"] = archive
		b, err := readTar(v, "/etc/passwd")
		noLeak(t, v, b, "/etc/passwd")
		if err == nil {
			t.Errorf("%s : lu sans erreur (%q)", name, b)
		}
	}
	v := newTarView(t, nil, image()...) // control: the intact archive
	if b, err := readTar(v, "/etc/passwd"); err != nil || string(b) != "root:x:0:0" || v.docker.ArchiveCloses.Load() != 1 {
		t.Errorf("archive intacte : %q %v", b, err)
	}
}

// Review I3: the first occurrence of a hard link is a path of the archive,
// but a later entry can turn one of its directories into a symbolic link.
// Docker would follow it: Open must refuse, from the index.
func TestTarHardLinkThroughSymlink(t *testing.T) {
	v := newTarView(t, nil,
		tdir("/"), tdir("/etc/"), tdir("/a/"), tfile("/a/x", "HOST-SECRET"),
		tlink("/a", "/etc"), // the directory /a becomes a link
		thard("/h", "/a/x"), // first occurrence: /a/x, now behind the link
	)
	b, err := readTar(v, "/h")
	noLeak(t, v, b)
	if !errors.Is(err, ErrNotRegular) {
		t.Errorf("Open(/h) : %v, attendu ErrNotRegular", err)
	}
	if err := v.CheckOpen(tctx, "/h"); !errors.Is(err, ErrNotRegular) {
		t.Errorf("CheckOpen(/h) : %v", err)
	}
}

// Review I2: &check=1 and HEAD on a file judge from the index only (spec
// §5.4: no GetArchive).
func TestTarCheckOpen(t *testing.T) {
	v := newTarView(t, nil, image()...)
	for _, p := range []string{"/etc/os-release", "/bin/sh", "/e"} {
		if err := v.CheckOpen(tctx, p); err != nil {
			t.Errorf("CheckOpen(%s) : %v", p, err)
		}
	}
	for _, p := range []string{"/dev/null", "/run.fifo", "/etc"} {
		if err := v.CheckOpen(tctx, p); !errors.Is(err, ErrNotRegular) {
			t.Errorf("CheckOpen(%s) : %v, attendu ErrNotRegular", p, err)
		}
	}
	if err := v.CheckOpen(tctx, "/loop1"); !errors.Is(err, syscall.ELOOP) {
		t.Errorf("CheckOpen(/loop1) : %v", err)
	}
	noLeak(t, v, nil)
}

func TestTarStatNoLinks(t *testing.T) {
	v := newTarView(t, nil, image()...)
	for _, p := range []string{"/etc/passwd", "/bin/sh", "/"} { // a hard link is no symbolic link
		if _, err := v.StatNoLinks(tctx, p); err != nil {
			t.Errorf("StatNoLinks(%s) : %v", p, err)
		}
	}
	for _, p := range []string{"/etc/os-release", "/lib/os-release", "/lib", "/c1"} {
		if _, err := v.StatNoLinks(tctx, p); !errors.Is(err, ErrLinkInPath) {
			t.Errorf("StatNoLinks(%s) : %v, attendu ErrLinkInPath", p, err)
		}
	}
	for _, p := range []string{"/nope", "/d/../share"} { // cleaned lexically: /share
		if _, err := v.StatNoLinks(tctx, p); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("StatNoLinks(%s) : %v", p, err)
		}
	}
	noLeak(t, v, nil)
}

func TestTarCheckArchive(t *testing.T) {
	bad := newTarView(t, []dockerx.Mount{{Type: "bind", Source: "/proc", Destination: "/host/proc"}}, image()...)
	if err := bad.CheckArchive(tctx, "/etc"); !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), "bind /proc -> /host/proc") {
		t.Errorf("bind de l'hôte : %v", err)
	}
	ok := newTarView(t, []dockerx.Mount{{Type: "bind", Source: "/srv/data", Destination: "/data"}}, image()...)
	if err := ok.CheckArchive(tctx, "/etc"); err != nil {
		t.Errorf("bind ordinaire : %v", err)
	}
	noLeak(t, bad, nil)
	noLeak(t, ok, nil)
}

func TestBuildIndexNames(t *testing.T) {
	v := newTarView(t, nil,
		tdir("./"), tdir("./a/"), tfile("./a/x", "1"),
		tfile("b/y", "22"), // relative name, parent never declared
		tfile("/c", "333"),
		tfile("/a/x", "4444"),                         // again: the last one wins, listed once
		tfile("/f", ""), tfile("/f/z", "HOST-SECRET"), // under a file: unreachable
		thard("/h1", "/nope"), thard("/h2", "/a"), // hard link to nothing, to a directory
	)
	if e, err := v.Stat(tctx, "/a/x"); err != nil || e.Size != 4 {
		t.Errorf("/a/x : %+v %v", e, err)
	}
	if a, _, err := v.List(tctx, "/a"); err != nil || len(a) != 1 {
		t.Errorf("doublon listé : %+v %v", a, err)
	}
	if e, err := v.Stat(tctx, "/b"); err != nil || e.Type != TypeDir {
		t.Errorf("parent implicite : %+v %v", e, err)
	}
	if e, err := v.Stat(tctx, "/b/y"); err != nil || e.Size != 2 {
		t.Errorf("/b/y : %+v %v", e, err)
	}
	if e, err := v.Stat(tctx, "/c"); err != nil || e.Size != 3 {
		t.Errorf("/c : %+v %v", e, err)
	}
	if _, err := v.Stat(tctx, "/f/z"); !errors.Is(err, ErrNotDir) {
		t.Errorf("/f/z : %v", err)
	}
	for _, p := range []string{"/h1", "/h2"} {
		if e, err := v.Stat(tctx, p); err != nil || e.Type != TypeOther {
			t.Errorf("Stat(%s) : %+v %v", p, e, err)
		}
		b, err := readTar(v, p)
		noLeak(t, v, b)
		if !errors.Is(err, ErrNotRegular) {
			t.Errorf("Open(%s) : %v", p, err)
		}
	}
}

type blockingReader struct{ ctx context.Context }

func (r blockingReader) Read([]byte) (int, error) { <-r.ctx.Done(); return 0, r.ctx.Err() }

func TestBuildIndexLimits(t *testing.T) {
	var hs []hdr
	for i := 0; i < 40; i++ {
		hs = append(hs, tfile(fmt.Sprintf("/f%d", i), strings.Repeat("x", 600)))
	}
	b := mkTar(t, hs...) // 40 × 1 536 + 1 024 octets
	lim := DefaultIndexLimits
	lim.MaxBytes = 4096
	if _, err := buildIndex(tctx, bytes.NewReader(b), lim, new(atomic.Int64)); !errors.Is(err, ErrTooLarge) {
		t.Errorf("plafond d'octets : %v", err)
	}
	lim = DefaultIndexLimits
	lim.MaxHeaders = 10
	if _, err := buildIndex(tctx, bytes.NewReader(b), lim, new(atomic.Int64)); !errors.Is(err, ErrTooLarge) {
		t.Errorf("plafond d'entrées : %v", err)
	}
	var read atomic.Int64 // control: within the limits, everything is read and counted
	if _, err := buildIndex(tctx, bytes.NewReader(b), DefaultIndexLimits, &read); err != nil || read.Load() < int64(len(b))-1024 || read.Load() > int64(len(b)) {
		t.Errorf("lecture complète : %d octets sur %d, %v", read.Load(), len(b), err)
	}
	dctx, cancel := context.WithTimeout(tctx, 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := buildIndex(dctx, blockingReader{dctx}, DefaultIndexLimits, new(atomic.Int64)); !errors.Is(err, ErrTooLarge) || time.Since(start) > 2*time.Second {
		t.Errorf("plafond de durée : %v après %s", err, time.Since(start))
	}
	cctx, ccancel := context.WithCancel(tctx)
	ccancel() // evicted from the cache: cancelled, not too large
	if _, err := buildIndex(cctx, bytes.NewReader(b), DefaultIndexLimits, new(atomic.Int64)); err == nil || errors.Is(err, ErrTooLarge) {
		t.Errorf("annulée : %v", err)
	}
	cut := len(b)/2 + 100 // mid-block
	if idx, err := buildIndex(tctx, bytes.NewReader(b[:cut]), DefaultIndexLimits, new(atomic.Int64)); err == nil || idx != nil || errors.Is(err, ErrTooLarge) {
		t.Errorf("archive tronquée : index %v, %v (attendu une erreur, jamais un index partiel)", idx != nil, err)
	}
}

func TestProgress(t *testing.T) {
	for _, c := range []struct {
		read, total int64
		want        float64
	}{{0, 0, 0}, {10, 0, 0}, {50, 100, 0.5}, {99, 100, 0.99}, {100, 100, 0.99}, {500, 100, 0.99}} {
		if got := progress(c.read, c.total); got != c.want {
			t.Errorf("progress(%d, %d) = %v, attendu %v", c.read, c.total, got, c.want)
		}
	}
}

// Review I5: bytes per index entry, against the 50 MB goal of spec §5.2 step 4.
func TestMeasureIndexMemory(t *testing.T) {
	const n = 50_000
	hs := []hdr{tdir("/")}
	for i := range n {
		hs = append(hs, tfile(fmt.Sprintf("/usr/share/doc/pkg%03d/file-%06d.txt", i%1000, i), ""))
	}
	b := mkTar(t, hs...)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	idx, err := buildIndex(tctx, bytes.NewReader(b), DefaultIndexLimits, new(atomic.Int64))
	if err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	// idx stays alive until after the second measure, or the GC frees it; the
	// input (hs, b) too, counted in "before", or its release skews the delta.
	runtime.KeepAlive(idx)
	runtime.KeepAlive(b)
	runtime.KeepAlive(hs)
	per := float64(int64(after.HeapAlloc)-int64(before.HeapAlloc)) / float64(len(idx.entries))
	t.Logf("MESURE : %.0f octets par entrée, soit %.0f Mo pour 1 000 000 d'entrées", per, per)
	if per < 50 || per > 2000 {
		t.Fatalf("mesure implausible : %.0f octets par entrée", per)
	}
}

// Spec §13, stopped container: runc mounts a --tmpfs at its destination
// resolved in the root filesystem, links followed. CheckWriteRoot resolves
// it the same way in the index (never out of the root), so package files
// refuses /run for a "tmpfs: [/var/run]" with /var/run -> ../run. A path
// that stops existing midway is resolved as far as it goes, the rest
// appended (runc creates it there: /dangling -> ../real gives /real/x).
func TestTarCheckWriteRootTmpfs(t *testing.T) {
	idx, err := buildIndex(tctx, bytes.NewReader(mkTar(t,
		tdir("/"), tdir("/run/"), tdir("/var/"), tlink("/var/run", "../run"),
		tlink("/var/lock", "/../../run/lock"), tdir("/run/lock/"), tdir("/tmp/"),
		tlink("/dangling", "../real"),
	)), DefaultIndexLimits, new(atomic.Int64))
	if err != nil {
		t.Fatal(err)
	}
	info := dockerx.ContainerInfo{ID: tarID, Tmpfs: []string{"/var/run", "/absent/x", "/dangling/x", "/var/run/new/../sub"},
		Mounts: []dockerx.Mount{{Type: "tmpfs", Destination: "/var/lock"}, {Type: "volume", Name: "v", Destination: "/tmp"}}}
	var w WriteRootChecker = newTarFS(&dockerx.Fake{}, info, idx, func() {})
	points, err := w.CheckWriteRoot(tctx)
	want := []string{"/run/lock", "/run", "/absent/x", "/real/x", "/run/sub"}
	if err != nil || !slices.Equal(points, want) {
		t.Fatalf("tmpfs résolus par l'index : %v %v, attendu %v", points, err, want)
	}
}
