package modules

import (
	"fmt"
	"time"

	"server/log"
)

type RestartPolicy struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

func DefaultPolicy() RestartPolicy {
	return RestartPolicy{
		MaxAttempts:    3,
		InitialBackoff: 500 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
	}
}

func startWithPolicy(name string, start func() error, policy RestartPolicy) error {
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 1
	}

	if policy.InitialBackoff <= 0 {
		policy.InitialBackoff = 100 * time.Millisecond
	}

	if policy.MaxBackoff <= 0 {
		policy.MaxBackoff = policy.InitialBackoff
	}

	backoff := policy.InitialBackoff

	var lastErr error

	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		if err := start(); err == nil {
			if attempt > 1 {
				log.TLogln("module started after retry", name, "attempt", attempt)
			}

			return nil
		} else {
			lastErr = err
			log.TLogln("module start failed", name, "attempt", attempt, "error", err)
		}

		if attempt < policy.MaxAttempts {
			time.Sleep(backoff)

			backoff *= 2
			if backoff > policy.MaxBackoff {
				backoff = policy.MaxBackoff
			}
		}
	}

	return fmt.Errorf("%s failed after %d attempts: %w", name, policy.MaxAttempts, lastErr)
}

func StartWithPolicy(name string, start func() error, policy RestartPolicy) error {
	return startWithPolicy(name, start, policy)
}

func safeStop(name string, stop func()) {
	defer func() {
		if r := recover(); r != nil {
			log.TLogln("module stop panic recovered", name, r)
		}
	}()
	stop()
}
