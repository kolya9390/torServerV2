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

	_ "server/docs"

	"server/internal/daemon"
)

func main() {
	result := daemon.Run(daemon.Invocation{
		Context: context.Background(),
		Args:    os.Args[1:],
		Stdout:  os.Stdout,
	}, daemon.DefaultDependencies())
	if result.Err != nil {
		if _, err := fmt.Fprintln(os.Stderr, daemon.UserMessage(result.Err)); err != nil {
			os.Exit(daemon.ExitFailure)
		}
	}

	if result.ExitCode != daemon.ExitOK {
		os.Exit(result.ExitCode)
	}
}
