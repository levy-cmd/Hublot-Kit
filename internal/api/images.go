package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/secrets"
)

// GET /api/images and GET /api/images/{id} (spec v0.2 §5.4): the images of
// the daemon, the containers that run each one, and the detail of one image.
// Read only: allowed in --read-only, nothing is written to Docker.
//
// What leaves the agent: the tags, sizes and dates; the configuration
// masked as a container's radiography (env by secrets.Env, entrypoint and
// command by secrets.Args, labels by secrets.Labels); the history by
// secrets.History (build arguments after "|N" masked whatever their name).
// No reveal: the reveal of a variable stays that of a container (Q28).
//
// {id} is the full ID only (imageID): never a name, which may change target
// between two requests, nor a short ID; anything else is 404 without asking
// Docker.
//
// Cost (the same questions as the Secrets page): the list is one
// GET /images/json and one GET /containers/json (172 ms measured for 74
// tags), whatever the number of images, never an Inspect per image;
// simultaneous lists share the walk in flight, detached from any request,
// each caller with its own deadline; the answer is smaller than Docker's own
// (no labels, digests, shared sizes). The detail is three calls for one
// image, under the guard of the file routes (spec v0.2 §6: 4 operations per
// image, 10 s). Both end at imagesTimeout (504); any other failure of Docker
// is 503 docker_unavailable with a fixed detail, the error being logged
// through secrets.Line (it may hold the address of Docker).

// imagesTimeout bounds a walk of the list and a detail.
var imagesTimeout = fsTimeout

// errImagesTimeout: Docker did not answer within imagesTimeout.
var errImagesTimeout = errors.New("Docker did not answer in time")

// fullImageID: 64 lowercase hex characters, "sha256:" before them or not.
var fullImageID = regexp.MustCompile(`^(?:sha256:)?([0-9a-f]{64})$`)

// imageID accepts the full ID of an image, 64 hex characters, "sha256:"
// accepted, and returns it as "sha256:<hex>". Anything else is refused.
func imageID(s string) (string, bool) {
	m := fullImageID.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	return "sha256:" + m[1], true
}

type imageContainerJSON struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

type imageJSON struct {
	ID         string               `json:"id"`
	Tags       []string             `json:"tags"`
	Size       int64                `json:"size"`
	Created    time.Time            `json:"created"`
	Containers []imageContainerJSON `json:"containers"`
}

type imageConfigJSON struct {
	Env           []secrets.Var   `json:"env"`
	Cmd           []string        `json:"cmd"`
	Entrypoint    []string        `json:"entrypoint"`
	CommandMasked bool            `json:"commandMasked"`
	WorkingDir    string          `json:"workingDir"`
	User          string          `json:"user"`
	ExposedPorts  []string        `json:"exposedPorts"`
	Labels        []secrets.Label `json:"labels"`
}

type layerJSON struct {
	CreatedBy string `json:"createdBy"`
	Created   string `json:"created"` // "" when Docker gives none
	Size      int64  `json:"size"`
	Empty     bool   `json:"empty"`
}

type imageDetailJSON struct {
	ID           string               `json:"id"`
	Tags         []string             `json:"tags"`
	Size         int64                `json:"size"`
	Created      string               `json:"created"`
	OS           string               `json:"os"`
	Architecture string               `json:"architecture"`
	Variant      string               `json:"variant"`
	Config       imageConfigJSON      `json:"config"`
	Layers       []layerJSON          `json:"layers"`
	Containers   []imageContainerJSON `json:"containers"`
}

// containersByImage: the containers of each image ID, sorted by name.
func containersByImage(list []dockerx.Container) map[string][]imageContainerJSON {
	out := map[string][]imageContainerJSON{}
	for _, c := range list {
		if c.ImageID != "" {
			out[c.ImageID] = append(out[c.ImageID], imageContainerJSON{ID: c.ID, Name: c.Name, State: c.State})
		}
	}
	for _, cs := range out {
		sort.Slice(cs, func(i, j int) bool { return cs[i].Name < cs[j].Name })
	}
	return out
}

// imagesFlight is the walk of the list in flight; its waiters read rows and
// err once done is closed.
type imagesFlight struct {
	done chan struct{}
	rows []imageJSON
	err  error
}

var (
	imagesMu      sync.Mutex
	imagesFlights = map[*server]*imagesFlight{}
)

// sharedImages joins the walk in flight, or starts one detached from any
// request: a caller that leaves does not end it for the others.
func (s *server) sharedImages() *imagesFlight {
	imagesMu.Lock()
	defer imagesMu.Unlock()
	if f := imagesFlights[s]; f != nil {
		return f
	}
	f := &imagesFlight{done: make(chan struct{})}
	imagesFlights[s] = f
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), imagesTimeout)
		defer cancel()
		f.rows, f.err = s.listImages(ctx)
		imagesMu.Lock()
		delete(imagesFlights, s)
		imagesMu.Unlock()
		close(f.done)
	}()
	return f
}

