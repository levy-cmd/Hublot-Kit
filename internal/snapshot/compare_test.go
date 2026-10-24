package snapshot

// Tests of the comparison (spec v0.2 §5.7, plan v0.2 tranche 9), written
// before the code: the kinds, a merge of two manifests read in a stream in
// constant memory, the key mismatch, the missing MAC, the cap of results.

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)

func fileE(p, mac string, size int64) Entry {
	return Entry{Path: p, Type: "file", Size: size, Mode: 0o644, MTime: t0, MAC: mac}
}

func dirE(p string) Entry { return Entry{Path: p, Type: "dir", Mode: 0o755, MTime: t0} }

// seqOf gives entries as a manifest read would.
func seqOf(es ...Entry) iter.Seq2[Entry, error] {
	return func(yield func(Entry, error) bool) {
		for _, e := range es {
			if !yield(e, nil) {
				return
			}
		}
	}
}

func compareAll(t *testing.T, a, b []Entry, sameKey bool) map[string]Result {
	t.Helper()
	o, err := Compare(seqOf(a...), seqOf(b...), sameKey, 10_000)
	if err != nil || o.Truncated {
		t.Fatalf("Compare : %v, tronqué %v", err, o.Truncated)
	}
	if o.UnknownCount != len(o.Unknown) {
		t.Fatalf("%d inconnus comptés, %d listés", o.UnknownCount, len(o.Unknown))
	}
	for _, r := range o.Unknown {
		if r.Kind != "unknown" {
			t.Fatalf("liste des inconnus : %+v", r)
		}
	}
	for _, r := range o.Results {
		if r.Kind == "unknown" {
			t.Fatalf("inconnu parmi les changements : %+v", r)
		}
	}
	res := append(slices.Clone(o.Results), o.Unknown...)
	slices.SortFunc(res, func(x, y Result) int { return ComparePaths(x.Path, y.Path) })
	out := map[string]Result{}
	for _, r := range res {
		if _, dup := out[r.Path]; dup {
			t.Fatalf("%s deux fois", r.Path)
		}
		out[r.Path] = r
	}
	if !slices.IsSortedFunc(res, func(x, y Result) int { return ComparePaths(x.Path, y.Path) }) {
		t.Fatalf("résultats hors de l'ordre des composants : %+v", res)
	}
	return out
}

func TestCompareKinds(t *testing.T) {
	mode := fileE("/meta", "m1", 3)
	mode.Mode = 0o600
	dirMTime := dirE("/d")
	dirMTime.MTime = t0.Add(time.Hour)
	unknownB := fileE("/unknown", "", 3)
	unknownB.MACSkipped = SkipBudget
	link := Entry{Path: "/l", Type: "symlink", Mode: 0o777, MTime: t0, Link: "a"}
	link2 := link
	link2.Link = "b"
	a := []Entry{
		dirE("/d"),
		fileE("/deleted", "x", 1),
		fileE("/meta", "m1", 3),
		fileE("/modified", "m1", 3),
		fileE("/same", "s", 3),
		fileE("/type", "t", 3),
		fileE("/unknown", "u", 3),
		link,
	}
	b := []Entry{
		dirMTime,
		fileE("/added", "y", 1),
		mode,
		fileE("/modified", "m2", 3),
		fileE("/same", "s", 3),
		dirE("/type"),
		unknownB,
		link2,
	}
	byPath := func(x, y Entry) int { return ComparePaths(x.Path, y.Path) }
	slices.SortFunc(a, byPath)
	slices.SortFunc(b, byPath)
	got := compareAll(t, a, b, true)
	want := map[string]struct {
		kind, typ string
		fields    []string
	}{
		"/d":        {"meta", "dir", []string{"mtime"}},
		"/added":    {"added", "file", nil},
		"/deleted":  {"deleted", "file", nil},
		"/meta":     {"meta", "file", []string{"mode"}},
		"/modified": {"modified", "file", nil},
		"/type":     {"type", "dir", nil},
		"/unknown":  {"unknown", "file", nil},
		"/l":        {"meta", "symlink", []string{"link"}},
	}
	if len(got) != len(want) {
		t.Fatalf("%d résultats, attendu %d (/same identique ne doit rien donner) : %+v", len(got), len(want), got)
	}
	for p, w := range want {
		r, ok := got[p]
		if !ok {
			t.Errorf("%s absent", p)
			continue
		}
		if r.Kind != w.kind || r.Type != w.typ || strings.Join(r.Fields, ",") != strings.Join(w.fields, ",") {
			t.Errorf("%s : %+v, attendu %s/%s %v", p, r, w.kind, w.typ, w.fields)
		}
	}
}

