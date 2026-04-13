package cli

import (
	"time"
)

// globalOptions holds CLI-wide settings available to all commands.
type globalOptions struct {
	Server           string
	User             string
	Pass             string
	Token            string
	Context          string
	Timeout          time.Duration
	Insecure         bool
	Output           string
	insecureExplicit bool
}
