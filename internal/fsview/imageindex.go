package fsview

// Files of an image, read only (spec v0.2 §5.5): no container is ever
// created. The agent reads the export of `docker save` (GET /images/get, a
// read) as a stream and builds an index of its layers, with the refusals of
// the index of a stopped container (spec §5.2). The layers come from the
// image's author, through any registry: nothing of them is ever written to
// the disk, extracted or run; the content of an entry is never read to build
// the index (Next() only); every header is reduced to a compact form; every
// blob is checked against its name; everything is capped.

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// ImageLimits caps the index of an image (spec v0.2 §5.5 step 5).
type ImageLimits struct {
	MaxBytes        int64 // bytes read of the export
	MaxUncompressed int64 // bytes decompressed, every layer (gzip bomb)
	MaxEntries      int   // headers of every layer, before the merge
	MaxName         int   // bytes of one name or one link target
	MaxNameBytes    int64 // bytes of every name and target kept
	MaxDuration     time.Duration
}

var DefaultImageLimits = ImageLimits{4 << 30, 16 << 30, 1_000_000, 4 << 10, 256 << 20, 5 * time.Minute}

// ExtractLimits caps the reading of one file of an image (spec v0.2 §5.5,
// "Lecture d'un fichier"): the export again, up to the end of its layer.
type ExtractLimits struct {
	MaxBytes        int64 // bytes read of the export
	MaxUncompressed int64
	MaxDuration     time.Duration
	Idle            time.Duration // cancelled when no request asked for it that long
	PerAgent        int           // extractions at once, for the whole agent
}

var DefaultExtractLimits = ExtractLimits{4 << 30, 16 << 30, 2 * time.Minute, 15 * time.Second, 2}

func (l ImageLimits) withDefaults() ImageLimits {
	d := DefaultImageLimits
	if l.MaxBytes <= 0 {
		l.MaxBytes = d.MaxBytes
	}
	if l.MaxUncompressed <= 0 {
		l.MaxUncompressed = d.MaxUncompressed
	}
	if l.MaxEntries <= 0 {
		l.MaxEntries = d.MaxEntries
	}
	if l.MaxName <= 0 {
		l.MaxName = d.MaxName
	}
	if l.MaxNameBytes <= 0 {
		l.MaxNameBytes = d.MaxNameBytes
	}
	if l.MaxDuration <= 0 {
		l.MaxDuration = d.MaxDuration
	}
	return l
}

func (l ExtractLimits) withDefaults() ExtractLimits {
	d := DefaultExtractLimits
	if l.MaxBytes <= 0 {
		l.MaxBytes = d.MaxBytes
	}
	if l.MaxUncompressed <= 0 {
		l.MaxUncompressed = d.MaxUncompressed
	}
	if l.MaxDuration <= 0 {
		l.MaxDuration = d.MaxDuration
	}
	if l.Idle <= 0 {
		l.Idle = d.Idle
	}
	if l.PerAgent <= 0 {
		l.PerAgent = d.PerAgent
	}
	return l
}

// MaxExtract: the content of one file read from an image (spec v0.2 §5.5,
// the preview of spec §6); one byte more is too_large.
const MaxExtract = 2 << 20

// maxManifest: manifest.json of `docker save` (spec v0.2 §5.5 step 3).
const maxManifest = 64 << 10

// unsupportedImage is the code unsupported_image (422) of spec v0.2 §6,
// named by APICode for package api (fserr.go).
type unsupportedImage struct{}

func (unsupportedImage) Error() string   { return "unsupported image" }
func (unsupportedImage) APICode() string { return "unsupported_image" }

// ErrUnsupportedImage: a zstd layer (Q62), a manifest of several entries, a
// blob whose content does not give its name, a missing layer, a malformed
// layer or archive, a name or link target over MaxName.
var ErrUnsupportedImage error = unsupportedImage{}

func unsupported(format string, a ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrUnsupportedImage}, a...)...)
}

