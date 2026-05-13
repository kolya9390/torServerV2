package cli

import "time"

const (
	// Default server configuration.
	defaultServerURL   = "http://127.0.0.1:8090"
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
	envToken    = "TS_SHUTDOWN_TOKEN"

	// Output formats.
	outputTable = "table"
	outputJSON  = "json"
)

// ValidOutputFormats returns all supported output format strings.
func ValidOutputFormats() []string {
	return []string{outputTable, outputJSON}
}
