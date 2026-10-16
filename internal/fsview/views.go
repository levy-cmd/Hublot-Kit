package fsview

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// Views opens the file view of a container (spec §4): procfs while it runs,
// tarindex otherwise. One Views per agent: it keeps the indexes of stopped
// containers (spec §5.2 steps 1, 2, 4).
type Views struct {
	docker dockerx.Client
	opt    Options
	mu     sync.Mutex
	builds map[string]*build // key: full ID + "\x00" + State.FinishedAt
	order  []string          // keys, least recently used first
}

// Options of NewViews; DefaultOptions are the agent's.
type Options struct {
	Limits    IndexLimits   // zero fields: DefaultIndexLimits
	MaxCached int           // indexes kept, builds in progress included; 0: 3
	FirstWait time.Duration // the request that starts a build waits that long before 202; 0: no wait
	IdleTTL   time.Duration // an index unused that long is dropped (review I5); 0: 10 min
	Grace     time.Duration // a finished index no request saw becomes evictable after that (R20); 0: 30 s
	SlowRetry time.Duration // a too_large of the time cap expires after that (Q11); 0: 5 min
	// ReadCgroup replaces the reading of /proc/<pid>/cgroup (spec §5.1 step 3).
	// nil in production; tests give one to a process of their own.
	ReadCgroup func(pid int) ([]byte, error)
}

// DefaultOptions and NewViews: see Interfaces. NewViews replaces every zero
// or negative field of opt.Limits by DefaultIndexLimits', MaxCached by 3,
// IdleTTL by 10 min, Grace by 30 s, SlowRetry by 5 min, then returns
// &Views{docker, opt, builds: map[string]*build{}}.

func DefaultOptions() Options {
	return Options{Limits: DefaultIndexLimits, MaxCached: 3, FirstWait: time.Second,
		IdleTTL: 10 * time.Minute, Grace: 30 * time.Second, SlowRetry: 5 * time.Minute}
}

func NewViews(docker dockerx.Client, opt Options) *Views {
	d := DefaultIndexLimits
	if opt.Limits.MaxBytes <= 0 {
		opt.Limits.MaxBytes = d.MaxBytes
	}
	if opt.Limits.MaxDuration <= 0 {
		opt.Limits.MaxDuration = d.MaxDuration
	}
	if opt.Limits.MaxHeaders <= 0 {
		opt.Limits.MaxHeaders = d.MaxHeaders
	}
	if opt.MaxCached <= 0 {
		opt.MaxCached = 3
	}
	if opt.IdleTTL <= 0 {
		opt.IdleTTL = 10 * time.Minute
	}
	if opt.Grace <= 0 {
		opt.Grace = 30 * time.Second
	}
	if opt.SlowRetry <= 0 {
		opt.SlowRetry = 5 * time.Minute
	}
	return &Views{docker: docker, opt: opt, builds: map[string]*build{}}
}

type build struct {
	id      string
	done    chan struct{} // closed once idx, err and doneAt are set
	cancel  context.CancelFunc
	read    atomic.Int64
	total   atomic.Int64 // SizeRootFs; 0 while unknown
	idx     *index
	err     error
	doneAt  time.Time
	served  bool        // under v.mu: its result reached a request at least once
	touched time.Time   // under v.mu: last request for it
	idle    *time.Timer // under v.mu: drops the finished index after IdleTTL unused
}

func finished(b *build) bool {
	select {
	case <-b.done:
		return true
	default:
		return false
	}
}

// For: id may be a name or a short ID; errors of Inspect pass through.
func (v *Views) For(ctx context.Context, id string) (FS, error) {
	info, err := v.docker.Inspect(ctx, id)
	if err != nil {
		return nil, err
	}
	if info.Running && info.Pid != 0 {
		return openProcFS(v.docker, info, v.opt.ReadCgroup)
	}
	// Spec §5.2 step 7: a container whose mounts may expose the host gets
	// no index at all, not even the tar of "/".
	if err := HostBind(ctx, v.docker, info.Mounts); err != nil {
		return nil, err
	}
	key := info.ID + "\x00" + info.FinishedAt
	idx, err := v.index(ctx, key, info)
	if err != nil {
		return nil, err
	}
	return newTarFS(v.docker, info, idx, func() { v.drop(key, nil) }), nil
}

// Forget drops every index of container id (full ID): plan Explorateur 3
// calls it after a write on a stopped container (spec §5.2 step 1).
func (v *Views) Forget(id string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for k, b := range v.builds {
		if b.id == id {
			v.removeLocked(k)
		}
	}
}

// drop removes key if it still holds b (any build when b is nil).
func (v *Views) drop(key string, b *build) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if cur, ok := v.builds[key]; ok && (b == nil || cur == b) {
		v.removeLocked(key)
	}
}

