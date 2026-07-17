package daemon

import (
	"context"
	"errors"
	"io"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"server/config"
	"server/settings"
)

func TestRunConfigFailureStopsInitializationAndClosesLogger(t *testing.T) {
	t.Parallel()

	configErr := errors.New("invalid config")
	lifecycle := &fakeLifecycle{}
	deps, logger := testRuntimeDependencies(lifecycle)
	deps.LoadConfig = func(string) (*config.Config, error) { return nil, configErr }

	var lifecycleCalls atomic.Int32
	deps.NewLifecycle = func(*settings.ExecArgs, *config.Config) (Lifecycle, error) {
		lifecycleCalls.Add(1)

		return lifecycle, nil
	}

	result := Run(Invocation{}, deps)
	assertStageError(t, result, ExitFailure, StageConfig, configErr)
	if lifecycleCalls.Load() != 0 {
		t.Fatalf("lifecycle factory calls = %d, want 0", lifecycleCalls.Load())
	}

	logger.assertOwnedOnce(t)
	if logger.infoCalls.Load() != 0 {
		t.Fatalf("failure was unexpectedly logged %d times", logger.infoCalls.Load())
	}
}

func TestRunBootstrapFailure(t *testing.T) {
	t.Parallel()

	bootstrapErr := errors.New("bootstrap failed")
	deps, logger := testRuntimeDependencies(&fakeLifecycle{})
	deps.NewLifecycle = func(*settings.ExecArgs, *config.Config) (Lifecycle, error) {
		return nil, bootstrapErr
	}

	result := Run(Invocation{}, deps)
	assertStageError(t, result, ExitFailure, StageBootstrap, bootstrapErr)
	logger.assertOwnedOnce(t)
}

func TestRunStartFailureDoesNotSubscribeOrStop(t *testing.T) {
	t.Parallel()

	startErr := errors.New("start failed")
	lifecycle := &fakeLifecycle{startErr: startErr}
	deps, logger := testRuntimeDependencies(lifecycle)

	var subscriptions atomic.Int32
	deps.SubscribeSignals = func() (<-chan os.Signal, func()) {
		subscriptions.Add(1)

		return make(chan os.Signal), func() {}
	}

	result := Run(Invocation{}, deps)
	assertStageError(t, result, ExitFailure, StageStart, startErr)
	if lifecycle.stopCalls.Load() != 0 {
		t.Fatalf("stop calls = %d, want 0", lifecycle.stopCalls.Load())
	}

	if subscriptions.Load() != 0 {
		t.Fatalf("signal subscriptions = %d, want 0", subscriptions.Load())
	}

	logger.assertOwnedOnce(t)
}

func TestRunNormalRuntimeExitDoesNotStopLifecycle(t *testing.T) {
	t.Parallel()

	lifecycle := &fakeLifecycle{}
	deps, logger := testRuntimeDependencies(lifecycle)

	var unsubscribed atomic.Int32
	deps.SubscribeSignals = func() (<-chan os.Signal, func()) {
		return make(chan os.Signal), func() { unsubscribed.Add(1) }
	}

	result := Run(Invocation{}, deps)
	if result.ExitCode != ExitOK || result.Err != nil {
		t.Fatalf("result = %+v", result)
	}

	if lifecycle.stopCalls.Load() != 0 {
		t.Fatalf("stop calls = %d, want 0", lifecycle.stopCalls.Load())
	}

	if unsubscribed.Load() != 1 {
		t.Fatalf("unsubscribe calls = %d, want 1", unsubscribed.Load())
	}

	logger.assertOwnedOnce(t)
}

func TestRunSignalPerformsOneGracefulStop(t *testing.T) {
	t.Parallel()

	waitDone := make(chan struct{})
	lifecycle := &fakeLifecycle{
		waitFn: func() error {
			<-waitDone

			return nil
		},
		stopFn: func(context.Context) error {
			close(waitDone)

			return nil
		},
	}
	deps, logger := testRuntimeDependencies(lifecycle)
	signals := make(chan os.Signal, 1)
	signals <- os.Interrupt
	deps.SubscribeSignals = func() (<-chan os.Signal, func()) { return signals, func() {} }

	result := Run(Invocation{}, deps)
	if result.ExitCode != ExitOK || result.Err != nil {
		t.Fatalf("result = %+v", result)
	}

	if lifecycle.stopCalls.Load() != 1 {
		t.Fatalf("stop calls = %d, want 1", lifecycle.stopCalls.Load())
	}

	if logger.infoCalls.Load() != 1 {
		t.Fatalf("signal info logs = %d, want 1", logger.infoCalls.Load())
	}

	logger.assertOwnedOnce(t)
}

