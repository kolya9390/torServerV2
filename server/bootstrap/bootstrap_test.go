package bootstrap

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"server/config"
	internalapp "server/internal/app"
	"server/settings"
)

type fakeRuntime struct {
	startFn func() error
	stopFn  func()
	waitFn  func() error
}

func (f *fakeRuntime) Start() error {
	if f.startFn != nil {
		return f.startFn()
	}

	return nil
}

func (f *fakeRuntime) Stop() {
	if f.stopFn != nil {
		f.stopFn()
	}
}

func (f *fakeRuntime) Wait() error {
	if f.waitFn != nil {
		return f.waitFn()
	}

	return nil
}

func TestNewRequiresArgs(t *testing.T) {
	_, err := New(nil, nil)
	if err == nil {
		t.Fatal("expected error for nil args")
	}
}

func TestNewCreatesBootstrap(t *testing.T) {
	prevArgs := settings.GetArgs()

	t.Cleanup(func() {
		settings.SetArgs(prevArgs)
	})

	b, err := New(&settings.ExecArgs{}, &config.Config{})
	if err != nil {
		t.Fatalf("new failed: %v", err)
	}

	if b == nil || b.app == nil {
		t.Fatal("expected bootstrap with initialized app")
	}
}

func TestNewWithContainerRequiresRuntime(t *testing.T) {
	_, err := newWithContainer(&internalapp.Container{})
	if err == nil {
		t.Fatal("expected error for nil runtime in container")
	}
}

func TestBootstrapStartStopWaitLifecycle(t *testing.T) {
	var started atomic.Bool

	var stopped atomic.Bool

	waitDone := make(chan struct{})

	rt := &fakeRuntime{
		startFn: func() error {
			started.Store(true)

			return nil
		},
		stopFn: func() {
			stopped.Store(true)
			close(waitDone)
		},
		waitFn: func() error {
			<-waitDone

			return nil
		},
	}

	b, err := newWithContainer(&internalapp.Container{Runtime: rt})
	if err != nil {
		t.Fatalf("newWithContainer error: %v", err)
	}

	if err := b.Start(context.Background()); err != nil {
		t.Fatalf("start error: %v", err)
	}

	if !started.Load() {
		t.Fatal("runtime was not started")
	}

	waitErr := make(chan error, 1)
	go func() {
		waitErr <- b.Wait()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := b.Stop(ctx); err != nil {
		t.Fatalf("stop error: %v", err)
	}

	if !stopped.Load() {
		t.Fatal("runtime was not stopped")
	}

	select {
	case err := <-waitErr:
		if err != nil {
			t.Fatalf("wait error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("wait did not return after stop")
	}
}

func TestBootstrapStartWrapsRuntimeError(t *testing.T) {
	rtErr := errors.New("boom")

	b, err := newWithContainer(&internalapp.Container{
		Runtime: &fakeRuntime{
			startFn: func() error { return rtErr },
		},
	})
	if err != nil {
		t.Fatalf("newWithContainer error: %v", err)
	}

	err = b.Start(context.Background())
	if err == nil || !errors.Is(err, rtErr) {
		t.Fatalf("expected wrapped runtime error, got %v", err)
	}
}

func TestBootstrapLifecycleRequiresInitializedValue(t *testing.T) {
	var b *Bootstrap
	if err := b.Start(context.Background()); err == nil {
		t.Fatal("expected start error for nil bootstrap")
	}

	if err := b.Stop(context.Background()); err == nil {
		t.Fatal("expected stop error for nil bootstrap")
	}

	if err := b.Wait(); err == nil {
		t.Fatal("expected wait error for nil bootstrap")
	}
}

func TestBootstrapStopWrapsRuntimeError(t *testing.T) {
	blockStop := make(chan struct{})
	rt := &fakeRuntime{
		startFn: func() error { return nil },
		stopFn:  func() { <-blockStop },
		waitFn:  func() error { return nil },
	}

	b, err := newWithContainer(&internalapp.Container{Runtime: rt})
	if err != nil {
		t.Fatalf("newWithContainer error: %v", err)
	}

	if err := b.Start(context.Background()); err != nil {
		t.Fatalf("start error: %v", err)
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err = b.Stop(stopCtx)
	if err == nil {
		t.Fatal("expected stop timeout error")
	}

	close(blockStop)
}
