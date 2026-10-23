package dockerx

import (
	"context"
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
