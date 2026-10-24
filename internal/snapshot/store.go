package snapshot

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"

	"golang.org/x/sys/unix"

	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

// The snapshots directory (spec v0.2 §5.7): <data-dir>/snapshots, opened by
// openat(O_DIRECTORY|O_NOFOLLOW) and checked on its descriptor (a directory
// of the agent's user, 0700). Every later operation (create, renameat,
// unlinkat, read) is relative to that descriptor, never a built path. A
// snapshot is <id>.jsonl.gz, 0600: two gzip members, the header line, then
// one line per entry. It is written in temporary files (".tmp-" names),
// synced, then published by renameat: a failure leaves no partial file,
// and the leftovers of a crash are removed by Open.

// MaxPerContainer and maxTotalBytes: the retention of Q66. Past them, a
// new snapshot is refused (too_large); nothing is ever deleted by the agent.
const MaxPerContainer = 20

var maxTotalBytes int64 = 1 << 30 // a variable: a test lowers it

const (
	dirName    = "snapshots"
	suffix     = ".jsonl.gz"
	tempPrefix = ".tmp-"
	maxHeader  = 64 << 10
)

// ErrNotFound: no snapshot of that ID (or not an ID at all).
var ErrNotFound = fmt.Errorf("no such snapshot: %w", fs.ErrNotExist)

type Store struct {
	err  error    // Unavailable: every method returns it
	data *os.File // the data directory (the key), owned by the store
	dir  *os.File // snapshots/
	mu   sync.Mutex
}

// Unavailable is a store whose every method fails with err: the agent runs
// on, the snapshot routes answer internal with the reason (spec v0.2 §5.7).
func Unavailable(err error) *Store { return &Store{err: err} }

// Open opens (or creates, 0700) dataDir/snapshots and removes the
// temporary files a crash left there. dataDir stays the caller's: the store
// keeps a duplicate of it.
func Open(dataDir *os.File) (*Store, error) {
	dfd := int(dataDir.Fd())
	created := true
	if err := unix.Mkdirat(dfd, dirName, 0o700); errors.Is(err, unix.EEXIST) {
		created = false
	} else if err != nil {
		return nil, fmt.Errorf("%s: %w", dirName, err)
	}
	fd, err := unix.Openat(dfd, dirName, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR) {
		return nil, fmt.Errorf("%s refused: a symbolic link or not a directory", dirName)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", dirName, err)
	}
	dir := os.NewFile(uintptr(fd), dirName)
	fail := func(err error) (*Store, error) {
		dir.Close()
		return nil, err
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return fail(fmt.Errorf("%s: %w", dirName, err))
	}
	if int(st.Uid) != os.Geteuid() {
		return fail(fmt.Errorf("%s refused: owned by uid %d, not by the agent (uid %d)", dirName, st.Uid, os.Geteuid()))
	}
	if created {
		if err := unix.Fchmod(fd, 0o700); err != nil { // the umask may have removed owner bits
			return fail(fmt.Errorf("%s: %w", dirName, err))
		}
	} else if st.Mode&0o077 != 0 {
		return fail(fmt.Errorf("%s refused: mode %#o, open to group or others (0700 expected)", dirName, st.Mode&0o777))
	}
	dup, err := unix.Dup(dfd)
	if err != nil {
		return fail(err)
	}
	unix.CloseOnExec(dup)
	s := &Store{data: os.NewFile(uintptr(dup), "data-dir"), dir: dir}
	names, err := s.names()
	if err != nil {
		s.Close()
		return nil, err
	}
	for _, n := range names {
		if strings.HasPrefix(n, tempPrefix) {
			unix.Unlinkat(fd, n, 0)
		}
	}
	return s, nil
}

func (s *Store) Close() error {
	if s.err != nil {
		return nil
	}
	s.data.Close()
	return s.dir.Close()
}

// Key reads (or creates) the key of the MACs; refused, the snapshots answer
// internal with the reason (spec v0.2 §5.7).
func (s *Store) Key() ([]byte, string, error) {
	if s.err != nil {
		return nil, "", s.err
	}
	return LoadOrCreateKey(s.data)
}

func (s *Store) dfd() int { return int(s.dir.Fd()) }

