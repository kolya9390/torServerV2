package modules

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestStartWithPolicyRetriesUntilSuccess(t *testing.T) {
	attempts := 0
	err := startWithPolicy("test-module", func() error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary")
		}
		return nil
	}, RestartPolicy{
		MaxAttempts:    3,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     2 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("startWithPolicy returned error: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestStartWithPolicyFailsAfterMaxAttempts(t *testing.T) {
	expected := errors.New("startup failure")
	err := startWithPolicy("dlna", func() error {
		return expected
	}, RestartPolicy{
		MaxAttempts:    2,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, expected) {
		t.Fatalf("expected wrapped startup error, got %v", err)
	}
	if !strings.Contains(err.Error(), "dlna failed after 2 attempts") {
		t.Fatalf("unexpected error text: %v", err)
	}
}

func TestSafeStopRecoversPanic(t *testing.T) {
	safeStop("panic-stop", func() {
		panic("boom")
	})
}