func TestRunStopTimeoutIsReturned(t *testing.T) {
	t.Parallel()

	waitDone := make(chan struct{})
	lifecycle := &fakeLifecycle{
		waitFn: func() error {
			<-waitDone

			return nil
		},
		stopFn: func(ctx context.Context) error {
			<-ctx.Done()
			close(waitDone)

			return ctx.Err()
		},
	}
	deps, logger := testRuntimeDependencies(lifecycle)
	deps.StopTimeout = 10 * time.Millisecond
	signals := make(chan os.Signal, 1)
	signals <- os.Interrupt
	deps.SubscribeSignals = func() (<-chan os.Signal, func()) { return signals, func() {} }

	result := Run(Invocation{}, deps)
	assertStageError(t, result, ExitFailure, StageStop, context.DeadlineExceeded)
	logger.assertOwnedOnce(t)
}

func TestRunRecoversWaitPanicAndStopsLifecycle(t *testing.T) {
	t.Parallel()

	panicErr := errors.New("runtime panic")
	lifecycle := &fakeLifecycle{
		waitFn: func() error { panic(panicErr) },
	}
	deps, logger := testRuntimeDependencies(lifecycle)

	result := Run(Invocation{}, deps)
	assertStageError(t, result, ExitFailure, StageWait, panicErr)

	var recovered *RuntimePanicError
	if !errors.As(result.Err, &recovered) || recovered.Value != panicErr {
		t.Fatalf("runtime panic error = %#v", result.Err)
	}

	if lifecycle.stopCalls.Load() != 1 {
		t.Fatalf("stop calls = %d, want 1", lifecycle.stopCalls.Load())
	}

	logger.assertOwnedOnce(t)
}

func TestRunPreservesConfigPrecedenceAndArgumentOverrides(t *testing.T) {
	t.Parallel()

	args := &settings.ExecArgs{Ssl: true, SslCert: "server.crt", SslKey: "server.key"}
	lifecycle := &fakeLifecycle{}
	deps, _ := testRuntimeDependencies(lifecycle)
	deps.ParseArgs = func([]string, io.Writer) (*settings.ExecArgs, error) { return args, nil }
	deps.Getenv = func(name string) string {
		if name == "TS_CONFIG" {
			return "/explicit/config.yml"
		}

		return ""
	}

	var loadedPath string
	loadedConfig := &config.Config{}
	deps.LoadConfig = func(path string) (*config.Config, error) {
		loadedPath = path

		return loadedConfig, nil
	}

	var staticConfigCalls atomic.Int32
	deps.SetStaticConfig = func(settings.StaticConfig) { staticConfigCalls.Add(1) }
	deps.NewLifecycle = func(_ *settings.ExecArgs, cfg *config.Config) (Lifecycle, error) {
		if cfg != loadedConfig {
			t.Fatalf("lifecycle config = %p, want %p", cfg, loadedConfig)
		}

		return lifecycle, nil
	}

	result := Run(Invocation{}, deps)
	if result.ExitCode != ExitOK || result.Err != nil {
		t.Fatalf("result = %+v", result)
	}

	if loadedPath != "/explicit/config.yml" {
		t.Fatalf("config path = %q", loadedPath)
	}

	if !loadedConfig.Server.SSL || loadedConfig.Server.SSLCert != "server.crt" || loadedConfig.Server.SSLKey != "server.key" {
		t.Fatalf("argument overrides not applied: %+v", loadedConfig.Server)
	}

	if staticConfigCalls.Load() != 1 {
		t.Fatalf("static config calls = %d, want 1", staticConfigCalls.Load())
	}
}

func TestRunArgumentFailureUsesUsageExitWithoutLogger(t *testing.T) {
	t.Parallel()

	parseErr := errors.New("bad arguments")
	deps, logger := testRuntimeDependencies(&fakeLifecycle{})
	deps.ParseArgs = func([]string, io.Writer) (*settings.ExecArgs, error) { return nil, parseErr }

	result := Run(Invocation{}, deps)
	assertStageError(t, result, ExitUsage, StageArguments, parseErr)
	if logger.initCalls.Load() != 0 || logger.closeCalls.Load() != 0 {
		t.Fatalf("logger calls = init:%d close:%d", logger.initCalls.Load(), logger.closeCalls.Load())
	}
}

func TestRunVersionRequestExitsSuccessfullyWithoutSideEffects(t *testing.T) {
	t.Parallel()

	deps, logger := testRuntimeDependencies(&fakeLifecycle{})
	deps.ParseArgs = func([]string, io.Writer) (*settings.ExecArgs, error) {
		return nil, errVersionRequested
	}

	result := Run(Invocation{}, deps)
	if result.ExitCode != ExitOK || result.Err != nil {
		t.Fatalf("result = %+v", result)
	}

	if logger.initCalls.Load() != 0 || logger.closeCalls.Load() != 0 {
		t.Fatalf("logger calls = init:%d close:%d", logger.initCalls.Load(), logger.closeCalls.Load())
	}
}

