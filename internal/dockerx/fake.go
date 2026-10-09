package dockerx

import (
	"bytes"
	"context"
	"fmt"
	"io"
)

// Fake is an in-memory Client for tests of other packages. It is not safe for
// concurrent use: tests send one request at a time.
type Fake struct {
	Containers []Container
	// Infos answers Inspect, by full ID or by name (tests add both keys).
	Infos map[string]ContainerInfo
	// Archives answers GetArchive, keyed by id + ":" + path.
	Archives map[string][]byte
	// ArchiveCalls records every GetArchive call as id + ":" + path.
	ArchiveCalls []string
	// Volumes answers VolumeOptions, by volume name.
	Volumes map[string]map[string]string
	Err     error
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
	return ContainerInfo{}, fmt.Errorf("%w : %s", ErrNotFound, id)
}

func (f *Fake) GetArchive(_ context.Context, id, path string) (io.ReadCloser, error) {
	f.ArchiveCalls = append(f.ArchiveCalls, id+":"+path)
	if f.Err != nil {
		return nil, f.Err
	}
	b, ok := f.Archives[id+":"+path]
	if !ok {
		return nil, fmt.Errorf("%w : %s:%s", ErrNotFound, id, path)
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (f *Fake) VolumeOptions(_ context.Context, name string) (map[string]string, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	if opts, ok := f.Volumes[name]; ok {
		return opts, nil
	}
	return nil, fmt.Errorf("%w : volume %s", ErrNotFound, name)
}
