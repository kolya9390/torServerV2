package retry

import (
	"context"
	"fmt"
	"time"
)

type Config struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
}

var DefaultConfig = Config{
	MaxAttempts:  3,
	InitialDelay: time.Second,
	MaxDelay:     10 * time.Second,
	Multiplier:   2.0,
}

type RetryableError interface {
	Temporary() bool
}

type Result[T any] struct {
	Value T
	Err   error
}

func Do[T any](ctx context.Context, cfg Config, fn func() (T, error)) Result[T] {
	var lastErr error

	delay := cfg.InitialDelay

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return Result[T]{Err: fmt.Errorf("retry cancelled: %w", ctx.Err())}
		default:
		}

		value, err := fn()
		if err == nil {
			return Result[T]{Value: value}
		}

		lastErr = err

		if re, ok := err.(RetryableError); ok && !re.Temporary() {
			return Result[T]{Err: err}
		}

		if attempt >= cfg.MaxAttempts {
			break
		}

		select {
		case <-ctx.Done():
			return Result[T]{Err: fmt.Errorf("retry cancelled: %w", ctx.Err())}
		case <-time.After(delay):
		}

		delay = min(time.Duration(float64(delay)*cfg.Multiplier), cfg.MaxDelay)
	}

	return Result[T]{Err: fmt.Errorf("max attempts (%d) exceeded: %w", cfg.MaxAttempts, lastErr)}
}

func DoNoResult(ctx context.Context, cfg Config, fn func() error) error {
	result := Do[any](ctx, cfg, func() (any, error) {
		return nil, fn()
	})

	return result.Err
}
