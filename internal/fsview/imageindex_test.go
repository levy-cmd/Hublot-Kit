package fsview

// Tests of the index of an image's layers (spec v0.2 §5.5, plan v0.2
// tranche 6), written before the code. The archives are `docker save`
// exports built in memory: blobs/sha256/<hex> (gzip layers, the config),
// then index.json, oci-layout and manifest.json, as Docker 29 writes them.
// The layers come from the image's author: every test here is an escape, a
// format trap or a cap. Nothing is ever written to the disk, no container is
// ever created, every blob is checked against its name.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

var imgID = "sha256:" + strings.Repeat("ab", 32)

// outerFile is one entry of the outer archive (typ 0: a regular file).
type outerFile struct {
	name string
	body []byte
	typ  byte
}

// span: where the content of one outer entry lies in the archive.
type span struct{ start, end int64 }

func hexOf(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func blobOf(b []byte) outerFile { return outerFile{name: "blobs/sha256/" + hexOf(b), body: b} }

func gzipped(t testing.TB, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if _, err := zw.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// layerOf is a gzip layer made of the headers hs (hdr of tarindex_test).
func layerOf(t *testing.T, hs ...hdr) []byte {
	t.Helper()
	return gzipped(t, mkTar(t, hs...))
}

// noise: n bytes that gzip does not shrink.
func noise(n int) []byte {
	b := make([]byte, n)
	x := uint32(2463534242)
	for i := range b {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		b[i] = byte(x)
	}
	return b
}

var testConfig = []byte(`{"architecture":"amd64","os":"linux","rootfs":{"type":"layers"}}`)

func manifestOf(layers ...[]byte) outerFile {
	paths := make([]string, 0, len(layers))
	for _, l := range layers {
		paths = append(paths, "blobs/sha256/"+hexOf(l))
	}
	return manifestRaw(paths...)
}

func manifestRaw(layers ...string) outerFile {
	b, _ := json.Marshal([]map[string]any{{"Config": "blobs/sha256/" + hexOf(testConfig), "RepoTags": []string{"t:1"}, "Layers": layers}})
	return outerFile{name: "manifest.json", body: b}
}

func outerTar(t testing.TB, files ...outerFile) ([]byte, []span) {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	spans := make([]span, 0, len(files))
	for _, f := range files {
		h := &tar.Header{Name: f.name, Typeflag: f.typ, Mode: 0o444, ModTime: time.Unix(1_700_000_000, 0)}
		if h.Typeflag == 0 {
			h.Typeflag, h.Size = tar.TypeReg, int64(len(f.body))
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		start := int64(buf.Len()) // the header block is written
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write(f.body); err != nil {
				t.Fatal(err)
			}
		}
		spans = append(spans, span{start, start + h.Size})
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), spans
}

// saveFiles: the entries of `docker save` for these encoded layers, the
// first layer sent first, the manifest last (spec v0.2 §5.5, measured).
func saveFiles(layers ...[]byte) []outerFile {
	files := []outerFile{{name: "blobs/", typ: tar.TypeDir}, {name: "blobs/sha256/", typ: tar.TypeDir}}
	for _, l := range layers {
		files = append(files, blobOf(l))
	}
	return append(files, blobOf(testConfig),
		outerFile{name: "index.json", body: []byte(`{"schemaVersion":2,"manifests":[]}`)},
		outerFile{name: "oci-layout", body: []byte(`{"imageLayoutVersion":"1.0.0"}`)},
		manifestOf(layers...))
}

func saveArchive(t testing.TB, layers ...[]byte) []byte {
	t.Helper()
	b, _ := outerTar(t, saveFiles(layers...)...)
	return b
}

func limitsWith(change func(*ImageLimits)) ImageLimits {
	lim := DefaultImageLimits
	if change != nil {
		change(&lim)
	}
	return lim
}

func buildArchive(archive []byte, lim ImageLimits) (*imageIndex, error) {
	var read atomic.Int64
	return buildImageIndex(tctx, bytes.NewReader(archive), lim, &read)
}

func mustBuild(t *testing.T, layers ...[]byte) *imageIndex {
	t.Helper()
	x, err := buildArchive(saveArchive(t, layers...), DefaultImageLimits)
	if err != nil {
		t.Fatalf("buildImageIndex : %v", err)
	}
	return x
}

func listNames(t *testing.T, x *imageIndex, dir string) []string {
	t.Helper()
	entries, _, err := (&imageFS{img: x}).List(tctx, dir)
	if err != nil {
		t.Fatalf("List(%s) : %v", dir, err)
	}
	out := []string{}
	for _, e := range entries {
		out = append(out, e.Name)
	}
	slices.Sort(out)
	return out
}

func statImage(x *imageIndex, p string) (Entry, error) { return (&imageFS{img: x}).Stat(tctx, p) }

func isUnsupported(err error) bool { return errors.Is(err, ErrUnsupportedImage) }

// --- raw tar blocks, for headers archive/tar refuses to write ---

func rawHeader(name string, typ byte, size int64) []byte {
	b := make([]byte, 512)
	copy(b[0:100], name)
	copy(b[100:108], "0000644\x00")
	copy(b[108:116], "0000000\x00")
	copy(b[116:124], "0000000\x00")
	copy(b[124:136], fmt.Sprintf("%011o\x00", size))
	copy(b[136:148], "14531464000\x00")
	b[156] = typ
	copy(b[257:263], "ustar\x00")
	copy(b[263:265], "00")
	for i := 148; i < 156; i++ {
		b[i] = ' '
	}
	sum := 0
	for _, c := range b {
		sum += int(c)
	}
	copy(b[148:156], fmt.Sprintf("%06o\x00 ", sum))
	return b
}

func paxRecord(k, v string) string {
	n := len(k) + len(v) + 3
	for {
		s := strconv.Itoa(n) + " " + k + "=" + v + "\n"
		if len(s) == n {
			return s
		}
		n = len(s)
	}
}

func padBlock(b []byte) []byte {
	if r := len(b) % 512; r != 0 {
		b = append(b, make([]byte, 512-r)...)
	}
	return b
}

// rawEntry: a PAX header of records (if any), then the header and body.
func rawEntry(name string, typ byte, body string, records ...string) []byte {
	var out []byte
	if len(records) > 0 {
		rec := strings.Join(records, "")
		out = append(out, rawHeader("PaxHeaders/"+name, tar.TypeXHeader, int64(len(rec)))...)
		out = append(out, padBlock([]byte(rec))...)
	}
	out = append(out, rawHeader(name, typ, int64(len(body)))...)
	return append(out, padBlock([]byte(body))...)
}

func endOfTar(b []byte) []byte { return append(b, make([]byte, 1024)...) }

// ===================== whiteouts and merge (spec v0.2 §5.5 step 4) =====================

func TestImageIndexWhiteoutFile(t *testing.T) {
	x := mustBuild(t,
		layerOf(t, tdir("etc/"), tfile("etc/a", "A"), tfile("etc/b", "B")),
		layerOf(t, tdir("etc/"), tfile("etc/.wh.a", "")))
	if _, err := statImage(x, "/etc/a"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("/etc/a blanchi : %v", err)
	}
	if got := listNames(t, x, "/etc"); !slices.Equal(got, []string{"b"}) {
		t.Errorf("List(/etc) = %v, attendu [b] (jamais .wh.a)", got)
	}
	if _, err := statImage(x, "/etc/.wh.a"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("/etc/.wh.a visible : %v", err)
	}
}

func TestImageIndexOpaqueDir(t *testing.T) {
	x := mustBuild(t,
		layerOf(t, tdir("d/"), tfile("d/x", "X"), tdir("d/sub/"), tfile("d/sub/z", "Z")),
		layerOf(t, tdir("d/"), tfile("d/.wh..wh..opq", ""), tfile("d/y", "Y")))
	if got := listNames(t, x, "/d"); !slices.Equal(got, []string{"y"}) {
		t.Errorf("List(/d) = %v, attendu [y]", got)
	}
	for _, p := range []string{"/d/x", "/d/sub/z", "/d/sub"} {
		if _, err := statImage(x, p); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s sous un dossier opaque : %v", p, err)
		}
	}
}

// Review Focus 1: a file whited out in a layer and recreated in a higher one
// shows the recreated version, not the absence.
func TestImageIndexWhiteoutThenRecreate(t *testing.T) {
	l1 := layerOf(t, tfile("f", "v1"))
	l2 := layerOf(t, tfile(".wh.f", ""))
	l3 := layerOf(t, tdir("etc/"), tfile("f", "v3-recreated"))
	x := mustBuild(t, l1, l2, l3)
	e, err := statImage(x, "/f")
	if err != nil || e.Type != TypeFile || e.Size != int64(len("v3-recreated")) {
		t.Fatalf("Stat(/f) = %+v, %v", e, err)
	}
	if got, want := x.origin["/f"], (layerRef{Digest: hexOf(l3), Rank: 1}); got != want {
		t.Errorf("origine de /f = %+v, attendu %+v (couche 3, rang 1)", got, want)
	}
}

// Names are cleaned into the index (CleanPath, as spec §5.2): the index is a
// table in memory, nothing is ever created on the disk.
func TestImageIndexDotDotNames(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(work) // a write by name would land under root
	x := mustBuild(t, layerOf(t, tfile("../../etc/shadow", "root:!:0"), tfile("/abs/x", "X"), tfile("./rel/../y", "Y")))
	for _, p := range []string{"/etc/shadow", "/abs/x", "/y"} {
		if e, err := statImage(x, p); err != nil || e.Type != TypeFile {
			t.Errorf("Stat(%s) = %+v, %v", p, e, err)
		}
	}
	var found []string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if p != root && p != filepath.Join(root, "a") && p != work {
			found = append(found, p)
		}
		return nil
	})
	if len(found) > 0 {
		t.Errorf("fichiers créés sur le disque : %v", found)
	}
}

