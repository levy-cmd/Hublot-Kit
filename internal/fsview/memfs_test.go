package fsview

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"
)

func TestMemFS(t *testing.T) {
	ctx := context.Background()
	m := NewMemFS().AddDir("/etc").AddFile("/etc/hosts", []byte("127.0.0.1")).AddLink("/etc/l", "hosts").
		AddDir("/ld").AddLink("/link", "/etc")
	entries, _, err := m.List(ctx, "/etc")
	if err != nil || len(entries) != 2 || entries[0].Name != "hosts" || entries[1].LinkTarget != "hosts" {
		t.Fatalf("List : %+v %v", entries, err)
	}
	if _, _, err := m.List(ctx, "/etc/hosts"); !errors.Is(err, ErrNotDir) {
		t.Fatalf("List d'un fichier : %v", err)
	}
	rc, err := m.Open(ctx, "/etc/hosts")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(rc); string(b) != "127.0.0.1" {
		t.Fatalf("Open : %q", b)
	}
	if _, err := m.Open(ctx, "/etc"); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("Open d'un dossier : %v", err)
	}
	if _, err := m.Stat(ctx, "/absent"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("absent : %v", err)
	}
	m.AddDir("/link/sub")
	if _, err := m.StatNoLinks(ctx, "/link/sub"); !errors.Is(err, ErrLinkInPath) {
		t.Fatalf("lien parent : %v", err)
	}
	if _, err := m.StatNoLinks(ctx, "/etc/l"); !errors.Is(err, ErrLinkInPath) {
		t.Fatalf("lien final : %v", err)
	}
	if _, err := m.StatNoLinks(ctx, "/etc/hosts"); err != nil {
		t.Fatal(err)
	}
	if err := m.CheckArchive(ctx, "/etc"); err != nil {
		t.Fatalf("CheckArchive : %v", err)
	}
	m.ArchiveErr = ErrForbiddenPath
	if err := m.CheckArchive(ctx, "/etc"); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("ArchiveErr : %v", err)
	}
	m.OpenFile = func(p string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("hook:" + p)), nil }
	rc, _ = m.Open(ctx, "/etc/hosts")
	if b, _ := io.ReadAll(rc); string(b) != "hook:/etc/hosts" {
		t.Fatalf("OpenFile : %q", b)
	}
	m.Err = ErrForbiddenPath
	if _, err := m.Stat(ctx, "/etc"); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("Err : %v", err)
	}
	m.Close()
	if m.Closed.Load() != 1 {
		t.Fatal("Close non compté")
	}
}
