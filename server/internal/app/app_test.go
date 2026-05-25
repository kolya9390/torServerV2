package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
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

func TestStartHonorsCanceledContext(t *testing.T) {
	rt := &fakeRuntime{}
	a := New(rt, time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := a.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestStopTimeout(t *testing.T) {
	rt := &fakeRuntime{
		stopFn: func() { time.Sleep(100 * time.Millisecond) },
	}
	a := New(rt, 10*time.Millisecond)

	err := a.Stop(context.Background())
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestStopUsesProvidedDeadline(t *testing.T) {
	stopped := make(chan struct{})
	rt := &fakeRuntime{
		stopFn: func() { close(stopped) },
	}
	a := New(rt, time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	if err := a.Stop(ctx); err != nil {
		t.Fatalf("unexpected stop error: %v", err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("runtime stop was not called")
	}
}

func TestStopReturnsRuntimePanic(t *testing.T) {
	rt := &fakeRuntime{
		stopFn: func() { panic("boom") },
	}
	a := New(rt, time.Second)

	start := time.Now()
	err := a.Stop(context.Background())
	if err == nil {
		t.Fatal("expected panic error")
	}

	if !strings.Contains(err.Error(), "runtime stop panic: boom") {
		t.Fatalf("expected runtime panic error, got %v", err)
	}

	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("expected panic to return without waiting for timeout, elapsed=%s", elapsed)
	}
}
