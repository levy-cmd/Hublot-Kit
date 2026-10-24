package snapshot

// Tests of the capture (spec v0.2 §5.7, §8), written before the code: keyed
// MAC, no content in the manifest, caps, archive entries never trusted.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

var bg = context.Background()

var testKey = bytes.Repeat([]byte{0x5a}, 32)

// fakeWalker walks in-memory entries, in the order given; read counts the
// bytes read from the files.
type fakeWalker struct {
	items []fakeItem
	read  atomic.Int64
	err   error // returned at the end
}

type fakeItem struct {
	p       string
	e       fsview.Entry
	content string
	openErr error
}

func (w *fakeWalker) file(p, content string) *fakeWalker {
	w.items = append(w.items, fakeItem{p: p, e: fsview.Entry{Type: fsview.TypeFile, Size: int64(len(content)), Mode: 0o644}, content: content})
	return w
}

func (w *fakeWalker) sized(p string, size int64) *fakeWalker {
	w.items = append(w.items, fakeItem{p: p, e: fsview.Entry{Type: fsview.TypeFile, Size: size, Mode: 0o644}, content: strings.Repeat("x", int(min(size, 1<<20)))})
	return w
}

func (w *fakeWalker) add(p string, t fsview.EntryType) *fakeWalker {
	w.items = append(w.items, fakeItem{p: p, e: fsview.Entry{Type: t, Mode: 0o755}})
	return w
}

type countReader struct {
	r io.Reader
	n *atomic.Int64
}

func (c countReader) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	c.n.Add(int64(n))
	return n, err
}

func (w *fakeWalker) Walk(ctx context.Context, fn func(p string, e fsview.Entry, open func() (io.ReadCloser, error)) error) error {
	for _, it := range w.items {
		if err := ctx.Err(); err != nil {
			return err
		}
		var open func() (io.ReadCloser, error)
		if it.e.Type == fsview.TypeFile {
			open = func() (io.ReadCloser, error) {
				if it.openErr != nil {
					return nil, it.openErr
				}
				return io.NopCloser(countReader{strings.NewReader(it.content), &w.read}), nil
			}
		}
		if err := fn(it.p, it.e, open); err != nil {
			return err
		}
	}
	return w.err
}

func mac(key []byte, s string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(s))
	return hex.EncodeToString(m.Sum(nil))
}

func byPath(c *Collector) map[string]Entry {
	out := map[string]Entry{}
	for _, e := range c.Entries {
		out[e.Path] = e
	}
	return out
}

func TestMACIsKeyed(t *testing.T) {
	var c Collector
	if err := CaptureProcfs(bg, (&fakeWalker{}).file("/pin", "1234"), testKey, DefaultLimits, &c); err != nil {
		t.Fatal(err)
	}
	got := byPath(&c)["/pin"].MAC
	sum := sha256.Sum256([]byte("1234"))
	if got == "" || got == hex.EncodeToString(sum[:]) {
		t.Fatalf("MAC %q : vide ou égal au SHA-256 nu", got)
	}
	if got == mac(bytes.Repeat([]byte{1}, 32), "1234") || got != mac(testKey, "1234") {
		t.Fatalf("MAC %q : pas le HMAC de la clé de l'agent", got)
	}
}

