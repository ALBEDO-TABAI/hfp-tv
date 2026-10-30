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
	"infinite-canvas/backend/internal/projectsnapshot"
	"infinite-canvas/backend/internal/referencestorage"
)

// Kernel is the only surface the team server uses to reach a workspace. A
// container-per-user deployment would implement it over the network.
type Kernel interface {
	GatewayHandler() http.Handler
	ProjectSnapshots() (*projectsnapshot.Service, error)
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
}

type entry struct {
	ready  chan struct{}
	kernel Kernel
	err    error
}

type Pool struct {
	opts    Options
	mu      sync.Mutex
	kernels map[string]*entry
	closed  bool
}

func New(opts Options) *Pool {
	if opts.ShutdownTimeout <= 0 {
		opts.ShutdownTimeout = 2 * time.Minute
	}
	if opts.RelayTemplate != nil {
		opts.Hosted.ManagedChannelIDs = append(opts.Hosted.ManagedChannelIDs, RelayChannelID)
	}
	return &Pool{opts: opts, kernels: make(map[string]*entry)}
}

var errPoolClosed = errors.New("团队服务器正在关闭")

// Get returns the open kernel for spec, opening it once even under concurrent
// requests. A failed open is not cached: the next request retries.
func (p *Pool) Get(ctx context.Context, spec Spec) (Kernel, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, errPoolClosed
	}
	current, exists := p.kernels[spec.ID]
	if !exists {
		current = &entry{ready: make(chan struct{})}
		p.kernels[spec.ID] = current
		p.mu.Unlock()
		current.kernel, current.err = p.open(spec)
		if current.err != nil {
			p.mu.Lock()
			if p.kernels[spec.ID] == current {
				delete(p.kernels, spec.ID)
			}
			p.mu.Unlock()
		}
		close(current.ready)
	} else {
		p.mu.Unlock()
	}
	select {
	case <-current.ready:
		return current.kernel, current.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
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
	runtime, err := bootstrap.Open(context.Background(), bootstrap.Config{
		Profile: bootstrap.ProfileHosted, DataDir: spec.DataDir, AutoMigrate: true,
		ShutdownTimeout: p.opts.ShutdownTimeout, Hosted: p.opts.Hosted,
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
	if p.opts.RelayTemplate == nil {
		return nil
	}
	key := ""
	if spec.RelayKey != nil {
		var err error
		if key, err = spec.RelayKey(); err != nil {
			return err
		}
	}
	return applyRelayChannel(spec.DataDir, p.opts.RelayTemplate, key)
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

// Close shuts one kernel down (account disabled). Missing kernels are a no-op.
func (p *Pool) Close(ctx context.Context, id string) error {
	p.mu.Lock()
	current, exists := p.kernels[id]
	if exists {
		delete(p.kernels, id)
	}
	p.mu.Unlock()
	if !exists {
		return nil
	}
	<-current.ready
	if current.kernel == nil {
		return nil
	}
	return current.kernel.Close(ctx)
}

// CloseAll stops accepting opens and closes every kernel in parallel.
func (p *Pool) CloseAll(ctx context.Context) error {
	p.mu.Lock()
	p.closed = true
	entries := make([]*entry, 0, len(p.kernels))
	for id, current := range p.kernels {
		entries = append(entries, current)
		delete(p.kernels, id)
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
	return errors.Join(errs...)
}

// OpenCount reports how many kernels are open or opening.
func (p *Pool) OpenCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.kernels)
}
