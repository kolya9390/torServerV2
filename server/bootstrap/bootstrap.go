package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"server/config"
	internalapp "server/internal/app"
	"server/log"
	"server/settings"
)

const defaultStopTimeout = 30 * time.Second

// Bootstrap wires application runtime and provides lifecycle operations.
type Bootstrap struct {
	app            *internalapp.App
	cleanupDeps    cacheCleanupDeps
	cleanupDepsSet bool

	cleanupMu     sync.Mutex
	cleanupCancel context.CancelFunc
	cleanupWG     sync.WaitGroup
}

// New creates a bootstrap instance with explicit runtime wiring.
func New(args *settings.ExecArgs, cfg *config.Config) (*Bootstrap, error) {
	if args == nil {
		return nil, errors.New("nil exec args")
	}

	// Keep the legacy process-global args snapshot in sync for compatibility wrappers.
	settings.SetArgs(args)

	runtime := internalapp.NewRuntimeWithConfig(cfg)

	return newWithRuntimeAndCleanupDeps(runtime, defaultCacheCleanupDeps())
}

func newWithRuntime(runtime internalapp.Runtime) (*Bootstrap, error) {
	return newWithRuntimeAndCleanupDeps(runtime, cacheCleanupDeps{})
}

func newWithRuntimeAndCleanupDeps(runtime internalapp.Runtime, cleanupDeps cacheCleanupDeps) (*Bootstrap, error) {
	if runtime == nil {
		return nil, errors.New("runtime is not initialized")
	}

	app := internalapp.New(runtime, defaultStopTimeout)

	return &Bootstrap{
		app:            app,
		cleanupDeps:    cleanupDeps,
		cleanupDepsSet: cleanupDeps.settingsProvider != nil || cleanupDeps.readDir != nil || cleanupDeps.remove != nil || cleanupDeps.listTorrents != nil,
	}, nil
}

// Start starts the application runtime.
func (b *Bootstrap) Start(ctx context.Context) error {
	if b == nil || b.app == nil {
		return errors.New("bootstrap is not initialized")
	}

	if err := b.app.Start(ctx); err != nil {
		return fmt.Errorf("start app: %w", err)
	}

	b.startCleanupWorker()

	return nil
}

// Stop gracefully stops the application runtime.
func (b *Bootstrap) Stop(ctx context.Context) error {
	if b == nil || b.app == nil {
		return errors.New("bootstrap is not initialized")
	}

	b.stopCleanupWorker(ctx)

	if err := b.app.Stop(ctx); err != nil {
		return fmt.Errorf("stop app: %w", err)
	}

	log.TLogln("Bootstrap stop complete")

	return nil
}

// Wait blocks until runtime server exits.
func (b *Bootstrap) Wait() error {
	if b == nil || b.app == nil {
		return errors.New("bootstrap is not initialized")
	}

	return b.app.Wait()
}

func (b *Bootstrap) startCleanupWorker() {
	b.cleanupMu.Lock()
	defer b.cleanupMu.Unlock()

	if b.cleanupCancel != nil {
		return
	}

	cleanupCtx, cancel := context.WithCancel(context.Background())
	b.cleanupCancel = cancel
	b.cleanupWG.Add(1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.TLogln("bootstrap cleanup goroutine panic recovered", "panic", r)
			}
		}()
		defer b.cleanupWG.Done()

		if b.cleanupDepsSet {
			runCacheCleanupWithDeps(cleanupCtx, b.cleanupDeps)

			return
		}

		runCacheCleanup(cleanupCtx)
	}()
}

func (b *Bootstrap) stopCleanupWorker(ctx context.Context) {
	b.cleanupMu.Lock()
	cancel := b.cleanupCancel
	b.cleanupCancel = nil
	b.cleanupMu.Unlock()

	if cancel != nil {
		cancel()
	}

	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.TLogln("bootstrap stop cleanup goroutine panic recovered", "panic", r)
			}
		}()
		b.cleanupWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}