func TestImageIndexSymlinkEscape(t *testing.T) {
	x := mustBuild(t, layerOf(t, tdir("etc/"), tfile("etc/passwd", "root:x:0:0:image"),
		tlink("x", "../../../../etc/passwd"), tlink("y", "/../../etc/passwd"), tlink("z", "../../../../host-only")))
	for _, p := range []string{"/x", "/y"} {
		e, err := statImage(x, p)
		if err != nil || e.Name != "passwd" || e.Size != int64(len("root:x:0:0:image")) {
			t.Errorf("Stat(%s) = %+v, %v : attendu /etc/passwd de l'image", p, e, err)
		}
	}
	if _, err := statImage(x, "/z"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("/z hors de l'index : %v, attendu not_found", err)
	}
	// Open resolves the same way: the content of /etc/passwd of the image.
	archive := saveArchive(t, layerOf(t, tdir("etc/"), tfile("etc/passwd", "root:x:0:0:image"), tlink("x", "../../../../etc/passwd")))
	fsys := readyImage(t, NewViews(imageDocker(archive), Options{}), imgID)
	if got, err := readImage(t, fsys, "/x"); err != nil || got != "root:x:0:0:image" {
		t.Errorf("Open(/x) = %q, %v", got, err)
	}
}

func TestImageIndexHardlinkAcrossLayers(t *testing.T) {
	l1 := layerOf(t, tdir("bin/"), tfile("bin/a", "AAA-layer-1"))
	l2 := layerOf(t, tdir("bin/"), thard("bin/b", "bin/a"))
	archive := saveArchive(t, l1, l2)
	x, err := buildArchive(archive, DefaultImageLimits)
	if err != nil {
		t.Fatal(err)
	}
	if x.origin["/bin/b"] != x.origin["/bin/a"] || x.origin["/bin/a"].Digest != hexOf(l1) {
		t.Errorf("origines : a=%+v b=%+v", x.origin["/bin/a"], x.origin["/bin/b"])
	}
	fsys := readyImage(t, NewViews(imageDocker(archive), Options{}), imgID)
	if got, err := readImage(t, fsys, "/bin/b"); err != nil || got != "AAA-layer-1" {
		t.Errorf("Open(/bin/b) = %q, %v", got, err)
	}
}

func TestImageIndexDirReplacedByFile(t *testing.T) {
	x := mustBuild(t,
		layerOf(t, tdir("d/"), tfile("d/x", "X"), tdir("d/sub/"), tfile("d/sub/y", "Y")),
		layerOf(t, tfile("d", "now a file")))
	if e, err := statImage(x, "/d"); err != nil || e.Type != TypeFile {
		t.Errorf("Stat(/d) = %+v, %v : attendu un fichier", e, err)
	}
	for _, p := range []string{"/d/x", "/d/sub/y", "/d/sub"} {
		if _, err := statImage(x, p); err == nil {
			t.Errorf("%s encore visible sous un fichier", p)
		}
		if _, ok := x.idx.entries[p]; ok {
			t.Errorf("%s encore dans l'index", p)
		}
	}
	if got := listNames(t, x, "/"); !slices.Equal(got, []string{"d"}) {
		t.Errorf("List(/) = %v, attendu un seul d", got)
	}
	if hits, _ := x.idx.search(tctx, "sub", 10); len(hits) != 0 {
		t.Errorf("recherche : %v", hits)
	}
}

func TestImageIndexDirReplacedBySymlink(t *testing.T) {
	x := mustBuild(t,
		layerOf(t, tdir("d/"), tfile("d/x", "OLD-X"), tdir("etc/"), tfile("etc/x", "ETC-X")),
		layerOf(t, tlink("d", "/etc")))
	e, err := statImage(x, "/d/x")
	if err != nil || e.Size != int64(len("ETC-X")) {
		t.Errorf("Stat(/d/x) = %+v, %v : attendu /etc/x de l'image", e, err)
	}
	if _, ok := x.idx.entries["/d/x"]; ok {
		t.Error("l'ancien /d/x est resté dans l'index")
	}
	if got := listNames(t, x, "/"); !slices.Equal(got, []string{"d", "etc"}) {
		t.Errorf("List(/) = %v", got)
	}
}

// ".wh.", ".wh.." and ".wh..." name nothing: neither /d nor its parent go.
func TestImageIndexWhiteoutDotNames(t *testing.T) {
	x := mustBuild(t,
		layerOf(t, tdir("p/"), tdir("p/d/"), tfile("p/d/keep", "K"), tfile("p/sibling", "S")),
		layerOf(t, tdir("p/d/"), tfile("p/d/.wh.", ""), tfile("p/d/.wh..", ""), tfile("p/d/.wh...", ""), tfile("p/d/.wh..wh.plnk", "")))
	if got := listNames(t, x, "/p/d"); !slices.Equal(got, []string{"keep"}) {
		t.Errorf("List(/p/d) = %v, attendu [keep]", got)
	}
	if got := listNames(t, x, "/p"); !slices.Equal(got, []string{"d", "sibling"}) {
		t.Errorf("List(/p) = %v", got)
	}
}

// The last occurrence wins, by its rank, at the index and at the extraction.
func TestImageIndexDuplicateInLayer(t *testing.T) {
	l1 := layerOf(t, tfile("f", "v1"), tfile("g", "G"), tfile("f", "version-2"))
	archive := saveArchive(t, l1)
	x, err := buildArchive(archive, DefaultImageLimits)
	if err != nil {
		t.Fatal(err)
	}
	if e, err := statImage(x, "/f"); err != nil || e.Size != int64(len("version-2")) {
		t.Errorf("Stat(/f) = %+v, %v", e, err)
	}
	if got := x.origin["/f"]; got.Rank != 2 {
		t.Errorf("rang de /f = %d, attendu 2", got.Rank)
	}
	if got := listNames(t, x, "/"); !slices.Equal(got, []string{"f", "g"}) {
		t.Errorf("List(/) = %v", got)
	}
	fsys := readyImage(t, NewViews(imageDocker(archive), Options{}), imgID)
	if got, err := readImage(t, fsys, "/f"); err != nil || got != "version-2" {
		t.Errorf("Open(/f) = %q, %v", got, err)
	}
}