func TestCaptureNoContentInManifest(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir)
	h := header("c1")
	w, err := s.Create(h)
	if err != nil {
		t.Fatal(err)
	}
	walker := (&fakeWalker{}).add("/run", fsview.TypeDir).file("/run/secret", "s3cr3t-contenu")
	if err := CaptureProcfs(bg, walker, testKey, DefaultLimits, w); err != nil {
		t.Fatal(err)
	}
	if err := w.Commit(h); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "snapshots", h.ID+".jsonl.gz"))
	if err != nil {
		t.Fatal(err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	text, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range [][]byte{raw, text} {
		if bytes.Contains(b, []byte("s3cr3t-contenu")) || bytes.Contains(b, []byte(hex.EncodeToString([]byte("s3cr3t-contenu")))) {
			t.Fatal("contenu du fichier dans le manifeste")
		}
	}
	if !bytes.Contains(text, []byte(mac(testKey, "s3cr3t-contenu"))) || !bytes.Contains(text, []byte(`"/run/secret"`)) {
		t.Fatalf("manifeste sans l'entrée attendue :\n%s", text)
	}
	if lines := bytes.Count(text, []byte("\n")); lines != 3 {
		t.Fatalf("%d lignes, attendu l'en-tête puis 2 entrées :\n%s", lines, text)
	}
}

// --- archives -------------------------------------------------------------

type tarEntry struct {
	h       tar.Header
	content string
}

func reg(name, content string) tarEntry {
	return tarEntry{tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(content)), ModTime: time.Unix(1700000000, 0)}, content}
}

func dir(name string) tarEntry {
	return tarEntry{tar.Header{Name: name, Typeflag: tar.TypeDir, Mode: 0o755, ModTime: time.Unix(1700000000, 0)}, ""}
}

