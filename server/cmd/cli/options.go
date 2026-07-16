package cli

import (
	"context"
	"io"
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
	stdout           io.Writer
	stderr           io.Writer
	ctx              context.Context
	isTerminal       func() bool
	readPassword     func(io.Writer) (string, error)
	readNewPassword  func(io.Writer) (string, error)
}

func (opts globalOptions) stdoutWriter() io.Writer {
	if opts.stdout != nil {
		return opts.stdout
	}

	return io.Discard
}

func (opts globalOptions) stderrWriter() io.Writer {
	if opts.stderr != nil {
		return opts.stderr
	}

	return io.Discard
}

func (opts globalOptions) commandContext() context.Context {
	if opts.ctx != nil {
		return opts.ctx
	}

	return context.Background()
}

func (opts globalOptions) timeoutContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(opts.commandContext(), timeout)
}

func (opts globalOptions) promptPassword(output io.Writer) (string, error) {
	if opts.readPassword != nil {
		return opts.readPassword(output)
	}

	return readPasswordInteractive(output)
}

func (opts globalOptions) promptNewPassword(output io.Writer) (string, error) {
	if opts.readNewPassword != nil {
		return opts.readNewPassword(output)
	}

	return readPasswordInteractively(output)
}