func TestImageIndexProcSysIgnored(t *testing.T) {
	x := mustBuild(t, layerOf(t, tdir("proc/"), tfile("proc/1/environ", "X"), tdir("sys/"), tfile("sys/k", "K")))
	for _, d := range []string{"/proc", "/sys"} {
		if got := listNames(t, x, d); len(got) != 0 {
			t.Errorf("List(%s) = %v", d, got)
		}
	}
}

// ===================== format (spec v0.2 §5.5 steps 2, 3) =====================

func TestImageIndexZstd(t *testing.T) {
	zstd := append([]byte{0x28, 0xb5, 0x2f, 0xfd}, bytes.Repeat([]byte{7}, 600)...)
	if _, err := buildArchive(saveArchive(t, zstd), DefaultImageLimits); !isUnsupported(err) {
		t.Errorf("couche zstd : %v, attendu ErrUnsupportedImage", err)
	}
}

// streamLayer writes a gzip layer through tar and gzip at once: the
// decompressed bytes are never all in memory (the tests run under -race in
// a VM of a few GiB).
func streamLayer(t *testing.T, write func(tw *tar.Writer)) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	tw := tar.NewWriter(zw)
	write(tw)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestImageIndexGzipBomb(t *testing.T) {
	layer := streamLayer(t, func(tw *tar.Writer) {
		tw.WriteHeader(&tar.Header{Name: "bomb", Typeflag: tar.TypeReg, Mode: 0o644, Size: 64 << 20})
		zeros := make([]byte, 1<<20)
		for range 64 {
			tw.Write(zeros)
		}
		tw.WriteHeader(&tar.Header{Name: "after", Typeflag: tar.TypeReg, Mode: 0o644})
	})
	t.Logf("couche : %d octets compressés, %d décompressés", len(layer), 64<<20)
	_, err := buildArchive(saveArchive(t, layer), limitsWith(func(l *ImageLimits) { l.MaxUncompressed = 32 << 20 }))
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("bombe gzip : %v, attendu ErrTooLarge", err)
	}
}

func TestImageIndexBlobDigestMismatch(t *testing.T) {
	good := layerOf(t, tfile("f", "F"))
	evil := layerOf(t, tfile("f", "EVIL"))
	files := saveFiles(good)
	for i := range files {
		if bytes.Equal(files[i].body, good) {
			files[i].body = evil // the name of good, the content of evil
		}
	}
	archive, _ := outerTar(t, files...)
	if _, err := buildArchive(archive, DefaultImageLimits); !isUnsupported(err) {
		t.Errorf("couche dont l'empreinte est fausse : %v", err)
	}
	// Any blob, not only a layer: the config too.
	files = saveFiles(good)
	for i := range files {
		if bytes.Equal(files[i].body, testConfig) {
			files[i].body = []byte(`{"os":"evil"}`)
		}
	}
	archive, _ = outerTar(t, files...)
	if _, err := buildArchive(archive, DefaultImageLimits); !isUnsupported(err) {
		t.Errorf("config dont l'empreinte est fausse : %v", err)
	}
}

func replaceManifest(files []outerFile, m outerFile) []outerFile {
	out := slices.Clone(files)
	for i := range out {
		if out[i].name == "manifest.json" {
			out[i] = m
		}
	}
	return out
}

func TestImageIndexManifestTwoEntries(t *testing.T) {
	l := layerOf(t, tfile("f", "F"))
	p := "blobs/sha256/" + hexOf(l)
	b, _ := json.Marshal([]map[string]any{{"Config": "c", "Layers": []string{p}}, {"Config": "c", "Layers": []string{p}}})
	archive, _ := outerTar(t, replaceManifest(saveFiles(l), outerFile{name: "manifest.json", body: b})...)
	if _, err := buildArchive(archive, DefaultImageLimits); !isUnsupported(err) {
		t.Errorf("manifeste à deux entrées : %v", err)
	}
}

func TestImageIndexMissingLayer(t *testing.T) {
	l := layerOf(t, tfile("f", "F"))
	absent := "blobs/sha256/" + strings.Repeat("0", 64)
	archive, _ := outerTar(t, replaceManifest(saveFiles(l), manifestRaw("blobs/sha256/"+hexOf(l), absent))...)
	if _, err := buildArchive(archive, DefaultImageLimits); !isUnsupported(err) {
		t.Errorf("couche absente : %v", err)
	}
	// No manifest at all.
	files := slices.DeleteFunc(saveFiles(l), func(f outerFile) bool { return f.name == "manifest.json" })
	archive, _ = outerTar(t, files...)
	if _, err := buildArchive(archive, DefaultImageLimits); !isUnsupported(err) {
		t.Errorf("sans manifeste : %v", err)
	}
	// The config blob is not a layer.
	archive, _ = outerTar(t, replaceManifest(saveFiles(l), manifestRaw("blobs/sha256/"+hexOf(testConfig)))...)
	if _, err := buildArchive(archive, DefaultImageLimits); !isUnsupported(err) {
		t.Errorf("config donnée pour couche : %v", err)
	}
}

func TestImageIndexBadLayerPath(t *testing.T) {
	l := layerOf(t, tfile("f", "F"))
	h := hexOf(l)
	for _, p := range []string{"../x", "layer.tar", h + "/layer.tar", "blobs/sha256/../../" + h, "/blobs/sha256/" + h,
		"blobs/sha256/" + strings.ToUpper(h), "blobs/sha512/" + h, "blobs/sha256/" + h + "/"} {
		archive, _ := outerTar(t, replaceManifest(saveFiles(l), manifestRaw(p))...)
		if _, err := buildArchive(archive, DefaultImageLimits); !isUnsupported(err) {
			t.Errorf("couche %q : %v", p, err)
		}
	}
	// A manifest over 64 KiB.
	big := outerFile{name: "manifest.json", body: append(manifestOf(l).body, bytes.Repeat([]byte(" "), 64<<10)...)}
	archive, _ := outerTar(t, replaceManifest(saveFiles(l), big)...)
	if _, err := buildArchive(archive, DefaultImageLimits); !isUnsupported(err) {
		t.Errorf("manifeste de plus de 64 Kio : %v", err)
	}
}

func TestImageIndexEntriesCap(t *testing.T) {
	hs := []hdr{}
	for i := range 11 {
		hs = append(hs, tfile(fmt.Sprintf("f%d", i), "x"))
	}
	lim := limitsWith(func(l *ImageLimits) { l.MaxEntries = 10 })
	if _, err := buildArchive(saveArchive(t, layerOf(t, hs...)), lim); !errors.Is(err, ErrTooLarge) {
		t.Errorf("11 entrées pour 10 : %v", err)
	}
	if _, err := buildArchive(saveArchive(t, layerOf(t, hs[:10]...)), lim); err != nil {
		t.Errorf("10 entrées pour 10 : %v", err)
	}
	// Every layer counts, before the merge.
	if _, err := buildArchive(saveArchive(t, layerOf(t, hs[:6]...), layerOf(t, hs[:5]...)), lim); !errors.Is(err, ErrTooLarge) {
		t.Errorf("6 + 5 entrées pour 10 : %v", err)
	}
}

