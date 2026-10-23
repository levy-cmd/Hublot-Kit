package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/secrets"
)

// GET /api/secrets (spec v0.2 §5.9): the inventory of what the radiography
// masks, for every container, by NAME only:
//   - env: the names of the variables secrets.Masked masks, each once;
//   - args: the arguments of the entrypoint and the command that
//     secrets.Args changed, by their position and the name detected
//     (argJSON), never their text: a partial mask of spec §13 leaves a rest
//     of the value in it (review of tranche 4, Codex);
//   - labels: the keys of the labels secrets.Labels masks.
//
// No value ever leaves here, masked or not: the reveal stays POST
// /api/containers/{id}/env/reveal, journaled (env.reveal), one variable at
// a time (Q28). Nothing is cached: the page reloads on demand.
//
// Cost: one walk at a time per agent (simultaneous inventories wait for the
// one in flight, each with its own deadline), one Inspect per container (2
// to 9 ms measured), inventoryParallel in flight, each bounded by
// inventoryInspectTimeout (dockerd holds the lock of a container during its
// export, v0.1.1: that container is unavailable, the others are not held).
// The walk is bounded by inventoryTimeout, and at most inventoryMax
// containers are inspected: past either, the rest is unavailable without an
// Inspect.

const inventoryParallel = 4

var (
	inventoryInspectTimeout = 3 * time.Second
	inventoryTimeout        = 15 * time.Second
	inventoryMax            = 1000
	// inventorySlots: the Inspect calls of every inventory in flight.
	inventorySlots = make(chan struct{}, inventoryParallel)
)

type inventoryJSON struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Project     string    `json:"project"`
	Service     string    `json:"service"`
	State       string    `json:"state"`
	Env         []string  `json:"env"`
	Args        []argJSON `json:"args"`
	Labels      []string  `json:"labels"`
	Unavailable bool      `json:"unavailable"`
}

// argJSON is a masked argument: where it is, and what was detected. Name is
// drawn from a flag or a key made of [A-Za-z0-9_.-] only, or from the shell
// and its flag: never from the value.
//   - flag: the argument after a secret flag (Name: "--requirepass");
//   - assignment: NAME=value with a secret name (Name: "PGPASSWORD");
//   - shell: the line of "sh -c" (Name: "sh -c", "bash -lc");
//   - url: a URL with credentials;
//   - authorization: "Authorization: Bearer …", "Basic …";
//   - other: anything else secrets.Args masked (a key=value inside, a text
//     too long).
type argJSON struct {
	Part  string `json:"part"` // "entrypoint" or "cmd"
	Index int    `json:"index"`
	Kind  string `json:"kind"`
	Name  string `json:"name,omitempty"`
}

// The forms of secrets.Args, to name what it masked (internal/secrets keeps
// its own; these only classify an argument it already changed).
var (
	invFlag      = regexp.MustCompile(`^-{1,2}[A-Za-z0-9_.-]+=?$`)
	invShells    = []string{"sh", "bash", "ash", "dash", "zsh", "ksh"}
	invShellFlag = regexp.MustCompile(`^-[A-Za-z]*c$`)
	invWhole     = regexp.MustCompile(`(?s)^(-{0,2}[A-Za-z0-9_.-]+)=.`)
	invAuth      = regexp.MustCompile(`(?i)\b(bearer|basic|authorization)\b`)
)

// invMaxArg: past this, secrets.Args masks an argument whole (its maxLen).
const invMaxArg = 8 << 10

// maskedArgs names the arguments of raw that secrets.Args changes.
func maskedArgs(part string, raw []string) []argJSON {
	masked, _ := secrets.Args(raw)
	var out []argJSON
	for i, m := range masked {
		if m == raw[i] {
			continue
		}
		a, prev := argJSON{Part: part, Index: i, Kind: "other"}, ""
		if i > 0 {
			prev = raw[i-1]
		}
		// A name comes only from text the masking left in clear (second
		// review of tranche 4): after "--password=", "--Tok3n_SECRET" is the
		// masked value, not a flag that names the next argument.
		clear := func(j int) bool { return j >= 0 && masked[j] == raw[j] }
		w := invWhole.FindStringSubmatch(raw[i])
		switch {
		case len(raw[i]) > invMaxArg:
		case clear(i-1) && invFlag.MatchString(prev) && secrets.SecretName(prev):
			a.Kind, a.Name = "flag", strings.TrimSuffix(prev, "=")
		case i > 0 && clear(0) && clear(i-1) && invShellFlag.MatchString(prev) && slices.Contains(invShells, path.Base(raw[0])):
			a.Kind, a.Name = "shell", path.Base(raw[0])+" "+prev
		case w != nil && strings.HasPrefix(m, w[1]+"=") && (secrets.SecretName(w[1]) || strings.Contains(strings.ToLower(w[1]), "pwd")):
			a.Kind, a.Name = "assignment", w[1]
		case secrets.URLCredentials(raw[i]):
			a.Kind = "url"
		case invAuth.MatchString(raw[i]):
			a.Kind = "authorization"
		}
		out = append(out, a)
	}
	return out
}

