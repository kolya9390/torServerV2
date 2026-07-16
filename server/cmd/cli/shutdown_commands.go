package cli

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

func cmdShutdown(cli *apiClient, opts globalOptions, mode, reason string) error {
	headers := map[string]string{}

	if strings.EqualFold(strings.TrimSpace(mode), "public") {
		if strings.TrimSpace(opts.Token) == "" {
			return errors.New("shutdown --mode public requires global --token")
		}

		headers["X-TS-Shutdown-Token"] = strings.TrimSpace(opts.Token)
	}

	path := "/api/v1/shutdown"

	if strings.TrimSpace(reason) != "" {
		path += "/" + url.PathEscape(strings.TrimSpace(reason))
	}

	ctx, cancel := opts.timeoutContext(maxDuration(opts.Timeout, 5*time.Second))
	defer cancel()

	if err := cli.doJSON(ctx, "POST", path, nil, nil, headers); err != nil {
		return err
	}

	return writeCommandResult(
		opts,
		map[string]any{"action": "shutdown_accepted", "mode": strings.ToLower(strings.TrimSpace(mode))},
		"OK: shutdown accepted",
	)
}
