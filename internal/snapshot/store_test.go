package snapshot

// Tests of the snapshots directory (spec v0.2 §5.7, §7), written before the
// code: opened without following a link, 0700, written relative to its
// descriptor, no partial file ever left.

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

func openStore(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := Open(openDir(t, dir))
	if err != nil {
		t.Fatalf("Open : %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func header(container string) Header {
	return Header{ID: NewID(), ContainerID: container, ContainerName: "web", CreatedAt: time.Now().UTC(),
		State: "running", Source: "procfs", KeyID: "0123456789abcdef"}
}

// put writes a snapshot of entries and returns its header as committed.
func put(t *testing.T, s *Store, h Header, entries ...Entry) Header {
	t.Helper()
	w, err := s.Create(h)
	if err != nil {
		t.Fatalf("Create : %v", err)
	}
	for _, e := range entries {
		if err := w.Add(e); err != nil {
			t.Fatalf("Add : %v", err)
		}
	}
	h.FinishedAt = time.Now().UTC()
	if err := w.Commit(h); err != nil {
		t.Fatalf("Commit : %v", err)
	}
	got, _, err := s.Reader(h.ID)
	if err != nil {
		t.Fatalf("Reader : %v", err)
	}
	return got
}

func TestStoreCreatesPrivateDir(t *testing.T) {
	dir := t.TempDir()
	openStore(t, dir)
	fi, err := os.Lstat(filepath.Join(dir, "snapshots"))
	if err != nil || !fi.IsDir() || fi.Mode().Perm() != 0o700 {
		t.Fatalf("snapshots/ : %v %v", fi, err)
	}
}

func TestStoreRefusesSymlinkDir(t *testing.T) {
	dir, other := t.TempDir(), t.TempDir()
	if err := os.Chmod(other, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, filepath.Join(dir, "snapshots")); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(openDir(t, dir)); err == nil {
		s.Close()
		t.Fatal("snapshots/ suivi à travers un lien")
	}
	if got := names(t, other); len(got) != 0 {
		t.Fatalf("écrit dans la cible du lien : %v", got)
	}
}

func TestStoreRefusesOpenDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(openDir(t, dir)); err == nil {
		s.Close()
		t.Fatal("snapshots/ en 0755 accepté")
	}
}

func TestStoreRefusesOtherOwner(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("ce test demande root (scripts/test-linux.sh)")
	}
	dir := t.TempDir()
	sd := filepath.Join(dir, "snapshots")
	if err := os.Mkdir(sd, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(sd, 12345, 12345); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(openDir(t, dir)); err == nil {
		s.Close()
		t.Fatal("snapshots/ d'un autre propriétaire accepté")
	}
}

func TestStoreCleansTempOnOpen(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir)
	w, err := s.Create(header("c1"))
	if err != nil {
		t.Fatal(err)
	}
	w.Add(Entry{Path: "/a", Type: "file"})
	// Crash: neither Commit nor Abort.
	if len(names(t, filepath.Join(dir, "snapshots"))) == 0 {
		t.Fatal("aucun fichier temporaire pendant l'écriture")
	}
	s.Close()
	openStore(t, dir)
	if got := names(t, filepath.Join(dir, "snapshots")); len(got) != 0 {
		t.Fatalf("restes d'un arrêt brutal : %v", got)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir)
	mt := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	entries := []Entry{
		{Path: "/a", Type: "dir", Mode: 0o755, MTime: mt},
		{Path: "/a/f", Type: "file", Size: 4, Mode: 0o644, MTime: mt, MAC: strings.Repeat("ab", 32)},
		{Path: "/a/l", Type: "symlink", Link: "f", MTime: mt},
		{Path: "/big", Type: "file", Size: 1 << 40, MACSkipped: "too_large", MTime: mt},
	}
	h := put(t, s, header("c1"), entries...)
	if h.Entries != 4 || h.HashedBytes != 4 || h.Partial {
		t.Fatalf("en-tête : %+v", h)
	}
	_, seq, err := s.Reader(h.ID)
	if err != nil {
		t.Fatal(err)
	}
	var got []Entry
	for e, err := range seq {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, e)
	}
	if len(got) != len(entries) {
		t.Fatalf("%d entrées relues, attendu %d", len(got), len(entries))
	}
	for i := range got {
		if got[i] != entries[i] {
			t.Errorf("entrée %d : %+v, attendu %+v", i, got[i], entries[i])
		}
	}
	list, err := s.List()
	if err != nil || len(list) != 1 || list[0].ID != h.ID || list[0].Entries != 4 {
		t.Fatalf("List : %+v %v", list, err)
	}
	fi, err := os.Lstat(filepath.Join(dir, "snapshots", h.ID+".jsonl.gz"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("fichier du snapshot : %v %v", fi, err)
	}
	if err := s.Delete(h.ID); err != nil {
		t.Fatal(err)
	}
	if got := names(t, filepath.Join(dir, "snapshots")); len(got) != 0 {
		t.Fatalf("après Delete : %v", got)
	}
	if err := s.Delete(h.ID); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("second Delete : %v, attendu fs.ErrNotExist", err)
	}
}

func TestStoreAbortLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir)
	w, err := s.Create(header("c1"))
	if err != nil {
		t.Fatal(err)
	}
	w.Add(Entry{Path: "/a", Type: "file"})
	w.Abort()
	if got := names(t, filepath.Join(dir, "snapshots")); len(got) != 0 {
		t.Fatalf("après Abort : %v", got)
	}
}

func TestStoreIDFormat(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir)
	// A file next to snapshots/ that "../victim" would name, a valid
	// manifest of that ID: neither read nor deleted.
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	json.NewEncoder(zw).Encode(Header{ID: "../victim", ContainerID: "c1"})
	zw.Close()
	victim := filepath.Join(dir, "victim.jsonl.gz")
	if err := os.WriteFile(victim, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Reader("../victim"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Reader(../victim) : %v, attendu fs.ErrNotExist", err)
	}
	if err := s.Delete("../victim"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Delete(../victim) : %v, attendu fs.ErrNotExist", err)
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("fichier hors de snapshots/ supprimé : %v", err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(NewID()) {
		t.Fatalf("NewID : %s, attendu un UUID v4 en minuscules", NewID())
	}
	for _, id := range []string{"../x", "", "x", strings.ToUpper(NewID()), NewID() + ".jsonl.gz", "../" + NewID()} {
		if ValidID(id) {
			t.Errorf("ValidID(%q) = true", id)
		}
		if _, _, err := s.Reader(id); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Reader(%q) : %v, attendu fs.ErrNotExist", id, err)
		}
		if err := s.Delete(id); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Delete(%q) : %v, attendu fs.ErrNotExist", id, err)
		}
	}
	if _, err := s.Create(Header{ID: "../x", ContainerID: "c1"}); err == nil {
		t.Error("Create avec un ID invalide accepté")
	}
}

// Q66: 20 per container, 1 GiB in all; past them, refused, nothing deleted.
func TestStoreRetention(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir)
	for range MaxPerContainer {
		put(t, s, header("c1"), Entry{Path: "/a", Type: "file"})
	}
	if _, err := s.Create(header("c1")); !errors.Is(err, fsview.ErrTooLarge) {
		t.Fatalf("21e snapshot : %v, attendu ErrTooLarge", err)
	}
	if w, err := s.Create(header("c2")); err != nil {
		t.Fatalf("autre conteneur : %v", err)
	} else {
		w.Abort()
	}
	if list, _ := s.List(); len(list) != MaxPerContainer {
		t.Fatalf("%d snapshots après le refus, attendu %d (aucune suppression)", len(list), MaxPerContainer)
	}
	old := maxTotalBytes
	t.Cleanup(func() { maxTotalBytes = old })
	maxTotalBytes = 1
	if _, err := s.Create(header("c2")); !errors.Is(err, fsview.ErrTooLarge) {
		t.Fatalf("plafond total : %v, attendu ErrTooLarge", err)
	}
}
