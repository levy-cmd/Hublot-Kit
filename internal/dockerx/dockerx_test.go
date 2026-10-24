package dockerx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/network"
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

// Spec v0.2 §5.1: project and service come from the list's labels, and pass
// the label rule (§5.2): a value §7 would mask is not sent, and the
// container counts as "no project". No other label leaves through the list.
func TestContainersProject(t *testing.T) {
	c := fromSummary(container.Summary{ID: "x", Names: []string{"/shop-web-1"}, Labels: map[string]string{
		"com.docker.compose.project": "shop", "com.docker.compose.service": "web",
		"traefik.http.middlewares.auth.basicauth.users": "admin:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/",
	}})
	if c.Project != "shop" || c.Service != "web" {
		t.Fatalf("projet, service : %q %q", c.Project, c.Service)
	}
	b, _ := json.Marshal(c)
	if !strings.Contains(string(b), `"project":"shop","service":"web"`) || strings.Contains(string(b), "apr1") ||
		strings.Contains(string(b), "traefik") {
		t.Errorf("JSON : %s", b)
	}
	// Outside Compose: both fields absent.
	b, _ = json.Marshal(fromSummary(container.Summary{ID: "y"}))
	if strings.Contains(string(b), "project") || strings.Contains(string(b), "service") {
		t.Errorf("hors Compose : %s", b)
	}
	// A value §7 masks: dropped, never sent.
	m := fromSummary(container.Summary{ID: "z", Labels: map[string]string{
		"com.docker.compose.project": "postgres://u:s3cr3t@db/app", "com.docker.compose.service": "--password s3cr3t",
	}})
	if b, _ := json.Marshal(m); m.Project != "" || m.Service != "" || strings.Contains(string(b), "s3cr3t") {
		t.Errorf("FUITE : %s", b)
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

// Q15: read_only only for the two read-only refusals Docker gives (measured
// messages); any other InvalidArgument of PutArchive is ErrPutInvalid
// (conflict: the path changed under us). On the read routes an
// InvalidArgument stays docker_unavailable.
func TestWrapPut(t *testing.T) {
	invalid := func(msg string) error {
		return fmt.Errorf("Error response from daemon: %s: %w", msg, cerrdefs.ErrInvalidArgument)
	}
	for _, msg := range []string{"container rootfs is marked read-only", "mounted volume is marked read-only"} {
		if err := wrapPut(invalid(msg)); !errors.Is(err, ErrRefused) || errors.Is(err, ErrPutInvalid) || errors.Is(err, ErrUnavailable) {
			t.Errorf("%s : %v, attendu ErrRefused", msg, err)
		}
	}
	if err := wrapPut(invalid("extraction point is not a directory")); !errors.Is(err, ErrPutInvalid) || errors.Is(err, ErrRefused) {
		t.Errorf("point d'extraction : %v, attendu ErrPutInvalid", err)
	}
	if err := wrapPut(fmt.Errorf("put: %w", cerrdefs.ErrNotFound)); !errors.Is(err, ErrNotFound) {
		t.Errorf("introuvable : %v", err)
	}
	if err := wrap(invalid("container rootfs is marked read-only")); errors.Is(err, ErrRefused) || !errors.Is(err, ErrUnavailable) {
		t.Errorf("hors PutArchive, InvalidArgument reste ErrUnavailable : %v", err)
	}
}

// v0.1.2 (audit, constat 4): with DOCKER_HOST=tcp://user:password@…, the
// daemon's errors name the host with its credentials. No error of the
// client carries them, whatever the call; every one is classified.
func TestErrorsHideDockerHostCredentials(t *testing.T) {
	t.Setenv("DOCKER_HOST", "tcp://alice:s3cr3t-pw@127.0.0.1:1")
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, errList := c.ListContainers(ctx)
	_, errInspect := c.Inspect(ctx, "web")
	_, errArchive := c.GetArchive(ctx, "web", "/")
	_, errLogs := c.Logs(ctx, "web", LogsOptions{Tail: "all"})
	for name, err := range map[string]error{
		"Ping": c.Ping(ctx), "ListContainers": errList, "Inspect": errInspect, "GetArchive": errArchive,
		"PutArchive": c.PutArchive(ctx, "web", "/", strings.NewReader("")), "Logs": errLogs,
	} {
		if err == nil {
			t.Errorf("%s : pas d'erreur sur un démon injoignable", name)
			continue
		}
		if strings.Contains(err.Error(), "s3cr3t-pw") {
			t.Errorf("%s : %q montre le mot de passe de DOCKER_HOST", name, err)
		}
		if name != "Ping" && !errors.Is(err, ErrUnavailable) {
			t.Errorf("%s : %v, attendu ErrUnavailable", name, err)
		}
	}
	for _, err := range []error{
		wrap(errors.New("Cannot connect to the Docker daemon at tcp://alice:s3cr3t-pw@h:2375. Is the docker daemon running?")),
		wrapPut(fmt.Errorf("daemon at tcp://alice:s3cr3t-pw@h:2375: %w", cerrdefs.ErrInvalidArgument)),
		wrap(fmt.Errorf("daemon at tcp://alice:s3cr3t-pw@h:2375: %w", cerrdefs.ErrNotFound)),
	} {
		if strings.Contains(err.Error(), "s3cr3t-pw") {
			t.Errorf("%q montre le mot de passe", err)
		}
	}
}

// Q21: the --tmpfs destinations (HostConfig.Tmpfs), sorted.
func TestFromInspectTmpfs(t *testing.T) {
	got := fromInspect(container.InspectResponse{ID: "x", HostConfig: &container.HostConfig{Tmpfs: map[string]string{"/tmp": "", "/run": "size=1m"}}})
	if !reflect.DeepEqual(got.Tmpfs, []string{"/run", "/tmp"}) {
		t.Fatalf("Tmpfs : %v", got.Tmpfs)
	}
	if got := fromInspect(container.InspectResponse{ID: "x"}); got.Tmpfs != nil {
		t.Fatalf("sans HostConfig : %v", got.Tmpfs)
	}
}

func TestFakePutArchive(t *testing.T) {
	f := &Fake{}
	if err := f.PutArchive(context.Background(), "abc", "/etc", strings.NewReader("TAR")); err != nil {
		t.Fatal(err)
	}
	puts := f.PutCalls()
	if len(puts) != 1 || puts[0].ID != "abc" || puts[0].Dir != "/etc" || string(puts[0].Tar) != "TAR" {
		t.Fatalf("appels : %+v", puts)
	}
	f.PutErr = ErrRefused
	if err := f.PutArchive(context.Background(), "abc", "/etc", strings.NewReader("")); !errors.Is(err, ErrRefused) {
		t.Fatalf("PutErr : %v", err)
	}
	if len(f.PutCalls()) != 2 {
		t.Fatal("un appel en échec est enregistré aussi")
	}
}

// F7: what the radiography shows of `docker inspect`. Env stays raw here:
// package secrets masks it in the API (spec §7).
func TestFromInspectDetails(t *testing.T) {
	p80 := network.MustParsePort("80/tcp")
	p53 := network.MustParsePort("53/udp")
	p9000 := network.MustParsePort("9000/tcp")
	p90 := network.MustParsePort("90/tcp") // -p 90: published on a port chosen at start
	in := container.InspectResponse{
		ID: "abc", Name: "/web", Image: "sha256:feed", Created: "2026-10-10T07:00:00Z", RestartCount: 3,
		State: &container.State{Status: "running", Running: true, Pid: 7, ExitCode: 0, StartedAt: "2026-10-10T07:00:01Z",
			FinishedAt: "0001-01-01T00:00:00Z", OOMKilled: true, Health: &container.Health{Status: "unhealthy"}},
		Config: &container.Config{Image: "nginx:1.27", Env: []string{"A=1", "DB_PASSWORD=x"}, Cmd: []string{"nginx", "-g", "daemon off;"},
			Entrypoint: []string{"/docker-entrypoint.sh"}, WorkingDir: "/srv", User: "101",
			ExposedPorts: network.PortSet{p80: {}, p53: {}, p9000: {}}},
		HostConfig: &container.HostConfig{NetworkMode: "bridge",
			RestartPolicy: container.RestartPolicy{Name: "on-failure", MaximumRetryCount: 5},
			PortBindings:  network.PortMap{p80: {{HostPort: "8080"}}, p90: {{HostPort: ""}}}},
		NetworkSettings: &container.NetworkSettings{
			Ports: network.PortMap{
				p80: {{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: "8080"}, {HostIP: netip.MustParseAddr("::"), HostPort: "8080"}},
				p53: {{HostIP: netip.MustParseAddr("127.0.0.1"), HostPort: "5353"}},
			},
			Networks: map[string]*network.EndpointSettings{
				"front": {IPAddress: netip.MustParseAddr("172.18.0.2"), Gateway: netip.MustParseAddr("172.18.0.1"), Aliases: []string{"web"},
					MacAddress: network.HardwareAddr{2, 66, 172, 18, 0, 2}},
				"back": {},
			},
		},
		Mounts: []container.MountPoint{{Type: "volume", Name: "data", Source: "/var/lib/docker/volumes/data/_data", Destination: "/data", Driver: "local", Mode: "z", RW: true}},
	}
	got := fromInspect(in).Details
	want := Details{
		Image: "nginx:1.27", ImageID: "sha256:feed", Created: "2026-10-10T07:00:00Z",
		Status: "running", StartedAt: "2026-10-10T07:00:01Z", OOMKilled: true, Health: "unhealthy",
		RestartCount: 3, RestartPolicy: "on-failure", MaxRetry: 5,
		Entrypoint: []string{"/docker-entrypoint.sh"}, Cmd: []string{"nginx", "-g", "daemon off;"},
		WorkingDir: "/srv", User: "101", Env: []string{"A=1", "DB_PASSWORD=x"},
		NetworkMode: "bridge",
		Ports: []Port{ // running: the real bindings, 0.0.0.0 and :: once; exposed without binding too
			{Private: 53, Public: 5353, Type: "udp", IP: "127.0.0.1"},
			{Private: 80, Public: 8080, Type: "tcp", IP: "0.0.0.0"},
			{Private: 9000, Type: "tcp"},
		},
		Networks: []Network{{Name: "back"}, {Name: "front", IP: "172.18.0.2", Gateway: "172.18.0.1", MAC: "02:42:ac:12:00:02", Aliases: []string{"web"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
	if m := fromInspect(in).Mounts[0]; m.Mode != "z" || !m.RW {
		t.Errorf("montage : %+v", m)
	}
	// The cache key of the index keeps Docker's zero date (spec §5.2 step 1).
	if f := fromInspect(in).FinishedAt; f != "0001-01-01T00:00:00Z" {
		t.Errorf("FinishedAt brut : %q", f)
	}
	// Stopped, even if real bindings are left: the configured ones. HostIp ""
	// is every interface, 0.0.0.0 as when running; -p 90 has an IP but no
	// Public, unlike a port only exposed.
	in.State.Running = false
	configured := []Port{{Private: 53, Type: "udp"}, {Private: 80, Public: 8080, Type: "tcp", IP: "0.0.0.0"},
		{Private: 90, Type: "tcp", IP: "0.0.0.0"}, {Private: 9000, Type: "tcp"}}
	if ports := fromInspect(in).Details.Ports; !reflect.DeepEqual(ports, configured) {
		t.Errorf("ports configurés : %+v", ports)
	}
	in.NetworkSettings.Ports = nil
	if ports := fromInspect(in).Details.Ports; !reflect.DeepEqual(ports, configured) {
		t.Errorf("ports configurés, sans liaison réelle : %+v", ports)
	}
	// docker create: never started, Docker's zero dates are not shown.
	in.State.StartedAt = "0001-01-01T00:00:00Z"
	if d := fromInspect(in).Details; d.StartedAt != "" {
		t.Errorf("date zéro : %q", d.StartedAt)
	}
	// Host network or another container's: Docker ignores -p, only the
	// exposed ports, running (Ports is {}) or not.
	exposedOnly := []Port{{Private: 53, Type: "udp"}, {Private: 80, Type: "tcp"}, {Private: 9000, Type: "tcp"}}
	for _, mode := range []container.NetworkMode{"host", "container:db"} {
		for _, running := range []bool{true, false} {
			in.HostConfig.NetworkMode, in.State.Running, in.NetworkSettings.Ports = mode, running, network.PortMap{}
			if ports := fromInspect(in).Details.Ports; !reflect.DeepEqual(ports, exposedOnly) {
				t.Errorf("%s, running %v : %+v", mode, running, ports)
			}
		}
	}
	if d := fromInspect(container.InspectResponse{ID: "x"}).Details; !reflect.DeepEqual(d, Details{}) {
		t.Errorf("Inspect vide : %+v", d)
	}
}

// Spec v0.2 §5.4: the configuration of an image, raw (package secrets masks
// it), its tags without "<none>:<none>", its exposed ports sorted.
func TestFromImageInspect(t *testing.T) {
	var res image.InspectResponse
	if err := json.Unmarshal([]byte(`{"Id":"sha256:abc","RepoTags":["<none>:<none>","app:1"],"Size":42,
		"Created":"2026-10-01T12:00:00Z","Os":"linux","Architecture":"arm64","Variant":"v8",
		"Config":{"Env":["A=1"],"Cmd":["run"],"Entrypoint":["/e"],"WorkingDir":"/w","User":"u",
		"ExposedPorts":{"8080/tcp":{},"53/udp":{}},"Labels":{"k":"v"}}}`), &res); err != nil {
		t.Fatal(err)
	}
	got := fromImageInspect(res)
	want := ImageInfo{ID: "sha256:abc", Tags: []string{"app:1"}, Size: 42, Created: "2026-10-01T12:00:00Z",
		OS: "linux", Architecture: "arm64", Variant: "v8", Env: []string{"A=1"}, Cmd: []string{"run"},
		Entrypoint: []string{"/e"}, WorkingDir: "/w", User: "u", ExposedPorts: []string{"53/udp", "8080/tcp"},
		Labels: map[string]string{"k": "v"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("\n obtenu  %+v\n attendu %+v", got, want)
	}
	if got := fromImageInspect(image.InspectResponse{ID: "sha256:x"}); got.Tags == nil || len(got.Tags) != 0 || got.Env != nil {
		t.Fatalf("sans configuration : %+v", got)
	}
}

func TestFromImageSummaries(t *testing.T) {
	got := fromImageSummaries([]image.Summary{
		{ID: "sha256:a", RepoTags: []string{"a:1"}, Size: 7, Created: 1_760_000_000},
		{ID: "sha256:b", RepoTags: []string{"<none>:<none>"}},
		{ID: "sha256:c"},
	})
	if len(got) != 3 || !reflect.DeepEqual(got[0], ImageSummary{ID: "sha256:a", Tags: []string{"a:1"}, Size: 7, Created: time.Unix(1_760_000_000, 0).UTC()}) {
		t.Fatalf("%+v", got)
	}
	for _, s := range got[1:] {
		if s.Tags == nil || len(s.Tags) != 0 {
			t.Errorf("sans étiquette : %+v", s)
		}
	}
}

func TestFromHistory(t *testing.T) {
	got := fromHistory([]image.HistoryResponseItem{
		{CreatedBy: `CMD ["sh"]`, Created: 1_760_000_000},
		{CreatedBy: "ADD rootfs", Created: 0, Size: 9},
	})
	want := []ImageLayer{
		{CreatedBy: `CMD ["sh"]`, Created: time.Unix(1_760_000_000, 0).UTC(), Empty: true},
		{CreatedBy: "ADD rootfs", Size: 9},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v", got)
	}
	if got := fromHistory(nil); got == nil {
		t.Fatal("jamais nil")
	}
}

// The image ID of a container is for the Images page only: the container
// list of the API does not change.
func TestContainerImageIDNotInJSON(t *testing.T) {
	c := fromSummary(container.Summary{ID: "x", ImageID: "sha256:abc"})
	if c.ImageID != "sha256:abc" {
		t.Fatalf("ImageID = %q", c.ImageID)
	}
	b, _ := json.Marshal(c)
	if strings.Contains(string(b), "sha256:abc") {
		t.Fatalf("ImageID dans le JSON : %s", b)
	}
}

func TestFakeImages(t *testing.T) {
	f := &Fake{
		ImageSummaries: []ImageSummary{{ID: "sha256:a", Tags: []string{"<none>:<none>"}}},
		Images:         map[string]ImageInfo{"sha256:a": {ID: "sha256:a"}, "sha256:b": {ID: "sha256:b"}},
		Histories:      map[string][]ImageLayer{"sha256:a": {{CreatedBy: "x"}}},
	}
	ctx := context.Background()
	if l, err := f.ImageList(ctx); err != nil || len(l) != 1 || l[0].Tags == nil || len(l[0].Tags) != 0 {
		t.Fatalf("ImageList : %+v %v", l, err)
	}
	if h, err := f.ImageHistory(ctx, "sha256:a"); err != nil || len(h) != 1 {
		t.Fatalf("ImageHistory : %+v %v", h, err)
	}
	if h, err := f.ImageHistory(ctx, "sha256:b"); err != nil || h == nil || len(h) != 0 {
		t.Fatalf("sans historique : %+v %v", h, err)
	}
	if _, err := f.ImageHistory(ctx, "sha256:z"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("inconnue : %v", err)
	}
	f.Err = ErrUnavailable
	if _, err := f.ImageList(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Err : %v", err)
	}
}