func allocDuring(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// archive/tar caps one special header at 1 MiB (spec v0.2 §5.5): a PAX
// header of 4 MiB is refused after at most 1 MiB + 1 byte read in memory.
func TestImageIndexHugePAXHeader(t *testing.T) {
	raw := endOfTar(rawEntry("f", tar.TypeReg, "x", paxRecord("comment", strings.Repeat("a", 4<<20))))
	archive := saveArchive(t, gzipped(t, raw))
	var err error
	alloc := allocDuring(func() { _, err = buildArchive(archive, DefaultImageLimits) })
	if !isUnsupported(err) {
		t.Errorf("en-tête PAX de 4 Mio : %v, attendu ErrUnsupportedImage", err)
	}
	if alloc >= 8<<20 {
		t.Errorf("%d octets alloués, attendu moins de 8 Mio", alloc)
	}
}

// paxChain streams, as the content of one blob of the outer archive, a gzip
// layer of n PAX headers of 900 KiB chained before one entry: one single
// Next() of archive/tar reads them all.
func paxChain(t *testing.T, n int) (io.Reader, func()) {
	pr, pw := io.Pipe()
	go func() {
		zw, _ := gzip.NewWriterLevel(pw, gzip.BestSpeed)
		rec := paxRecord("comment", strings.Repeat("a", 900<<10))
		block := append(rawHeader("PaxHeaders/x", tar.TypeXHeader, int64(len(rec))), padBlock([]byte(rec))...)
		for range n {
			if _, err := zw.Write(block); err != nil {
				return
			}
		}
		zw.Write(endOfTar(rawEntry("f", tar.TypeReg, "x")))
		zw.Close()
		pw.Close()
	}()
	head := rawHeader("blobs/sha256/"+strings.Repeat("1", 64), tar.TypeReg, 4<<30)
	return io.MultiReader(bytes.NewReader(head), pr), func() { pr.CloseWithError(errors.New("test over")) }
}

func TestImageIndexPAXChain(t *testing.T) {
	r, stop := paxChain(t, 1000)
	var read atomic.Int64
	_, err := buildImageIndex(tctx, r, limitsWith(func(l *ImageLimits) { l.MaxUncompressed = 32 << 20 }), &read)
	stop()
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("chaîne PAX sous 32 Mio décompressés : %v, attendu ErrTooLarge", err)
	}

	r, stop = paxChain(t, 1000)
	defer stop()
	ctx, cancel := context.WithCancel(tctx)
	defer cancel()
	time.AfterFunc(50*time.Millisecond, cancel)
	start := time.Now()
	_, err = buildImageIndex(ctx, r, DefaultImageLimits, &read)
	if d := time.Since(start); d > 200*time.Millisecond {
		t.Errorf("contexte annulé à 50 ms, retour à %v", d)
	}
	if err == nil {
		t.Error("chaîne PAX annulée : pas d'erreur")
	}
}

