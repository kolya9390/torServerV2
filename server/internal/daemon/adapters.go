package daemon

import (
	"io"
	"os"
	"os/signal"
	"syscall"

	"server/bootstrap"
	"server/config"
	"server/log"
	"server/settings"
	buildversion "server/version"
)

type processLogger struct{}

// DefaultDependencies returns production adapters without starting the daemon
// or registering signal handlers.
func DefaultDependencies() Dependencies {
	buildInfo := buildversion.Current()

	return Dependencies{
		Getenv: os.Getenv,
		ParseArgs: func(args []string, output io.Writer) (*settings.ExecArgs, error) {
			return parseProcessArgs(args, output, buildInfo)
		},
		LoadConfig:       config.Load,
		SetStaticConfig:  settings.SetStaticConfig,
		NewLifecycle:     newBootstrapLifecycle,
		Logger:           processLogger{},
		SubscribeSignals: subscribeProcessSignals,
		StopTimeout:      DefaultStopTimeout,
	}
}

func newRuntimeDeps(dependencies Dependencies) runtimeDeps {
	defaults := DefaultDependencies()

	if dependencies.Getenv == nil {
		dependencies.Getenv = defaults.Getenv
	}
	if dependencies.ParseArgs == nil {
		dependencies.ParseArgs = defaults.ParseArgs
	}
	if dependencies.LoadConfig == nil {
		dependencies.LoadConfig = defaults.LoadConfig
	}
	if dependencies.SetStaticConfig == nil {
		dependencies.SetStaticConfig = defaults.SetStaticConfig
	}
	if dependencies.NewLifecycle == nil {
		dependencies.NewLifecycle = defaults.NewLifecycle
	}
	if dependencies.Logger == nil {
		dependencies.Logger = defaults.Logger
	}
	if dependencies.SubscribeSignals == nil {
		dependencies.SubscribeSignals = defaults.SubscribeSignals
	}
	if dependencies.StopTimeout <= 0 {
		dependencies.StopTimeout = defaults.StopTimeout
	}

	return runtimeDeps{
		getenv:           dependencies.Getenv,
		parseArgs:        dependencies.ParseArgs,
		loadConfig:       dependencies.LoadConfig,
		setStaticConfig:  dependencies.SetStaticConfig,
		newLifecycle:     dependencies.NewLifecycle,
		logger:           dependencies.Logger,
		subscribeSignals: dependencies.SubscribeSignals,
		stopTimeout:      dependencies.StopTimeout,
	}
}

func newBootstrapLifecycle(args *settings.ExecArgs, cfg *config.Config) (Lifecycle, error) {
	return bootstrap.New(args, cfg)
}

func subscribeProcessSignals() (<-chan os.Signal, func()) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	return signals, func() { signal.Stop(signals) }
}

func (processLogger) Init(logPath, webLogPath string) {
	log.Init(logPath, webLogPath)
}

func (processLogger) Info(args ...any) {
	log.TLogln(args...)
}

func (processLogger) Close() {
	log.Close()
}
