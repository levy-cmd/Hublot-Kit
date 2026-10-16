// Package dockerx is the agent's only gateway to the Docker Engine API.
package dockerx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

type Port struct {
	Private uint16 `json:"private"`
	Public  uint16 `json:"public,omitempty"`
	Type    string `json:"type"`
	IP      string `json:"ip,omitempty"`
}

type Container struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Image   string    `json:"image"`
	State   string    `json:"state"`
	Status  string    `json:"status"`
	Created time.Time `json:"created"`
	Ports   []Port    `json:"ports"`
}

// ContainerInfo is the part of `docker inspect` the agent uses.
type ContainerInfo struct {
	ID      string // full 64-character ID
	Name    string // without the leading "/"
	Running bool
	Pid     int // 0 when the container does not run
	// FinishedAt is State.FinishedAt as Docker gives it: part of the index
	// cache key (spec §5.2 step 1). "0001-01-01T00:00:00Z" if never started.
	FinishedAt string
	Mounts     []Mount
	// Tmpfs: destinations of --tmpfs (HostConfig.Tmpfs), sorted. Absent from
	// Mounts; Docker never writes into them (plan Explorateur 3, Q21).
	Tmpfs []string
	// Tty: the logs come as one raw stream, not multiplexed (F6).
	Tty bool
	// LogDriver: HostConfig.LogConfig.Type ("json-file", "local", "none"…).
	LogDriver string
	// Details: the rest of the radiography (F7).
	Details Details
}

// Details is what the radiography (F7) shows of `docker inspect`, beyond
// ContainerInfo. Env, Cmd and Entrypoint are raw: package secrets masks
// them before they leave the agent (spec §7).
type Details struct {
	Image        string // as asked (Config.Image: "nginx:1.27")
	ImageID      string // "sha256:…"
	Created      string
	Status       string // State.Status: created, running, paused, restarting, exited, dead
	ExitCode     int
	Error        string
	StartedAt    string
	OOMKilled    bool
	Health       string // healthy, unhealthy, starting; "" without a health check
	RestartCount int
	// RestartPolicy: no, always, on-failure, unless-stopped; MaxRetry for on-failure.
	RestartPolicy string
	MaxRetry      int
	Entrypoint    []string
	Cmd           []string
	WorkingDir    string
	User          string
	Env           []string // "NAME=value", raw
	// Ports: the real bindings of a running container, else the configured
	// ones; none on the host network or another container's (Docker ignores
	// -p there). An exposed port without binding has no IP and no Public; a
	// configured -p 90 (host port chosen at start) has an IP, no Public.
	// Sorted.
	Ports       []Port
	NetworkMode string
	Networks    []Network // sorted by name
}

// Network is one network the container is attached to.
type Network struct {
	Name    string   `json:"name"`
	IP      string   `json:"ip,omitempty"`
	Gateway string   `json:"gateway,omitempty"`
	MAC     string   `json:"mac,omitempty"`
	Aliases []string `json:"aliases,omitempty"`
}

// Change is one entry of GET /containers/{id}/changes (F5).
type Change struct {
	Path string `json:"path"`
	Kind string `json:"kind"` // "added" | "modified" | "deleted"
}

// Mount is one entry of Inspect.Mounts (binds and volumes).
type Mount struct {
	Type        string // "bind", "volume", "tmpfs"…
	Name        string // volume name
	Source      string // host path of a bind
	Destination string // path in the container
	Driver      string // volume driver ("local"…)
	Mode        string // options given by the user ("ro", "z"…)
	RW          bool
}

// ErrNotFound: no such container (or no such path, for an archive).
// ErrRefused: Docker refused a write because its target is read-only
// (measured on Docker 29.7: "container rootfs is marked read-only",
// "mounted volume is marked read-only").
// ErrPutInvalid: any other InvalidArgument of PutArchive, e.g. "extraction
// point is not a directory": the path changed after the agent's checks.
// ErrUnavailable: any other failure of the daemon or of the connection to it.
var (
	ErrNotFound    = errors.New("not found in Docker")
	ErrRefused     = errors.New("refused by Docker")
	ErrPutInvalid  = errors.New("extraction refused by Docker")
	ErrUnavailable = errors.New("Docker unavailable")
)