func (s *server) listImages(ctx context.Context) ([]imageJSON, error) {
	var (
		wg         sync.WaitGroup
		imgs       []dockerx.ImageSummary
		ctrs       []dockerx.Container
		ierr, cerr error
	)
	wg.Go(func() { imgs, ierr = s.d.Docker.ImageList(ctx) })
	wg.Go(func() { ctrs, cerr = s.d.Docker.ListContainers(ctx) })
	wg.Wait()
	if ctx.Err() != nil {
		return nil, errImagesTimeout
	}
	if err := errors.Join(ierr, cerr); err != nil {
		return nil, err
	}
	byImage := containersByImage(ctrs)
	rows := make([]imageJSON, 0, len(imgs))
	for _, im := range imgs {
		rows = append(rows, imageJSON{ID: im.ID, Tags: nonNil(im.Tags), Size: im.Size, Created: im.Created,
			Containers: nonNil(byImage[im.ID])})
	}
	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].Created.Equal(rows[j].Created) {
			return rows[i].Created.After(rows[j].Created)
		}
		return rows[i].ID < rows[j].ID
	})
	return rows, nil
}

func (s *server) images(w http.ResponseWriter, r *http.Request) {
	// The walk ends by imagesTimeout; the margin is for its last call to
	// give up.
	ctx, cancel := context.WithTimeout(r.Context(), imagesTimeout+5*time.Second)
	defer cancel()
	f := s.sharedImages()
	select {
	case <-f.done:
	case <-ctx.Done():
		writeImagesError(w, errImagesTimeout)
		return
	}
	if f.err != nil {
		writeImagesError(w, f.err)
		return
	}
	writeJSON(w, http.StatusOK, f.rows)
}

// errImageMismatch: Docker answered for another image than the one asked.
var errImageMismatch = errors.New("Docker answered another image")

func (s *server) image(w http.ResponseWriter, r *http.Request) {
	id, ok := imageID(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "no such image")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), imagesTimeout)
	defer cancel()
	d, release, err := guarded(s.guard, ctx, id, func() (imageDetailJSON, error) { return s.imageDetail(ctx, id) }, nil)
	release()
	if err != nil {
		writeImagesError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *server) imageDetail(ctx context.Context, id string) (imageDetailJSON, error) {
	var (
		wg               sync.WaitGroup
		info             dockerx.ImageInfo
		hist             []dockerx.ImageLayer
		ctrs             []dockerx.Container
		ierr, herr, cerr error
	)
	wg.Go(func() { info, ierr = s.d.Docker.ImageInspect(ctx, id) })
	wg.Go(func() { hist, herr = s.d.Docker.ImageHistory(ctx, id) })
	wg.Go(func() { ctrs, cerr = s.d.Docker.ListContainers(ctx) })
	wg.Wait()
	if ctx.Err() != nil {
		return imageDetailJSON{}, errImagesTimeout
	}
	if err := errors.Join(ierr, herr, cerr); err != nil {
		return imageDetailJSON{}, err
	}
	if info.ID != id {
		return imageDetailJSON{}, errImageMismatch
	}
	ep, cmd, cmdMasked := maskCommand(info.Entrypoint, info.Cmd)
	out := imageDetailJSON{
		ID: info.ID, Tags: nonNil(info.Tags), Size: info.Size, Created: info.Created,
		OS: info.OS, Architecture: info.Architecture, Variant: info.Variant,
		Config: imageConfigJSON{
			Env: secrets.Env(info.Env), Entrypoint: ep, Cmd: cmd, CommandMasked: cmdMasked,
			WorkingDir: info.WorkingDir, User: info.User, ExposedPorts: nonNil(info.ExposedPorts),
			Labels: secrets.Labels(info.Labels),
		},
		Layers:     make([]layerJSON, 0, len(hist)),
		Containers: nonNil(containersByImage(ctrs)[info.ID]),
	}
	for _, l := range hist {
		lj := layerJSON{CreatedBy: secrets.History(l.CreatedBy), Size: l.Size, Empty: l.Empty}
		if !l.Created.IsZero() {
			lj.Created = l.Created.UTC().Format(time.RFC3339)
		}
		out.Layers = append(out.Layers, lj)
	}
	return out, nil
}

// writeImagesError answers a failure of the image routes with a fixed
// detail: never the text of Docker's error.
func writeImagesError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errImagesTimeout), errors.Is(err, errTimeout):
		writeError(w, http.StatusGatewayTimeout, "internal", errImagesTimeout.Error())
	case errors.Is(err, errBusy):
		writeError(w, http.StatusServiceUnavailable, "internal", "too many requests in progress on this image")
	case errors.Is(err, dockerx.ErrNotFound), errors.Is(err, errImageMismatch):
		writeError(w, http.StatusNotFound, "not_found", "no such image")
	default:
		slog.Warn("images: Docker", "err", secrets.Line(err.Error()))
		writeError(w, http.StatusServiceUnavailable, "docker_unavailable", "Docker unavailable")
	}
}