func TestRunPreflightFailuresHaveNoDaemonSideEffects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "former management command", args: []string{"shutdown"}},
		{name: "unknown argument", args: []string{"--unknown=private"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			deps, logger := testRuntimeDependencies(&fakeLifecycle{})
			deps.ParseArgs = DefaultDependencies().ParseArgs

			var configCalls, lifecycleCalls, signalCalls atomic.Int32
			deps.LoadConfig = func(string) (*config.Config, error) {
				configCalls.Add(1)

				return &config.Config{}, nil
			}
			deps.NewLifecycle = func(*settings.ExecArgs, *config.Config) (Lifecycle, error) {
				lifecycleCalls.Add(1)

				return &fakeLifecycle{}, nil
			}
			deps.SubscribeSignals = func() (<-chan os.Signal, func()) {
				signalCalls.Add(1)

				return make(chan os.Signal), func() {}
			}

			result := Run(Invocation{Args: test.args}, deps)
			if result.ExitCode != ExitUsage || result.Err == nil {
				t.Fatalf("result = %+v", result)
			}

			if logger.initCalls.Load() != 0 || logger.closeCalls.Load() != 0 || configCalls.Load() != 0 ||
				lifecycleCalls.Load() != 0 || signalCalls.Load() != 0 {
				t.Fatalf(
					"side effects = logger:%d/%d config:%d lifecycle:%d signals:%d",
					logger.initCalls.Load(),
					logger.closeCalls.Load(),
					configCalls.Load(),
					lifecycleCalls.Load(),
					signalCalls.Load(),
				)
			}
		})
	}
}

func testRuntimeDependencies(lifecycle Lifecycle) (Dependencies, *fakeLogger) {
	logger := &fakeLogger{}
	signals := make(chan os.Signal)

	return Dependencies{
		Getenv: func(string) string { return "" },
		ParseArgs: func([]string, io.Writer) (*settings.ExecArgs, error) {
			return &settings.ExecArgs{}, nil
		},
		LoadConfig:      func(string) (*config.Config, error) { return &config.Config{}, nil },
		SetStaticConfig: func(settings.StaticConfig) {},
		NewLifecycle: func(*settings.ExecArgs, *config.Config) (Lifecycle, error) {
			return lifecycle, nil
		},
		Logger:           logger,
		SubscribeSignals: func() (<-chan os.Signal, func()) { return signals, func() {} },
		StopTimeout:      time.Second,
	}, logger
}

func assertStageError(t *testing.T, result Result, exitCode int, stage Stage, cause error) {
	t.Helper()

	if result.ExitCode != exitCode {
		t.Fatalf("exit code = %d, want %d", result.ExitCode, exitCode)
	}

	var stageErr *StageError
	if !errors.As(result.Err, &stageErr) || stageErr.Stage != stage {
		t.Fatalf("stage error = %#v, want stage %q", result.Err, stage)
	}

	if !errors.Is(result.Err, cause) {
		t.Fatalf("error = %v, want cause %v", result.Err, cause)
	}
}

type fakeLifecycle struct {
	startErr error
	waitFn   func() error
	stopFn   func(context.Context) error

	startCalls atomic.Int32
	stopCalls  atomic.Int32
}

func (lifecycle *fakeLifecycle) Start(context.Context) error {
	lifecycle.startCalls.Add(1)

	return lifecycle.startErr
}

func (lifecycle *fakeLifecycle) Wait() error {
	if lifecycle.waitFn != nil {
		return lifecycle.waitFn()
	}

	return nil
}

func (lifecycle *fakeLifecycle) Stop(ctx context.Context) error {
	lifecycle.stopCalls.Add(1)
	if lifecycle.stopFn != nil {
		return lifecycle.stopFn(ctx)
	}

	return nil
}

type fakeLogger struct {
	initCalls  atomic.Int32
	infoCalls  atomic.Int32
	closeCalls atomic.Int32
}

func (logger *fakeLogger) Init(string, string) {
	logger.initCalls.Add(1)
}

func (logger *fakeLogger) Info(...any) {
	logger.infoCalls.Add(1)
}

func (logger *fakeLogger) Close() {
	logger.closeCalls.Add(1)
}

func (logger *fakeLogger) assertOwnedOnce(t *testing.T) {
	t.Helper()

	if logger.initCalls.Load() != 1 || logger.closeCalls.Load() != 1 {
		t.Fatalf("logger ownership = init:%d close:%d, want 1/1", logger.initCalls.Load(), logger.closeCalls.Load())
	}
}