// layerEntry is the compact form of one header of a layer: PAXRecords,
// Xattrs, Uname and Gname are dropped (spec v0.2 §5.5 step 2), Rank is its
// position in its layer.
type layerEntry struct {
	Name, Linkname string
	Typeflag       byte
	Size, Mode     int64
	UID, GID       int
	ModTime        time.Time
	Rank           int
}

func compact(h *tar.Header, rank int) layerEntry {
	return layerEntry{Name: h.Name, Linkname: h.Linkname, Typeflag: h.Typeflag, Size: h.Size, Mode: h.Mode,
		UID: h.Uid, GID: h.Gid, ModTime: h.ModTime, Rank: rank}
}

func (e layerEntry) header() *tar.Header {
	return &tar.Header{Name: e.Name, Linkname: e.Linkname, Typeflag: e.Typeflag, Size: e.Size, Mode: e.Mode,
		Uid: e.UID, Gid: e.GID, ModTime: e.ModTime}
}

// layerRef: the layer (hex of its SHA-256) and the rank of the entry that
// holds the visible version of a path; the rank, not the name: a path may
// appear twice in a layer.
type layerRef struct {
	Digest string
	Rank   int
}

type imageIndex struct {
	idx    *index
	origin map[string]layerRef // regular file → where its content is; a hard link → its target's
	// blobEnd: bytes of the export read when each blob ended (progress of
	// an extraction, which reads up to the end of its layer).
	blobEnd      map[string]int64
	uncompressed int64 // bytes decompressed by the build
}

var (
	blobPath  = regexp.MustCompile(`^blobs/sha256/([0-9a-f]{64})$`)
	zstdMagic = []byte{0x28, 0xb5, 0x2f, 0xfd}
)

// blobDigest: the hex of blobs/sha256/<hex>, "" for any other name.
func blobDigest(name string) string {
	if m := blobPath.FindStringSubmatch(name); m != nil {
		return m[1]
	}
	return ""
}

var (
	errTooManyUncompressed = errors.New("decompressed byte cap reached")
	errTooManyEntries      = errors.New("entry cap reached")
)

// sourceError: an error of the stream below (Docker, the connection), not
// of the archive's format.
type sourceError struct{ err error }

func (e *sourceError) Error() string { return e.err.Error() }
func (e *sourceError) Unwrap() error { return e.err }

// meter counts the bytes read through it and stops at the end of ctx or
// past max, at every Read: under archive/tar (and under gzip), it also
// stops a chain of special headers inside one single Next() (spec v0.2
// §5.5 step 2).
type meter struct {
	ctx    context.Context
	r      io.Reader
	n      *atomic.Int64
	max    int64
	over   error
	source bool // r is the export: its errors are sourceError
}

func (m *meter) Read(b []byte) (int, error) {
	if err := m.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := m.r.Read(b)
	if m.n.Add(int64(n)) > m.max {
		return n, m.over
	}
	if err != nil && err != io.EOF && m.source {
		err = &sourceError{err}
	}
	return n, err
}

// classify turns an error of a build or an extraction into the codes of
// spec v0.2 §6: a cap is too_large, Docker is docker_unavailable, the rest
// of the format unsupported_image.
func classify(ctx context.Context, err error, what string, maxDuration time.Duration) error {
	var se *sourceError
	switch {
	case errors.Is(err, ErrTooLarge), errors.Is(err, ErrUnsupportedImage):
		return err
	case errors.Is(err, errTooManyBytes), errors.Is(err, errTooManyUncompressed), errors.Is(err, errTooManyEntries):
		return fmt.Errorf("%w: %s: %v", ErrTooLarge, what, err)
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return fmt.Errorf("%w: %s beyond %s (%w)", ErrTooLarge, what, maxDuration, errTooSlow)
	case ctx.Err() != nil:
		return ctx.Err()
	case errors.As(err, &se):
		if errors.Is(err, dockerx.ErrNotFound) || errors.Is(err, dockerx.ErrUnavailable) {
			return se.err
		}
		return fmt.Errorf("%w: reading the export: %v", dockerx.ErrUnavailable, se.err)
	}
	return unsupported("%s: %v", what, err)
}