// secretNames fills the names of row from a fresh Inspect.
func secretNames(row *inventoryJSON, d dockerx.Details) {
	for _, v := range secrets.Env(d.Env) {
		// Docker may keep two variables of the same name (review of tranche 4).
		if v.Masked && !slices.Contains(row.Env, v.Name) {
			row.Env = append(row.Env, v.Name)
		}
	}
	row.Args = append(row.Args, maskedArgs("entrypoint", d.Entrypoint)...)
	row.Args = append(row.Args, maskedArgs("cmd", d.Cmd)...)
	for _, l := range secrets.Labels(d.Labels) {
		if l.Masked {
			row.Labels = append(row.Labels, l.Key)
		}
	}
}

// inventoryFlight is the walk in flight; its waiters read rows and err once
// done is closed.
type inventoryFlight struct {
	done chan struct{}
	rows []inventoryJSON
	err  error
}

var (
	inventoryMu      sync.Mutex
	inventoryFlights = map[*server]*inventoryFlight{}
)

// sharedInventory joins the walk in flight, or starts one detached from any
// request: a caller that leaves does not end it for the others.
func (s *server) sharedInventory() *inventoryFlight {
	inventoryMu.Lock()
	defer inventoryMu.Unlock()
	if f := inventoryFlights[s]; f != nil {
		return f
	}
	f := &inventoryFlight{done: make(chan struct{})}
	inventoryFlights[s] = f
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), inventoryTimeout)
		defer cancel()
		f.rows, f.err = s.takeInventory(ctx)
		inventoryMu.Lock()
		delete(inventoryFlights, s)
		inventoryMu.Unlock()
		close(f.done)
	}()
	return f
}

func (s *server) secretsInventory(w http.ResponseWriter, r *http.Request) {
	// The walk ends by inventoryTimeout; the margin is for its last Inspect
	// to give up.
	ctx, cancel := context.WithTimeout(r.Context(), inventoryTimeout+5*time.Second)
	defer cancel()
	f := s.sharedInventory()
	select {
	case <-f.done:
	case <-ctx.Done():
		writeError(w, http.StatusGatewayTimeout, "internal", "inventory timed out")
		return
	}
	if f.err != nil {
		// The error may hold the address of Docker (tcp://user:pass@host).
		slog.Warn("secrets inventory: list", "err", secrets.Line(f.err.Error()))
		writeError(w, http.StatusServiceUnavailable, "docker_unavailable", "Docker unavailable")
		return
	}
	writeJSON(w, http.StatusOK, f.rows)
}

func (s *server) takeInventory(ctx context.Context) ([]inventoryJSON, error) {
	list, err := s.d.Docker.ListContainers(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]inventoryJSON, len(list))
	gone := make([]bool, len(list)) // removed between the list and its Inspect
	var wg sync.WaitGroup
	for i, c := range list {
		rows[i] = inventoryJSON{ID: c.ID, Name: c.Name, Project: c.Project, Service: c.Service, State: c.State,
			Env: []string{}, Args: []argJSON{}, Labels: []string{}, Unavailable: true}
		if i >= inventoryMax || ctx.Err() != nil {
			continue
		}
		select {
		case inventorySlots <- struct{}{}:
		case <-ctx.Done():
			continue
		}
		// select picks at random when a place frees up as the deadline
		// passes: no Inspect past it.
		if ctx.Err() != nil {
			<-inventorySlots
			continue
		}
		wg.Go(func() {
			defer func() { <-inventorySlots }()
			ictx, icancel := context.WithTimeout(ctx, inventoryInspectTimeout)
			defer icancel()
			info, err := s.d.Docker.Inspect(ictx, c.ID)
			switch {
			case errors.Is(err, dockerx.ErrNotFound):
				gone[i] = true
			case err == nil:
				rows[i].Unavailable = false
				secretNames(&rows[i], info.Details)
			}
		})
	}
	wg.Wait()
	out := make([]inventoryJSON, 0, len(rows))
	for i, row := range rows {
		if !gone[i] {
			out = append(out, row)
		}
	}
	return out, nil
}
