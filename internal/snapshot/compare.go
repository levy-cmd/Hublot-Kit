package snapshot

import (
	"errors"
	"iter"
	"time"

	"golang.org/x/sys/unix"
)

// The comparison of two manifests (spec v0.2 §5.7, Q59): a merge of two
// streams of entries, both in the order of the components (ComparePaths),
// in O(n) and in constant memory: one entry of each side at a time, plus
// the results, capped. The unknown are counted apart, a few of them
// listed (review of tranche 9: without fingerprints every unchanged file
// is unknown, and 20 000 of them filled the cap and hid a file added).
// The real changes are never pushed out by them. No content is kept anywhere, so the only verdict on
// a content is the comparison of two MACs of the same key.

// Result is one difference. Kind: added, deleted, modified (MACs of the
// same key differ), meta (metadata only: Fields), type (the type changed)
// or unknown (same size and same mtime, but a MAC is missing or of another
// key: nothing can be said of the content). Type is the entry's type (b's,
// a's for a deleted entry): the page hides the directories whose mtime
// alone changed.
type Result struct {
	Path   string   `json:"path"`
	Kind   string   `json:"kind"`
	Type   string   `json:"type"`
	Fields []string `json:"fields"`
}

// ErrOutOfOrder: a manifest is not in the order of its components (or has
// a path twice); merged anyway, every later entry would be wrong.
var ErrOutOfOrder = errors.New("snapshot manifest not in the order of its components")

// ErrResultsFull: the Merger has its limit of real changes and one more;
// Add returns it to stop the capture that feeds it. The unknown never
// fill it.
var ErrResultsFull = errors.New("comparison: results full")

// MaxUnknownPaths: the unknown listed by a comparison; past it, they are
// counted only.
const MaxUnknownPaths = 1000

// Outcome is the end of a comparison. Results: the real changes (added,
// deleted, modified, meta, type), at most the limit, then Truncated.
// Unknown: the first MaxUnknownPaths entries of kind unknown; UnknownCount
// counts them all (up to the end of the comparison, which a Truncated one
// reaches early). All in the order of the components.
type Outcome struct {
	Results      []Result
	Truncated    bool
	Unknown      []Result
	UnknownCount int
}

// Merger compares the entries of a (pulled from its stream) with those of
// b, pushed by Add as a capture gives them (spec v0.2 §5.7: against the
// current container, the walk feeds it, nothing is kept but the results).
// It is a Sink.
type Merger struct {
	next    func() (Entry, error, bool)
	stop    func()
	started bool
	cur     Entry // the current entry of a
	has     bool  // cur is valid
	lastA   string
	lastB   string
	anyB    bool
	same    bool
	limit   int
	results []Result
	unknown []Result
	nUnk    int
	full    bool
	partial bool
	err     error
}

// NewMerger compares a with what Add receives; sameKey: the MACs of a and
// of b are of the same key (ContentComparable). Finish (or Close) must be
// called.
func NewMerger(a iter.Seq2[Entry, error], sameKey bool, limit int) *Merger {
	next, stop := iter.Pull2(a)
	return &Merger{next: next, stop: stop, same: sameKey, limit: limit}
}

// advance reads the next entry of a, checking its order.
func (m *Merger) advance() error {
	e, err, ok := m.next()
	if !ok {
		m.has = false
		return nil
	}
	if err != nil {
		return err
	}
	if m.started && ComparePaths(m.lastA, e.Path) >= 0 {
		return ErrOutOfOrder
	}
	m.started, m.lastA, m.cur, m.has = true, e.Path, e, true
	return nil
}

func (m *Merger) begin() error {
	if m.started || m.err != nil {
		return m.err
	}
	if err := m.advance(); err != nil {
		m.err = err
	}
	m.started = true
	return m.err
}

func (m *Merger) emit(r Result) error {
	if r.Fields == nil {
		r.Fields = []string{}
	}
	if r.Kind == "unknown" { // apart: never counted in the limit
		if m.nUnk++; len(m.unknown) < MaxUnknownPaths {
			m.unknown = append(m.unknown, r)
		}
		return nil
	}
	if len(m.results) >= m.limit {
		m.full = true
		return ErrResultsFull
	}
	m.results = append(m.results, r)
	return nil
}