func isSparse(h *tar.Header) bool {
	if h.Typeflag == tar.TypeGNUSparse {
		return true
	}
	for k := range h.PAXRecords {
		if strings.HasPrefix(k, "GNU.sparse.") {
			return true
		}
	}
	return false
}

// next is tar.Reader.Next, with ErrInsecurePath accepted: names are
// cleaned into the index (CleanPath), never used on the disk.
func next(tr *tar.Reader) (*tar.Header, error) {
	h, err := tr.Next()
	if h != nil && errors.Is(err, tar.ErrInsecurePath) {
		err = nil
	}
	return h, err
}

// buildImageIndex reads the export r to its end (spec v0.2 §5.5 steps 2 to
// 4): a failure fails the whole build, never a partial index.
func buildImageIndex(ctx context.Context, r io.Reader, lim ImageLimits, read *atomic.Int64) (*imageIndex, error) {
	lim = lim.withDefaults()
	ctx, cancel := context.WithTimeout(ctx, lim.MaxDuration)
	defer cancel()
	b := &imageBuilder{ctx: ctx, lim: lim, layers: map[string][]layerEntry{}, blobEnd: map[string]int64{}}
	x, err := b.run(r, read)
	if err != nil {
		return nil, classify(ctx, err, "image index", lim.MaxDuration)
	}
	return x, nil
}

type imageBuilder struct {
	ctx          context.Context
	lim          ImageLimits
	layers       map[string][]layerEntry // digest → its headers, until the manifest says the order
	blobEnd      map[string]int64
	uncompressed atomic.Int64
	entries      int
	nameBytes    int64
}

func (b *imageBuilder) run(r io.Reader, read *atomic.Int64) (*imageIndex, error) {
	outer := tar.NewReader(&meter{ctx: b.ctx, r: r, n: read, max: b.lim.MaxBytes, over: errTooManyBytes, source: true})
	var manifest []byte
	for {
		if err := b.ctx.Err(); err != nil {
			return nil, err
		}
		h, err := next(outer)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		digest := blobDigest(h.Name)
		switch {
		case h.Typeflag != tar.TypeReg:
			continue // directories, links…: skipped by Next()
		case digest == "" && h.Name != "manifest.json":
			continue // index.json, oci-layout, repositories, anything else
		case isSparse(h):
			return nil, unsupported("sparse entry %q in the export", h.Name)
		case digest != "":
			if err := b.blob(digest, outer); err != nil {
				return nil, err
			}
			b.blobEnd[digest] = read.Load()
		default:
			if manifest != nil {
				return nil, unsupported("two manifest.json")
			}
			if manifest, err = io.ReadAll(io.LimitReader(outer, maxManifest+1)); err != nil {
				return nil, err
			}
			if len(manifest) > maxManifest {
				return nil, unsupported("manifest.json over %d bytes", maxManifest)
			}
		}
	}
	order, err := layersOf(manifest)
	if err != nil {
		return nil, err
	}
	for _, d := range order {
		if _, ok := b.layers[d]; !ok {
			return nil, unsupported("layer %.12s absent from the export", d)
		}
	}
	return b.merge(order)
}

// layersOf reads manifest.json: exactly one entry (several platforms
// present: refused, spec v0.2 §13), layers named blobs/sha256/<hex>.
func layersOf(manifest []byte) ([]string, error) {
	if manifest == nil {
		return nil, unsupported("no manifest.json")
	}
	var m []struct{ Layers []string }
	if err := json.Unmarshal(manifest, &m); err != nil {
		return nil, unsupported("manifest.json: %v", err)
	}
	if len(m) != 1 {
		return nil, unsupported("manifest.json of %d entries", len(m))
	}
	out := make([]string, 0, len(m[0].Layers))
	for _, l := range m[0].Layers {
		d := blobDigest(l)
		if d == "" {
			return nil, unsupported("layer %q out of blobs/sha256/", l)
		}
		out = append(out, d)
	}
	return out, nil
}

// blobKind tells a blob by its first 512 bytes.
type blobKind int

const (
	blobOther blobKind = iota
	blobGzip
	blobZstd
	blobTar
)

