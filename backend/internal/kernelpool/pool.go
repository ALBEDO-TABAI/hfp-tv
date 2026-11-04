// Package kernelpool opens one hosted workspace kernel per team-server user
// inside the server process and hands the gateway its handler.
package kernelpool

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"infinite-canvas/backend/internal/bootstrap"
	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/projectsnapshot"
	"infinite-canvas/backend/internal/referencestorage"
)

// Kernel is the only surface the team server uses to reach a workspace. A
// container-per-user deployment would implement it over the network.
type Kernel interface {
	GatewayHandler() http.Handler
	ProjectSnapshots() (*projectsnapshot.Service, error)
	// Idle reports that closing the kernel interrupts nothing: no queued or
	// running task, no background job and no assistant turn.
	Idle(ctx context.Context) bool
	Close(ctx context.Context) error
}

// Spec is what the pool needs to open or provision one workspace. RelayKey is
// resolved only when provisioning, not on every gateway request.
type Spec struct {
	ID       string
	DataDir  string // absolute
	RelayKey func() (string, error)
}

type Options struct {
	Hosted           bootstrap.HostedConfig
	RelayTemplate    map[string]any
	ReferenceStorage *referencestorage.Config
	ShutdownTimeout  time.Duration
	// IdleTimeout closes a kernel this long after its last request once it
	// reports Idle; the next request reopens it. Zero keeps kernels open.
	IdleTimeout time.Duration
	// ReapInterval is how often idle kernels are looked for (default 1 minute).
	ReapInterval time.Duration
	// ReportFailure receives failed tasks from every kernel, tagged with the
	// workspace they came from.
	ReportFailure func(workspaceID string, report kernel.FailureReport)
}

type entry struct {
	ready  chan struct{}
	kernel Kernel
	err    error
	// inflight and lastUsed are guarded by Pool.mu. A zero lastUsed marks a
	// kernel opened only to resume work, reclaimable as soon as it is idle.
	inflight int
	lastUsed time.Time
}

type Pool struct {
	opts    Options
	mu      sync.Mutex
	kernels map[string]*entry
	// closing holds kernels being shut down; an open of the same workspace
	// waits so two kernels never share one data directory.
	closing map[string]chan struct{}
	closed  bool
	now     func() time.Time
	// openKernel is p.open; tests replace it.
	openKernel func(Spec) (Kernel, error)
	// relay is the company relay definition written into every workspace; an
	// administrator can replace it while the server runs.
	relayMu sync.RWMutex
	relay   map[string]any
}

// RelayTemplate returns the company relay definition in effect, or nil.
func (p *Pool) RelayTemplate() map[string]any {
	p.relayMu.RLock()
	defer p.relayMu.RUnlock()
	return p.relay
}

// SetRelayTemplate replaces the definition used by later provisioning. Call
// Reprovision for each workspace to apply it to kernels that are open.
func (p *Pool) SetRelayTemplate(template map[string]any) {
	p.relayMu.Lock()
	defer p.relayMu.Unlock()
	p.relay = template
}

func New(opts Options) *Pool {
	if opts.ShutdownTimeout <= 0 {
		opts.ShutdownTimeout = 2 * time.Minute
	}
	if opts.ReapInterval <= 0 {
		opts.ReapInterval = time.Minute
	}
	// Managed even before a relay is defined: an administrator can define it
	// from the browser while kernels are open, and a managed channel never
	// has its key echoed to the browser.
	opts.Hosted.ManagedChannelIDs = append(opts.Hosted.ManagedChannelIDs, RelayChannelID)
	pool := &Pool{opts: opts, kernels: make(map[string]*entry), closing: make(map[string]chan struct{}), now: time.Now, relay: opts.RelayTemplate}
	pool.openKernel = pool.open
	return pool
}

var errPoolClosed = errors.New("团队服务器正在关闭")

// Acquire returns the open kernel for spec and marks it in use until release
// is called. The kernel is opened once even under concurrent requests; a
// failed open is not cached, so the next request retries.
func (p *Pool) Acquire(ctx context.Context, spec Spec) (Kernel, func(), error) {
	current, err := p.entryFor(ctx, spec, true)
	if err != nil {
		return nil, nil, err
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			p.mu.Lock()
			current.inflight--
			current.lastUsed = p.now()
			p.mu.Unlock()
		})
	}
	select {
	case <-current.ready:
	case <-ctx.Done():
		release()
		return nil, nil, ctx.Err()
	}
	if current.err != nil {
		release()
		return nil, nil, current.err
	}
	return current.kernel, release, nil
}