// A modified file says which metadata changed too.
func TestCompareModifiedFields(t *testing.T) {
	b := fileE("/f", "m2", 4)
	b.MTime = t0.Add(time.Second)
	got := compareAll(t, []Entry{fileE("/f", "m1", 3)}, []Entry{b}, true)
	if r := got["/f"]; r.Kind != "modified" || !slices.Equal(r.Fields, []string{"size", "mtime"}) {
		t.Fatalf("%+v", r)
	}
}

func TestCompareKeyMismatch(t *testing.T) {
	sized := fileE("/sized", "m2", 9)
	got := compareAll(t,
		[]Entry{fileE("/same-size", "m1", 3), fileE("/sized", "m1", 3)},
		[]Entry{fileE("/same-size", "m2", 3), sized}, false)
	if r := got["/same-size"]; r.Kind != "unknown" {
		t.Errorf("même taille et même date, autre clé : %+v, attendu unknown", r)
	}
	if r := got["/sized"]; r.Kind != "meta" || !slices.Equal(r.Fields, []string{"size"}) {
		t.Errorf("autre taille, autre clé : %+v, attendu meta [size]", r)
	}
	for _, r := range got {
		if r.Kind == "modified" {
			t.Errorf("modified sans clé commune : %+v", r)
		}
	}
	// Same MAC under two keys says nothing either.
	if got := compareAll(t, []Entry{fileE("/f", "m", 3)}, []Entry{fileE("/f", "m", 3)}, false); got["/f"].Kind != "unknown" {
		t.Errorf("même MAC, autre clé : %+v", got["/f"])
	}
}

func TestCompareMissingMAC(t *testing.T) {
	skipped := fileE("/f", "", 3)
	skipped.MACSkipped = SkipTooLarge
	for name, pair := range map[string][2]Entry{
		"ancienne": {fileE("/f", "", 3), fileE("/f", "m", 3)},
		"nouvelle": {fileE("/f", "m", 3), skipped},
		"les deux": {fileE("/f", "", 3), fileE("/f", "", 3)},
	} {
		got := compareAll(t, []Entry{pair[0]}, []Entry{pair[1]}, true)
		if r := got["/f"]; r.Kind != "unknown" {
			t.Errorf("%s sans MAC : %+v, attendu unknown", name, r)
		}
	}
}

func TestCompareLimit(t *testing.T) {
	var a, b []Entry
	for i := range 10_001 {
		p := fmt.Sprintf("/f%05d", i)
		a = append(a, fileE(p, "a", 1))
		b = append(b, fileE(p, "b", 1))
	}
	o, err := Compare(seqOf(a...), seqOf(b...), true, 10_000)
	if err != nil || !o.Truncated || len(o.Results) != 10_000 {
		t.Fatalf("%d résultats, tronqué %v, %v", len(o.Results), o.Truncated, err)
	}
	if o.Results[9_999].Path != "/f09999" {
		t.Fatalf("dernier : %s", o.Results[9_999].Path)
	}
	// Exactly the cap: not truncated.
	o, err = Compare(seqOf(a[:10_000]...), seqOf(b[:10_000]...), true, 10_000)
	if err != nil || o.Truncated || len(o.Results) != 10_000 {
		t.Fatalf("au plafond : %d, tronqué %v, %v", len(o.Results), o.Truncated, err)
	}
}

