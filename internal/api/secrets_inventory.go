package api

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/secrets"
)

// GET /api/secrets (spec v0.2 §5.9): the inventory of what the radiography
// masks, for every container, by NAME only:
//   - env: the names of the variables secrets.Masked masks;
//   - args: the arguments of the entrypoint and the command that
//     secrets.Args changed, in their masked form, as the radiography shows
//     them;
//   - labels: the keys of the labels secrets.Labels masks.
//
// No value ever leaves here, masked or not: the reveal stays POST
// /api/containers/{id}/env/reveal, journaled (env.reveal), one variable at
// a time (Q28). Nothing is cached: the page reloads on demand.
//
// Cost: one Inspect per container (2 to 9 ms measured), inventoryParallel
// in flight for the whole agent (two pages open share them), each bounded
// by inventoryInspectTimeout (dockerd holds the lock of a container during
// its export, v0.1.1: that container is unavailable, the others are not
// held). The whole answer is bounded by inventoryTimeout, and at most
// inventoryMax containers are inspected: past either, the rest is
// unavailable without an Inspect.

const inventoryParallel = 4

var (
	inventoryInspectTimeout = 3 * time.Second
	inventoryTimeout        = 15 * time.Second
	inventoryMax            = 1000
	// inventorySlots: the Inspect calls of every inventory in flight.
	inventorySlots = make(chan struct{}, inventoryParallel)
)

type inventoryJSON struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Project     string   `json:"project"`
	Service     string   `json:"service"`
	State       string   `json:"state"`
	Env         []string `json:"env"`
	Args        []string `json:"args"`
	Labels      []string `json:"labels"`
	Unavailable bool     `json:"unavailable"`
}

// secretNames fills the names of row from a fresh Inspect.
func secretNames(row *inventoryJSON, d dockerx.Details) {
	for _, v := range secrets.Env(d.Env) {
		if v.Masked {
			row.Env = append(row.Env, v.Name)
		}
	}
	for _, raw := range [][]string{d.Entrypoint, d.Cmd} {
		masked, _ := secrets.Args(raw)
		for i, a := range masked {
			if a != raw[i] {
				row.Args = append(row.Args, a)
			}
		}
	}
	for _, l := range secrets.Labels(d.Labels) {
		if l.Masked {
			row.Labels = append(row.Labels, l.Key)
		}
	}
}

func (s *server) secretsInventory(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), inventoryTimeout)
	defer cancel()
	list, err := s.d.Docker.ListContainers(ctx)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "docker_unavailable", err.Error())
		return
	}
	rows := make([]inventoryJSON, len(list))
	gone := make([]bool, len(list)) // removed between the list and its Inspect
	var wg sync.WaitGroup
	for i, c := range list {
		rows[i] = inventoryJSON{ID: c.ID, Name: c.Name, Project: c.Project, Service: c.Service, State: c.State,
			Env: []string{}, Args: []string{}, Labels: []string{}, Unavailable: true}
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
	writeJSON(w, http.StatusOK, out)
}
