package dockerx

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moby/moby/client"
)

// A daemon that answers /_ping and /version, as Docker 29.8 does.
func fakeDaemon(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			w.Header().Set("Api-Version", "1.44")
			w.Header().Set("Ostype", "linux")
			_, _ = w.Write([]byte("OK"))
		case strings.HasSuffix(r.URL.Path, "/version"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Version":"29.8.2","ApiVersion":"1.44","MinAPIVersion":"1.24"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func engineFor(t *testing.T, host string) *engine {
	t.Helper()
	c, err := client.New(client.WithHost(host))
	if err != nil {
		t.Fatal(err)
	}
	return &engine{c: c}
}

// Spec v0.2 §5.3: the list never asks Docker for sizes (565 ms against 69
// ms for 39 containers, measured); ContainerSize and ImageInspect read what
// the daemon answers, a missing size staying nil.
func TestSizesOnlyWhereAsked(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Path+"?"+r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			w.Header().Set("Api-Version", "1.44")
			_, _ = w.Write([]byte("OK"))
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			_, _ = w.Write([]byte(`[{"Id":"abc","Names":["/web"],"Labels":{"com.docker.compose.project":"shop"}}]`))
		case strings.HasSuffix(r.URL.Path, "/containers/abc/json"):
			_, _ = w.Write([]byte(`{"Id":"abc","SizeRw":1234,"SizeRootFs":5678}`))
		case strings.HasSuffix(r.URL.Path, "/containers/nosize/json"):
			_, _ = w.Write([]byte(`{"Id":"nosize"}`))
		case strings.HasSuffix(r.URL.Path, "/images/sha256:feed/json"):
			_, _ = w.Write([]byte(`{"Id":"sha256:feed","RepoTags":["nginx:1.27"],"Size":999,"Created":"2026-10-01T00:00:00Z"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"no such object"}`))
		}
	}))
	t.Cleanup(srv.Close)
	e := engineFor(t, "tcp://"+srv.Listener.Addr().String())
	ctx := context.Background()
	list, err := e.ListContainers(ctx)
	if err != nil || len(list) != 1 || list[0].Project != "shop" {
		t.Fatalf("liste : %+v %v", list, err)
	}
	for _, q := range queries {
		if strings.HasSuffix(strings.Split(q, "?")[0], "/containers/json") && strings.Contains(q, "size=") {
			t.Errorf("la liste demande les tailles : %s", q)
		}
	}
	rw, root, err := e.ContainerSize(ctx, "abc")
	if err != nil || rw == nil || root == nil || *rw != 1234 || *root != 5678 {
		t.Errorf("ContainerSize : %v %v %v", rw, root, err)
	}
	if last := queries[len(queries)-1]; !strings.Contains(last, "size=1") {
		t.Errorf("ContainerSize sans size : %s", last)
	}
	if rw, root, err := e.ContainerSize(ctx, "nosize"); err != nil || rw != nil || root != nil {
		t.Errorf("tailles absentes : %v %v %v", rw, root, err)
	}
	img, err := e.ImageInspect(ctx, "sha256:feed")
	if err != nil || img.ID != "sha256:feed" || img.Size != 999 || len(img.Tags) != 1 || img.Created == "" {
		t.Errorf("ImageInspect : %+v %v", img, err)
	}
	if _, err := e.ImageInspect(ctx, "sha256:gone"); !errors.Is(err, ErrNotFound) {
		t.Errorf("image absente : %v", err)
	}
}

func TestInfoConnected(t *testing.T) {
	srv := fakeDaemon(t)
	host := "tcp://" + srv.Listener.Addr().String()
	info, err := engineFor(t, host).Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := DaemonInfo{Host: host, ServerVersion: "29.8.2", APIVersion: "1.44", Connected: true}
	if info != want {
		t.Fatalf("info = %+v, want %+v", info, want)
	}
}

// Spec v0.2 §5.10: connected comes from the Ping. A daemon that answers the
// Ping but not /version stays connected, without a version.
func TestInfoVersionFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/_ping") {
			w.Header().Set("Api-Version", "1.44")
			_, _ = w.Write([]byte("OK"))
			return
		}
		http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	info, err := engineFor(t, "tcp://"+srv.Listener.Addr().String()).Info(context.Background())
	if err == nil || !info.Connected || info.ServerVersion != "" || info.APIVersion != "1.44" {
		t.Fatalf("info = %+v, err = %v", info, err)
	}
}

func TestInfoUnreachable(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host := "tcp://" + l.Addr().String()
	l.Close() // nothing listens there any more
	info, err := engineFor(t, host).Info(context.Background())
	if err == nil || info.Connected || info.Host != host || info.ServerVersion != "" {
		t.Fatalf("info = %+v, err = %v", info, err)
	}
}

func TestFakeInfo(t *testing.T) {
	f := &Fake{Daemon: DaemonInfo{Host: "unix:///var/run/docker.sock", Connected: true}}
	if info, err := f.Info(context.Background()); err != nil || !info.Connected {
		t.Fatalf("%+v %v", info, err)
	}
	f.Err = ErrUnavailable
	if info, err := f.Info(context.Background()); err == nil || info.Connected || info.Host == "" {
		t.Fatalf("Docker down: %+v %v", info, err)
	}
}
