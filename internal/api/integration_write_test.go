//go:build integration

package api

// Writes end to end on a stopped container (spec §5.3, §10): real Docker,
// real Views. Portable: Docker Desktop answers from macOS.

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// whileIndexing repeats a request while it answers 202 (60 s at most).
func whileIndexing(t *testing.T, do func() *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	t.Helper()
	rec := do()
	for deadline := time.Now().Add(60 * time.Second); rec.Code == http.StatusAccepted && time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		rec = do()
	}
	if rec.Code == http.StatusAccepted {
		t.Fatalf("indexation non terminée après 60 s : %s", rec.Body)
	}
	return rec
}

func TestIntegrationWriteStopped(t *testing.T) {
	name, ro := "hublot-it-write-stopped", "hublot-it-write-ro"
	exec.Command("docker", "rm", "-f", name, ro).Run()
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name, ro).Run() })
	if out, err := exec.Command("docker", "run", "--name", name, "alpine:3", "sh", "-c", "echo v1 > /etc/app.conf").CombinedOutput(); err != nil {
		t.Fatalf("docker run : %v %s", err, out)
	}
	if out, err := exec.Command("docker", "create", "--name", ro, "--read-only", "alpine:3").CombinedOutput(); err != nil {
		t.Fatalf("docker create : %v %s", err, out)
	}
	dc, err := dockerx.New()
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{}
	f.init(t, false, dc, nil) // real Views, DefaultOptions
	c := f.session(t)
	file := "/api/containers/" + name + "/fs/file?path="
	preview := func(p string) func() *httptest.ResponseRecorder {
		return func() *httptest.ResponseRecorder { return f.do("GET", file+p, "", c) }
	}
	rec := whileIndexing(t, preview("/etc/app.conf"))
	if rec.Code != http.StatusOK || rec.Body.String() != "v1\n" {
		t.Fatalf("aperçu : %d %q", rec.Code, rec.Body)
	}
	tag := rec.Header().Get("ETag")
	if rec := f.send("PUT", file+"/etc/app.conf", []byte("v2\n"), c, map[string]string{"If-Match": tag}); rec.Code != http.StatusOK {
		t.Fatalf("édition : %d %s", rec.Code, rec.Body)
	}
	// The index was dropped: the next access builds it again and sees v2.
	if rec := whileIndexing(t, preview("/etc/app.conf")); rec.Code != http.StatusOK || rec.Body.String() != "v2\n" {
		t.Fatalf("relu après l'édition : %d %q", rec.Code, rec.Body)
	}
	if rec := f.send("PUT", file+"/etc/app.conf", []byte("v3\n"), c, map[string]string{"If-Match": tag}); rec.Code != http.StatusConflict {
		t.Fatalf("ancien If-Match : %d %s", rec.Code, rec.Body)
	}
	rec = whileIndexing(t, func() *httptest.ResponseRecorder { return f.upload(t, c, name, "/etc", "new.conf", []byte("n\n"), "*") })
	if rec.Code != http.StatusCreated {
		t.Fatalf("envoi : %d %s", rec.Code, rec.Body)
	}
	if rec := whileIndexing(t, preview("/etc/new.conf")); rec.Code != http.StatusOK || rec.Body.String() != "n\n" {
		t.Fatalf("fichier envoyé : %d %q", rec.Code, rec.Body)
	}
	// --read-only: Docker refuses the write itself → read_only, its message as detail.
	rec = whileIndexing(t, func() *httptest.ResponseRecorder { return f.upload(t, c, ro, "/etc", "x", []byte("x"), "*") })
	if rec.Code != http.StatusForbidden || errCode(t, rec) != "read_only" {
		t.Fatalf("rootfs en lecture seule : %d %s", rec.Code, rec.Body)
	}
	t.Logf("MESURE refus de Docker : %s", rec.Body)
}
