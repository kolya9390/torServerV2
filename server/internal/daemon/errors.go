package daemon

import (
	"errors"
	"fmt"
)

// Stage identifies the daemon lifecycle phase that failed.
type Stage string

const (
	StageArguments Stage = "parse arguments"
	StageConfig    Stage = "load config"
	StageBootstrap Stage = "initialize daemon"
	StageStart     Stage = "start daemon"
	StageWait      Stage = "wait for daemon"
	StageStop      Stage = "stop daemon"
)

// StageError preserves a machine-classifiable lifecycle phase and root cause.
type StageError struct {
	Stage Stage
	Err   error
}

type userMessageProvider interface {
	UserMessage() string
}

// UserMessage returns a safe process-facing error without discarding the
// wrapped error used for programmatic classification.
func UserMessage(err error) string {
	if err == nil {
		return ""
	}

	var provider userMessageProvider
	if errors.As(err, &provider) {
		return provider.UserMessage()
	}

	return err.Error()
}

func (err *StageError) Error() string {
	if err == nil {
		return "daemon lifecycle error"
	}

	return fmt.Sprintf("%s: %v", err.Stage, err.Err)
}

func (err *StageError) Unwrap() error {
	if err == nil {
		return nil
	}

	return err.Err
}

// RuntimePanicError reports a panic recovered from Lifecycle.Wait.
type RuntimePanicError struct {
	Value any
}

func (err *RuntimePanicError) Error() string {
	if err == nil {
		return "daemon runtime panic"
	}

	return fmt.Sprintf("daemon runtime panic: %v", err.Value)
}

func (err *RuntimePanicError) Unwrap() error {
	if err == nil {
		return nil
	}

	cause, ok := err.Value.(error)
	if !ok {
		return nil
	}

	return cause
}
