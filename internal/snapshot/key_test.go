package snapshot

// Tests of the HMAC key (spec v0.2 §5.7, §8), written before the code.

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func openDir(t *testing.T, dir string) *os.File {
	t.Helper()
	d, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func names(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}

func TestKeyCreated0600(t *testing.T) {
	dir := t.TempDir()
	key, id, err := LoadOrCreateKey(openDir(t, dir))
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 32 || bytes.Equal(key, make([]byte, 32)) {
		t.Fatalf("clé de %d octets : %x", len(key), key)
	}
	fi, err := os.Lstat(filepath.Join(dir, "snapshot.key"))
	if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm() != 0o600 || fi.Size() != 32 {
		t.Fatalf("snapshot.key : %v %v", fi, err)
	}
	again, id2, err := LoadOrCreateKey(openDir(t, dir))
	if err != nil || !bytes.Equal(again, key) || id2 != id {
		t.Fatalf("seconde lecture : %x %s %v", again, id2, err)
	}
	if got := names(t, dir); len(got) != 1 {
		t.Fatalf("dossier : %v, attendu snapshot.key seul", got)
	}
}

func writeKey(t *testing.T, dir string, b []byte, mode os.FileMode) {
	t.Helper()
	p := filepath.Join(dir, "snapshot.key")
	if err := os.WriteFile(p, b, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
}

func TestKeyRefusesSymlink(t *testing.T) {
	dir, other := t.TempDir(), t.TempDir()
	target := filepath.Join(other, "k")
	if err := os.WriteFile(target, bytes.Repeat([]byte{7}, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "snapshot.key")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadOrCreateKey(openDir(t, dir)); err == nil {
		t.Fatal("clé lue à travers un lien")
	}
}

func TestKeyRefusesGroupReadable(t *testing.T) {
	dir := t.TempDir()
	writeKey(t, dir, bytes.Repeat([]byte{7}, 32), 0o640)
	if _, _, err := LoadOrCreateKey(openDir(t, dir)); err == nil {
		t.Fatal("clé lisible par le groupe acceptée")
	}
}

func TestKeyRefusesOtherOwner(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("ce test demande root (scripts/test-linux.sh)")
	}
	dir := t.TempDir()
	writeKey(t, dir, bytes.Repeat([]byte{7}, 32), 0o600)
	if err := os.Chown(filepath.Join(dir, "snapshot.key"), 12345, 12345); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadOrCreateKey(openDir(t, dir)); err == nil {
		t.Fatal("clé d'un autre propriétaire acceptée")
	}
}

func TestKeyRefusesWrongLength(t *testing.T) {
	for _, n := range []int{0, 31, 33} {
		dir := t.TempDir()
		writeKey(t, dir, bytes.Repeat([]byte{7}, n), 0o600)
		if _, _, err := LoadOrCreateKey(openDir(t, dir)); err == nil {
			t.Errorf("clé de %d octets acceptée", n)
		}
	}
}

func TestKeyConcurrentCreate(t *testing.T) {
	for range 20 {
		dir := t.TempDir()
		const n = 8
		keys := make([][]byte, n)
		errs := make([]error, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Go(func() {
				d, err := os.Open(dir)
				if err != nil {
					errs[i] = err
					return
				}
				defer d.Close()
				keys[i], _, errs[i] = LoadOrCreateKey(d)
			})
		}
		wg.Wait()
		for i := range n {
			if errs[i] != nil || !bytes.Equal(keys[i], keys[0]) {
				t.Fatalf("création %d : %x %v (première : %x)", i, keys[i], errs[i], keys[0])
			}
		}
		if got := names(t, dir); len(got) != 1 || got[0] != "snapshot.key" {
			t.Fatalf("dossier : %v, attendu snapshot.key seul (aucun fichier temporaire)", got)
		}
	}
}

func TestKeyNoPartialFile(t *testing.T) {
	dir := t.TempDir()
	old := beforeKeyPublish
	t.Cleanup(func() { beforeKeyPublish = old })
	beforeKeyPublish = func() error { return errors.New("arrêt simulé") }
	if _, _, err := LoadOrCreateKey(openDir(t, dir)); err == nil {
		t.Fatal("échec simulé ignoré")
	}
	if got := names(t, dir); len(got) != 0 {
		t.Fatalf("dossier : %v, attendu vide (ni snapshot.key ni fichier temporaire)", got)
	}
}

func TestKeyRotate(t *testing.T) {
	dir := t.TempDir()
	key, id, err := LoadOrCreateKey(openDir(t, dir))
	if err != nil {
		t.Fatal(err)
	}
	id2, err := RotateKey(openDir(t, dir))
	if err != nil || id2 == id {
		t.Fatalf("rotation : %s (avant %s) %v", id2, id, err)
	}
	key2, id3, err := LoadOrCreateKey(openDir(t, dir))
	if err != nil || bytes.Equal(key2, key) || id3 != id2 {
		t.Fatalf("après rotation : %s %v", id3, err)
	}
	fi, _ := os.Lstat(filepath.Join(dir, "snapshot.key"))
	if fi.Mode().Perm() != 0o600 || fi.Size() != 32 {
		t.Fatalf("snapshot.key après rotation : %v", fi)
	}
	if got := names(t, dir); len(got) != 1 {
		t.Fatalf("dossier : %v, attendu snapshot.key seul", got)
	}
	// Snapshots of two keys compare by their metadata only.
	if ContentComparable(Header{KeyID: id}, Header{KeyID: id2}) {
		t.Fatal("contenus comparables entre deux clés")
	}
	if !ContentComparable(Header{KeyID: id2}, Header{KeyID: id2}) {
		t.Fatal("contenus non comparables avec la même clé")
	}
	if ContentComparable(Header{}, Header{}) {
		t.Fatal("contenus comparables sans clé (photo des métadonnées)")
	}
}

func TestKeyIDStable(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	m := hmac.New(sha256.New, key)
	m.Write([]byte("hublot-snapshot-key-id"))
	want := hex.EncodeToString(m.Sum(nil)[:8])
	if got := KeyID(key); got != want || KeyID(key) != got {
		t.Fatalf("KeyID = %s, attendu %s", got, want)
	}
	if KeyID(bytes.Repeat([]byte{0x43}, 32)) == want {
		t.Fatal("deux clés, même KeyID")
	}
}
