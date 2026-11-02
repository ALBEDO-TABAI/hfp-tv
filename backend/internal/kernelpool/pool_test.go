package kernelpool

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"infinite-canvas/backend/internal/projectsnapshot"
)

type fakeKernel struct {
	id      int
	idle    atomic.Bool
	closed  atomic.Bool
	closing chan struct{} // when set, Close blocks until it is closed
}

func (k *fakeKernel) GatewayHandler() http.Handler { return http.NotFoundHandler() }
func (k *fakeKernel) ProjectSnapshots() (*projectsnapshot.Service, error) {
	return nil, nil
}
func (k *fakeKernel) Idle(context.Context) bool { return k.idle.Load() }
func (k *fakeKernel) Close(context.Context) error {
	if k.closing != nil {
		<-k.closing
	}
	k.closed.Store(true)
	return nil
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

type fakeOpener struct {
	mu      sync.Mutex
	opened  []*fakeKernel
	closing chan struct{}
}

func (o *fakeOpener) open(Spec) (Kernel, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	kernel := &fakeKernel{id: len(o.opened) + 1, closing: o.closing}
	kernel.idle.Store(true)
	o.opened = append(o.opened, kernel)
	return kernel, nil
}

func (o *fakeOpener) count() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.opened)
}

func newTestPool(t *testing.T) (*Pool, *fakeOpener, *fakeClock) {
	t.Helper()
	pool := New(Options{IdleTimeout: 30 * time.Minute, ShutdownTimeout: 5 * time.Second})
	opener := &fakeOpener{}
	clock := &fakeClock{now: time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)}
	pool.openKernel = opener.open
	pool.now = clock.Now
	t.Cleanup(func() { _ = pool.CloseAll(context.Background()) })
	return pool, opener, clock
}

var spec = Spec{ID: "w1", DataDir: "/unused"}

func TestAcquireOpensOnceAndReapsOnlyIdleUnusedKernels(t *testing.T) {
	pool, opener, clock := newTestPool(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	releases := make([]func(), 8)
	for index := range releases {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, release, err := pool.Acquire(ctx, spec)
			if err != nil {
				t.Error(err)
				return
			}
			releases[index] = release
		}()
	}
	wg.Wait()
	if opener.count() != 1 {
		t.Fatalf("concurrent acquires opened %d kernels", opener.count())
	}
	first := opener.opened[0]

	// Still in use: never closed, however long ago it was acquired.
	clock.Advance(2 * time.Hour)
	pool.reapIdle(ctx)
	if first.closed.Load() {
		t.Fatal("reaped a kernel with requests in flight")
	}
	for _, release := range releases {
		release()
		release() // a second release is a no-op
	}

	// Released recently: kept.
	clock.Advance(10 * time.Minute)
	pool.reapIdle(ctx)
	if first.closed.Load() {
		t.Fatal("reaped a kernel used 10 minutes ago")
	}

	// Busy with work: kept even after the idle timeout.
	first.idle.Store(false)
	clock.Advance(time.Hour)
	pool.reapIdle(ctx)
	if first.closed.Load() || pool.OpenCount() != 1 {
		t.Fatal("reaped a kernel that reported running work")
	}

	// Idle and unused: closed, and the next request opens a fresh kernel.
	first.idle.Store(true)
	pool.reapIdle(ctx)
	if !first.closed.Load() || pool.OpenCount() != 0 {
		t.Fatal("idle kernel was not reclaimed")
	}
	kernel, release, err := pool.Acquire(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if kernel.(*fakeKernel).id != 2 || opener.count() != 2 {
		t.Fatalf("reopen returned kernel %d after %d opens", kernel.(*fakeKernel).id, opener.count())
	}
}

func TestWarmKernelIsReclaimedOnceIdle(t *testing.T) {
	pool, opener, _ := newTestPool(t)
	ctx := context.Background()
	opener.closing = nil
	if err := pool.Warm(ctx, spec); err != nil {
		t.Fatal(err)
	}
	warm := opener.opened[0]
	warm.idle.Store(false)
	pool.reapIdle(ctx)
	if warm.closed.Load() {
		t.Fatal("warm kernel with queued work was closed")
	}
	warm.idle.Store(true)
	pool.reapIdle(ctx)
	if !warm.closed.Load() {
		t.Fatal("warm kernel with nothing to do stayed open")
	}
}

func TestReopenWaitsForClose(t *testing.T) {
	pool, opener, clock := newTestPool(t)
	ctx := context.Background()
	opener.closing = make(chan struct{})
	_, release, err := pool.Acquire(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	release()
	clock.Advance(time.Hour)
	reaped := make(chan struct{})
	go func() {
		pool.reapIdle(ctx)
		close(reaped)
	}()
	for pool.OpenCount() != 0 {
		time.Sleep(time.Millisecond)
	}
	acquired := make(chan struct{})
	go func() {
		_, release, err := pool.Acquire(ctx, spec)
		if err == nil {
			release()
		}
		close(acquired)
	}()
	select {
	case <-acquired:
		t.Fatal("a second kernel opened while the first was still closing")
	case <-time.After(50 * time.Millisecond):
	}
	if opener.count() != 1 {
		t.Fatalf("opened %d kernels while closing", opener.count())
	}
	close(opener.closing)
	<-reaped
	<-acquired
	if opener.count() != 2 || !opener.opened[0].closed.Load() {
		t.Fatal("reopen did not follow the finished close")
	}
}
