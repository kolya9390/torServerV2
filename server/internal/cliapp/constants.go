package cliapp

import (
	"time"

	"server/internal/apiclient"
)

const (
	// Default server configuration.
	defaultServerURL   = apiclient.DefaultBaseURL
	defaultContextName = "local"
	defaultHTTPPort    = "8090"
	defaultHTTPSPort   = "8091"

	// Default CLI behavior.
	defaultTimeout = 15 * time.Second
	defaultOutput  = "table"
	defaultReason  = "torrctl"

	// Environment variable names.
	envContext  = "TSCTL_CONTEXT"
	envConfig   = "TSCTL_CONFIG"
	envUser     = "TS_USER"
	envPassword = "TS_PASSWORD"
	envToken    = "TS_SHUTDOWN_TOKEN" // #nosec G101 -- environment variable name, not a secret value.

	// Output formats.
	outputTable = "table"
	outputJSON  = "json"
)

// ValidOutputFormats returns all supported output format strings.
func ValidOutputFormats() []string {
	return []string{outputTable, outputJSON}
}
