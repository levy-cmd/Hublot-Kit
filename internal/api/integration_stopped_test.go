//go:build integration

package api

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"slices"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

func TestIntegrationStoppedAndChanges(t *testing.T) {
	name := "hublot-it-api-stopped"
	exec.Command("docker", "rm", "-f", name).Run()
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })
	if out, err := exec.Command("docker", "run", "--name", name, "alpine:3", "sh", "-c", "echo bonjour > /root/note.txt").CombinedOutput(); err != nil {
		t.Fatalf("docker run : %v %s", err, out)
	}
	dc, err := dockerx.New()
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{}
	f.init(t, false, dc, nil) // real Views, DefaultOptions
	c := f.session(t)
	base := "/api/containers/" + name
	rec := f.do("GET", base+"/fs?path=/root", "", c)
	for deadline := time.Now().Add(60 * time.Second); rec.Code == http.StatusAccepted && time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		rec = f.do("GET", base+"/fs?path=/root", "", c)
	}
	if rec.Code == http.StatusAccepted {
		t.Fatalf("indexation non terminée après 60 s : %s", rec.Body)
	}
	decodeListing(t, rec)
	if rec := f.do("GET", base+"/fs/file?path=/root/note.txt", "", c); rec.Code != http.StatusOK || rec.Body.String() != "bonjour\n" {
		t.Errorf("aperçu : %d %q", rec.Code, rec.Body)
	}
	rec = f.do("GET", base+"/fs/download?path=/root", "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("tar : %d", rec.Code)
	}
	found := false
	tr := tar.NewReader(bytes.NewReader(rec.Body.Bytes()))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar : %v", err)
		}
		if h.Name == "root/note.txt" {
			found = true
			if b, err := io.ReadAll(tr); err != nil || string(b) != "bonjour\n" {
				t.Errorf("contenu de root/note.txt : %q %v", b, err)
			}
		}
	}
	if !found {
		t.Error("root/note.txt absent du tar")
	}
	rec = f.do("GET", base+"/changes", "", c)
	var cs []dockerx.Change
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &cs) != nil ||
		!slices.Contains(cs, dockerx.Change{Path: "/root/note.txt", Kind: "added"}) {
		t.Errorf("changes : %d %s", rec.Code, rec.Body)
	}
}