func kindOf(head []byte) blobKind {
	switch {
	case len(head) >= 2 && head[0] == 0x1f && head[1] == 0x8b:
		return blobGzip
	case bytes.HasPrefix(head, zstdMagic):
		return blobZstd
	case len(head) == 512 && ustarHeader(head):
		return blobTar
	}
	return blobOther
}

// ustarHeader: "ustar" at byte 257 and a right checksum (signed or unsigned
// sum, the header's checksum field counted as spaces).
func ustarHeader(b []byte) bool {
	if string(b[257:262]) != "ustar" {
		return false
	}
	want, err := strconv.ParseInt(strings.Trim(string(b[148:156]), " \x00"), 8, 64)
	if err != nil {
		return false
	}
	var unsigned, signed int64
	for i, c := range b {
		if i >= 148 && i < 156 {
			c = ' '
		}
		unsigned += int64(c)
		signed += int64(int8(c))
	}
	return want == unsigned || want == signed
}

// layerReader opens the content of a layer blob, gzip or plain tar, read
// from br (whose first 512 bytes were peeked).
func layerReader(kind blobKind, br *bufio.Reader) (io.Reader, error) {
	switch kind {
	case blobGzip:
		return gzip.NewReader(br)
	case blobTar:
		return br, nil
	case blobZstd:
		return nil, unsupported("zstd layer (Q62)")
	}
	return nil, unsupported("blob that is not a layer")
}

// blob reads one blob of the export: its SHA-256 over every byte, its
// headers if it is a layer (Next() only), the rest read and thrown away.
func (b *imageBuilder) blob(digest string, r io.Reader) error {
	sum := sha256.New()
	br := bufio.NewReaderSize(io.TeeReader(r, sum), 4096)
	head, _ := br.Peek(512) // shorter: an ordinary blob; an error comes back below
	kind := kindOf(head)
	var entries []layerEntry
	switch kind {
	case blobZstd:
		return unsupported("zstd layer %.12s (Q62)", digest)
	case blobGzip, blobTar:
		lr, err := layerReader(kind, br)
		if err != nil {
			return err
		}
		if entries, err = b.layer(&meter{ctx: b.ctx, r: lr, n: &b.uncompressed, max: b.lim.MaxUncompressed, over: errTooManyUncompressed}); err != nil {
			return err
		}
	}
	// The rest of the blob, compressed or not: its digest needs every byte.
	n, zeros, err := drain(br)
	if err != nil {
		return err
	}
	if err := checkDigest(sum, digest); err != nil {
		return err
	}
	switch {
	case kind != blobOther:
		b.layers[digest] = entries
	case zeros && n >= 1024 && n%512 == 0:
		b.layers[digest] = nil // an empty layer exported uncompressed: two zero blocks or more
	}
	return nil
}

func checkDigest(sum hash.Hash, digest string) error {
	if got := hex.EncodeToString(sum.Sum(nil)); got != digest {
		return unsupported("blob %.12s: its content gives %.12s", digest, got)
	}
	return nil
}

// drain reads r to its end; zeros: every byte read was 0.
func drain(r io.Reader) (n int64, zeros bool, err error) {
	buf := make([]byte, 32<<10)
	zeros = true
	for {
		k, err := r.Read(buf)
		if zeros && k > 0 && bytes.ContainsFunc(buf[:k], func(c rune) bool { return c != 0 }) {
			zeros = false
		}
		n += int64(k)
		if err == io.EOF {
			return n, zeros, nil
		}
		if err != nil {
			return n, zeros, err
		}
	}
}

