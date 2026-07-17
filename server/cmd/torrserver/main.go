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
	"io"
	"os"

	_ "server/docs"

	"server/internal/daemon"
)

type daemonRunner func(daemon.Invocation, daemon.Dependencies) daemon.Result

func main() {
	os.Exit(execute(
		context.Background(),
		os.Args[1:],
		os.Stdout,
		os.Stderr,
		daemon.DefaultDependencies(),
		daemon.Run,
	))
}

func execute(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	dependencies daemon.Dependencies,
	run daemonRunner,
) int {
	result := run(daemon.Invocation{
		Context: ctx,
		Args:    args,
		Stdout:  stdout,
	}, dependencies)
	if result.Err != nil {
		if _, err := fmt.Fprintln(stderr, daemon.UserMessage(result.Err)); err != nil {
			return daemon.ExitFailure
		}
	}

	return result.ExitCode
}
