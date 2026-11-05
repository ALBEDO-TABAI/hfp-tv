package kernelpool

import (
	"context"
	"errors"
	"infinite-canvas/backend/internal/workspace"
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

func TestReprovisionSerializesWithInitialProvision(t *testing.T) {
	p, _, _ := newTestPool(t)
	p.SetRelayTemplate(map[string]any{"baseUrl": "https://relay.invalid/v1"})
	dir := t.TempDir()
	entered, resume := make(chan struct{}), make(chan struct{})
	first, second := make(chan error, 1), make(chan error, 1)
	go func() {
		first <- p.provision(Spec{ID: "same", DataDir: dir, RelayKey: func() (string, error) { close(entered); <-resume; return "SYNTHETIC_OLD", nil }})
	}()
	<-entered
	go func() {
		second <- p.Reprovision(Spec{ID: "same", DataDir: dir, RelayKey: func() (string, error) { return "", nil }})
	}()
	var early bool
	select {
	case err := <-second:
		early = true
		if err != nil {
			t.Error(err)
		}
	case <-time.After(30 * time.Millisecond):
	}
	close(resume)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if !early {
		if err := <-second; err != nil {
			t.Fatal(err)
		}
	}
	store, _ := workspace.NewProviderConfig(dir)
	got, _, err := store.LoadEffectiveModelConfig()
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range got.Config["channels"].([]any) {
		channel := raw.(map[string]any)
		if channel["id"] == RelayChannelID && channel["apiKey"] != "" {
			t.Error("older provisioning resurrected cleared credential")
		}
	}
	if early {
		t.Error("reprovision bypassed in-progress provisioning")
	}
}

func TestFailedReprovisionClosesOpenKernel(t *testing.T) {
	p, opener, _ := newTestPool(t)
	s := Spec{ID: "failure", DataDir: t.TempDir(), RelayKey: func() (string, error) { return "", errors.New("synthetic failure") }}
	_, release, err := p.Acquire(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	release()
	p.SetRelayTemplate(map[string]any{"baseUrl": "https://relay.invalid/v1"})
	if err := p.Reprovision(s); err == nil {
		t.Fatal("expected failed provisioning")
	}
	if p.OpenCount() != 0 || !opener.opened[0].closed.Load() {
		t.Fatal("failed provisioning left old kernel usable")
	}
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

// A failed reprovision closes only the kernel that may hold the old settings;
// one opened after that attempt began provisioned itself and keeps running.
func TestFailedReprovisionLeavesALaterKernelOpen(t *testing.T) {
	p, opener, _ := newTestPool(t)
	s := Spec{ID: "later", DataDir: t.TempDir(), RelayKey: func() (string, error) { return "SYNTHETIC_KEY_1234", nil }}
	p.SetRelayTemplate(map[string]any{"baseUrl": "https://relay.invalid/v1"})
	_, release, err := p.Acquire(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	release()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// The failed attempt captured an earlier kernel, since replaced.
	if err := p.closeEntry(ctx, s.ID, &entry{ready: make(chan struct{})}); err != nil {
		t.Fatal(err)
	}
	if p.OpenCount() != 1 || opener.opened[0].closed.Load() {
		t.Fatal("closed a kernel opened after the failed attempt began")
	}
	// With no kernel open when it started, a failed reprovision closes nothing.
	failing := s
	failing.ID = "never-open"
	failing.RelayKey = func() (string, error) { return "", errors.New("synthetic failure") }
	if err := p.Reprovision(failing); err == nil {
		t.Fatal("expected failed provisioning")
	}
	if p.OpenCount() != 1 {
		t.Fatal("unrelated kernel closed")
	}
}

// A kernel still opening waits for the provisioning lock; a reprovision must
// wait for it without holding that lock, then decide against the started one.
func TestReprovisionWaitsForAKernelStillOpeningWithoutDeadlock(t *testing.T) {
	p, _, _ := newTestPool(t)
	p.SetRelayTemplate(map[string]any{"baseUrl": "https://relay.invalid/v1"})
	opening := &entry{ready: make(chan struct{})}
	p.mu.Lock()
	p.kernels["opening"] = opening
	p.mu.Unlock()
	go func() {
		// The open needs the provisioning lock to finish.
		unlock := p.lockProvision("opening")
		time.Sleep(50 * time.Millisecond)
		unlock()
		close(opening.ready)
	}()
	done := make(chan error, 1)
	go func() {
		done <- p.Reprovision(Spec{ID: "opening", DataDir: t.TempDir(), RelayKey: func() (string, error) { return "", errors.New("synthetic failure") }})
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected failed provisioning")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("reprovision deadlocked with a kernel that was still opening")
	}
	p.mu.Lock()
	_, stillThere := p.kernels["opening"]
	p.mu.Unlock()
	if stillThere {
		t.Fatal("the kernel that finished opening with old settings was not closed")
	}
}