// Add compares b with the entries of a up to its path.
func (m *Merger) Add(b Entry) error {
	if err := m.begin(); err != nil {
		return err
	}
	if m.anyB && ComparePaths(m.lastB, b.Path) >= 0 {
		m.err = ErrOutOfOrder
		return m.err
	}
	m.anyB, m.lastB = true, b.Path
	for m.has && ComparePaths(m.cur.Path, b.Path) < 0 {
		if m.err = m.emit(Result{Path: m.cur.Path, Kind: "deleted", Type: m.cur.Type}); m.err != nil {
			return m.err
		}
		if m.err = m.advance(); m.err != nil {
			return m.err
		}
	}
	if m.has && m.cur.Path == b.Path {
		a := m.cur
		if m.err = m.advance(); m.err != nil {
			return m.err
		}
		if r, ok := diff(a, b, m.same); ok {
			m.err = m.emit(r)
		}
		return m.err
	}
	m.err = m.emit(Result{Path: b.Path, Kind: "added", Type: b.Type})
	return m.err
}

// MarkPartial: the capture of b was partial (a directory not read
// entirely, the hash budget reached).
func (m *Merger) MarkPartial() { m.partial = true }

// Partial reports MarkPartial.
func (m *Merger) Partial() bool { return m.partial }

// Finish takes the rest of a as deleted, and returns the outcome.
func (m *Merger) Finish() (Outcome, error) {
	defer m.stop()
	if err := m.begin(); err != nil && !errors.Is(err, ErrResultsFull) {
		return Outcome{}, err
	}
	for !m.full && m.has {
		if err := m.emit(Result{Path: m.cur.Path, Kind: "deleted", Type: m.cur.Type}); err != nil {
			break
		}
		if err := m.advance(); err != nil {
			return Outcome{}, err
		}
	}
	o := Outcome{Results: m.results, Truncated: m.full, Unknown: m.unknown, UnknownCount: m.nUnk}
	if o.Results == nil {
		o.Results = []Result{}
	}
	if o.Unknown == nil {
		o.Unknown = []Result{}
	}
	return o, nil
}

// Close releases a without a result (an aborted capture).
func (m *Merger) Close() { m.stop() }

// Compare merges the manifests a and b (spec v0.2 §5.7): at most limit
// real changes, then Truncated; the unknown apart. sameKey: the MACs of
// both are of the same key.
func Compare(a, b iter.Seq2[Entry, error], sameKey bool, limit int) (Outcome, error) {
	m := NewMerger(a, sameKey, limit)
	for e, err := range b {
		if err == nil {
			err = m.Add(e)
		}
		if errors.Is(err, ErrResultsFull) {
			break
		}
		if err != nil {
			m.Close()
			return Outcome{}, err
		}
	}
	return m.Finish()
}

// diff is the verdict on one path present on both sides; false: nothing
// changed (that can be seen).
func diff(a, b Entry, sameKey bool) (Result, bool) {
	r := Result{Path: b.Path, Type: b.Type}
	if a.Type != b.Type {
		r.Kind = "type"
		return r, true
	}
	r.Fields = fields(a, b)
	if a.Type == "file" {
		content := sameKey && a.MAC != "" && b.MAC != ""
		switch {
		case content && a.MAC != b.MAC:
			r.Kind = "modified"
			return r, true
		case !content && a.Size == b.Size && a.MTime.Equal(b.MTime):
			r.Kind = "unknown"
			return r, true
		}
	}
	if len(r.Fields) == 0 {
		return r, false
	}
	r.Kind = "meta"
	return r, true
}

// fields: the metadata that differ, in a fixed order.
func fields(a, b Entry) []string {
	var out []string
	add := func(diff bool, name string) {
		if diff {
			out = append(out, name)
		}
	}
	add(a.Size != b.Size, "size")
	add(a.Mode != b.Mode, "mode")
	add(a.UID != b.UID, "uid")
	add(a.GID != b.GID, "gid")
	add(!sameTime(a.MTime, b.MTime), "mtime")
	add(a.Link != b.Link, "link")
	return out
}

func sameTime(a, b time.Time) bool { return a.Equal(b) }

// ErrNoKey: no snapshot.key in the data directory yet.
var ErrNoKey = errors.New("no snapshot key")

// ExistingKey reads the key of the MACs with the checks of Key, but never
// creates it (ErrNoKey): a comparison writes nothing (spec v0.2 §5.7).
func (s *Store) ExistingKey() ([]byte, string, error) {
	if s.err != nil {
		return nil, "", s.err
	}
	key, err := readKey(int(s.data.Fd()))
	if errors.Is(err, unix.ENOENT) {
		return nil, "", ErrNoKey
	}
	if err != nil {
		return nil, "", err
	}
	return key, KeyID(key), nil
}
