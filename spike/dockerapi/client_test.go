package dockerapi

import (
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// fakeDocker serves h on a unix socket and returns a Client pointed at it.
func fakeDocker(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(h)
	srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return New(sock)
}

func TestInspect(t *testing.T) {
	c := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.44/containers/web/json" || r.URL.Query().Get("size") != "1" {
			t.Errorf("unexpected request %s", r.URL)
		}
		w.Write([]byte(`{"Id":"abc123","Name":"/web","State":{"Running":true,"Pid":42,"StartedAt":"t0","FinishedAt":""},"SizeRootFs":2048}`))
	})
	ct, err := c.Inspect(context.Background(), "web", true)
	if err != nil {
		t.Fatal(err)
	}
	if ct.ID != "abc123" || ct.State.Pid != 42 || !ct.State.Running || ct.SizeRootFs == nil || *ct.SizeRootFs != 2048 {
		t.Fatalf("bad decode: %+v", ct)
	}
}

func TestInspectErrorCarriesStatus(t *testing.T) {
	c := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"No such container: nope"}`, http.StatusNotFound)
	})
	_, err := c.Inspect(context.Background(), "nope", false)
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("want 404 error, got %v", err)
	}
}

func TestChanges(t *testing.T) {
	c := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"Path":"/added.txt","Kind":1},{"Path":"/etc/issue","Kind":2}]`))
	})
	ch, err := c.Changes(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	if len(ch) != 2 || ch[0].Path != "/added.txt" || ch[0].Kind != 1 || ch[1].Kind != 2 {
		t.Fatalf("bad decode: %+v", ch)
	}
}

func TestArchiveStat(t *testing.T) {
	stat := base64.StdEncoding.EncodeToString([]byte(`{"name":"os-release","size":12,"mode":420,"mtime":"m","linkTarget":""}`))
	c := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead || r.URL.Query().Get("path") != "/etc/os-release" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		w.Header().Set("X-Docker-Container-Path-Stat", stat)
	})
	st, err := c.ArchiveStat(context.Background(), "web", "/etc/os-release")
	if err != nil {
		t.Fatal(err)
	}
	if st.Name != "os-release" || st.Size != 12 || st.Mode != 420 {
		t.Fatalf("bad decode: %+v", st)
	}
}