type Client interface {
	ListContainers(ctx context.Context) ([]Container, error)
	Ping(ctx context.Context) error
	// Inspect accepts a full ID, a short ID or a name.
	Inspect(ctx context.Context, id string) (ContainerInfo, error)
	// GetArchive streams the tar of path (GET /containers/{id}/archive).
	// The caller closes it.
	GetArchive(ctx context.Context, id, path string) (io.ReadCloser, error)
	// VolumeOptions returns the driver options of a named volume
	// (GET /volumes/{name}, field Options), e.g. o=bind, device=/proc.
	VolumeOptions(ctx context.Context, name string) (map[string]string, error)
	// SizeRootFs is Inspect with size=true: the size of the container's
	// root filesystem, for the progress of an index (spec §5.2 step 3). Costly
	// for Docker: called once per index build. 0 when Docker gives none.
	SizeRootFs(ctx context.Context, id string) (int64, error)
	// Changes is GET /containers/{id}/changes (F5): never nil.
	Changes(ctx context.Context, id string) ([]Change, error)
	// PutArchive extracts the tar read from content into directory dir of the
	// container (PUT /containers/{id}/archive). dir is sent as is: package
	// files gives the parent of the path it checked (spec §5.3 step 5).
	// Docker never replaces a directory by a file (noOverwriteDirNonDir) and
	// keeps the UID/GID of the tar headers (no copyUIDGID).
	PutArchive(ctx context.Context, id, dir string, content io.Reader) error
	// Logs is GET /containers/{id}/logs with stdout, stderr and timestamps
	// (F6): the raw stream, multiplexed unless the container has a TTY; read
	// it with ReadLogs. The caller closes it; it also ends with ctx.
	// ErrNoLogs when the logging driver cannot be read.
	Logs(ctx context.Context, id string, opt LogsOptions) (io.ReadCloser, error)
}

type engine struct{ c *client.Client }

// New connects lazily: no request is sent until the first call.
// The API version is negotiated on the first request.
func New() (Client, error) {
	c, err := client.New(client.FromEnv)
	if err != nil {
		return nil, err
	}
	return &engine{c: c}, nil
}

func (e *engine) Ping(ctx context.Context) error {
	_, err := e.c.Ping(ctx, client.PingOptions{})
	return err
}

