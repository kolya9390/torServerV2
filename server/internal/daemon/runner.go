package daemon

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"server/config"
	"server/settings"
)

const DefaultStopTimeout = 30 * time.Second

const (
	ExitOK      = 0
	ExitFailure = 1
	ExitUsage   = 2
)

var (
	errHelpRequested    = errors.New("daemon help requested")
	errVersionRequested = errors.New("daemon version requested")
)

// Lifecycle is the daemon runtime surface owned by the runner.
type Lifecycle interface {
	Start(context.Context) error
	Wait() error
	Stop(context.Context) error
}

// Logger owns the process logger for the entire initialized daemon run.
type Logger interface {
	Init(logPath, webLogPath string)
	Info(args ...any)
	Close()
}

// Invocation contains process inputs for one daemon run.
type Invocation struct {
	Context context.Context
	Args    []string
	Stdout  io.Writer
}

// Result is returned to a thin process entry point for final exit mapping.
type Result struct {
	ExitCode int
	Err      error
}

// Dependencies contains replaceable process and lifecycle adapters.
type Dependencies struct {
	Getenv           func(string) string
	ParseArgs        func([]string, io.Writer) (*settings.ExecArgs, error)
	LoadConfig       func(string) (*config.Config, error)
	SetStaticConfig  func(settings.StaticConfig)
	NewLifecycle     func(*settings.ExecArgs, *config.Config) (Lifecycle, error)
	Logger           Logger
	SubscribeSignals func() (<-chan os.Signal, func())
	StopTimeout      time.Duration
}

type runtimeDeps struct {
	getenv           func(string) string
	parseArgs        func([]string, io.Writer) (*settings.ExecArgs, error)
	loadConfig       func(string) (*config.Config, error)
	setStaticConfig  func(settings.StaticConfig)
	newLifecycle     func(*settings.ExecArgs, *config.Config) (Lifecycle, error)
	logger           Logger
	subscribeSignals func() (<-chan os.Signal, func())
	stopTimeout      time.Duration
}

// Run owns daemon initialization, supervision, and graceful shutdown. It never
// calls os.Exit and does not write an error that the caller may report again.
func Run(invocation Invocation, dependencies Dependencies) Result {
	deps := newRuntimeDeps(dependencies)
	ctx := contextOrBackground(invocation.Context)
	args, err := deps.parseArgs(append([]string(nil), invocation.Args...), writerOrDiscard(invocation.Stdout))
	if errors.Is(err, errHelpRequested) || errors.Is(err, errVersionRequested) {
		return Result{ExitCode: ExitOK}
	}

	if err != nil {
		return failure(ExitUsage, StageArguments, err)
	}

	deps.logger.Init(args.LogPath, args.WebLogPath)
	defer deps.logger.Close()

	cfg, err := loadRuntimeConfig(args, deps)
	if err != nil {
		return failure(ExitFailure, StageConfig, err)
	}

	lifecycle, err := deps.newLifecycle(args, cfg)
	if err != nil {
		return failure(ExitFailure, StageBootstrap, err)
	}

	if err := lifecycle.Start(ctx); err != nil {
		return failure(ExitFailure, StageStart, err)
	}

	return supervise(ctx, lifecycle, deps)
}

func loadRuntimeConfig(args *settings.ExecArgs, deps runtimeDeps) (*config.Config, error) {
	if args == nil {
		return nil, errors.New("arguments are not initialized")
	}

	cfg, err := deps.loadConfig(deps.getenv("TS_CONFIG"))
	if err != nil {
		return nil, err
	}

	if cfg == nil {
		return nil, errors.New("config loader returned nil config")
	}

	deps.setStaticConfig(cfg.ToStaticConfig())
	applyArgumentOverrides(args, cfg)

	return cfg, nil
}

func applyArgumentOverrides(args *settings.ExecArgs, cfg *config.Config) {
	if args.Ssl {
		cfg.Server.SSL = true
	}

	if args.SslCert != "" {
		cfg.Server.SSLCert = args.SslCert
		cfg.Server.SSLKey = args.SslKey
	}
}

func supervise(ctx context.Context, lifecycle Lifecycle, deps runtimeDeps) Result {
	waitResult := waitForLifecycle(lifecycle)
	signals, unsubscribe := deps.subscribeSignals()
	if unsubscribe != nil {
		defer unsubscribe()
	}

	select {
	case err := <-waitResult:
		if err == nil {
			deps.logger.Info("Runtime exited")

			return Result{ExitCode: ExitOK}
		}

		return stopAfterRuntimeFailure(ctx, lifecycle, deps, err)
	case signal := <-signals:
		if signal != nil {
			deps.logger.Info("Received signal: ", signal.String())
		}

		return stopLifecycle(ctx, lifecycle, deps)
	case <-ctx.Done():
		deps.logger.Info("Daemon context canceled")

		return stopLifecycle(ctx, lifecycle, deps)
	}
}

func waitForLifecycle(lifecycle Lifecycle) <-chan error {
	result := make(chan error, 1)

	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				result <- &RuntimePanicError{Value: recovered}
			}
		}()

		result <- lifecycle.Wait()
	}()

	return result
}

func stopAfterRuntimeFailure(
	ctx context.Context,
	lifecycle Lifecycle,
	deps runtimeDeps,
	waitErr error,
) Result {
	stopResult := stopLifecycle(ctx, lifecycle, deps)
	waitStageErr := &StageError{Stage: StageWait, Err: waitErr}
	if stopResult.Err != nil {
		return Result{ExitCode: ExitFailure, Err: errors.Join(waitStageErr, stopResult.Err)}
	}

	return Result{ExitCode: ExitFailure, Err: waitStageErr}
}

func stopLifecycle(ctx context.Context, lifecycle Lifecycle, deps runtimeDeps) Result {
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), deps.stopTimeout)
	defer cancel()

	if err := lifecycle.Stop(stopCtx); err != nil {
		return failure(ExitFailure, StageStop, err)
	}

	return Result{ExitCode: ExitOK}
}

func failure(exitCode int, stage Stage, err error) Result {
	return Result{
		ExitCode: exitCode,
		Err:      &StageError{Stage: stage, Err: err},
	}
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}

	return context.Background()
}

func writerOrDiscard(writer io.Writer) io.Writer {
	if writer != nil {
		return writer
	}

	return io.Discard
}