func mkTar(t *testing.T, es ...tarEntry) []byte {
	t.Helper()
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	for _, e := range es {
		h := e.h
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if e.content != "" {
			if _, err := io.WriteString(tw, e.content); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestCaptureArchiveEntries(t *testing.T) {
	raw := mkTar(t, dir("/"), dir("/etc/"), reg("/etc/motd", "bonjour"),
		tarEntry{tar.Header{Name: "/etc/link", Typeflag: tar.TypeSymlink, Linkname: "motd", ModTime: time.Unix(1700000000, 0)}, ""},
		tarEntry{tar.Header{Name: "/etc/hard", Typeflag: tar.TypeLink, Linkname: "/etc/motd", ModTime: time.Unix(1700000000, 0)}, ""},
		tarEntry{tar.Header{Name: "/etc/fifo", Typeflag: tar.TypeFifo, ModTime: time.Unix(1700000000, 0)}, ""},
		dir("/a-b/"), dir("/a/"), reg("/a/b", "x"))
	var c Collector
	if err := CaptureArchive(bg, bytes.NewReader(raw), nil, testKey, DefaultLimits, &c); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range c.Entries {
		got = append(got, e.Path)
	}
	if want := []string{"/a", "/a/b", "/a-b", "/etc", "/etc/fifo", "/etc/hard", "/etc/link", "/etc/motd"}; !slices.Equal(got, want) {
		t.Fatalf("entrées : %v, attendu %v (ordre des composants, sans /)", got, want)
	}
	m := byPath(&c)
	if e := m["/etc/motd"]; e.Type != "file" || e.MAC != mac(testKey, "bonjour") || e.Size != 7 || e.Mode != 0o644 {
		t.Errorf("/etc/motd : %+v", e)
	}
	if e := m["/etc/hard"]; e.Type != "file" || e.MAC != m["/etc/motd"].MAC || e.Size != 7 {
		t.Errorf("/etc/hard (lien physique) : %+v", e)
	}
	if e := m["/etc/link"]; e.Type != "symlink" || e.Link != "motd" || e.MAC != "" {
		t.Errorf("/etc/link : %+v", e)
	}
	if e := m["/etc/fifo"]; e.Type != "other" || e.MAC != "" {
		t.Errorf("/etc/fifo : %+v", e)
	}
}

func TestCaptureArchiveSkipsMounts(t *testing.T) {
	raw := mkTar(t, dir("/data/"), reg("/data/db", "volume"), dir("/data/sub/"), reg("/database", "pas un montage"),
		dir("/proc/"), reg("/proc/kcore", "x"), dir("/sys/"), reg("/sys/x", "x"), reg("/etc/hosts", "127.0.0.1"))
	var c Collector
	if err := CaptureArchive(bg, bytes.NewReader(raw), []string{"/data", "/etc/hosts/"}, testKey, DefaultLimits, &c); err != nil {
		t.Fatal(err)
	}
	m := byPath(&c)
	if e, ok := m["/data"]; !ok || e.Type != "mount" || e.MAC != "" {
		t.Errorf("/data : %+v (présent : %v), attendu mount", e, ok)
	}
	if e, ok := m["/etc/hosts"]; !ok || e.Type != "mount" || e.MAC != "" {
		t.Errorf("/etc/hosts : %+v (présent : %v), attendu mount", e, ok)
	}
	for _, p := range []string{"/data/db", "/data/sub", "/proc/kcore", "/sys/x"} {
		if _, ok := m[p]; ok {
			t.Errorf("%s noté", p)
		}
	}
	for _, p := range []string{"/proc", "/sys"} {
		if m[p].Type != "inaccessible" {
			t.Errorf("%s : %+v, attendu inaccessible", p, m[p])
		}
	}
	if m["/database"].MAC != mac(testKey, "pas un montage") {
		t.Errorf("/database : %+v", m["/database"])
	}
}

func TestCaptureEntriesCap(t *testing.T) {
	lim := DefaultLimits
	lim.MaxEntries = 3
	w := (&fakeWalker{}).file("/a", "1").file("/b", "2").file("/c", "3").file("/d", "4")
	var c Collector
	if err := CaptureProcfs(bg, w, testKey, lim, &c); !errors.Is(err, fsview.ErrTooLarge) {
		t.Fatalf("procfs : %v, attendu ErrTooLarge", err)
	}
	var es []tarEntry
	for i := range 4 {
		es = append(es, reg("/f"+strconv.Itoa(i), "x"))
	}
	var c2 Collector
	if err := CaptureArchive(bg, bytes.NewReader(mkTar(t, es...)), nil, testKey, lim, &c2); !errors.Is(err, fsview.ErrTooLarge) {
		t.Fatalf("archive : %v, attendu ErrTooLarge", err)
	}
	if len(c2.Entries) != 0 {
		t.Fatalf("archive : %d entrées écrites malgré le refus", len(c2.Entries))
	}
}

func TestCaptureHashBudget(t *testing.T) {
	lim := DefaultLimits
	lim.MaxHashed = 10
	w := (&fakeWalker{}).file("/a", "123456").file("/b", "123456").file("/c", "1234")
	var c Collector
	if err := CaptureProcfs(bg, w, testKey, lim, &c); err != nil {
		t.Fatal(err)
	}
	m := byPath(&c)
	if m["/a"].MAC == "" || m["/b"].MAC != "" || m["/b"].MACSkipped != "budget" || m["/c"].MAC == "" {
		t.Fatalf("budget : %+v", c.Entries)
	}
	if !c.Partial {
		t.Fatal("snapshot au-delà du budget non marqué partial")
	}
}

func TestCaptureFileCap(t *testing.T) {
	lim := DefaultLimits
	lim.MaxFile = 1 << 20
	w := (&fakeWalker{}).sized("/big", 1<<20+1)
	var c Collector
	if err := CaptureProcfs(bg, w, testKey, lim, &c); err != nil {
		t.Fatal(err)
	}
	if e := byPath(&c)["/big"]; e.MAC != "" || e.MACSkipped != "too_large" {
		t.Fatalf("/big : %+v", e)
	}
	if n := w.read.Load(); n != 0 {
		t.Fatalf("%d octets lus d'un fichier au-delà du plafond", n)
	}
	if c.Partial {
		t.Fatal("too_large ne rend pas le snapshot partial")
	}
}

func TestCaptureUnreadable(t *testing.T) {
	w := &fakeWalker{}
	w.items = append(w.items, fakeItem{p: "/x", e: fsview.Entry{Type: fsview.TypeFile, Size: 3}, openErr: fsview.ErrLinkInPath})
	var c Collector
	if err := CaptureProcfs(bg, w, testKey, DefaultLimits, &c); err != nil {
		t.Fatal(err)
	}
	if e := byPath(&c)["/x"]; e.MAC != "" || e.MACSkipped != "unreadable" {
		t.Fatalf("/x : %+v", e)
	}
}

// A directory of more than MaxEntries names: the walk goes on, partial.
func TestCaptureWalkTruncated(t *testing.T) {
	w := (&fakeWalker{err: fsview.ErrTruncated}).file("/a", "1")
	var c Collector
	if err := CaptureProcfs(bg, w, testKey, DefaultLimits, &c); err != nil || !c.Partial || len(c.Entries) != 1 {
		t.Fatalf("parcours tronqué : %v partial=%v %d entrées", err, c.Partial, len(c.Entries))
	}
}

func TestCaptureProcfsDuration(t *testing.T) {
	lim := DefaultLimits
	lim.MaxDuration = 50 * time.Millisecond
	w := &slowWalker{}
	var c Collector
	start := time.Now()
	if err := CaptureProcfs(bg, w, testKey, lim, &c); !errors.Is(err, fsview.ErrTooLarge) {
		t.Fatalf("au-delà de MaxDuration : %v, attendu ErrTooLarge", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("abandon en %v", d)
	}
}

type slowWalker struct{}

func (slowWalker) Walk(ctx context.Context, fn func(string, fsview.Entry, func() (io.ReadCloser, error)) error) error {
	for i := 0; ; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
		if err := fn(fmt.Sprintf("/f%09d", i), fsview.Entry{Type: fsview.TypeDir}, nil); err != nil {
			return err
		}
	}
}

// rawSparse is a PAX entry declared sparse, of logical size 2^60, with 5
// bytes of data: GNU.sparse.* records (format 0.1).
func rawSparse(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	h := &tar.Header{Name: "/bomb", Typeflag: tar.TypeReg, Size: 5, Mode: 0o644, Format: tar.FormatPAX,
		PAXRecords: map[string]string{"GNU.sparse.map": "0,5", "GNU.sparse.numblocks": "1", "GNU.sparse.size": strconv.FormatInt(1<<60, 10)}}
	if err := tw.WriteHeader(h); err != nil {
		t.Fatal(err)
	}
	tw.Write([]byte("hello"))
	if err := tw.WriteHeader(&tar.Header{Name: "/after", Typeflag: tar.TypeReg, Size: 1, Mode: 0o644}); err != nil {
		t.Fatal(err)
	}
	tw.Write([]byte("A"))
	tw.Close()
	return b.Bytes()
}

func TestCaptureArchiveSparse(t *testing.T) {
	raw := rawSparse(t)
	done := make(chan error, 1)
	var c Collector
	go func() { done <- CaptureArchive(bg, bytes.NewReader(raw), nil, testKey, DefaultLimits, &c) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("capture jamais finie en 1 s : l'entrée creuse est hachée")
	}
	m := byPath(&c)
	if e := m["/bomb"]; e.MAC != "" || e.MACSkipped != "sparse" {
		t.Fatalf("/bomb : %+v", e)
	}
	if m["/after"].MAC != mac(testKey, "A") {
		t.Fatalf("/after : %+v", m["/after"])
	}
}

// An archive whose declared size passes the cap is skipped by Next(), never
// read into the HMAC.
func TestCaptureArchiveFileCap(t *testing.T) {
	lim := DefaultLimits
	lim.MaxFile = 4
	raw := mkTar(t, reg("/big", "12345"), reg("/small", "1234"))
	var c Collector
	if err := CaptureArchive(bg, bytes.NewReader(raw), nil, testKey, lim, &c); err != nil {
		t.Fatal(err)
	}
	m := byPath(&c)
	if m["/big"].MAC != "" || m["/big"].MACSkipped != "too_large" || m["/small"].MAC != mac(testKey, "1234") {
		t.Fatalf("plafond par fichier : %+v", c.Entries)
	}
}

type slowReader struct {
	r io.Reader
}

func (s slowReader) Read(b []byte) (int, error) {
	time.Sleep(20 * time.Millisecond)
	if len(b) > 512 {
		b = b[:512]
	}
	return s.r.Read(b)
}

func TestCaptureArchiveLimits(t *testing.T) {
	var es []tarEntry
	for i := range 50 {
		es = append(es, reg("/f"+strconv.Itoa(i), strings.Repeat("x", 1000)))
	}
	raw := mkTar(t, es...)
	lim := DefaultLimits
	lim.MaxArchiveBytes = int64(len(raw) / 2)
	var c Collector
	if err := CaptureArchive(bg, bytes.NewReader(raw), nil, testKey, lim, &c); !errors.Is(err, fsview.ErrTooLarge) {
		t.Fatalf("au-delà de MaxArchiveBytes : %v, attendu ErrTooLarge", err)
	}
	if len(c.Entries) != 0 {
		t.Fatalf("%d entrées écrites malgré le refus", len(c.Entries))
	}
	lim = DefaultLimits
	lim.MaxArchiveDuration = 100 * time.Millisecond
	var c2 Collector
	start := time.Now()
	if err := CaptureArchive(bg, slowReader{bytes.NewReader(raw)}, nil, testKey, lim, &c2); !errors.Is(err, fsview.ErrTooLarge) {
		t.Fatalf("au-delà de MaxArchiveDuration : %v, attendu ErrTooLarge", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("abandon en %v", d)
	}
	if len(c2.Entries) != 0 {
		t.Fatalf("%d entrées écrites malgré le refus", len(c2.Entries))
	}
}

// Without a key (the count): every entry, no content read.
func TestCaptureWithoutKeyReadsNothing(t *testing.T) {
	w := (&fakeWalker{}).file("/a", "123").file("/b", "456")
	var c Collector
	if err := CaptureProcfs(bg, w, nil, DefaultLimits, &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Entries) != 2 || w.read.Load() != 0 || c.Entries[0].MAC != "" {
		t.Fatalf("sans clé : %+v, %d octets lus", c.Entries, w.read.Load())
	}
}

func TestCaptureIndexMetadataOnly(t *testing.T) {
	m := fsview.NewMemFS()
	m.AddDir("/etc").AddFile("/etc/motd", []byte("bonjour")).AddLink("/etc/l", "motd").
		AddDir("/data").AddFile("/data/db", []byte("volume")).AddDir("/proc").AddFile("/a-b", nil).AddDir("/a").AddFile("/a/b", nil)
	var c Collector
	if err := CaptureIndex(MaskMounts(m, []string{"/data"}), &c); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range c.Entries {
		got = append(got, e.Path)
		if e.MAC != "" {
			t.Errorf("%s : MAC dans une photo des métadonnées", e.Path)
		}
	}
	if want := []string{"/a", "/a/b", "/a-b", "/data", "/etc", "/etc/l", "/etc/motd", "/proc"}; !slices.Equal(got, want) {
		t.Fatalf("entrées : %v, attendu %v", got, want)
	}
	byP := byPath(&c)
	if byP["/data"].Type != "mount" || byP["/proc"].Type != "inaccessible" || byP["/etc/l"].Link != "motd" || byP["/etc/motd"].Size != 7 {
		t.Fatalf("entrées : %+v", c.Entries)
	}
}

func TestComparePaths(t *testing.T) {
	ps := []string{"/a-b", "/a/b", "/a", "/b", "/a/b/c", "/a.b", "/a/a"}
	sort.Slice(ps, func(i, j int) bool { return ComparePaths(ps[i], ps[j]) < 0 })
	if want := []string{"/a", "/a/a", "/a/b", "/a/b/c", "/a-b", "/a.b", "/b"}; !slices.Equal(ps, want) {
		t.Fatalf("ordre : %v, attendu %v", ps, want)
	}
}
