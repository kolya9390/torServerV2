package cliapp

import (
	"errors"
	"strings"
	"time"

	"server/internal/apiclient"
)

func cmdShutdown(cli shutdownAPI, opts globalOptions, mode, reason string) error {
	if strings.EqualFold(strings.TrimSpace(mode), "public") {
		if strings.TrimSpace(opts.Token) == "" {
			return errors.New("shutdown --mode public requires global --token")
		}
	}

	ctx, cancel := opts.timeoutContext(maxDuration(opts.Timeout, 5*time.Second))
	defer cancel()

	if err := cli.Shutdown(ctx, apiclient.ShutdownRequest{
		Mode:   mode,
		Reason: reason,
		Token:  opts.Token,
	}); err != nil {
		return err
	}

	return writeCommandResult(
		opts,
		map[string]any{"action": "shutdown_accepted", "mode": strings.ToLower(strings.TrimSpace(mode))},
		"OK: shutdown accepted",
	)
}