// A manifest out of the order of the components would turn every later
// entry into added and deleted: refused, never a wrong answer.
func TestCompareOutOfOrder(t *testing.T) {
	_, err := Compare(seqOf(fileE("/b", "m", 1), fileE("/a", "m", 1)), seqOf(), true, 10)
	if !errors.Is(err, ErrOutOfOrder) {
		t.Fatalf("a désordonné : %v", err)
	}
	_, err = Compare(seqOf(), seqOf(fileE("/a-b", "m", 1), fileE("/a/b", "m", 1)), true, 10)
	if !errors.Is(err, ErrOutOfOrder) {
		t.Fatalf("b désordonné : %v", err)
	}
	_, err = Compare(seqOf(fileE("/a", "m", 1), fileE("/a", "m", 1)), seqOf(), true, 10)
	if !errors.Is(err, ErrOutOfOrder) {
		t.Fatalf("doublon : %v", err)
	}
}

// A read error of either manifest ends the comparison with it.
func TestCompareReadError(t *testing.T) {
	boom := errors.New("boom")
	bad := func(yield func(Entry, error) bool) {
		if yield(fileE("/a", "m", 1), nil) {
			yield(Entry{}, boom)
		}
	}
	if _, err := Compare(bad, seqOf(fileE("/z", "m", 1)), true, 10); !errors.Is(err, boom) {
		t.Fatalf("a : %v", err)
	}
	if _, err := Compare(seqOf(fileE("/z", "m", 1)), bad, true, 10); !errors.Is(err, boom) {
		t.Fatalf("b : %v", err)
	}
}

// The Merger is the Sink of a capture against the current container: fed
// as the walk goes, it stops the capture once the results are full.
func TestMergerStopsWhenFull(t *testing.T) {
	m := NewMerger(seqOf(), true, 2)
	for i, p := range []string{"/a", "/b"} {
		if err := m.Add(fileE(p, "m", 1)); err != nil {
			t.Fatalf("Add %d : %v", i, err)
		}
	}
	if err := m.Add(fileE("/c", "m", 1)); !errors.Is(err, ErrResultsFull) {
		t.Fatalf("Add au-delà du plafond : %v", err)
	}
	o, err := m.Finish()
	if err != nil || !o.Truncated || len(o.Results) != 2 {
		t.Fatalf("%+v %v", o, err)
	}
}

