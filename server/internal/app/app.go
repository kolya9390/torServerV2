package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"server/log"
)

// Runtime describes concrete application runtime lifecycle.
type Runtime interface {
	Start() error
	Stop()
	Wait() error
}

// App orchestrates runtime lifecycle with context-aware stop semantics.
type App struct {
	runtime     Runtime
	stopTimeout time.Duration
}

// New creates application orchestrator.
func New(runtime Runtime, stopTimeout time.Duration) *App {
	if stopTimeout <= 0 {
		stopTimeout = 30 * time.Second
	}

	return &App{
		runtime:     runtime,
		stopTimeout: stopTimeout,
	}
}

// Start starts runtime and respects early context cancellation.
func (a *App) Start(ctx context.Context) error {
	if a == nil || a.runtime == nil {
		return errors.New("app runtime is not configured")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return a.runtime.Start()
}

// Stop stops runtime with deadline bound from context or default timeout.
func (a *App) Stop(ctx context.Context) error {
	if a == nil || a.runtime == nil {
		return errors.New("app runtime is not configured")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	stopCtx := ctx

	var cancel context.CancelFunc

	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		stopCtx, cancel = context.WithTimeout(ctx, a.stopTimeout)
		defer cancel()
	}

	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.TLogln("app runtime stop goroutine panic recovered", "panic", r)
			}
		}()
		a.runtime.Stop()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-stopCtx.Done():
		return fmt.Errorf("stop timeout/cancel: %w", stopCtx.Err())
	}
}

// Wait waits until runtime exits.
func (a *App) Wait() error {
	if a == nil || a.runtime == nil {
		return errors.New("app runtime is not configured")
	}

	return a.runtime.Wait()
}