// names lists the directory through a new descriptor of it (".", relative
// to the directory's own): a read never moves the offset of s.dir.
func (s *Store) names() ([]string, error) {
	fd, err := unix.Openat(s.dfd(), ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", dirName, err)
	}
	d := os.NewFile(uintptr(fd), dirName)
	defer d.Close()
	return d.Readdirnames(-1)
}

// open opens the snapshot file name for reading, never through a link.
func (s *Store) open(name string) (*os.File, int64, error) {
	fd, err := unix.Openat(s.dfd(), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, fmt.Errorf("snapshot %s: %w", name, err)
	}
	f := os.NewFile(uintptr(fd), name)
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFREG {
		f.Close()
		return nil, 0, fmt.Errorf("snapshot %s: not a regular file", name)
	}
	return f, st.Size, nil
}

// decoder reads the JSON values of a snapshot file in order.
func decoder(f *os.File) (*json.Decoder, func(), error) {
	zr, err := gzip.NewReader(bufio.NewReader(f))
	if err != nil {
		return nil, nil, err
	}
	return json.NewDecoder(zr), func() { zr.Close() }, nil
}

type stored struct {
	h    Header
	size int64
}

func (s *Store) readHeader(id string) (stored, error) {
	f, size, err := s.open(id + suffix)
	if err != nil {
		return stored{}, err
	}
	defer f.Close()
	dec, done, err := decoder(f)
	if err != nil {
		return stored{}, fmt.Errorf("snapshot %s: %w", id, err)
	}
	defer done()
	var h Header
	if err := dec.Decode(&h); err != nil {
		return stored{}, fmt.Errorf("snapshot %s: header: %w", id, err)
	}
	if h.ID != id {
		return stored{}, fmt.Errorf("snapshot %s: header of %q", id, h.ID)
	}
	return stored{h, size}, nil
}

// all reads the header of every snapshot; an unreadable one is logged and
// left out.
func (s *Store) all() ([]stored, error) {
	if s.err != nil {
		return nil, s.err
	}
	names, err := s.names()
	if err != nil {
		return nil, err
	}
	var out []stored
	for _, n := range names {
		id, ok := strings.CutSuffix(n, suffix)
		if !ok || !ValidID(id) {
			continue
		}
		st, err := s.readHeader(id)
		if err != nil {
			if !errors.Is(err, ErrNotFound) {
				slog.Warn("unreadable snapshot", "id", id, "err", err)
			}
			continue
		}
		out = append(out, st)
	}
	return out, nil
}