// An entry declared sparse (PAX GNU.sparse.*) of logical size 2^60 holds a
// few physical bytes: the index never reads the content of an entry, Next()
// skips the physical bytes only.
func TestImageIndexSparseBomb(t *testing.T) {
	raw := endOfTar(append(rawEntry("bomb", tar.TypeReg, "hello",
		paxRecord("GNU.sparse.map", "0,5"), paxRecord("GNU.sparse.numblocks", "1"),
		paxRecord("GNU.sparse.size", strconv.FormatInt(1<<60, 10))),
		rawEntry("after", tar.TypeReg, "A")...))
	archive := saveArchive(t, gzipped(t, raw))
	type result struct {
		x   *imageIndex
		err error
	}
	done := make(chan result, 1)
	start := time.Now()
	go func() {
		x, err := buildArchive(archive, limitsWith(func(l *ImageLimits) { l.MaxDuration = 2 * time.Second }))
		done <- result{x, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("buildImageIndex : %v (après %v)", r.err, time.Since(start))
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("index en %v, attendu moins d'1 s", d)
		}
		if r.x.uncompressed != int64(len(raw)) {
			t.Errorf("octets décompressés : %d, attendu %d (les octets physiques)", r.x.uncompressed, len(raw))
		}
		if e, err := statImage(r.x, "/bomb"); err != nil || e.Size != 1<<60 {
			t.Errorf("Stat(/bomb) = %+v, %v", e, err)
		}
		if _, err := statImage(r.x, "/after"); err != nil {
			t.Errorf("Stat(/after) : %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("index jamais fini en 1 s : le contenu de l'entrée creuse est lu")
	}
}

// cancelAfter cancels when n bytes have been read through it.
type cancelAfter struct {
	r      io.Reader
	n      int64
	cancel func()
	at     atomic.Int64 // UnixNano of the cancel
}

func (c *cancelAfter) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	if c.n -= int64(n); c.n <= 0 && c.at.Load() == 0 {
		c.at.Store(time.Now().UnixNano())
		c.cancel()
	}
	return n, err
}

func TestImageIndexContextChecked(t *testing.T) {
	layer := streamLayer(t, func(tw *tar.Writer) {
		for i := range 100_000 {
			tw.WriteHeader(&tar.Header{Name: fmt.Sprintf("d%d/f%d", i%100, i), Typeflag: tar.TypeReg, Mode: 0o644})
		}
	})
	archive, spans := outerTar(t, saveFiles(layer)...)
	ctx, cancel := context.WithCancel(tctx)
	defer cancel()
	mid := spans[2].start + int64(len(layer))/2 // the middle of the layer
	r := &cancelAfter{r: bytes.NewReader(archive), n: mid, cancel: cancel}
	var read atomic.Int64
	_, err := buildImageIndex(ctx, r, DefaultImageLimits, &read)
	if err == nil {
		t.Fatal("annulé au milieu de la couche : pas d'erreur")
	}
	if d := time.Since(time.Unix(0, r.at.Load())); r.at.Load() == 0 || d > 100*time.Millisecond {
		t.Errorf("retour %v après l'annulation, attendu moins de 100 ms", d)
	}
}

func TestImageIndexNameCap(t *testing.T) {
	name := func(n int) string { return strings.Repeat("n/", (n-1)/2) + strings.Repeat("x", n-2*((n-1)/2)) }
	if len(name(4097)) != 4097 || len(name(4096)) != 4096 {
		t.Fatal("aide name")
	}
	for _, h := range []hdr{tfile(name(4097), "x"), tlink("l", name(4097)), thard("h", name(4097))} {
		if _, err := buildArchive(saveArchive(t, layerOf(t, h)), DefaultImageLimits); !isUnsupported(err) {
			t.Errorf("nom ou cible de 4 097 octets (%c) : %v", h.typ, err)
		}
	}
	for _, h := range []hdr{tfile(name(4096), "x"), tlink("l", name(4096))} {
		if _, err := buildArchive(saveArchive(t, layerOf(t, h)), DefaultImageLimits); err != nil {
			t.Errorf("nom ou cible de 4 096 octets (%c) : %v", h.typ, err)
		}
	}
}

func TestImageIndexNameBytesCap(t *testing.T) {
	hs := []hdr{}
	for i := range 300 {
		hs = append(hs, tfile(fmt.Sprintf("%04d-%s", i, strings.Repeat("n", 3500)), ""))
	}
	lim := limitsWith(func(l *ImageLimits) { l.MaxNameBytes = 1 << 20 })
	if _, err := buildArchive(saveArchive(t, layerOf(t, hs...)), lim); !errors.Is(err, ErrTooLarge) {
		t.Errorf("1,05 Mio de noms pour 1 Mio : %v", err)
	}
	if _, err := buildArchive(saveArchive(t, layerOf(t, hs[:250]...)), lim); err != nil {
		t.Errorf("0,88 Mio de noms pour 1 Mio : %v", err)
	}
}

// The compact form of a header keeps no PAXRecords, Xattrs, Uname nor Gname.
func TestImageIndexDropsPAXAndXattrs(t *testing.T) {
	h := &tar.Header{Name: "/etc/f", Typeflag: tar.TypeReg, Size: 5, Mode: 0o4755, Uid: 7, Gid: 8,
		ModTime: time.Unix(1_700_000_000, 0), Uname: strings.Repeat("u", 1000), Gname: strings.Repeat("g", 1000),
		PAXRecords: map[string]string{"comment": strings.Repeat("c", 100<<10), "SCHILY.xattr.user.big": strings.Repeat("x", 1000)},
		Xattrs:     map[string]string{"user.big": strings.Repeat("x", 1000)}}
	got := compact(h, 3)
	want := layerEntry{Name: "/etc/f", Typeflag: tar.TypeReg, Size: 5, Mode: 0o4755, UID: 7, GID: 8, ModTime: h.ModTime, Rank: 3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("compact = %+v, attendu %+v", got, want)
	}
	typ := reflect.TypeFor[layerEntry]()
	for i := range typ.NumField() {
		f := typ.Field(i)
		switch f.Type.Kind() {
		case reflect.Map, reflect.Slice, reflect.Pointer, reflect.Interface:
			t.Errorf("layerEntry.%s (%s) : seuls des champs scalaires", f.Name, f.Type)
		}
		if slices.Contains([]string{"PAXRecords", "Xattrs", "Uname", "Gname"}, f.Name) {
			t.Errorf("layerEntry garde %s", f.Name)
		}
	}
	// The same header in a layer is indexed.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	h.Name, h.Xattrs = "etc/f", nil
	if err := tw.WriteHeader(h); err != nil {
		t.Fatal(err)
	}
	tw.Write([]byte("hello"))
	tw.Close()
	x, err := buildArchive(saveArchive(t, gzipped(t, buf.Bytes())), DefaultImageLimits)
	if err != nil {
		t.Fatal(err)
	}
	if e, err := statImage(x, "/etc/f"); err != nil || e.Size != 5 || e.UID != 7 {
		t.Errorf("Stat(/etc/f) = %+v, %v", e, err)
	}
}

func TestImageIndexUncompressedLayer(t *testing.T) {
	raw := mkTar(t, tdir("etc/"), tfile("etc/raw", "RAW"))
	x := mustBuild(t, raw)
	if e, err := statImage(x, "/etc/raw"); err != nil || e.Size != 3 {
		t.Errorf("couche non compressée : %+v, %v", e, err)
	}
	// 512 bytes with "ustar" at 257 and a wrong checksum: an ordinary blob,
	// read and thrown away.
	fake := rawHeader("x", tar.TypeReg, 0)
	fake[148] ^= 1
	gz := layerOf(t, tfile("g", "G"))
	files := saveFiles(gz)
	files = slices.Insert(files, 2, blobOf(fake))
	archive, _ := outerTar(t, files...)
	if _, err := buildArchive(archive, DefaultImageLimits); err != nil {
		t.Errorf("blob ordinaire à « ustar » faux : %v", err)
	}
	archive, _ = outerTar(t, replaceManifest(files, manifestOf(gz, fake))...)
	if _, err := buildArchive(archive, DefaultImageLimits); !isUnsupported(err) {
		t.Errorf("blob à somme fausse donné pour couche : %v", err)
	}
}

// An empty layer exported uncompressed (two zero blocks) is a layer.
func TestImageIndexEmptyRawLayer(t *testing.T) {
	x := mustBuild(t, layerOf(t, tfile("f", "F")), make([]byte, 1024))
	if _, err := statImage(x, "/f"); err != nil {
		t.Error(err)
	}
}

// An entry of the outer archive declared sparse is refused: its holes would
// be hashed without reading the stream.
func TestImageIndexOuterSparseRefused(t *testing.T) {
	archive := saveArchive(t, layerOf(t, tfile("f", "F")))
	archive = archive[:len(archive)-1024] // without the end of the tar
	sparse := rawEntry("blobs/sha256/"+strings.Repeat("5", 64), tar.TypeReg, "hello",
		paxRecord("GNU.sparse.map", "0,5"), paxRecord("GNU.sparse.numblocks", "1"),
		paxRecord("GNU.sparse.size", strconv.FormatInt(1<<40, 10)))
	done := make(chan error, 1)
	go func() {
		_, err := buildArchive(endOfTar(append(archive, sparse...)), DefaultImageLimits)
		done <- err
	}()
	select {
	case err := <-done:
		if !isUnsupported(err) {
			t.Errorf("blob creux : %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blob creux jamais refusé en 1 s : ses trous sont hachés")
	}
}

func TestImageIndexOuterBytesCap(t *testing.T) {
	archive := saveArchive(t, layerOf(t, tfile("f", string(noise(64<<10)))))
	lim := limitsWith(func(l *ImageLimits) { l.MaxBytes = 16 << 10 })
	if _, err := buildArchive(archive, lim); !errors.Is(err, ErrTooLarge) {
		t.Errorf("16 Kio lus au plus : %v", err)
	}
}

func TestImageIndexProgressCounter(t *testing.T) {
	archive := saveArchive(t, layerOf(t, tfile("f", "F")))
	var read atomic.Int64
	if _, err := buildImageIndex(tctx, bytes.NewReader(archive), DefaultImageLimits, &read); err != nil {
		t.Fatal(err)
	}
	if read.Load() == 0 || read.Load() > int64(len(archive)) {
		t.Errorf("octets lus : %d sur %d", read.Load(), len(archive))
	}
}

// ===================== extraction (spec v0.2 §5.5 "Lecture d'un fichier") =====================

func imageDocker(archive []byte) *dockerx.Fake {
	return &dockerx.Fake{
		Images:     map[string]dockerx.ImageInfo{imgID: {ID: imgID, Size: int64(len(archive))}},
		ImageSaves: map[string][]byte{imgID: archive},
		NoWrites:   true,
	}
}

// readyImage polls ForImage until the index is built.
func readyImage(t *testing.T, v *Views, id string) ImageFS {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(2 * time.Millisecond) {
		fsys, err := v.ForImage(tctx, id)
		if !errors.Is(err, ErrIndexing) {
			if err != nil {
				t.Fatalf("ForImage(%s) : %v", id, err)
			}
			return fsys
		}
		if time.Now().After(deadline) {
			t.Fatalf("index d'image jamais prêt : %v", err)
		}
	}
}

// readImage polls Open while the extraction runs (as the UI does).
func readImage(t *testing.T, fsys ImageFS, p string) (string, error) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(2 * time.Millisecond) {
		rc, err := fsys.Open(tctx, p)
		if errors.Is(err, ErrIndexing) {
			if time.Now().After(deadline) {
				t.Fatalf("extraction de %s jamais finie", p)
			}
			continue
		}
		if err != nil {
			return "", err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		return string(b), err
	}
}

// saveHook passes the first export of each image (the index build) and
// gives every later one (an extraction) to hook.
type saveHook struct {
	*dockerx.Fake
	mu    sync.Mutex
	calls map[string]int
	hook  func(ctx context.Context, id string, rc io.ReadCloser) io.ReadCloser
}

func newSaveHook(f *dockerx.Fake, hook func(ctx context.Context, id string, rc io.ReadCloser) io.ReadCloser) *saveHook {
	return &saveHook{Fake: f, calls: map[string]int{}, hook: hook}
}

func (s *saveHook) ImageSave(ctx context.Context, id string) (io.ReadCloser, error) {
	s.mu.Lock()
	s.calls[id]++
	n := s.calls[id]
	s.mu.Unlock()
	rc, err := s.Fake.ImageSave(ctx, id)
	if err != nil || n == 1 || s.hook == nil {
		return rc, err
	}
	return s.hook(ctx, id, rc), nil
}

func (s *saveHook) callsOf(id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[id]
}

// held holds a reader until open is closed or ctx ends; it counts closes.
type held struct {
	ctx    context.Context
	open   chan struct{}
	rc     io.ReadCloser
	closes *atomic.Int32
}

func (h *held) Read(b []byte) (int, error) {
	select {
	case <-h.open:
		return h.rc.Read(b)
	case <-h.ctx.Done():
		return 0, h.ctx.Err()
	}
}

func (h *held) Close() error { h.closes.Add(1); return h.rc.Close() }

func TestImageOpenNotRegular(t *testing.T) {
	archive := saveArchive(t, layerOf(t, tdir("dev/"), tnode("dev/fifo", tar.TypeFifo), tnode("dev/null", tar.TypeChar),
		tnode("dev/sda", tar.TypeBlock), thard("dev/orphan", "missing"), tdir("dir/")))
	f := imageDocker(archive)
	fsys := readyImage(t, NewViews(f, Options{}), imgID)
	for _, p := range []string{"/dev/fifo", "/dev/null", "/dev/sda", "/dev/orphan", "/dir"} {
		if _, err := fsys.Open(tctx, p); !errors.Is(err, ErrNotRegular) {
			t.Errorf("Open(%s) : %v, attendu ErrNotRegular", p, err)
		}
	}
	if n := f.ImageSaveCalls.Load(); n != 1 {
		t.Errorf("%d exports, attendu 1 (l'index seul)", n)
	}
	if _, err := fsys.Open(tctx, "/absent"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Open(/absent) : %v", err)
	}
}

// countingRC counts the bytes read of an export.
type countingRC struct {
	rc io.ReadCloser
	n  *atomic.Int64
}

func (c *countingRC) Read(b []byte) (int, error) {
	n, err := c.rc.Read(b)
	c.n.Add(int64(n))
	return n, err
}

func (c *countingRC) Close() error { return c.rc.Close() }

func TestImageOpenVerifiesBlob(t *testing.T) {
	l1 := layerOf(t, tfile("first", "FIRST"), tfile("pad", strings.Repeat("p", 20<<10)))
	l2 := layerOf(t, tfile("second", string(noise(256<<10))))
	files := saveFiles(l1, l2)
	archive, spans := outerTar(t, files...)
	var read atomic.Int64
	f := imageDocker(archive)
	h := newSaveHook(f, func(_ context.Context, _ string, rc io.ReadCloser) io.ReadCloser {
		return &countingRC{rc: rc, n: &read}
	})
	fsys := readyImage(t, NewViews(h, Options{}), imgID)
	if got, err := readImage(t, fsys, "/first"); err != nil || got != "FIRST" {
		t.Fatalf("Open(/first) = %q, %v", got, err)
	}
	l1Span, l2Span := spans[2], spans[3]
	if read.Load() < l1Span.end {
		t.Errorf("%d octets lus, attendu au moins %d (la couche jusqu'à sa fin, pour son empreinte)", read.Load(), l1Span.end)
	}
	if read.Load() >= l2Span.start {
		t.Errorf("%d octets lus : le blob suivant (à %d) a été lu", read.Load(), l2Span.start)
	}
}

func TestImageOpenDigestMismatch(t *testing.T) {
	good := layerOf(t, tfile("f", "GOOD-CONTENT"))
	evil := layerOf(t, tfile("f", "EVIL-CONTENT"))
	archive := saveArchive(t, good)
	files := saveFiles(good)
	for i := range files {
		if bytes.Equal(files[i].body, good) {
			files[i].body = evil
		}
	}
	altered, _ := outerTar(t, files...)
	h := newSaveHook(imageDocker(archive), func(_ context.Context, _ string, rc io.ReadCloser) io.ReadCloser {
		rc.Close()
		return io.NopCloser(bytes.NewReader(altered))
	})
	fsys := readyImage(t, NewViews(h, Options{}), imgID)
	got, err := readImage(t, fsys, "/f")
	if !isUnsupported(err) || got != "" {
		t.Errorf("couche altérée entre l'index et l'extraction : %q, %v", got, err)
	}
}

// imagesDocker: n images, image i holding /f = "content-i".
func imagesDocker(t *testing.T, n int) (*dockerx.Fake, []string) {
	f := &dockerx.Fake{Images: map[string]dockerx.ImageInfo{}, ImageSaves: map[string][]byte{}, NoWrites: true}
	ids := []string{}
	for i := range n {
		id := fmt.Sprintf("sha256:%064x", 0xfee0+i)
		a := saveArchive(t, layerOf(t, tfile("f", fmt.Sprintf("content-%d", i))))
		f.Images[id] = dockerx.ImageInfo{ID: id, Size: int64(len(a))}
		f.ImageSaves[id] = a
		ids = append(ids, id)
	}
	return f, ids
}

func TestImageExtractLimits(t *testing.T) {
	t.Run("durée", func(t *testing.T) {
		never := make(chan struct{})
		var closes atomic.Int32
		h := newSaveHook(imageDocker(saveArchive(t, layerOf(t, tfile("f", "F")))), func(ctx context.Context, _ string, rc io.ReadCloser) io.ReadCloser {
			return &held{ctx: ctx, open: never, rc: rc, closes: &closes}
		})
		v := NewViews(h, Options{Extract: ExtractLimits{MaxDuration: 100 * time.Millisecond}})
		if _, err := readImage(t, readyImage(t, v, imgID), "/f"); !errors.Is(err, ErrTooLarge) {
			t.Errorf("extraction au-delà de MaxDuration : %v", err)
		}
	})
	t.Run("décompressé", func(t *testing.T) {
		archive := saveArchive(t, layerOf(t, tfile("big", strings.Repeat("z", 64<<10)), tfile("f", "F")))
		v := NewViews(imageDocker(archive), Options{Extract: ExtractLimits{MaxUncompressed: 4 << 10}})
		if _, err := readImage(t, readyImage(t, v, imgID), "/f"); !errors.Is(err, ErrTooLarge) {
			t.Errorf("extraction au-delà de MaxUncompressed : %v", err)
		}
	})
	t.Run("octets lus", func(t *testing.T) {
		archive := saveArchive(t, layerOf(t, tfile("big", string(noise(64<<10))), tfile("f", "F")))
		v := NewViews(imageDocker(archive), Options{Extract: ExtractLimits{MaxBytes: 2 << 10}})
		if _, err := readImage(t, readyImage(t, v, imgID), "/f"); !errors.Is(err, ErrTooLarge) {
			t.Errorf("extraction au-delà de MaxBytes : %v", err)
		}
	})
	t.Run("contenu de plus de 2 Mio", func(t *testing.T) {
		archive := saveArchive(t, layerOf(t, tfile("f", strings.Repeat("y", 2<<20+1)), tfile("ok", strings.Repeat("y", 2<<20))))
		fsys := readyImage(t, NewViews(imageDocker(archive), Options{}), imgID)
		if _, err := readImage(t, fsys, "/f"); !errors.Is(err, ErrTooLarge) {
			t.Errorf("2 Mio + 1 : %v", err)
		}
		if got, err := readImage(t, fsys, "/ok"); err != nil || len(got) != 2<<20 {
			t.Errorf("2 Mio : %d, %v", len(got), err)
		}
	})
	t.Run("deux au plus pour l'agent", func(t *testing.T) {
		f, ids := imagesDocker(t, 3)
		open := make(chan struct{})
		var closes atomic.Int32
		h := newSaveHook(f, func(ctx context.Context, _ string, rc io.ReadCloser) io.ReadCloser {
			return &held{ctx: ctx, open: open, rc: rc, closes: &closes}
		})
		v := NewViews(h, Options{Extract: ExtractLimits{PerAgent: 2}})
		fss := []ImageFS{}
		for _, id := range ids {
			fss = append(fss, readyImage(t, v, id))
		}
		for i := range 2 {
			if _, err := fss[i].Open(tctx, "/f"); !errors.Is(err, ErrIndexing) {
				t.Fatalf("extraction %d : %v", i, err)
			}
		}
		if _, err := fss[2].Open(tctx, "/f"); !errors.Is(err, ErrIndexing) {
			t.Errorf("troisième extraction : %v, attendu ErrIndexing", err)
		}
		if n := h.callsOf(ids[2]); n != 1 {
			t.Errorf("troisième image : %d exports, attendu 1 (l'index seul : rien de lancé)", n)
		}
		close(open)
		for i, fsys := range fss {
			if got, err := readImage(t, fsys, "/f"); err != nil || got != fmt.Sprintf("content-%d", i) {
				t.Errorf("image %d : %q, %v", i, got, err)
			}
		}
	})
	t.Run("une à la fois par image", func(t *testing.T) {
		open := make(chan struct{})
		var closes atomic.Int32
		h := newSaveHook(imageDocker(saveArchive(t, layerOf(t, tfile("a", "A"), tfile("b", "B")))), func(ctx context.Context, _ string, rc io.ReadCloser) io.ReadCloser {
			return &held{ctx: ctx, open: open, rc: rc, closes: &closes}
		})
		fsys := readyImage(t, NewViews(h, Options{}), imgID)
		fsys.Open(tctx, "/a")
		for deadline := time.Now().Add(2 * time.Second); h.callsOf(imgID) < 2; time.Sleep(time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatal("extraction de /a jamais lancée")
			}
		}
		if _, err := fsys.Open(tctx, "/b"); !errors.Is(err, ErrIndexing) {
			t.Errorf("seconde extraction de l'image : %v", err)
		}
		time.Sleep(20 * time.Millisecond)
		if n := h.callsOf(imgID); n != 2 {
			t.Errorf("%d exports, attendu 2 (l'index, puis /a seul)", n)
		}
		close(open)
		for _, p := range []string{"/a", "/b"} {
			if got, err := readImage(t, fsys, p); err != nil || got != strings.ToUpper(p[1:]) {
				t.Errorf("%s : %q, %v", p, got, err)
			}
		}
	})
}

func TestImageExtractAbandoned(t *testing.T) {
	f, ids := imagesDocker(t, 2)
	never := make(chan struct{})
	var closes atomic.Int32
	h := newSaveHook(f, func(ctx context.Context, _ string, rc io.ReadCloser) io.ReadCloser {
		return &held{ctx: ctx, open: never, rc: rc, closes: &closes}
	})
	v := NewViews(h, Options{Extract: ExtractLimits{PerAgent: 1, Idle: 50 * time.Millisecond}})
	a, b := readyImage(t, v, ids[0]), readyImage(t, v, ids[1])
	if _, err := a.Open(tctx, "/f"); !errors.Is(err, ErrIndexing) {
		t.Fatalf("extraction : %v", err)
	}
	for deadline := time.Now().Add(2 * time.Second); closes.Load() == 0; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("extraction sans client jamais annulée : lecteur jamais fermé")
		}
	}
	// Its slot is free: the next image starts an export.
	for deadline := time.Now().Add(2 * time.Second); h.callsOf(ids[1]) < 2; time.Sleep(5 * time.Millisecond) {
		b.Open(tctx, "/f")
		if time.Now().After(deadline) {
			t.Fatal("le créneau de l'extraction annulée n'est jamais rendu")
		}
	}
}

