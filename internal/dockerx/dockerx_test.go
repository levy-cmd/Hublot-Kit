package dockerx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"reflect"
	"slices"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
)

func TestFromSummary(t *testing.T) {
	s := container.Summary{
		ID:      "0123456789abcdef",
		Names:   []string{"/web"},
		Image:   "nginx:1.27",
		Created: 1_760_000_000,
		State:   container.ContainerState("running"),
		Status:  "Up 3 hours",
		Ports: []container.PortSummary{
			{IP: netip.MustParseAddr("0.0.0.0"), PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
			{IP: netip.MustParseAddr("::"), PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
			{PrivatePort: 443, Type: "tcp"},
		},
	}
	c := fromSummary(s)
	if c.ID != s.ID || c.Name != "web" || c.Image != "nginx:1.27" || c.State != "running" || c.Status != "Up 3 hours" {
		t.Fatalf("bad mapping: %+v", c)
	}
	if !c.Created.Equal(time.Unix(1_760_000_000, 0)) {
		t.Fatalf("created = %v", c.Created)
	}
	if len(c.Ports) != 2 {
		t.Fatalf("IPv4/IPv6 duplicate must collapse: %+v", c.Ports)
	}
	if c.Ports[0] != (Port{Private: 80, Public: 8080, Type: "tcp", IP: "0.0.0.0"}) || c.Ports[1] != (Port{Private: 443, Type: "tcp"}) {
		t.Fatalf("ports = %+v", c.Ports)
	}
}

func TestFromSummaryWithoutName(t *testing.T) {
	c := fromSummary(container.Summary{ID: "0123456789abcdef0123"})
	if c.Name != "0123456789ab" {
		t.Fatalf("name falls back to the short id, got %q", c.Name)
	}
	if c.Ports == nil {
		t.Fatal("ports must be an empty slice, not nil (JSON [])")
	}
}

func TestFromSummaryUnspecifiedOrder(t *testing.T) {
	c := fromSummary(container.Summary{ID: "x", Ports: []container.PortSummary{
		{IP: netip.MustParseAddr("::"), PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
		{IP: netip.MustParseAddr("0.0.0.0"), PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
	}})
	if len(c.Ports) != 1 || c.Ports[0].IP != "0.0.0.0" {
		t.Fatalf("ports = %+v", c.Ports)
	}
}

func TestFromSummarySpecificIPsStaySeparate(t *testing.T) {
	c := fromSummary(container.Summary{ID: "x", Ports: []container.PortSummary{
		{IP: netip.MustParseAddr("127.0.0.1"), PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
		{IP: netip.MustParseAddr("192.168.1.5"), PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
	}})
	if len(c.Ports) != 2 || c.Ports[0].IP != "127.0.0.1" || c.Ports[1].IP != "192.168.1.5" {
		t.Fatalf("ports = %+v", c.Ports)
	}
}

func TestFromSummaryNameSkipsLinkAlias(t *testing.T) {
	c := fromSummary(container.Summary{ID: "x", Names: []string{"/other/alias", "/web"}})
	if c.Name != "web" {
		t.Fatalf("name = %q", c.Name)
	}
}

func TestFromInspect(t *testing.T) {
	got := fromInspect(container.InspectResponse{
		ID: "abc", Name: "/web", State: &container.State{Running: true, Pid: 42},
		Mounts: []container.MountPoint{
			{Type: "bind", Source: "/proc", Destination: "/host/proc"},
			{Type: "volume", Name: "data", Source: "/var/lib/docker/volumes/data/_data", Destination: "/data", Driver: "local"},
		},
	})
	want := ContainerInfo{ID: "abc", Name: "web", Running: true, Pid: 42, Mounts: []Mount{
		{Type: "bind", Source: "/proc", Destination: "/host/proc"},
		{Type: "volume", Name: "data", Source: "/var/lib/docker/volumes/data/_data", Destination: "/data", Driver: "local"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	if got := fromInspect(container.InspectResponse{ID: "x"}); got.Running || got.Pid != 0 || got.Mounts != nil {
		t.Fatalf("a nil State must read as stopped: %+v", got)
	}
}

func TestWrapClassifiesErrors(t *testing.T) {
	if err := wrap(fmt.Errorf("inspect: %w", cerrdefs.ErrNotFound)); !errors.Is(err, ErrNotFound) || errors.Is(err, ErrUnavailable) {
		t.Fatalf("not found: %v", err)
	}
	if err := wrap(errors.New("dial unix /var/run/docker.sock: connect: no such file or directory")); !errors.Is(err, ErrUnavailable) || errors.Is(err, ErrNotFound) {
		t.Fatalf("unavailable: %v", err)
	}
}

func TestFakeInspectAndArchive(t *testing.T) {
	f := &Fake{
		Infos:    map[string]ContainerInfo{"web": {ID: "full", Name: "web", Running: true, Pid: 7}},
		Archives: map[string][]byte{"full:/etc": []byte("TAR")},
	}
	if info, err := f.Inspect(context.Background(), "web"); err != nil || info.ID != "full" {
		t.Fatalf("inspect: %+v %v", info, err)
	}
	if _, err := f.Inspect(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown container: %v", err)
	}
	rc, err := f.GetArchive(context.Background(), "full", "/etc")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	if string(b) != "TAR" || len(f.ArchiveCalls) != 1 || f.ArchiveCalls[0] != "full:/etc" {
		t.Fatalf("archive %q, calls %v", b, f.ArchiveCalls)
	}
	if _, err := f.GetArchive(context.Background(), "full", "/absent"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("absent path: %v", err)
	}
	f.Volumes = map[string]map[string]string{"v": {"o": "bind", "device": "/srv"}}
	if opts, err := f.VolumeOptions(context.Background(), "v"); err != nil || opts["device"] != "/srv" {
		t.Fatalf("volume: %v %v", opts, err)
	}
	if _, err := f.VolumeOptions(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("absent volume: %v", err)
	}
}

func TestFromInspectFinishedAt(t *testing.T) {
	got := fromInspect(container.InspectResponse{ID: "x", State: &container.State{FinishedAt: "2026-10-09T10:00:00Z"}})
	if got.FinishedAt != "2026-10-09T10:00:00Z" {
		t.Fatalf("FinishedAt : %q", got.FinishedAt)
	}
}

func TestFromChanges(t *testing.T) {
	got, err := fromChanges([]container.FilesystemChange{
		{Kind: container.ChangeAdd, Path: "/a"}, {Kind: container.ChangeModify, Path: "/b"}, {Kind: container.ChangeDelete, Path: "/c"},
	})
	want := []Change{{"/a", "added"}, {"/b", "modified"}, {"/c", "deleted"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := fromChanges([]container.FilesystemChange{{Kind: 9, Path: "/x"}}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("type inconnu : %v", err)
	}
	if got, err := fromChanges(nil); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("aucun changement : %#v %v (attendu un tableau vide, pas nil)", got, err)
	}
}

func TestFakeSizesDiffsCalls(t *testing.T) {
	f := &Fake{Sizes: map[string]int64{"full": 42}, Diffs: map[string][]Change{"web": {{"/a", "added"}}},
		Archives: map[string][]byte{"full:/": []byte("T")}}
	if n, err := f.SizeRootFs(context.Background(), "full"); err != nil || n != 42 {
		t.Fatalf("taille : %d %v", n, err)
	}
	if cs, err := f.Changes(context.Background(), "web"); err != nil || len(cs) != 1 {
		t.Fatalf("diff : %v %v", cs, err)
	}
	if _, err := f.Changes(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("diff inconnu : %v", err)
	}
	f.GetArchive(context.Background(), "full", "/")
	if c := f.Calls(); !slices.Equal(c, []string{"full:/"}) {
		t.Fatalf("appels : %v", c)
	}
}