// Warm opens a kernel so queued work resumes after a restart. It does not
// count as use: the reaper closes it once its work is done.
func (p *Pool) Warm(ctx context.Context, spec Spec) error {
	current, err := p.entryFor(ctx, spec, false)
	if err != nil {
		return err
	}
	select {
	case <-current.ready:
		return current.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Pool) entryFor(ctx context.Context, spec Spec, use bool) (*entry, error) {
	for {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return nil, errPoolClosed
		}
		if done, closing := p.closing[spec.ID]; closing {
			p.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		current, exists := p.kernels[spec.ID]
		if !exists {
			current = &entry{ready: make(chan struct{})}
			p.kernels[spec.ID] = current
		}
		if use {
			current.inflight++
			current.lastUsed = p.now()
		}
		p.mu.Unlock()
		if !exists {
			go p.load(spec, current)
		}
		return current, nil
	}
}

func (p *Pool) load(spec Spec, current *entry) {
	current.kernel, current.err = p.openKernel(spec)
	if current.err != nil {
		p.mu.Lock()
		if p.kernels[spec.ID] == current {
			delete(p.kernels, spec.ID)
		}
		p.mu.Unlock()
	}
	close(current.ready)
}

func (p *Pool) open(spec Spec) (Kernel, error) {
	if spec.ID == "" || spec.DataDir == "" {
		return nil, errors.New("工作区登记不完整")
	}
	if err := os.MkdirAll(spec.DataDir, 0o700); err != nil {
		return nil, err
	}
	if err := p.provision(spec); err != nil {
		return nil, fmt.Errorf("准备工作区配置失败：%w", err)
	}
	hosted := p.opts.Hosted
	if report := p.opts.ReportFailure; report != nil {
		workspaceID := spec.ID
		hosted.ReportFailure = func(failure kernel.FailureReport) { report(workspaceID, failure) }
	}
	runtime, err := bootstrap.Open(context.Background(), bootstrap.Config{
		Profile: bootstrap.ProfileHosted, DataDir: spec.DataDir, AutoMigrate: true,
		ShutdownTimeout: p.opts.ShutdownTimeout, Hosted: hosted,
	})
	if err != nil {
		return nil, err
	}
	if err := runtime.Start(); err != nil {
		_ = runtime.Close(context.Background())
		return nil, err
	}
	log.Printf("workspace kernel opened workspace=%s", spec.ID)
	return runtime, nil
}

func (p *Pool) provision(spec Spec) error {
	if err := applyReferenceStorage(spec.DataDir, p.opts.ReferenceStorage); err != nil {
		return err
	}
	template := p.RelayTemplate()
	if template == nil {
		return nil
	}
	key := ""
	if spec.RelayKey != nil {
		var err error
		if key, err = spec.RelayKey(); err != nil {
			return err
		}
	}
	return applyRelayChannel(spec.DataDir, template, key)
}

// Reprovision rewrites deployment-managed settings, e.g. after an admin binds
// or clears a relay key. The provider config file has a per-directory lock, so
// this is safe while the kernel is open; the assistant child picks up a changed
// provider on its next status check.
func (p *Pool) Reprovision(spec Spec) error {
	if err := os.MkdirAll(spec.DataDir, 0o700); err != nil {
		return err
	}
	return p.provision(spec)
}

// detach removes an open kernel from the pool and marks it closing. The
// returned finish must be called once the kernel is closed.
func (p *Pool) detach(id string) func() {
	delete(p.kernels, id)
	done := make(chan struct{})
	p.closing[id] = done
	return func() {
		p.mu.Lock()
		if p.closing[id] == done {
			delete(p.closing, id)
		}
		p.mu.Unlock()
		close(done)
	}
}

// Close shuts one kernel down (account disabled). Missing kernels are a no-op.
func (p *Pool) Close(ctx context.Context, id string) error {
	p.mu.Lock()
	current, exists := p.kernels[id]
	if !exists {
		p.mu.Unlock()
		return nil
	}
	finish := p.detach(id)
	p.mu.Unlock()
	defer finish()
	<-current.ready
	if current.kernel == nil {
		return nil
	}
	return current.kernel.Close(ctx)
}

// RunReaper closes idle kernels until ctx ends. It is a no-op without an
// IdleTimeout.
func (p *Pool) RunReaper(ctx context.Context) {
	if p.opts.IdleTimeout <= 0 {
		return
	}
	ticker := time.NewTicker(p.opts.ReapInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.reapIdle(ctx)
		}
	}
}

func (p *Pool) reapIdle(ctx context.Context) {
	type candidate struct {
		id       string
		current  *entry
		lastUsed time.Time
	}
	now := p.now()
	var candidates []candidate
	p.mu.Lock()
	for id, current := range p.kernels {
		select {
		case <-current.ready:
		default:
			continue
		}
		if current.kernel == nil || current.inflight > 0 || now.Sub(current.lastUsed) < p.opts.IdleTimeout {
			continue
		}
		candidates = append(candidates, candidate{id: id, current: current, lastUsed: current.lastUsed})
	}
	p.mu.Unlock()
	for _, item := range candidates {
		probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		idle := item.current.kernel.Idle(probeCtx)
		cancel()
		if !idle || ctx.Err() != nil {
			continue
		}
		p.mu.Lock()
		// A request may have arrived while Idle was probing.
		if p.closed || p.kernels[item.id] != item.current || item.current.inflight > 0 || !item.current.lastUsed.Equal(item.lastUsed) {
			p.mu.Unlock()
			continue
		}
		finish := p.detach(item.id)
		p.mu.Unlock()
		closeCtx, cancelClose := context.WithTimeout(context.Background(), p.opts.ShutdownTimeout)
		if err := item.current.kernel.Close(closeCtx); err != nil {
			log.Printf("close idle workspace=%s: %v", item.id, err)
		} else {
			log.Printf("workspace kernel closed after idle workspace=%s", item.id)
		}
		cancelClose()
		finish()
	}
}

// CloseAll stops accepting opens, closes every kernel in parallel and waits
// for closes already in progress.
func (p *Pool) CloseAll(ctx context.Context) error {
	p.mu.Lock()
	p.closed = true
	entries := make([]*entry, 0, len(p.kernels))
	for id, current := range p.kernels {
		entries = append(entries, current)
		delete(p.kernels, id)
	}
	closing := make([]chan struct{}, 0, len(p.closing))
	for _, done := range p.closing {
		closing = append(closing, done)
	}
	p.mu.Unlock()
	var wg sync.WaitGroup
	errs := make([]error, len(entries))
	for index, current := range entries {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-current.ready
			if current.kernel != nil {
				errs[index] = current.kernel.Close(ctx)
			}
		}()
	}
	wg.Wait()
	for _, done := range closing {
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
	return errors.Join(errs...)
}

// OpenCount reports how many kernels are open or opening.
func (p *Pool) OpenCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.kernels)
}