// The comparison against the current container writes nothing: it reads
// the key, never creates it.
func TestExistingKeyNeverCreates(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir)
	if _, _, err := s.ExistingKey(); !errors.Is(err, ErrNoKey) {
		t.Fatalf("sans clé : %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, keyName)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("clé créée : %v", err)
	}
	k, id, err := s.Key()
	if err != nil {
		t.Fatal(err)
	}
	k2, id2, err := s.ExistingKey()
	if err != nil || !bytes.Equal(k, k2) || id != id2 {
		t.Fatalf("clé relue : %v, %s ≠ %s", err, id2, id)
	}
	if err := os.Chmod(filepath.Join(dir, keyName), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ExistingKey(); err == nil || errors.Is(err, ErrNoKey) {
		t.Fatalf("clé ouverte au groupe acceptée : %v", err)
	}
}

// Review of tranche 9 (auditor, proved): without fingerprints (a photo of
// the metadata, offered first, or a key changed since), every unchanged
// file is unknown. Counted in the cap of results, 20 000 of them hid the
// one file an attacker added. The unknown are counted apart, a few listed;
// the real changes are never pushed out by them.
func TestCompareUnknownNeverEvictsChanges(t *testing.T) {
	var a, b []Entry
	for i := range 20_000 {
		e := fileE(fmt.Sprintf("/f%05d", i), "", 1)
		a, b = append(a, e), append(b, e)
	}
	b = append(b, fileE("/zz-added", "", 1))
	o, err := Compare(seqOf(a...), seqOf(b...), false, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	if o.Truncated || len(o.Results) != 1 || o.Results[0].Path != "/zz-added" || o.Results[0].Kind != "added" {
		t.Fatalf("changements : %+v, tronqué %v", o.Results, o.Truncated)
	}
	if o.UnknownCount != 20_000 || len(o.Unknown) != MaxUnknownPaths || o.Unknown[0].Path != "/f00000" {
		t.Fatalf("%d inconnus comptés, %d listés", o.UnknownCount, len(o.Unknown))
	}
	// The real changes keep their own cap, whatever the unknown.
	b = b[:20_000]
	for i := range 10_001 {
		b = append(b, fileE(fmt.Sprintf("/g%05d", i), "", 1))
	}
	o, err = Compare(seqOf(a...), seqOf(b...), false, 10_000)
	if err != nil || !o.Truncated || len(o.Results) != 10_000 || o.UnknownCount != 20_000 {
		t.Fatalf("%d changements, tronqué %v, %d inconnus, %v", len(o.Results), o.Truncated, o.UnknownCount, err)
	}
}

// heapPeak samples the heap in use every 4096 entries of seq.
type heapPeak struct {
	n    int
	peak uint64
}

func (h *heapPeak) wrap(seq iter.Seq2[Entry, error]) iter.Seq2[Entry, error] {
	return func(yield func(Entry, error) bool) {
		for e, err := range seq {
			if h.n++; h.n%4096 == 0 {
				var ms runtime.MemStats
				runtime.ReadMemStats(&ms)
				h.peak = max(h.peak, ms.HeapAlloc)
			}
			if !yield(e, err) {
				return
			}
		}
	}
}

func TestCompareStreaming(t *testing.T) {
	const n = 500_000
	s := openStore(t, t.TempDir())
	write := func(change int) string {
		h := header(strings.Repeat("c", 64))
		w, err := s.Create(h)
		if err != nil {
			t.Fatal(err)
		}
		for i := range n {
			mac := fmt.Sprintf("%064x", i)
			if change > 0 && i%change == 0 {
				mac = fmt.Sprintf("%064x", i+1)
			}
			if err := w.Add(fileE(fmt.Sprintf("/usr/share/data/f%07d", i), mac, int64(i))); err != nil {
				t.Fatal(err)
			}
		}
		h.FinishedAt = time.Now().UTC()
		if err := w.Commit(h); err != nil {
			t.Fatal(err)
		}
		return h.ID
	}
	ida, idb := write(0), write(100)
	_, seqA, err := s.Reader(ida)
	if err != nil {
		t.Fatal(err)
	}
	_, seqB, err := s.Reader(idb)
	if err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var base runtime.MemStats
	runtime.ReadMemStats(&base)
	hp := &heapPeak{}
	o, err := Compare(hp.wrap(seqA), hp.wrap(seqB), true, 10_000)
	if err != nil || o.Truncated || len(o.Results) != n/100 {
		t.Fatalf("%d résultats, tronqué %v, %v", len(o.Results), o.Truncated, err)
	}
	if hp.n != 2*n {
		t.Fatalf("%d entrées lues, attendu %d", hp.n, 2*n)
	}
	used := int64(hp.peak) - int64(base.HeapAlloc)
	t.Logf("tas : %.1f Mio au départ, pic %.1f Mio (+%.1f Mio) pour 2 × %d entrées",
		float64(base.HeapAlloc)/(1<<20), float64(hp.peak)/(1<<20), float64(used)/(1<<20), n)
	if used > 32<<20 {
		t.Fatalf("la comparaison a tenu %.1f Mio de tas, plafond 32 Mio : elle ne lit pas en flux", float64(used)/(1<<20))
	}
}
