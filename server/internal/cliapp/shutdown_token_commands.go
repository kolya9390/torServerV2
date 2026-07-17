package cliapp

import (
	"errors"
	"strconv"
	"strings"
)

const (
	minimumShutdownTokenBytes = 16
)

func cmdShutdownTokenStatus(cli shutdownTokenAPI, opts globalOptions) error {
	ctx, cancel := opts.timeoutContext(opts.Timeout)
	defer cancel()

	status, err := cli.ShutdownTokenStatus(ctx)
	if err != nil {
		return err
	}

	return writeCommandResult(
		opts,
		status,
		"Shutdown token configured: "+strconv.FormatBool(status.Configured),
	)
}

func cmdGenerateShutdownToken(cli shutdownTokenAPI, opts globalOptions) error {
	ctx, cancel := opts.timeoutContext(opts.Timeout)
	defer cancel()

	generated, err := cli.GenerateShutdownToken(ctx)
	if err != nil {
		return err
	}

	generated.Token = strings.TrimSpace(generated.Token)
	if generated.Token == "" {
		return errors.New("server returned an empty generated shutdown token")
	}

	if opts.Output == outputJSON {
		return writeJSONSuccess(opts.stdoutWriter(), generated)
	}

	return writeTextLine(opts.stdoutWriter(), generated.Token)
}

func cmdSetShutdownToken(cli shutdownTokenAPI, opts globalOptions) error {
	token := strings.TrimSpace(opts.Token)
	if len(token) < minimumShutdownTokenBytes {
		return errors.New("shutdown token must contain at least 16 characters; set TS_SHUTDOWN_TOKEN or global --token")
	}

	ctx, cancel := opts.timeoutContext(opts.Timeout)
	defer cancel()

	if err := cli.SetShutdownToken(ctx, token); err != nil {
		return err
	}

	return writeCommandResult(
		opts,
		map[string]any{"status": "configured"},
		"Shutdown token configured",
	)
}
