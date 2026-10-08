package dockerx

import "context"

// Fake is an in-memory Client for tests of other packages.
type Fake struct {
	Containers []Container
	Err        error
}

func (f *Fake) ListContainers(context.Context) ([]Container, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Containers, nil
}

func (f *Fake) Ping(context.Context) error { return f.Err }