// layer keeps the compact headers of one layer; the content of an entry is
// never read: Next() skips its physical bytes, which the meter counts. An
// entry declared sparse of logical size 2^60 is thus skipped at the cost of
// its few physical bytes.
func (b *imageBuilder) layer(r io.Reader) ([]layerEntry, error) {
	tr := tar.NewReader(r)
	var out []layerEntry
	for rank := 0; ; rank++ {
		if err := b.ctx.Err(); err != nil {
			return nil, err
		}
		h, err := next(tr)
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if b.entries++; b.entries > b.lim.MaxEntries {
			return nil, fmt.Errorf("%w (%d)", errTooManyEntries, b.lim.MaxEntries)
		}
		if len(h.Name) > b.lim.MaxName || len(h.Linkname) > b.lim.MaxName {
			return nil, unsupported("name or link target over %d bytes", b.lim.MaxName)
		}
		if b.nameBytes += int64(len(h.Name) + len(h.Linkname)); b.nameBytes > b.lim.MaxNameBytes {
			return nil, fmt.Errorf("%w: names and targets over %d bytes", ErrTooLarge, b.lim.MaxNameBytes)
		}
		out = append(out, compact(h, rank))
	}
}

// Whiteouts of the OCI and Docker layers.
const (
	whPrefix = ".wh."
	whOpaque = ".wh..wh..opq"
)

// mergeCheckEvery: the merge looks at its context every that many entries.
const mergeCheckEvery = 1024

// merge applies the layers in the order of the manifest, the lowest first
// (spec v0.2 §5.5 step 4), into an index of spec §5.2: same CleanPath, same
// add, same hard links, same /proc/ and /sys/ left out.
func (b *imageBuilder) merge(order []string) (*imageIndex, error) {
	x := &imageIndex{
		idx: &index{
			entries:   map[string]Entry{"/": {Name: "/", Type: TypeDir, Mode: 0o755}},
			children:  map[string][]string{},
			hardlinks: map[string]string{},
		},
		origin:       map[string]layerRef{},
		blobEnd:      b.blobEnd,
		uncompressed: b.uncompressed.Load(),
	}
	seen := 0
	check := func() error {
		if seen++; seen%mergeCheckEvery == 0 {
			return b.ctx.Err()
		}
		return nil
	}
	for _, digest := range order {
		entries := b.layers[digest]
		// Its whiteouts first, on the lower layers.
		for _, e := range entries {
			if err := check(); err != nil {
				return nil, err
			}
			p := CleanPath(e.Name)
			base := path.Base(p)
			if !strings.HasPrefix(base, whPrefix) {
				continue
			}
			switch name := base[len(whPrefix):]; {
			case base == whOpaque:
				x.clearDir(path.Dir(p))
			case strings.HasPrefix(name, whPrefix): // .wh..wh.*: other metadata of aufs, never shown
			case name == "" || name == "." || name == "..": // names nothing: neither the directory nor its parent
			default:
				x.removeTree(path.Join(path.Dir(p), name))
			}
		}
		// Then its entries, in the order of the layer.
		for _, e := range entries {
			if err := check(); err != nil {
				return nil, err
			}
			p := CleanPath(e.Name)
			if strings.HasPrefix(path.Base(p), whPrefix) || e.Typeflag == tar.TypeXGlobalHeader {
				continue
			}
			// A directory replaced by anything else loses all its
			// descendants, those of the lower layers included.
			if old, ok := x.idx.entries[p]; ok && p != "/" && old.Type == TypeDir && e.Typeflag != tar.TypeDir {
				x.removeTree(p)
			}
			x.idx.add(e.header())
			x.setOrigin(p, e, digest)
		}
	}
	x.tidy()
	return x, nil
}

func (x *imageIndex) setOrigin(p string, e layerEntry, digest string) {
	got, ok := x.idx.entries[p]
	switch {
	case !ok || got.Type != TypeFile:
		delete(x.origin, p)
	case e.Typeflag == tar.TypeLink: // add made it a file only if its target is one
		if ref, ok := x.origin[CleanPath(e.Linkname)]; ok {
			x.origin[p] = ref
		} else {
			delete(x.origin, p)
		}
	default:
		x.origin[p] = layerRef{Digest: digest, Rank: e.Rank}
	}
}

// removeTree removes p and everything under it. Its name may stay in its
// parent's list: tidy drops it.
func (x *imageIndex) removeTree(p string) {
	if p == "/" {
		return
	}
	for _, n := range x.idx.children[p] {
		x.removeTree(childPath(p, n))
	}
	delete(x.idx.children, p)
	delete(x.idx.entries, p)
	delete(x.idx.hardlinks, p)
	delete(x.origin, p)
}

