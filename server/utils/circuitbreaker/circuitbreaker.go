package circuitbreaker

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var ErrCircuitOpen = errors.New("circuit breaker is open")

type State int32

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

type Config struct {
	FailureThreshold int
	SuccessThreshold int
	Timeout          time.Duration
	HalfOpenMaxCalls int
	ReadyToTrip      func(failures int32, totalCalls int32) bool
	OnStateChange    func(oldState, newState State)
	OnRequestBlocked func()
}

var DefaultConfig = Config{
	FailureThreshold: 5,
	SuccessThreshold: 3,
	Timeout:          30 * time.Second,
	HalfOpenMaxCalls: 3,
	ReadyToTrip:      nil,
	OnStateChange:    nil,
	OnRequestBlocked: nil,
}

type CircuitBreaker struct {
	name          string
	config        Config
	state         atomic.Int32
	failures      atomic.Int32
	successes     atomic.Int32
	totalCalls    atomic.Int32
	lastAttempt   atomic.Int64
	halfOpenCalls atomic.Int32
	mu            sync.Mutex
	openTime      time.Time
}

func New(name string, config Config) *CircuitBreaker {
	cb := &CircuitBreaker{
		name:   name,
		config: config,
	}
	if config.ReadyToTrip == nil {
		cb.config.ReadyToTrip = defaultReadyToTrip
	}

	return cb
}

func defaultReadyToTrip(failures int32, totalCalls int32) bool {
	return failures >= 5
}

func (cb *CircuitBreaker) State() State {
	return State(cb.state.Load())
}

func (cb *CircuitBreaker) Allow() error {
	state := cb.State()

	switch state {
	case StateClosed:
		return nil
	case StateOpen:
		if cb.shouldAllowHalfOpen() {
			cb.state.Store(int32(StateHalfOpen))
			cb.halfOpenCalls.Store(0)
			cb.recordStateChange(StateOpen, StateHalfOpen)

			return nil
		}

		if cb.config.OnRequestBlocked != nil {
			cb.config.OnRequestBlocked()
		}

		return ErrCircuitOpen
	case StateHalfOpen:
		if cb.halfOpenCalls.Load() >= int32(cb.config.HalfOpenMaxCalls) {
			return ErrCircuitOpen
		}

		cb.halfOpenCalls.Add(1)

		return nil
	}

	return nil
}

func (cb *CircuitBreaker) shouldAllowHalfOpen() bool {
	if cb.config.Timeout <= 0 {
		return true
	}

	elapsed := time.Since(cb.openTime)

	return elapsed >= cb.config.Timeout
}

func (cb *CircuitBreaker) recordSuccess() {
	cb.failures.Store(0)
	cb.totalCalls.Add(1)

	switch cb.State() {
	case StateHalfOpen:
		cb.successes.Add(1)

		if cb.successes.Load() >= int32(cb.config.SuccessThreshold) {
			cb.state.Store(int32(StateClosed))
			cb.successes.Store(0)
			cb.halfOpenCalls.Store(0)
			cb.recordStateChange(StateHalfOpen, StateClosed)
		}
	case StateClosed:
		// Reset failures on success
	}
}

func (cb *CircuitBreaker) recordFailure() {
	cb.failures.Add(1)
	cb.totalCalls.Add(1)
	cb.lastAttempt.Store(time.Now().Unix())

	switch cb.State() {
	case StateHalfOpen:
		cb.state.Store(int32(StateOpen))
		cb.openTime = time.Now()
		cb.successes.Store(0)
		cb.halfOpenCalls.Store(0)
		cb.recordStateChange(StateHalfOpen, StateOpen)
	case StateClosed:
		if cb.config.ReadyToTrip(cb.failures.Load(), cb.totalCalls.Load()) {
			cb.state.Store(int32(StateOpen))
			cb.openTime = time.Now()
			cb.recordStateChange(StateClosed, StateOpen)
		}
	}
}

func (cb *CircuitBreaker) recordStateChange(oldState, newState State) {
	if cb.config.OnStateChange != nil {
		cb.config.OnStateChange(oldState, newState)
	}
}

func (cb *CircuitBreaker) Execute(fn func() error) error {
	if err := cb.Allow(); err != nil {
		return err
	}

	err := fn()

	if err != nil {
		cb.recordFailure()
	} else {
		cb.recordSuccess()
	}

	return err
}

func (cb *CircuitBreaker) Metrics() map[string]any {
	return map[string]any{
		"name":         cb.name,
		"state":        cb.State().String(),
		"failures":     cb.failures.Load(),
		"successes":    cb.successes.Load(),
		"total_calls":  cb.totalCalls.Load(),
		"last_attempt": time.Unix(cb.lastAttempt.Load(), 0).Unix(),
		"half_open":    cb.halfOpenCalls.Load(),
	}
}
