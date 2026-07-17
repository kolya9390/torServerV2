package cliapp

import (
	"fmt"
	"strings"
	"time"
)

func cmdStatus(cli statusAPI, opts globalOptions) error {
	ctx, cancel := opts.timeoutContext(opts.Timeout)
	defer cancel()

	version, err := cli.Version(ctx)
	if err != nil {
		return err
	}

	ready, err := cli.Readiness(ctx)
	if err != nil {
		return err
	}

	if opts.Output == "json" {
		return writeJSONSuccess(opts.stdoutWriter(), map[string]any{
			"server":  redactURLCredentials(cli.BaseURL()),
			"version": version,
			"ready":   ready,
		})
	}

	lines := []string{"Server: " + redactURLCredentials(cli.BaseURL())}
	if strings.TrimSpace(opts.Context) != "" {
		lines = append(lines, "Context: "+opts.Context)
	}

	lines = append(
		lines,
		fmt.Sprintf("Product: %v", version.Product),
		fmt.Sprintf("Application: %v", version.ApplicationVersion),
		fmt.Sprintf("API: %v", version.Current),
		fmt.Sprintf("Ready: %v", ready.Status),
		fmt.Sprintf("HTTP: %v", ready.HTTP),
		fmt.Sprintf("Torrent: %v", ready.Torrent),
	)

	return writeTextLines(opts.stdoutWriter(), lines...)
}

func shortHash(hash string) string {
	if len(hash) <= 12 {
		return hash
	}

	return hash[:12]
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}

	return b
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}

	return b
}