// clearDir removes every entry under d, d itself kept (opaque directory).
func (x *imageIndex) clearDir(d string) {
	if e, ok := x.idx.entries[d]; !ok || e.Type != TypeDir {
		return
	}
	for _, n := range x.idx.children[d] {
		x.removeTree(childPath(d, n))
	}
	delete(x.idx.children, d)
}

// tidy keeps in each directory's list the names still in the index, once.
func (x *imageIndex) tidy() {
	for d, names := range x.idx.children {
		if e, ok := x.idx.entries[d]; !ok || e.Type != TypeDir {
			delete(x.idx.children, d)
			continue
		}
		seen := make(map[string]bool, len(names))
		kept := names[:0]
		for _, n := range names {
			if _, ok := x.idx.entries[childPath(d, n)]; ok && !seen[n] {
				seen[n] = true
				kept = append(kept, n)
			}
		}
		x.idx.children[d] = kept
	}
}

// ImageFS is the read view of an image (spec v0.2 §5.5): List and Stat in
// the index, Open by an extraction of the layer that holds the file. Paths
// are those of the image; links are resolved in the index only.
type ImageFS interface {
	List(ctx context.Context, dir string) (entries []Entry, truncated bool, err error)
	Stat(ctx context.Context, path string) (Entry, error)
	// Open returns a regular file (ErrNotRegular otherwise, without asking
	// Docker), of MaxExtract bytes at most (ErrTooLarge beyond), or
	// *IndexingError while its extraction runs.
	Open(ctx context.Context, path string) (io.ReadCloser, error)
}

type imageFS struct {
	v   *Views
	id  string // "sha256:<hex>"
	img *imageIndex
}

var _ ImageFS = (*imageFS)(nil)

// List and Stat are those of a stopped container (spec §5.2 step 5).
func (f *imageFS) List(ctx context.Context, dir string) ([]Entry, bool, error) {
	return (&tarFS{idx: f.img.idx}).List(ctx, dir)
}

func (f *imageFS) Stat(ctx context.Context, p string) (Entry, error) {
	return (&tarFS{idx: f.img.idx}).Stat(ctx, p)
}

func (f *imageFS) Open(ctx context.Context, p string) (io.ReadCloser, error) {
	r, err := f.img.idx.resolve(p)
	if err != nil {
		return nil, err
	}
	ref, ok := f.img.origin[r]
	if f.img.idx.entries[r].Type != TypeFile || !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotRegular, p)
	}
	b, err := f.v.extract(ctx, f.id, r, ref, f.img.blobEnd[ref.Digest])
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

// extractFrom reads the export r up to the entry ref and returns its content
// (MaxExtract bytes at most), once the whole blob of its layer has given its
// name back as SHA-256 (spec v0.2 §5.5): the blobs after it are never read.
func extractFrom(ctx context.Context, r io.Reader, ref layerRef, read *atomic.Int64, lim ExtractLimits) ([]byte, error) {
	outer := tar.NewReader(&meter{ctx: ctx, r: r, n: read, max: lim.MaxBytes, over: errTooManyBytes, source: true})
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		h, err := next(outer)
		if err == io.EOF {
			return nil, unsupported("layer %.12s absent from the export", ref.Digest)
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag != tar.TypeReg || blobDigest(h.Name) != ref.Digest {
			continue // skipped by Next()
		}
		if isSparse(h) {
			return nil, unsupported("sparse entry %q in the export", h.Name)
		}
		return entryOf(ctx, outer, ref, lim)
	}
}