// A request keeps its extraction alive: polled every 20 ms, it is never
// cancelled by Idle.
func TestImageExtractPolledKeepsGoing(t *testing.T) {
	open := make(chan struct{})
	var closes atomic.Int32
	h := newSaveHook(imageDocker(saveArchive(t, layerOf(t, tfile("f", "F")))), func(ctx context.Context, _ string, rc io.ReadCloser) io.ReadCloser {
		return &held{ctx: ctx, open: open, rc: rc, closes: &closes}
	})
	fsys := readyImage(t, NewViews(h, Options{Extract: ExtractLimits{Idle: 60 * time.Millisecond}}), imgID)
	for range 15 {
		fsys.Open(tctx, "/f")
		time.Sleep(20 * time.Millisecond)
	}
	if closes.Load() != 0 {
		t.Fatal("extraction annulée alors qu'elle est demandée")
	}
	close(open)
	if got, err := readImage(t, fsys, "/f"); err != nil || got != "F" {
		t.Errorf("%q, %v", got, err)
	}
	if n := h.callsOf(imgID); n != 2 {
		t.Errorf("%d exports, attendu 2", n)
	}
}

// The content stays for the next poll: a second read asks nothing of Docker.
func TestImageExtractKept(t *testing.T) {
	f := imageDocker(saveArchive(t, layerOf(t, tfile("f", "F"))))
	fsys := readyImage(t, NewViews(f, Options{}), imgID)
	for range 3 {
		if got, err := readImage(t, fsys, "/f"); err != nil || got != "F" {
			t.Fatalf("%q, %v", got, err)
		}
	}
	if n := f.ImageSaveCalls.Load(); n != 2 {
		t.Errorf("%d exports, attendu 2 (l'index, puis une extraction)", n)
	}
}