func (v *Views) index(ctx context.Context, key string, info dockerx.ContainerInfo) (*index, error) {
	v.mu.Lock()
	b, ok := v.builds[key]
	if ok && finished(b) && errors.Is(b.err, errTooSlow) && time.Since(b.doneAt) > v.opt.SlowRetry {
		v.removeLocked(key) // Q11: the time cap may have met a slow moment of Docker
		ok = false
	}
	if !ok {
		for k, old := range v.builds {
			if old.id == info.ID {
				v.removeLocked(k) // restarted since: stale
			}
		}
		if b = v.startLocked(key, info); b == nil {
			v.mu.Unlock()
			return nil, &IndexingError{} // every slot holds a build in progress (review I1)
		}
	}
	v.touchLocked(key)
	v.mu.Unlock()
	if !ok && v.opt.FirstWait > 0 { // only the request that started it: polls never hold a guard slot
		timer := time.NewTimer(v.opt.FirstWait)
		defer timer.Stop()
		select {
		case <-b.done:
		case <-timer.C:
		case <-ctx.Done():
		}
	}
	if !finished(b) {
		return nil, &IndexingError{Progress: progress(b.read.Load(), b.total.Load())}
	}
	v.mu.Lock()
	b.served = true
	if b.err != nil && !errors.Is(b.err, ErrTooLarge) && v.builds[key] == b {
		v.removeLocked(key) // delivered once: the next access builds again
	}
	v.mu.Unlock()
	if b.err != nil {
		return nil, b.err
	}
	return b.idx, nil
}

// startLocked starts the build of key, or returns nil when no slot can be
// freed. Review I1: a build in progress is never evicted, nor, for Grace, an
// index no request has seen yet; otherwise four containers polled at once
// would evict each other forever. R20: past Grace, an unseen index (its
// request went away) is evictable too. The victim is the least recently used.
func (v *Views) startLocked(key string, info dockerx.ContainerInfo) *build {
	for len(v.builds) >= v.opt.MaxCached {
		i := slices.IndexFunc(v.order, func(k string) bool {
			b := v.builds[k]
			return finished(b) && (b.served || time.Since(b.touched) > v.opt.Grace)
		})
		if i < 0 {
			return nil
		}
		v.removeLocked(v.order[i])
	}
	ctx, cancel := context.WithTimeout(context.Background(), v.opt.Limits.MaxDuration)
	b := &build{id: info.ID, done: make(chan struct{}), cancel: cancel}
	v.builds[key] = b
	v.order = append(v.order, key)
	go v.run(ctx, key, b, info)
	return b
}

// touchLocked records a request for key: last use, idle timer re-armed,
// end of v.order (most recently used).
func (v *Views) touchLocked(key string) {
	b := v.builds[key]
	b.touched = time.Now()
	if b.idle != nil {
		b.idle.Reset(v.opt.IdleTTL)
	}
	if i := slices.Index(v.order, key); i >= 0 {
		v.order = append(slices.Delete(v.order, i, i+1), key)
	}
}

// removeLocked stops the idle timer and cancels the build of key, then
// forgets it (a missing key is a no-op).
func (v *Views) removeLocked(key string) {
	if b, ok := v.builds[key]; ok {
		if b.idle != nil {
			b.idle.Stop()
		}
		b.cancel()
		delete(v.builds, key)
	}
	if i := slices.Index(v.order, key); i >= 0 {
		v.order = slices.Delete(v.order, i, i+1)
	}
}

func (v *Views) run(ctx context.Context, key string, b *build, info dockerx.ContainerInfo) {
	defer b.cancel()
	go func() { // progress only: an unknown size shows 0 %
		if size, err := v.docker.SizeRootFs(ctx, info.ID); err == nil {
			b.total.Store(size)
		}
	}()
	idx, err := v.buildFor(ctx, b, info)
	if err != nil && errors.Is(ctx.Err(), context.Canceled) {
		err = &IndexingError{} // removed meanwhile (restart, Forget): its waiters ask again, never a 503
	}
	b.idx, b.err, b.doneAt = idx, err, time.Now()
	close(b.done)
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.builds[key] != b {
		return
	}
	if err != nil && !errors.Is(err, ErrTooLarge) {
		return // kept until one request has received it (see index), so that a poll never rebuilds before seeing the error
	}
	if err != nil {
		return // a too_large holds no index: sticky until FinishedAt changes (Q11, R21)
	}
	// Review I5: spec §5.2 step 4 counts memory at rest; an index unused for
	// IdleTTL goes away.
	b.idle = time.AfterFunc(v.opt.IdleTTL, func() { v.drop(key, b) })
}

func (v *Views) buildFor(ctx context.Context, b *build, info dockerx.ContainerInfo) (*index, error) {
	rc, err := v.docker.GetArchive(ctx, info.ID, "/")
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: index beyond %s (%w)", ErrTooLarge, v.opt.Limits.MaxDuration, errTooSlow)
		}
		return nil, err
	}
	defer rc.Close()
	idx, err := buildIndex(ctx, rc, v.opt.Limits, &b.read)
	if err != nil {
		return nil, err
	}
	// Started, or restarted and stopped, during the build: the archive may
	// mix two states of the container.
	now, err := v.docker.Inspect(ctx, info.ID)
	if err != nil {
		return nil, err
	}
	if (now.Running && now.Pid != 0) || now.FinishedAt != info.FinishedAt {
		return nil, changed("container %.12s started during indexing", info.ID)
	}
	return idx, nil
}
