package cli

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
)

const (
	shutdownTokenPath         = "/api/v1/config/shutdown-token" // #nosec G101 -- HTTP endpoint path, not a credential.
	shutdownTokenGeneratePath = shutdownTokenPath + "/generate"
	minimumShutdownTokenBytes = 16
)

type shutdownTokenStatus struct {
	Configured bool `json:"configured"`
}

type generatedShutdownToken struct {
	Status string `json:"status"`
	Token  string `json:"token"` // #nosec G117 -- the API must return a newly generated token exactly once.
}

func cmdShutdownTokenStatus(cli *apiClient, opts globalOptions) error {
	ctx, cancel := opts.timeoutContext(opts.Timeout)
	defer cancel()

	var status shutdownTokenStatus

	if err := cli.doJSON(ctx, http.MethodGet, shutdownTokenPath, nil, &status, nil); err != nil {
		return err
	}

	return writeCommandResult(
		opts,
		status,
		"Shutdown token configured: "+strconv.FormatBool(status.Configured),
	)
}

func cmdGenerateShutdownToken(cli *apiClient, opts globalOptions) error {
	ctx, cancel := opts.timeoutContext(opts.Timeout)
	defer cancel()

	var generated generatedShutdownToken

	if err := cli.doJSON(ctx, http.MethodPost, shutdownTokenGeneratePath, nil, &generated, nil); err != nil {
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

func cmdSetShutdownToken(cli *apiClient, opts globalOptions) error {
	token := strings.TrimSpace(opts.Token)
	if len(token) < minimumShutdownTokenBytes {
		return errors.New("shutdown token must contain at least 16 characters; set TS_SHUTDOWN_TOKEN or global --token")
	}

	ctx, cancel := opts.timeoutContext(opts.Timeout)
	defer cancel()

	payload := map[string]string{"token": token}
	if err := cli.doJSON(ctx, http.MethodPost, shutdownTokenPath, payload, nil, nil); err != nil {
		return err
	}

	return writeCommandResult(
		opts,
		map[string]any{"status": "configured"},
		"Shutdown token configured",
	)
}