func entryOf(ctx context.Context, r io.Reader, ref layerRef, lim ExtractLimits) ([]byte, error) {
	sum := sha256.New()
	br := bufio.NewReaderSize(io.TeeReader(r, sum), 4096)
	head, _ := br.Peek(512)
	lr, err := layerReader(kindOf(head), br)
	if err != nil {
		return nil, err
	}
	var unc atomic.Int64
	tr := tar.NewReader(&meter{ctx: ctx, r: lr, n: &unc, max: lim.MaxUncompressed, over: errTooManyUncompressed})
	var content []byte
	for rank := 0; ; rank++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		h, err := next(tr)
		if err == io.EOF {
			return nil, unsupported("entry %d absent from layer %.12s", ref.Rank, ref.Digest)
		}
		if err != nil {
			return nil, err
		}
		if rank < ref.Rank {
			continue
		}
		switch h.Typeflag {
		case tar.TypeReg, tar.TypeCont, tar.TypeGNUSparse:
		default:
			return nil, unsupported("entry %d of layer %.12s is not a file", ref.Rank, ref.Digest)
		}
		if content, err = io.ReadAll(io.LimitReader(tr, MaxExtract+1)); err != nil {
			return nil, err
		}
		if len(content) > MaxExtract {
			return nil, fmt.Errorf("%w: file over %d bytes", ErrTooLarge, MaxExtract)
		}
		break
	}
	if _, _, err := drain(br); err != nil {
		return nil, err
	}
	if err := checkDigest(sum, ref.Digest); err != nil {
		return nil, err
	}
	return content, nil
}

// imageKey: the key of an image's build in Views.builds. The NUL never
// appears in a container's reference, so Pending, Indexing and Forget never
// match an image.
func imageKey(id string) string { return "image\x00" + id }

// imageSticky: an image is immutable, its too_large and unsupported_image
// stay until its slot is evicted.
func imageSticky(err error) bool {
	return errors.Is(err, ErrTooLarge) || errors.Is(err, ErrUnsupportedImage)
}

// ForImage opens the files of the image id ("sha256:<hex>", the full ID: the
// export goes by ID, never by name). The index shares the slots of the
// containers' (spec v0.2 §5.5 step 6): *IndexingError while it is built.
func (v *Views) ForImage(ctx context.Context, id string) (ImageFS, error) {
	key := imageKey(id)
	v.mu.Lock()
	_, known := v.builds[key]
	v.mu.Unlock()
	var size int64
	if !known { // a poll of a build in progress asks Docker nothing
		info, err := v.docker.ImageInspect(ctx, id)
		if err != nil {
			return nil, err
		}
		if info.ID != id {
			return nil, fmt.Errorf("%w: Docker answered image %s for %s", dockerx.ErrNotFound, info.ID, id)
		}
		size = info.Size
	}
	b, err := v.await(ctx, key, imageSticky, func() *build {
		b := &build{id: key, name: key}
		b.total.Store(size)
		return v.startLocked(key, b, v.opt.Image.MaxDuration, func(ctx context.Context, b *build) { v.runImage(ctx, key, b, id) })
	})
	if err != nil {
		return nil, err
	}
	return &imageFS{v: v, id: id, img: b.img}, nil
}

func (v *Views) runImage(ctx context.Context, key string, b *build, id string) {
	defer b.cancel()
	img, err := v.buildImage(ctx, b, id)
	if err != nil && errors.Is(ctx.Err(), context.Canceled) {
		err = &IndexingError{} // evicted meanwhile: its waiters ask again
	}
	b.img, b.err, b.doneAt = img, err, time.Now()
	close(b.done)
	v.settle(key, b)
}

func (v *Views) buildImage(ctx context.Context, b *build, id string) (*imageIndex, error) {
	rc, err := v.docker.ImageSave(ctx, id)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: image index beyond %s (%w)", ErrTooLarge, v.opt.Image.MaxDuration, errTooSlow)
		}
		return nil, err
	}
	defer rc.Close()
	return buildImageIndex(ctx, rc, v.opt.Image, &b.read)
}

// extractKeep: the content of an extraction stays that long for the next
// poll (spec v0.2 §5.5), then it is forgotten.
const extractKeep = 60 * time.Second

// extraction reads one file of an image (spec v0.2 §5.5): its own context,
// detached from the requests, bounded by MaxDuration and cancelled when no
// request asked for it for Idle.
type extraction struct {
	done    chan struct{} // closed once content and err are set
	idle    *time.Timer   // under v.mu: cancels it after Idle without a request
	read    atomic.Int64
	total   int64 // bytes of the export up to the end of its layer, from the index
	content []byte
	err     error
}