// Plan v0.2 tranche 6: no write to Docker, no container created. The fake
// panics on its one write (PutArchive); the Client has no create, start,
// pull, remove nor commit at all.
func TestImageFSNoDockerWrites(t *testing.T) {
	f := imageDocker(saveArchive(t, layerOf(t, tdir("etc/"), tfile("etc/os-release", "NAME=test"))))
	if !f.NoWrites {
		t.Fatal("NoWrites")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("le faux dockerx ne panique pas sur PutArchive")
			}
		}()
		f.PutArchive(tctx, "x", "/", bytes.NewReader(nil))
	}()
	client := reflect.TypeFor[dockerx.Client]()
	for _, m := range []string{"ContainerCreate", "ContainerStart", "ContainerStop", "ContainerRemove", "ImagePull",
		"ImageRemove", "ContainerCommit", "ImageLoad", "ImageTag", "ImageImport", "ImageBuild"} {
		if _, ok := client.MethodByName(m); ok {
			t.Errorf("dockerx.Client a %s", m)
		}
	}
	fsys := readyImage(t, NewViews(f, Options{}), imgID)
	if got := slices.Collect(func(yield func(string) bool) {
		entries, _, _ := fsys.List(tctx, "/etc")
		for _, e := range entries {
			yield(e.Name)
		}
	}); !slices.Equal(got, []string{"os-release"}) {
		t.Errorf("List(/etc) = %v", got)
	}
	if got, err := readImage(t, fsys, "/etc/os-release"); err != nil || got != "NAME=test" {
		t.Errorf("Open : %q, %v", got, err)
	}
}

// The export goes by ID: Docker answering another image is refused.
func TestImageForImageByID(t *testing.T) {
	f := imageDocker(saveArchive(t, layerOf(t, tfile("f", "F"))))
	other := "sha256:" + strings.Repeat("cd", 32)
	f.Images[other] = dockerx.ImageInfo{ID: imgID}
	if _, err := NewViews(f, Options{}).ForImage(tctx, other); err == nil || errors.Is(err, ErrIndexing) {
		t.Errorf("ForImage d'un ID que Docker résout ailleurs : %v", err)
	}
	if _, err := NewViews(f, Options{}).ForImage(tctx, "sha256:"+strings.Repeat("ee", 32)); !errors.Is(err, dockerx.ErrNotFound) {
		t.Errorf("image inconnue : %v", err)
	}
	if n := f.ImageSaveCalls.Load(); n != 0 {
		t.Errorf("%d exports", n)
	}
}

// Errors are cached like the index of a container (spec §5.2 step 4): an
// unsupported image is not exported again on every poll.
func TestImageIndexErrorCached(t *testing.T) {
	f := imageDocker(saveArchive(t, append([]byte{0x28, 0xb5, 0x2f, 0xfd}, make([]byte, 600)...)))
	v := NewViews(f, Options{})
	for range 5 {
		_, err := v.ForImage(tctx, imgID)
		for errors.Is(err, ErrIndexing) {
			time.Sleep(2 * time.Millisecond)
			_, err = v.ForImage(tctx, imgID)
		}
		if !isUnsupported(err) {
			t.Fatalf("ForImage : %v", err)
		}
	}
	if n := f.ImageSaveCalls.Load(); n != 1 {
		t.Errorf("%d exports, attendu 1", n)
	}
}

