package platform

import (
	"context"
	"log"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

type Worker struct {
	mu          sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	started     bool
	draining    atomic.Bool
	paused      atomic.Bool
	activeTasks atomic.Int64
}

func (r *Worker) Start() (context.Context, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return r.ctx, false
	}
	r.ctx, r.cancel = context.WithCancel(context.Background())
	r.started = true
	r.draining.Store(false)
	r.paused.Store(false)
	return r.ctx, true
}

func (r *Worker) GoLoop(fn func(context.Context)) bool {
	r.mu.Lock()
	if !r.started || r.draining.Load() || r.paused.Load() {
		r.mu.Unlock()
		return false
	}
	ctx := r.ctx
	r.wg.Add(1)
	r.mu.Unlock()
	go func() {
		defer r.wg.Done()
		// A panic must not take down the process: the team server runs many
		// workspaces in one process. Restart the loop until the worker drains.
		for recoverWorkerPanic("loop", func() { fn(ctx) }) && ctx.Err() == nil {
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
			}
		}
	}()
	return true
}

func (r *Worker) GoTask(fn func()) bool {
	r.mu.Lock()
	if !r.started || r.draining.Load() || r.paused.Load() {
		r.mu.Unlock()
		return false
	}
	r.wg.Add(1)
	r.activeTasks.Add(1)
	r.mu.Unlock()
	go func() {
		defer r.wg.Done()
		defer r.activeTasks.Add(-1)
		// The task's lease expires and restart recovery settles its state.
		recoverWorkerPanic("task", fn)
	}()
	return true
}

// recoverWorkerPanic reports whether fn panicked.
func recoverWorkerPanic(kind string, fn func()) (panicked bool) {
	defer func() {
		if value := recover(); value != nil {
			panicked = true
			log.Printf("background %s panic recovered: %v\n%s", kind, value, debug.Stack())
		}
	}()
	fn()
	return false
}

func (r *Worker) BeginDrain() {
	r.mu.Lock()
	r.draining.Store(true)
	if r.cancel != nil {
		r.cancel()
	}
	r.mu.Unlock()
}

func (r *Worker) Stop(ctx context.Context) error {
	r.BeginDrain()
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		r.mu.Lock()
		r.started = false
		r.ctx = nil
		r.cancel = nil
		r.mu.Unlock()
		return nil
	}
}

func (r *Worker) IsDraining() bool { return r.draining.Load() || r.paused.Load() }

// PauseIfIdle fences dispatch before a reversible storage-migration preflight.
// Unlike BeginDrain it leaves loop contexts alive so a rejected migration can resume.
func (r *Worker) PauseIfIdle() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.draining.Load() || r.activeTasks.Load() != 0 {
		return false
	}
	r.paused.Store(true)
	return true
}

func (r *Worker) Resume() { r.paused.Store(false) }

// Context is the runtime-owned cancellation scope. It is nil when the
// worker has not started or has fully stopped. Callers must not Start
// the worker to obtain it.
func (r *Worker) Context() context.Context {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.started || r.ctx == nil {
		return nil
	}
	return r.ctx
}

func (r *Worker) ActiveTaskCount() int64 { return r.activeTasks.Load() }
