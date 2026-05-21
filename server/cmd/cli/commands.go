package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func cmdStatus(cli *apiClient, opts globalOptions) error {
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
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
		return printJSON(map[string]any{
			"server":  cli.baseURL.String(),
			"version": version,
			"ready":   ready,
		})
	}

	fmt.Printf("Server: %s\n", cli.baseURL.String())

	if strings.TrimSpace(opts.Context) != "" {
		fmt.Printf("Context: %s\n", opts.Context)
	}

	fmt.Printf("Version: %v\n", version["current"])
	fmt.Printf("Ready: %v\n", ready["status"])
	fmt.Printf("HTTP: %v\n", ready["http"])
	fmt.Printf("Torrent: %v\n", ready["torrent"])

	return nil
}

func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")

	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
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