// SearchImage reads only an index already served, never exports.
func TestImageSearch(t *testing.T) {
	f := imageDocker(saveArchive(t, layerOf(t, tdir("etc/"), tdir("etc/nginx/"), tfile("etc/nginx/nginx.conf", "x"))))
	v := NewViews(f, Options{})
	if hits, _, indexed := v.SearchImage(tctx, imgID, "nginx", 10); indexed || len(hits) != 0 {
		t.Errorf("avant l'index : %v %v", hits, indexed)
	}
	if n := f.ImageSaveCalls.Load(); n != 0 {
		t.Fatalf("la recherche a lancé %d export(s)", n)
	}
	// Built, but never served to the explorer (its request went away).
	if _, err := v.ForImage(tctx, imgID); !errors.Is(err, ErrIndexing) {
		t.Fatalf("ForImage : %v", err)
	}
	for deadline := time.Now().Add(2 * time.Second); ; time.Sleep(time.Millisecond) {
		v.mu.Lock()
		b := v.builds[imageKey(imgID)]
		v.mu.Unlock()
		if finished(b) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("index jamais construit")
		}
	}
	if _, _, indexed := v.SearchImage(tctx, imgID, "nginx", 10); indexed {
		t.Error("index jamais servi : indexed")
	}
	readyImage(t, v, imgID)
	hits, _, indexed := v.SearchImage(tctx, imgID, "nginx.c", 10)
	if !indexed || len(hits) != 1 || hits[0].Path != "/etc/nginx/nginx.conf" {
		t.Errorf("après l'index : %v %v", hits, indexed)
	}
	if _, _, indexed := v.SearchImage(tctx, "sha256:"+strings.Repeat("ee", 32), "nginx", 10); indexed {
		t.Error("autre image : indexed")
	}
}

// A pax global header ('g') of a layer is not a file: never listed.
func TestImageIndexGlobalHeaderSkipped(t *testing.T) {
	layer := streamLayer(t, func(tw *tar.Writer) {
		tw.WriteHeader(&tar.Header{Name: "pax_global_header", Typeflag: tar.TypeXGlobalHeader,
			PAXRecords: map[string]string{"comment": "global"}})
		tw.WriteHeader(&tar.Header{Name: "f", Typeflag: tar.TypeReg, Mode: 0o644})
	})
	x := mustBuild(t, layer)
	if got := listNames(t, x, "/"); !slices.Equal(got, []string{"f"}) {
		t.Errorf("List(/) = %v, attendu [f]", got)
	}
}

// A too_large of an extraction is kept like a content (extractKeep): the
// next poll asks nothing of Docker.
func TestImageExtractTooLargeKept(t *testing.T) {
	f := imageDocker(saveArchive(t, layerOf(t, tfile("big", strings.Repeat("y", MaxExtract+1)))))
	fsys := readyImage(t, NewViews(f, Options{}), imgID)
	for range 3 {
		if _, err := readImage(t, fsys, "/big"); !errors.Is(err, ErrTooLarge) {
			t.Fatalf("Open(/big) : %v", err)
		}
	}
	if n := f.ImageSaveCalls.Load(); n != 2 {
		t.Errorf("%d exports, attendu 2 (l'index, puis une extraction)", n)
	}
}

// failing is an export cut in the middle (Docker restarted, connection lost).
type failing struct{ rc io.ReadCloser }

func (f failing) Read([]byte) (int, error) { return 0, errors.New("connection reset by peer") }
func (f failing) Close() error             { return f.rc.Close() }

// failingAfter is an export cut once release is closed: the extraction
// cannot end before the test lets it.
type failingAfter struct {
	release chan struct{}
	rc      io.ReadCloser
}

func (f failingAfter) Read([]byte) (int, error) {
	<-f.release
	return 0, errors.New("connection reset by peer")
}
func (f failingAfter) Close() error { return f.rc.Close() }

// extractState: whether v holds an extraction of image id at path p, and
// its error once it has ended.
func extractState(v *Views, id, p string) (held, ended bool, err error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	e, ok := v.extracts[id+"\x00"+p]
	if !ok {
		return false, false, nil
	}
	select {
	case <-e.done:
		return true, true, e.err
	default:
		return true, false, nil
	}
}

// waitState polls extractState until cond holds (an observable state, never
// a duration: review of the CI run 40090985257).
func waitState(t *testing.T, v *Views, what string, cond func(held, ended bool, err error) bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !cond(extractState(v, imgID, "/f")); time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%s : jamais atteint", what)
		}
	}
}

// A passing error (Docker cut) is not kept: delivered once to the client
// that still polls, forgotten after Idle when nobody asks for it (review of
// tranche 6: an hour later, the file showed "Docker unavailable"); then a
// new extraction succeeds.
func TestImageExtractTransientErrorForgotten(t *testing.T) {
	var n atomic.Int32 // extractions started
	release := make(chan struct{})
	h := newSaveHook(imageDocker(saveArchive(t, layerOf(t, tfile("f", "F")))), func(_ context.Context, _ string, rc io.ReadCloser) io.ReadCloser {
		switch n.Add(1) {
		case 1:
			return failing{rc} // Docker cut, a client polls
		case 2:
			return failingAfter{release, rc} // cut again, the client gone
		}
		return rc // Docker back
	})
	// Idle far above the few statements between the start of the second
	// extraction and its failure: it fails as a passing error, not as an
	// abandoned one.
	v := NewViews(h, Options{Extract: ExtractLimits{Idle: time.Second}})
	fsys := readyImage(t, v, imgID)

	// 1. Delivered once to the client that polls, and not kept.
	if _, err := readImage(t, fsys, "/f"); !errors.Is(err, dockerx.ErrUnavailable) {
		t.Fatalf("export coupé : %v", err)
	}
	if held, _, _ := extractState(v, imgID, "/f"); held {
		t.Fatal("erreur passagère gardée après sa délivrance")
	}
	// 2. The next request starts a new extraction; it cannot have ended.
	if _, err := fsys.Open(tctx, "/f"); !errors.Is(err, ErrIndexing) {
		t.Fatalf("seconde extraction : %v", err)
	}
	waitState(t, v, "seconde extraction lancée", func(held, ended bool, _ error) bool { return held && !ended && n.Load() == 2 })
	close(release) // it fails, nobody asks for it
	waitState(t, v, "erreur passagère de la seconde extraction", func(held, ended bool, err error) bool {
		return held && ended && errors.Is(err, dockerx.ErrUnavailable)
	})
	// 3. Forgotten after Idle, without any request.
	waitState(t, v, "erreur oubliée", func(held, _ bool, _ error) bool { return !held })
	// 4. Docker is back: a new extraction succeeds.
	if got, err := readImage(t, fsys, "/f"); err != nil || got != "F" {
		t.Errorf("après le retour de Docker : %q, %v", got, err)
	}
	if got := n.Load(); got != 3 {
		t.Errorf("%d extractions, attendu 3", got)
	}
}

// An unsupported_image of an extraction (a layer altered since the index)
// is definitive for an immutable image: kept like a content, the next polls
// ask nothing of Docker.
func TestImageExtractUnsupportedKept(t *testing.T) {
	good := layerOf(t, tfile("f", "GOOD-CONTENT"))
	evil := layerOf(t, tfile("f", "EVIL-CONTENT"))
	files := saveFiles(good)
	for i := range files {
		if bytes.Equal(files[i].body, good) {
			files[i].body = evil
		}
	}
	altered, _ := outerTar(t, files...)
	h := newSaveHook(imageDocker(saveArchive(t, good)), func(_ context.Context, _ string, rc io.ReadCloser) io.ReadCloser {
		rc.Close()
		return io.NopCloser(bytes.NewReader(altered))
	})
	fsys := readyImage(t, NewViews(h, Options{}), imgID)
	for range 3 {
		if _, err := readImage(t, fsys, "/f"); !isUnsupported(err) {
			t.Fatalf("Open(/f) : %v", err)
		}
	}
	if n := h.callsOf(imgID); n != 2 {
		t.Errorf("%d exports, attendu 2 (l'index, puis une extraction)", n)
	}
}
