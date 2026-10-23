package dockerx

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"slices"
	"sync"
	"sync/atomic"
)

// Fake is an in-memory Client for tests of other packages. Concurrent calls
// are safe: an index is built in a goroutine, so GetArchive may run while the
// test reads Calls().
type Fake struct {
	mu         sync.Mutex // guards ArchiveCalls and the PutArchive calls
	Containers []Container
	// Infos answers Inspect, by full ID or by name (tests add both keys).
	Infos map[string]ContainerInfo
	// Archives answers GetArchive, keyed by id + ":" + path.
	Archives map[string][]byte
	// ArchiveGate, when not nil, holds every GetArchive call (after it is
	// recorded) until the channel is closed or the context ends: a test
	// keeps an index build in flight as long as it needs.
	ArchiveGate chan struct{}
	// ArchiveCalls records every GetArchive call as id + ":" + path.
	ArchiveCalls []string
	// Sizes answers SizeRootFs, by full ID (absent: 0).
	Sizes map[string]int64
	// Diffs answers Changes, by id as given (absent: ErrNotFound).
	Diffs map[string][]Change
	// ArchiveCloses counts the Close calls of the readers GetArchive returned.
	ArchiveCloses atomic.Int32
	// Volumes answers VolumeOptions, by volume name.
	Volumes map[string]map[string]string
	// PutErr answers PutArchive (nil: success); every call is recorded,
	// failed ones included (read them with PutCalls).
	PutErr error
	puts   []Put
	// LogStreams answers Logs with these raw bytes, by id as given (absent:
	// ErrNotFound). LogsErr, when set, is returned instead.
	LogStreams map[string][]byte
	LogsErr    error
	// LogsGate, when not nil, holds every logs stream after its bytes (a
	// container that keeps running) until it is closed (end of stream) or
	// the context ends (ctx.Err()).
	LogsGate   chan struct{}
	LogsCloses atomic.Int32
	logsCalls  []LogsOptions
	// Daemon answers Info, with DaemonErr (a Ping that works and a /version
	// that fails, for instance); with Err set, Connected is false.
	Daemon    DaemonInfo
	DaemonErr error
	// ContainerSizes answers ContainerSize, by id as given (absent: both
	// nil, as Docker without a size); SizeGate, when not nil, holds every
	// call until it is closed or the context ends (dockerd's container lock).
	ContainerSizes map[string]SizePair
	SizeGate       chan struct{}
	SizeCalls      atomic.Int32
	// Images answers ImageInspect, by id as given (absent: ErrNotFound);
	// ImageCalls counts the calls of ImageInspect and ImageHistory.
	Images     map[string]ImageInfo
	ImageCalls atomic.Int32
	// ImageSummaries answers ImageList; Histories answers ImageHistory, by
	// id as given (absent: ErrNotFound).
	ImageSummaries []ImageSummary
	Histories      map[string][]ImageLayer
	Err            error
}

// SizePair is what ContainerSize answers.
type SizePair struct{ RW, RootFs *int64 }

