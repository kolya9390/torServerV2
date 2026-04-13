// @title TorrServer API
// @version 1.0
// @description Minimalist torrent streaming server API.
// @host localhost:8090
// @BasePath /
// @schemes http

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	_ "server/docs"

	"github.com/alexflint/go-arg"

	"server/bootstrap"
	"server/cmd/cli"
	"server/config"
	"server/log"
	"server/settings"
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Determine mode: CLI or Server
	// If args exist and the first one does NOT start with '-', it's a CLI command.
	// If no args or starts with '-', it's Server mode (pass flags to go-arg).
	isCli := len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-")

	if isCli {
		cli.Execute()

		return
	}

	runServer()
}

func runServer() {
	args, err := parseArgs(os.Args[1:])

	if err != nil {
		fmt.Println(err)
		os.Exit(2)
	}

	settings.SetArgs(args)

	log.Init(args.LogPath, args.WebLogPath)
	defer log.Close()

	cfg, err := loadConfig(args)

	if err != nil {
		log.TLogln("Failed to load config:", err)
	}

	app, err := bootstrap.New(args, cfg)

	if err != nil {
		log.TLogln("Failed to initialize:", err)

		return
	}

	if err := app.Start(context.Background()); err != nil {
		log.TLogln("Failed to start:", err)

		return
	}

	waitErr := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.TLogln("main wait goroutine panic recovered", "panic", r)
				waitErr <- fmt.Errorf("panic: %v", r)
			}
		}()
		waitErr <- app.Wait()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	shouldStop := true

	select {
	case sig := <-quit:
		log.TLogln("Received signal:", sig.String())
	case err := <-waitErr:
		if err != nil {
			log.TLogln("Runtime exited with error:", err)
		} else {
			log.TLogln("Runtime exited")

			shouldStop = false
		}
	}

	if !shouldStop {
		return
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := app.Stop(stopCtx); err != nil {
		log.TLogln("Stop error:", err)
	}
}

func parseArgs(_ []string) (*settings.ExecArgs, error) {
	var parsed settings.ExecArgs
	p := arg.MustParse(&parsed)

	if p.Subcommand() != nil {
		p.WriteHelp(os.Stdout)
		os.Exit(0)
	}

	return &parsed, nil
}

func loadConfig(args *settings.ExecArgs) (*config.Config, error) {
	configPath := os.Getenv("TS_CONFIG")

	if configPath == "" && args.Path != "" {
		configPath = args.Path + "/config.yml"
	}

	cfg, err := config.Load(configPath)

	if err != nil {
		return nil, err
	}

	// Set static configuration from config.yaml
	settings.SetStaticConfig(cfg.ToStaticConfig())

	if args.Ssl {
		cfg.Server.SSL = true
	}

	if args.SslCert != "" {
		cfg.Server.SSLCert = args.SslCert
		cfg.Server.SSLKey = args.SslKey
	}

	return cfg, nil
}
