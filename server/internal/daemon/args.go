package daemon

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/alexflint/go-arg"

	"server/settings"
	buildversion "server/version"
)

const serveCommand = "serve"

var managementCommands = map[string]string{
	"auth":       "torrctl auth",
	"completion": "torrctl completion",
	"config":     "torrctl config",
	"context":    "torrctl context",
	"settings":   "torrctl settings",
	"shutdown":   "torrctl shutdown",
	"status":     "torrctl status",
	"torrents":   "torrctl torrents",
	"url":        "torrctl url",
}

var managementValueFlags = map[string]struct{}{
	"--context": {},
	"--output":  {},
	"--pass":    {},
	"--server":  {},
	"--timeout": {},
	"--token":   {},
	"--user":    {},
}

// managementCommandError redirects an invocation from the former mixed
// binary without retaining flags or values that may contain credentials.
type managementCommandError struct {
	Command     string
	Replacement string
}

func (err *managementCommandError) Error() string {
	return err.UserMessage()
}

func (err *managementCommandError) UserMessage() string {
	if err == nil {
		return "torrserver: management commands moved to torrctl"
	}

	return fmt.Sprintf(
		"torrserver %s: management commands moved to torrctl; use `%s`",
		err.Command,
		err.Replacement,
	)
}

// invalidArgumentsError deliberately hides raw argv values from process output.
type invalidArgumentsError struct {
	cause error
}

func (err *invalidArgumentsError) Error() string {
	return err.UserMessage()
}

func (err *invalidArgumentsError) UserMessage() string {
	return "torrserver: invalid daemon arguments; usage: torrserver serve [flags]; " +
		"run `torrserver serve --help`"
}

func (err *invalidArgumentsError) Unwrap() error {
	if err == nil {
		return nil
	}

	return err.cause
}

func parseProcessArgs(args []string, output io.Writer, buildInfo buildversion.Info) (*settings.ExecArgs, error) {
	args, explicitServe := normalizeServeArgs(args)
	if !explicitServe {
		if command, replacement, ok := formerManagementCommand(args); ok {
			return nil, &managementCommandError{Command: command, Replacement: replacement}
		}
	}

	if isVersionRequest(args) {
		_, err := fmt.Fprintln(output, buildversion.Concise("torrserver", buildInfo))
		if err != nil {
			return nil, fmt.Errorf("write version: %w", err)
		}

		return nil, errVersionRequested
	}

	var parsed settings.ExecArgs

	parser, err := arg.NewParser(arg.Config{
		Program: "torrserver serve",
		Out:     output,
		Exit:    func(int) {},
	}, &parsed)
	if err != nil {
		return nil, fmt.Errorf("create argument parser: %w", err)
	}

	if err := parser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(output)

			return nil, errHelpRequested
		}

		return nil, &invalidArgumentsError{cause: err}
	}

	return &parsed, nil
}

func normalizeServeArgs(args []string) ([]string, bool) {
	if len(args) == 0 || args[0] != serveCommand {
		return args, false
	}

	return args[1:], true
}

func isVersionRequest(args []string) bool {
	return len(args) == 1 && (args[0] == "--version" || args[0] == "-v")
}

func formerManagementCommand(args []string) (string, string, bool) {
	for idx := 0; idx < len(args); idx++ {
		value := args[idx]
		if replacement, ok := managementCommands[value]; ok {
			return value, replacement, true
		}

		if value == "--insecure" {
			continue
		}

		flag, inlineValue := splitFlag(value)
		if _, ok := managementValueFlags[flag]; !ok {
			return "", "", false
		}

		if !inlineValue {
			idx++
		}
	}

	return "", "", false
}

func splitFlag(value string) (string, bool) {
	flag, _, found := strings.Cut(value, "=")

	return flag, found
}
