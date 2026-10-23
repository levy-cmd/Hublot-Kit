//go:build integration

package fsview

// The files of a real image (spec v0.2 §5.5, plan v0.2 tranche 6 step 6):
// an image built here on alpine:3, with a whiteout, a directory removed then
// recreated (opaque or whited out, as the builder writes it) and a hard
// link, read by its export, without a container. Run on both stores of the
// CI (overlay2, containerd) and in the VMs.

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

const testImageDockerfile = `FROM alpine:3
RUN mkdir /d && echo a > /d/x && echo f1 > /f
RUN rm /f && rm -r /d && mkdir /d && echo y > /d/y && ln /bin/busybox /bin/hardlink-test
RUN echo f3 > /f
`

// imageReady polls ForImage until the index is built.
func imageReady(t *testing.T, v *Views, id string) (ImageFS, time.Duration) {
	t.Helper()
	start := time.Now()
	for deadline := time.Now().Add(2 * time.Minute); ; time.Sleep(20 * time.Millisecond) {
		fsys, err := v.ForImage(context.Background(), id)
		if !errors.Is(err, ErrIndexing) {
			if err != nil {
				t.Fatalf("ForImage(%s) : %v", id, err)
			}
			return fsys, time.Since(start)
		}
		if time.Now().After(deadline) {
			t.Fatalf("index jamais prêt : %v", err)
		}
	}
}

// imageRead polls Open while the extraction runs.
func imageRead(t *testing.T, fsys ImageFS, p string) ([]byte, time.Duration) {
	t.Helper()
	start := time.Now()
	for deadline := time.Now().Add(2 * time.Minute); ; time.Sleep(20 * time.Millisecond) {
		rc, err := fsys.Open(context.Background(), p)
		if errors.Is(err, ErrIndexing) {
			if time.Now().After(deadline) {
				t.Fatalf("extraction de %s jamais finie", p)
			}
			continue
		}
		if err != nil {
			t.Fatalf("Open(%s) : %v", p, err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		return b, time.Since(start)
	}
}

func TestImageIndexReal(t *testing.T) {
	const tag = "hublot-it-image:latest"
	exec.Command("docker", "rmi", "-f", tag).Run()
	t.Cleanup(func() { exec.Command("docker", "rmi", "-f", tag).Run() })
	cmd := exec.Command("docker", "build", "-q", "-t", tag, "-")
	cmd.Stdin = strings.NewReader(testImageDockerfile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("docker build : %v\n%s", err, out)
	}
	id := dockerCmd(t, "image", "inspect", "-f", "{{.Id}}", tag)
	t.Logf("pilote : %s, image %s", dockerCmd(t, "info", "-f", "{{.Driver}}"), id)
	dc, err := dockerx.New()
	if err != nil {
		t.Fatal(err)
	}
	v := NewViews(dc, Options{})
	fsys, took := imageReady(t, v, id)
	t.Logf("index de l'image de test : %v", took)
	ctx := context.Background()

	entries, _, err := fsys.List(ctx, "/d")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, e := range entries {
		names = append(names, e.Name)
	}
	if !slices.Equal(names, []string{"y"}) {
		t.Errorf("List(/d) = %v, attendu [y] (dossier supprimé puis recréé)", names)
	}
	if _, err := fsys.Stat(ctx, "/d/x"); err == nil {
		t.Error("/d/x visible : le blanchiment n'est pas appliqué")
	}
	if b, took := imageRead(t, fsys, "/f"); string(b) != "f3\n" {
		t.Errorf("Open(/f) = %q, attendu la version recréée", b)
	} else {
		t.Logf("aperçu de /f : %v", took)
	}
	if b, _ := imageRead(t, fsys, "/d/y"); string(b) != "y\n" {
		t.Errorf("Open(/d/y) = %q", b)
	}
	bb, err1 := fsys.Stat(ctx, "/bin/busybox")
	hl, err2 := fsys.Stat(ctx, "/bin/hardlink-test")
	if err1 != nil || err2 != nil || hl.Type != TypeFile || hl.Size != bb.Size || hl.Size == 0 {
		t.Errorf("lien physique : %+v %v / %+v %v", bb, err1, hl, err2)
	}
	if b, _ := imageRead(t, fsys, "/bin/hardlink-test"); int64(len(b)) != hl.Size {
		t.Errorf("Open(/bin/hardlink-test) : %d octets, attendu %d", len(b), hl.Size)
	}
	// The layer of alpine:3, under the three of the test.
	if b, _ := imageRead(t, fsys, "/etc/os-release"); !strings.Contains(string(b), "Alpine") {
		t.Errorf("/etc/os-release : %q", b)
	}
	if hits, _, indexed := v.SearchImage(ctx, id, "hardlink", 10); !indexed || len(hits) != 1 {
		t.Errorf("recherche : %v %v", hits, indexed)
	}
}

// HUBLOT_IT_MEASURE=<image> measures the index of an image already present
// and the preview of one of its files (HUBLOT_IT_FILE, /etc/os-release by
// default): plan v0.2 tranche 6 step 7. Nothing is pulled.
func TestImageIndexRealMeasure(t *testing.T) {
	ref := os.Getenv("HUBLOT_IT_MEASURE")
	if ref == "" {
		t.Skip("HUBLOT_IT_MEASURE absent")
	}
	file := os.Getenv("HUBLOT_IT_FILE")
	if file == "" {
		file = "/etc/os-release"
	}
	id := dockerCmd(t, "image", "inspect", "-f", "{{.Id}}", ref)
	dc, err := dockerx.New()
	if err != nil {
		t.Fatal(err)
	}
	fsys, took := imageReady(t, NewViews(dc, Options{}), id)
	n := 0
	var walk func(string)
	walk = func(d string) {
		entries, _, _ := fsys.List(context.Background(), d)
		for _, e := range entries {
			n++
			if e.Type == TypeDir {
				walk(childPath(d, e.Name))
			}
		}
	}
	walk("/")
	b, read := imageRead(t, fsys, file)
	t.Logf("%s : index en %v (%d entrées visibles), aperçu de %s (%d octets) en %v", ref, took, n, file, len(b), read)
}
