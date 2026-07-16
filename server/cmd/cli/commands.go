package cli

import (
	"fmt"
	"strings"
	"time"
)

func cmdStatus(cli *apiClient, opts globalOptions) error {
	ctx, cancel := opts.timeoutContext(opts.Timeout)
	defer cancel()

	version := map[string]any{}

	if err := cli.doJSON(ctx, "GET", "/api/v1/version", nil, &version, nil); err != nil {
		return err
	}

	ready := map[string]any{}

	if err := cli.doJSON(ctx, "GET", "/readyz", nil, &ready, nil); err != nil {
		return err
	}

	if opts.Output == "json" {
		return writeJSONSuccess(opts.stdoutWriter(), map[string]any{
			"server":  redactURLCredentials(cli.baseURL.String()),
			"version": version,
			"ready":   ready,
		})
	}

	lines := []string{"Server: " + redactURLCredentials(cli.baseURL.String())}
	if strings.TrimSpace(opts.Context) != "" {
		lines = append(lines, "Context: "+opts.Context)
	}

	lines = append(
		lines,
		fmt.Sprintf("Version: %v", version["current"]),
		fmt.Sprintf("Ready: %v", ready["status"]),
		fmt.Sprintf("HTTP: %v", ready["http"]),
		fmt.Sprintf("Torrent: %v", ready["torrent"]),
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
