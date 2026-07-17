package daemon

import (
	"errors"
	"testing"
)

func TestUserMessageUsesSafeNestedPresentation(t *testing.T) {
	t.Parallel()

	migrationErr := &managementCommandError{Command: "shutdown", Replacement: "torrctl shutdown"}
	err := &StageError{Stage: StageArguments, Err: migrationErr}

	const want = "torrserver shutdown: management commands moved to torrctl; use `torrctl shutdown`"
	if got := UserMessage(err); got != want {
		t.Fatalf("user message = %q, want %q", got, want)
	}

	if !errors.Is(err, migrationErr) {
		t.Fatal("safe presentation discarded wrapped classification")
	}
}

func TestUserMessagePreservesOrdinaryLifecycleError(t *testing.T) {
	t.Parallel()

	err := &StageError{Stage: StageConfig, Err: errors.New("invalid configuration")}
	if got := UserMessage(err); got != "load config: invalid configuration" {
		t.Fatalf("user message = %q", got)
	}
}
