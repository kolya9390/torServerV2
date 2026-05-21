package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

func cmdShutdown(cli *apiClient, opts globalOptions, args []string) error {
	fs := flag.NewFlagSet("shutdown", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	mode := fs.String("mode", "local", "shutdown mode: local|public")
	reason := fs.String("reason", defaultReason, "shutdown reason")

	if err := fs.Parse(args); err != nil {
		return err
	}

	headers := map[string]string{}

	if strings.EqualFold(strings.TrimSpace(*mode), "public") {
		if strings.TrimSpace(opts.Token) == "" {
			return errors.New("shutdown --mode public requires global --token")
		}

		headers["X-TS-Shutdown-Token"] = strings.TrimSpace(opts.Token)
	}

	path := "/api/v1/shutdown"

	if strings.TrimSpace(*reason) != "" {
		path += "/" + url.PathEscape(strings.TrimSpace(*reason))
	}

	ctx, cancel := context.WithTimeout(context.Background(), maxDuration(opts.Timeout, 5*time.Second))
	defer cancel()

	if err := cli.doJSON(ctx, "POST", path, nil, nil, headers); err != nil {
		return err
	}

	fmt.Println("OK: shutdown accepted")

	return nil
}
