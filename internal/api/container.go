package api

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/levy-cmd/hublot-kit/internal/audit"
	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/secrets"
)

type stateJSON struct {
	Status     string `json:"status"`
	Running    bool   `json:"running"`
	ExitCode   int    `json:"exitCode"`
	Error      string `json:"error,omitempty"`
	StartedAt  string `json:"startedAt,omitempty"`
	FinishedAt string `json:"finishedAt,omitempty"`
	OOMKilled  bool   `json:"oomKilled"`
	Health     string `json:"health,omitempty"`
}

type restartJSON struct {
	Name     string `json:"name"`
	MaxRetry int    `json:"maxRetry,omitempty"`
}

type mountJSON struct {
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	Source      string `json:"source,omitempty"`
	Destination string `json:"destination"`
	Driver      string `json:"driver,omitempty"`
	Mode        string `json:"mode,omitempty"`
	RW          bool   `json:"rw"`
}

// radioJSON is GET /api/containers/{id} (F7). Env and the command are
// masked by package secrets (spec §7): nothing secret leaves the agent
// here; one variable at a time goes through POST /env/reveal.
type radioJSON struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Image         string            `json:"image"`
	ImageID       string            `json:"imageId"`
	Created       string            `json:"created"`
	State         stateJSON         `json:"state"`
	RestartCount  int               `json:"restartCount"`
	RestartPolicy restartJSON       `json:"restartPolicy"`
	Entrypoint    []string          `json:"entrypoint"`
	Cmd           []string          `json:"cmd"`
	CommandMasked bool              `json:"commandMasked"`
	WorkingDir    string            `json:"workingDir,omitempty"`
	User          string            `json:"user,omitempty"`
	Tty           bool              `json:"tty"`
	LogDriver     string            `json:"logDriver"`
	Env           []secrets.Var     `json:"env"`
	Ports         []dockerx.Port    `json:"ports"`
	NetworkMode   string            `json:"networkMode,omitempty"`
	Networks      []dockerx.Network `json:"networks"`
	Mounts        []mountJSON       `json:"mounts"`
	Tmpfs         []string          `json:"tmpfs"`
	// Labels: by secrets.Labels only (spec v0.2 §5.2), never revealed.
	Labels []secrets.Label `json:"labels"`
}

func toRadioJSON(c dockerx.ContainerInfo) radioJSON {
	d := c.Details
	ep, epMasked := secrets.Args(d.Entrypoint)
	cmd, cmdMasked := secrets.Args(d.Cmd)
	out := radioJSON{
		ID: c.ID, Name: c.Name, Image: d.Image, ImageID: d.ImageID, Created: d.Created,
		State: stateJSON{Status: d.Status, Running: c.Running, ExitCode: d.ExitCode, Error: secrets.Line(d.Error),
			StartedAt: d.StartedAt, FinishedAt: dockerx.Date(c.FinishedAt), OOMKilled: d.OOMKilled, Health: d.Health},
		RestartCount: d.RestartCount, RestartPolicy: restartJSON{Name: d.RestartPolicy, MaxRetry: d.MaxRetry},
		Entrypoint: nonNil(ep), Cmd: nonNil(cmd), CommandMasked: epMasked || cmdMasked,
		WorkingDir: d.WorkingDir, User: d.User, Tty: c.Tty, LogDriver: c.LogDriver,
		Env: secrets.Env(d.Env), Ports: nonNil(d.Ports), NetworkMode: d.NetworkMode,
		Networks: nonNil(d.Networks), Mounts: []mountJSON{}, Tmpfs: nonNil(c.Tmpfs), Labels: secrets.Labels(d.Labels),
	}
	for _, m := range c.Mounts {
		out.Mounts = append(out.Mounts, mountJSON{Type: m.Type, Name: m.Name, Source: m.Source,
			Destination: m.Destination, Driver: m.Driver, Mode: m.Mode, RW: m.RW})
	}
	return out
}

// nonNil: an empty JSON array rather than null.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// container is GET /api/containers/{id} (F7).
func (s *server) container(w http.ResponseWriter, r *http.Request) {
	info, err := s.inspectByRef(r)
	if err != nil {
		writeFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toRadioJSON(info))
}

// errNoVariable: the body names no variable of the container.
var errNoVariable = fmt.Errorf("%w: unknown variable", dockerx.ErrNotFound)

// revealEnv is POST /api/containers/{id}/env/reveal (spec §6, §7): {name} →
// {name, value}. The reveal is journaled (env.reveal, the name in path,
// never the value) before the value is sent; if the journal cannot be
// written, nothing is revealed. A read: the read-only mode allows it.
func (s *server) revealEnv(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	// One JSON object of 4 KiB at most, nothing after it but blanks, even
	// past the 4 KiB (review of tranche 4, decodeOne).
	if err := decodeOne(r.Body, 4096, &body); err != nil {
		writeFSError(w, fmt.Errorf("%w (%v)", errNoVariable, err))
		return
	}
	info, err := s.inspectByRef(r)
	if err != nil {
		writeFSError(w, err)
		return
	}
	value, ok := secrets.Lookup(info.Details.Env, body.Name)
	if !ok {
		writeFSError(w, errNoVariable)
		return
	}
	if err := s.d.Audit.Write(audit.Event{Action: "env.reveal", Container: info.ID, Path: body.Name, Result: "ok"}); err != nil {
		slog.Error("audit write", "err", err, "action", "env.reveal")
		writeError(w, http.StatusInternalServerError, "internal", "audit log: nothing revealed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": body.Name, "value": value})
}