func (e *engine) ListContainers(ctx context.Context) ([]Container, error) {
	res, err := e.c.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	out := make([]Container, 0, len(res.Items))
	for _, s := range res.Items {
		out = append(out, fromSummary(s))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func fromSummary(s container.Summary) Container {
	name := ""
	for _, n := range s.Names {
		if t := strings.TrimPrefix(n, "/"); t != "" && !strings.Contains(t, "/") {
			name = t // link aliases look like "/other/alias"
			break
		}
	}
	if name == "" && len(s.Names) > 0 {
		name = strings.TrimPrefix(s.Names[0], "/")
	}
	if name == "" {
		name = s.ID[:min(12, len(s.ID))]
	}
	type portKey struct {
		private, public uint16
		typ, ip         string
	}
	ports := make([]Port, 0, len(s.Ports))
	seen := map[portKey]bool{}
	for _, p := range s.Ports {
		ip := ""
		if p.IP.IsValid() {
			ip = p.IP.String()
			if p.IP.IsUnspecified() {
				ip = "0.0.0.0" // 0.0.0.0 and :: are the same mapping
			}
		}
		key := portKey{p.PrivatePort, p.PublicPort, p.Type, ip}
		if seen[key] {
			continue
		}
		seen[key] = true
		ports = append(ports, Port{Private: p.PrivatePort, Public: p.PublicPort, Type: p.Type, IP: ip})
	}
	return Container{
		ID:      s.ID,
		Name:    name,
		Image:   s.Image,
		State:   string(s.State),
		Status:  s.Status,
		Created: time.Unix(s.Created, 0).UTC(),
		Ports:   ports,
	}
}

func (e *engine) Inspect(ctx context.Context, id string) (ContainerInfo, error) {
	res, err := e.c.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return ContainerInfo{}, wrap(err)
	}
	return fromInspect(res.Container), nil
}

func fromInspect(c container.InspectResponse) ContainerInfo {
	info := ContainerInfo{ID: c.ID, Name: strings.TrimPrefix(c.Name, "/")}
	if c.State != nil {
		info.Running = c.State.Running
		info.Pid = c.State.Pid
		info.FinishedAt = c.State.FinishedAt
	}
	for _, m := range c.Mounts {
		info.Mounts = append(info.Mounts, Mount{
			Type: string(m.Type), Name: m.Name, Source: m.Source, Destination: m.Destination, Driver: m.Driver,
			Mode: m.Mode, RW: m.RW,
		})
	}
	if c.Config != nil {
		info.Tty = c.Config.Tty
	}
	info.Details = fromInspectDetails(c)
	if c.HostConfig != nil {
		info.LogDriver = c.HostConfig.LogConfig.Type
		for d := range c.HostConfig.Tmpfs {
			info.Tmpfs = append(info.Tmpfs, d)
		}
		sort.Strings(info.Tmpfs)
	}
	return info
}

func fromInspectDetails(c container.InspectResponse) Details {
	d := Details{ImageID: c.Image, Created: c.Created, RestartCount: c.RestartCount}
	if s := c.State; s != nil {
		d.Status, d.ExitCode, d.Error, d.StartedAt, d.OOMKilled = string(s.Status), s.ExitCode, s.Error, Date(s.StartedAt), s.OOMKilled
		if s.Health != nil {
			d.Health = string(s.Health.Status)
		}
	}
	var exposed network.PortSet
	if cf := c.Config; cf != nil {
		d.Image, d.Entrypoint, d.Cmd, d.WorkingDir, d.User, d.Env = cf.Image, cf.Entrypoint, cf.Cmd, cf.WorkingDir, cf.User, cf.Env
		exposed = cf.ExposedPorts
	}
	var bindings network.PortMap
	if h := c.HostConfig; h != nil {
		d.RestartPolicy, d.MaxRetry = string(h.RestartPolicy.Name), h.RestartPolicy.MaximumRetryCount
		d.NetworkMode = string(h.NetworkMode)
		bindings = h.PortBindings
	}
	// Host network or another container's: Docker ignores -p.
	shared := d.NetworkMode == "host" || strings.HasPrefix(d.NetworkMode, "container:")
	if shared {
		bindings = nil
	}
	if n := c.NetworkSettings; n != nil {
		if !shared && c.State != nil && c.State.Running && len(n.Ports) > 0 {
			bindings = n.Ports
		}
		for name, ep := range n.Networks {
			nw := Network{Name: name}
			if ep != nil {
				nw.Aliases = ep.Aliases
				if ep.IPAddress.IsValid() {
					nw.IP = ep.IPAddress.String()
				}
				if ep.Gateway.IsValid() {
					nw.Gateway = ep.Gateway.String()
				}
				if len(ep.MacAddress) > 0 {
					nw.MAC = ep.MacAddress.String()
				}
			}
			d.Networks = append(d.Networks, nw)
		}
		sort.Slice(d.Networks, func(i, j int) bool { return d.Networks[i].Name < d.Networks[j].Name })
	}
	d.Ports = portsOf(bindings, exposed)
	return d
}

// Date is a date of Docker's State, "" for its zero date
// ("0001-01-01T00:00:00Z": never started, or not finished yet).
func Date(s string) string {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil && t.IsZero() {
		return ""
	}
	return s
}

// portsOf lists each binding once (0.0.0.0 and :: are one mapping, as in
// fromSummary; HostIp "" of a configured binding is every interface too),
// then the exposed ports without binding (no IP).
func portsOf(bindings network.PortMap, exposed network.PortSet) []Port {
	var out []Port
	seen := map[Port]bool{}
	for p, bs := range bindings {
		for _, b := range bs {
			ip := "0.0.0.0"
			if b.HostIP.IsValid() && !b.HostIP.IsUnspecified() {
				ip = b.HostIP.String()
			}
			public, _ := strconv.ParseUint(b.HostPort, 10, 16)
			port := Port{Private: p.Num(), Public: uint16(public), Type: string(p.Proto()), IP: ip}
			if !seen[port] {
				seen[port] = true
				out = append(out, port)
			}
		}
	}
	for p := range exposed {
		if len(bindings[p]) == 0 {
			out = append(out, Port{Private: p.Num(), Type: string(p.Proto())})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Private != b.Private {
			return a.Private < b.Private
		}
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		return a.IP < b.IP
	})
	return out
}

func (e *engine) GetArchive(ctx context.Context, id, path string) (io.ReadCloser, error) {
	res, err := e.c.CopyFromContainer(ctx, id, client.CopyFromContainerOptions{SourcePath: path})
	if err != nil {
		return nil, wrap(err)
	}
	return res.Content, nil
}

func (e *engine) VolumeOptions(ctx context.Context, name string) (map[string]string, error) {
	res, err := e.c.VolumeInspect(ctx, name, client.VolumeInspectOptions{})
	if err != nil {
		return nil, wrap(err)
	}
	return res.Volume.Options, nil
}

func (e *engine) SizeRootFs(ctx context.Context, id string) (int64, error) {
	res, err := e.c.ContainerInspect(ctx, id, client.ContainerInspectOptions{Size: true})
	if err != nil {
		return 0, wrap(err)
	}
	if res.Container.SizeRootFs == nil {
		return 0, nil
	}
	return *res.Container.SizeRootFs, nil
}

func (e *engine) Changes(ctx context.Context, id string) ([]Change, error) {
	res, err := e.c.ContainerDiff(ctx, id, client.ContainerDiffOptions{})
	if err != nil {
		return nil, wrap(err)
	}
	return fromChanges(res.Changes)
}

func fromChanges(cs []container.FilesystemChange) ([]Change, error) {
	out := make([]Change, 0, len(cs))
	for _, c := range cs {
		var kind string
		switch c.Kind {
		case container.ChangeAdd:
			kind = "added"
		case container.ChangeModify:
			kind = "modified"
		case container.ChangeDelete:
			kind = "deleted"
		default:
			return nil, fmt.Errorf("%w: unknown change kind %d for %s", ErrUnavailable, c.Kind, c.Path)
		}
		out = append(out, Change{Path: c.Path, Kind: kind})
	}
	return out, nil
}

func (e *engine) PutArchive(ctx context.Context, id, dir string, content io.Reader) error {
	_, err := e.c.CopyToContainer(ctx, id, client.CopyToContainerOptions{DestinationPath: dir, Content: content})
	if err != nil {
		return wrapPut(err)
	}
	return nil
}

// wrapPut: wrap, plus Docker's own refusals of a write, InvalidArgument:
// the target is read-only (both measured messages say "marked read-only") →
// ErrRefused; anything else → ErrPutInvalid. Only for PutArchive: on a read
// route an InvalidArgument stays docker_unavailable (plan Explorateur 3, Q15).
func wrapPut(err error) error {
	if cerrdefs.IsInvalidArgument(err) {
		if strings.Contains(err.Error(), "marked read-only") {
			return fmt.Errorf("%w: %v", ErrRefused, err)
		}
		return fmt.Errorf("%w: %v", ErrPutInvalid, err)
	}
	return wrap(err)
}

// wrap classifies a daemon error for the API (not_found / docker_unavailable).
func wrap(err error) error {
	if cerrdefs.IsNotFound(err) {
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	return fmt.Errorf("%w: %v", ErrUnavailable, err)
}