// List returns the headers of the snapshots, the most recent first.
func (s *Store) List() ([]Header, error) {
	all, err := s.all()
	if err != nil {
		return nil, err
	}
	out := make([]Header, len(all))
	for i, st := range all {
		out[i] = st.h
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// Reader returns the header of snapshot id and its entries, read in a
// stream (the file is opened again by the iteration, and closed by it).
func (s *Store) Reader(id string) (Header, iter.Seq2[Entry, error], error) {
	if s.err != nil {
		return Header{}, nil, s.err
	}
	if !ValidID(id) {
		return Header{}, nil, ErrNotFound
	}
	st, err := s.readHeader(id)
	if err != nil {
		return Header{}, nil, err
	}
	seq := func(yield func(Entry, error) bool) {
		f, _, err := s.open(id + suffix)
		if err != nil {
			yield(Entry{}, err)
			return
		}
		defer f.Close()
		dec, done, err := decoder(f)
		if err != nil {
			yield(Entry{}, err)
			return
		}
		defer done()
		var h Header
		if err := dec.Decode(&h); err != nil {
			yield(Entry{}, err)
			return
		}
		for {
			var e Entry
			err := dec.Decode(&e)
			if err == io.EOF {
				return
			}
			if !yield(e, err) || err != nil {
				return
			}
		}
	}
	return st.h, seq, nil
}

// Delete removes snapshot id by unlinkat on the directory's descriptor.
func (s *Store) Delete(id string) error {
	if s.err != nil {
		return s.err
	}
	if !ValidID(id) {
		return ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := unix.Unlinkat(s.dfd(), id+suffix, 0); errors.Is(err, unix.ENOENT) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("snapshot %s: %w", id, err)
	}
	unix.Fsync(s.dfd())
	return nil
}

// Writer writes the entries of a new snapshot; Commit publishes it, Abort
// removes it. It is the Sink of a capture.
type Writer struct {
	s       *Store
	id      string
	tmp     string // the entries, one gzip member
	f       *os.File
	zw      *gzip.Writer
	bw      *bufio.Writer
	enc     *json.Encoder
	n       int
	hashed  int64
	partial bool
}

// Create checks the retention, then opens the temporary file of the
// entries of h.ID.
func (s *Store) Create(h Header) (*Writer, error) {
	if s.err != nil {
		return nil, s.err
	}
	if !ValidID(h.ID) || h.ContainerID == "" {
		return nil, fmt.Errorf("snapshot: invalid header (ID %q, container %q)", h.ID, h.ContainerID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.all()
	if err != nil {
		return nil, err
	}
	var n int
	var total int64
	for _, st := range all {
		total += st.size
		if st.h.ContainerID == h.ContainerID {
			n++
		}
	}
	if n >= MaxPerContainer {
		return nil, fmt.Errorf("%w: this container already has %d snapshots, the most kept (delete one first)", fsview.ErrTooLarge, n)
	}
	if total >= maxTotalBytes {
		return nil, fmt.Errorf("%w: the snapshots already take %d bytes, the most kept is %d (delete some first)", fsview.ErrTooLarge, total, maxTotalBytes)
	}
	tmp := tempPrefix + h.ID + ".entries"
	f, err := s.createTemp(tmp)
	if err != nil {
		return nil, err
	}
	w := &Writer{s: s, id: h.ID, tmp: tmp, f: f}
	w.zw = gzip.NewWriter(f)
	w.bw = bufio.NewWriterSize(w.zw, 64<<10)
	w.enc = json.NewEncoder(w.bw)
	return w, nil
}

func (s *Store) createTemp(name string) (*os.File, error) {
	fd, err := unix.Openat(s.dfd(), name, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("snapshot: %w", err)
	}
	if err := unix.Fchmod(fd, 0o600); err != nil {
		unix.Close(fd)
		unix.Unlinkat(s.dfd(), name, 0)
		return nil, fmt.Errorf("snapshot: %w", err)
	}
	return os.NewFile(uintptr(fd), name), nil
}

func (w *Writer) Add(e Entry) error {
	if err := w.enc.Encode(e); err != nil {
		return err
	}
	w.n++
	if e.MAC != "" {
		w.hashed += e.Size
	}
	return nil
}

// MarkPartial: some contents were not hashed (budget) or a directory not
// read entirely.
func (w *Writer) MarkPartial() { w.partial = true }

// Commit writes the header h (its counts are the writer's), then the
// entries, in a second temporary file, syncs it and publishes it by
// renameat. On error, nothing is left.
func (w *Writer) Commit(h Header) (err error) {
	defer func() {
		if err != nil {
			w.Abort()
		}
	}()
	h.ID, h.Entries, h.HashedBytes, h.Partial = w.id, w.n, w.hashed, h.Partial || w.partial
	if err := w.bw.Flush(); err != nil {
		return err
	}
	if err := w.zw.Close(); err != nil {
		return err
	}
	if _, err := w.f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	final := tempPrefix + w.id + suffix
	out, err := w.s.createTemp(final)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		out.Close()
		if !ok {
			unix.Unlinkat(w.s.dfd(), final, 0)
		}
	}()
	zh := gzip.NewWriter(out)
	if err := json.NewEncoder(zh).Encode(h); err != nil {
		return err
	}
	if err := zh.Close(); err != nil {
		return err
	}
	if _, err := io.Copy(out, w.f); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := unix.Renameat(w.s.dfd(), final, w.s.dfd(), w.id+suffix); err != nil {
		return fmt.Errorf("snapshot: renameat: %w", err)
	}
	ok = true
	w.Abort() // the entries' temporary file
	unix.Fsync(w.s.dfd())
	return nil
}

// Abort removes the temporary file of the entries (and closes it); safe to
// call twice.
func (w *Writer) Abort() {
	if w.f == nil {
		return
	}
	w.f.Close()
	unix.Unlinkat(w.s.dfd(), w.tmp, 0)
	w.f = nil
}