// keepExtract: the results kept extractKeep (a content, or a definitive
// error of an immutable image). Any other error is passing (Docker cut, the
// connection lost): delivered once to a request, then the next request
// extracts again; nobody asking, it is forgotten after Idle.
func keepExtract(err error) bool {
	return err == nil || errors.Is(err, ErrTooLarge) || errors.Is(err, ErrUnsupportedImage)
}

// extract returns the content of the regular file r of image id, whose
// visible version is ref; total is the end of its layer in the export.
// One extraction at a time per image, PerAgent for the whole agent: beyond,
// *IndexingError without starting anything, and the next poll tries again.
func (v *Views) extract(ctx context.Context, id, r string, ref layerRef, total int64) ([]byte, error) {
	key := id + "\x00" + r
	lim := v.opt.Extract
	v.mu.Lock()
	e, ok := v.extracts[key]
	switch {
	case ok:
		if e.idle != nil {
			e.idle.Reset(lim.Idle) // a request: still wanted
		}
	case v.running[id]:
		v.mu.Unlock()
		return nil, &IndexingError{}
	default:
		select {
		case v.slots <- struct{}{}:
		default:
			v.mu.Unlock()
			return nil, &IndexingError{}
		}
		e = v.startExtract(key, id, ref, total)
	}
	v.mu.Unlock()
	if !ok && v.opt.FirstWait > 0 { // only the request that started it
		timer := time.NewTimer(v.opt.FirstWait)
		defer timer.Stop()
		select {
		case <-e.done:
		case <-timer.C:
		case <-ctx.Done():
		}
	}
	select {
	case <-e.done:
	default:
		return nil, &IndexingError{Progress: progress(e.read.Load(), e.total)}
	}
	if !keepExtract(e.err) {
		v.mu.Lock()
		if v.extracts[key] == e {
			delete(v.extracts, key)
		}
		v.mu.Unlock()
	}
	return e.content, e.err
}

// startExtract, under v.mu, with a slot of v.slots taken.
func (v *Views) startExtract(key, id string, ref layerRef, total int64) *extraction {
	lim := v.opt.Extract
	ctx, cancel := context.WithTimeout(context.Background(), lim.MaxDuration)
	e := &extraction{done: make(chan struct{}), total: total}
	e.idle = time.AfterFunc(lim.Idle, cancel)
	v.extracts[key] = e
	v.running[id] = true
	go func() {
		defer cancel()
		content, err := v.runExtract(ctx, id, ref, &e.read)
		if err != nil && errors.Is(ctx.Err(), context.Canceled) {
			err = &IndexingError{} // abandoned: nobody asked for it during Idle
		}
		e.content, e.err = content, err
		v.mu.Lock()
		defer v.mu.Unlock()
		e.idle.Stop()
		e.idle = nil
		close(e.done)
		delete(v.running, id)
		<-v.slots
		forget := func(after time.Duration) {
			time.AfterFunc(after, func() {
				v.mu.Lock()
				defer v.mu.Unlock()
				if v.extracts[key] == e {
					delete(v.extracts, key)
				}
			})
		}
		switch {
		case v.extracts[key] != e:
		case keepExtract(err):
			forget(extractKeep)
		case errors.Is(err, ErrIndexing):
			delete(v.extracts, key) // abandoned: the next request starts again
		default:
			forget(lim.Idle) // passing: for a request still polling, never for the next visit
		}
	}()
	return e
}

func (v *Views) runExtract(ctx context.Context, id string, ref layerRef, read *atomic.Int64) ([]byte, error) {
	lim := v.opt.Extract
	rc, err := v.docker.ImageSave(ctx, id)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: extraction beyond %s (%w)", ErrTooLarge, lim.MaxDuration, errTooSlow)
		}
		return nil, err
	}
	defer rc.Close() // the blobs after the layer are never read
	content, err := extractFrom(ctx, rc, ref, read, lim)
	if err != nil {
		return nil, classify(ctx, err, "extraction", lim.MaxDuration)
	}
	return content, nil
}