func (f *Fake) ContainerSize(ctx context.Context, id string) (rw, rootfs *int64, err error) {
	f.SizeCalls.Add(1)
	if f.SizeGate != nil {
		select {
		case <-f.SizeGate:
		case <-ctx.Done():
			return nil, nil, fmt.Errorf("%w: %v", ErrUnavailable, ctx.Err())
		}
	}
	if f.Err != nil {
		return nil, nil, f.Err
	}
	if _, ok := f.Infos[id]; !ok {
		return nil, nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	s := f.ContainerSizes[id]
	return s.RW, s.RootFs, nil
}

func (f *Fake) ImageInspect(_ context.Context, id string) (ImageInfo, error) {
	f.ImageCalls.Add(1)
	if f.Err != nil {
		return ImageInfo{}, f.Err
	}
	if img, ok := f.Images[id]; ok {
		return img, nil
	}
	return ImageInfo{}, fmt.Errorf("%w: image %s", ErrNotFound, id)
}

func (f *Fake) ImageList(context.Context) ([]ImageSummary, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	out := make([]ImageSummary, len(f.ImageSummaries))
	for i, s := range f.ImageSummaries {
		s.Tags = realTags(s.Tags) // as the engine
		out[i] = s
	}
	return out, nil
}

func (f *Fake) ImageHistory(_ context.Context, id string) ([]ImageLayer, error) {
	f.ImageCalls.Add(1)
	if f.Err != nil {
		return nil, f.Err
	}
	if h, ok := f.Histories[id]; ok {
		return append([]ImageLayer{}, h...), nil
	}
	if _, ok := f.Images[id]; ok {
		return []ImageLayer{}, nil
	}
	return nil, fmt.Errorf("%w: image %s", ErrNotFound, id)
}

func (f *Fake) Info(context.Context) (DaemonInfo, error) {
	if f.Err != nil {
		info := f.Daemon
		info.Connected = false
		return info, f.Err
	}
	return f.Daemon, f.DaemonErr
}

// Put is one PutArchive call: the tar is read whole.
type Put struct {
	ID, Dir string
	Tar     []byte
}

func (f *Fake) ListContainers(context.Context) ([]Container, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Containers, nil
}

func (f *Fake) Ping(context.Context) error { return f.Err }

func (f *Fake) Inspect(_ context.Context, id string) (ContainerInfo, error) {
	if f.Err != nil {
		return ContainerInfo{}, f.Err
	}
	if info, ok := f.Infos[id]; ok {
		return info, nil
	}
	return ContainerInfo{}, fmt.Errorf("%w: %s", ErrNotFound, id)
}

func (f *Fake) GetArchive(ctx context.Context, id, path string) (io.ReadCloser, error) {
	f.mu.Lock()
	f.ArchiveCalls = append(f.ArchiveCalls, id+":"+path)
	f.mu.Unlock()
	if f.ArchiveGate != nil {
		select {
		case <-f.ArchiveGate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.Err != nil {
		return nil, f.Err
	}
	b, ok := f.Archives[id+":"+path]
	if !ok {
		return nil, fmt.Errorf("%w: %s:%s", ErrNotFound, id, path)
	}
	return &countingCloser{Reader: bytes.NewReader(b), n: &f.ArchiveCloses}, nil
}

type countingCloser struct {
	io.Reader
	n *atomic.Int32
}

func (c *countingCloser) Close() error { c.n.Add(1); return nil }

func (f *Fake) VolumeOptions(_ context.Context, name string) (map[string]string, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	if opts, ok := f.Volumes[name]; ok {
		return opts, nil
	}
	return nil, fmt.Errorf("%w: volume %s", ErrNotFound, name)
}

func (f *Fake) PutArchive(_ context.Context, id, dir string, content io.Reader) error {
	b, err := io.ReadAll(content)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.puts = append(f.puts, Put{ID: id, Dir: dir, Tar: b})
	f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	return f.PutErr
}

func (f *Fake) Logs(ctx context.Context, id string, opt LogsOptions) (io.ReadCloser, error) {
	f.mu.Lock()
	f.logsCalls = append(f.logsCalls, opt)
	f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	if f.LogsErr != nil {
		return nil, f.LogsErr
	}
	b, ok := f.LogStreams[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return &logsStream{Reader: bytes.NewReader(b), ctx: ctx, gate: f.LogsGate, closes: &f.LogsCloses}, nil
}

// LogsCalls returns a copy of the options of every Logs call.
func (f *Fake) LogsCalls() []LogsOptions {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.logsCalls)
}

// logsStream serves its bytes, then waits at the gate, if any.
type logsStream struct {
	*bytes.Reader
	ctx    context.Context
	gate   chan struct{}
	closes *atomic.Int32
}

func (s *logsStream) Read(p []byte) (int, error) {
	if s.Reader.Len() > 0 {
		return s.Reader.Read(p)
	}
	if s.gate != nil {
		select {
		case <-s.gate:
		case <-s.ctx.Done():
			return 0, s.ctx.Err()
		}
	}
	return 0, io.EOF
}

func (s *logsStream) Close() error { s.closes.Add(1); return nil }

// PutCalls returns a copy of the PutArchive calls, read under the lock.
func (f *Fake) PutCalls() []Put {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.puts)
}

// Calls returns a copy of ArchiveCalls, read under the lock.
func (f *Fake) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.ArchiveCalls)
}

func (f *Fake) SizeRootFs(_ context.Context, id string) (int64, error) {
	if f.Err != nil {
		return 0, f.Err
	}
	return f.Sizes[id], nil
}

func (f *Fake) Changes(_ context.Context, id string) ([]Change, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	cs, ok := f.Diffs[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if cs == nil {
		return []Change{}, nil
	}
	return slices.Clone(cs), nil
}
